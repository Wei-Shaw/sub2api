package repository

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestClaudeUsageFetcherReadsUsageWithoutAModelRequest(t *testing.T) {
	type observedRequest struct {
		method, path string
		bodyBytes    int
	}
	seen := make(chan observedRequest, 1)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls.Add(1)
		seen <- observedRequest{r.Method, r.URL.Path, len(body)}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":17,"resets_at":"2026-11-08T00:00:00Z"},"seven_day":{"utilization":53,"resets_at":"2026-11-12T00:00:00Z"}}`))
	}))
	defer upstream.Close()
	fetcher := &claudeUsageService{usageURL: upstream.URL + "/api/oauth/usage", allowPrivateHosts: true}
	usage, err := fetcher.FetchUsageWithOptions(t.Context(), &service.ClaudeUsageFetchOptions{AccessToken: "test-token", AccountID: 2})
	require.NoError(t, err)
	request := <-seen
	require.Equal(t, http.MethodGet, request.method)
	require.Equal(t, "/api/oauth/usage", request.path)
	require.Zero(t, request.bodyBytes)
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, 17.0, usage.FiveHour.Utilization)
	require.Equal(t, 53.0, usage.SevenDay.Utilization)
}
