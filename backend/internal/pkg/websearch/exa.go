package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	exaSearchEndpoint = "https://api.exa.ai/search"
	exaProviderName   = "exa"
)

// ExaProvider implements web search via the Exa Search API.
type ExaProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewExaProvider creates an Exa Search provider.
// The caller is responsible for configuring the http.Client with proxy/timeouts.
func NewExaProvider(apiKey string, httpClient *http.Client) *ExaProvider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &ExaProvider{apiKey: apiKey, httpClient: httpClient}
}

func (e *ExaProvider) Name() string { return exaProviderName }

func (e *ExaProvider) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	numResults := req.MaxResults
	if numResults <= 0 {
		numResults = defaultMaxResults
	}

	// Search type and highlight length are left to Exa's defaults.
	payload := exaRequest{
		Query:      req.Query,
		NumResults: numResults,
		Contents:   exaContents{Highlights: true},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("exa: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, exaSearchEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("exa: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", e.apiKey)

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("exa: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("exa: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exa: status %d: %s", resp.StatusCode, truncateBody(body))
	}

	var raw exaResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("exa: decode response: %w", err)
	}

	results := make([]SearchResult, 0, len(raw.Results))
	for _, r := range raw.Results {
		results = append(results, SearchResult{
			URL:     r.URL,
			Title:   r.Title,
			Snippet: strings.Join(r.Highlights, "\n"),
			PageAge: r.PublishedDate,
		})
	}

	return &SearchResponse{Results: results, Query: req.Query}, nil
}

type exaRequest struct {
	Query      string      `json:"query"`
	NumResults int         `json:"numResults"`
	Contents   exaContents `json:"contents"`
}

type exaContents struct {
	Highlights bool `json:"highlights"`
}

type exaResponse struct {
	Results []exaResult `json:"results"`
}

type exaResult struct {
	URL           string   `json:"url"`
	Title         string   `json:"title"`
	PublishedDate string   `json:"publishedDate"`
	Highlights    []string `json:"highlights"`
}
