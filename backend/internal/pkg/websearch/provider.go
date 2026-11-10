package websearch

import "context"

// Provider is the interface every search backend must implement.
type Provider interface {
	// Name returns the provider identifier ("brave", "tavily" or "deepseek").
	Name() string
	// Search executes a web search and returns results.
	Search(ctx context.Context, req SearchRequest) (*SearchResponse, error)
}
