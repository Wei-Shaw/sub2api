package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func newOfficialCodexTransparentPassthroughContext() *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.145.0")
	return c
}

func newOfficialCodexTransparentPassthroughAccount() *service.Account {
	return &service.Account{
		ID:          70,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"},
		Extra:       map[string]any{"openai_passthrough": true},
	}
}

func TestValidateOpenAIResponsesAttemptCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &OpenAIGatewayHandler{}
	invalidContinuation := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"function_call_output","call_id":"call_1","output":"{}"}]}`)

	t.Run("official transparent passthrough defers continuation validation", func(t *testing.T) {
		c := newOfficialCodexTransparentPassthroughContext()
		ok := h.validateOpenAIResponsesAttemptCompatibility(
			c,
			newOfficialCodexTransparentPassthroughAccount(),
			invalidContinuation,
			"msg_not_a_response_id",
			service.OpenAIPreviousResponseIDKindMessageID,
			zap.NewNop(),
		)
		require.True(t, ok)
		require.False(t, c.Writer.Written())
	})

	t.Run("non passthrough keeps previous response validation", func(t *testing.T) {
		c := newOfficialCodexTransparentPassthroughContext()
		account := newOfficialCodexTransparentPassthroughAccount()
		account.Extra = nil
		ok := h.validateOpenAIResponsesAttemptCompatibility(
			c,
			account,
			invalidContinuation,
			"msg_not_a_response_id",
			service.OpenAIPreviousResponseIDKindMessageID,
			zap.NewNop(),
		)
		require.False(t, ok)
		require.Equal(t, http.StatusBadRequest, c.Writer.Status())
	})
}

type openAITransparentMixedAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r *openAITransparentMixedAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	accounts := make([]service.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform && account.IsSchedulable() {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func (r *openAITransparentMixedAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

func (r *openAITransparentMixedAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

func (r *openAITransparentMixedAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for _, account := range r.accounts {
		if account.ID == id {
			copy := account
			return &copy, nil
		}
	}
	return nil, nil
}

type openAITransparentMixedAccountUpstream struct {
	service.HTTPUpstream
	accountIDs   []int64
	bodies       [][]byte
	responseBody string
	contentType  string
}

func (u *openAITransparentMixedAccountUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.accountIDs = append(u.accountIDs, accountID)
	requestBody, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.bodies = append(u.bodies, requestBody)
	responseID := "resp_api_key_first"
	switch accountID {
	case 70:
		responseID = "resp_oauth_issuer"
	case 71:
		responseID = "resp_oauth_other"
	}
	body := fmt.Sprintf(`{"id":%q,"object":"response","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`, responseID)
	if u.responseBody != "" {
		body = u.responseBody
	}
	contentType := u.contentType
	if contentType == "" {
		contentType = "application/json"
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func TestOpenAIResponsesContinuationPreservesIssuerAffinityAndReleasesAcquiredSlot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(4204)
	issuerAccount := *newOfficialCodexTransparentPassthroughAccount()
	issuerAccount.Name = "oauth-issuer"
	issuerAccount.Status = service.StatusActive
	issuerAccount.Schedulable = true
	issuerAccount.Priority = 1
	issuerAccount.Concurrency = 1
	issuerAccount.Credentials["base_url"] = "https://api.example.test"
	otherAccount := issuerAccount
	otherAccount.ID = 71
	otherAccount.Name = "oauth-other"
	otherAccount.Priority = 2
	otherAccount.Credentials = map[string]any{"access_token": "other-token", "base_url": "https://api.example.test"}
	otherAccount.Extra = map[string]any{"openai_passthrough": true}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.MaxAccountSwitches = 2
	repo := &openAITransparentMixedAccountRepo{accounts: []service.Account{issuerAccount, otherAccount}}
	upstream := &openAITransparentMixedAccountUpstream{}
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	concurrency := service.NewConcurrencyService(cache)
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	gateway := service.NewOpenAIGatewayService(
		repo, nil, nil, nil, nil, nil, nil, cfg, nil, concurrency,
		service.NewBillingService(cfg, nil), nil, billingCache, upstream,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	h := NewOpenAIGatewayHandler(
		gateway,
		concurrency,
		billingCache,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil, cfg,
	)

	request := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.Header.Set("User-Agent", "codex_cli_rs/0.145.0")
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
			ID: 1804, GroupID: &groupID,
			User:  &service.User{ID: 1704, Status: service.StatusActive},
			Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
		})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1704, Concurrency: 0})
		h.Responses(c)
		return recorder
	}

	first := request(`{"model":"gpt-5.6-sol","input":"hello","stream":false}`)
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, "resp_oauth_issuer", gjson.GetBytes(first.Body.Bytes(), "id").String())
	repo.accounts[0].Priority = 2
	repo.accounts[1].Priority = 1

	probeSelection, probeDecision, err := gateway.SelectOfficialCodexTransparentHTTPAccountByPreviousResponseID(
		context.Background(), &groupID, "resp_oauth_issuer", "gpt-5.6-sol", nil,
		service.OpenAIEndpointCapabilityResponses, false,
	)
	require.NoError(t, err)
	require.NotNil(t, probeSelection)
	require.True(t, probeDecision.StickyPreviousHit)
	require.Equal(t, issuerAccount.ID, probeDecision.SelectedAccountID)
	require.Equal(t, issuerAccount.ID, probeSelection.Account.ID)
	if probeSelection.ReleaseFunc != nil {
		probeSelection.ReleaseFunc()
	}

	continuation := `{"model":"gpt-5.6-sol","previous_response_id":"resp_oauth_issuer","input":[{"type":"function_call_output","call_id":"call_1","output":"{}"}],"stream":false}`
	releasesBeforeContinuation := atomic.LoadInt32(&cache.releaseAccountCalled)
	second := request(continuation)

	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, []int64{70, 70}, upstream.accountIDs)
	require.Equal(t, "resp_oauth_issuer", gjson.GetBytes(upstream.bodies[1], "previous_response_id").String())
	require.Equal(t, releasesBeforeContinuation+1, atomic.LoadInt32(&cache.releaseAccountCalled), "selected continuation account slot must be released exactly once")

	miss := request(`{"model":"gpt-5.6-sol","previous_response_id":"resp_unknown","input":"continue","stream":false}`)
	require.Equal(t, http.StatusBadRequest, miss.Code)
	require.Equal(t, "previous_response_id is not bound to an available transparent OAuth account", gjson.GetBytes(miss.Body.Bytes(), "error.message").String())
	require.Equal(t, []int64{70, 70}, upstream.accountIDs, "affinity miss must not migrate to the newly preferred account")
}

func TestOpenAIResponsesMandatoryTransformsReachUpstreamThroughHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(4205)
	group := &service.Group{
		ID:                 groupID,
		Platform:           service.PlatformOpenAI,
		Status:             service.StatusActive,
		MaxReasoningEffort: "medium",
	}
	account := *newOfficialCodexTransparentPassthroughAccount()
	account.Name = "mapped-oauth"
	account.Status = service.StatusActive
	account.Schedulable = true
	account.Priority = 1
	account.Credentials["base_url"] = "https://api.example.test"
	repo := &openAITransparentMixedAccountRepo{accounts: []service.Account{account}}
	upstream := &openAITransparentMixedAccountUpstream{}
	channelService := service.NewChannelService(
		&openAIWSUsageHandlerChannelRepoStub{
			channels: []service.Channel{{
				ID:       8805,
				Name:     "transparent-handler-mapping",
				Status:   service.StatusActive,
				GroupIDs: []int64{groupID},
				ModelMapping: map[string]map[string]string{
					service.PlatformOpenAI: {"alias": "gpt-5.6-sol"},
				},
			}},
			groupPlatforms: map[int64]string{groupID: service.PlatformOpenAI},
		},
		nil, nil, nil,
	)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	gateway := service.NewOpenAIGatewayService(
		repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCache, upstream,
		&service.DeferredService{}, nil, nil, nil, channelService, nil, nil, nil,
	)
	h := NewOpenAIGatewayHandler(
		gateway, service.NewConcurrencyService(nil), billingCache,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil, cfg,
	)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"alias","reasoning":{"effort":"high"},"input":"test","stream":false}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.145.0")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 1805, GroupID: &groupID,
		User:  &service.User{ID: 1705, Status: service.StatusActive},
		Group: group,
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1705, Concurrency: 0})

	h.Responses(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, upstream.bodies, 1)
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "medium", gjson.GetBytes(upstream.bodies[0], "reasoning.effort").String())
}

func TestOpenAIResponsesTransparentFailureSanitizesClientAndRecordsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(4206)
	account := *newOfficialCodexTransparentPassthroughAccount()
	account.Name = "failed-oauth"
	account.Status = service.StatusActive
	account.Schedulable = true
	account.Priority = 1
	account.Credentials["base_url"] = "https://api.example.test"
	repo := &openAITransparentMixedAccountRepo{accounts: []service.Account{account}}
	upstream := &openAITransparentMixedAccountUpstream{
		contentType: "text/event-stream",
		responseBody: "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"error\":{\"message\":\"sensitive upstream failure\"},\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n" +
			"data: [DONE]\n\n",
	}
	usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 1)}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	gateway := service.NewOpenAIGatewayService(
		repo, usageRepo, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCache, upstream,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	h := NewOpenAIGatewayHandler(
		gateway, service.NewConcurrencyService(nil), billingCache,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil, cfg,
	)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.6-sol","input":"test","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.145.0")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 1806, GroupID: &groupID,
		User:  &service.User{ID: 1706, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1706, Concurrency: 0})

	h.Responses(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"response.failed"`))
	require.Contains(t, recorder.Body.String(), "Upstream request failed")
	require.NotContains(t, recorder.Body.String(), "sensitive upstream failure")
	require.NotContains(t, recorder.Body.String(), "[DONE]")
	select {
	case usageLog := <-usageRepo.created:
		require.Equal(t, 2, usageLog.InputTokens)
		require.Equal(t, 1, usageLog.OutputTokens)
		require.True(t, usageLog.Stream)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for failed transparent stream usage record")
	}
}

func TestOpenAIResponsesTransparentFailureWithEmptyUsageDoesNotRecordZeroUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(4207)
	account := *newOfficialCodexTransparentPassthroughAccount()
	account.Name = "failed-without-usage-oauth"
	account.Status = service.StatusActive
	account.Schedulable = true
	account.Priority = 1
	account.Credentials["base_url"] = "https://api.example.test"
	repo := &openAITransparentMixedAccountRepo{accounts: []service.Account{account}}
	upstream := &openAITransparentMixedAccountUpstream{
		contentType: "text/event-stream",
		responseBody: "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed_empty_usage\",\"error\":{\"message\":\"sensitive upstream failure\"},\"usage\":{}}}\n\n" +
			"data: [DONE]\n\n",
	}
	usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 1)}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	gateway := service.NewOpenAIGatewayService(
		repo, usageRepo, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCache, upstream,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	h := NewOpenAIGatewayHandler(
		gateway, service.NewConcurrencyService(nil), billingCache,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil, cfg,
	)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.6-sol","input":"test","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.145.0")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 1807, GroupID: &groupID,
		User:  &service.User{ID: 1707, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1707, Concurrency: 0})

	h.Responses(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"response.failed"`))
	require.Contains(t, recorder.Body.String(), "Upstream request failed")
	require.NotContains(t, recorder.Body.String(), "sensitive upstream failure")
	require.NotContains(t, recorder.Body.String(), "[DONE]")
	select {
	case usageLog := <-usageRepo.created:
		t.Fatalf("unexpected zero-usage record: %+v", usageLog)
	case <-time.After(200 * time.Millisecond):
	}
}
