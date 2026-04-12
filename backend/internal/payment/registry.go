package payment

import (
	"sync"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Registry is a thread-safe registry mapping PaymentType to Provider.
type Registry struct {
	mu        sync.RWMutex
	providers map[PaymentType]Provider
}

// ErrProviderNotFound is returned when a requested payment provider is not registered.
var ErrProviderNotFound = infraerrors.NotFound("PROVIDER_NOT_FOUND", "payment provider not registered")

// NewRegistry creates a new empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[PaymentType]Provider),
	}
}

// Register adds a provider for each of its supported payment types.
// If a type was previously registered, it is overwritten.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range p.SupportedTypes() {
		r.providers[t] = p
	}
}

// GetProvider returns the provider registered for the given payment type.
func (r *Registry) GetProvider(t PaymentType) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[t]
	if !ok {
		return nil, ErrProviderNotFound
	}
	return p, nil
}

// GetProviderByKey returns the first provider whose ProviderKey matches the given key.
func (r *Registry) GetProviderByKey(key string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.providers {
		if p.ProviderKey() == key {
			return p, nil
		}
	}
	return nil, ErrProviderNotFound
}

// GetProviderKey returns the provider key for the given payment type, or empty string if not found.
func (r *Registry) GetProviderKey(t PaymentType) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[t]
	if !ok {
		return ""
	}
	return p.ProviderKey()
}

// SupportedTypes returns all currently registered payment types.
func (r *Registry) SupportedTypes() []PaymentType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]PaymentType, 0, len(r.providers))
	for t := range r.providers {
		types = append(types, t)
	}
	return types
}

// Clear removes all registered providers.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers = make(map[PaymentType]Provider)
}
