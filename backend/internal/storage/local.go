package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	root string
}

func NewLocalStorage(root string) (*LocalStorage, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("storage root is required")
	}

	cleanRoot := filepath.Clean(root)
	paths := []string{
		filepath.Join(cleanRoot, "files"),
		filepath.Join(cleanRoot, "tmp", "uploads"),
		filepath.Join(cleanRoot, "previews"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return nil, fmt.Errorf("create storage path %s: %w", p, err)
		}
	}

	return &LocalStorage{root: cleanRoot}, nil
}

func (s *LocalStorage) PutStream(ctx context.Context, key string, reader io.Reader) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	tmp := path + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(file, &contextReader{ctx: ctx, reader: reader}); err != nil {
		file.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("copy stream: %w", err)
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("promote temp file: %w", err)
	}

	return nil
}

func (s *LocalStorage) GetStream(ctx context.Context, key string) (io.ReadCloser, error) {
	path, err := s.resolve(key)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return &contextReadCloser{ctx: ctx, rc: file}, nil
}

func (s *LocalStorage) GetRangeStream(ctx context.Context, key string, start int64, end int64) (io.ReadCloser, error) {
	path, err := s.resolve(key)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	size := stat.Size()
	if start < 0 || end < start || start >= size {
		file.Close()
		return nil, fmt.Errorf("invalid range")
	}
	if end >= size {
		end = size - 1
	}

	if _, err := file.Seek(start, io.SeekStart); err != nil {
		file.Close()
		return nil, err
	}

	length := end - start + 1
	return &limitedReadCloser{ctx: ctx, rc: file, reader: io.LimitReader(file, length)}, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	path, err := s.resolve(key)
	if err != nil {
		return false, err
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalStorage) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	path, err := s.resolve(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if ctx.Err() != nil {
		return ObjectInfo{}, ctx.Err()
	}
	st, err := os.Stat(path)
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{
		Key:   key,
		Size:  st.Size(),
		MTime: st.ModTime().Unix(),
		ETag:  fmt.Sprintf("\"%x-%x\"", st.Size(), st.ModTime().UnixNano()),
	}, nil
}

func (s *LocalStorage) Move(ctx context.Context, from string, to string) error {
	fromPath, err := s.resolve(from)
	if err != nil {
		return err
	}
	toPath, err := s.resolve(to)
	if err != nil {
		return err
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if err := os.MkdirAll(filepath.Dir(toPath), 0o755); err != nil {
		return err
	}

	return os.Rename(fromPath, toPath)
}

func (s *LocalStorage) resolve(key string) (string, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return "", fmt.Errorf("storage key is required")
	}
	trimmed = strings.TrimPrefix(trimmed, "/")
	clean := filepath.Clean(trimmed)
	if clean == "." || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid storage key")
	}

	resolved := filepath.Join(s.root, clean)
	rootWithSep := s.root + string(os.PathSeparator)
	if resolved != s.root && !strings.HasPrefix(resolved, rootWithSep) {
		return "", fmt.Errorf("invalid storage key path")
	}
	return resolved, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type contextReadCloser struct {
	ctx context.Context
	rc  io.ReadCloser
}

func (c *contextReadCloser) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.rc.Read(p)
}

func (c *contextReadCloser) Close() error {
	return c.rc.Close()
}

type limitedReadCloser struct {
	ctx    context.Context
	rc     io.ReadCloser
	reader io.Reader
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	if err := l.ctx.Err(); err != nil {
		return 0, err
	}
	return l.reader.Read(p)
}

func (l *limitedReadCloser) Close() error {
	return l.rc.Close()
}
