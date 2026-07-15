package repository

import (
	"context"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// PromptCompressionTelemetryRepository is the persistence boundary for RTK
// run metadata. Implementations must never persist request bodies or diff
// contents; the service intentionally treats persistence as best effort.
type PromptCompressionTelemetryRepository interface {
	AppendTelemetry(context.Context, service.PromptCompressionTelemetry) error
	ListTelemetry(context.Context, int) ([]service.PromptCompressionTelemetry, error)
}

// InMemoryPromptCompressionTelemetryRepository is a bounded fallback used by
// the control plane before the SQL migrations are installed. It is also handy
// for preview/tests. The gateway does not depend on this fallback being
// available, so a full buffer evicts the oldest record.
type InMemoryPromptCompressionTelemetryRepository struct {
	mu      sync.Mutex
	limit   int
	entries []service.PromptCompressionTelemetry
}

func NewInMemoryPromptCompressionTelemetryRepository(limit int) *InMemoryPromptCompressionTelemetryRepository {
	if limit <= 0 {
		limit = 1024
	}
	return &InMemoryPromptCompressionTelemetryRepository{limit: limit}
}

func (r *InMemoryPromptCompressionTelemetryRepository) AppendTelemetry(_ context.Context, event service.PromptCompressionTelemetry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) >= r.limit {
		copy(r.entries, r.entries[1:])
		r.entries = r.entries[:r.limit-1]
	}
	r.entries = append(r.entries, event)
	return nil
}

func (r *InMemoryPromptCompressionTelemetryRepository) ListTelemetry(_ context.Context, limit int) ([]service.PromptCompressionTelemetry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 || limit > len(r.entries) {
		limit = len(r.entries)
	}
	start := len(r.entries) - limit
	out := make([]service.PromptCompressionTelemetry, limit)
	copy(out, r.entries[start:])
	return out, nil
}
