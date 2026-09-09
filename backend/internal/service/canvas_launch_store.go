package service

import (
	"context"
	"errors"
	"time"
)

var ErrCanvasLaunchNotFound = errors.New("canvas launch code not found")

type CanvasLaunchStore interface {
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	GetDel(ctx context.Context, key string) (string, error)
}
