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
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"space/backend/internal/observability"
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
		if err := r.generateOfficePDFPreview(ctx, job); err != nil {
			return true, r.markJobFailure(ctx, job, maxAttempts, err)
		}
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
	if mediaMeta, metaErr := r.extractMediaMetadata(ctx, file); metaErr == nil && mediaMeta != nil {
		payload["media"] = mediaMeta
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

func (r Runner) extractMediaMetadata(ctx context.Context, file fileForMetadata) (map[string]any, error) {
	mimeType := ""
	if file.MimeType != nil {
		mimeType = strings.ToLower(strings.TrimSpace(*file.MimeType))
	}
	if !strings.HasPrefix(mimeType, "video/") && !strings.HasPrefix(mimeType, "audio/") {
		return nil, nil
	}

	tmpDir, err := os.MkdirTemp("", "space-media-meta-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	ext := "bin"
	if file.Extension != nil && strings.TrimSpace(*file.Extension) != "" {
		ext = strings.TrimPrefix(strings.TrimSpace(*file.Extension), ".")
	}
	sourcePath := filepath.Join(tmpDir, "source."+ext)
	sourceFile, err := os.Create(sourcePath)
	if err != nil {
		return nil, err
	}

	stream, err := r.Storage.GetStream(ctx, file.StorageKey)
	if err != nil {
		sourceFile.Close()
		return nil, err
	}
	if _, err := io.Copy(sourceFile, stream); err != nil {
		stream.Close()
		sourceFile.Close()
		return nil, err
	}
	stream.Close()
	if err := sourceFile.Close(); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(
		ctx,
		"ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		sourcePath,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (r Runner) generateThumbnailPreview(ctx context.Context, job previewJob) error {
	file, err := r.fetchFileForMetadata(ctx, job.FileID)
	if err != nil {
		return err
	}

	if isVideoFile(file) {
		return r.generateVideoThumbnailPreview(ctx, job, file)
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

func (r Runner) generateVideoThumbnailPreview(ctx context.Context, job previewJob, file fileForMetadata) error {
	tmpDir, err := os.MkdirTemp("", "space-video-thumb-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ext := "bin"
	if file.Extension != nil && strings.TrimSpace(*file.Extension) != "" {
		ext = strings.TrimPrefix(strings.TrimSpace(*file.Extension), ".")
	}
	sourcePath := filepath.Join(tmpDir, "source."+ext)
	sourceFile, err := os.Create(sourcePath)
	if err != nil {
		return err
	}

	stream, err := r.Storage.GetStream(ctx, file.StorageKey)
	if err != nil {
		sourceFile.Close()
		return err
	}
	if _, err := io.Copy(sourceFile, stream); err != nil {
		stream.Close()
		sourceFile.Close()
		return err
	}
	stream.Close()
	if err := sourceFile.Close(); err != nil {
		return err
	}

	outPath := filepath.Join(tmpDir, "thumb.jpg")
	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-ss", "00:00:01",
		"-i", sourcePath,
		"-frames:v", "1",
		"-vf", "scale='min(320,iw)':-1",
		outPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Retry without seek for very short videos.
		cmd2 := exec.CommandContext(
			ctx,
			"ffmpeg",
			"-y",
			"-i", sourcePath,
			"-frames:v", "1",
			"-vf", "scale='min(320,iw)':-1",
			outPath,
		)
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			return fmt.Errorf("ffmpeg thumbnail failed: %v (%s / %s)", err2, strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
		}
	}

	thumbBytes, err := os.ReadFile(outPath)
	if err != nil {
		return err
	}
	if len(thumbBytes) == 0 {
		return fmt.Errorf("video thumbnail is empty")
	}

	outputKey := fmt.Sprintf("previews/thumbnails/%s.jpg", file.ID)
	if err := r.Storage.PutStream(ctx, outputKey, bytes.NewReader(thumbBytes)); err != nil {
		return err
	}
	if err := r.upsertFilePreview(ctx, file.ID, "thumbnail", outputKey, "image/jpeg", int64(len(thumbBytes))); err != nil {
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

func (r Runner) generateOfficePDFPreview(ctx context.Context, job previewJob) error {
	file, err := r.fetchFileForMetadata(ctx, job.FileID)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "space-office-preview-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ext := "bin"
	if file.Extension != nil && strings.TrimSpace(*file.Extension) != "" {
		ext = strings.TrimPrefix(strings.TrimSpace(*file.Extension), ".")
	}
	sourceName := "source." + ext
	sourcePath := filepath.Join(tmpDir, sourceName)
	sourceFile, err := os.Create(sourcePath)
	if err != nil {
		return err
	}

	stream, err := r.Storage.GetStream(ctx, file.StorageKey)
	if err != nil {
		sourceFile.Close()
		return err
	}
	if _, err := io.Copy(sourceFile, stream); err != nil {
		stream.Close()
		sourceFile.Close()
		return err
	}
	stream.Close()
	if err := sourceFile.Close(); err != nil {
		return err
	}

	timeout := r.loadOfficeTimeout(ctx)
	convertCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(
		convertCtx,
		"soffice",
		"--headless",
		"--norestore",
		"--nolockcheck",
		"--nodefault",
		"--convert-to", "pdf",
		"--outdir", tmpDir,
		sourcePath,
	)
	output, err := cmd.CombinedOutput()
	if convertCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("office conversion timed out after %s", timeout.String())
	}
	if err != nil {
		return fmt.Errorf("soffice conversion failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}

	pdfPath := filepath.Join(tmpDir, strings.TrimSuffix(sourceName, filepath.Ext(sourceName))+".pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		entries, scanErr := os.ReadDir(tmpDir)
		if scanErr != nil {
			return fmt.Errorf("converted PDF not found")
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.EqualFold(filepath.Ext(entry.Name()), ".pdf") {
				pdfPath = filepath.Join(tmpDir, entry.Name())
				break
			}
		}
		if _, err := os.Stat(pdfPath); err != nil {
			return fmt.Errorf("converted PDF not found")
		}
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		return err
	}
	if len(pdfBytes) == 0 {
		return fmt.Errorf("converted PDF is empty")
	}

	outputKey := fmt.Sprintf("previews/pdf/%s.pdf", file.ID)
	if err := r.Storage.PutStream(ctx, outputKey, bytes.NewReader(pdfBytes)); err != nil {
		return err
	}
	if err := r.upsertFilePreview(ctx, file.ID, "pdf", outputKey, "application/pdf", int64(len(pdfBytes))); err != nil {
		return err
	}

	_, err = r.DB.Exec(ctx, `
		UPDATE preview_jobs
		SET status='completed', output_storage_key=$1, error_message=NULL, updated_at=now()
		WHERE id=$2
	`, outputKey, job.ID)
	return err
}

func (r Runner) loadOfficeTimeout(ctx context.Context) time.Duration {
	const fallback = 120 * time.Second

	var raw json.RawMessage
	err := r.DB.QueryRow(ctx, `SELECT value FROM system_settings WHERE key='preview.office_conversion_timeout_seconds'`).Scan(&raw)
	if err != nil {
		return fallback
	}
	var seconds int64
	if err := json.Unmarshal(raw, &seconds); err != nil || seconds <= 0 {
		return fallback
	}
	if seconds > 1800 {
		seconds = 1800
	}
	return time.Duration(seconds) * time.Second
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

func isVideoFile(file fileForMetadata) bool {
	if file.MimeType != nil {
		mimeType := strings.ToLower(strings.TrimSpace(*file.MimeType))
		if strings.HasPrefix(mimeType, "video/") {
			return true
		}
	}
	if file.Extension != nil {
		switch strings.ToLower(strings.TrimSpace(*file.Extension)) {
		case "mp4", "webm", "mov", "mkv", "m4v", "avi", "ogv":
			return true
		}
	}
	return false
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
	observability.IncPreviewJobFailure()

	if status == "failed" {
		slog.Warn("preview job failed permanently", "job_id", job.ID, "attempts", job.Attempts, "error", message)
	} else {
		slog.Warn("preview job requeued", "job_id", job.ID, "attempts", job.Attempts, "error", message)
	}
	return nil
}
