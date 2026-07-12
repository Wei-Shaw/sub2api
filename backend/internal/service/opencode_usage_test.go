package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenCodeAuthCookie(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "raw token", in: " session-token ", want: "auth=session-token"},
		{name: "already named", in: "auth=session-token", want: "auth=session-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeOpenCodeAuthCookie(tt.in))
		})
	}
}

func TestFetchOpenCodeUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "auth=session-token", r.Header.Get("Cookie"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		require.NotEmpty(t, r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"rolling": map[string]any{"status": "ok", "usagePercent": 29, "resetsInSeconds": 3471},
			"weekly":  map[string]any{"status": "ok", "usagePercent": 11, "resetInSec": 464797},
			"monthly": map[string]any{"status": "ok", "usagePercent": 37, "resetsInSeconds": 1811918},
		}))
	}))
	defer server.Close()

	account := &Account{Credentials: map[string]any{"auth_cookie": "session-token"}}
	snapshot, err := fetchOpenCodeUsage(context.Background(), account, server.URL)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Equal(t, float64(29), snapshot.Rolling.UsagePercent)
	require.Equal(t, int64(3471), snapshot.Rolling.resetSeconds())
	require.Equal(t, int64(464797), snapshot.Weekly.resetSeconds())
	require.Equal(t, float64(37), snapshot.Monthly.UsagePercent)
}

func TestFetchOpenCodeUsageDoesNotExposeErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("rejected auth=session-token"))
	}))
	defer server.Close()

	account := &Account{Credentials: map[string]any{"auth_cookie": "session-token"}}
	_, err := fetchOpenCodeUsage(context.Background(), account, server.URL)
	require.Error(t, err)
	require.Equal(t, "OpenCode usage endpoint returned HTTP 401", err.Error())
	require.NotContains(t, err.Error(), "session-token")
}

func TestFetchOpenCodeUsageTreatsHTMLLoginPageAsExpiredSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>login</html>"))
	}))
	defer server.Close()

	account := &Account{Credentials: map[string]any{"auth_cookie": "expired-session"}}
	_, err := fetchOpenCodeUsage(context.Background(), account, server.URL)
	require.Error(t, err)
	require.Equal(t, "OpenCode quota auth cookie is invalid or expired", err.Error())
	require.Equal(t, "unauthenticated", classifyOpenCodeUsageError(err))
}

func TestBuildOpenCodeUsageExtraRoundTrip(t *testing.T) {
	now := time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)
	snapshot := &openCodeUsageResponse{
		Rolling: &openCodeUsageWindow{Status: "ok", UsagePercent: 29, ResetsInSeconds: 3600},
		Weekly:  &openCodeUsageWindow{Status: "ok", UsagePercent: 11, ResetsInSeconds: 7 * 24 * 3600},
		Monthly: &openCodeUsageWindow{Status: "ok", UsagePercent: 137, ResetsInSeconds: 30 * 24 * 3600},
	}

	usage := buildOpenCodeUsageInfo(snapshot, now)
	require.Equal(t, float64(29), usage.OpenCodeRolling.Utilization)
	require.Equal(t, 3600, usage.OpenCodeRolling.RemainingSeconds)
	require.Equal(t, float64(100), usage.OpenCodeMonthly.Utilization, "upstream percentages are clamped")

	extra := buildOpenCodeUsageExtraUpdates(usage, now)
	restored := buildOpenCodeUsageFromExtra(&Account{Extra: extra}, now.Add(time.Minute))
	require.NotNil(t, restored)
	require.Equal(t, float64(29), restored.OpenCodeRolling.Utilization)
	require.Equal(t, 3540, restored.OpenCodeRolling.RemainingSeconds)
	require.Equal(t, float64(11), restored.OpenCodeWeekly.Utilization)
	require.Equal(t, float64(100), restored.OpenCodeMonthly.Utilization)
	require.True(t, isOpenCodeUsageSnapshotFresh(&Account{Extra: extra}, now.Add(time.Minute)))
	require.False(t, isOpenCodeUsageSnapshotFresh(&Account{Extra: extra}, now.Add(openCodeUsageCacheTTL+time.Second)))
}

func TestClassifyOpenCodeUsageError(t *testing.T) {
	require.Equal(t, "unauthenticated", classifyOpenCodeUsageError(&openCodeUsageHTTPError{StatusCode: http.StatusUnauthorized}))
	require.Equal(t, "forbidden", classifyOpenCodeUsageError(&openCodeUsageHTTPError{StatusCode: http.StatusForbidden}))
	require.Equal(t, "rate_limited", classifyOpenCodeUsageError(&openCodeUsageHTTPError{StatusCode: http.StatusTooManyRequests}))
	require.Equal(t, "network_error", classifyOpenCodeUsageError(context.DeadlineExceeded))
}
