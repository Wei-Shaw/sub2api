package payment

import (
	"fmt"
	"sync"
)

// Registry is a thread-safe registry mapping PaymentType to Provider.
type Registry struct {
	mu        sync.RWMutex
	providers map[PaymentType]Provider
}

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
		return nil, fmt.Errorf("no payment provider registered for type: %s", t)
	}
	return p, nil
}

// GetProviderByKey returns the first provider whose ProviderKey matches the given key.
func (r *Registry) GetProviderByKey(key string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[string]bool)
	for _, p := range r.providers {
		k := p.ProviderKey()
		if k == key && !seen[k] {
			return p, nil
		}
		seen[k] = true
	}
	return nil, fmt.Errorf("no payment provider registered with key: %s", key)
}

// GetProviderKey returns the provider key for the given payment type,
// or empty string if not found.
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
