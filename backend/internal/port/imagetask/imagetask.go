// Package imagetask contains the port interface for the image-task bounded
// context: the Redis-backed key/value store for asynchronous image-request
// records. The contract references only domain types so the repository layer
// can implement it without importing internal/service. The service package
// keeps a type alias to the interface so existing call sites and test stubs
// continue to satisfy the contract.
package imagetask

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ImageTaskStore persists and retrieves async image-task records.
type ImageTaskStore interface {
	Save(ctx context.Context, task *domain.ImageTaskRecord, ttl time.Duration) error
	Get(ctx context.Context, id string) (*domain.ImageTaskRecord, error)
}
