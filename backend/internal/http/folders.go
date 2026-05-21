package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"space/backend/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type itemType string

const (
	itemTypeFolder itemType = "folder"
	itemTypeFile   itemType = "file"
)

type browserItem struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       itemType  `json:"type"`
	ParentID   *string   `json:"parentId,omitempty"`
	SizeBytes  *int64    `json:"sizeBytes,omitempty"`
	MimeType   *string   `json:"mimeType,omitempty"`
	Extension  *string   `json:"extension,omitempty"`
	ModifiedAt time.Time `json:"modifiedAt"`
	CreatedAt  time.Time `json:"createdAt"`
}

type listItemsResponse struct {
	ParentID *string       `json:"parentId,omitempty"`
	Items    []browserItem `json:"items"`
}

func (h Handler) registerFolderRoutes(api chi.Router, authMW middleware.AuthMiddleware) {
	api.With(authMW.RequireAuth).Get("/folders/items", h.listFolderItems)
	api.With(authMW.RequireAuth).Post("/folders", h.createFolder)
	api.With(authMW.RequireAuth).Patch("/folders/{id}", h.renameFolder)
	api.With(authMW.RequireAuth).Delete("/folders/{id}", h.deleteFolder)
	api.With(authMW.RequireAuth).Post("/folders/{id}/move", h.moveFolder)
	api.With(authMW.RequireAuth).Get("/folders/{id}/download-zip", h.downloadFolderZip)
}

func (h Handler) listFolderItems(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	parentID, err := optionalUUIDFromQuery(r, "parentId")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.validateParentOwnership(r, user.ID, parentID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sortBy"))
	if sortBy == "" {
		sortBy = "name"
	}
	order := strings.TrimSpace(r.URL.Query().Get("order"))
	if order == "" {
		order = "asc"
	}

	items := make([]browserItem, 0, 256)

	folderRows, err := h.DB.Query(r.Context(), `
		SELECT id, name, parent_id, created_at, updated_at
		FROM folders
		WHERE owner_id = $1
		  AND deleted_at IS NULL
		  AND (($2::uuid IS NULL AND parent_id IS NULL) OR parent_id = $2::uuid)
		ORDER BY name ASC
	`, user.ID, parentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load folders"})
		return
	}
	defer folderRows.Close()

	for folderRows.Next() {
		var item browserItem
		item.Type = itemTypeFolder
		if err := folderRows.Scan(&item.ID, &item.Name, &item.ParentID, &item.CreatedAt, &item.ModifiedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not decode folder rows"})
			return
		}
		if matchesSearch(item.Name, search) {
			items = append(items, item)
		}
	}
	if err := folderRows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to iterate folder rows"})
		return
	}

	fileRows, err := h.DB.Query(r.Context(), `
		SELECT id, name, folder_id, size_bytes, mime_type, extension, created_at, updated_at
		FROM files
		WHERE owner_id = $1
		  AND deleted_at IS NULL
		  AND (($2::uuid IS NULL AND folder_id IS NULL) OR folder_id = $2::uuid)
		ORDER BY name ASC
	`, user.ID, parentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load files"})
		return
	}
	defer fileRows.Close()

	for fileRows.Next() {
		var item browserItem
		item.Type = itemTypeFile
		if err := fileRows.Scan(&item.ID, &item.Name, &item.ParentID, &item.SizeBytes, &item.MimeType, &item.Extension, &item.CreatedAt, &item.ModifiedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not decode file rows"})
			return
		}
		if matchesSearch(item.Name, search) {
			items = append(items, item)
		}
	}
	if err := fileRows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to iterate file rows"})
		return
	}

	sortBrowserItems(items, sortBy, order)
	writeJSON(w, http.StatusOK, listItemsResponse{ParentID: parentID, Items: items})
}

type createFolderRequest struct {
	ParentID *string `json:"parentId"`
	Name     string  `json:"name"`
}

func (h Handler) createFolder(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	var req createFolderRequest
	if err := ReadJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	name := normalizeNodeName(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "folder name is required"})
		return
	}

	parentID, err := optionalUUIDPtr(req.ParentID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.validateParentOwnership(r, user.ID, parentID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO folders (id, owner_id, parent_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, user.ID, parentID, name, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "an item with this name already exists in this folder"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create folder"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "folder.created", "folder", &id, clientIP(r), r.UserAgent(), map[string]any{"name": name, "parentId": parentID})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": name, "parentId": parentID})
}

type renameFolderRequest struct {
	Name string `json:"name"`
}

func (h Handler) renameFolder(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	folderID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(folderID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder id"})
		return
	}

	var req renameFolderRequest
	if err := ReadJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	name := normalizeNodeName(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	cmd, err := h.DB.Exec(r.Context(), `
		UPDATE folders
		SET name = $1, updated_at = now()
		WHERE id = $2 AND owner_id = $3 AND deleted_at IS NULL
	`, name, folderID, user.ID)
	if err != nil {
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "an item with this name already exists in this folder"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not rename folder"})
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "folder.renamed", "folder", &folderID, clientIP(r), r.UserAgent(), map[string]any{"name": name})
	writeJSON(w, http.StatusOK, map[string]any{"id": folderID, "name": name})
}

func (h Handler) deleteFolder(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	folderID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(folderID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder id"})
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not start delete transaction"})
		return
	}
	defer tx.Rollback(r.Context())

	var exists bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM folders WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL)`, folderID, user.ID).Scan(&exists); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not validate folder"})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	if _, err := tx.Exec(r.Context(), `
		WITH RECURSIVE subfolders AS (
			SELECT id FROM folders WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL
			UNION ALL
			SELECT f.id
			FROM folders f
			JOIN subfolders s ON f.parent_id = s.id
			WHERE f.owner_id=$2 AND f.deleted_at IS NULL
		)
		UPDATE folders
		SET deleted_at = now(), updated_at = now()
		WHERE id IN (SELECT id FROM subfolders)
	`, folderID, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete folder tree"})
		return
	}

	if _, err := tx.Exec(r.Context(), `
		WITH RECURSIVE subfolders AS (
			SELECT id FROM folders WHERE id=$1 AND owner_id=$2
			UNION ALL
			SELECT f.id
			FROM folders f
			JOIN subfolders s ON f.parent_id = s.id
			WHERE f.owner_id=$2
		)
		UPDATE files
		SET deleted_at = now(), updated_at = now(), status = 'deleted'
		WHERE owner_id=$2 AND deleted_at IS NULL AND folder_id IN (SELECT id FROM subfolders)
	`, folderID, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete child files"})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not commit delete operation"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "folder.deleted", "folder", &folderID, clientIP(r), r.UserAgent(), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type moveFolderRequest struct {
	ParentID *string `json:"parentId"`
}

func (h Handler) moveFolder(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	folderID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(folderID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder id"})
		return
	}

	var req moveFolderRequest
	if err := ReadJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	parentID, err := optionalUUIDPtr(req.ParentID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if parentID != nil && *parentID == folderID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot move folder into itself"})
		return
	}

	if err := h.validateParentOwnership(r, user.ID, parentID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if parentID != nil {
		var isDescendant bool
		err := h.DB.QueryRow(r.Context(), `
			WITH RECURSIVE subtree AS (
				SELECT id FROM folders WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL
				UNION ALL
				SELECT f.id FROM folders f JOIN subtree s ON f.parent_id = s.id
				WHERE f.owner_id=$2 AND f.deleted_at IS NULL
			)
			SELECT EXISTS(SELECT 1 FROM subtree WHERE id=$3)
		`, folderID, user.ID, *parentID).Scan(&isDescendant)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not validate destination folder"})
			return
		}
		if isDescendant {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot move folder into its own subtree"})
			return
		}
	}

	cmd, err := h.DB.Exec(r.Context(), `
		UPDATE folders
		SET parent_id=$1, updated_at=now()
		WHERE id=$2 AND owner_id=$3 AND deleted_at IS NULL
	`, parentID, folderID, user.ID)
	if err != nil {
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "an item with this name already exists in destination folder"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not move folder"})
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	h.insertAudit(r.Context(), &user.ID, "folder.moved", "folder", &folderID, clientIP(r), r.UserAgent(), map[string]any{"parentId": parentID})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h Handler) validateParentOwnership(r *http.Request, userID string, parentID *string) error {
	if parentID == nil {
		return nil
	}

	var exists bool
	if err := h.DB.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM folders WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL
		)
	`, *parentID, userID).Scan(&exists); err != nil {
		return fmt.Errorf("could not validate parent folder")
	}
	if !exists {
		return fmt.Errorf("parent folder not found")
	}
	return nil
}

func optionalUUIDFromQuery(r *http.Request, key string) (*string, error) {
	val := strings.TrimSpace(r.URL.Query().Get(key))
	if val == "" {
		return nil, nil
	}
	if _, err := uuid.Parse(val); err != nil {
		return nil, fmt.Errorf("invalid %s", key)
	}
	return &val, nil
}

func optionalUUIDPtr(input *string) (*string, error) {
	if input == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*input)
	if trimmed == "" {
		return nil, nil
	}
	if _, err := uuid.Parse(trimmed); err != nil {
		return nil, fmt.Errorf("invalid parentId")
	}
	return &trimmed, nil
}

func normalizeNodeName(name string) string {
	clean := strings.TrimSpace(name)
	clean = strings.ReplaceAll(clean, "\\", "")
	clean = strings.ReplaceAll(clean, "/", "")
	return clean
}

func matchesSearch(name, search string) bool {
	if search == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(search))
}

func sortBrowserItems(items []browserItem, sortBy string, order string) {
	desc := strings.EqualFold(order, "desc")

	compare := func(i, j int) bool {
		a := items[i]
		b := items[j]

		if a.Type != b.Type {
			return a.Type == itemTypeFolder
		}

		switch sortBy {
		case "size":
			aSize := int64(0)
			if a.SizeBytes != nil {
				aSize = *a.SizeBytes
			}
			bSize := int64(0)
			if b.SizeBytes != nil {
				bSize = *b.SizeBytes
			}
			if aSize != bSize {
				if desc {
					return aSize > bSize
				}
				return aSize < bSize
			}
		case "type":
			if string(a.Type) != string(b.Type) {
				if desc {
					return string(a.Type) > string(b.Type)
				}
				return string(a.Type) < string(b.Type)
			}
		case "modified", "modifiedDate":
			if !a.ModifiedAt.Equal(b.ModifiedAt) {
				if desc {
					return a.ModifiedAt.After(b.ModifiedAt)
				}
				return a.ModifiedAt.Before(b.ModifiedAt)
			}
		}

		if desc {
			return strings.ToLower(a.Name) > strings.ToLower(b.Name)
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}

	sort.SliceStable(items, compare)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505"
}
