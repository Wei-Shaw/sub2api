package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/stretchr/testify/require"
)

func TestHTTPUpstreamDrainContinuousPingsCannotExtendBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_ = http.NewResponseController(w).Flush()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				_, _ = io.WriteString(w, ": ping\n\n")
				_ = http.NewResponseController(w).Flush()
			}
		}
	}))
	defer srv.Close()
	client, cancelClient := context.WithCancel(context.Background())
	defer cancelClient()
	req, err := http.NewRequestWithContext(pkghttputil.DetachUpstreamContext(client), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := doUpstreamRequest(srv.Client(), req, 40*time.Millisecond)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	cancelClient()
	started := time.Now()
	body, err := io.ReadAll(resp.Body)
	require.Error(t, err)
	require.Contains(t, string(body), ": ping")
	require.Less(t, time.Since(started), time.Second)
}

func TestHTTPUpstreamDrainPreservesLateTerminalUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_ = http.NewResponseController(w).Flush()
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(w, "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()
	client, cancelClient := context.WithCancel(context.Background())
	defer cancelClient()
	req, err := http.NewRequestWithContext(pkghttputil.DetachUpstreamContext(client), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := doUpstreamRequest(srv.Client(), req, 200*time.Millisecond)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	cancelClient()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(body), "output_tokens\":7") && strings.Contains(string(body), "message_stop"))
}
