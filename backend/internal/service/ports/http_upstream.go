package ports

import "net/http"

// HTTPUpstream interface for making HTTP requests to upstream APIs (Claude, OpenAI, etc.)
// This is a generic interface that can be used for any HTTP-based upstream service.
type HTTPUpstream interface {
	Do(req *http.Request, proxyURL string) (*http.Response, error)
}
