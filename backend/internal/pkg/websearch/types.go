package websearch

// SearchResult represents a single web search result.
type SearchResult struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	PageAge string `json:"page_age,omitempty"`
}

// SearchRequest describes a web search to perform.
type SearchRequest struct {
	Query      string
	MaxResults int    // defaults to defaultMaxResults if <= 0
	ProxyURL   string // optional HTTP proxy URL
}

// SearchResponse holds the results of a web search.
type SearchResponse struct {
	Results []SearchResult
	Query   string // the query that was actually executed
}

const defaultMaxResults = 5

// Provider type identifiers.
const (
	ProviderTypeBrave  = "brave"
	ProviderTypeTavily = "tavily"
)
