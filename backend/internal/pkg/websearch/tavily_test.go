package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTavilyProvider_Name(t *testing.T) {
	p := NewTavilyProvider("key", nil)
	require.Equal(t, "tavily", p.Name())
}

func TestTavilyProvider_Search_SendsBearerAuth(t *testing.T) {
	var seen http.Request
	var seenBody []byte
	resp := `{"results":[{"url":"https://go.dev","title":"Go","content":"Go programming language","score":0.95}]}`
	p := NewTavilyProvider("tvly-test-key", exaTestClient(http.StatusOK, resp, &seen, &seenBody))

	out, err := p.Search(context.Background(), SearchRequest{Query: "golang", MaxResults: 3})

	require.NoError(t, err)
	require.Equal(t, http.MethodPost, seen.Method)
	require.Equal(t, tavilySearchEndpoint, seen.URL.String())
	require.Equal(t, "Bearer tvly-test-key", seen.Header.Get("Authorization"))
	require.Equal(t, "application/json", seen.Header.Get("Content-Type"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(seenBody, &body))
	require.Equal(t, map[string]any{
		"query":        "golang",
		"max_results":  float64(3),
		"search_depth": "basic",
	}, body, "the API key must travel in the Authorization header, not the request body")

	require.Equal(t, []SearchResult{
		{URL: "https://go.dev", Title: "Go", Snippet: "Go programming language"},
	}, out.Results)
}

func TestTavilyProvider_Search_ReportsUpstreamStatus(t *testing.T) {
	var seen http.Request
	var seenBody []byte
	p := NewTavilyProvider("bad-key", exaTestClient(http.StatusUnauthorized, `{"detail":{"error":"Unauthorized"}}`, &seen, &seenBody))

	_, err := p.Search(context.Background(), SearchRequest{Query: "golang"})

	require.ErrorContains(t, err, "tavily: status 401")
}

func TestTavilyProvider_Search_ResponseParsing(t *testing.T) {
	rawResp := `{"results":[{"url":"https://go.dev","title":"Go","content":"Go programming language","score":0.95}]}`
	var resp tavilyResponse
	require.NoError(t, json.Unmarshal([]byte(rawResp), &resp))
	require.Len(t, resp.Results, 1)
	require.Equal(t, "https://go.dev", resp.Results[0].URL)
	require.Equal(t, "Go programming language", resp.Results[0].Content)
	require.InDelta(t, 0.95, resp.Results[0].Score, 0.001)

	// Verify mapping to SearchResult
	results := make([]SearchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		results = append(results, SearchResult{
			URL: r.URL, Title: r.Title, Snippet: r.Content,
		})
	}
	require.Equal(t, "Go programming language", results[0].Snippet)
	require.Equal(t, "", results[0].PageAge)
}

func TestTavilyProvider_Search_EmptyResults(t *testing.T) {
	var resp tavilyResponse
	require.NoError(t, json.Unmarshal([]byte(`{"results":[]}`), &resp))
	require.Empty(t, resp.Results)
}

func TestTavilyProvider_Search_InvalidJSON(t *testing.T) {
	var resp tavilyResponse
	require.Error(t, json.Unmarshal([]byte("not json"), &resp))
}
