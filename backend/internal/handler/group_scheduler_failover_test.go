package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestEffectiveSameAccountRetryLimitForContextUsesGroupOverride(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("sub2api.scheduler_config", service.GroupSchedulerConfig{
		Strategy:                 "high_availability",
		SameAccountRetryAttempts: 1,
	})
	account := &service.Account{
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"pool_mode": true, "pool_mode_retry_count": 4},
	}
	if got := effectiveSameAccountRetryLimitForContext(c, &service.UpstreamFailoverError{}, account); got != 1 {
		t.Fatalf("group retry override = %d, want 1", got)
	}
}

func TestEffectiveSameAccountRetryLimitForContextPreservesLegacyAccountSetting(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	account := &service.Account{
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"pool_mode": true, "pool_mode_retry_count": 4},
	}
	if got := effectiveSameAccountRetryLimitForContext(c, &service.UpstreamFailoverError{}, account); got != 4 {
		t.Fatalf("legacy account retry limit = %d, want 4", got)
	}
}
