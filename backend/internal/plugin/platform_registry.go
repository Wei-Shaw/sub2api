package plugin

import (
	"sort"
	"sync"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	"google.golang.org/grpc"
)

// RegisteredPlatform pairs a platform declaration with the owning plugin's
// gRPC connection so the account handler can forward RPCs.
type RegisteredPlatform struct {
	Decl       pluginsdk.PlatformDecl
	PluginName string
	Conn       *grpc.ClientConn
}

// PlatformRegistry collects platform declarations from all loaded gateway
// plugins and provides lookup APIs for the account handler and frontend.
//
// Thread-safe: reads use RLock, writes use Lock. The PluginManager calls
// Register/Unregister during plugin lifecycle events.
type PlatformRegistry struct {
	mu        sync.RWMutex
	platforms map[string]*RegisteredPlatform
}

func NewPlatformRegistry() *PlatformRegistry {
	return &PlatformRegistry{platforms: make(map[string]*RegisteredPlatform)}
}

// Register adds all platforms declared by a plugin. Duplicate platform IDs
// are rejected (first-win): the second plugin's declaration is silently
// dropped and logged by the caller.
func (r *PlatformRegistry) Register(pluginName string, decl pluginsdk.PlatformDecl, conn *grpc.ClientConn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.platforms[decl.Platform]; exists {
		return false
	}
	r.platforms[decl.Platform] = &RegisteredPlatform{
		Decl:       decl,
		PluginName: pluginName,
		Conn:       conn,
	}
	return true
}

// Unregister removes all platforms owned by the named plugin.
func (r *PlatformRegistry) Unregister(pluginName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range r.platforms {
		if v.PluginName == pluginName {
			delete(r.platforms, k)
		}
	}
}

// Get returns the registered platform for the given ID.
func (r *PlatformRegistry) Get(platform string) (*RegisteredPlatform, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.platforms[platform]
	return p, ok
}

// All returns all registered platforms sorted by SortOrder then Platform.
func (r *PlatformRegistry) All() []RegisteredPlatform {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RegisteredPlatform, 0, len(r.platforms))
	for _, p := range r.platforms {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Decl.SortOrder != out[j].Decl.SortOrder {
			return out[i].Decl.SortOrder < out[j].Decl.SortOrder
		}
		return out[i].Decl.Platform < out[j].Decl.Platform
	})
	return out
}

// AllPlatformIDs returns sorted platform identifiers.
func (r *PlatformRegistry) AllPlatformIDs() []string {
	all := r.All()
	ids := make([]string, len(all))
	for i := range all {
		ids[i] = all[i].Decl.Platform
	}
	return ids
}

// HasPlatform returns true if the given platform is registered.
func (r *PlatformRegistry) HasPlatform(platform string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.platforms[platform]
	return ok
}

// AccountTypesFor returns the declared account types for a platform.
func (r *PlatformRegistry) AccountTypesFor(platform string) []pluginsdk.AccountTypeDecl {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.platforms[platform]
	if !ok {
		return nil
	}
	return p.Decl.AccountTypes
}
