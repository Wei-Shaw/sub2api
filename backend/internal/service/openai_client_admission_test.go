package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type accountAwareCodexAdmissionDetector struct {
	calls atomic.Int64
}

func (d *accountAwareCodexAdmissionDetector) Detect(_ *gin.Context, account *Account, _ CodexRestrictionPolicy, _ []byte) CodexClientRestrictionDetectionResult {
	d.calls.Add(1)
	if account != nil && account.IsCodexCLIOnlyAppServerAllowed() {
		return CodexClientRestrictionDetectionResult{Enabled: true, Matched: true, Reason: CodexClientRestrictionReasonMatchedAppServerClient}
	}
	return CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: CodexClientRestrictionReasonNotMatchedUA}
}

func newCodexAdmissionContext(t *testing.T, svc *OpenAIGatewayService) context.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return svc.WithOpenAICodexClientAdmission(context.Background(), c, []byte(`{"model":"gpt-5.1-codex"}`))
}

func codexAdmissionAccount(id int64, restricted bool) Account {
	now := time.Now().UTC()
	extra := map[string]any{}
	if restricted {
		extra["codex_cli_only"] = true
	}
	return Account{
		ID:          id,
		Name:        "account",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		UpdatedAt:   now,
		Extra:       extra,
	}
}

type codexAdmissionAccountRepo struct {
	AccountRepository
	accounts   []Account
	byID       map[int64]*Account
	getErr     error
	getErrByID map[int64]error
	getCalls   atomic.Int64
}

func (r *codexAdmissionAccountRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

func (r *codexAdmissionAccountRepo) ListSchedulableUngroupedByPlatform(context.Context, string) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

func (r *codexAdmissionAccountRepo) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

func (r *codexAdmissionAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.getCalls.Add(1)
	if r.getErr != nil {
		return nil, r.getErr
	}
	if err := r.getErrByID[id]; err != nil {
		return nil, err
	}
	if r.byID == nil || r.byID[id] == nil {
		return nil, nil
	}
	copy := *r.byID[id]
	return &copy, nil
}

type alwaysMatchedCodexAdmissionDetector struct{}

func (alwaysMatchedCodexAdmissionDetector) Detect(_ *gin.Context, _ *Account, _ CodexRestrictionPolicy, _ []byte) CodexClientRestrictionDetectionResult {
	return CodexClientRestrictionDetectionResult{Enabled: true, Matched: true, Reason: CodexClientRestrictionReasonMatchedUA}
}

type mutableCodexAdmissionDetector struct {
	matched atomic.Bool
	calls   atomic.Int64
}

func (d *mutableCodexAdmissionDetector) Detect(_ *gin.Context, _ *Account, _ CodexRestrictionPolicy, _ []byte) CodexClientRestrictionDetectionResult {
	d.calls.Add(1)
	matched := d.matched.Load()
	reason := CodexClientRestrictionReasonNotMatchedUA
	if matched {
		reason = CodexClientRestrictionReasonMatchedUA
	}
	return CodexClientRestrictionDetectionResult{Enabled: true, Matched: matched, Reason: reason}
}

type codexAdmissionSnapshotCache struct {
	SchedulerCache
	account *Account
	err     error
	calls   atomic.Int64
}

type countingCodexStickyCache struct {
	stubGatewayCache
	getCalls atomic.Int64
	setCalls atomic.Int64
}

func (c *countingCodexStickyCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	c.getCalls.Add(1)
	return c.stubGatewayCache.GetSessionAccountID(ctx, groupID, sessionHash)
}

func (c *countingCodexStickyCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	c.setCalls.Add(1)
	return c.stubGatewayCache.SetSessionAccountID(ctx, groupID, sessionHash, accountID, ttl)
}

type recoveringCodexAdmissionRepo struct {
	AccountRepository
	parent      *Account
	parentCalls atomic.Int64
	totalCalls  atomic.Int64
}

func (r *recoveringCodexAdmissionRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	r.totalCalls.Add(1)
	if r.parentCalls.Add(1) == 1 {
		return nil, nil
	}
	if r.parent == nil {
		return nil, nil
	}
	copy := *r.parent
	return &copy, nil
}

func (c *codexAdmissionSnapshotCache) GetAccount(context.Context, int64) (*Account, error) {
	c.calls.Add(1)
	if c.err != nil {
		return nil, c.err
	}
	if c.account == nil {
		return nil, nil
	}
	copy := *c.account
	return &copy, nil
}

func TestCodexClientAdmissionFreezesRequestDetection(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	svc := &OpenAIGatewayService{codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)
	require.Equal(t, int64(2), detector.calls.Load(), "入口只预计算普通与 app-server 两种结果")

	standard := codexAdmissionAccount(1, true)
	vetoed, _ := svc.codexClientAdmissionVetoReason(ctx, &standard, nil)
	require.True(t, vetoed)

	appServer := codexAdmissionAccount(2, true)
	appServer.Extra["codex_cli_only_allow_app_server"] = true
	vetoed, _ = svc.codexClientAdmissionVetoReason(ctx, &appServer, nil)
	require.False(t, vetoed)
	require.Equal(t, int64(2), detector.calls.Load(), "候选与终检必须复用同一请求快照")
}

func TestCodexClientAdmissionSkipsNoOpGateForUniversallyMatchedClient(t *testing.T) {
	svc := &OpenAIGatewayService{codexDetector: alwaysMatchedCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)
	require.False(t, codexClientAdmissionActive(ctx))
	require.False(t, openAIStickyAdmissionDeferred(ctx), "不会被任何账号配置拒绝的客户端必须保留原 eager sticky 语义")
}

func TestCodexClientAdmissionUniversallyMatchedClientKeepsFrozenForwardDecision(t *testing.T) {
	detector := &mutableCodexAdmissionDetector{}
	detector.matched.Store(true)
	svc := &OpenAIGatewayService{codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)
	require.False(t, codexClientAdmissionActive(ctx))
	require.Equal(t, int64(2), detector.calls.Load())

	detector.matched.Store(false)
	restricted := codexAdmissionAccount(3, true)
	result := svc.detectCodexClientRestrictionForForward(ctx, nil, &restricted, nil)
	require.True(t, result.Matched, "最终保护必须复用入口冻结的策略与请求身份")
	require.Equal(t, int64(2), detector.calls.Load(), "Forward 不得在同一请求内重新执行检测器")
}

func TestCodexClientAdmissionLegacySelection(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	groupID := int64(77)

	t.Run("empty pool does not infer forbidden from precomputed denial", func(t *testing.T) {
		repo := &codexAdmissionAccountRepo{}
		svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: detector}
		ctx := newCodexAdmissionContext(t, svc)

		snapshot, ok := codexClientAdmissionSnapshotFromContext(ctx)
		require.True(t, ok)
		require.False(t, snapshot.hadDenied(), "预计算拒绝结果不等于实际发生过账号 veto")

		_, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "", nil)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		require.False(t, errors.Is(err, ErrCodexClientRestricted))
		require.False(t, snapshot.hadDenied())
	})

	t.Run("mixed pool skips restricted account", func(t *testing.T) {
		restricted := codexAdmissionAccount(10, true)
		restricted.Priority = 0
		regular := codexAdmissionAccount(11, false)
		regular.Priority = 1
		repo := &codexAdmissionAccountRepo{accounts: []Account{restricted, regular}}
		svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: detector}
		ctx := newCodexAdmissionContext(t, svc)

		selected, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "", nil)
		require.NoError(t, err)
		require.NotNil(t, selected)
		require.Equal(t, regular.ID, selected.ID)
	})

	t.Run("all otherwise eligible accounts return typed forbidden", func(t *testing.T) {
		restricted := codexAdmissionAccount(20, true)
		repo := &codexAdmissionAccountRepo{accounts: []Account{restricted}}
		svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: detector}
		ctx := newCodexAdmissionContext(t, svc)
		snapshot, ok := codexClientAdmissionSnapshotFromContext(ctx)
		require.True(t, ok)
		require.False(t, snapshot.hadDenied())

		_, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "", nil)
		require.ErrorIs(t, err, ErrCodexClientRestricted)
		require.True(t, snapshot.hadDenied(), "只有实际候选 veto 后才能升级为 typed 403")
		result, ok := CodexClientRestrictionResultFromError(err)
		require.True(t, ok)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("model incompatible restricted account does not cause false forbidden", func(t *testing.T) {
		restricted := codexAdmissionAccount(30, true)
		restricted.Credentials = map[string]any{
			"model_mapping": map[string]any{"gpt-other": "gpt-other"},
		}
		repo := &codexAdmissionAccountRepo{accounts: []Account{restricted}}
		svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: detector}
		ctx := newCodexAdmissionContext(t, svc)

		_, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "gpt-5.1-codex", nil)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		require.False(t, errors.Is(err, ErrCodexClientRestricted))
	})
}

func TestCodexClientAdmissionShadowInheritsParent(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	parent := codexAdmissionAccount(40, true)
	parentID := parent.ID
	shadow := codexAdmissionAccount(41, false)
	shadow.ParentAccountID = &parentID
	repo := &codexAdmissionAccountRepo{
		accounts: []Account{shadow},
		byID:     map[int64]*Account{parent.ID: &parent},
	}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)

	_, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "", "", nil)
	require.ErrorIs(t, err, ErrCodexClientRestricted)
}

func TestCodexClientAdmissionShadowParentResolutionFailsClosed(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	parentID := int64(42)
	shadow := codexAdmissionAccount(43, false)
	shadow.ParentAccountID = &parentID

	tests := []struct {
		name   string
		parent *Account
	}{
		{name: "missing parent"},
		{
			name: "nested shadow parent",
			parent: func() *Account {
				parent := codexAdmissionAccount(parentID, true)
				grandparentID := int64(41)
				parent.ParentAccountID = &grandparentID
				return &parent
			}(),
		},
		{
			name: "non oauth parent",
			parent: func() *Account {
				parent := codexAdmissionAccount(parentID, true)
				parent.Type = AccountTypeAPIKey
				return &parent
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &OpenAIGatewayService{codexDetector: detector}
			ctx := newCodexAdmissionContext(t, svc)
			vetoed, reason := svc.codexClientAdmissionVetoReason(ctx, &shadow, func(int64) *Account {
				return tc.parent
			})
			require.True(t, vetoed)
			require.Equal(t, codexClientAdmissionFilterReason, reason)
			err := codexClientAdmissionErrorFromContext(ctx)
			require.ErrorIs(t, err, ErrCodexClientRestricted)
			result, ok := CodexClientRestrictionResultFromError(err)
			require.True(t, ok)
			require.Equal(t, codexClientAdmissionShadowParentUnresolvedReason, result.Reason)
		})
	}
}

func TestCodexClientAdmissionShadowParentTransientRecoveryCannotBypass(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	parent := codexAdmissionAccount(44, true)
	parentID := parent.ID
	shadow := codexAdmissionAccount(45, false)
	shadow.ParentAccountID = &parentID
	repo := &recoveringCodexAdmissionRepo{parent: &parent}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)

	_, admissionErr := svc.OpenAITerminalAdmissionLatest(ctx, &shadow)
	require.ErrorIs(t, admissionErr, ErrCodexClientAdmissionUnavailable, "母账号首次读取失败时必须在终检 fail-closed")
	require.False(t, errors.Is(admissionErr, ErrCodexClientRestricted), "读取失败不能冒充真实客户端策略拒绝")

	resolved, err := resolveCredentialAccount(ctx, repo, &shadow)
	require.NoError(t, err)
	require.Equal(t, parent.ID, resolved.ID, "后续凭据解析恢复证明该窗口原本可把受限母账号发往上游")
	require.Equal(t, int64(2), repo.totalCalls.Load())
}

func TestCodexClientAdmissionForwardGuardFailsClosedForUnresolvedShadow(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	parentID := int64(46)
	shadow := codexAdmissionAccount(47, false)
	shadow.ParentAccountID = &parentID
	svc := &OpenAIGatewayService{
		accountRepo:   &codexAdmissionAccountRepo{},
		codexDetector: detector,
	}
	ctx := newCodexAdmissionContext(t, svc)

	result := svc.detectCodexClientRestrictionForForward(ctx, nil, &shadow, nil)
	require.True(t, result.Enabled)
	require.False(t, result.Matched)
	require.Equal(t, codexClientAdmissionShadowParentUnresolvedReason, result.Reason)
}

func TestLiveSidebandClientAdmissionRejectsRestrictedPinnedAccount(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	restricted := codexAdmissionAccount(48, true)
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{restricted.ID: &restricted}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)
	record := &LiveCallRecord{AccountID: restricted.ID}

	err := svc.ValidateLiveSidebandClientAdmission(ctx, record)
	require.ErrorIs(t, err, ErrCodexClientRestricted)
	result, ok := CodexClientRestrictionResultFromError(err)
	require.True(t, ok)
	require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
}

func TestLiveSidebandLateGuardRejectsBeforeCredentialOrDial(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	restricted := codexAdmissionAccount(49, true)
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{restricted.ID: &restricted}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)

	conn, err := svc.dialLiveSideband(ctx, &LiveCallRecord{AccountID: restricted.ID, CallID: "call_restricted"})
	require.Nil(t, conn)
	require.ErrorIs(t, err, ErrCodexClientRestricted, "晚期保护必须在读取 OAuth 凭据或拨号前拒绝")
}

func TestLiveSidebandAccountReadFailureIsAdmissionUnavailable(t *testing.T) {
	repo := &codexAdmissionAccountRepo{getErr: errors.New("database unavailable")}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)
	record := &LiveCallRecord{AccountID: 490, CallID: "call_unavailable"}

	t.Run("before upgrade", func(t *testing.T) {
		err := svc.ValidateLiveSidebandClientAdmission(ctx, record)
		require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
		require.False(t, errors.Is(err, ErrCodexClientRestricted))
	})

	t.Run("after upgrade", func(t *testing.T) {
		conn, err := svc.dialLiveSideband(ctx, record)
		require.Nil(t, conn)
		require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
		require.False(t, errors.Is(err, ErrCodexClientRestricted))
	})
}

func TestCodexClientAdmissionPreviousResponseBindingIsPreserved(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	groupID := int64(78)
	restricted := codexAdmissionAccount(45, true)
	restricted.GroupIDs = []int64{groupID}
	restricted.Extra["openai_oauth_responses_websockets_v2_enabled"] = true
	cache := &countingCodexStickyCache{}
	store := NewOpenAIWSStateStore(cache)
	repo := &codexAdmissionAccountRepo{
		accounts: []Account{restricted},
		byID:     map[int64]*Account{restricted.ID: &restricted},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		cfg:                newOpenAIWSV2TestConfig(),
		codexDetector:      detector,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
		openaiWSStateStore: store,
	}
	ctx := newCodexAdmissionContext(t, svc)
	require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_codex_restricted", restricted.ID, time.Hour))

	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx,
		&groupID,
		"resp_codex_restricted",
		"",
		"gpt-5.1",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityChatCompletions,
		false,
		false,
		true,
		PlatformOpenAI,
	)
	require.Nil(t, selection)
	require.ErrorIs(t, err, ErrCodexClientRestricted, "不可迁移的 previous_response_id 必须精确拒绝")
	boundID, getErr := store.GetResponseAccount(ctx, groupID, "resp_codex_restricted")
	require.NoError(t, getErr)
	require.Equal(t, restricted.ID, boundID, "客户端不兼容是请求级状态，不得删除 response binding")

	recovered := restricted
	recovered.Extra = map[string]any{"openai_oauth_responses_websockets_v2_enabled": true}
	repo.accounts = []Account{recovered}
	repo.byID[recovered.ID] = &recovered
	recoveredCtx := newCodexAdmissionContext(t, svc)
	selection, _, err = svc.SelectAccountWithSchedulerForCapability(
		recoveredCtx,
		&groupID,
		"resp_codex_restricted",
		"",
		"gpt-5.1",
		nil,
		OpenAIUpstreamTransportAny,
		OpenAIEndpointCapabilityChatCompletions,
		false,
		false,
		true,
		PlatformOpenAI,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, recovered.ID, selection.Account.ID, "限制解除后同一 binding 应自动恢复命中")
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestCodexClientAdmissionSessionStickySkipsWithoutOverwritingBinding(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	groupID := int64(79)
	restricted := codexAdmissionAccount(46, true)
	restricted.GroupIDs = []int64{groupID}
	restricted.Priority = 0
	regular := codexAdmissionAccount(47, false)
	regular.GroupIDs = []int64{groupID}
	regular.Priority = 1
	cache := &countingCodexStickyCache{}
	repo := &codexAdmissionAccountRepo{
		accounts: []Account{restricted, regular},
		byID: map[int64]*Account{
			restricted.ID: &restricted,
			regular.ID:    &regular,
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		cfg:                &config.Config{},
		codexDetector:      detector,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	ctx := newCodexAdmissionContext(t, svc)
	const sessionHash = "codex-restricted-sticky"
	require.NoError(t, svc.setStickySessionAccountID(ctx, &groupID, sessionHash, restricted.ID, time.Hour))

	selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, regular.ID, selection.Account.ID, "本请求应跳过不兼容的旧粘性账号")
	cache.getCalls.Store(0)
	cache.setCalls.Store(0)
	require.NoError(t, svc.BindStickySessionAfterProfitAdmission(ctx, &groupID, sessionHash, regular.ID))
	require.Equal(t, int64(1), cache.getCalls.Load(), "实际跳过旧 sticky 后才需要读取既有 binding")
	require.Zero(t, cache.setCalls.Load(), "不兼容 fallback 不得覆盖旧 binding")
	require.Equal(t, restricted.ID, cache.sessionBindings[svc.openAISessionCacheKey(sessionHash)], "fallback 账号不得覆盖既有不兼容 binding")
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	recovered := restricted
	recovered.Extra = map[string]any{
		"codex_cli_only":                  true,
		"codex_cli_only_allow_app_server": true,
	}
	repo.accounts = []Account{recovered, regular}
	repo.byID[recovered.ID] = &recovered
	selection, _, err = svc.SelectAccountWithScheduler(ctx, &groupID, "", sessionHash, "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, recovered.ID, selection.Account.ID, "账号恢复为当前客户端兼容后应重新命中旧 binding")
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestCodexClientAdmissionOrdinaryFailoverStillReplacesStickyBinding(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	groupID := int64(80)
	oldAccount := codexAdmissionAccount(480, false)
	newAccount := codexAdmissionAccount(481, false)
	cache := &countingCodexStickyCache{}
	svc := &OpenAIGatewayService{
		accountRepo:   &codexAdmissionAccountRepo{byID: map[int64]*Account{oldAccount.ID: &oldAccount, newAccount.ID: &newAccount}},
		cache:         cache,
		cfg:           &config.Config{},
		codexDetector: detector,
	}
	ctx := newCodexAdmissionContext(t, svc)
	const sessionHash = "ordinary-upstream-failover"
	require.NoError(t, svc.setStickySessionAccountID(ctx, &groupID, sessionHash, oldAccount.ID, time.Hour))
	cache.getCalls.Store(0)
	cache.setCalls.Store(0)

	require.NoError(t, svc.BindStickySessionAfterProfitAdmission(ctx, &groupID, sessionHash, newAccount.ID))
	require.Zero(t, cache.getCalls.Load(), "未发生客户端否决的普通流量不得增加 Redis GET")
	require.Equal(t, int64(1), cache.setCalls.Load(), "普通 failover 应保持单次 SET 热路径")
	require.Equal(t, newAccount.ID, cache.sessionBindings[svc.openAISessionCacheKey(sessionHash)], "未被客户端门否决的旧账号不得阻止普通 failover 更新粘性")
}

func TestCodexClientAdmissionLateVetoPreservesExistingStickyBinding(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	groupID := int64(81)
	restricted := codexAdmissionAccount(482, true)
	replacement := codexAdmissionAccount(483, false)
	cache := &countingCodexStickyCache{}
	svc := &OpenAIGatewayService{
		accountRepo:   &codexAdmissionAccountRepo{byID: map[int64]*Account{restricted.ID: &restricted, replacement.ID: &replacement}},
		cache:         cache,
		cfg:           &config.Config{},
		codexDetector: detector,
	}
	ctx := newCodexAdmissionContext(t, svc)
	const sessionHash = "late-client-veto"
	require.NoError(t, svc.setStickySessionAccountID(ctx, &groupID, sessionHash, restricted.ID, time.Hour))

	vetoed, _, vetoErr := svc.codexClientAdmissionVeto(ctx, &restricted)
	require.NoError(t, vetoErr)
	require.True(t, vetoed, "模拟准入后、发送前才发现限制变化")
	require.NoError(t, svc.BindStickySessionAfterProfitAdmission(ctx, &groupID, sessionHash, replacement.ID))
	require.Equal(t, restricted.ID, cache.sessionBindings[svc.openAISessionCacheKey(sessionHash)], "槽后终检否决也必须保留原 sticky，兼容客户端恢复后可自动重粘连")
}

func TestOpenAITerminalAdmissionRefreshesOnceAndNeverDowngradesFreshSelection(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	selected := codexAdmissionAccount(50, true)
	selected.UpdatedAt = time.Now().UTC()
	stale := codexAdmissionAccount(50, false)
	stale.UpdatedAt = selected.UpdatedAt.Add(-time.Minute)
	cache := &codexAdmissionSnapshotCache{account: &stale}
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{selected.ID: &selected}}
	snapshot := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)
	svc := &OpenAIGatewayService{accountRepo: repo, schedulerSnapshot: snapshot, codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)

	result, err := svc.OpenAITerminalAdmissionLatest(ctx, &selected)
	require.NoError(t, err)
	require.Equal(t, int64(1), repo.getCalls.Load(), "客户端终检必须只做一次权威账号刷新")
	require.Zero(t, cache.calls.Load(), "客户端安全门不得采用可能陈旧的 scheduler cache")
	require.NotSame(t, &selected, result.Account)
	require.True(t, result.ClientVetoed)
	require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.ClientRestriction.Reason)
}

func TestOpenAITerminalAdmissionUsesNewerReplacementObject(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	selected := codexAdmissionAccount(51, false)
	selected.UpdatedAt = time.Now().UTC().Add(-time.Minute)
	replacement := codexAdmissionAccount(51, true)
	replacement.UpdatedAt = selected.UpdatedAt.Add(time.Minute)
	stale := selected
	cache := &codexAdmissionSnapshotCache{account: &stale}
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{selected.ID: &replacement}}
	snapshot := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)
	svc := &OpenAIGatewayService{accountRepo: repo, schedulerSnapshot: snapshot, codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)

	result, err := svc.OpenAITerminalAdmissionLatest(ctx, &selected)
	require.NoError(t, err)
	require.Equal(t, int64(1), repo.getCalls.Load(), "客户端终检必须只做一次权威账号刷新")
	require.Zero(t, cache.calls.Load(), "陈旧 cache 不得掩盖数据库中新开启的限制")
	require.NotSame(t, &selected, result.Account, "终检必须采用缓存中替换后的新对象")
	require.Equal(t, replacement.UpdatedAt, result.Account.UpdatedAt)
	require.True(t, result.ClientVetoed)
	require.False(t, selected.IsCodexCLIOnlyEnabled(), "终检不得通过原地修改旧指针伪造刷新")
}

func TestOpenAITerminalAdmissionRefreshFailureFailsClosed(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	selected := codexAdmissionAccount(53, false)
	cache := &codexAdmissionSnapshotCache{account: &selected}
	repo := &codexAdmissionAccountRepo{getErr: errors.New("database unavailable")}
	snapshot := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)
	svc := &OpenAIGatewayService{accountRepo: repo, schedulerSnapshot: snapshot, codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)

	result, err := svc.OpenAITerminalAdmissionLatest(ctx, &selected)
	require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
	require.False(t, result.ClientVetoed)
	require.Equal(t, int64(1), repo.getCalls.Load())
	require.Zero(t, cache.calls.Load(), "读取失败不能回退到可能陈旧的安全策略缓存")
	ctxErr := CodexClientAdmissionErrorFromContext(ctx)
	require.ErrorIs(t, ctxErr, ErrCodexClientAdmissionUnavailable)
	require.False(t, errors.Is(ctxErr, ErrCodexClientRestricted))
}

func TestOpenAITerminalAdmissionShadowUsesFreshParentPolicy(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	parent := codexAdmissionAccount(54, true)
	parentID := parent.ID
	shadow := codexAdmissionAccount(55, false)
	shadow.ParentAccountID = &parentID
	staleParent := parent
	staleParent.Extra = map[string]any{}
	cache := &codexAdmissionSnapshotCache{account: &staleParent}
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{
		shadow.ID: &shadow,
		parent.ID: &parent,
	}}
	snapshot := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)
	svc := &OpenAIGatewayService{accountRepo: repo, schedulerSnapshot: snapshot, codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)

	result, err := svc.OpenAITerminalAdmissionLatest(ctx, &shadow)
	require.NoError(t, err)
	require.True(t, result.ClientVetoed)
	require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.ClientRestriction.Reason)
	require.Equal(t, int64(2), repo.getCalls.Load(), "影子账号与其母账号都必须从数据库权威读取")
	require.Zero(t, cache.calls.Load(), "陈旧母账号缓存不得绕过客户端限制")
}

func TestOpenAITerminalAdmissionShadowUsesFreshParentTopology(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	oldParent := codexAdmissionAccount(56, false)
	newParent := codexAdmissionAccount(57, true)
	oldParentID := oldParent.ID
	newParentID := newParent.ID
	selected := codexAdmissionAccount(58, false)
	selected.ParentAccountID = &oldParentID
	freshShadow := selected
	freshShadow.ParentAccountID = &newParentID
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{
		freshShadow.ID: &freshShadow,
		newParent.ID:   &newParent,
	}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)

	result, err := svc.OpenAITerminalAdmissionLatest(ctx, &selected)
	require.NoError(t, err)
	require.True(t, result.ClientVetoed)
	require.Equal(t, newParentID, *result.Account.ParentAccountID)
	require.Equal(t, int64(2), repo.getCalls.Load())
}

func TestOpenAITerminalAdmissionShadowParentReadFailureIsUnavailable(t *testing.T) {
	detector := &accountAwareCodexAdmissionDetector{}
	parentID := int64(59)
	shadow := codexAdmissionAccount(60, false)
	shadow.ParentAccountID = &parentID
	repo := &codexAdmissionAccountRepo{
		byID:       map[int64]*Account{shadow.ID: &shadow},
		getErrByID: map[int64]error{parentID: errors.New("parent read failed")},
	}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: detector}
	ctx := newCodexAdmissionContext(t, svc)

	result, err := svc.OpenAITerminalAdmissionLatest(ctx, &shadow)
	require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
	require.False(t, result.ClientVetoed)
	require.Equal(t, int64(2), repo.getCalls.Load())
}

func TestOpenAITerminalAdmissionInactiveOfficialClientSkipsDB(t *testing.T) {
	selected := codexAdmissionAccount(61, true)
	repo := &codexAdmissionAccountRepo{getErr: errors.New("must not be called")}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: alwaysMatchedCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)
	require.False(t, codexClientAdmissionActive(ctx))

	result, err := svc.OpenAITerminalAdmissionLatest(ctx, &selected)
	require.NoError(t, err)
	require.Same(t, &selected, result.Account)
	require.Zero(t, repo.getCalls.Load())
}

func TestOpenAITerminalAdmissionInactiveOfficialShadowSkipsDB(t *testing.T) {
	parentID := int64(611)
	shadow := codexAdmissionAccount(612, true)
	shadow.ParentAccountID = &parentID
	repo := &codexAdmissionAccountRepo{getErr: errors.New("must not be called")}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: alwaysMatchedCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)
	require.False(t, codexClientAdmissionActive(ctx))

	result, err := svc.OpenAITerminalAdmissionLatest(ctx, &shadow)
	require.NoError(t, err)
	require.Same(t, &shadow, result.Account)
	require.False(t, result.ClientVetoed)
	require.Zero(t, repo.getCalls.Load(), "官方客户端的影子账号不得新增母账号数据库读取")
	lookupCalls := 0
	vetoed, reason := svc.codexClientAdmissionVetoReason(ctx, &shadow, func(int64) *Account {
		lookupCalls++
		panic("inactive admission must not resolve shadow parent")
	})
	require.False(t, vetoed)
	require.Empty(t, reason)
	require.Zero(t, lookupCalls, "官方客户端候选过滤不得解析影子母账号")
}

func TestOpenAITerminalAdmissionSkipsRefreshForAPIKeyAccount(t *testing.T) {
	selected := codexAdmissionAccount(52, false)
	selected.Type = AccountTypeAPIKey
	replacement := selected
	cache := &codexAdmissionSnapshotCache{account: &replacement}
	snapshot := NewSchedulerSnapshotService(cache, nil, &codexAdmissionAccountRepo{}, nil, nil)
	svc := &OpenAIGatewayService{schedulerSnapshot: snapshot, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)

	result, err := svc.OpenAITerminalAdmissionLatest(ctx, &selected)
	require.NoError(t, err)
	require.Zero(t, cache.calls.Load(), "账号级 Codex 限制不适用于 API Key，普通请求不得增加快照读取")
	require.Same(t, &selected, result.Account)
	require.False(t, result.ClientVetoed)
}

func TestOpenAITerminalAdmissionAPIKeyProfitRefreshFailureRemainsFailOpen(t *testing.T) {
	selected := codexAdmissionAccount(62, false)
	selected.Type = AccountTypeAPIKey
	rate := 0.1
	selected.RateMultiplier = &rate
	cache := &codexAdmissionSnapshotCache{err: errors.New("scheduler cache unavailable")}
	repo := &codexAdmissionAccountRepo{getErr: errors.New("database unavailable")}
	snapshot := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)
	svc := &OpenAIGatewayService{accountRepo: repo, schedulerSnapshot: snapshot, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)
	ctx = context.WithValue(ctx, openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{
		groupID:   1,
		platform:  PlatformOpenAI,
		threshold: 0.2,
	})

	result, err := svc.OpenAITerminalAdmissionLatest(ctx, &selected)
	require.NoError(t, err, "API Key 仅有利润门时仍沿用快照刷新失败 fail-open 语义")
	require.Same(t, &selected, result.Account)
	require.False(t, result.ProfitVetoed)
	require.False(t, result.ClientVetoed)
	require.Equal(t, int64(1), cache.calls.Load())
	require.Equal(t, int64(1), repo.getCalls.Load(), "利润门快照 miss 仍沿用既有数据库 fallback；失败后必须 fail-open")
}
