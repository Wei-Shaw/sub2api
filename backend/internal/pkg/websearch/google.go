package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const (
	googleProviderName = "google"
)

// GoogleProvider implements web search via Google Vertex / Antigravity Grounding.
type GoogleProvider struct {
	endpoint   string
	bearerAuth string
	httpClient *http.Client
}

// NewGoogleProvider creates a Google Search provider using the Antigravity endpoint.
func NewGoogleProvider(endpoint, bearerAuth string, httpClient *http.Client) *GoogleProvider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	endpoint = strings.TrimRight(endpoint, "/")
	return &GoogleProvider{
		endpoint:   endpoint,
		bearerAuth: bearerAuth,
		httpClient: httpClient,
	}
}

func (g *GoogleProvider) Name() string { return googleProviderName }

func (g *GoogleProvider) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("google search: empty query")
	}

	targetURL := g.endpoint
	if !strings.HasSuffix(targetURL, "/v1/responses") && !strings.HasSuffix(targetURL, "/responses") {
		targetURL += "/v1/responses"
	}

	payload := map[string]any{
		"model": "gemini-3.8-flash",
		"input": []map[string]any{
			{
				"role":    "user",
				"content": fmt.Sprintf("请针对以下检索主题搜索网络并提取核心事实摘要与相关来源链接：\n%s", query),
			},
		},
		"tools": []map[string]any{
			{"type": "web_search_20250305"},
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("google search: marshal payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("google search: new request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if g.bearerAuth != "" {
		httpReq.Header.Set("Authorization", "Bearer "+g.bearerAuth)
	}

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("google search: execute request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google search: upstream status %d: %s", resp.StatusCode, string(respBytes))
	}

	var rawResp struct {
		Output []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawResp); err != nil {
		return nil, fmt.Errorf("google search: decode response: %w", err)
	}

	return parseGoogleSearchOutput(rawResp.Output, query), nil
}

var linkPattern = regexp.MustCompile(`\[\d+\]\s*\[(.*?)\]\((.*?)\)`)

func parseGoogleSearchOutput(output []struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}, query string) *SearchResponse {
	var textBlocks []string
	var sources []SearchResult

	for _, item := range output {
		if item.Type != "message" {
			continue
		}
		for _, c := range item.Content {
			if c.Type == "output_text" && c.Text != "" {
				textBlocks = append(textBlocks, c.Text)
			}
		}
	}

	fullText := strings.Join(textBlocks, "\n\n")
	matches := linkPattern.FindAllStringSubmatch(fullText, -1)
	seenURLs := make(map[string]bool)

	for _, match := range matches {
		if len(match) >= 3 {
			title := strings.TrimSpace(match[1])
			rawURL := strings.TrimSpace(match[2])
			if rawURL != "" && !seenURLs[rawURL] {
				seenURLs[rawURL] = true
				sources = append(sources, SearchResult{
					Title:   title,
					URL:     rawURL,
					Snippet: title,
				})
			}
		}
	}

	summarySnippet := fullText
	if idx := strings.Index(summarySnippet, "\n\n---\nWeb search queries:"); idx != -1 {
		summarySnippet = strings.TrimSpace(summarySnippet[:idx])
	}

	if len(sources) == 0 {
		sources = append(sources, SearchResult{
			Title:   "Google Search Summary",
			URL:     "https://www.google.com/search?q=" + query,
			Snippet: summarySnippet,
		})
	} else {
		sources[0].Snippet = summarySnippet
	}

	return &SearchResponse{
		Query:   query,
		Results: sources,
	}
}
