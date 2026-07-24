// Package tlsfingerprint contains the port interfaces (repository/cache
// abstractions) for the TLS fingerprint profile bounded context.
// DTO/value types live in internal/domain; this package only owns the
// persistence/cache port contracts.
package tlsfingerprint

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ProfileRepository persists TLS fingerprint profile templates.
type ProfileRepository interface {
	List(ctx context.Context) ([]*domain.TLSFingerprintProfile, error)
	GetByID(ctx context.Context, id int64) (*domain.TLSFingerprintProfile, error)
	Create(ctx context.Context, profile *domain.TLSFingerprintProfile) (*domain.TLSFingerprintProfile, error)
	Update(ctx context.Context, profile *domain.TLSFingerprintProfile) (*domain.TLSFingerprintProfile, error)
	Delete(ctx context.Context, id int64) error
}

// ProfileCache caches TLS fingerprint profile templates across instances.
type ProfileCache interface {
	Get(ctx context.Context) ([]*domain.TLSFingerprintProfile, bool)
	Set(ctx context.Context, profiles []*domain.TLSFingerprintProfile) error
	Invalidate(ctx context.Context) error
	NotifyUpdate(ctx context.Context) error
	SubscribeUpdates(ctx context.Context, handler func())
}
