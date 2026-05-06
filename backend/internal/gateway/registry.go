package gateway

import "sync"

// ProviderRegistry maps account platforms to GatewayProvider implementations.
// Phase 1: populated at startup with three host-internal adapters.
// Phase 2+: plugin manager dynamically registers/removes providers.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]GatewayProvider
}

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{providers: make(map[string]GatewayProvider)}
}

func (r *ProviderRegistry) Register(p GatewayProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Platform()] = p
}

func (r *ProviderRegistry) Unregister(platform string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, platform)
}

func (r *ProviderRegistry) Get(platform string) (GatewayProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[platform]
	return p, ok
}

// ForProtocol returns all registered providers that support the given
// input protocol. Used by the scheduler to filter the account pool.
func (r *ProviderRegistry) ForProtocol(protocol string) []GatewayProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []GatewayProvider
	for _, p := range r.providers {
		for _, proto := range p.Protocols() {
			if proto == protocol {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// HasProvider returns true if a provider is registered for the given
// (platform, protocol) pair. The scheduler uses this to exclude accounts
// whose platform has no active provider from the candidate pool.
func (r *ProviderRegistry) HasProvider(platform, protocol string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[platform]
	if !ok {
		return false
	}
	for _, proto := range p.Protocols() {
		if proto == protocol {
			return true
		}
	}
	return false
}
