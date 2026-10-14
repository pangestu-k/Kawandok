package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound           = errors.New("storage: object not found")
	ErrPresignUnsupported = errors.New("storage: presigned URLs not supported by this backend")
)

type Storage interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}
