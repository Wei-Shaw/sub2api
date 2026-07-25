// Package setting contains the port interfaces (repository abstractions)
// for the setting bounded context. DTO/value types live in internal/domain;
// this package only owns the persistence port contracts.
package setting

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Repository persists system settings as key/value pairs.
type Repository interface {
	Get(ctx context.Context, key string) (*domain.Setting, error)
	GetValue(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetMultiple(ctx context.Context, keys []string) (map[string]string, error)
	SetMultiple(ctx context.Context, settings map[string]string) error
	GetAll(ctx context.Context) (map[string]string, error)
	Delete(ctx context.Context, key string) error
}
