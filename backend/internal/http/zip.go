package httpapi

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"space/backend/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type zipFileEntry struct {
	FileID      string
	StorageKey  string
	FileName    string
	RelativeDir string
	UpdatedAt   time.Time
}

func (h Handler) downloadFolderZip(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	if !h.enforceRateLimitWithSubject(w, r, "zip_download_private", user.ID, h.Cfg.ZipDownloadRatePerMin) {
		return
	}

	folderID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(folderID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder id"})
		return
	}

	var folderName string
	err := h.DB.QueryRow(r.Context(), `SELECT name FROM folders WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL`, folderID, user.ID).Scan(&folderName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load folder"})
		return
	}

	h.streamFolderZip(w, r, folderID, user.ID, normalizeNodeName(folderName)+".zip")
}

func (h Handler) streamFolderZip(w http.ResponseWriter, r *http.Request, rootFolderID string, ownerID string, zipFileName string) {
	entries, err := h.collectFolderZipEntries(r, rootFolderID, ownerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not prepare folder zip"})
		return
	}

	if strings.TrimSpace(zipFileName) == "" {
		zipFileName = "folder.zip"
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", zipFileName))
	w.Header().Set("Cache-Control", "no-store")

	pipeR, pipeW := io.Pipe()
	go func() {
		zw := zip.NewWriter(pipeW)
		for _, entry := range entries {
			stream, err := h.Storage.GetStream(r.Context(), entry.StorageKey)
			if err != nil {
				_ = pipeW.CloseWithError(err)
				return
			}

			name := path.Clean(path.Join(entry.RelativeDir, entry.FileName))
			if strings.HasPrefix(name, "../") || name == ".." {
				_ = stream.Close()
				_ = pipeW.CloseWithError(fmt.Errorf("invalid zip path"))
				return
			}

			hdr := &zip.FileHeader{
				Name:     name,
				Method:   zip.Deflate,
				Modified: entry.UpdatedAt,
			}
			writer, err := zw.CreateHeader(hdr)
			if err != nil {
				_ = stream.Close()
				_ = pipeW.CloseWithError(err)
				return
			}

			if _, err := io.Copy(writer, stream); err != nil {
				_ = stream.Close()
				_ = pipeW.CloseWithError(err)
				return
			}
			_ = stream.Close()
		}

		if err := zw.Close(); err != nil {
			_ = pipeW.CloseWithError(err)
			return
		}
		_ = pipeW.Close()
	}()

	_, _ = io.Copy(w, pipeR)
	_ = pipeR.Close()
}

func (h Handler) collectFolderZipEntries(r *http.Request, rootFolderID string, ownerID string) ([]zipFileEntry, error) {
	rows, err := h.DB.Query(r.Context(), `
		WITH RECURSIVE tree AS (
			SELECT id, owner_id, name, parent_id, name::text AS rel
			FROM folders
			WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL
			UNION ALL
			SELECT f.id, f.owner_id, f.name, f.parent_id, (t.rel || '/' || f.name)
			FROM folders f
			JOIN tree t ON f.parent_id = t.id
			WHERE f.owner_id=$2 AND f.deleted_at IS NULL
		)
		SELECT fi.id, fi.storage_key, fi.original_name, tree.rel, fi.updated_at
		FROM files fi
		JOIN tree ON fi.folder_id = tree.id
		WHERE fi.owner_id=$2 AND fi.deleted_at IS NULL
		ORDER BY tree.rel, fi.original_name
	`, rootFolderID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]zipFileEntry, 0, 256)
	for rows.Next() {
		var e zipFileEntry
		if err := rows.Scan(&e.FileID, &e.StorageKey, &e.FileName, &e.RelativeDir, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.FileName = normalizeNodeName(e.FileName)
		if e.FileName == "" {
			e.FileName = e.FileID
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}
