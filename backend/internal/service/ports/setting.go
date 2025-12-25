package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/model"
)

type SettingRepository interface {
	Get(ctx context.Context, key string) (*model.Setting, error)
	GetValue(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetMultiple(ctx context.Context, keys []string) (map[string]string, error)
	SetMultiple(ctx context.Context, settings map[string]string) error
	GetAll(ctx context.Context) (map[string]string, error)
	Delete(ctx context.Context, key string) error
}
