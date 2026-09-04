package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type codexProfileGatewayCache struct {
	GatewayCache
	values      map[string]int64
	deleted     map[string]bool
	setCalls    int
	failSetCall int
}

type codexProfileAtomicErrorCache struct {
	*codexProfileGatewayCache
}

func (c *codexProfileAtomicErrorCache) RebindCodexProfileAffinity(
	context.Context,
	int64,
	string,
	string,
	string,
	int64,
	time.Duration,
) error {
	return errors.New("injected atomic affinity error")
}

type codexDeviceLeaseCache struct {
	ConcurrencyCache
	mu     sync.Mutex
	owners map[string]string
}

func (c *codexDeviceLeaseCache) AcquireCodexDeviceConversationLease(_ context.Context, slotKey, leaseID string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owners == nil {
		c.owners = make(map[string]string)
	}
	if c.owners[slotKey] != "" {
		return false, nil
	}
	c.owners[slotKey] = leaseID
	return true, nil
}

func (c *codexDeviceLeaseCache) RefreshCodexDeviceConversationLease(_ context.Context, slotKey, leaseID string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.owners[slotKey] == leaseID, nil
}

func (c *codexDeviceLeaseCache) ReleaseCodexDeviceConversationLease(_ context.Context, slotKey, leaseID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owners[slotKey] == leaseID {
		delete(c.owners, slotKey)
	}
	return nil
}

func (c *codexProfileGatewayCache) GetSessionAccountID(_ context.Context, _ int64, key string) (int64, error) {
	value, ok := c.values[key]
	if !ok {
		return 0, ErrStickySessionNotFound
	}
	return value, nil
}

func (c *codexProfileGatewayCache) SetSessionAccountID(_ context.Context, _ int64, key string, accountID int64, _ time.Duration) error {
	c.setCalls++
	if c.failSetCall > 0 && c.setCalls == c.failSetCall {
		return errors.New("injected affinity cache write failure")
	}
	if c.values == nil {
		c.values = make(map[string]int64)
	}
	c.values[key] = accountID
	return nil
}

func (c *codexProfileGatewayCache) DeleteSessionAccountID(_ context.Context, _ int64, key string) error {
	delete(c.values, key)
	if c.deleted == nil {
		c.deleted = make(map[string]bool)
	}
	c.deleted[key] = true
	return nil
}

func (c *codexProfileGatewayCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

type codexProfileGatewayAccountRepo struct {
	AccountRepository
	accounts      map[int64]*Account
	schedulable   []Account
	resolvedSlots map[int64]*CodexResolvedDeviceSlot
	rebinds       [][2]int64
}

func (r *codexProfileGatewayAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]Account, error) {
	result := make([]Account, 0, len(r.schedulable))
	for _, account := range r.schedulable {
		if account.Platform == platform {
			result = append(result, account)
		}
	}
	return result, nil
}

func (r *codexProfileGatewayAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	return r.accounts[id], nil
}

func (r *codexProfileGatewayAccountRepo) ResolveCodexDeviceBinding(_ context.Context, accountID, apiKeyID int64, osClass CodexOSClass, surface CodexClientSurface) (*CodexResolvedDeviceSlot, error) {
	if slot := r.resolvedSlots[accountID]; slot != nil {
		copySlot := *slot
		copySlot.APIKeyID = apiKeyID
		copySlot.OSClass = osClass
		copySlot.CanonicalSurface = surface
		return &copySlot, nil
	}
	return nil, fmt.Errorf("missing test binding for account %d: %w", accountID, ErrDeviceProfileUnsupported)
}

func (r *codexProfileGatewayAccountRepo) RebindCodexDeviceBinding(_ context.Context, oldAccountID, newAccountID, apiKeyID int64, osClass CodexOSClass, surface CodexClientSurface) (*CodexResolvedDeviceSlot, error) {
	r.rebinds = append(r.rebinds, [2]int64{oldAccountID, newAccountID})
	return r.ResolveCodexDeviceBinding(context.Background(), newAccountID, apiKeyID, osClass, surface)
}

func (r *codexProfileGatewayAccountRepo) DeleteCodexDeviceBinding(context.Context, int64, int64, CodexOSClass, CodexClientSurface) error {
	return nil
}

func (r *codexProfileGatewayAccountRepo) ListCodexDeviceSlots(context.Context, int64, CodexOSClass, CodexClientSurface, bool) ([]CodexResolvedDeviceSlot, error) {
	return nil, nil
}

func (r *codexProfileGatewayAccountRepo) FinalizeDrainedCodexDeviceSlots(context.Context, int64) (int64, error) {
	return 0, nil
}

type codexProfileEchoUpstream struct {
	requestHeader http.Header
	requestBody   []byte
	stream        bool
}

func (u *codexProfileEchoUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.requestHeader = req.Header.Clone()
	u.requestBody = body
	session := gjson.GetBytes(body, "client_metadata.session_id").String()
	metadata := gjson.GetBytes(body, "client_metadata").Raw
	if metadata == "" {
		metadata = `{}`
	}
	if u.stream {
		streamBody := "data: {\"type\":\"response.created\",\"response\":{\"client_metadata\":" + metadata + "}}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + session + "\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_profile\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(streamBody)),
		}, nil
	}
	responseBody := `{"id":"resp_profile","model":"gpt-5.6-sol","client_metadata":` + metadata + `,"output":[{"type":"message","content":[{"type":"output_text","text":"` + session + `"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}, nil
}

func (u *codexProfileEchoUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestCodexProfileAffinityMissPreservesLegacyStickyForOffAccount(t *testing.T) {
	cache := &codexProfileGatewayCache{values: map[string]int64{}}
	legacy := &Account{
		ID: 99, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, ProvisioningState: AccountProvisioningActive,
		CodexIdentityPolicy: DefaultCodexIdentityPolicySpec(),
	}
	svc := &OpenAIGatewayService{
		cache:       cache,
		accountRepo: &codexProfileGatewayAccountRepo{accounts: map[int64]*Account{legacy.ID: legacy}},
	}
	ctx := codexProfileTestContext(7, 101, CodexClientProfile{OSClass: CodexOSLinux, Surface: CodexSurfaceCLI, Architecture: CodexArchARM64}, "session-a")
	cache.values[svc.openAISessionCacheKey("session-a")] = 99

	accountID, err := svc.resolveCodexAwareStickyAccountID(ctx, nil, "session-a")
	require.NoError(t, err)
	require.Equal(t, int64(99), accountID)
}

func TestCodexProfileKeysSeparateSurfacesWithinOS(t *testing.T) {
	base := codexProfileRequest{
		Profile:     CodexClientProfile{OSClass: CodexOSWindows, Surface: CodexSurfaceDesktop},
		APIKeyScope: "user:7|key:101",
	}
	desktopKey := codexProfileAffinityKey(base, "shared-session", 3, false)
	base.Profile.Surface = CodexSurfaceCLI
	cliKey := codexProfileAffinityKey(base, "shared-session", 3, false)
	require.NotEqual(t, desktopKey, cliKey)

	desktopCtx := withCodexProfileRequest(WithHTTPUpstreamIsolationScope(context.Background(), 7, 101), codexProfileRequest{
		Profile: CodexClientProfile{OSClass: CodexOSWindows, Surface: CodexSurfaceDesktop}, APIKeyScope: "user:7|key:101",
	})
	cliCtx := withCodexProfileRequest(WithHTTPUpstreamIsolationScope(context.Background(), 7, 101), codexProfileRequest{
		Profile: CodexClientProfile{OSClass: CodexOSWindows, Surface: CodexSurfaceCLI}, APIKeyScope: "user:7|key:101",
	})
	require.NotEqual(t, scopedOpenAIWSStateKey(desktopCtx, "shared-session"), scopedOpenAIWSStateKey(cliCtx, "shared-session"))
}

func TestCodexProfileAffinityMissRejectsLegacyStickyForNewModeAccount(t *testing.T) {
	cache := &codexProfileGatewayCache{values: map[string]int64{}}
	profileAccount := codexProfileTestAccount(t, 77, CodexOSLinux, CodexSurfaceCLI, CodexArchX8664, false)
	svc := &OpenAIGatewayService{
		cache:       cache,
		accountRepo: &codexProfileGatewayAccountRepo{accounts: map[int64]*Account{profileAccount.ID: profileAccount}},
	}
	ctx := codexProfileTestContext(7, 101, CodexClientProfile{OSClass: CodexOSLinux, Surface: CodexSurfaceCLI, Architecture: CodexArchARM64}, "session-new-mode")
	legacyKey := svc.openAISessionCacheKey("session-new-mode")
	cache.values[legacyKey] = profileAccount.ID

	accountID, err := svc.resolveCodexAwareStickyAccountID(ctx, nil, "session-new-mode")
	require.NoError(t, err)
	require.Zero(t, accountID)
	require.True(t, cache.deleted[legacyKey])
}

func TestCodexProfileAffinityCannotRestorePendingAccountWithoutSnapshot(t *testing.T) {
	cache := &codexProfileGatewayCache{values: map[string]int64{}}
	account := codexProfileTestAccount(t, 77, CodexOSLinux, CodexSurfaceCLI, CodexArchX8664, false)
	account.ProvisioningState = AccountProvisioningPending
	repo := &codexProfileGatewayAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	svc := &OpenAIGatewayService{cache: cache, accountRepo: repo}
	ctx := codexProfileTestContext(7, 101, CodexClientProfile{OSClass: CodexOSLinux, Surface: CodexSurfaceCLI, Architecture: CodexArchARM64}, "session-pending")
	request, ok := codexProfileRequestFromContext(ctx)
	require.True(t, ok)
	indexKey := codexProfileAffinityKey(request, "session-pending", 0, true)
	bindingKey := codexProfileAffinityKey(request, "session-pending", account.CodexIdentityPolicy.Version, false)
	cache.values[indexKey] = account.ID
	cache.values[bindingKey] = account.ID

	accountID, handled, err := svc.getCodexProfileAffinityAccountID(ctx, nil, "session-pending")
	require.NoError(t, err)
	require.True(t, handled)
	require.Zero(t, accountID)
	require.True(t, cache.deleted[indexKey])
	require.True(t, cache.deleted[bindingKey])
}

func TestCodexProfileAffinityPartialBindingStillPreventsLegacyDowngrade(t *testing.T) {
	cache := &codexProfileGatewayCache{values: map[string]int64{}}
	profileAccount := codexProfileTestAccount(t, 81, CodexOSLinux, CodexSurfaceCLI, CodexArchX8664, false)
	svc := &OpenAIGatewayService{
		cache:       cache,
		accountRepo: &codexProfileGatewayAccountRepo{accounts: map[int64]*Account{profileAccount.ID: profileAccount}},
	}
	ctx := codexProfileTestContext(7, 101, CodexClientProfile{OSClass: CodexOSLinux, Surface: CodexSurfaceCLI, Architecture: CodexArchARM64}, "partial-binding")
	request, ok := codexProfileRequestFromContext(ctx)
	require.True(t, ok)
	indexKey := codexProfileAffinityKey(request, "partial-binding", 0, true)
	cache.values[indexKey] = profileAccount.ID

	accountID, handled, err := svc.getCodexProfileAffinityAccountID(ctx, nil, "partial-binding")
	require.ErrorIs(t, err, ErrStickySessionNotFound)
	require.True(t, handled)
	require.Zero(t, accountID)
	require.True(t, codexProfileAffinityActive(ctx))
	offAccount := &Account{ID: 82, Platform: PlatformOpenAI, Type: AccountTypeOAuth, CodexIdentityPolicy: DefaultCodexIdentityPolicySpec()}
	require.False(t, svc.codexProfileAccountCompatible(ctx, offAccount))
}

func TestCodexProfileAffinityFallbackCacheFailsClosedOnPartialWrite(t *testing.T) {
	cache := &codexProfileGatewayCache{values: map[string]int64{}, failSetCall: 2}
	first := codexProfileTestAccount(t, 83, CodexOSLinux, CodexSurfaceCLI, CodexArchX8664, false)
	second := codexProfileTestAccount(t, 84, CodexOSLinux, CodexSurfaceCLI, CodexArchX8664, false)
	repo := &codexProfileGatewayAccountRepo{accounts: map[int64]*Account{first.ID: first, second.ID: second}}
	svc := &OpenAIGatewayService{cache: cache, accountRepo: repo}
	ctx := codexProfileTestContext(7, 101, CodexClientProfile{
		OSClass: CodexOSLinux, Surface: CodexSurfaceCLI, Architecture: CodexArchX8664,
	}, "no-bounce-session")
	request, ok := codexProfileRequestFromContext(ctx)
	require.True(t, ok)
	indexKey := codexProfileAffinityKey(request, "no-bounce-session", 0, true)
	bindingKey := codexProfileAffinityKey(request, "no-bounce-session", first.CodexIdentityPolicy.Version, false)
	cache.values[indexKey] = first.ID
	cache.values[bindingKey] = first.ID

	handled, err := svc.setCodexProfileAffinityAccountID(ctx, nil, "no-bounce-session", second.ID)
	require.True(t, handled)
	require.ErrorContains(t, err, "injected affinity cache write failure")
	require.NotContains(t, cache.values, indexKey, "a failed publish must not leave the old account index readable")
	require.NotContains(t, cache.values, bindingKey, "the unpublished replacement binding must be removed")

	accountID, handled, err := svc.getCodexProfileAffinityAccountID(ctx, nil, "no-bounce-session")
	require.NoError(t, err)
	require.False(t, handled)
	require.Zero(t, accountID)
}

func TestCodexProfileAffinityAtomicErrorClearsAmbiguousBinding(t *testing.T) {
	baseCache := &codexProfileGatewayCache{values: map[string]int64{}}
	cache := &codexProfileAtomicErrorCache{codexProfileGatewayCache: baseCache}
	first := codexProfileTestAccount(t, 85, CodexOSLinux, CodexSurfaceCLI, CodexArchX8664, false)
	second := codexProfileTestAccount(t, 86, CodexOSLinux, CodexSurfaceCLI, CodexArchX8664, false)
	svc := &OpenAIGatewayService{
		cache: cache,
		accountRepo: &codexProfileGatewayAccountRepo{
			accounts: map[int64]*Account{first.ID: first, second.ID: second},
		},
	}
	ctx := codexProfileTestContext(7, 101, CodexClientProfile{
		OSClass: CodexOSLinux, Surface: CodexSurfaceCLI, Architecture: CodexArchX8664,
	}, "ambiguous-session")
	request, ok := codexProfileRequestFromContext(ctx)
	require.True(t, ok)
	indexKey := codexProfileAffinityKey(request, "ambiguous-session", 0, true)
	bindingKey := codexProfileAffinityKey(request, "ambiguous-session", first.CodexIdentityPolicy.Version, false)
	baseCache.values[indexKey] = first.ID
	baseCache.values[bindingKey] = first.ID

	handled, err := svc.setCodexProfileAffinityAccountID(ctx, nil, "ambiguous-session", second.ID)
	require.True(t, handled)
	require.ErrorContains(t, err, "injected atomic affinity error")
	require.NotContains(t, baseCache.values, indexKey)
	require.NotContains(t, baseCache.values, bindingKey)
}

func TestCodexProfilePolicyDoesNotFilterNonCodexRequestWithoutProfileContext(t *testing.T) {
	account := codexProfileTestAccount(t, 78, CodexOSLinux, CodexSurfaceCLI, CodexArchX8664, false)
	svc := &OpenAIGatewayService{}
	require.True(t, svc.codexProfileAccountCompatible(context.Background(), account))
	offAccount := &Account{ID: 79, Platform: PlatformOpenAI, Type: AccountTypeOAuth, CodexIdentityPolicy: DefaultCodexIdentityPolicySpec()}
	require.True(t, svc.codexProfileAccountCompatible(context.Background(), offAccount))
	require.False(t, svc.codexProfileAccountCompatible(withCodexProfileAffinityActive(context.Background()), offAccount), "an established Profile affinity cannot downgrade to legacy off mode")
}

func TestGenerateSessionHashDoesNotStageProfileForNonResponsesRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/responses/input_tokens", "/v1/images/generations"} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		request := httptest.NewRequest(http.MethodPost, path, nil)
		request.Header.Set("User-Agent", "codex_cli_rs/0.146.0 (Linux; x86_64) xterm")
		request = request.WithContext(WithHTTPUpstreamIsolationScope(request.Context(), 7, 101))
		c.Request = request
		body := []byte(`{"prompt_cache_key":"session"}`)
		require.NotEmpty(t, (&OpenAIGatewayService{}).GenerateSessionHash(c, body))
		_, staged := codexProfileRequestFromContext(c.Request.Context())
		require.False(t, staged, path)
	}
}

func TestCodexProfileRepeatedStagePreservesAffinityActiveState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newCodexProfileGatewayContext(t, 7, 101, nil)
	stageCodexProfileRequest(c, []byte(`{"prompt_cache_key":"first"}`), "first")
	markCodexProfileAffinityActive(c.Request.Context())
	require.True(t, codexProfileAffinityActive(c.Request.Context()))
	stageCodexProfileRequest(c, []byte(`{"prompt_cache_key":"second"}`), "second")
	require.True(t, codexProfileAffinityActive(c.Request.Context()), "restaging a later WS turn must retain the failover-mode marker")
}

func TestGenerateSessionHashFallbackRestagesProfileConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newCodexProfileGatewayContext(t, 7, 101, nil)
	hash := (&OpenAIGatewayService{}).GenerateSessionHashWithFallback(c, []byte(`{}`), "stable-ws-fallback")
	require.NotEmpty(t, hash)
	request, ok := codexProfileRequestFromContext(c.Request.Context())
	require.True(t, ok)
	require.Equal(t, hash, request.ConversationHash)
}

func TestResponsesProfileContextFiltersHigherPriorityIncompatibleAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	windows := codexProfileTestAccount(t, 83, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	windows.Priority = 1
	linux := codexProfileTestAccount(t, 84, CodexOSLinux, CodexSurfaceCLI, CodexArchX8664, false)
	linux.Priority = 2
	repo := &codexProfileGatewayAccountRepo{
		accounts:    map[int64]*Account{windows.ID: windows, linux.ID: linux},
		schedulable: []Account{*windows, *linux},
	}
	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       &codexProfileGatewayCache{values: map[string]int64{}},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	request.Header.Set("User-Agent", "codex_cli_rs/0.146.0 (Ubuntu 22.04; arm64) xterm-256color")
	request = request.WithContext(WithHTTPUpstreamIsolationScope(request.Context(), 7, 101))
	c.Request = request
	body := []byte(`{"model":"gpt-5.6-sol","prompt_cache_key":"linux-session"}`)
	sessionHash := svc.GenerateSessionHash(c, body)
	require.NotEmpty(t, sessionHash)
	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		c.Request.Context(), nil, "", sessionHash, "", nil,
		OpenAIUpstreamTransportAny, "", false, false, true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, linux.ID, selection.Account.ID, "the incompatible higher-priority Windows account must be filtered before admission")
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayCodexProfileHTTPRoundTripPreservesModelText(t *testing.T) {
	for _, tt := range []struct {
		name        string
		passthrough bool
		stream      bool
	}{
		{name: "non passthrough json"},
		{name: "non passthrough sse", stream: true},
		{name: "passthrough sse", passthrough: true, stream: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			account := codexProfileTestAccount(t, 91, CodexOSWindows, CodexSurfaceCLI, CodexArchX8664, tt.passthrough)
			repo := &codexProfileGatewayAccountRepo{
				accounts: map[int64]*Account{account.ID: account},
				resolvedSlots: map[int64]*CodexResolvedDeviceSlot{
					account.ID: {
						AccountID: account.ID, SlotID: 501, ProfileID: 401,
						OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceCLI,
						Architecture: CodexArchX8664, CatalogVersion: 1,
						SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
					},
				},
			}
			upstream := &codexProfileEchoUpstream{stream: tt.stream}
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			svc := &OpenAIGatewayService{
				accountRepo:   repo,
				cache:         &codexProfileGatewayCache{values: map[string]int64{}},
				cfg:           cfg,
				httpUpstream:  upstream,
				toolCorrector: NewCodexToolCorrector(),
			}

			body := []byte(`{"model":"gpt-5.6-sol","stream":` + boolString(tt.stream) + `,"prompt_cache_key":"client-cache","input":[{"role":"user","content":"hello"}],"client_metadata":{"session_id":"client-session","os":"windows","arch":"arm64","surface":"cli"}}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			request.Header.Set("User-Agent", "codex_cli_rs/0.146.0 (Windows 11; arm64) WindowsTerminal")
			request.Header.Set("originator", "codex_cli_rs")
			request.Header.Set("session-id", "client-session")
			request = request.WithContext(WithHTTPUpstreamIsolationScope(request.Context(), 7, 101))
			c.Request = request
			groupID := int64(3)
			c.Set("api_key", &APIKey{ID: 101, GroupID: &groupID, User: &User{ID: 7}})
			require.NotEmpty(t, svc.GenerateSessionHash(c, body))

			result, err := svc.Forward(c.Request.Context(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "windows", gjson.GetBytes(upstream.requestBody, "client_metadata.os").String())
			require.Equal(t, "x86_64", gjson.GetBytes(upstream.requestBody, "client_metadata.arch").String())
			require.Equal(t, "cli", gjson.GetBytes(upstream.requestBody, "client_metadata.surface").String())
			require.Equal(t, "codex_cli_rs", upstream.requestHeader.Get("originator"))
			require.Equal(t, gjson.GetBytes(upstream.requestBody, "client_metadata.user_agent").String(), upstream.requestHeader.Get("user-agent"))
			alias := gjson.GetBytes(upstream.requestBody, "client_metadata.session_id").String()
			require.NotEmpty(t, alias)
			require.NotEqual(t, "client-session", alias)
			if tt.stream {
				require.Contains(t, recorder.Body.String(), `"session_id":"client-session"`)
				require.Contains(t, recorder.Body.String(), `"os":"windows"`)
				require.Contains(t, recorder.Body.String(), `"arch":"arm64"`)
				require.Contains(t, recorder.Body.String(), `"surface":"cli"`)
				require.NotContains(t, recorder.Body.String(), `"user_agent"`)
				require.Contains(t, recorder.Body.String(), `"delta":"`+alias+`"`, "ordinary model text must not be restored")
			} else {
				require.Equal(t, "client-session", gjson.Get(recorder.Body.String(), "client_metadata.session_id").String())
				require.Equal(t, "windows", gjson.Get(recorder.Body.String(), "client_metadata.os").String())
				require.Equal(t, "arm64", gjson.Get(recorder.Body.String(), "client_metadata.arch").String())
				require.Equal(t, "cli", gjson.Get(recorder.Body.String(), "client_metadata.surface").String())
				require.False(t, gjson.Get(recorder.Body.String(), "client_metadata.user_agent").Exists())
				require.Equal(t, alias, gjson.Get(recorder.Body.String(), "output.0.content.0.text").String(), "ordinary model text must not be restored")
			}
		})
	}
}

func TestCodexProfileCompactKeepsLegacyBodyShape(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprintf("passthrough=%t", passthrough), func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			account := codexProfileTestAccount(t, 190, CodexOSWindows, CodexSurfaceCLI, CodexArchX8664, passthrough)
			repo := &codexProfileGatewayAccountRepo{
				accounts: map[int64]*Account{account.ID: account},
				resolvedSlots: map[int64]*CodexResolvedDeviceSlot{account.ID: {
					AccountID: account.ID, SlotID: 19001, ProfileID: 19000,
					OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceCLI,
					Architecture: CodexArchX8664, CatalogVersion: 1,
					SlotIndex: 0, Epoch: 1, State: "active", PolicyVersion: 1,
				}},
			}
			upstream := &codexProfileEchoUpstream{}
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			svc := &OpenAIGatewayService{
				accountRepo: repo, cache: &codexProfileGatewayCache{values: map[string]int64{}},
				cfg: cfg, httpUpstream: upstream, toolCorrector: NewCodexToolCorrector(),
			}
			body := []byte(`{"model":"gpt-5.6-sol","stream":false,"instructions":"compact","input":"hello"}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			request := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(body))
			request.Header.Set("User-Agent", "codex_cli_rs/0.146.0 (Windows 11; arm64) WindowsTerminal")
			request.Header.Set("session-id", "client-session")
			request = request.WithContext(WithHTTPUpstreamIsolationScope(request.Context(), 7, 101))
			c.Request = request
			c.Set("api_key", &APIKey{ID: 101, User: &User{ID: 7}})
			require.NotEmpty(t, svc.GenerateSessionHash(c, body))

			_, err := svc.Forward(c.Request.Context(), c, account, body)
			require.NoError(t, err)
			require.False(t, gjson.GetBytes(upstream.requestBody, "client_metadata").Exists())
			require.Equal(t, "compact", gjson.GetBytes(upstream.requestBody, "instructions").String())
			require.Equal(t, "codex_cli_rs", upstream.requestHeader.Get("originator"))
			require.NotEmpty(t, upstream.requestHeader.Get("session-id"))
		})
	}
}

func TestCodexProfileSlotClientVersionDrivesHeadersAndMetadata(t *testing.T) {
	for _, tt := range []struct {
		name          string
		mode          CodexClientVersionMode
		pinnedVersion string
		wantVersion   string
	}{
		{name: "inherit", mode: CodexClientVersionInherit, wantVersion: "0.201.0"},
		{name: "pinned", mode: CodexClientVersionPinned, pinnedVersion: "0.188.0", wantVersion: "0.188.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := codexProfileTestAccount(t, 290, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
			account.CodexIdentityPolicy.Profiles[0].Slots = []CodexDeviceSlotPolicy{{
				Index: 0, ClientVersionMode: tt.mode, ClientVersion: tt.pinnedVersion,
			}}
			repo := &codexProfileGatewayAccountRepo{
				accounts: map[int64]*Account{account.ID: account},
				resolvedSlots: map[int64]*CodexResolvedDeviceSlot{account.ID: {
					AccountID: account.ID, SlotID: 29001, ProfileID: 29000,
					OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
					Architecture: CodexArchX8664, CatalogVersion: 1,
					SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
					ClientVersionMode: tt.mode, ClientVersion: tt.pinnedVersion,
				}},
			}
			settings := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
				SettingKeyOpenAICodexClientVersion: "0.201.0",
			}}, nil)
			svc := &OpenAIGatewayService{accountRepo: repo, settingService: settings}
			body := []byte(`{"model":"gpt-5.6-sol","prompt_cache_key":"version-session","client_metadata":{"session_id":"client-session","surface":"desktop","version":"0.1.0","user_agent":"client-ua","app_build":"client-build"}}`)
			c := newCodexProfileGatewayContext(t, 7, 101, body)
			c.Request.Header.Set("x-codex-turn-metadata", `{"version":"0.1.0","user_agent":"client-ua","app_build":"client-build"}`)
			require.NotEmpty(t, svc.GenerateSessionHash(c, body))

			prepared, err := svc.PrepareCodexProfileAttempt(c.Request.Context(), c, account, body)
			require.NoError(t, err)
			defer svc.ReleaseCodexProfileAttempt(c, prepared)
			plan := stagedCodexIdentityAttemptPlan(c, prepared)
			require.NotNil(t, plan)
			require.Equal(t, tt.wantVersion, plan.Profile.Version)
			require.Contains(t, plan.Profile.UserAgent, "/"+tt.wantVersion+" ")
			require.Equal(t, "26.616.71553", plan.Profile.AppBuild, "slot version must not change the Desktop app build")

			headers := c.Request.Header.Clone()
			require.True(t, applyStagedCodexProfileHeaders(c, prepared, headers))
			enforceCodexIdentityHeadersForAttempt(c, prepared, headers, "")
			require.Equal(t, plan.Profile.Version, headers.Get("version"))
			require.Equal(t, plan.Profile.UserAgent, headers.Get("user-agent"))
			require.Equal(t, plan.Profile.Originator, headers.Get("originator"))
			require.Equal(t, plan.Profile.Version, gjson.Get(headers.Get("x-codex-turn-metadata"), "version").String())
			require.Equal(t, plan.Profile.UserAgent, gjson.Get(headers.Get("x-codex-turn-metadata"), "user_agent").String())
			require.Equal(t, plan.Profile.AppBuild, gjson.Get(headers.Get("x-codex-turn-metadata"), "app_build").String())

			upstreamBody, changed, err := ApplyCodexIdentityPlanToJSON(body, plan)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, plan.Profile.Version, gjson.GetBytes(upstreamBody, "client_metadata.version").String())
			require.Equal(t, plan.Profile.UserAgent, gjson.GetBytes(upstreamBody, "client_metadata.user_agent").String())
			require.Equal(t, plan.Profile.AppBuild, gjson.GetBytes(upstreamBody, "client_metadata.app_build").String())
		})
	}
}

func TestCodexProfileSlotClientVersionRejectsUnknownMode(t *testing.T) {
	svc := &OpenAIGatewayService{}
	_, err := svc.resolveCodexDeviceSlotClientVersion(context.Background(), &CodexResolvedDeviceSlot{
		ClientVersionMode: CodexClientVersionMode("custom"), ClientVersion: "0.200.0",
	})
	require.ErrorContains(t, err, "unsupported Codex client version mode")
}

func TestCodexProfileSlotClientVersionFallsBackOrRejectsVersionsBelowUpstreamMinimum(t *testing.T) {
	settings := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
		SettingKeyOpenAICodexClientVersion: "0.143.9",
	}}, nil)
	svc := &OpenAIGatewayService{settingService: settings}

	version, err := svc.resolveCodexDeviceSlotClientVersion(context.Background(), &CodexResolvedDeviceSlot{
		ClientVersionMode: CodexClientVersionInherit,
	})
	require.NoError(t, err)
	require.Equal(t, codexCLIVersion, version)

	_, err = svc.resolveCodexDeviceSlotClientVersion(context.Background(), &CodexResolvedDeviceSlot{
		ClientVersionMode: CodexClientVersionPinned,
		ClientVersion:     "0.143.9",
	})
	require.ErrorContains(t, err, codexUpstreamMinVersion)
}

func TestCodexProfilePassthroughJSONResponseRestoresKnownIdentityOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	input := codexRuntimeAttemptInput(t)
	plan, err := BuildCodexIdentityAttemptPlan(input)
	require.NoError(t, err)
	alias := plan.UpstreamValue(CodexIdentitySession)
	account := &Account{ID: input.AccountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	stageCodexIdentityAttemptPlan(c, plan)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_passthrough_profile","client_metadata":{"session_id":"` + alias + `"},` +
				`"output":[{"type":"message","content":[{"type":"output_text","text":"` + alias + `"}]}],` +
				`"usage":{"input_tokens":1,"output_tokens":1}}`,
		)),
	}
	svc := &OpenAIGatewayService{}
	result, err := svc.handleNonStreamingResponsePassthroughWithAccount(context.Background(), response, c, account, "gpt-5.6-sol", "gpt-5.6-sol")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "client-session", gjson.Get(recorder.Body.String(), "client_metadata.session_id").String())
	require.Equal(t, alias, gjson.Get(recorder.Body.String(), "output.0.content.0.text").String())
}

func TestCodexProfileCyberErrorRestoresStructuredIdentityOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	input := codexRuntimeAttemptInput(t)
	plan, err := BuildCodexIdentityAttemptPlan(input)
	require.NoError(t, err)
	alias := plan.UpstreamValue(CodexIdentitySession)
	account := &Account{ID: input.AccountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	stageCodexIdentityAttemptPlan(c, plan)
	response := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"code":"cyber_policy","message":"ordinary text ` + alias + `","metadata":{"session_id":"` + alias + `"}}}`,
		)),
	}

	result, err := (&OpenAIGatewayService{}).handleErrorResponse(
		context.Background(), response, c, account, nil,
	)
	require.Nil(t, result)
	require.ErrorContains(t, err, "cyber_policy")
	require.Equal(t, "client-session", gjson.Get(recorder.Body.String(), "error.metadata.session_id").String())
	require.Equal(t, "ordinary text "+alias, gjson.Get(recorder.Body.String(), "error.message").String())
}

func TestCodexProfileSharedDevicePreservesPR2WSAndStateIsolation(t *testing.T) {
	base := codexRuntimeAttemptInput(t)
	base.Slot = CodexResolvedSlot{Index: 0, Epoch: 4}
	base.APIKeyScope = "user:7|key:101"
	planA, err := BuildCodexIdentityAttemptPlan(base)
	require.NoError(t, err)
	other := base
	other.APIKeyScope = "user:7|key:202"
	other.ConversationKey = "conversation-b"
	other.Source.SessionID = "client-session-b"
	planB, err := BuildCodexIdentityAttemptPlan(other)
	require.NoError(t, err)
	require.Equal(t, planA.UpstreamValue(CodexIdentityInstallation), planB.UpstreamValue(CodexIdentityInstallation))
	require.NotEqual(t, planA.UpstreamValue(CodexIdentitySession), planB.UpstreamValue(CodexIdentitySession))

	ctxA := WithHTTPUpstreamIsolationScope(context.Background(), 7, 101)
	ctxB := WithHTTPUpstreamIsolationScope(context.Background(), 7, 202)
	stateKeyA := scopedOpenAIWSStateKey(ctxA, "shared-response")
	stateKeyB := scopedOpenAIWSStateKey(ctxB, "shared-response")
	require.NotEqual(t, stateKeyA, stateKeyB)
	store := NewOpenAIWSStateStore(nil)
	require.NoError(t, store.BindResponseAccount(ctxA, 3, stateKeyA, 91, time.Hour))
	require.NoError(t, store.BindResponseAccount(ctxB, 3, stateKeyB, 92, time.Hour))
	accountA, err := store.GetResponseAccount(ctxA, 3, stateKeyA)
	require.NoError(t, err)
	accountB, err := store.GetResponseAccount(ctxB, 3, stateKeyB)
	require.NoError(t, err)
	require.Equal(t, int64(91), accountA)
	require.Equal(t, int64(92), accountB)
}

func TestCodexProfileWSHandshakeCompatibilityIncludesProfileSlotAndProxy(t *testing.T) {
	account := codexProfileTestAccount(t, 391, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	base := http.Header{
		"User-Agent":              {"Codex Desktop/0.146.0 (Windows 11; x86_64) unknown (Codex Desktop; 26.616.71553)"},
		"Originator":              {"Codex Desktop"},
		"Version":                 {"0.146.0"},
		"X-Codex-Installation-Id": {"device-slot-0"},
		"Session-Id":              {"session-a"},
		"Thread-Id":               {"thread-a"},
		"X-Codex-Window-Id":       {"window-a"},
	}
	key := normalizeOpenAIWSHandshakeCompatibility(account, base, "http://proxy-a:8080")

	otherProfile := base.Clone()
	otherProfile.Set("User-Agent", "codex_cli_rs/0.146.0 (Windows 11; x86_64) WindowsTerminal")
	otherProfile.Set("Originator", "codex_cli_rs")
	require.NotEqual(t, key, normalizeOpenAIWSHandshakeCompatibility(account, otherProfile, "http://proxy-a:8080"))

	otherSlot := base.Clone()
	otherSlot.Set("X-Codex-Installation-Id", "device-slot-1")
	require.NotEqual(t, key, normalizeOpenAIWSHandshakeCompatibility(account, otherSlot, "http://proxy-a:8080"))
	require.NotEqual(t, key, normalizeOpenAIWSHandshakeCompatibility(account, base, "http://proxy-b:8080"))

	legacy := &Account{ID: 392, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	legacyA := normalizeOpenAIWSHandshakeCompatibility(legacy, base, "http://proxy-a:8080")
	legacyB := normalizeOpenAIWSHandshakeCompatibility(legacy, otherProfile, "http://proxy-b:8080")
	require.Equal(t, legacyA, legacyB, "new compatibility dimensions must not change legacy off-mode pooling")
}

func TestCodexDeviceSharedAdapterFailureReleasesDistributedLease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := codexProfileTestAccount(t, 191, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	account.CodexIdentityPolicy.SessionPolicy = CodexSessionPolicySpec{
		Mode:                          CodexSessionDeviceShared,
		MaxActiveConversationsPerSlot: 1,
		DisableCrossKeyContinuation:   true,
	}
	repo := &codexProfileGatewayAccountRepo{
		accounts: map[int64]*Account{account.ID: account},
		resolvedSlots: map[int64]*CodexResolvedDeviceSlot{
			account.ID: {
				AccountID: account.ID, SlotID: 901, ProfileID: 801,
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
				Architecture: CodexArchX8664, CatalogVersion: 1,
				SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
			},
		},
	}
	leaseCache := &codexDeviceLeaseCache{}
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              &codexProfileGatewayCache{values: map[string]int64{}},
		concurrencyService: NewConcurrencyService(leaseCache),
		cfg:                &config.Config{},
	}
	malformed := []byte(`{"model":"gpt-5.6-sol","stream":false,"client_metadata":"not-an-object"}`)
	c1 := newCodexProfileGatewayContext(t, 7, 101, malformed)
	require.NotEmpty(t, svc.GenerateSessionHash(c1, malformed))
	_, err := svc.Forward(c1.Request.Context(), c1, account, malformed)
	require.ErrorContains(t, err, "client_metadata must be an object")

	valid := []byte(`{"model":"gpt-5.6-sol","stream":false,"client_metadata":{"session_id":"second"}}`)
	c2 := newCodexProfileGatewayContext(t, 8, 202, valid)
	require.NotEmpty(t, svc.GenerateSessionHash(c2, valid))
	prepared, err := svc.PrepareCodexProfileAttempt(c2.Request.Context(), c2, account, valid)
	require.NoError(t, err, "the adapter failure must release the slot immediately")
	require.NotNil(t, prepared)
	svc.ReleaseCodexProfileAttempt(c2, prepared)
}

func TestCodexDeviceSharedAttemptReleaseDoesNotCancelFailoverBaseContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	first := codexProfileTestAccount(t, 211, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	second := codexProfileTestAccount(t, 212, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	for _, account := range []*Account{first, second} {
		account.CodexIdentityPolicy.SessionPolicy = CodexSessionPolicySpec{
			Mode: CodexSessionDeviceShared, MaxActiveConversationsPerSlot: 1, DisableCrossKeyContinuation: true,
		}
	}
	second.Extra[codexFingerprintSeedExtraKey] = "22222222-2222-4222-8222-222222222222"
	repo := &codexProfileGatewayAccountRepo{
		accounts: map[int64]*Account{first.ID: first, second.ID: second},
		resolvedSlots: map[int64]*CodexResolvedDeviceSlot{
			first.ID: {
				AccountID: first.ID, SlotID: 21101, OSClass: CodexOSWindows,
				CanonicalSurface: CodexSurfaceDesktop, Architecture: CodexArchX8664,
				CatalogVersion: 1, SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
			},
			second.ID: {
				AccountID: second.ID, SlotID: 21201, OSClass: CodexOSWindows,
				CanonicalSurface: CodexSurfaceDesktop, Architecture: CodexArchX8664,
				CatalogVersion: 1, SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
			},
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              &codexProfileGatewayCache{values: map[string]int64{}},
		concurrencyService: NewConcurrencyService(&codexDeviceLeaseCache{}),
	}
	body := []byte(`{"model":"gpt-5.6-sol","client_metadata":{"session_id":"client-session"}}`)
	c := newCodexProfileGatewayContext(t, 7, 101, body)
	require.NotEmpty(t, svc.GenerateSessionHashWithFallback(c, body, "device-shared-failover"))
	baseCtx := c.Request.Context()
	preparedFirst, err := svc.PrepareCodexProfileAttempt(baseCtx, c, first, body)
	require.NoError(t, err)
	require.True(t, codexProfileAffinityActive(baseCtx))
	offAccount := &Account{ID: 213, Platform: PlatformOpenAI, Type: AccountTypeOAuth, CodexIdentityPolicy: DefaultCodexIdentityPolicySpec()}
	require.False(t, svc.codexProfileAccountCompatible(baseCtx, offAccount), "the failover pool cannot downgrade to off after entering Profile mode")
	attemptCtx := svc.CodexProfileAttemptContext(c, preparedFirst, baseCtx)
	require.NoError(t, attemptCtx.Err())
	svc.ReleaseCodexProfileAttempt(c, preparedFirst)
	require.ErrorIs(t, attemptCtx.Err(), context.Canceled)
	require.NoError(t, baseCtx.Err(), "releasing one attempt must not cancel the outer failover loop")

	preparedSecond, err := svc.PrepareCodexProfileAttempt(baseCtx, c, second, body)
	require.NoError(t, err)
	require.Equal(t, [][2]int64{{first.ID, second.ID}}, repo.rebinds)
	svc.ReleaseCodexProfileAttempt(c, preparedSecond)
}

func TestCodexProfileDirectWSIngressRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	account := codexProfileTestAccount(t, 291, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	account.Extra["openai_oauth_responses_websockets_v2_mode"] = OpenAIWSIngressModeCtxPool
	repo := &codexProfileGatewayAccountRepo{
		accounts: map[int64]*Account{account.ID: account},
		resolvedSlots: map[int64]*CodexResolvedDeviceSlot{
			account.ID: {
				AccountID: account.ID, SlotID: 1901, ProfileID: 1801,
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
				Architecture: CodexArchX8664, CatalogVersion: 1,
				SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
			},
		},
	}
	captureConn := &openAIWSCaptureConn{}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)
	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		cache:            &codexProfileGatewayCache{values: map[string]int64{}},
		cfg:              cfg,
		httpUpstream:     &codexProfileEchoUpstream{stream: true},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}

	serverErr := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = client.CloseNow() }()
		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, err := client.Read(readCtx)
		cancel()
		if err != nil {
			serverErr <- err
			return
		}
		ginCtx := newCodexProfileGatewayContext(t, 7, 101, firstMessage)
		sessionHash := svc.GenerateSessionHashWithFallback(ginCtx, firstMessage, "fallback")
		prepared, err := svc.PrepareCodexProfileAttempt(ginCtx.Request.Context(), ginCtx, account, firstMessage)
		if err != nil {
			serverErr <- err
			return
		}
		plan := stagedCodexIdentityAttemptPlan(ginCtx, prepared)
		alias := plan.UpstreamValue(CodexIdentitySession)
		captureConn.mu.Lock()
		captureConn.events = [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_ws_profile_1","client_metadata":{"session_id":"` + alias + `"},"output":[{"type":"message","content":[{"type":"output_text","text":"` + alias + `"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`),
			[]byte(`{"type":"response.completed","response":{"id":"resp_ws_profile_2","client_metadata":{"session_id":"` + alias + `"},"usage":{"input_tokens":1,"output_tokens":1}}}`),
		}
		captureConn.mu.Unlock()
		err = svc.ProxyResponsesWebSocketFromClient(ginCtx.Request.Context(), ginCtx, client, prepared, "test-token", firstMessage, nil)
		svc.ReleaseCodexProfileAttempt(ginCtx, prepared)
		_ = sessionHash
		serverErr <- err
	}))
	defer wsServer.Close()

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	client, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancel()
	require.NoError(t, err)
	firstMessage := []byte(`{"type":"response.create","model":"gpt-5.6-sol","client_metadata":{"session_id":"client-session","os":"windows","arch":"arm64","surface":"desktop"}}`)
	writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, client.Write(writeCtx, coderws.MessageText, firstMessage))
	cancel()
	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, response, err := client.Read(readCtx)
	cancel()
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(response, "type").String())
	require.Equal(t, "client-session", gjson.GetBytes(response, "response.client_metadata.session_id").String())
	readCtx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	_, delta, err := client.Read(readCtx)
	cancel()
	require.NoError(t, err)
	aliasText := gjson.GetBytes(delta, "delta").String()
	require.NotEmpty(t, aliasText)
	require.NotEqual(t, "client-session", aliasText, "ordinary WS output text must not be restored")
	readCtx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	_, completed, err := client.Read(readCtx)
	cancel()
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
	secondMessage := []byte(`{"type":"response.create","model":"gpt-5.6-sol","client_metadata":{"session_id":"client-session","turn_id":"client-turn-2","os":"windows","arch":"arm64","surface":"desktop"}}`)
	writeCtx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, client.Write(writeCtx, coderws.MessageText, secondMessage))
	cancel()
	readCtx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	_, secondResponse, err := client.Read(readCtx)
	cancel()
	require.NoError(t, err)
	require.Equal(t, "client-session", gjson.GetBytes(secondResponse, "response.client_metadata.session_id").String())
	_ = client.Close(coderws.StatusNormalClosure, "done")

	select {
	case err := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		if err != nil && (!errors.As(err, &closeErr) || closeErr.StatusCode() != coderws.StatusNormalClosure) {
			require.NoError(t, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for direct WS ingress")
	}
	require.Zero(t, captureDialer.DialCount(), "Profile mode must use the host HTTP bridge instead of native upstream WebSocket")
	echo := svc.httpUpstream.(*codexProfileEchoUpstream)
	originator := echo.requestHeader.Get("originator")
	architecture := gjson.GetBytes(echo.requestBody, "client_metadata.arch").String()
	require.Equal(t, "Codex Desktop", originator)
	require.Equal(t, "x86_64", architecture)
}

func TestRefreshCodexProfileTurnPlanPrefersCurrentFrameTurnIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := codexProfileTestAccount(t, 491, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	c := newCodexProfileGatewayContext(t, 7, 101, nil)
	c.Request.Header.Set("x-codex-turn-metadata", `{"turn_id":"handshake-turn","thread_id":"handshake-thread","window_id":"handshake-window","os":"linux","arch":"arm64","surface":"cli"}`)
	ctx := withCodexProfileRequest(c.Request.Context(), codexProfileRequest{
		Profile:     CodexClientProfile{OSClass: CodexOSWindows, Surface: CodexSurfaceCLI, Architecture: CodexArchARM64},
		APIKeyScope: HTTPUpstreamIsolationScopeFromContext(c.Request.Context()), ConversationHash: "conversation-turn-refresh",
	})
	c.Request = c.Request.WithContext(ctx)
	profile, err := ResolveCodexRuntimeProfile(account.CodexIdentityPolicy.Profiles[0])
	require.NoError(t, err)
	initial, err := BuildCodexIdentityAttemptPlan(CodexIdentityAttemptInput{
		Mode: account.CodexIdentityPolicy.Mode, AccountID: account.ID,
		APIKeyScope: HTTPUpstreamIsolationScopeFromContext(ctx), AccountSeed: codexRuntimeTestSeed,
		Profile: profile, Slot: CodexResolvedSlot{Index: 0, Epoch: 4},
		SessionPolicy:   account.CodexIdentityPolicy.SessionPolicy,
		ConversationKey: "conversation-turn-refresh", RequestNonce: "initial",
		Source: ExtractCodexIdentitySource(c.Request.Header, nil),
	})
	require.NoError(t, err)
	stageCodexIdentityAttemptPlan(c, initial)
	next, err := refreshCodexProfileTurnPlan(c, account, []byte(`{"client_metadata":{"session_id":"client-session","turn_id":"frame-turn-2","thread_id":"frame-thread-2","window_id":"frame-window-2"}}`))
	require.NoError(t, err)
	require.NotNil(t, next)
	turnMapping, ok := codexIdentityMappingForKind(next, CodexIdentityTurn)
	require.True(t, ok)
	require.Equal(t, "frame-turn-2", turnMapping.ClientValue)
	threadMapping, ok := codexIdentityMappingForKind(next, CodexIdentityThread)
	require.True(t, ok)
	require.Equal(t, "frame-thread-2", threadMapping.ClientValue)
	require.NotEqual(t, initial.UpstreamValue(CodexIdentityTurn), next.UpstreamValue(CodexIdentityTurn))
	require.Equal(t, initial.UpstreamValue(CodexIdentitySession), next.UpstreamValue(CodexIdentitySession))
	profileField := make(map[string]CodexProfileFieldMapping)
	for _, mapping := range next.ProfileMappings {
		profileField[mapping.Field] = mapping
	}
	require.True(t, profileField["os"].ClientPresent)
	require.Equal(t, "linux", profileField["os"].ClientValue)
	require.Equal(t, "arm64", profileField["arch"].ClientValue)
}

func codexProfileTestContext(userID, apiKeyID int64, profile CodexClientProfile, sessionHash string) context.Context {
	ctx := WithHTTPUpstreamIsolationScope(context.Background(), userID, apiKeyID)
	return withCodexProfileRequest(ctx, codexProfileRequest{
		Profile:          profile,
		APIKeyScope:      HTTPUpstreamIsolationScopeFromContext(ctx),
		ConversationHash: sessionHash,
	})
}

func codexProfileTestAccount(t *testing.T, id int64, osClass CodexOSClass, surface CodexClientSurface, arch CodexArchitecture, passthrough bool) *Account {
	t.Helper()
	policy, err := (CodexIdentityPolicySpec{
		Mode:               CodexIdentityPolicyOSProfileDevicePool,
		BindingScope:       CodexIdentityBindingAPIKeyOS,
		SessionPolicy:      CodexSessionPolicySpec{Mode: CodexSessionConversationIsolated},
		AffinityTTLSeconds: 3600,
		UnsupportedPolicy:  CodexUnsupportedProfileReject,
		Version:            1,
		Profiles: []CodexOSProfilePolicy{{
			OSClass: osClass, CanonicalSurface: surface, Architecture: arch,
			SlotCount: 1, Epoch: 4, CatalogVersion: 1,
		}},
	}).NormalizeAndValidate(PlatformOpenAI, AccountTypeOAuth)
	require.NoError(t, err)
	extra := map[string]any{codexFingerprintSeedExtraKey: codexRuntimeTestSeed}
	if passthrough {
		extra["openai_oauth_passthrough"] = true
	}
	return &Account{
		ID: id, Name: "profile-account", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, ProvisioningState: AccountProvisioningActive,
		Concurrency: 2, CodexIdentityPolicy: policy, Extra: extra,
		Credentials: map[string]any{"access_token": "test-token", "account_id": "chatgpt-account"},
	}
}

func newCodexProfileGatewayContext(t *testing.T, userID, apiKeyID int64, body []byte) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	request.Header.Set("User-Agent", "Codex Desktop/0.146.0 (Windows 11; arm64) unknown (Codex Desktop; 26.616.71553)")
	request.Header.Set("originator", "Codex Desktop")
	request = request.WithContext(WithHTTPUpstreamIsolationScope(request.Context(), userID, apiKeyID))
	c.Request = request
	groupID := int64(3)
	c.Set("api_key", &APIKey{ID: apiKeyID, GroupID: &groupID, User: &User{ID: userID}})
	return c
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
