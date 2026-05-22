package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"log/slog"
	"strings"
	"time"

	"space/backend/internal/storage"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errNoPreviewJobs = errors.New("no preview jobs available")

type Runner struct {
	DB           *pgxpool.Pool
	Storage      storage.Interface
	PollInterval time.Duration
	MaxAttempts  int
}

type previewJob struct {
	ID       string
	FileID   string
	JobType  string
	Attempts int
}

type fileForMetadata struct {
	ID             string
	OwnerID        string
	Name           string
	OriginalName   string
	StorageKey     string
	SizeBytes      int64
	MimeType       *string
	Extension      *string
	ChecksumSHA256 *string
	UpdatedAt      time.Time
}

func (r Runner) Start(ctx context.Context) {
	interval := r.PollInterval
	if interval <= 0 {
		interval = 3 * time.Second
	}
	maxAttempts := r.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("preview worker started", "poll_interval", interval.String(), "max_attempts", maxAttempts)
	for {
		select {
		case <-ctx.Done():
			slog.Info("preview worker stopped")
			return
		default:
		}

		processed, err := r.processOne(ctx, maxAttempts)
		if err != nil && !errors.Is(err, errNoPreviewJobs) {
			slog.Error("preview worker process failed", "error", err)
		}
		if processed {
			continue
		}

		select {
		case <-ctx.Done():
			slog.Info("preview worker stopped")
			return
		case <-ticker.C:
		}
	}
}

func (r Runner) processOne(ctx context.Context, maxAttempts int) (bool, error) {
	job, err := r.claimNextJob(ctx)
	if err != nil {
		if errors.Is(err, errNoPreviewJobs) {
			return false, errNoPreviewJobs
		}
		return false, err
	}

	switch job.JobType {
	case "metadata":
		if err := r.generateMetadataPreview(ctx, job); err != nil {
			return true, r.markJobFailure(ctx, job, maxAttempts, err)
		}
	case "thumbnail":
		if err := r.generateThumbnailPreview(ctx, job); err != nil {
			return true, r.markJobFailure(ctx, job, maxAttempts, err)
		}
	case "office_to_pdf":
		return true, r.markJobFailure(ctx, job, maxAttempts, fmt.Errorf("office preview conversion is not available in this worker image"))
	default:
		return true, r.markJobFailure(ctx, job, maxAttempts, fmt.Errorf("unsupported preview job type: %s", job.JobType))
	}

	return true, nil
}

func (r Runner) claimNextJob(ctx context.Context) (previewJob, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return previewJob{}, err
	}
	defer tx.Rollback(ctx)

	var job previewJob
	err = tx.QueryRow(ctx, `
		WITH next_job AS (
			SELECT id
			FROM preview_jobs
			WHERE status='queued'
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE preview_jobs p
		SET status='processing', attempts=p.attempts + 1, updated_at=now()
		FROM next_job
		WHERE p.id = next_job.id
		RETURNING p.id, p.file_id, p.job_type, p.attempts
	`).Scan(&job.ID, &job.FileID, &job.JobType, &job.Attempts)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return previewJob{}, errNoPreviewJobs
		}
		return previewJob{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return previewJob{}, err
	}
	return job, nil
}

func (r Runner) generateMetadataPreview(ctx context.Context, job previewJob) error {
	file, err := r.fetchFileForMetadata(ctx, job.FileID)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"fileId":         file.ID,
		"ownerId":        file.OwnerID,
		"name":           file.Name,
		"originalName":   file.OriginalName,
		"storageKey":     file.StorageKey,
		"sizeBytes":      file.SizeBytes,
		"mimeType":       file.MimeType,
		"extension":      file.Extension,
		"checksumSha256": file.ChecksumSHA256,
		"updatedAt":      file.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"generatedAt":    time.Now().UTC().Format(time.RFC3339Nano),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	outputKey := fmt.Sprintf("previews/metadata/%s.json", file.ID)
	if err := r.Storage.PutStream(ctx, outputKey, bytes.NewReader(body)); err != nil {
		return err
	}

	if err := r.upsertFilePreview(ctx, file.ID, "metadata", outputKey, "application/json", int64(len(body))); err != nil {
		return err
	}

	_, err = r.DB.Exec(ctx, `
		UPDATE preview_jobs
		SET status='completed', output_storage_key=$1, error_message=NULL, updated_at=now()
		WHERE id=$2
	`, outputKey, job.ID)
	return err
}

func (r Runner) generateThumbnailPreview(ctx context.Context, job previewJob) error {
	file, err := r.fetchFileForMetadata(ctx, job.FileID)
	if err != nil {
		return err
	}

	stream, err := r.Storage.GetStream(ctx, file.StorageKey)
	if err != nil {
		return err
	}
	defer stream.Close()

	src, _, err := image.Decode(stream)
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}
	thumb := resizeImageContain(src, 320, 320)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, thumb, &jpeg.Options{Quality: 82}); err != nil {
		return err
	}

	outputKey := fmt.Sprintf("previews/thumbnails/%s.jpg", file.ID)
	if err := r.Storage.PutStream(ctx, outputKey, bytes.NewReader(out.Bytes())); err != nil {
		return err
	}
	if err := r.upsertFilePreview(ctx, file.ID, "thumbnail", outputKey, "image/jpeg", int64(out.Len())); err != nil {
		return err
	}

	_, err = r.DB.Exec(ctx, `
		UPDATE preview_jobs
		SET status='completed', output_storage_key=$1, error_message=NULL, updated_at=now()
		WHERE id=$2
	`, outputKey, job.ID)
	return err
}

func (r Runner) upsertFilePreview(ctx context.Context, fileID string, previewType string, storageKey string, mimeType string, sizeBytes int64) error {
	_, err := r.DB.Exec(ctx, `
		INSERT INTO file_previews (id, file_id, preview_type, storage_key, mime_type, size_bytes, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,'ready',now(),now())
		ON CONFLICT (file_id, preview_type)
		DO UPDATE SET
			storage_key=EXCLUDED.storage_key,
			mime_type=EXCLUDED.mime_type,
			size_bytes=EXCLUDED.size_bytes,
			status='ready',
			updated_at=now()
	`, uuid.NewString(), fileID, previewType, storageKey, mimeType, sizeBytes)
	return err
}

func resizeImageContain(src image.Image, maxW int, maxH int) image.Image {
	b := src.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	if srcW <= 0 || srcH <= 0 {
		return src
	}
	if srcW <= maxW && srcH <= maxH {
		return src
	}

	scaleW := float64(maxW) / float64(srcW)
	scaleH := float64(maxH) / float64(srcH)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}
	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	for y := 0; y < dstH; y++ {
		srcY := b.Min.Y + (y*srcH)/dstH
		for x := 0; x < dstW; x++ {
			srcX := b.Min.X + (x*srcW)/dstW
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func (r Runner) fetchFileForMetadata(ctx context.Context, fileID string) (fileForMetadata, error) {
	var file fileForMetadata
	err := r.DB.QueryRow(ctx, `
		SELECT id, owner_id, name, original_name, storage_key, size_bytes, mime_type, extension, checksum_sha256, updated_at
		FROM files
		WHERE id=$1 AND deleted_at IS NULL
	`, fileID).Scan(
		&file.ID,
		&file.OwnerID,
		&file.Name,
		&file.OriginalName,
		&file.StorageKey,
		&file.SizeBytes,
		&file.MimeType,
		&file.Extension,
		&file.ChecksumSHA256,
		&file.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fileForMetadata{}, fmt.Errorf("file not found for preview job")
		}
		return fileForMetadata{}, err
	}
	return file, nil
}

func (r Runner) markJobFailure(ctx context.Context, job previewJob, maxAttempts int, processErr error) error {
	message := strings.TrimSpace(processErr.Error())
	if message == "" {
		message = "preview processing failed"
	}

	status := "queued"
	if job.Attempts >= maxAttempts {
		status = "failed"
	}

	_, err := r.DB.Exec(ctx, `
		UPDATE preview_jobs
		SET status=$1, error_message=$2, updated_at=now()
		WHERE id=$3
	`, status, message, job.ID)
	if err != nil {
		return err
	}

	if status == "failed" {
		slog.Warn("preview job failed permanently", "job_id", job.ID, "attempts", job.Attempts, "error", message)
	} else {
		slog.Warn("preview job requeued", "job_id", job.ID, "attempts", job.Attempts, "error", message)
	}
	return nil
}
