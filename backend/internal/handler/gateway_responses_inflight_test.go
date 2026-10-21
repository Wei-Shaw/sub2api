//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGatewayResponsesInflightSurvivesStickyFailover(t *testing.T) {
	for _, failover := range []bool{false, true} {
		name := "selection"
		if failover {
			name = "failover"
		}
		t.Run(name, func(t *testing.T) {
			cache := newHandlerInflightCache(100)
			cfg := &config.Config{}
			cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: 60}
			billing := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billing.Stop)
			pool := newUsageRecordTestPool(t)
			unblock := make(chan struct{})
			defer close(unblock)
			released := make(chan struct{})
			pool.Submit(func(context.Context) {
				select {
				case <-unblock:
				case <-released:
				}
			})

			ok := stickyScript{statusCode: http.StatusOK, streamBody: stickyAnthropicStream}
			upstream := &stickyUpstream{scripts: map[int64][]stickyScript{1: {ok}, 2: {ok}}}
			if failover {
				upstream.scripts[1] = []stickyScript{stickyFailScript()}
			}
			var seen []*service.InflightReservation
			upstream.observe = func(ctx context.Context) { seen = append(seen, service.InflightReservationFromContext(ctx)) }
			fixture := newStickyFixture(t, []*service.Account{stickyTestAccount(1, 0, .8), stickyTestAccount(2, 10, 1)}, upstream, newStickyGatewayTestCache(), stickyFixtureOptions{
				profitControl: true,
				configure:     func(h *GatewayHandler) { h.billingCacheService = billing; h.usageRecordWorkerPool = pool },
			})
			t.Cleanup(fixture.cleanup)
			heldDuringBilling := make(chan int, 1)
			fixture.usage.onCreate = func() { heldDuringBilling <- cache.count() }
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"claude-sonnet-4-5","input":"hello","max_output_tokens":64,"stream":true}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("session_id", "inflight-sticky-session")
			rec := httptest.NewRecorder()
			fixture.router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			expected := []int64{1}
			if failover {
				expected = []int64{1, 2}
			}
			require.Equal(t, expected, upstream.hitOrder())
			require.Equal(t, 1, cache.count(), "handler 返回后，排队中的计费任务必须继续持有预留")
			require.Len(t, seen, len(expected))
			require.NotNil(t, seen[0], "选号后上游请求应携带预留")
			for _, reservation := range seen {
				require.Same(t, seen[0], reservation)
			}
			close(released)
			require.Eventually(t, func() bool { return cache.count() == 0 }, 3*time.Second, 10*time.Millisecond)
			fixture.usage.mu.Lock()
			defer fixture.usage.mu.Unlock()
			require.Len(t, fixture.usage.logs, 1)
			require.Equal(t, 1, <-heldDuringBilling, "计费任务执行期间必须继续持有该预留引用")
		})
	}
}
