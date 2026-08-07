package service

// 国产 Coding Plan 配额监控（DeepSeek / 智谱 GLM / Kimi For Coding）。
//
// 参考 cc-switch 的 src-tauri/src/services/coding_plan.rs：
//   - Kimi:  GET {endpoint}/coding/v1/usages                    (Authorization: Bearer)
//   - GLM:   GET {endpoint}/api/monitor/usage/quota/limit       (Authorization: 裸 key)
//   - DeepSeek: GET {endpoint}/user/balance                     (Authorization: Bearer，仅余额)
//
// 与推理型 provider 的区别：这里不做 challenge 校验，而是把上游返回的套餐配额
// 解析成 MonitorQuotaSnapshot 存进 CheckResult.Quota，供前端渲染进度条/余额。
//
// 特别地，GLM 的 limits[].type 字段已从 TOKENS_LIMIT 变更为 CREDIT_LIMIT
// （cc-switch issue #6153），这里同时兼容两者。

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const (
	// codingPlanDeepseekBalancePath DeepSeek 余额查询路径。
	codingPlanDeepseekBalancePath = "/user/balance"
	// codingPlanGLMQuotaPath 智谱 GLM 套餐额度查询路径。
	codingPlanGLMQuotaPath = "/api/monitor/usage/quota/limit"
	// codingPlanKimiUsagesPath Kimi For Coding 用量查询路径。
	codingPlanKimiUsagesPath = "/coding/v1/usages"
)

// codingPlanDefaultModel 是 coding-plan provider 未显式指定主模型时使用的占位名。
// 它不会作为推理模型被请求，仅用于满足 schema 的 NotEmpty 约束 + 前端展示。
func codingPlanDefaultModel(provider string) string {
	switch provider {
	case MonitorProviderDeepseek:
		return "balance"
	case MonitorProviderGLM:
		return "coding-plan"
	case MonitorProviderKimi:
		return "kimi-for-coding"
	default:
		return "coding-plan"
	}
}

// runCodingPlanCheck 对 coding-plan provider 执行一次配额/余额查询。
// 与 runCheckForModel 对称：所有失败都包装进 CheckResult.Status，不返回 error。
func runCodingPlanCheck(ctx context.Context, provider, endpoint, apiKey, model string) *CheckResult {
	res := &CheckResult{
		Model:     model,
		Status:    MonitorStatusError,
		CheckedAt: time.Now(),
	}

	start := time.Now()
	var (
		snapshot *MonitorQuotaSnapshot
		errMsg   string
		authErr  bool
	)
	switch provider {
	case MonitorProviderDeepseek:
		snapshot, errMsg, authErr = queryDeepseekBalance(ctx, endpoint, apiKey)
	case MonitorProviderGLM:
		snapshot, errMsg, authErr = queryGLMQuota(ctx, endpoint, apiKey)
	case MonitorProviderKimi:
		snapshot, errMsg, authErr = queryKimiUsage(ctx, endpoint, apiKey)
	default:
		res.Message = truncateMessage(fmt.Sprintf("unsupported coding plan provider %q", provider))
		return res
	}
	latencyMs := int(time.Since(start) / time.Millisecond)
	res.LatencyMs = &latencyMs

	if errMsg != "" {
		if authErr {
			res.Status = MonitorStatusFailed
		}
		res.Message = truncateMessage(sanitizeErrorMessage(errMsg))
		return res
	}

	res.Quota = snapshot
	res.Status = MonitorStatusOperational
	res.Message = truncateMessage(codingPlanSummary(provider, snapshot))
	return res
}

// codingPlanSummary 生成一行简短文本存入 history.message（配额 JSON 另存 quota 列）。
func codingPlanSummary(provider string, s *MonitorQuotaSnapshot) string {
	if s == nil {
		return ""
	}
	var parts []string
	for _, t := range s.Tiers {
		switch t.Name {
		case QuotaTierBalance:
			if t.Balance != nil && t.Currency != nil {
				parts = append(parts, fmt.Sprintf("balance %s %s", *t.Balance, *t.Currency))
			}
		default:
			if t.Utilization != nil {
				parts = append(parts, fmt.Sprintf("%s %.0f%%", t.Name, *t.Utilization))
			}
		}
	}
	if s.PlanLevel != "" {
		parts = append(parts, "plan="+s.PlanLevel)
	}
	if s.Available != nil && !*s.Available {
		parts = append(parts, "balance depleted")
	}
	return strings.Join(parts, "; ")
}

// doCodingPlanGet 发起一次 GET 查询并读取响应体（限制大小），返回 body 与 HTTP status。
// authScheme 为 "bearer" 时带 Authorization: ****** "raw" 时带裸 key（智谱）。
func doCodingPlanGet(ctx context.Context, endpoint, path, apiKey, authScheme string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(endpoint, path), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	switch authScheme {
	case "raw":
		req.Header.Set("Authorization", apiKey)
	default:
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := monitorHTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, monitorResponseMaxBytes))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return body, resp.StatusCode, nil
}

// classifyCodingPlanHTTP 把 HTTP 层结果归一为 (errMsg, authErr)。
// 返回空 errMsg 表示成功。
func classifyCodingPlanHTTP(status int, body []byte, netErr error) (string, bool) {
	if netErr != nil {
		return "Network error: " + netErr.Error(), false
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Sprintf("authentication failed (HTTP %d)", status), true
	}
	if status < 200 || status >= 300 {
		return fmt.Sprintf("API error (HTTP %d): %s", status, truncateForErrorBody(string(body))), false
	}
	return "", false
}

// parseCodingPlanJSON 解析响应 JSON；失败返回错误文案（确定性错误，非瞬时）。
func parseCodingPlanJSON(body []byte) (gjson.Result, string) {
	if !gjson.ValidBytes(body) {
		return gjson.Result{}, "failed to parse response JSON"
	}
	return gjson.ParseBytes(body), ""
}

// ── DeepSeek（仅余额） ─────────────────────────────────────

// balanceTierFromInfo 把一条 balance_infos 记录转为 balance tier。
// total/currency 任一为空时返回 nil（视为无效记录）；字符串原样保留上游精度。
func balanceTierFromInfo(info gjson.Result) *MonitorQuotaTier {
	total := info.Get("total_balance").String()
	currency := info.Get("currency").String()
	if total == "" || currency == "" {
		return nil
	}
	tier := &MonitorQuotaTier{Name: QuotaTierBalance, Balance: &total, Currency: &currency}
	if granted := info.Get("granted_balance").String(); granted != "" {
		tier.GrantedBalance = &granted
	}
	if toppedUp := info.Get("topped_up_balance").String(); toppedUp != "" {
		tier.ToppedUp = &toppedUp
	}
	return tier
}

func queryDeepseekBalance(ctx context.Context, endpoint, apiKey string) (*MonitorQuotaSnapshot, string, bool) {
	body, status, err := doCodingPlanGet(ctx, endpoint, codingPlanDeepseekBalancePath, apiKey, "bearer")
	if msg, auth := classifyCodingPlanHTTP(status, body, err); msg != "" {
		return nil, msg, auth
	}
	root, perr := parseCodingPlanJSON(body)
	if perr != "" {
		return nil, perr, false
	}

	available := root.Get("is_available").Bool()
	snap := &MonitorQuotaSnapshot{Available: &available}

	// 遍历全部 balance_infos：DeepSeek 账户可能同时持有 CNY/USD 多个币种子账户，
	// 每个有效（非空）币种各生成一条 balance tier；字符串原样保留上游精度。
	root.Get("balance_infos").ForEach(func(_, info gjson.Result) bool {
		if tier := balanceTierFromInfo(info); tier != nil {
			snap.Tiers = append(snap.Tiers, *tier)
		}
		return true
	})
	return snap, "", false
}

// ── 智谱 GLM ───────────────────────────────────────────────

// classifyGLMWindow 按 unit 字段判定 TOKENS_LIMIT 条目所属窗口。
// 实测形态（bigmodel.cn 与 z.ai 共用同一后端）：
//   - unit: 3 → 5 小时滚动窗口
//   - unit: 6 → 每周窗口
func classifyGLMWindow(unit int64) string {
	switch unit {
	case 3:
		return QuotaTierFiveHour
	case 6:
		return QuotaTierWeekly
	default:
		return ""
	}
}

func queryGLMQuota(ctx context.Context, endpoint, apiKey string) (*MonitorQuotaSnapshot, string, bool) {
	body, status, err := doCodingPlanGet(ctx, endpoint, codingPlanGLMQuotaPath, apiKey, "raw")
	if msg, auth := classifyCodingPlanHTTP(status, body, err); msg != "" {
		return nil, msg, auth
	}
	root, perr := parseCodingPlanJSON(body)
	if perr != "" {
		return nil, perr, false
	}

	// 业务级错误：{"code":..., "msg":"...", "success":false}
	if root.Get("success").Exists() && !root.Get("success").Bool() {
		msg := root.Get("msg").String()
		if msg == "" {
			msg = "unknown error"
		}
		return nil, "API error: " + msg, false
	}

	data := root.Get("data")
	if !data.Exists() {
		return nil, "missing 'data' field in response", false
	}

	snap := &MonitorQuotaSnapshot{PlanLevel: data.Get("level").String()}
	var fiveHour, weekly *MonitorQuotaTier

	data.Get("limits").ForEach(func(_, item gjson.Result) bool {
		// 兼容 issue #6153：type 已从 TOKENS_LIMIT 变更为 CREDIT_LIMIT，两者都识别。
		t := item.Get("type").String()
		if !strings.EqualFold(t, "TOKENS_LIMIT") && !strings.EqualFold(t, "CREDIT_LIMIT") {
			return true
		}
		pct := item.Get("percentage").Float()
		tier := &MonitorQuotaTier{Utilization: &pct}
		if ms := item.Get("nextResetTime"); ms.Exists() && ms.Int() > 0 {
			ts := millisToTime(ms.Int())
			tier.ResetsAt = &ts
		}
		switch classifyGLMWindow(item.Get("unit").Int()) {
		case QuotaTierFiveHour:
			if fiveHour == nil {
				tier.Name = QuotaTierFiveHour
				fiveHour = tier
			}
		case QuotaTierWeekly:
			if weekly == nil {
				tier.Name = QuotaTierWeekly
				weekly = tier
			}
		}
		return true
	})

	for _, slot := range []*MonitorQuotaTier{fiveHour, weekly} {
		if slot != nil {
			snap.Tiers = append(snap.Tiers, *slot)
		}
	}
	return snap, "", false
}

// ── Kimi For Coding ────────────────────────────────────────

func queryKimiUsage(ctx context.Context, endpoint, apiKey string) (*MonitorQuotaSnapshot, string, bool) {
	body, status, err := doCodingPlanGet(ctx, endpoint, codingPlanKimiUsagesPath, apiKey, "bearer")
	if msg, auth := classifyCodingPlanHTTP(status, body, err); msg != "" {
		return nil, msg, auth
	}
	root, perr := parseCodingPlanJSON(body)
	if perr != "" {
		return nil, perr, false
	}

	snap := &MonitorQuotaSnapshot{}

	// 5 小时窗口限额：limits[].detail {limit, remaining, resetTime}
	root.Get("limits").ForEach(func(_, item gjson.Result) bool {
		detail := item.Get("detail")
		if !detail.Exists() {
			return true
		}
		if tier := kimiTierFromLimit(detail, QuotaTierFiveHour); tier != nil {
			snap.Tiers = append(snap.Tiers, *tier)
		}
		return true
	})

	// 总体用量（周限额）：usage {limit, remaining, resetTime}
	if usage := root.Get("usage"); usage.Exists() {
		if tier := kimiTierFromLimit(usage, QuotaTierWeekly); tier != nil {
			snap.Tiers = append(snap.Tiers, *tier)
		}
	}
	return snap, "", false
}

// kimiTierFromLimit 从 {limit, remaining, resetTime} 结构计算 utilization 百分比。
func kimiTierFromLimit(v gjson.Result, name string) *MonitorQuotaTier {
	limit := v.Get("limit").Float()
	remaining := v.Get("remaining").Float()
	if limit <= 0 {
		return nil
	}
	used := limit - remaining
	if used < 0 {
		used = 0
	}
	util := used / limit * 100
	tier := &MonitorQuotaTier{Name: name, Utilization: &util}
	if ts := kimiResetTime(v.Get("resetTime")); ts != nil {
		tier.ResetsAt = ts
	}
	return tier
}

// kimiResetTime 兼容字符串（ISO8601）与数字（秒/毫秒）重置时间。
func kimiResetTime(v gjson.Result) *time.Time {
	if !v.Exists() {
		return nil
	}
	if s := v.String(); s != "" && v.Type == gjson.String {
		if ts, err := time.Parse(time.RFC3339, s); err == nil {
			return &ts
		}
		return nil
	}
	n := v.Int()
	if n <= 0 {
		return nil
	}
	ts := millisToTime(n)
	return &ts
}

// millisToTime 把时间戳（自动区分秒/毫秒）转为 time.Time。
func millisToTime(n int64) time.Time {
	if n < 1_000_000_000_000 {
		n *= 1000
	}
	return time.UnixMilli(n).UTC()
}
