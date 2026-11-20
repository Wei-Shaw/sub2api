package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutcrawlProvider_Name(t *testing.T) {
	p := NewOutcrawlProvider("key", nil)
	require.Equal(t, "outcrawl", p.Name())
}

func TestOutcrawlProvider_Search_RequestConstruction(t *testing.T) {
	data, err := json.Marshal(outcrawlRequest{Query: "golang", Limit: 3})
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	require.Equal(t, "golang", parsed["query"])
	require.Equal(t, float64(3), parsed["limit"])
	require.Len(t, parsed, 2)
}

func TestOutcrawlProvider_Search_ResponseParsing(t *testing.T) {
	rawResp := `{"hits":[{"url":"https://go.dev","title":"Go","snippet":"Go programming language","rank":1}],"usage":{"credits":"1"}}`
	var resp outcrawlResponse
	require.NoError(t, json.Unmarshal([]byte(rawResp), &resp))
	require.Len(t, resp.Hits, 1)
	require.Equal(t, "https://go.dev", resp.Hits[0].URL)
	require.Equal(t, "Go programming language", resp.Hits[0].Snippet)
	require.Equal(t, 1, resp.Hits[0].Rank)
}

func TestOutcrawlProvider_Search_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		var body outcrawlRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "golang", body.Query)
		require.Equal(t, 3, body.Limit)
		_, _ = w.Write([]byte(`{"hits":[{"url":"https://go.dev","title":"Go","snippet":"Go lang","rank":1}],"usage":{"credits":"1"}}`))
	}))
	defer srv.Close()

	origEndpoint := outcrawlSearchEndpoint
	outcrawlSearchEndpoint = srv.URL
	defer func() { outcrawlSearchEndpoint = origEndpoint }()

	p := NewOutcrawlProvider("test-key", srv.Client())
	resp, err := p.Search(context.Background(), SearchRequest{Query: "golang", MaxResults: 3})
	require.NoError(t, err)
	require.Equal(t, "golang", resp.Query)
	require.Len(t, resp.Results, 1)
	require.Equal(t, SearchResult{URL: "https://go.dev", Title: "Go", Snippet: "Go lang"}, resp.Results[0])
}

func TestOutcrawlProvider_Search_DefaultMaxResults(t *testing.T) {
	var receivedLimit int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body outcrawlRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedLimit = body.Limit
		_, _ = w.Write([]byte(`{"hits":[]}`))
	}))
	defer srv.Close()

	origEndpoint := outcrawlSearchEndpoint
	outcrawlSearchEndpoint = srv.URL
	defer func() { outcrawlSearchEndpoint = origEndpoint }()

	p := NewOutcrawlProvider("k", srv.Client())
	resp, err := p.Search(context.Background(), SearchRequest{Query: "test"})
	require.NoError(t, err)
	require.Empty(t, resp.Results)
	require.Equal(t, defaultMaxResults, receivedLimit)
}

func TestOutcrawlProvider_Search_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	origEndpoint := outcrawlSearchEndpoint
	outcrawlSearchEndpoint = srv.URL
	defer func() { outcrawlSearchEndpoint = origEndpoint }()

	p := NewOutcrawlProvider("bad", srv.Client())
	_, err := p.Search(context.Background(), SearchRequest{Query: "test"})
	require.ErrorContains(t, err, "outcrawl: status 401")
}

func TestOutcrawlProvider_Search_InvalidJSON(t *testing.T) {
	var resp outcrawlResponse
	require.Error(t, json.Unmarshal([]byte("not json"), &resp))
}
