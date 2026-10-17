//go:build unit

package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
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

type clientAdmissionAccountRepo struct {
	service.AccountRepository
	getErr error
}

func (r clientAdmissionAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return clientAdmissionRejectedAccount(id), nil
}

func newClientAdmissionGateway(repo service.AccountRepository) *service.OpenAIGatewayService {
	return service.NewOpenAIGatewayService(
		repo,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
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
	gw := newClientAdmissionGateway(clientAdmissionAccountRepo{})
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
	gw := newClientAdmissionGateway(clientAdmissionAccountRepo{})
	h := &OpenAIGatewayHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctx := clientAdmissionRejectedContext(t, gw, c)

	admission, admissionErr := gw.OpenAITerminalAdmissionLatest(ctx, clientAdmissionRejectedAccount(201))
	require.NoError(t, admissionErr)
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

func TestOpenAIClientAdmissionUnavailableReturns503AndReleasesSlot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := newClientAdmissionGateway(clientAdmissionAccountRepo{getErr: errors.New("database unavailable")})
	h := &OpenAIGatewayHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctx := clientAdmissionRejectedContext(t, gw, c)
	var directRelease atomic.Int64
	selection := &service.AccountSelectionResult{
		Account:  clientAdmissionRejectedAccount(301),
		Acquired: true,
		ReleaseFunc: func() {
			directRelease.Add(1)
		},
	}
	h.gatewayService = gw
	streamStarted := false

	release, result := h.acquireResponsesAccountSlot(c, nil, "", selection, false, &streamStarted, zap.NewNop())
	require.Equal(t, openAISlotAcquireClientAdmissionUnavailable, result)
	require.Nil(t, release)
	require.Equal(t, int64(1), directRelease.Load())
	require.Zero(t, w.Body.Len())

	h.handleOpenAIClientAdmissionExhausted(c, ctx, false, false)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Equal(t, "api_error", gjson.Get(w.Body.String(), "error.type").String())
	require.NotContains(t, w.Body.String(), service.CodexOfficialClientsOnlyMessage)
	require.False(t, service.HasOpsClientBusinessLimited(c), "数据库故障不得被标记为客户端业务限流")
}

func TestOpenAIClientAdmissionUnavailableReleasesFastAndWaitSlots(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := newClientAdmissionGateway(clientAdmissionAccountRepo{getErr: errors.New("database unavailable")})

	t.Run("fast acquire", func(t *testing.T) {
		cache := &profitCountingConcurrencyCache{}
		h := &OpenAIGatewayHandler{
			gatewayService:    gw,
			concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatClaude, 0),
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		clientAdmissionRejectedContext(t, gw, c)
		account := clientAdmissionRejectedAccount(302)
		selection := &service.AccountSelectionResult{
			Account:  account,
			Acquired: false,
			WaitPlan: &service.AccountWaitPlan{AccountID: account.ID, MaxConcurrency: 2, Timeout: time.Second, MaxWaiting: 2},
		}
		streamStarted := false

		release, result := h.acquireResponsesAccountSlot(c, nil, "", selection, false, &streamStarted, zap.NewNop())
		require.Equal(t, openAISlotAcquireClientAdmissionUnavailable, result)
		require.Nil(t, release)
		require.Equal(t, int64(1), cache.accountReleases.Load())
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
		account := clientAdmissionRejectedAccount(303)
		selection := &service.AccountSelectionResult{
			Account:  account,
			Acquired: false,
			WaitPlan: &service.AccountWaitPlan{AccountID: account.ID, MaxConcurrency: 2, Timeout: time.Second, MaxWaiting: 2},
		}
		streamStarted := false

		release, result := h.acquireResponsesAccountSlot(c, nil, "", selection, false, &streamStarted, zap.NewNop())
		require.Equal(t, openAISlotAcquireClientAdmissionUnavailable, result)
		require.Nil(t, release)
		require.Equal(t, int64(2), cache.acquireCalls.Load())
		require.Equal(t, int64(1), cache.accountRelease.Load())
		require.Zero(t, w.Body.Len())
	})
}

func TestOpenAIClientAdmissionWSCloseClassification(t *testing.T) {
	status, message := openAIClientAdmissionWSClose(service.ErrCodexClientAdmissionUnavailable, service.CodexClientRestrictionDetectionResult{})
	require.Equal(t, coderws.StatusTryAgainLater, status)
	require.Contains(t, message, "temporarily unavailable")

	restricted := &service.CodexClientAdmissionError{Result: service.CodexClientRestrictionDetectionResult{
		Enabled: true,
		Matched: false,
		Reason:  service.CodexClientRestrictionReasonNotMatchedUA,
	}}
	status, message = openAIClientAdmissionWSClose(restricted, restricted.Result)
	require.Equal(t, coderws.StatusPolicyViolation, status)
	require.Equal(t, service.CodexOfficialClientsOnlyMessage, message)
}

func TestOpenAIClientAdmissionUnavailablePreservesStreamingResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	h := &OpenAIGatewayHandler{}

	require.True(t, h.handleOpenAICodexAdmissionError(c, service.ErrCodexClientAdmissionUnavailable, true, false))
	require.Contains(t, w.Body.String(), "event: error")
	require.Contains(t, w.Body.String(), "temporarily unavailable")
	require.False(t, strings.HasPrefix(strings.TrimSpace(w.Body.String()), "{"), "已开始的 Images SSE 不得尾随普通 JSON")
}

type pinnedClientAdmissionRepo struct {
	codexModelsFailoverAccountRepo
	getErr error
}

func (r pinnedClientAdmissionRepo) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.codexModelsFailoverAccountRepo.GetByID(ctx, id)
}

func pinnedClientAdmissionOAuthAccount(id int64, restricted bool) service.Account {
	account := newPinnedCodexAccount(id, service.StatusActive, true, false)
	account.Type = service.AccountTypeOAuth
	account.Credentials = map[string]any{"access_token": "pinned-oauth-token"}
	account.Extra = map[string]any{}
	if restricted {
		account.Extra["codex_cli_only"] = true
	}
	return account
}

// 兼容 OAuth 固定账号的放行路径会请求 ChatGPT 地址，由 service 包的
// TestFetchPinnedCodexModelsManifestAppliesClientAdmission 用本地服务覆盖；
// 这里只验证 handler 对准入错误的映射，不发出任何上游请求。
func TestCodexModelsPinnedAccountsMapClientAdmissionErrors(t *testing.T) {
	for _, tc := range []struct {
		name        string
		accounts    []service.Account
		pinnedIDs   []int64
		fallback    bool
		getErr      error
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "all pinned accounts restricted returns typed forbidden",
			accounts:    []service.Account{pinnedClientAdmissionOAuthAccount(2, true)},
			pinnedIDs:   []int64{2},
			wantStatus:  http.StatusForbidden,
			wantMessage: service.CodexOfficialClientsOnlyMessage,
		},
		{
			name:        "repository failure fails closed without scheduler fallback",
			accounts:    []service.Account{pinnedClientAdmissionOAuthAccount(2, false)},
			pinnedIDs:   []int64{2},
			fallback:    true,
			getErr:      errors.New("database unavailable"),
			wantStatus:  http.StatusServiceUnavailable,
			wantMessage: "temporarily unavailable",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &codexModelsPinnedHTTPUpstream{}
			gin.SetMode(gin.TestMode)
			gatewayService := service.NewOpenAIGatewayService(
				pinnedClientAdmissionRepo{codexModelsFailoverAccountRepo: codexModelsFailoverAccountRepo{accounts: tc.accounts}, getErr: tc.getErr},
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				upstream,
				nil, nil, nil, nil, nil, nil, nil, nil,
			)
			handler := &OpenAIGatewayHandler{gatewayService: gatewayService, maxAccountSwitches: 3}
			group := &service.Group{
				ID:       7801,
				Platform: service.PlatformOpenAI,
				CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{
					Enabled:             true,
					AccountIDs:          tc.pinnedIDs,
					FallbackToScheduler: tc.fallback,
				},
			}

			recorder := performPinnedCodexModelsRequest(t, handler, group, "")
			require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			if tc.wantMessage != "" {
				require.Contains(t, gjson.Get(recorder.Body.String(), "error.message").String(), tc.wantMessage)
			}
			require.Empty(t, upstream.accountIDs(), "未通过客户端准入的固定账号不得请求上游")
		})
	}
}
