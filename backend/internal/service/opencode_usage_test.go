package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const openCodeTestWorkspaceID = "wrk_01KKKYPCYDAY6DQDY1VK1AJ2QT"

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

func TestNormalizeOpenCodeWorkspaceID(t *testing.T) {
	workspaceID, err := normalizeOpenCodeWorkspaceID("  " + openCodeTestWorkspaceID + "  ")
	require.NoError(t, err)
	require.Equal(t, openCodeTestWorkspaceID, workspaceID)

	for _, invalid := range []string{"", "workspace-123", "wrk_valid/../admin", "wrk_bad value"} {
		t.Run(invalid, func(t *testing.T) {
			_, err := normalizeOpenCodeWorkspaceID(invalid)
			require.Error(t, err)
			require.Equal(t, "credentials_invalid", classifyOpenCodeUsageError(err))
		})
	}
}

func TestFetchOpenCodeUsageScrapesWorkspaceDashboard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/workspace/"+openCodeTestWorkspaceID+"/go", r.URL.Path)
		require.Equal(t, "auth=session-token", r.Header.Get("Cookie"))
		require.Contains(t, r.Header.Get("Accept"), "text/html")
		require.Equal(t, serverURL(r)+"/workspace/"+openCodeTestWorkspaceID, r.Header.Get("Referer"))
		require.NotEmpty(t, r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`
			<html><body><script>
			$R[24]($R[18],$R[25]={mine:!0,useBalance:!1,
			rollingUsage:$R[30]={status:"ok",resetInSec:3471,usagePercent:29},
			weeklyUsage:$R[31]={status:"ok",resetInSec:464797,usagePercent:11},
			monthlyUsage:$R[32]={status:"ok",resetInSec:1811918,usagePercent:37}
			});
			</script></body></html>`))
	}))
	defer server.Close()

	account := &Account{Credentials: map[string]any{
		"auth_cookie":  "session-token",
		"workspace_id": openCodeTestWorkspaceID,
	}}
	snapshot, err := fetchOpenCodeUsage(context.Background(), account, server.URL)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Equal(t, float64(29), snapshot.Rolling.UsagePercent)
	require.Equal(t, int64(3471), snapshot.Rolling.resetSeconds())
	require.Equal(t, int64(464797), snapshot.Weekly.resetSeconds())
	require.Equal(t, float64(37), snapshot.Monthly.UsagePercent)
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func TestFetchOpenCodeUsageRequiresWorkspaceID(t *testing.T) {
	account := &Account{Credentials: map[string]any{"auth_cookie": "session-token"}}
	_, err := fetchOpenCodeUsage(context.Background(), account, "https://opencode.ai")
	require.Error(t, err)
	require.Equal(t, "OpenCode Go quota workspace ID is not configured", err.Error())
	require.Equal(t, "credentials_invalid", classifyOpenCodeUsageError(err))
}

func TestGetOpenCodeUsageRequiresCredentialPair(t *testing.T) {
	usage, err := (&AccountUsageService{}).getOpenCodeUsage(context.Background(), &Account{
		Credentials: map[string]any{"auth_cookie": "session-token"},
	}, false)
	require.NoError(t, err)
	require.Equal(t, "credentials_missing", usage.ErrorCode)
	require.Contains(t, usage.Error, "both workspace ID and auth cookie")
}

func TestFetchOpenCodeUsageDoesNotExposeErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("rejected auth=session-token workspace=" + openCodeTestWorkspaceID))
	}))
	defer server.Close()

	account := &Account{Credentials: map[string]any{
		"auth_cookie":  "session-token",
		"workspace_id": openCodeTestWorkspaceID,
	}}
	_, err := fetchOpenCodeUsage(context.Background(), account, server.URL)
	require.Error(t, err)
	require.Equal(t, "OpenCode workspace dashboard returned HTTP 401", err.Error())
	require.NotContains(t, err.Error(), "session-token")
	require.NotContains(t, err.Error(), openCodeTestWorkspaceID)
}

func TestFetchOpenCodeUsageTreatsRedirectedLoginPageAsExpiredSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/authorize" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html><head><title>OpenAuth</title></head><body>Authenticate</body></html>"))
			return
		}
		http.Redirect(w, r, "/authorize", http.StatusFound)
	}))
	defer server.Close()

	account := &Account{Credentials: map[string]any{
		"auth_cookie":  "expired-session",
		"workspace_id": openCodeTestWorkspaceID,
	}}
	_, err := fetchOpenCodeUsage(context.Background(), account, server.URL)
	require.Error(t, err)
	require.Equal(t, "OpenCode quota auth cookie is invalid or expired", err.Error())
	require.Equal(t, "unauthenticated", classifyOpenCodeUsageError(err))
}

func TestParseOpenCodeDashboardUsageRejectsMissingQuotaWithoutLeakingHTML(t *testing.T) {
	body := "<html><body>upstream changed secret=do-not-log</body></html>"
	_, err := parseOpenCodeDashboardUsage(body)
	require.Error(t, err)
	require.Equal(t, "upstream_unavailable", classifyOpenCodeUsageError(err))
	require.NotContains(t, err.Error(), "do-not-log")
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

func TestBuildOpenCodeUsageInfoPreservesRateLimitedWindow(t *testing.T) {
	now := time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)
	usage := buildOpenCodeUsageInfo(&openCodeUsageResponse{
		Weekly: &openCodeUsageWindow{
			Status:          "rate-limited",
			UsagePercent:    100,
			ResetsInSeconds: 54743,
		},
	}, now)

	require.NotNil(t, usage.OpenCodeWeekly)
	require.Equal(t, float64(100), usage.OpenCodeWeekly.Utilization)
	require.Equal(t, 54743, usage.OpenCodeWeekly.RemainingSeconds)
	require.Equal(t, now.Add(54743*time.Second), *usage.OpenCodeWeekly.ResetsAt)
}

func TestClassifyOpenCodeUsageError(t *testing.T) {
	require.Equal(t, "credentials_invalid", classifyOpenCodeUsageError(&openCodeUsageCredentialsError{}))
	require.Equal(t, "unauthenticated", classifyOpenCodeUsageError(&openCodeUsageHTTPError{StatusCode: http.StatusUnauthorized}))
	require.Equal(t, "forbidden", classifyOpenCodeUsageError(&openCodeUsageHTTPError{StatusCode: http.StatusForbidden}))
	require.Equal(t, "rate_limited", classifyOpenCodeUsageError(&openCodeUsageHTTPError{StatusCode: http.StatusTooManyRequests}))
	require.Equal(t, "upstream_unavailable", classifyOpenCodeUsageError(&openCodeUsageParseError{}))
	require.Equal(t, "network_error", classifyOpenCodeUsageError(context.DeadlineExceeded))
}
