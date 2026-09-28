//go:build unit

package service

// 国产 coding plan 额度探测的健壮性：
//
//   - 无效的 2xx 响应（非 JSON / 缺容器 / 解析不出窗口）报错、不落快照；
//   - 探测成功后清掉本次没返回的窗口；
//   - 解析器区分「窗口未声明」（合法缺席）与「声明了但字段非法」（整次失败）。
//
// 全部通过 QueryUsageForAccount / QueryUsage / cnQuotaExtraUpdates /
// EvaluateAccountSchedulingThreshold 测行为。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// scriptedCNQuotaUpstream 固定回一份状态码 + 响应体。
type scriptedCNQuotaUpstream struct {
	status int
	body   string
	calls  int
}

func (u *scriptedCNQuotaUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.calls++
	return &http.Response{StatusCode: u.status, Body: io.NopCloser(strings.NewReader(u.body)), Header: make(http.Header)}, nil
}

func (u *scriptedCNQuotaUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

// cnQuotaSnapshotRepo 记录 UpdateExtra；account 非空时按仓储的 JSONB
// `COALESCE(extra,'{}') || $1::jsonb` 语义合并回账号（经 JSON 往返：nil 写成 null、
// 键保留，读回仍是 nil），模拟「落库后下一次加载」看到的 extra。
type cnQuotaSnapshotRepo struct {
	AccountRepository
	account      *Account
	extraUpdates []map[string]any
}

func (r *cnQuotaSnapshotRepo) GetByID(context.Context, int64) (*Account, error) {
	if r.account == nil {
		return nil, ErrAccountNotFound
	}
	return r.account, nil
}

func (r *cnQuotaSnapshotRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.extraUpdates = append(r.extraUpdates, updates)
	if r.account != nil {
		payload, err := json.Marshal(updates)
		if err != nil {
			return err
		}
		var decoded map[string]any
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return err
		}
		if r.account.Extra == nil {
			r.account.Extra = map[string]any{}
		}
		for k, v := range decoded {
			r.account.Extra[k] = v
		}
	}
	return nil
}

// cnProbeAccount 构造一个可直接走额度探测的 Coding Plan 账号（base_url 用平台默认）。
func cnProbeAccount(id int64, platform string) *Account {
	credentials := map[string]any{"api_key": "sk-test"}
	if platform != PlatformOpenCodeGo {
		credentials["account_mode"] = AccountModeCoding
	}
	return &Account{ID: id, Platform: platform, Type: AccountTypeAPIKey, Status: StatusActive, Credentials: credentials}
}

// ---------------------------------------------------------------------------
// 拒绝无效的 2xx
// ---------------------------------------------------------------------------

// HTTP 2xx 但响应体不可信（非 JSON / 缺容器 / 解析不出任何窗口）时：必须报错、
// 不落快照（连 <provider>_usage_updated_at 也不能刷新）。
func TestCNQuotaProbe_InvalidResponseDoesNotPersist(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		body     string
	}{
		// (a) 不是合法 JSON
		{name: "kimi non json", platform: PlatformKimi, body: `<html><body>502 Bad Gateway</body></html>`},
		{name: "zhipu non json", platform: PlatformZhipu, body: `not json at all`},
		{name: "minimax non json", platform: PlatformMiniMax, body: ``},
		{name: "opencode go non json", platform: PlatformOpenCodeGo, body: `<!doctype html>`},

		// (b) 合法 JSON 但缺少解析器要读的容器
		{name: "kimi missing containers", platform: PlatformKimi, body: `{"code":0,"message":"ok"}`},
		{name: "zhipu missing data limits", platform: PlatformZhipu, body: `{"success":true,"data":{"level":"pro"}}`},
		{name: "minimax missing model remains", platform: PlatformMiniMax, body: `{"base_resp":{"status_code":0}}`},
		{name: "opencode go missing usage", platform: PlatformOpenCodeGo, body: `{"plan":"go"}`},

		// (c) 容器在但一个窗口都解析不出来
		{name: "kimi empty limits", platform: PlatformKimi, body: `{"limits":[]}`},
		{name: "zhipu empty limits", platform: PlatformZhipu, body: `{"success":true,"data":{"limits":[]}}`},
		{name: "zhipu unknown limit types", platform: PlatformZhipu, body: `{"success":true,"data":{"limits":[{"type":"OTHER_LIMIT","percentage":9}]}}`},
		{name: "minimax without general model", platform: PlatformMiniMax, body: `{"model_remains":[{"model_name":"video"}]}`},
		{name: "opencode go empty usage", platform: PlatformOpenCodeGo, body: `{"usage":{}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &cnQuotaSnapshotRepo{}
			upstream := &scriptedCNQuotaUpstream{status: http.StatusOK, body: tc.body}
			svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

			result, err := svc.QueryUsageForAccount(context.Background(), cnProbeAccount(11, tc.platform))

			require.NoError(t, err)
			require.Equal(t, 1, upstream.calls)
			require.Empty(t, repo.extraUpdates, "an untrustworthy response must not touch the snapshot (not even usage_updated_at)")
			require.False(t, result.Success, "tiers=%v", result.Tiers)
			require.False(t, result.Persisted)
			require.Contains(t, result.Error, "CN_QUOTA_INVALID_RESPONSE")
			require.Empty(t, result.Tiers)
		})
	}
}

// 业务级错误（智谱 success=false / MiniMax base_resp）仍然回原始文案、不落快照。
// 这是回归保护：业务错误的原始文案不能被「结构不对」的笼统报错覆盖。
func TestCNQuotaProbe_BusinessErrorsKeepTheirMessage(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		body     string
		wantErr  string
	}{
		{name: "zhipu business error", platform: PlatformZhipu, body: `{"success":false,"msg":"当前用户不存在coding plan"}`, wantErr: "当前用户不存在coding plan"},
		{name: "minimax business error", platform: PlatformMiniMax, body: `{"base_resp":{"status_code":1004,"status_msg":"invalid api key"}}`, wantErr: "invalid api key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &cnQuotaSnapshotRepo{}
			upstream := &scriptedCNQuotaUpstream{status: http.StatusOK, body: tc.body}
			svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

			result, err := svc.QueryUsageForAccount(context.Background(), cnProbeAccount(12, tc.platform))

			require.NoError(t, err)
			require.False(t, result.Success)
			require.Contains(t, result.Error, tc.wantErr)
			require.NotContains(t, result.Error, "CN_QUOTA_INVALID_RESPONSE")
			require.Empty(t, repo.extraUpdates)
		})
	}
}

// 端到端：90% 周用量 + 80% 阈值、快照已过期 1 小时的账号，被周期任务（QueryUsage）
// 探测时遇到网关错误页。快照必须原样保留——包括 usage_updated_at，否则前端
// CNProviderQuotaCell 会把旧快照当成 15 分钟内的新快照、不再自动探测——停调也不能解除。
func TestCNQuotaProbe_InvalidProbeKeepsSnapshotFreshnessAndPause(t *testing.T) {
	now := time.Now().UTC()
	staleUpdatedAt := now.Add(-time.Hour).Format(time.RFC3339)
	account := cnProbeAccount(28, PlatformMiniMax)
	account.Extra = map[string]any{
		"minimax_5h_used_percent":     10.0,
		"minimax_5h_reset_at":         now.Add(time.Hour).Format(time.RFC3339),
		"minimax_weekly_used_percent": 90.0,
		"minimax_weekly_reset_at":     now.Add(48 * time.Hour).Format(time.RFC3339),
		"minimax_usage_updated_at":    staleUpdatedAt,
	}
	thresholds := map[string]int{PlatformMiniMax: 80}
	require.True(t, EvaluateAccountSchedulingThreshold(account, thresholds, now).ShouldPause,
		"precondition: the account is paused by the 90% weekly window")

	repo := &cnQuotaSnapshotRepo{account: account}
	upstream := &scriptedCNQuotaUpstream{status: http.StatusOK, body: `<html>502</html>`}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	result, err := svc.QueryUsage(context.Background(), account.ID)

	require.NoError(t, err)
	require.Equal(t, 1, upstream.calls)
	require.Equal(t, staleUpdatedAt, account.Extra["minimax_usage_updated_at"],
		"a garbage 2xx must not make the stale snapshot look fresh")
	require.False(t, result.Success, "tiers=%v", result.Tiers)
	require.Contains(t, result.Error, "CN_QUOTA_INVALID_RESPONSE", "the failure is surfaced, not swallowed")
	require.Empty(t, repo.extraUpdates)
	require.Equal(t, 90.0, account.Extra["minimax_weekly_used_percent"])

	decision := EvaluateAccountSchedulingThreshold(account, thresholds, now)
	require.True(t, decision.ShouldPause, "the account must stay paused after an invalid probe")
	require.Equal(t, "weekly", decision.Window)
	require.InDelta(t, 90.0, decision.UsedPercent, 1e-9)
}

// ---------------------------------------------------------------------------
// 探测成功后清掉这次没返回的窗口
// ---------------------------------------------------------------------------

// 快照更新必须覆盖全部三档窗口键：缺席的窗口写 null，出现的窗口写值。
func TestCNQuotaProbe_ExtraUpdatesClearsAbsentWindows(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	updates := cnQuotaExtraUpdates(PlatformMiniMax, []CNQuotaTier{
		{Window: "5h", UsedPercent: 8, ResetAt: "2026-09-22T05:00:00Z"},
	}, now)

	require.Equal(t, 8.0, updates["minimax_5h_used_percent"])
	require.Equal(t, "2026-09-22T05:00:00Z", updates["minimax_5h_reset_at"])
	for _, key := range []string{
		"minimax_weekly_used_percent",
		"minimax_weekly_reset_at",
		"minimax_monthly_used_percent",
		"minimax_monthly_reset_at",
	} {
		require.Contains(t, updates, key)
		require.Nil(t, updates[key], key)
	}

	// 上游回了窗口但没给重置时间时，reset 键同样被清掉（不留上一轮的旧值）。
	noReset := cnQuotaExtraUpdates(PlatformKimi, []CNQuotaTier{{Window: "weekly", UsedPercent: 3}}, now)
	require.Equal(t, 3.0, noReset["kimi_weekly_used_percent"])
	require.Contains(t, noReset, "kimi_weekly_reset_at")
	require.Nil(t, noReset["kimi_weekly_reset_at"])
	require.Contains(t, noReset, "kimi_5h_used_percent")
	require.Nil(t, noReset["kimi_5h_used_percent"])
}

// 端到端：上游停发某个窗口（MiniMax 关掉周限额 / 智谱换成只有 5h 的套餐 /
// Kimi 不再下发 usage）后，探测成功，旧窗口必须从快照里消失，不能继续用旧值停调。
func TestCNQuotaProbe_DroppedWindowIsClearedAfterSuccessfulProbe(t *testing.T) {
	now := time.Now().UTC()
	futureMs := now.Add(3 * time.Hour).UnixMilli()
	cases := []struct {
		name        string
		platform    string
		body        string
		staleKey    string
		wantWindows []string
	}{
		{
			name:     "minimax weekly limit turned off",
			platform: PlatformMiniMax,
			body: `{"model_remains":[{
				"model_name":"general",
				"current_interval_remaining_percent":80,
				"end_time":` + jsonInt(futureMs) + `,
				"current_weekly_status":0
			}]}`,
			staleKey:    "minimax_weekly_used_percent",
			wantWindows: []string{"5h"},
		},
		{
			name:        "zhipu plan changed to 5h only",
			platform:    PlatformZhipu,
			body:        `{"success":true,"data":{"level":"lite","limits":[{"type":"TOKENS_LIMIT","unit":3,"percentage":15,"nextResetTime":` + jsonInt(futureMs) + `}]}}`,
			staleKey:    "zhipu_weekly_used_percent",
			wantWindows: []string{"5h"},
		},
		{
			name:        "kimi without usage object",
			platform:    PlatformKimi,
			body:        `{"limits":[{"detail":{"limit":100,"remaining":40,"resetTime":"` + now.Add(3*time.Hour).Format(time.RFC3339) + `"}}]}`,
			staleKey:    "kimi_weekly_used_percent",
			wantWindows: []string{"5h"},
		},
		{
			name:        "opencode go rolling only",
			platform:    PlatformOpenCodeGo,
			body:        `{"usage":{"rolling":{"percent":12.5,"resetsAt":"` + now.Add(3*time.Hour).Format(time.RFC3339) + `"}}}`,
			staleKey:    "opencode_go_weekly_used_percent",
			wantWindows: []string{"5h"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.platform
			account := cnProbeAccount(26, p)
			account.Extra = map[string]any{
				p + "_5h_used_percent":     10.0,
				p + "_5h_reset_at":         now.Add(time.Hour).Format(time.RFC3339),
				p + "_weekly_used_percent": 95.0,
				p + "_weekly_reset_at":     now.Add(48 * time.Hour).Format(time.RFC3339),
				p + "_usage_updated_at":    now.Add(-time.Hour).Format(time.RFC3339),
			}
			thresholds := map[string]int{p: 80}
			require.True(t, EvaluateAccountSchedulingThreshold(account, thresholds, now).ShouldPause,
				"precondition: paused by the 95% weekly window")

			repo := &cnQuotaSnapshotRepo{account: account}
			upstream := &scriptedCNQuotaUpstream{status: http.StatusOK, body: tc.body}
			svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

			result, err := svc.QueryUsageForAccount(context.Background(), account)

			require.NoError(t, err)
			require.True(t, result.Success, result.Error)
			require.True(t, result.Persisted)
			require.Len(t, result.Tiers, len(tc.wantWindows))
			for i, window := range tc.wantWindows {
				require.Equal(t, window, result.Tiers[i].Window)
			}
			require.Len(t, repo.extraUpdates, 1)
			require.Contains(t, repo.extraUpdates[0], tc.staleKey, "the dropped window must be cleared explicitly")
			require.Nil(t, account.Extra[tc.staleKey], "stale weekly value must not survive a successful probe")
			require.Nil(t, account.Extra[p+"_weekly_reset_at"])
			require.False(t, EvaluateAccountSchedulingThreshold(account, thresholds, now).ShouldPause,
				"the dropped weekly window must no longer pause the account")
		})
	}
}

// 读侧兼容性：null（JSONB 合并写入的 JSON null，读回为 nil）必须等同「窗口不存在」。
// 覆盖上游所有读 CN 窗口快照的后端入口：阈值停调候选与 429 冷却重置点。
func TestCNQuotaProbe_NullWindowIsAbsentForReaders(t *testing.T) {
	now := time.Now().UTC()
	raw := map[string]any{
		"minimax_5h_used_percent":      85.0,
		"minimax_5h_reset_at":          now.Add(time.Hour).Format(time.RFC3339),
		"minimax_weekly_used_percent":  nil,
		"minimax_weekly_reset_at":      nil,
		"minimax_monthly_used_percent": nil,
		"minimax_monthly_reset_at":     nil,
	}
	payload, err := json.Marshal(raw)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"minimax_weekly_used_percent":null`)
	var extra map[string]any
	require.NoError(t, json.Unmarshal(payload, &extra))
	require.Contains(t, extra, "minimax_weekly_used_percent")
	require.Nil(t, extra["minimax_weekly_used_percent"])

	account := cnProbeAccount(30, PlatformMiniMax)
	account.Extra = extra

	// 阈值 80：只有 5h 窗口（85%）能触发，null 周窗口不产生候选。
	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformMiniMax: 80}, now)
	require.True(t, decision.ShouldPause)
	require.Equal(t, "5h", decision.Window)

	// 阈值 90：5h 不够，null 周窗口也不能被当成 0% 以外的任何值触发停调。
	require.False(t, EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformMiniMax: 90}, now).ShouldPause)

	// 只剩 null 窗口时：不停调。
	onlyNull := cnProbeAccount(31, PlatformMiniMax)
	onlyNull.Extra = map[string]any{"minimax_weekly_used_percent": nil, "minimax_weekly_reset_at": nil}
	require.False(t, EvaluateAccountSchedulingThreshold(onlyNull, map[string]int{PlatformMiniMax: 0}, now).ShouldPause)

	// 429 冷却重置点：null 的 reset 键被跳过，只取 5h。
	reset := cnProviderQuotaSnapshotReset(account, now)
	require.NotNil(t, reset)
	require.WithinDuration(t, now.Add(time.Hour), *reset, 2*time.Second)
	require.Nil(t, cnProviderQuotaSnapshotReset(onlyNull, now))
}

// ---------------------------------------------------------------------------
// 解析器区分「没声明」和「声明了但非法」
// ---------------------------------------------------------------------------

// 窗口已声明但必需字段缺失/非法：整次探测判失败、不写快照。
// 否则这些情况会被降级成假的 0%（或 100%、负数）、或悄悄丢掉窗口，然后照常落快照。
func TestCNQuotaProbe_DeclaredButInvalidDoesNotPersist(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		body     string
	}{
		{name: "kimi weekly declared without fields", platform: PlatformKimi, body: `{"usage":{}}`},
		{name: "kimi detail missing limit", platform: PlatformKimi, body: `{"limits":[{"detail":{"remaining":5}}]}`},
		{name: "kimi detail limit zero", platform: PlatformKimi, body: `{"limits":[{"detail":{"limit":0,"remaining":0}}]}`},
		{name: "kimi weekly remaining not a number", platform: PlatformKimi, body: `{"usage":{"limit":100,"remaining":"abc"}}`},
		{
			name:     "zhipu entry missing percentage",
			platform: PlatformZhipu,
			body:     `{"success":true,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"nextResetTime":1700000000000}]}}`,
		},
		{
			name:     "zhipu credit fallback negative percentage",
			platform: PlatformZhipu,
			body:     `{"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","percentage":-1}]}}`,
		},
		{
			name:     "minimax weekly enabled without percent",
			platform: PlatformMiniMax,
			body: `{"model_remains":[{
				"model_name":"general",
				"current_interval_remaining_percent":20,
				"end_time":1790000000000,
				"current_weekly_status":1
			}]}`,
		},
		{
			name:     "minimax 5h percent missing",
			platform: PlatformMiniMax,
			body:     `{"model_remains":[{"model_name":"general","current_weekly_status":1,"current_weekly_remaining_percent":30}]}`,
		},
		{
			name:     "opencode go weekly node without percent",
			platform: PlatformOpenCodeGo,
			body:     `{"usage":{"rolling":{"percent":12},"weekly":{}}}`,
		},
		{
			name:     "opencode go negative percent",
			platform: PlatformOpenCodeGo,
			body:     `{"usage":{"rolling":{"percent":-5}}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &cnQuotaSnapshotRepo{}
			upstream := &scriptedCNQuotaUpstream{status: http.StatusOK, body: tc.body}
			svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

			result, err := svc.QueryUsageForAccount(context.Background(), cnProbeAccount(15, tc.platform))

			require.NoError(t, err)
			require.Equal(t, 1, upstream.calls)
			require.Empty(t, repo.extraUpdates, "a declared-but-invalid window must not be persisted")
			require.False(t, result.Success, "tiers=%v", result.Tiers)
			require.False(t, result.Persisted)
			require.Contains(t, result.Error, "CN_QUOTA_INVALID_RESPONSE")
		})
	}
}

// 端到端：账号被 90% 周窗口停调（阈值 80）。上游这次回的周窗口字段非法——
// 若被当成 0% 覆盖进快照（Kimi limit<=0、智谱缺 percentage），停调会被误解除。
func TestCNQuotaProbe_DeclaredButInvalidMustNotLiftPause(t *testing.T) {
	now := time.Now().UTC()
	futureMs := now.Add(48 * time.Hour).UnixMilli()
	fiveHourReset := now.Add(time.Hour).Format(time.RFC3339)
	cases := []struct {
		name     string
		platform string
		body     string
	}{
		{
			name:     "kimi weekly limit zero",
			platform: PlatformKimi,
			body: `{
				"limits":[{"detail":{"limit":100,"remaining":40,"resetTime":"` + fiveHourReset + `"}}],
				"usage":{"limit":0,"remaining":0,"resetTime":"` + now.Add(48*time.Hour).Format(time.RFC3339) + `"}
			}`,
		},
		{
			name:     "kimi weekly fields missing",
			platform: PlatformKimi,
			body: `{
				"limits":[{"detail":{"limit":100,"remaining":40,"resetTime":"` + fiveHourReset + `"}}],
				"usage":{"resetTime":"` + now.Add(48*time.Hour).Format(time.RFC3339) + `"}
			}`,
		},
		{
			name:     "zhipu weekly entry missing percentage",
			platform: PlatformZhipu,
			body: `{"success":true,"data":{"limits":[
				{"type":"TOKENS_LIMIT","unit":3,"percentage":10,"nextResetTime":` + jsonInt(now.Add(time.Hour).UnixMilli()) + `},
				{"type":"TOKENS_LIMIT","unit":6,"nextResetTime":` + jsonInt(futureMs) + `}
			]}}`,
		},
		{
			name:     "minimax weekly enabled without percent",
			platform: PlatformMiniMax,
			body: `{"model_remains":[{
				"model_name":"general",
				"current_interval_remaining_percent":80,
				"end_time":` + jsonInt(now.Add(time.Hour).UnixMilli()) + `,
				"current_weekly_status":1,
				"weekly_end_time":` + jsonInt(futureMs) + `
			}]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.platform
			staleUpdatedAt := now.Add(-time.Hour).Format(time.RFC3339)
			account := cnProbeAccount(29, p)
			account.Extra = map[string]any{
				p + "_5h_used_percent":     10.0,
				p + "_5h_reset_at":         fiveHourReset,
				p + "_weekly_used_percent": 90.0,
				p + "_weekly_reset_at":     now.Add(48 * time.Hour).Format(time.RFC3339),
				p + "_usage_updated_at":    staleUpdatedAt,
			}
			thresholds := map[string]int{p: 80}
			require.True(t, EvaluateAccountSchedulingThreshold(account, thresholds, now).ShouldPause,
				"precondition: paused by the 90% weekly window")

			repo := &cnQuotaSnapshotRepo{account: account}
			upstream := &scriptedCNQuotaUpstream{status: http.StatusOK, body: tc.body}
			svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

			result, err := svc.QueryUsageForAccount(context.Background(), account)
			require.NoError(t, err)

			decision := EvaluateAccountSchedulingThreshold(account, thresholds, now)
			require.True(t, decision.ShouldPause,
				"pause must survive; weekly now=%v tiers=%v", account.Extra[p+"_weekly_used_percent"], result.Tiers)
			require.Equal(t, "weekly", decision.Window)
			require.InDelta(t, 90.0, decision.UsedPercent, 1e-9)
			require.Equal(t, staleUpdatedAt, account.Extra[p+"_usage_updated_at"], "snapshot must not be refreshed")
			require.False(t, result.Success, "tiers=%v", result.Tiers)
			require.Contains(t, result.Error, "CN_QUOTA_INVALID_RESPONSE")
		})
	}
}

// 没声明的窗口仍然算合法缺席、正常成功（不能把它们也打成失败）；
// remaining > limit / 剩余 >100% 这类上游超发值仍按 0 已用夹住，不当错误。
func TestCNQuotaProbe_UndeclaredOrClampedStillSucceeds(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		body     string
		want     map[string]float64
	}{
		{
			name:     "kimi remaining above limit",
			platform: PlatformKimi,
			body:     `{"usage":{"limit":100,"remaining":140}}`,
			want:     map[string]float64{"weekly": 0},
		},
		{
			name:     "minimax weekly status off without percent",
			platform: PlatformMiniMax,
			body:     `{"model_remains":[{"model_name":"general","current_interval_remaining_percent":120,"current_weekly_status":0}]}`,
			want:     map[string]float64{"5h": 0},
		},
		{
			name:     "zhipu non-window entries are skipped",
			platform: PlatformZhipu,
			body:     `{"success":true,"data":{"limits":[{"type":"OTHER_LIMIT"},{"type":"TOKENS_LIMIT","unit":3,"percentage":"12.5"}]}}`,
			want:     map[string]float64{"5h": 12.5},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &cnQuotaSnapshotRepo{}
			upstream := &scriptedCNQuotaUpstream{status: http.StatusOK, body: tc.body}
			svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

			result, err := svc.QueryUsageForAccount(context.Background(), cnProbeAccount(16, tc.platform))

			require.NoError(t, err)
			require.True(t, result.Success, result.Error)
			require.True(t, result.Persisted)
			got := map[string]float64{}
			for _, tier := range result.Tiers {
				got[tier.Window] = tier.UsedPercent
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
