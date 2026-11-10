package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// newDeepSeekTestServer points the provider endpoint at handler for the test's lifetime.
func newDeepSeekTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	orig := deepseekSearchEndpoint
	deepseekSearchEndpoint = srv.URL
	t.Cleanup(func() {
		deepseekSearchEndpoint = orig
		srv.Close()
	})
	return srv
}

func TestDeepSeekProvider_Name(t *testing.T) {
	p := NewDeepSeekProvider("key", nil)
	require.Equal(t, "deepseek", p.Name())
}

func TestManager_BuildProvider_DeepSeek(t *testing.T) {
	// Unknown types silently fall back to brave, so check the factory routes this one.
	p := (&Manager{}).buildProvider(ProviderConfig{Type: ProviderTypeDeepSeek, APIKey: "k"}, nil)
	require.IsType(t, &DeepSeekProvider{}, p)
}

func TestDeepSeekProvider_Search_Success(t *testing.T) {
	srv := newDeepSeekTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req deepseekRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "golang", req.Query)
		require.Equal(t, 3, req.Count)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"rerank_score":2.4,"title":"Go","url":"https://go.dev","snippet":"short","summary":"long model summary","content":"","site_name":"Go","site_icon":null,"date":"2026-06-01T00:00:00+00:00","authority":3},
			{"rerank_score":1.1,"title":"Pkg","url":"https://pkg.go.dev","snippet":"only snippet","summary":"","content":"","site_name":null,"site_icon":null,"date":null,"authority":-1}
		]}`))
	})

	p := NewDeepSeekProvider("test-key", srv.Client())
	resp, err := p.Search(context.Background(), SearchRequest{Query: "golang", MaxResults: 3})
	require.NoError(t, err)
	require.Equal(t, "golang", resp.Query)
	require.Len(t, resp.Results, 2)

	require.Equal(t, "https://go.dev", resp.Results[0].URL)
	require.Equal(t, "Go", resp.Results[0].Title)
	require.Equal(t, "long model summary", resp.Results[0].Snippet)
	require.Equal(t, "2026-06-01T00:00:00+00:00", resp.Results[0].PageAge)

	require.Equal(t, "only snippet", resp.Results[1].Snippet, "falls back to snippet when summary is empty")
	require.Equal(t, "", resp.Results[1].PageAge)
}

func TestDeepSeekProvider_Search_TruncatesLongSummary(t *testing.T) {
	long := strings.Repeat("摘要", 2000)
	srv := newDeepSeekTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(deepseekResponse{Results: []deepseekResult{
			{URL: "https://a.example", Title: "A", Summary: long},
		}})
	})
	p := NewDeepSeekProvider("key", srv.Client())

	resp, err := p.Search(context.Background(), SearchRequest{Query: "q"})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	require.Equal(t, deepseekMaxSnippetRunes, utf8.RuneCountInString(resp.Results[0].Snippet))
	require.True(t, utf8.ValidString(resp.Results[0].Snippet))
}

func TestDeepSeekProvider_Search_ClampsCountAndQuery(t *testing.T) {
	var got deepseekRequest
	srv := newDeepSeekTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_, _ = w.Write([]byte(`{"results":[]}`))
	})
	p := NewDeepSeekProvider("key", srv.Client())

	// Multi-byte characters: the limit is 128 characters, not bytes.
	long := strings.Repeat("搜索", 100)
	resp, err := p.Search(context.Background(), SearchRequest{Query: long, MaxResults: 999})
	require.NoError(t, err)
	require.Equal(t, 50, got.Count)
	require.Equal(t, 128, utf8.RuneCountInString(got.Query))
	require.True(t, utf8.ValidString(got.Query))
	require.Equal(t, got.Query, resp.Query)
	require.Empty(t, resp.Results)

	_, err = p.Search(context.Background(), SearchRequest{Query: "q"})
	require.NoError(t, err)
	require.Equal(t, defaultMaxResults, got.Count)
	require.Equal(t, "q", got.Query)
}

func TestDeepSeekProvider_Search_HTTPError(t *testing.T) {
	srv := newDeepSeekTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"message":"Insufficient Balance"}}`))
	})
	p := NewDeepSeekProvider("key", srv.Client())

	_, err := p.Search(context.Background(), SearchRequest{Query: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "deepseek: status 402")
	require.Contains(t, err.Error(), "Insufficient Balance")
}

func TestDeepSeekProvider_Search_InvalidJSON(t *testing.T) {
	srv := newDeepSeekTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})
	p := NewDeepSeekProvider("key", srv.Client())

	_, err := p.Search(context.Background(), SearchRequest{Query: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "deepseek: decode response")
}
