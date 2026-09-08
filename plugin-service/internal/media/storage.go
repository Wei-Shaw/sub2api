package media

import (
	"context"
	"errors"
	"io"
	"net/url"
	"time"
)

var ErrNotFound = errors.New("media object not found")

type Object struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

type Storage interface {
	Put(ctx context.Context, key, contentType string, body io.Reader, size int64) error
	Get(ctx context.Context, key string) (*Object, error)
	Delete(ctx context.Context, key string) error
	PresignGet(ctx context.Context, key string, expiry time.Duration) (*url.URL, error)
}
