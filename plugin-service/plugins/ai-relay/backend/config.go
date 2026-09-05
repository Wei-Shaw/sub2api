package backend

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var routeSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

var (
	ErrInvalidRouteConfig = errors.New("invalid relay route configuration")
	ErrRouteNotFound      = errors.New("relay route not found")
)

type RouteConfig struct {
	Platform     string            `json:"platform"`
	Slug         string            `json:"slug"`
	Name         string            `json:"name"`
	BaseURL      string            `json:"base_url"`
	PathMappings map[string]string `json:"path_mappings"`
}

type RouteQuery struct {
	Platform string
	Search   string
}

type RouteReference struct {
	Platform string `json:"platform"`
	Slug     string `json:"slug"`
}

type RouteRepository interface {
	Get(ctx context.Context, platform, slug string) (RouteConfig, bool, error)
	List(ctx context.Context, query RouteQuery) ([]RouteConfig, error)
	Upsert(ctx context.Context, config RouteConfig) (RouteConfig, error)
	Delete(ctx context.Context, platform, slug string) error
	DeleteMany(ctx context.Context, routes []RouteReference) error
}

type MemoryRouteRepository struct {
	mu     sync.RWMutex
	routes map[string]RouteConfig
}

func NewMemoryRouteRepository() *MemoryRouteRepository {
	return &MemoryRouteRepository{routes: make(map[string]RouteConfig)}
}

func NormalizeRouteConfig(config RouteConfig) (RouteConfig, error) {
	config.Platform = strings.ToLower(strings.TrimSpace(config.Platform))
	config.Slug = strings.ToLower(strings.TrimSpace(config.Slug))
	config.Name = strings.TrimSpace(config.Name)
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.Platform == "" || !routeSlugPattern.MatchString(config.Platform) || !routeSlugPattern.MatchString(config.Slug) {
		return RouteConfig{}, ErrInvalidRouteConfig
	}
	parsedURL, err := url.Parse(config.BaseURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return RouteConfig{}, ErrInvalidRouteConfig
	}
	if config.Name == "" {
		config.Name = config.Slug
	}
	config.PathMappings, err = normalizePathMappings(config.PathMappings)
	if err != nil {
		return RouteConfig{}, ErrInvalidRouteConfig
	}
	return config, nil
}

func (r *MemoryRouteRepository) Get(_ context.Context, platform, slug string) (RouteConfig, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	config, ok := r.routes[routeKey(platform, slug)]
	return copyRouteConfig(config), ok, nil
}

func (r *MemoryRouteRepository) List(_ context.Context, query RouteQuery) ([]RouteConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	platform := strings.ToLower(strings.TrimSpace(query.Platform))
	search := strings.ToLower(strings.TrimSpace(query.Search))
	configs := make([]RouteConfig, 0, len(r.routes))
	for _, config := range r.routes {
		if platform != "" && config.Platform != platform {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(config.Name), search) && !strings.Contains(config.Slug, search) {
			continue
		}
		configs = append(configs, copyRouteConfig(config))
	}
	sort.Slice(configs, func(i, j int) bool {
		if configs[i].Platform == configs[j].Platform {
			return configs[i].Slug < configs[j].Slug
		}
		return configs[i].Platform < configs[j].Platform
	})
	return configs, nil
}

func (r *MemoryRouteRepository) Upsert(_ context.Context, config RouteConfig) (RouteConfig, error) {
	normalized, err := NormalizeRouteConfig(config)
	if err != nil {
		return RouteConfig{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[routeKey(normalized.Platform, normalized.Slug)] = copyRouteConfig(normalized)
	return copyRouteConfig(normalized), nil
}

func (r *MemoryRouteRepository) Delete(_ context.Context, platform, slug string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := routeKey(platform, slug)
	if _, ok := r.routes[key]; !ok {
		return ErrRouteNotFound
	}
	delete(r.routes, key)
	return nil
}

func (r *MemoryRouteRepository) DeleteMany(_ context.Context, routes []RouteReference) error {
	if len(routes) == 0 {
		return ErrInvalidRouteConfig
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(routes))
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		key := routeKey(route.Platform, route.Slug)
		if _, ok := seen[key]; ok || !routeSlugPattern.MatchString(strings.ToLower(strings.TrimSpace(route.Platform))) || !routeSlugPattern.MatchString(strings.ToLower(strings.TrimSpace(route.Slug))) {
			return ErrInvalidRouteConfig
		}
		if _, ok := r.routes[key]; !ok {
			return ErrRouteNotFound
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for _, key := range keys {
		delete(r.routes, key)
	}
	return nil
}

func routeKey(platform, slug string) string {
	return strings.ToLower(strings.TrimSpace(platform)) + ":" + strings.ToLower(strings.TrimSpace(slug))
}

func copyRouteConfig(config RouteConfig) RouteConfig {
	config.PathMappings = copyPathMappings(config.PathMappings)
	return config
}
