package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ── stub helpers ─────────────────────────────────────────────────────────────

// stubQuotaAccountRepo 是多账号 AccountRepository stub，仅实现 GetByID。
type stubQuotaAccountRepo struct {
	AccountRepository
	accounts map[int64]*Account
}

func (r *stubQuotaAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	acc, ok := r.accounts[id]
	if !ok {
		return nil, fmt.Errorf("account %d not found", id)
	}
	return acc, nil
}

// stubQuotaTokenCache 实现 OpenAITokenCache，返回预设静态 token。
type stubQuotaTokenCache struct {
	tokens map[string]string
}

func (c *stubQuotaTokenCache) GetAccessToken(_ context.Context, key string) (string, error) {
	if t, ok := c.tokens[key]; ok {
		return t, nil
	}
	return "", errors.New("token not found")
}

func (c *stubQuotaTokenCache) SetAccessToken(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
}

func (c *stubQuotaTokenCache) DeleteAccessToken(_ context.Context, _ string) error { return nil }

func (c *stubQuotaTokenCache) AcquireRefreshLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (c *stubQuotaTokenCache) ReleaseRefreshLock(_ context.Context, _ string) error { return nil }

// newQuotaRedirectingFactory 返回 PrivacyClientFactory，将请求重定向到 httptest.Server。
func newQuotaRedirectingFactory(srv *httptest.Server) PrivacyClientFactory {
	targetURL, _ := url.Parse(srv.URL)
	return func(_ string) (*req.Client, error) {
		c := req.C().WrapRoundTripFunc(func(rt req.RoundTripper) req.RoundTripFunc {
			return func(r *req.Request) (*req.Response, error) {
				r.URL.Scheme = targetURL.Scheme
				r.URL.Host = targetURL.Host
				return rt.RoundTrip(r)
			}
		})
		return c, nil
	}
}

// ── Part A: buildCodexSparkWindowExtraUpdates ─────────────────────────────────

// TestBuildCodexSparkWindowExtraUpdates_ContainsCodexKeys 验证:
//   - 产出包含 codex_5h_used_percent / codex_7d_used_percent
//   - 不含任何 codex_spark_ 前缀的 key（Method Z 前缀已禁止）
//   - 数值正确映射（primary 较短→5h，secondary 较长→7d）
func TestBuildCodexSparkWindowExtraUpdates_ContainsCodexKeys(t *testing.T) {
	now := time.Now().UTC()
	usage := &OpenAIQuotaUsage{
		AdditionalRateLimits: []OpenAIAdditionalRateLimit{
			{
				MeteredFeature: "codex_bengalfox",
				RateLimit: &OpenAIRateLimit{
					PrimaryWindow: &OpenAIRateLimitWindow{
						UsedPercent:        0.42,
						LimitWindowSeconds: 18000, // 300 min = 5 h
						ResetAfterSeconds:  3600,
					},
					SecondaryWindow: &OpenAIRateLimitWindow{
						UsedPercent:        0.15,
						LimitWindowSeconds: 604800, // 7 d
						ResetAfterSeconds:  86400,
					},
				},
			},
		},
	}

	updates := buildCodexSparkWindowExtraUpdates(usage, now)
	require.NotNil(t, updates, "expected non-nil updates for valid codex_bengalfox entry")

	// 必须含有 codex_5h_* 和 codex_7d_* 键
	require.Contains(t, updates, "codex_5h_used_percent")
	require.Contains(t, updates, "codex_7d_used_percent")

	// 任何键不得含有 codex_spark_ 前缀（Method Z 已禁止）
	for k := range updates {
		require.False(t, strings.Contains(k, "codex_spark_"),
			"unexpected Method-Z prefix in key: %s", k)
	}

	// 数值验证（primary=5h, secondary=7d）
	require.InDelta(t, 0.42, updates["codex_5h_used_percent"], 1e-9)
	require.InDelta(t, 0.15, updates["codex_7d_used_percent"], 1e-9)
}

// TestBuildCodexSparkWindowExtraUpdates_NilUsage 验证 nil usage 返回 nil。
func TestBuildCodexSparkWindowExtraUpdates_NilUsage(t *testing.T) {
	require.Nil(t, buildCodexSparkWindowExtraUpdates(nil, time.Now()))
}

// TestBuildCodexSparkWindowExtraUpdates_NoBengalfox 验证无 codex_bengalfox 条目时返回 nil。
func TestBuildCodexSparkWindowExtraUpdates_NoBengalfox(t *testing.T) {
	usage := &OpenAIQuotaUsage{
		AdditionalRateLimits: []OpenAIAdditionalRateLimit{
			{MeteredFeature: "other_feature", RateLimit: &OpenAIRateLimit{}},
		},
	}
	require.Nil(t, buildCodexSparkWindowExtraUpdates(usage, time.Now()))
}

// ── Part C: ResetCredit 影子拒绝 ───────────────────────────────────────────

// TestResetCreditShadowRejected 验证:
//   - ResetCredit(ctx, shadowID) 返回 ErrSparkShadowResetNotSupported
//   - 不触达上游（privacyClientFactory 为 nil，若调用则 panic）
func TestResetCreditShadowRejected(t *testing.T) {
	pid := int64(100)
	shadow := &Account{
		ID:              200,
		ParentAccountID: &pid,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		QuotaDimension:  QuotaDimensionSpark,
	}
	repo := &stubQuotaAccountRepo{
		accounts: map[int64]*Account{200: shadow},
	}
	// privacyClientFactory 故意为 nil —— 若流程误到上游则 prepareUpstreamCall 会先在
	// 配置检查处报错，但我们在此之前就应该拦截并返回 ErrSparkShadowResetNotSupported。
	svc := &OpenAIQuotaService{accountRepo: repo}

	_, err := svc.ResetCredit(context.Background(), 200)
	require.ErrorIs(t, err, ErrSparkShadowResetNotSupported,
		"shadow ResetCredit should return ErrSparkShadowResetNotSupported, got: %v", err)
	// 外审 F6:必须是结构化 409(而非裸 error→500)。
	require.Equal(t, http.StatusConflict, infraerrors.Code(err),
		"shadow ResetCredit 应映射为 409 Conflict 而非 500")
}

// ── Part B: prepareUpstreamCall 影子 resolve ──────────────────────────────

// TestPrepareUpstreamCallShadowResolve 验证影子账号（200）QueryUsage 时:
//   - 不因 chatgpt_account_id 为空而报错
//   - 使用母账号（100）的 chatgpt_account_id("org-parent123")
//
// 测试策略: 直接调用包内可见的 prepareUpstreamCall，注入 stubTokenCache（命中路径）
// 和 stubQuotaAccountRepo（同时持有影子+母账号），绕开 /wham/usage HTTP 往返。
// 这比 httptest 端到端 mock 更轻量且对实现细节的耦合更低。
func TestPrepareUpstreamCallShadowResolve(t *testing.T) {
	ctx := context.Background()
	pid := int64(100)

	// 影子账号：无 chatgpt_account_id credentials
	shadow := &Account{
		ID:              200,
		ParentAccountID: &pid,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Status:          StatusActive,
		QuotaDimension:  QuotaDimensionSpark,
	}
	// 母账号：有完整 credentials
	parent := &Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "org-parent123",
		},
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{200: shadow, 100: parent}}

	// stubTokenCache 为母账号 cache key 提供 fake token（走缓存命中路径，无需真实刷新）
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(parent): "fake-access-token",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)

	// privacyClientFactory 可以是任意合法工厂；prepareUpstreamCall 在返回前不调用它
	svc := NewOpenAIQuotaService(repo, nil, tokenProvider, func(_ string) (*req.Client, error) {
		return req.C(), nil
	})

	_, chatGPTAccountID, _, _, err := svc.prepareUpstreamCall(ctx, 200)
	require.NoError(t, err, "shadow resolve should succeed; got error: %v", err)
	require.Equal(t, "org-parent123", chatGPTAccountID,
		"prepareUpstreamCall should use parent's chatgpt_account_id after shadow resolve")
}

func TestParseOpenAIRateLimitResetCreditDetails_CompatibleContainers(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "credits",
			body: `{"credits":[{"id":"secret-id","expires_at":"2026-07-03T04:05:06Z"}]}`,
			want: []string{"2026-07-03T04:05:06Z"},
		},
		{
			name: "rate limit reset credits",
			body: `{"rate_limit_reset_credits":[{"expiresAt":"2026-07-04T04:05:06Z"}]}`,
			want: []string{"2026-07-04T04:05:06Z"},
		},
		{
			name: "items",
			body: `{"items":[{"expires_at":"2026-07-05T04:05:06Z"}]}`,
			want: []string{"2026-07-05T04:05:06Z"},
		},
		{
			name: "data",
			body: `{"data":[{"expires_at":"2026-07-06T04:05:06Z"}]}`,
			want: []string{"2026-07-06T04:05:06Z"},
		},
		{
			name: "array",
			body: `[{"expires_at":"2026-07-07T04:05:06Z"}]`,
			want: []string{"2026-07-07T04:05:06Z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOpenAIRateLimitResetCreditDetails([]byte(tt.body))
			require.NoError(t, err)
			require.Len(t, got, len(tt.want))
			for i := range tt.want {
				require.Equal(t, tt.want[i], got[i].ExpiresAt)
			}
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "secret-id")
		})
	}
}

func TestQueryUsageIncludesResetCreditExpirations_EndToEnd(t *testing.T) {
	ctx := context.Background()
	account := &Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "org-parent123",
		},
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{100: account}}
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(account): "fake-token",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)

	var capturedBeta string
	var detailCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			_ = json.NewEncoder(w).Encode(OpenAIQuotaUsage{
				RateLimitResetCredits: &OpenAIRateLimitResetCredits{AvailableCount: 2},
			})
		case "/backend-api/wham/rate-limit-reset-credits":
			detailCalls++
			capturedBeta = r.Header.Get("OpenAI-Beta")
			require.Equal(t, "org-parent123", r.Header.Get("ChatGPT-Account-ID"))
			_, _ = w.Write([]byte(`{"credits":[{"id":"secret-credit-id","expires_at":"2026-07-03T04:05:06Z"},{"expiresAt":"2026-07-04T04:05:06Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv))
	usage, err := svc.QueryUsage(ctx, 100)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, usage.RateLimitResetCredits)
	require.Equal(t, 2, usage.RateLimitResetCredits.AvailableCount)
	require.Equal(t, 1, detailCalls)
	require.Equal(t, openaiQuotaCodexBeta, capturedBeta)
	require.Equal(t, []OpenAIRateLimitResetCreditDetail{
		{ExpiresAt: "2026-07-03T04:05:06Z"},
		{ExpiresAt: "2026-07-04T04:05:06Z"},
	}, usage.RateLimitResetCredits.Credits)

	encoded, err := json.Marshal(usage)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret-credit-id")
}

func TestQueryUsageResetCreditDetails401NonFatal(t *testing.T) {
	ctx := context.Background()
	account := &Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "org-parent123",
		},
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{100: account}}
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(account): "fake-token",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)

	var detailCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			_ = json.NewEncoder(w).Encode(OpenAIQuotaUsage{
				RateLimitResetCredits: &OpenAIRateLimitResetCredits{AvailableCount: 1},
			})
		case "/backend-api/wham/rate-limit-reset-credits":
			detailCalls++
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized","id":"secret-error-id"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv))
	usage, err := svc.QueryUsage(ctx, 100)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, usage.RateLimitResetCredits)
	require.Equal(t, 1, usage.RateLimitResetCredits.AvailableCount)
	require.Equal(t, 1, detailCalls)
	require.Empty(t, usage.RateLimitResetCredits.Credits)
}

// TestResetCreditGetByIDError_FailsClosed 验证守卫「失败关闭」语义：
// 当守卫的 GetByID 发生瞬时错误时，ResetCredit 必须立即返回该错误，
// 不得旁路进入 prepareUpstreamCall（否则影子账号会借 resolve 路径操作母账号）。
//
// 区分方法：privacyClientFactory/tokenProvider 留 nil；
//   - 旁路路径：prepareUpstreamCall 配置检查先命中，报 "not configured"
//   - 守卫正确关闭：报 "account not found"（来自守卫的 infraerrors）
func TestResetCreditGetByIDError_FailsClosed(t *testing.T) {
	// 空 map：GetByID(200) 返回 "account 200 not found"
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{}}
	// tokenProvider / privacyClientFactory 故意为 nil：
	// 若代码泄漏到 prepareUpstreamCall，会因配置检查而报 "not configured" 而非 "account not found"。
	svc := &OpenAIQuotaService{accountRepo: repo}

	_, err := svc.ResetCredit(context.Background(), 200)
	require.Error(t, err, "GetByID error must propagate; got nil")
	require.NotContains(t, err.Error(), "not configured",
		"error reached prepareUpstreamCall config-check — guard did not fail-closed; got: %v", err)
}

// TestQueryUsageShadowResolve_EndToEnd 是端到端补充：通过 httptest 服务真实 /wham/usage
// 路径，验证影子账号的 QueryUsage 能成功拿到服务器响应（header 由母账号注入）。
func TestQueryUsageShadowResolve_EndToEnd(t *testing.T) {
	ctx := context.Background()
	pid := int64(100)

	shadow := &Account{
		ID: 200, ParentAccountID: &pid,
		Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, QuotaDimension: QuotaDimensionSpark,
	}
	parent := &Account{
		ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"chatgpt_account_id": "org-e2e-parent"},
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{200: shadow, 100: parent}}

	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(parent): "fake-token-e2e",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)

	// httptest server 记录收到的 chatgpt-account-id header，返回空 usage JSON
	var capturedAccountID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAccountID = r.Header.Get("chatgpt-account-id")
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(OpenAIQuotaUsage{})
	}))
	defer srv.Close()

	svc := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv))
	usage, err := svc.QueryUsage(ctx, 200)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, "org-e2e-parent", capturedAccountID,
		"upstream should receive parent's chatgpt-account-id; got: %s", capturedAccountID)
}
