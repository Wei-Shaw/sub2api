//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const geminiCacheHardeningPricing = `{
	"gemini-3.8-flash":{"litellm_provider":"gemini","input_cost_per_token":0.00000075,"output_cost_per_token":0.00000375,
		"cache_storage_cost_per_token_per_hour":0.0000005},
	"gemini-2.5-pro":{"litellm_provider":"gemini","input_cost_per_token":0.00000125,"output_cost_per_token":0.00001,
		"cache_storage_cost_per_token_per_hour":0.0000045}}`

func newGeminiCacheBillingGatewayForTest(t *testing.T) (*GatewayService, *openAIRecordUsageLogRepoStub) {
	t.Helper()
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
	pricing := &PricingService{}
	data, err := pricing.parsePricingData([]byte(geminiCacheHardeningPricing))
	require.NoError(t, err)
	pricing.pricingData = data
	svc.billingService = NewBillingService(svc.cfg, pricing)
	return svc, usageRepo
}

func TestGeminiCachedContentStoragePriceFollowsBillingModel(t *testing.T) {
	ctx := context.Background()
	svc, usageRepo := newGeminiCacheBillingGatewayForTest(t)
	groupID := int64(33)
	apiKey := &APIKey{ID: 821, GroupID: &groupID, User: &User{ID: 621},
		Group: &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 2, Hydrated: true}}
	tokenHours := GeminiCachedContentStorageTokenHours(1000, 24*time.Hour)

	mapped := &RecordUsageInput{
		Result: &ForwardResult{RequestID: "cc-mapped", Model: "gemini-2.5-pro", UpstreamModel: "gemini-3.8-flash",
			Usage: ClaudeUsage{InputTokens: 1000}, CacheStorageTokenHours: tokenHours},
		APIKey: apiKey, User: apiKey.User, Account: &Account{ID: 713, Platform: PlatformGemini},
	}
	require.True(t, svc.CacheStoragePriced(ctx, mapped))
	want := (1000*0.00000125 + tokenHours*0.0000045) * 2
	require.InDelta(t, want, svc.QuoteUsageCost(ctx, mapped), 1e-12, "存储与 token 同按计费模型（请求模型）定价")
	require.NoError(t, svc.RecordUsage(ctx, mapped))
	require.InDelta(t, want, usageRepo.lastLog.ActualCost, 1e-12, "报价与实际计费同口径")

	alias := &RecordUsageInput{
		Result: &ForwardResult{Model: "customer-alias", UpstreamModel: "gemini-3.8-flash", CacheStorageTokenHours: tokenHours},
		APIKey: apiKey,
	}
	require.True(t, svc.CacheStoragePriced(ctx, alias))
	require.InDelta(t, tokenHours*0.0000005*2, svc.QuoteUsageCost(ctx, alias), 1e-12, "计费模型无存储价时回退到实际转发模型")

	unpriced := &RecordUsageInput{
		Result: &ForwardResult{Model: "customer-alias", UpstreamModel: "unknown-upstream", CacheStorageTokenHours: tokenHours},
		APIKey: apiKey,
	}
	require.False(t, svc.CacheStoragePriced(ctx, unpriced))
}

func TestGeminiCachedContentStorageOnlyIsNotARequest(t *testing.T) {
	ctx := context.Background()
	svc, usageRepo := newGeminiCacheBillingGatewayForTest(t)
	groupID := int64(34)
	perRequest := 0.05
	pricing := []ChannelModelPricing{{Models: []string{"gemini-3.8-flash"}, BillingMode: BillingModePerRequest, PerRequestPrice: &perRequest}}
	svc.channelService = newTestChannelServiceForStats(t, &Channel{ID: 1, Status: StatusActive,
		AccountStatsPricingRules: []AccountStatsPricingRule{{GroupIDs: []int64{groupID}, Pricing: pricing}}}, groupID, PlatformGemini)
	svc.resolver = NewModelPricingResolver(svc.channelService, svc.billingService)
	apiKey := &APIKey{ID: 822, GroupID: &groupID, User: &User{ID: 622},
		Group: &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 1, Hydrated: true, ModelPricing: pricing}}
	tokenHours := 20000.0
	storage := tokenHours * 0.0000005

	record := func(tokens int) *UsageLog {
		require.NoError(t, svc.RecordUsage(ctx, &RecordUsageInput{
			Result: &ForwardResult{RequestID: fmt.Sprintf("cc-%d", tokens), Model: "gemini-3.8-flash",
				Usage: ClaudeUsage{InputTokens: tokens}, CacheStorageTokenHours: tokenHours},
			APIKey: apiKey, User: apiKey.User, Account: &Account{ID: 712, Platform: PlatformGemini},
		}))
		return usageRepo.lastLog
	}

	created := record(10000)
	require.InDelta(t, perRequest+storage, created.TotalCost, 1e-12, "创建是一次请求：按次价 + 存储费")
	require.NotNil(t, created.AccountStatsCost)
	require.InDelta(t, perRequest+storage, *created.AccountStatsCost, 1e-12)

	extended := record(0)
	require.InDelta(t, storage, extended.TotalCost, 1e-12, "延长有效期不算一次请求，只收存储费")
	require.Nil(t, extended.AccountStatsCost, "账号统计成本回落 total_cost，不套按次规则")
}

func TestFindGeminiCachedContentReferenceRejectsAmbiguousKeys(t *testing.T) {
	for name, body := range map[string]string{
		"duplicate top-level":       `{"cachedContent":"cachedContents/a","contents":[],"cachedContent":"cachedContents/b"}`,
		"snake top-level":           `{"cached_content":"cachedContents/a"}`,
		"snake beside camel":        `{"cachedContent":"cachedContents/a","cached_content":"cachedContents/b"}`,
		"duplicate nested":          `{"generateContentRequest":{"cachedContent":"cachedContents/a","cachedContent":"cachedContents/b"}}`,
		"snake nested":              `{"generateContentRequest":{"cached_content":"cachedContents/a"}}`,
		"snake parent with cache":   `{"generate_content_request":{"cachedContent":"cachedContents/a"}}`,
		"duplicate nested parent":   `{"generateContentRequest":{},"generateContentRequest":{"cachedContent":"cachedContents/a"}}`,
		"conflicting two locations": `{"cachedContent":"cachedContents/a","generateContentRequest":{"cachedContent":"cachedContents/b"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := FindGeminiCachedContentReference([]byte(body))
			require.Error(t, err)
		})
	}

	for name, body := range map[string]string{
		"snake parent without cache": `{"generate_content_request":{"contents":[]}}`,
		"key name inside content":    `{"contents":[{"role":"user","cached_content":"x","parts":[{"text":"cached_content"}]}]}`,
		"not an object":              `[{"cachedContent":"cachedContents/a"}]`,
	} {
		t.Run("allows "+name, func(t *testing.T) {
			ref, err := FindGeminiCachedContentReference([]byte(body))
			require.NoError(t, err)
			require.Nil(t, ref)
		})
	}

	ref, err := FindGeminiCachedContentReference([]byte(`{"cachedContent":"cachedContents/a","generateContentRequest":{"cachedContent":"cachedContents/a"}}`))
	require.NoError(t, err)
	require.Equal(t, []string{"cachedContent", "generateContentRequest.cachedContent"}, ref.Paths)
}

func TestGeminiCachedContentPayloadNormalization(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	maxTTL := 24 * time.Hour

	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash","display_name":"docs","expire_time":"2026-10-09T10:00:00Z"}`), now, maxTTL)
	require.NoError(t, err)
	require.Equal(t, "docs", req.DisplayName)
	require.Equal(t, 2*time.Hour, req.TTL)
	for _, key := range []string{"display_name", "expire_time", "displayName", "expireTime"} {
		require.NotContains(t, req.payload, key, "元数据字段不随内容发往上游")
	}

	for name, body := range map[string]string{
		"both spellings": `{"model":"gemini-3.8-flash","displayName":"a","display_name":"b"}`,
		"trailing data":  `{"model":"gemini-3.8-flash"} {"model":"x"}`,
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, err := ParseGeminiCachedContentCreateRequest([]byte(body), now, maxTTL)
			require.Error(t, err)
		})
	}

	ttl, err := ParseGeminiCachedContentPatchRequest([]byte(`{"expire_time":"2026-10-09T09:00:00Z"}`), "", now, maxTTL)
	require.NoError(t, err)
	require.Equal(t, time.Hour, ttl)
	_, err = ParseGeminiCachedContentPatchRequest([]byte(`{"display_name":"x"}`), "", now, maxTTL)
	require.ErrorContains(t, err, "immutable")
}

func TestGeminiCachedContentTTLRoundsUpToWholeSeconds(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash","ttl":"1e-10s"}`), now, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, time.Second, req.TTL, "极小的显式有效期按 1 秒处理，不回落到默认 1 小时")
	require.True(t, req.ttlExplicit)

	ttl, err := ParseGeminiCachedContentPatchRequest([]byte(`{"ttl":"1e-10s"}`), "", now, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, time.Second, ttl)

	req, err = ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash","expireTime":"2026-10-09T08:00:10.2Z"}`), now, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, 11*time.Second, req.TTL, "expireTime 折算的有效期与发往上游的整秒 ttl 一致")
}

func TestGeminiCachedContentCreateKeepsNumbersVerbatim(t *testing.T) {
	svc, httpStub := newGeminiCachedContentServiceForTest(t, http.StatusOK,
		`{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":2000}}`)
	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"models/gemini-3.8-flash","contents":[{"role":"user","parts":[
		{"functionResponse":{"name":"lookup","response":{"order_id":1790000000000000001,"ratio":0.10}}}]}]}`), time.Now(), 24*time.Hour)
	require.NoError(t, err)
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}}
	_, err = svc.CreateUpstream(context.Background(), account, req, "gemini-3.8-flash", "pub1", time.Hour)
	require.NoError(t, err)
	raw, err := io.ReadAll(httpStub.lastReq.Body)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"order_id":1790000000000000001`)
	require.Contains(t, string(raw), `"ratio":0.10`)
}

func TestGeminiCachedContentCreateUpstreamUnusableResponse(t *testing.T) {
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}}
	for name, tc := range map[string]struct {
		body     string
		wantName string
	}{
		"missing token count": {`{"name":"cachedContents/up9","expireTime":"2099-01-01T00:00:00Z"}`, "cachedContents/up9"},
		"zero token count":    {`{"name":"cachedContents/up9","usageMetadata":{"totalTokenCount":0}}`, "cachedContents/up9"},
		"unexpected name":     {`{"name":"cachedContents/a/../b","usageMetadata":{"totalTokenCount":5}}`, ""},
		"not json":            {`<html>`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			svc, _ := newGeminiCachedContentServiceForTest(t, http.StatusOK, tc.body)
			req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash"}`), time.Now(), 24*time.Hour)
			require.NoError(t, err)
			_, err = svc.CreateUpstream(context.Background(), account, req, "gemini-3.8-flash", "pub1", time.Hour)
			var malformed *GeminiCachedContentMalformedResponseError
			require.ErrorAs(t, err, &malformed)
			require.Equal(t, tc.wantName, malformed.Name)
		})
	}
}

type geminiCachedContentCtxProbeUpstream struct {
	geminiCompatHTTPUpstreamStub
	ctxErrAtSend error
}

func (s *geminiCachedContentCtxProbeUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	s.ctxErrAtSend = req.Context().Err()
	return s.geminiCompatHTTPUpstreamStub.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestGeminiCachedContentUpstreamCallIgnoresClientCancel(t *testing.T) {
	up := &geminiCachedContentCtxProbeUpstream{geminiCompatHTTPUpstreamStub: geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":2000}}`)),
	}}}
	svc := NewGeminiCachedContentService(newGeminiCachedContentRepoStub(),
		&GeminiMessagesCompatService{httpUpstream: up, cfg: &config.Config{}}, &config.Config{})
	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash"}`), time.Now(), 24*time.Hour)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}}
	_, err = svc.CreateUpstream(ctx, account, req, "gemini-3.8-flash", "pub1", time.Hour)
	require.NoError(t, err)
	require.NoError(t, up.ctxErrAtSend, "客户端断开不取消发往上游的创建")
}

func TestClampGeminiCachedContentExpire(t *testing.T) {
	limit := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	require.Equal(t, limit, ClampGeminiCachedContentExpire(time.Time{}, limit))
	require.Equal(t, limit, ClampGeminiCachedContentExpire(limit.Add(100*time.Hour), limit), "上游回报的到期时间不超过请求的有效期")
	earlier := limit.Add(-time.Second)
	require.Equal(t, earlier, ClampGeminiCachedContentExpire(earlier, limit))
}

type geminiBoundCacheRateLimitRepo struct {
	errorPolicyRepoStub
	rateLimitedCalls int
}

func (r *geminiBoundCacheRateLimitRepo) SetRateLimited(context.Context, int64, time.Time) error {
	r.rateLimitedCalls++
	return nil
}

func TestGeminiForwardNative_BoundCacheUpstreamGatewayErrorsSkipAccountPenalty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := fmt.Sprintf(`{"error":{"code":%d,"message":"The account holding this cached content is temporarily unavailable","status":"UNAVAILABLE"}}`, status)
			svc, httpStub, _ := newGeminiCachedContentTestService(status, body)
			repo := &geminiBoundCacheRateLimitRepo{}
			svc.accountRepo = repo
			svc.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			account := &Account{ID: 711, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"api_key": "k", "base_url": "https://gw.example.com", "explicit_cache_upstream": true,
			}}
			c, _ := newGeminiNativeTestContext(t)

			_, err := svc.ForwardNative(WithGeminiCachedContentBound(context.Background()), c, account,
				"gemini-3.8-flash", "generateContent", false, []byte(geminiCachedContentRequestBody))

			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr), "交由绑定请求的耗尽处理透传")
			require.Equal(t, status, failoverErr.StatusCode)
			require.JSONEq(t, body, string(failoverErr.ResponseBody))
			require.True(t, failoverErr.BoundUpstreamPassthrough, "标记为上游网关账号的响应，handler 原样回写")
			require.False(t, failoverErr.RetryableOnSameAccount)
			require.Equal(t, 1, httpStub.calls, "不在同一账号上退避重试")
			require.Zero(t, repo.rateLimitedCalls, "不限流整个网关账号")
			require.Zero(t, repo.tempCalls)
			require.Zero(t, repo.setErrCalls)
		})
	}
}

func TestGeminiForwardNative_BoundCacheUpstreamGatewayErrorsRetryablePerPoolModeCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		status    int
		retryable bool
	}{
		{status: http.StatusTooManyRequests, retryable: true},
		{status: http.StatusServiceUnavailable, retryable: false},
	} {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			body := fmt.Sprintf(`{"error":{"code":%d,"message":"Resource exhausted. Please try again later.","status":"RESOURCE_EXHAUSTED"}}`, tt.status)
			svc, httpStub, _ := newGeminiCachedContentTestService(tt.status, body)
			repo := &geminiBoundCacheRateLimitRepo{}
			svc.accountRepo = repo
			svc.rateLimitService = NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			account := &Account{ID: 712, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"api_key": "k", "base_url": "https://gw.example.com", "explicit_cache_upstream": true,
				"pool_mode": true, "pool_mode_retry_count": float64(2), "pool_mode_retry_status_codes": []any{float64(429)},
			}}
			c, _ := newGeminiNativeTestContext(t)

			_, err := svc.ForwardNative(WithGeminiCachedContentBound(context.Background()), c, account,
				"gemini-3.8-flash", "generateContent", false, []byte(geminiCachedContentRequestBody))

			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr))
			require.True(t, failoverErr.BoundUpstreamPassthrough, "重试用尽后仍由 handler 原样回写")
			require.Equal(t, tt.retryable, failoverErr.RetryableOnSameAccount, "按池模式重试状态码决定同账号重试")
			require.Equal(t, tt.retryable, failoverErr.RequestScopedTransient, "重试用尽不临时封禁本账号")
			require.Equal(t, 1, httpStub.calls, "同账号重试由 handler 执行")
			require.Zero(t, repo.rateLimitedCalls)
			require.Zero(t, repo.tempCalls)
			require.Zero(t, repo.setErrCalls)

			raw, ok := c.Get(OpsUpstreamErrorsKey)
			if !tt.retryable {
				require.False(t, ok, "不可重试时不登记上游事件，由耗尽处理写 ops")
				return
			}
			require.True(t, ok)
			events, ok := raw.([]*OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.Len(t, events, 1, "每次尝试登记一个上游事件，重试成功时 ops 记为恢复")
			require.Equal(t, tt.status, events[0].UpstreamStatusCode)
			require.Equal(t, "retry", events[0].Kind, "同账号重试不计入换号次数")
		})
	}
}

type fixedChargeCacheStub struct {
	billingCacheWorkerStub
	balance float64
	sub     *SubscriptionCacheData
	rate    *APIKeyRateLimitCacheData
	// reserved 按维度（user:{id} / scope 名）记录在途预留。
	reserved map[string]map[string]float64
}

func (s *fixedChargeCacheStub) GetUserBalance(context.Context, int64) (float64, error) {
	return s.balance, nil
}

func (s *fixedChargeCacheStub) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	if s.sub == nil {
		return nil, errors.New("cache miss")
	}
	return s.sub, nil
}

func (s *fixedChargeCacheStub) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	if s.rate == nil {
		return nil, errors.New("cache miss")
	}
	return s.rate, nil
}

func (s *fixedChargeCacheStub) reserve(key, requestID string, amount float64) float64 {
	if s.reserved == nil {
		s.reserved = map[string]map[string]float64{}
	}
	if s.reserved[key] == nil {
		s.reserved[key] = map[string]float64{}
	}
	sum := 0.0
	for _, v := range s.reserved[key] {
		sum += v
	}
	s.reserved[key][requestID] = amount
	return sum
}

func (s *fixedChargeCacheStub) ReserveInflightBalance(_ context.Context, userID int64, requestID string, amount, balance float64, _ time.Duration) (bool, float64, error) {
	key := fmt.Sprintf("user:%d", userID)
	others := s.reserve(key, requestID, amount)
	if len(s.reserved[key]) > 1 && balance-others < amount {
		delete(s.reserved[key], requestID)
		return false, others, nil
	}
	return true, others, nil
}

func (s *fixedChargeCacheStub) ReleaseInflightBalance(_ context.Context, userID int64, requestID string) error {
	delete(s.reserved[fmt.Sprintf("user:%d", userID)], requestID)
	return nil
}

func (s *fixedChargeCacheStub) ReserveInflightScope(_ context.Context, scope, requestID string, amount float64, _ time.Duration) (float64, error) {
	return s.reserve(scope, requestID, amount), nil
}

func (s *fixedChargeCacheStub) ReleaseInflightScope(_ context.Context, scope, requestID string) error {
	delete(s.reserved[scope], requestID)
	return nil
}

type fixedChargeAPIKeyRepoStub struct {
	APIKeyRepository
	data *APIKeyRateLimitData
}

func (r *fixedChargeAPIKeyRepoStub) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	return r.data, nil
}

type fixedChargeSubscriptionRepoStub struct {
	UserSubscriptionRepository
	sub *UserSubscription
}

func (r *fixedChargeSubscriptionRepoStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return r.sub, nil
}

func TestReserveFixedCharge(t *testing.T) {
	ctx := context.Background()
	user := &User{ID: 1}
	newSvc := func(cache *fixedChargeCacheStub) *BillingCacheService {
		svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
		t.Cleanup(svc.Stop)
		return svc
	}
	reservingSvc := func(cache *fixedChargeCacheStub, subRepo UserSubscriptionRepository, apiKeyRepo APIKeyRepository) *BillingCacheService {
		cfg := &config.Config{}
		cfg.Billing.InflightReservation.Enabled = true
		svc := NewBillingCacheService(cache, nil, subRepo, apiKeyRepo, nil, nil, cfg, nil)
		t.Cleanup(svc.Stop)
		return svc
	}
	reserve := func(svc *BillingCacheService, apiKey *APIKey, group *Group, sub *UserSubscription, amount float64) error {
		res, err := svc.ReserveFixedCharge(ctx, user, apiKey, group, sub, amount)
		res.Release()
		return err
	}

	t.Run("balance must cover the amount", func(t *testing.T) {
		svc := newSvc(&fixedChargeCacheStub{balance: 10})
		require.NoError(t, reserve(svc, nil, nil, nil, 10))
		require.ErrorIs(t, reserve(svc, nil, nil, nil, 10.01), ErrInsufficientBalance)
		require.NoError(t, reserve(svc, nil, nil, nil, 0))
	})

	t.Run("subscription windows must cover the amount", func(t *testing.T) {
		daily := 5.0
		group := &Group{ID: 2, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily}
		svc := newSvc(&fixedChargeCacheStub{sub: &SubscriptionCacheData{
			Status: SubscriptionStatusActive, ExpiresAt: time.Now().Add(time.Hour), DailyUsage: 3,
		}})
		sub := &UserSubscription{ID: 1}
		require.NoError(t, reserve(svc, nil, group, sub, 2))
		require.ErrorIs(t, reserve(svc, nil, group, sub, 2.5), ErrDailyLimitExceeded)
	})

	t.Run("api key quota and rate windows include the amount", func(t *testing.T) {
		svc := newSvc(&fixedChargeCacheStub{balance: 100, rate: &APIKeyRateLimitCacheData{
			Usage1d: 8, Window1d: time.Now().Unix(),
		}})
		quotaKey := &APIKey{ID: 5, Quota: 10, QuotaUsed: 9}
		require.NoError(t, reserve(svc, quotaKey, nil, nil, 1))
		require.ErrorIs(t, reserve(svc, quotaKey, nil, nil, 1.5), ErrAPIKeyQuotaExhausted)

		windowKey := &APIKey{ID: 6, RateLimit1d: 10}
		require.NoError(t, reserve(svc, windowKey, nil, nil, 2))
		require.ErrorIs(t, reserve(svc, windowKey, nil, nil, 2.5), ErrAPIKeyRateLimit1dExceeded)
	})

	t.Run("usage is read from the database instead of request snapshots", func(t *testing.T) {
		now := time.Now()
		apiKeyRepo := &fixedChargeAPIKeyRepoStub{data: &APIKeyRateLimitData{QuotaUsed: 9.5, Usage1d: 8, Window1dStart: &now}}
		svc := reservingSvc(&fixedChargeCacheStub{balance: 100}, nil, apiKeyRepo)
		require.ErrorIs(t, reserve(svc, &APIKey{ID: 5, Quota: 10}, nil, nil, 1), ErrAPIKeyQuotaExhausted, "请求上下文中的 QuotaUsed 可能滞后")
		require.ErrorIs(t, reserve(svc, &APIKey{ID: 6, RateLimit1d: 10}, nil, nil, 2.5), ErrAPIKeyRateLimit1dExceeded)

		daily := 5.0
		group := &Group{ID: 2, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily}
		subRepo := &fixedChargeSubscriptionRepoStub{sub: &UserSubscription{Status: SubscriptionStatusActive, DailyUsageUSD: 4}}
		svc = reservingSvc(&fixedChargeCacheStub{sub: &SubscriptionCacheData{Status: SubscriptionStatusActive}}, subRepo, nil)
		require.ErrorIs(t, reserve(svc, nil, group, &UserSubscription{ID: 1}, 2), ErrDailyLimitExceeded, "订阅缓存用量可能滞后")
	})

	t.Run("unsettled charges hold subscription and api key headroom", func(t *testing.T) {
		daily := 110.0
		group := &Group{ID: 2, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily}
		sub := &UserSubscription{ID: 1}
		cache := &fixedChargeCacheStub{balance: 1000, sub: &SubscriptionCacheData{Status: SubscriptionStatusActive}}
		svc := reservingSvc(cache, nil, nil)
		first, err := svc.ReserveFixedCharge(ctx, user, nil, group, sub, 103.5)
		require.NoError(t, err)
		_, err = svc.ReserveFixedCharge(ctx, user, nil, group, sub, 103.5)
		require.ErrorIs(t, err, ErrDailyLimitExceeded)
		first.Release()
		second, err := svc.ReserveFixedCharge(ctx, user, nil, group, sub, 103.5)
		require.NoError(t, err)
		second.Release()

		apiKey := &APIKey{ID: 5, Quota: 110}
		first, err = svc.ReserveFixedCharge(ctx, user, apiKey, nil, nil, 103.5)
		require.NoError(t, err)
		_, err = svc.ReserveFixedCharge(ctx, user, apiKey, nil, nil, 103.5)
		require.ErrorIs(t, err, ErrAPIKeyQuotaExhausted)
		first.Release()
		for key, reservations := range cache.reserved {
			require.Empty(t, reservations, "拒绝与释放都不留下预留：%s", key)
		}
	})

	t.Run("simple mode skips the check", func(t *testing.T) {
		cfg := &config.Config{RunMode: config.RunModeSimple}
		svc := NewBillingCacheService(&fixedChargeCacheStub{}, nil, nil, nil, nil, nil, cfg, nil)
		t.Cleanup(svc.Stop)
		require.NoError(t, reserve(svc, nil, nil, nil, 1000))
	})
}
