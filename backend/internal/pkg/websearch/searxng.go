package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	searxngProviderName = "searxng"
	searxngMaxCount     = 20
)

// SearxngProvider implements web search against a self-hosted SearXNG instance.
//
// SearXNG is a metasearch engine: one query fans out to several upstream engines
// (Google, Bing, DuckDuckGo, …) and the results are merged and ranked locally.
// That is the reason to prefer it over a single-engine scraper — recall survives
// one upstream being rate-limited — and why it needs no API key.
//
// Requires `formats: [html, json]` in the instance's settings.yml; without the
// json format the /search endpoint answers with HTML and decoding fails.
type SearxngProvider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewSearxngProvider creates a SearXNG provider.
// baseURL is the instance root, e.g. "http://searxng:8080".
// apiKey is optional: only instances fronted by an authenticating proxy need it.
func NewSearxngProvider(baseURL, apiKey string, httpClient *http.Client) *SearxngProvider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &SearxngProvider{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: httpClient,
	}
}

func (s *SearxngProvider) Name() string { return searxngProviderName }

func (s *SearxngProvider) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	if s.baseURL == "" {
		return nil, fmt.Errorf("searxng: base_url is not configured")
	}

	count := req.MaxResults
	if count <= 0 {
		count = defaultMaxResults
	}
	if count > searxngMaxCount {
		count = searxngMaxCount
	}

	endpoint, err := url.Parse(s.baseURL + "/search")
	if err != nil {
		return nil, fmt.Errorf("searxng: invalid base_url %q: %w", s.baseURL, err)
	}
	q := endpoint.Query()
	q.Set("q", req.Query)
	q.Set("format", "json")
	endpoint.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("searxng: build request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	if s.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("searxng: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("searxng: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng: status %d: %s", resp.StatusCode, truncateBody(body))
	}

	var raw searxngResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		// A bare 200 carrying HTML almost always means the json format is not
		// enabled on the instance; say so instead of surfacing a decode error.
		if looksLikeHTML(body) {
			return nil, fmt.Errorf("searxng: got HTML instead of JSON (enable `formats: [html, json]` in settings.yml)")
		}
		return nil, fmt.Errorf("searxng: decode response: %w", err)
	}

	results := make([]SearchResult, 0, len(raw.Results))
	for _, r := range raw.Results {
		if r.URL == "" {
			continue
		}
		results = append(results, SearchResult{
			URL:     r.URL,
			Title:   r.Title,
			Snippet: r.Content,
			PageAge: r.PublishedDate,
		})
		if len(results) >= count {
			break
		}
	}

	return &SearchResponse{Results: results, Query: req.Query}, nil
}

// searxngResponse is the minimal structure of SearXNG's `?format=json` payload.
type searxngResponse struct {
	Query   string          `json:"query"`
	Results []searxngResult `json:"results"`
}

type searxngResult struct {
	URL           string `json:"url"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	PublishedDate string `json:"publishedDate"`
}

func looksLikeHTML(body []byte) bool {
	head := strings.TrimSpace(string(body))
	if len(head) > 256 {
		head = head[:256]
	}
	head = strings.ToLower(head)
	return strings.HasPrefix(head, "<!doctype") || strings.HasPrefix(head, "<html")
}
