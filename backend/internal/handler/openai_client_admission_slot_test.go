//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

type clientAdmissionSequenceConcurrencyCache struct {
	fakeConcurrencyCache
	acquireCalls   atomic.Int64
	accountRelease atomic.Int64
}

func (c *clientAdmissionSequenceConcurrencyCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	// acquireResponsesAccountSlot 先走一次快速抢槽；返回 false 后，WaitPlan
	// helper 会再次尝试。第二次成功即可稳定覆盖真实等待分支而不引入 sleep。
	return c.acquireCalls.Add(1) >= 2, nil
}

func (c *clientAdmissionSequenceConcurrencyCache) ReleaseAccountSlot(context.Context, int64, string) error {
	c.accountRelease.Add(1)
	return nil
}

func clientAdmissionRejectedAccount(id int64) *service.Account {
	rate := 0.1
	return &service.Account{
		ID:             id,
		Platform:       service.PlatformOpenAI,
		Type:           service.AccountTypeOAuth,
		Status:         service.StatusActive,
		Schedulable:    true,
		Concurrency:    2,
		RateMultiplier: &rate,
		Extra: map[string]any{
			"codex_cli_only": true,
		},
	}
}

func clientAdmissionRejectedContext(t *testing.T, gw *service.OpenAIGatewayService, c *gin.Context) context.Context {
	t.Helper()
	if c.Request == nil {
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	}
	c.Request.Header.Set("User-Agent", "curl/8.0")
	ctx := gw.WithOpenAICodexClientAdmission(c.Request.Context(), c, []byte(`{"model":"gpt-5.1"}`))
	require.True(t, service.OpenAICodexClientAdmissionActive(ctx))
	c.Request = c.Request.WithContext(ctx)
	return ctx
}

func TestAcquireResponsesAccountSlotClientAdmissionRecheckAllSlotPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := &service.OpenAIGatewayService{}
	groupID := int64(50)

	t.Run("scheduler already acquired", func(t *testing.T) {
		cache := &profitCountingConcurrencyCache{}
		h := &OpenAIGatewayHandler{
			gatewayService:    gw,
			concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatClaude, 0),
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		clientAdmissionRejectedContext(t, gw, c)
		var directRelease atomic.Int64
		selection := &service.AccountSelectionResult{
			Account:  clientAdmissionRejectedAccount(101),
			Acquired: true,
			ReleaseFunc: func() {
				directRelease.Add(1)
			},
		}
		streamStarted := false

		release, result := h.acquireResponsesAccountSlot(c, &groupID, "", selection, false, &streamStarted, zap.NewNop())
		require.Equal(t, openAISlotAcquireClientVetoed, result)
		require.Nil(t, release)
		require.Equal(t, int64(1), directRelease.Load(), "已抢槽路径否决后必须释放且只释放一次")
		require.Zero(t, cache.accountReleases.Load(), "不得额外释放未由 helper 获取的槽位")
		require.Zero(t, w.Body.Len(), "终检否决由调用方重选，不得提前写响应")
	})

	t.Run("fast acquire", func(t *testing.T) {
		cache := &profitCountingConcurrencyCache{}
		h := &OpenAIGatewayHandler{
			gatewayService:    gw,
			concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatClaude, 0),
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		clientAdmissionRejectedContext(t, gw, c)
		account := clientAdmissionRejectedAccount(102)
		selection := &service.AccountSelectionResult{
			Account:  account,
			Acquired: false,
			WaitPlan: &service.AccountWaitPlan{AccountID: account.ID, MaxConcurrency: 2, Timeout: time.Second, MaxWaiting: 2},
		}
		streamStarted := false

		release, result := h.acquireResponsesAccountSlot(c, &groupID, "", selection, false, &streamStarted, zap.NewNop())
		require.Equal(t, openAISlotAcquireClientVetoed, result)
		require.Nil(t, release)
		require.Equal(t, int64(1), cache.accountReleases.Load(), "快速抢槽否决后必须释放且只释放一次")
		require.Zero(t, w.Body.Len())
	})

	t.Run("wait plan", func(t *testing.T) {
		cache := &clientAdmissionSequenceConcurrencyCache{}
		h := &OpenAIGatewayHandler{
			gatewayService:    gw,
			concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatClaude, 0),
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		clientAdmissionRejectedContext(t, gw, c)
		account := clientAdmissionRejectedAccount(103)
		selection := &service.AccountSelectionResult{
			Account:  account,
			Acquired: false,
			WaitPlan: &service.AccountWaitPlan{AccountID: account.ID, MaxConcurrency: 2, Timeout: time.Second, MaxWaiting: 2},
		}
		streamStarted := false

		release, result := h.acquireResponsesAccountSlot(c, &groupID, "", selection, false, &streamStarted, zap.NewNop())
		require.Equal(t, openAISlotAcquireClientVetoed, result)
		require.Nil(t, release)
		require.Equal(t, int64(2), cache.acquireCalls.Load(), "必须先快速抢槽失败，再进入 WaitPlan 获取")
		require.Equal(t, int64(1), cache.accountRelease.Load(), "WaitPlan 终检否决后必须释放且只释放一次")
		require.Zero(t, w.Body.Len())
	})
}

func TestOpenAIClientAdmissionAllPoolVetoReturnsExactForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := &service.OpenAIGatewayService{}
	h := &OpenAIGatewayHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctx := clientAdmissionRejectedContext(t, gw, c)

	admission := gw.OpenAITerminalAdmissionLatest(ctx, clientAdmissionRejectedAccount(201))
	require.True(t, admission.ClientVetoed, "必须先实际否决账号，不能仅凭预计算结果推断 403")

	failed := make(map[int64]struct{})
	vetoCount := 0
	for id := int64(1); id <= int64(maxClientAdmissionVetoAttempts); id++ {
		if recordOpenAIClientAdmissionVeto(failed, id, &vetoCount) {
			continue
		}
		h.handleOpenAIClientAdmissionExhausted(c, ctx, false, false)
		break
	}

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Equal(t, "forbidden_error", gjson.Get(w.Body.String(), "error.type").String())
	require.Equal(t, service.CodexOfficialClientsOnlyMessage, gjson.Get(w.Body.String(), "error.message").String())
	require.Len(t, failed, maxClientAdmissionVetoAttempts)
	require.Equal(t, maxClientAdmissionVetoAttempts, vetoCount)
}
