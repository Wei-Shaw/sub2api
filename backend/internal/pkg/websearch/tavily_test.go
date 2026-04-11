package websearch

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTavilyProvider_Name(t *testing.T) {
	p := NewTavilyProvider("key", nil)
	require.Equal(t, "tavily", p.Name())
REDACTED

func TestTavilyProvider_Search_RequestConstruction(t *testing.T) {
	// Verify tavilyRequest struct fields map correctly
	req := tavilyRequest{
		APIKey:      "test-key",
		Query:       "golang",
		MaxResults:  3,
		SearchDepth: tavilySearchDepthBasic,
REDACTED
	data, err := json.Marshal(req)
REDACTED

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	require.Equal(t, "test-key", parsed["api_key"])
	require.Equal(t, "golang", parsed["query"])
	require.Equal(t, float64(3), parsed["max_results"])
	require.Equal(t, "basic", parsed["search_depth"])
REDACTED

func TestTavilyProvider_Search_ResponseParsing(t *testing.T) {
	rawResp := `{"results":[{"url":"https://go.dev","title":"Go","content":"Go programming language","score":0.95REDACTED]REDACTED`
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
	REDACTED)
REDACTED
	require.Equal(t, "Go programming language", results[0].Snippet)
	require.Equal(t, "", results[0].PageAge)
REDACTED

func TestTavilyProvider_Search_EmptyResults(t *testing.T) {
	var resp tavilyResponse
	require.NoError(t, json.Unmarshal([]byte(`{"results":[]REDACTED`), &resp))
	require.Empty(t, resp.Results)
REDACTED

func TestTavilyProvider_Search_InvalidJSON(t *testing.T) {
	var resp tavilyResponse
	require.Error(t, json.Unmarshal([]byte("not json"), &resp))
REDACTED
