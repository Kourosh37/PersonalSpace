package storage

import (
	"context"
	"io"
)

type ObjectInfo struct {
	Key   string
	Size  int64
	ETag  string
	MTime int64
}

type Interface interface {
	PutStream(ctx context.Context, key string, reader io.Reader) error
	GetStream(ctx context.Context, key string) (io.ReadCloser, error)
	GetRangeStream(ctx context.Context, key string, start int64, end int64) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	Move(ctx context.Context, from string, to string) error
}