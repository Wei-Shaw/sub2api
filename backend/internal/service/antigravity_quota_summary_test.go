//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// withAntigravityQuotaBaseURLs 临时替换 Antigravity 端点并重置可用性追踪，测试结束后恢复。
func withAntigravityQuotaBaseURLs(t *testing.T, urls ...string) {
	t.Helper()
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	t.Cleanup(func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
	})
	antigravity.BaseURLs = urls
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)
}

func agqFloat64Ptr(v float64) *float64 { return &v }

// ---------------------------------------------------------------------------
// normalizeTier：对齐官方 Antigravity Tools 的档位口径
// ---------------------------------------------------------------------------

func TestNormalizeTier_OfficialParity(t *testing.T) {
	tests := []struct {
		raw      string
		expected string
	}{
		{raw: "g1-pro-tier", expected: "PRO"},
		{raw: "Google AI Pro", expected: "PRO"},
		{raw: "Google One AI Premium", expected: "PRO"},
		{raw: "Gemini Advanced", expected: "PRO"},
		{raw: "g1-ultra-tier", expected: "ULTRA"},
		{raw: "Google AI Ultra", expected: "ULTRA"},
		{raw: "GOOGLE_ONE_HELIUM", expected: "ULTRA"},
		{raw: "GDP_HELIUM", expected: "ULTRA"},
		{raw: "helium", expected: "ULTRA"},
		{raw: "free-tier", expected: "FREE"},
		{raw: "Antigravity Starter Quota", expected: "FREE"},
		{raw: "standard-tier", expected: "UNKNOWN"},
		{raw: "", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			require.Equal(t, tt.expected, normalizeTier(tt.raw), "normalizeTier(%q)", tt.raw)
		})
	}
}

// ---------------------------------------------------------------------------
// resolveAntigravityQuotaBaseURLs：默认与转发端点一致优先 daily
// ---------------------------------------------------------------------------

func TestResolveAntigravityQuotaBaseURLs(t *testing.T) {
	t.Setenv(antigravityForwardBaseURLEnv, "")
	withAntigravityQuotaBaseURLs(t, "https://prod.example", "https://daily.example")

	paid := &Account{Platform: PlatformAntigravity, Credentials: map[string]any{"plan_type": "Pro"}}
	require.Equal(t, []string{"https://daily.example", "https://prod.example"}, resolveAntigravityQuotaBaseURLs(paid))

	free := &Account{Platform: PlatformAntigravity, Credentials: map[string]any{"plan_type": "free"}}
	require.Equal(t, []string{"https://daily.example", "https://prod.example"}, resolveAntigravityQuotaBaseURLs(free))

	t.Setenv(antigravityForwardBaseURLEnv, "daily")
	require.Equal(t, []string{"https://daily.example", "https://prod.example"}, resolveAntigravityQuotaBaseURLs(free))

	t.Setenv(antigravityForwardBaseURLEnv, "prod")
	require.Equal(t, []string{"https://prod.example", "https://daily.example"}, resolveAntigravityQuotaBaseURLs(paid))

	t.Setenv(antigravityForwardBaseURLEnv, "")
	withAntigravityQuotaBaseURLs(t, "https://only.example")
	require.Equal(t, []string{"https://only.example"}, resolveAntigravityQuotaBaseURLs(paid))
}

// ---------------------------------------------------------------------------
// selectAntigravityQuotaBucket：最受限窗口选择
// ---------------------------------------------------------------------------

func TestSelectAntigravityQuotaBucket(t *testing.T) {
	tests := []struct {
		name    string
		buckets []antigravity.QuotaSummaryBucket
		wantID  string
	}{
		{
			name: "仅 5h 桶",
			buckets: []antigravity.QuotaSummaryBucket{
				{BucketID: "gemini-5h", Window: "5h", RemainingFraction: agqFloat64Ptr(0.4)},
			},
			wantID: "gemini-5h",
		},
		{
			name: "仅 weekly 桶",
			buckets: []antigravity.QuotaSummaryBucket{
				{BucketID: "gemini-weekly", Window: "weekly", RemainingFraction: agqFloat64Ptr(0.7)},
			},
			wantID: "gemini-weekly",
		},
		{
			name: "5h 更受限时取 5h",
			buckets: []antigravity.QuotaSummaryBucket{
				{BucketID: "gemini-5h", Window: "5h", RemainingFraction: agqFloat64Ptr(0.3)},
				{BucketID: "gemini-weekly", Window: "weekly", RemainingFraction: agqFloat64Ptr(0.8)},
			},
			wantID: "gemini-5h",
		},
		{
			name: "weekly 更受限时取 weekly",
			buckets: []antigravity.QuotaSummaryBucket{
				{BucketID: "gemini-5h", Window: "5h", RemainingFraction: agqFloat64Ptr(0.9)},
				{BucketID: "gemini-weekly", Window: "weekly", RemainingFraction: agqFloat64Ptr(0.2)},
			},
			wantID: "gemini-weekly",
		},
		{
			name: "周配额耗尽时直接取 weekly",
			buckets: []antigravity.QuotaSummaryBucket{
				{BucketID: "gemini-5h", Window: "5h", RemainingFraction: agqFloat64Ptr(0.9)},
				{BucketID: "gemini-weekly", Window: "weekly", RemainingFraction: agqFloat64Ptr(0.001)},
			},
			wantID: "gemini-weekly",
		},
		{
			name: "窗口缺失时取首个带比例的桶",
			buckets: []antigravity.QuotaSummaryBucket{
				{BucketID: "daily", Window: "daily", RemainingFraction: agqFloat64Ptr(0.5)},
			},
			wantID: "daily",
		},
		{
			name: "缺少 remainingFraction 时返回 nil",
			buckets: []antigravity.QuotaSummaryBucket{
				{BucketID: "gemini-5h", Window: "5h"},
			},
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket := selectAntigravityQuotaBucket(tt.buckets)
			if tt.wantID == "" {
				require.Nil(t, bucket)
				return
			}
			require.NotNil(t, bucket)
			require.Equal(t, tt.wantID, bucket.BucketID)
		})
	}
}

// ---------------------------------------------------------------------------
// applyAntigravityQuotaSummaryToModels：用官方摘要覆盖目录里的假 100%
// ---------------------------------------------------------------------------

func agqTestSummary() *antigravity.QuotaSummaryResponse {
	return &antigravity.QuotaSummaryResponse{
		Groups: []antigravity.QuotaSummaryGroup{
			{
				DisplayName: "Gemini Models",
				Buckets: []antigravity.QuotaSummaryBucket{
					{BucketID: "gemini-5h", Window: "5h", RemainingFraction: agqFloat64Ptr(0.6), ResetTime: "2026-03-08T12:00:00Z"},
					{BucketID: "gemini-weekly", Window: "weekly", RemainingFraction: agqFloat64Ptr(0.9), ResetTime: "2026-03-10T12:00:00Z"},
				},
			},
			{
				DisplayName: "Claude and GPT models",
				Buckets: []antigravity.QuotaSummaryBucket{
					{BucketID: "3p-5h", Window: "5h", RemainingFraction: agqFloat64Ptr(0.8), ResetTime: "2026-03-08T18:00:00Z"},
					{BucketID: "3p-weekly", Window: "weekly", RemainingFraction: agqFloat64Ptr(0.001), ResetTime: "2026-03-12T00:00:00Z"},
				},
			},
			{
				DisplayName: "Gemini 3 Argon models",
				Buckets: []antigravity.QuotaSummaryBucket{
					{BucketID: "argon-5h", Window: "5h", RemainingFraction: agqFloat64Ptr(0.25)},
				},
			},
		},
	}
}

func TestApplyAntigravityQuotaSummaryToModels(t *testing.T) {
	info := &UsageInfo{
		AntigravityQuota: map[string]*AntigravityModelQuota{
			"gemini-3-pro-high":   {Utilization: 0, ResetTime: "2026-01-01T00:00:00Z"},
			"gemini-3-pro-image":  {Utilization: 0},
			"claude-sonnet-4-5":   {Utilization: 0},
			"gpt-oss-120b-medium": {Utilization: 0},
			"gemini-4-argon-high": {Utilization: 0},
			"imagen-3.0-generate": {Utilization: 0},
		},
	}

	applyAntigravityQuotaSummaryToModels(info, agqTestSummary())

	gemini := info.AntigravityQuota["gemini-3-pro-high"]
	require.Equal(t, 40, gemini.Utilization, "(1 - 0.6) * 100 = 40，目录里的假 100% 必须被覆盖")
	require.Equal(t, "2026-03-08T12:00:00Z", gemini.ResetTime)
	require.Equal(t, 40, info.AntigravityQuota["gemini-3-pro-image"].Utilization, "image 模型同样属于 Gemini 组")

	// 周配额已耗尽：直接取周窗口
	require.Equal(t, 100, info.AntigravityQuota["claude-sonnet-4-5"].Utilization)
	require.Equal(t, "2026-03-12T00:00:00Z", info.AntigravityQuota["claude-sonnet-4-5"].ResetTime)
	require.Equal(t, 100, info.AntigravityQuota["gpt-oss-120b-medium"].Utilization)

	require.Equal(t, 75, info.AntigravityQuota["gemini-4-argon-high"].Utilization)

	// 无法归组的模型保持原值，避免误改
	require.Equal(t, 0, info.AntigravityQuota["imagen-3.0-generate"].Utilization)
}

func TestApplyAntigravityQuotaSummaryBuckets(t *testing.T) {
	info := &UsageInfo{AntigravityQuota: map[string]*AntigravityModelQuota{}}
	summary := &antigravity.QuotaSummaryResponse{
		// 顶层 buckets 与 groups[].buckets 需要合并
		Buckets: []antigravity.QuotaSummaryBucket{
			{BucketID: "gemini-5h", Window: "5h", RemainingFraction: agqFloat64Ptr(0.4), ResetTime: "2026-03-08T12:00:00Z"},
			{BucketID: "gemini-weekly", Window: "weekly", RemainingFraction: agqFloat64Ptr(0.9), ResetTime: "2026-03-10T12:00:00Z"},
		},
		Groups: []antigravity.QuotaSummaryGroup{
			{
				DisplayName: "Claude and GPT models",
				Buckets: []antigravity.QuotaSummaryBucket{
					// 周配额耗尽时上游把 5h 桶标为 disabled + remainingFraction=1
					{BucketID: "3p-5h", Window: "5h", RemainingFraction: agqFloat64Ptr(1), Disabled: true},
					{BucketID: "3p-weekly", Window: "weekly", RemainingFraction: agqFloat64Ptr(0), ResetTime: "2026-03-12T00:00:00Z"},
					{BucketID: "   ", Window: "weekly", RemainingFraction: agqFloat64Ptr(0.5)},
					{BucketID: "no-fraction", Window: "weekly"},
				},
			},
		},
	}

	applyAntigravityQuotaSummaryBuckets(info, summary)

	require.Equal(t, 60, info.AntigravityQuota["gemini-5h"].Utilization)
	require.Equal(t, "2026-03-08T12:00:00Z", info.AntigravityQuota["gemini-5h"].ResetTime)
	require.Equal(t, 10, info.AntigravityQuota["gemini-weekly"].Utilization)
	require.Equal(t, 100, info.AntigravityQuota["3p-weekly"].Utilization)
	// disabled 的 5h 桶必须跳过，否则会展示一条假的 0% 已用
	require.NotContains(t, info.AntigravityQuota, "3p-5h")
	require.NotContains(t, info.AntigravityQuota, "no-fraction")
}

func TestApplyAntigravityQuotaSummaryBuckets_NilSafety(t *testing.T) {
	applyAntigravityQuotaSummaryBuckets(nil, agqTestSummary())
	applyAntigravityQuotaSummaryBuckets(&UsageInfo{}, nil)

	info := &UsageInfo{}
	applyAntigravityQuotaSummaryBuckets(info, &antigravity.QuotaSummaryResponse{})
	require.Empty(t, info.AntigravityQuota)
}

func TestAntigravityQuotaModelKind_IgnoresSyntheticBuckets(t *testing.T) {
	require.Empty(t, antigravityQuotaModelKind("gemini-5h"))
	require.Empty(t, antigravityQuotaModelKind("3p-weekly"))
	require.Equal(t, "gemini", antigravityQuotaModelKind("gemini-3-pro-high"))
	require.Equal(t, "3p", antigravityQuotaModelKind("gpt-oss-120b-medium"))
}

func TestApplyAntigravityQuotaSummaryToModels_NilSafety(t *testing.T) {
	applyAntigravityQuotaSummaryToModels(nil, agqTestSummary())
	applyAntigravityQuotaSummaryToModels(&UsageInfo{}, agqTestSummary())
	applyAntigravityQuotaSummaryToModels(&UsageInfo{AntigravityQuota: map[string]*AntigravityModelQuota{"gemini-3-pro-high": nil}}, agqTestSummary())
	applyAntigravityQuotaSummaryToModels(&UsageInfo{}, nil)

	info := &UsageInfo{AntigravityQuota: map[string]*AntigravityModelQuota{"gemini-3-pro-high": {Utilization: 0}}}
	applyAntigravityQuotaSummaryToModels(info, &antigravity.QuotaSummaryResponse{})
	require.Equal(t, 0, info.AntigravityQuota["gemini-3-pro-high"].Utilization)
}

func TestApplyAntigravityQuotaSummaryWindows(t *testing.T) {
	info := &UsageInfo{FiveHour: &UsageProgress{Utilization: 1}}
	applyAntigravityQuotaSummaryWindows(info, agqTestSummary())

	require.NotNil(t, info.FiveHour)
	require.Equal(t, 1.0, info.FiveHour.Utilization, "已有窗口保持不变")

	require.NotNil(t, info.SevenDay)
	require.Equal(t, 100.0, info.SevenDay.Utilization, "周配额 0.001 → 100%")

	empty := &UsageInfo{}
	applyAntigravityQuotaSummaryWindows(empty, agqTestSummary())
	require.NotNil(t, empty.FiveHour)
	require.Equal(t, 75.0, empty.FiveHour.Utilization, "目录没有 5h 窗口时用最受限的 5h 桶补齐 (argon 0.25)")
	require.NotNil(t, empty.SevenDay)
}

func TestApplyAntigravityQuotaSummaryWindows_TopLevelBucketsOnly(t *testing.T) {
	info := &UsageInfo{}
	applyAntigravityQuotaSummaryWindows(info, &antigravity.QuotaSummaryResponse{
		Buckets: []antigravity.QuotaSummaryBucket{
			{BucketID: "gemini-5h", Window: "5h", RemainingFraction: agqFloat64Ptr(0.5)},
			{BucketID: "gemini-weekly", Window: "weekly", RemainingFraction: agqFloat64Ptr(0.25)},
		},
	})
	require.NotNil(t, info.FiveHour)
	require.Equal(t, 50.0, info.FiveHour.Utilization)
	require.NotNil(t, info.SevenDay)
	require.Equal(t, 75.0, info.SevenDay.Utilization)
}

func TestBuildUsageInfoWithSummary_UsesFusedQuotaForFiveHour(t *testing.T) {
	fetcher := &AntigravityQuotaFetcher{}

	modelsResp := &antigravity.FetchAvailableModelsResponse{
		Models: map[string]antigravity.ModelInfo{
			// 目录对免费 / 付费账号都返回 remainingFraction=1 的假配额
			"claude-sonnet-4-20250514": {QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: 1}},
			"gemini-3-pro-high":        {QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: 1}},
		},
	}

	info := fetcher.buildUsageInfoWithSummary(modelsResp, "g1-pro-tier", "PRO", nil, agqTestSummary())

	require.Equal(t, 100, info.AntigravityQuota["claude-sonnet-4-20250514"].Utilization)
	require.Equal(t, 40, info.AntigravityQuota["gemini-3-pro-high"].Utilization)
	require.NotNil(t, info.FiveHour)
	require.Equal(t, 100.0, info.FiveHour.Utilization, "窗口必须在融合之后推导，否则会沿用未修正的目录用量")
	require.NotNil(t, info.SevenDay)
}

// ---------------------------------------------------------------------------
// FetchQuota：端点顺序 + 摘要融合（端到端）
// ---------------------------------------------------------------------------

const agqModelsBody = `{"models":{"gemini-3-pro-high":{"quotaInfo":{"remainingFraction":1}},"claude-sonnet-4-5":{"quotaInfo":{"remainingFraction":1}}}}`

const agqSummaryBody = `{"groups":[{"displayName":"Gemini Models","buckets":[{"bucketId":"gemini-5h","window":"5h","remainingFraction":0.6,"resetTime":"2026-03-08T12:00:00Z"}]},{"displayName":"Claude and GPT models","buckets":[{"bucketId":"3p-5h","window":"5h","remainingFraction":0.5,"resetTime":"2026-03-08T13:00:00Z"}]}]}`

const agqLoadCodeAssistBody = `{"paidTier":{"id":"g1-pro-tier","availableCredits":[{"creditType":"GOOGLE_ONE_AI","creditAmount":"25","minimumCreditAmountForUsage":"5"}]}}`

// antigravityQuotaTestServer 返回完整的三段式响应（目录 / 摘要 / 订阅）。
func antigravityQuotaTestServer(t *testing.T, calls *[]string, loadCodeAssistBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			*calls = append(*calls, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "project") {
			t.Errorf("Google One 账号无 project_id 时不应发送 project 字段: %s", body)
		}
		switch r.URL.Path {
		case "/v1internal:fetchAvailableModels":
			_, _ = io.WriteString(w, agqModelsBody)
		case "/v1internal:retrieveUserQuotaSummary":
			_, _ = io.WriteString(w, agqSummaryBody)
		case "/v1internal:loadCodeAssist":
			_, _ = io.WriteString(w, loadCodeAssistBody)
		default:
			t.Errorf("unexpected antigravity path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestFetchQuota_GoogleOnePrefersDailyAndFusesSummary(t *testing.T) {
	t.Setenv(antigravityForwardBaseURLEnv, "")

	var prodCalls int
	prod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		prodCalls++
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"NOT_FOUND"}`)
	}))
	defer prod.Close()

	var dailyPaths []string
	daily := antigravityQuotaTestServer(t, &dailyPaths, agqLoadCodeAssistBody)
	defer daily.Close()

	withAntigravityQuotaBaseURLs(t, prod.URL, daily.URL)

	fetcher := NewAntigravityQuotaFetcher(nil, &config.Config{})
	result, err := fetcher.FetchQuota(context.Background(), &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token",
			"plan_type":    "Pro",
		},
	}, "")
	require.NoError(t, err)

	require.Equal(t, 0, prodCalls, "Google One / 付费账号必须优先 daily 端点")
	require.Contains(t, dailyPaths, "/v1internal:fetchAvailableModels")
	require.Contains(t, dailyPaths, "/v1internal:retrieveUserQuotaSummary")
	require.Contains(t, dailyPaths, "/v1internal:loadCodeAssist")

	require.Equal(t, 40, result.UsageInfo.AntigravityQuota["gemini-3-pro-high"].Utilization)
	require.Equal(t, 50, result.UsageInfo.AntigravityQuota["claude-sonnet-4-5"].Utilization)
	// 活跃配额桶按 bucketId 记录，供前端按 Gemini / Claude 两组展示 5h + 7d
	require.Equal(t, 40, result.UsageInfo.AntigravityQuota["gemini-5h"].Utilization)
	require.Equal(t, 50, result.UsageInfo.AntigravityQuota["3p-5h"].Utilization)
	require.Equal(t, "PRO", result.UsageInfo.SubscriptionTier)
	require.Equal(t, "g1-pro-tier", result.UsageInfo.SubscriptionTierRaw)
	require.NotNil(t, result.UsageInfo.FiveHour)
	require.Equal(t, 50.0, result.UsageInfo.FiveHour.Utilization, "最受限的 5h 桶为 3p 组 0.5")
	require.Len(t, result.UsageInfo.AICredits, 1)
	require.Equal(t, "GOOGLE_ONE_AI", result.UsageInfo.AICredits[0].CreditType)
}

func TestFetchQuota_FreeAccountPrefersProdAndFallsBackToDaily(t *testing.T) {
	t.Setenv(antigravityForwardBaseURLEnv, "")

	var prodCalls int
	prod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		prodCalls++
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"NOT_FOUND"}`)
	}))
	defer prod.Close()

	var dailyPaths []string
	daily := antigravityQuotaTestServer(t, &dailyPaths, `{"currentTier":{"id":"free-tier"}}`)
	defer daily.Close()

	withAntigravityQuotaBaseURLs(t, prod.URL, daily.URL)

	fetcher := NewAntigravityQuotaFetcher(nil, &config.Config{})
	result, err := fetcher.FetchQuota(context.Background(), &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token",
			"plan_type":    "free",
		},
	}, "")
	require.NoError(t, err)

	require.Greater(t, prodCalls, 0, "免费账号应优先 prod 端点")
	require.NotEmpty(t, dailyPaths, "prod 404 后必须回退到 daily")
	require.Equal(t, 40, result.UsageInfo.AntigravityQuota["gemini-3-pro-high"].Utilization)
	require.Equal(t, "FREE", result.UsageInfo.SubscriptionTier)
}

func TestFetchQuota_QuotaSummaryFailureKeepsCatalogQuota(t *testing.T) {
	t.Setenv(antigravityForwardBaseURLEnv, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1internal:fetchAvailableModels":
			_, _ = io.WriteString(w, agqModelsBody)
		case "/v1internal:retrieveUserQuotaSummary":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"quota summary unavailable"}`)
		case "/v1internal:loadCodeAssist":
			_, _ = io.WriteString(w, agqLoadCodeAssistBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	withAntigravityQuotaBaseURLs(t, server.URL)

	fetcher := NewAntigravityQuotaFetcher(nil, &config.Config{})
	result, err := fetcher.FetchQuota(context.Background(), &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token",
			"plan_type":    "Pro",
		},
	}, "")
	require.NoError(t, err, "配额摘要失败必须降级而不影响主流程")
	require.NotNil(t, result.UsageInfo)
	require.Equal(t, 0, result.UsageInfo.AntigravityQuota["gemini-3-pro-high"].Utilization)
	require.Equal(t, "PRO", result.UsageInfo.SubscriptionTier)
}
