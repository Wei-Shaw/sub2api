package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	deepseekProviderName = "deepseek"
	deepseekMaxCount     = 50
	// DeepSeek rejects queries longer than 128 Unicode characters, while
	// model-generated web_search queries often exceed that.
	deepseekMaxQueryRunes = 128
	// summary is documented as 600-800 characters but often runs to several
	// thousand; cap it so one search does not flood the model's context.
	deepseekMaxSnippetRunes = 800
)

// deepseekSearchEndpoint is a var so tests can point it at an httptest server.
var deepseekSearchEndpoint = "https://api.deepseek.com/v1/tools/search"

// DeepSeekProvider implements web search via the DeepSeek Search API.
type DeepSeekProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewDeepSeekProvider creates a DeepSeek Search provider.
// The caller is responsible for configuring the http.Client with proxy/timeouts.
func NewDeepSeekProvider(apiKey string, httpClient *http.Client) *DeepSeekProvider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &DeepSeekProvider{apiKey: apiKey, httpClient: httpClient}
}

func (d *DeepSeekProvider) Name() string { return deepseekProviderName }

func (d *DeepSeekProvider) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	count := req.MaxResults
	if count <= 0 {
		count = defaultMaxResults
	}
	if count > deepseekMaxCount {
		count = deepseekMaxCount
	}
	query := truncateRunes(req.Query, deepseekMaxQueryRunes)

	bodyBytes, err := json.Marshal(deepseekRequest{Query: query, Count: count})
	if err != nil {
		return nil, fmt.Errorf("deepseek: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, deepseekSearchEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("deepseek: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+d.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("deepseek: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("deepseek: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek: status %d: %s", resp.StatusCode, truncateBody(body))
	}

	var raw deepseekResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("deepseek: decode response: %w", err)
	}

	results := make([]SearchResult, 0, len(raw.Results))
	for _, r := range raw.Results {
		// summary is the longer model-oriented abstract; snippet is a short UI blurb.
		snippet := r.Summary
		if snippet == "" {
			snippet = r.Snippet
		}
		var pageAge string
		if r.Date != nil {
			pageAge = *r.Date
		}
		results = append(results, SearchResult{
			URL:     r.URL,
			Title:   r.Title,
			Snippet: truncateRunes(snippet, deepseekMaxSnippetRunes),
			PageAge: pageAge,
		})
	}

	return &SearchResponse{Results: results, Query: query}, nil
}

type deepseekRequest struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

// deepseekResponse is the minimal structure of the DeepSeek Search API response.
type deepseekResponse struct {
	Results []deepseekResult `json:"results"`
}

type deepseekResult struct {
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Summary string  `json:"summary"`
	Date    *string `json:"date"`
}
