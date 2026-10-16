package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearxngProvider_Name(t *testing.T) {
	p := NewSearxngProvider("http://searxng:8080", "", nil)
	require.Equal(t, "searxng", p.Name())
}

func TestSearxngProvider_Search_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/search", r.URL.Path)
		require.Equal(t, "golang", r.URL.Query().Get("q"))
		require.Equal(t, "json", r.URL.Query().Get("format"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		// A self-hosted instance is normally unauthenticated; no key configured
		// means no Authorization header at all.
		require.Empty(t, r.Header.Get("Authorization"))

		_ = json.NewEncoder(w).Encode(searxngResponse{
			Query: "golang",
			Results: []searxngResult{
				{URL: "https://go.dev", Title: "Go", Content: "Go lang", PublishedDate: "2026-09-01"},
				{URL: "https://pkg.go.dev", Title: "Pkg", Content: "Packages"},
			},
		})
	}))
	defer srv.Close()

	p := NewSearxngProvider(srv.URL, "", srv.Client())
	resp, err := p.Search(context.Background(), SearchRequest{Query: "golang", MaxResults: 2})

	require.NoError(t, err)
	require.Equal(t, "golang", resp.Query)
	require.Len(t, resp.Results, 2)
	require.Equal(t, "https://go.dev", resp.Results[0].URL)
	require.Equal(t, "Go", resp.Results[0].Title)
	require.Equal(t, "Go lang", resp.Results[0].Snippet)
	require.Equal(t, "2026-09-01", resp.Results[0].PageAge)
}

func TestSearxngProvider_Search_SendsBearerWhenKeyConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(searxngResponse{})
	}))
	defer srv.Close()

	p := NewSearxngProvider(srv.URL, "test-key", srv.Client())
	_, err := p.Search(context.Background(), SearchRequest{Query: "test"})
	require.NoError(t, err)
}

func TestSearxngProvider_Search_TrimsTrailingSlashFromBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(searxngResponse{})
	}))
	defer srv.Close()

	p := NewSearxngProvider(srv.URL+"/", "", srv.Client())
	_, err := p.Search(context.Background(), SearchRequest{Query: "test"})

	require.NoError(t, err)
	require.Equal(t, "/search", gotPath)
}

func TestSearxngProvider_Search_SkipsResultsWithoutURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(searxngResponse{
			Results: []searxngResult{
				{URL: "", Title: "No target"},
				{URL: "https://go.dev", Title: "Go"},
			},
		})
	}))
	defer srv.Close()

	p := NewSearxngProvider(srv.URL, "", srv.Client())
	resp, err := p.Search(context.Background(), SearchRequest{Query: "test"})

	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	require.Equal(t, "https://go.dev", resp.Results[0].URL)
}

func TestSearxngProvider_Search_TruncatesToMaxResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		results := make([]searxngResult, 0, 10)
		for i := 0; i < 10; i++ {
			results = append(results, searxngResult{
				URL:   fmt.Sprintf("https://example.com/%d", i),
				Title: fmt.Sprintf("Result %d", i),
			})
		}
		_ = json.NewEncoder(w).Encode(searxngResponse{Results: results})
	}))
	defer srv.Close()

	p := NewSearxngProvider(srv.URL, "", srv.Client())
	resp, err := p.Search(context.Background(), SearchRequest{Query: "test", MaxResults: 3})

	require.NoError(t, err)
	require.Len(t, resp.Results, 3)
	require.Equal(t, "https://example.com/0", resp.Results[0].URL)
}

func TestSearxngProvider_Search_DefaultMaxResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		results := make([]searxngResult, 0, 10)
		for i := 0; i < 10; i++ {
			results = append(results, searxngResult{URL: fmt.Sprintf("https://example.com/%d", i)})
		}
		_ = json.NewEncoder(w).Encode(searxngResponse{Results: results})
	}))
	defer srv.Close()

	p := NewSearxngProvider(srv.URL, "", srv.Client())
	resp, err := p.Search(context.Background(), SearchRequest{Query: "test", MaxResults: 0})

	require.NoError(t, err)
	require.Len(t, resp.Results, defaultMaxResults)
}

func TestSearxngProvider_Search_CapsRequestedResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		results := make([]searxngResult, 0, searxngMaxCount+5)
		for i := 0; i < searxngMaxCount+5; i++ {
			results = append(results, searxngResult{URL: fmt.Sprintf("https://example.com/%d", i)})
		}
		_ = json.NewEncoder(w).Encode(searxngResponse{Results: results})
	}))
	defer srv.Close()

	p := NewSearxngProvider(srv.URL, "", srv.Client())
	resp, err := p.Search(context.Background(), SearchRequest{Query: "test", MaxResults: 100})

	require.NoError(t, err)
	require.Len(t, resp.Results, searxngMaxCount)
}

func TestSearxngProvider_Search_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	p := NewSearxngProvider(srv.URL, "", srv.Client())
	_, err := p.Search(context.Background(), SearchRequest{Query: "test"})

	require.ErrorContains(t, err, "searxng: status 429")
}

func TestSearxngProvider_Search_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := NewSearxngProvider(srv.URL, "", srv.Client())
	_, err := p.Search(context.Background(), SearchRequest{Query: "test"})

	require.ErrorContains(t, err, "searxng: decode response")
}

func TestSearxngProvider_Search_HTMLResponsePointsAtJSONFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<!DOCTYPE html>\n<html><body>results</body></html>"))
	}))
	defer srv.Close()

	p := NewSearxngProvider(srv.URL, "", srv.Client())
	_, err := p.Search(context.Background(), SearchRequest{Query: "test"})

	// The HTML-vs-JSON confusion is the single most common misconfiguration of a
	// fresh instance, so the error has to name the fix rather than report a
	// decode failure.
	require.ErrorContains(t, err, "enable `formats: [html, json]`")
}

func TestSearxngProvider_Search_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(searxngResponse{})
	}))
	defer srv.Close()

	p := NewSearxngProvider(srv.URL, "", srv.Client())
	resp, err := p.Search(context.Background(), SearchRequest{Query: "test"})

	require.NoError(t, err)
	require.Empty(t, resp.Results)
}

func TestSearxngProvider_Search_MissingBaseURL(t *testing.T) {
	p := NewSearxngProvider("   ", "", nil)
	_, err := p.Search(context.Background(), SearchRequest{Query: "test"})

	require.ErrorContains(t, err, "base_url is not configured")
}

func TestSearxngProvider_Search_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(searxngResponse{})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewSearxngProvider(srv.URL, "", srv.Client())
	_, err := p.Search(ctx, SearchRequest{Query: "test"})

	require.ErrorContains(t, err, "searxng: request failed")
}
