package service

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

func newTestGinContextWithScheduler(scheduler GroupSchedulerConfig) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(requestSchedulerConfigKey, scheduler)
	return c
}

func TestShouldRetryUpstreamErrorForRequestFastFails524InHighAvailability(t *testing.T) {
	newService := func(strategy string) *GatewayService {
		return &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
			OpenAIScheduler: config.GatewayOpenAISchedulerConfig{Strategy: strategy},
		}}}
	}

	nonPool := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": false}}
	pool := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}

	if newService("high_availability").shouldRetryUpstreamErrorForRequest(nil, nonPool, 524) {
		t.Fatal("high-availability non-pool 524 must skip same-account retry")
	}
	poolBaseline := newService("legacy").shouldRetryUpstreamError(pool, 524)
	if got := newService("high_availability").shouldRetryUpstreamErrorForRequest(nil, pool, 524); got != poolBaseline {
		t.Fatalf("pool-mode 524 must preserve same-account retry policy: got %v, want %v", got, poolBaseline)
	}
	legacyBaseline := newService("legacy").shouldRetryUpstreamError(nonPool, 524)
	if got := newService("legacy").shouldRetryUpstreamErrorForRequest(nil, nonPool, 524); got != legacyBaseline {
		t.Fatalf("legacy 524 behavior must remain unchanged: got %v, want %v", got, legacyBaseline)
	}
}

func TestShouldRetryUpstreamErrorForRequestRespectsFirstByteFailoverSwitch(t *testing.T) {
	ginCtx := newTestGinContextWithScheduler(GroupSchedulerConfig{
		Strategy:          "high_availability",
		FirstByteFailover: false,
	})
	account := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": false}}
	svc := &GatewayService{}
	legacy := svc.shouldRetryUpstreamError(account, 524)
	if got := svc.shouldRetryUpstreamErrorForRequest(ginCtx, account, 524); got != legacy {
		t.Fatalf("disabling first_byte_failover must preserve same-account retry: got %v, want %v", got, legacy)
	}

	ginCtx = newTestGinContextWithScheduler(GroupSchedulerConfig{
		Strategy:          "high_availability",
		FirstByteFailover: true,
	})
	if svc.shouldRetryUpstreamErrorForRequest(ginCtx, account, 524) {
		t.Fatal("enabling first_byte_failover must skip same-account retry")
	}
}

func TestShouldFailoverTransportErrorForRequestOnlyHANonPool(t *testing.T) {
	newService := func(strategy string) *GatewayService {
		return &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
			OpenAIScheduler: config.GatewayOpenAISchedulerConfig{Strategy: strategy},
		}}}
	}
	nonPool := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": false}}
	pool := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}
	if !newService("high_availability").shouldFailoverTransportErrorForRequest(nil, nonPool) {
		t.Fatal("HA non-pool transport errors must be eligible for failover")
	}
	if newService("high_availability").shouldFailoverTransportErrorForRequest(nil, pool) {
		t.Fatal("pool-mode transport errors must preserve legacy handling")
	}
	if newService("legacy").shouldFailoverTransportErrorForRequest(nil, nonPool) {
		t.Fatal("legacy transport errors must preserve terminal handling")
	}
}
