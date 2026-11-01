package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const outcrawlProviderName = "outcrawl"

// outcrawlSearchEndpoint is a variable so tests can point it at a local server.
var outcrawlSearchEndpoint = "https://api.outcrawl.ai/v1/search"

// OutcrawlProvider implements web search via the Outcrawl Search API
// (https://outcrawl.ai/docs/search/overview).
type OutcrawlProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewOutcrawlProvider creates an Outcrawl Search provider.
// The caller is responsible for configuring the http.Client with proxy/timeouts.
func NewOutcrawlProvider(apiKey string, httpClient *http.Client) *OutcrawlProvider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OutcrawlProvider{apiKey: apiKey, httpClient: httpClient}
}

func (o *OutcrawlProvider) Name() string { return outcrawlProviderName }

func (o *OutcrawlProvider) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	limit := req.MaxResults
	if limit <= 0 {
		limit = defaultMaxResults
	}

	bodyBytes, err := json.Marshal(outcrawlRequest{Query: req.Query, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("outcrawl: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, outcrawlSearchEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("outcrawl: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("outcrawl: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("outcrawl: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("outcrawl: status %d: %s", resp.StatusCode, truncateBody(body))
	}

	var raw outcrawlResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("outcrawl: decode response: %w", err)
	}

	results := make([]SearchResult, 0, len(raw.Hits))
	for _, h := range raw.Hits {
		results = append(results, SearchResult{
			URL:     h.URL,
			Title:   h.Title,
			Snippet: h.Snippet,
		})
	}

	return &SearchResponse{Results: results, Query: req.Query}, nil
}

type outcrawlRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type outcrawlResponse struct {
	Hits []outcrawlHit `json:"hits"`
}

type outcrawlHit struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Rank    int    `json:"rank"`
}
