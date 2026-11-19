//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// loopbackUpstream sends upstream requests over a real HTTP client.
type loopbackUpstream struct{ client *http.Client }

func (u loopbackUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.client.Do(req)
}

func (u loopbackUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.client.Do(req)
}

const upstreamWaitPing = "event: ping\ndata: {\"type\":\"ping\"}\n\n"

// slowScriptedUpstream holds every response's headers back for delay and
// answers the n-th request with statuses[n]: a complete Messages stream for
// 200, an error body otherwise.
func slowScriptedUpstream(t *testing.T, delay time.Duration, statuses ...int) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		status := statuses[calls]
		calls++
		mu.Unlock()
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		if status != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"upstream overloaded"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(compatStreamUsagePartialStream + compatStreamUsageCompletion))
	}))
	t.Cleanup(srv.Close)
	return srv, func() int {
		mu.Lock()
		defer mu.Unlock()
		return calls
	}
}

// runMessagesWithSlowUpstream sends one streaming /v1/messages request through
// GatewayHandler.Messages to a group of two accounts that both relay to srv.
func runMessagesWithSlowUpstream(t *testing.T, srv *httptest.Server) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	groupID := int64(9500)
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 1}
	accounts := make([]*service.Account, 0, 2)
	for _, id := range []int64{9501, 9502} {
		accounts = append(accounts, &service.Account{
			ID: id, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1,
			Credentials:   map[string]any{"api_key": "test-key", "base_url": srv.URL},
			Extra:         map[string]any{"anthropic_passthrough": true},
			AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
		})
	}
	cfg := &config.Config{
		RunMode:  config.RunModeSimple,
		Gateway:  config.GatewayConfig{StreamKeepaliveInterval: 1},
		Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false, AllowInsecureHTTP: true}},
	}
	snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: accounts}, nil, nil, nil, nil)
	gw := service.NewGatewayService(
		nil, &fakeGroupRepo{group: group}, nil, nil, nil, nil, nil, &stickyWritebackCache{}, cfg,
		snapshot, nil, service.NewBillingService(cfg, nil), nil, nil, nil, loopbackUpstream{client: &http.Client{}},
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	h := &GatewayHandler{
		gatewayService: gw, billingCacheService: billingCache, cfg: cfg, maxAccountSwitches: 1,
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := `{"model":"claude-sonnet-4.5","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"slow upstream"}]}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	key := &service.APIKey{ID: 9503, UserID: 9504, GroupID: &groupID, Group: group, User: &service.User{ID: 9504, Balance: 100}}
	c.Set(string(middleware.ContextKeyAPIKey), key)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9504})

	h.Messages(c)
	return rec
}

// requireSSEOnly checks that every line of body belongs to an SSE event.
func requireSSEOnly(t *testing.T, body string) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if line == "" {
			continue
		}
		require.True(t, strings.HasPrefix(line, "event: ") || strings.HasPrefix(line, "data: "), "non-SSE line %q in %q", line, body)
	}
}

// A late 5xx after the pings fails over to the next account, and its stream
// follows the pings as if the failed attempt never happened.
func TestGatewayMessagesFailsOverAfterUpstreamWaitPings(t *testing.T) {
	srv, calls := slowScriptedUpstream(t, 1500*time.Millisecond, http.StatusBadGateway, http.StatusOK)

	rec := runMessagesWithSlowUpstream(t, srv)

	require.Equal(t, 2, calls(), "the 502 after the pings fails over to the second account")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.True(t, strings.HasPrefix(body, upstreamWaitPing+upstreamWaitPing), "pings from both attempts precede the stream, got %q", body)
	require.Equal(t, 1, strings.Count(body, "event: message_start"))
	require.Contains(t, body, "event: message_stop")
	require.NotContains(t, body, `"type":"error"`)
	requireSSEOnly(t, body)
}

// Once the pings have committed a 200 SSE response, running out of accounts is
// reported as an SSE error event, never as a JSON body.
func TestGatewayMessagesExhaustedFailoverAfterUpstreamWaitPingsIsSSEError(t *testing.T) {
	srv, calls := slowScriptedUpstream(t, 1500*time.Millisecond, http.StatusBadGateway, http.StatusBadGateway)

	rec := runMessagesWithSlowUpstream(t, srv)

	require.Equal(t, 2, calls())
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.True(t, strings.HasPrefix(body, upstreamWaitPing+upstreamWaitPing), "got %q", body)
	require.Contains(t, body, `data: {"type":"error"`)
	requireSSEOnly(t, body)
}
