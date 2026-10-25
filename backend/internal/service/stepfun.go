package service

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const (
	// DefaultStepFunTestModel 是连接测试的默认模型；Step Plan 与 payg 均可用。
	DefaultStepFunTestModel = "step-3.5-flash"

	// StepFunUsageAutoRefreshExtraKey 控制 Step Plan 账号是否参与周期用量探测。
	// 字段缺失按开启处理，保持接入账号级开关前的既有行为。
	StepFunUsageAutoRefreshExtraKey = "stepfun_usage_auto_refresh"
	// StepFunUsageSnapshotExtraKey 保存最近一次 Step Plan 用量探测的完整展示状态。
	StepFunUsageSnapshotExtraKey = "stepfun_usage_snapshot"
	// StepFunUsageAttemptSeqExtraKey 保存数据库分配的最后一次探测序号。
	// 探测开始时先取序号，完成写入时只允许更大序号覆盖，避免跨实例时钟漂移。
	StepFunUsageAttemptSeqExtraKey = "stepfun_usage_attempt_seq"

	StepFunUsageStatusOK          = "ok"
	StepFunUsageStatusFailed      = "failed"
	StepFunUsageStatusUnavailable = "unavailable"
)

// stepPlanManagedExtraKeys 是 Step Plan 探测/刷新服务独占的运行态字段。
// 普通账号编辑、创建、批量更新和 key 级 Extra 合并都必须剥离客户端值。
var stepPlanManagedExtraKeys = []string{
	StepFunUsageAutoRefreshExtraKey,
	StepFunUsageSnapshotExtraKey,
	StepFunUsageAttemptSeqExtraKey,
	cnExtraKey(PlatformStepfun, cnExtraSuffix5hUsed),
	cnExtraKey(PlatformStepfun, cnExtraSuffix5hReset),
	cnExtraKey(PlatformStepfun, cnExtraSuffixWeeklyUsed),
	cnExtraKey(PlatformStepfun, cnExtraSuffixWeeklyReset),
	cnExtraKey(PlatformStepfun, cnExtraSuffixMonthlyUsed),
	cnExtraKey(PlatformStepfun, cnExtraSuffixMonthlyReset),
	cnExtraKey(PlatformStepfun, cnExtraSuffixUsageUpdated),
}

func deleteStepPlanManagedExtraKeys(extra map[string]any) {
	if extra == nil {
		return
	}
	for _, key := range stepPlanManagedExtraKeys {
		delete(extra, key)
	}
}

func stepFunUsageIdentity(account *Account) map[string]any {
	if account == nil || account.GetCodingPlanProvider() != PlatformStepfun {
		return nil
	}
	return map[string]any{
		"api_key":  account.GetCNAPIKey(),
		"base_url": account.GetOpenAIBaseURL(),
	}
}

// StepFunUsageSnapshot 是 Step Plan 最近一次探测的结构化快照。平铺的
// stepfun_5h/weekly/monthly_* 键继续供调度阈值使用，本结构供账号编辑面板
// 展示状态、套餐、失败原因和最近尝试时间。
type StepFunUsageSnapshot struct {
	Status          string        `json:"status"`
	CredentialValid bool          `json:"credential_valid"`
	PlanLevel       string        `json:"plan_level,omitempty"`
	Tiers           []CNQuotaTier `json:"tiers,omitempty"`
	FetchedAt       *time.Time    `json:"fetched_at,omitempty"`
	LastAttemptAt   time.Time     `json:"last_attempt_at"`
	HTTPStatus      int           `json:"http_status,omitempty"`
	LastError       string        `json:"last_error,omitempty"`
}

// StepPlanUsageState 是账号编辑页使用的 Step Plan 用量状态。
type StepPlanUsageState struct {
	AccountID          int64                 `json:"account_id"`
	Eligible           bool                  `json:"eligible"`
	AutoRefreshEnabled bool                  `json:"auto_refresh_enabled"`
	Snapshot           *StepFunUsageSnapshot `json:"snapshot,omitempty"`
}

func newStepFunUsageSnapshot(result *CNProviderQuotaProbeResult, now time.Time) StepFunUsageSnapshot {
	snapshot := StepFunUsageSnapshot{
		Status:          StepFunUsageStatusFailed,
		LastAttemptAt:   now.UTC(),
		CredentialValid: result != nil && result.CredentialValid,
	}
	if result == nil {
		return snapshot
	}
	if result.Success {
		snapshot.Status = StepFunUsageStatusOK
		fetchedAt := now.UTC()
		snapshot.FetchedAt = &fetchedAt
	} else if result.CredentialValid && result.StatusCode >= 200 && result.StatusCode < 300 {
		// 2xx 只说明鉴权通过；业务响应明确表示套餐/额度不可用时才标 unavailable。
		snapshot.Status = StepFunUsageStatusUnavailable
	}
	snapshot.PlanLevel = result.PlanLevel
	snapshot.HTTPStatus = result.StatusCode
	snapshot.LastError = result.Error
	if len(result.Tiers) > 0 {
		snapshot.Tiers = append([]CNQuotaTier(nil), result.Tiers...)
	}
	return snapshot
}

// stepFunUsageAutoRefreshEnabled 返回 Step Plan 账号级自动刷新开关；缺失时默认开启。
func stepFunUsageAutoRefreshEnabled(account *Account) bool {
	if account == nil || account.Extra == nil {
		return true
	}
	enabled, ok := account.Extra[StepFunUsageAutoRefreshExtraKey].(bool)
	if !ok {
		return true
	}
	return enabled
}

// stepFunQuotaUnavailable 判定 Step Plan 订阅额度是否不可用，语义照搬
// cc-switch 的 usage_script extractor：
//
//	quota.state 必须恰为 "known"；
//	credit_buckets 与 windows 不能同时为空；
//	quota.allowed 为 false 同样视为不可用。
//
// 返回空串表示可用，否则为可直接展示的原因。HTTP 200 只证明 Key 有效，
// 不代表账号开着 Step Plan。
func stepFunQuotaUnavailable(body []byte) string {
	quota := gjson.GetBytes(body, "quota")
	if strings.TrimSpace(quota.Get("state").String()) != "known" {
		return stepFunQuotaUnavailableMessage(body)
	}
	if len(quota.Get("credit_buckets").Array()) == 0 && len(quota.Get("windows").Array()) == 0 {
		return stepFunQuotaUnavailableMessage(body)
	}
	if allowed := quota.Get("allowed"); allowed.Exists() && !allowed.Bool() {
		return stepFunQuotaUnavailableMessage(body)
	}
	return ""
}

// stepFunQuotaUnavailableMessage 复刻 cc-switch 的提示优先级：先取上游
// quota.reason，再按 plan.state 区分「有套餐但无额度数据」与「未开通」。
func stepFunQuotaUnavailableMessage(body []byte) string {
	if reason := strings.TrimSpace(gjson.GetBytes(body, "quota.reason").String()); reason != "" {
		return reason
	}
	if strings.TrimSpace(gjson.GetBytes(body, "plan.state").String()) == "known" {
		return "Step Plan quota has no usage data"
	}
	return "Step Plan is not activated on this account"
}

// isStepFunCodingBaseURL 只认 StepFun 官方域名下的 Step Plan 基址，避免把
// Step Plan Key 发往自定义中转。coding/v1（Chat）与 coding（Anthropic）
// 两种默认 base 都要识别。
func isStepFunCodingBaseURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(raw, "#") {
		return false
	}
	host := strings.ToLower(parsed.Host)
	if host != "api.stepfun.com" && host != "api.stepfun.ai" &&
		host != "api.stepfun.com:443" && host != "api.stepfun.ai:443" {
		return false
	}
	switch parsed.EscapedPath() {
	case "/step_plan", "/step_plan/", "/step_plan/v1", "/step_plan/v1/":
		return true
	default:
		return false
	}
}

// stepfunQuotaURL 从 Step Plan 的推理基址推导订阅用量端点。Chat 基址带
// /v1，Anthropic 基址不带，统一剥掉后拼 /v1/usage。
func stepfunQuotaURL(baseURL string) string {
	base := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(baseURL), "/"), "/v1")
	return base + "/v1/usage"
}

// stepFunQuotaExtraUpdates 在通用窗口快照之上显式清空本次成功响应未包含的
// Step Plan 窗口。否则旧 5h/weekly/monthly 键会跨套餐/窗口变更继续参与调度阈值。
func stepFunQuotaExtraUpdates(tiers []CNQuotaTier, now time.Time) map[string]any {
	updates := cnQuotaExtraUpdates(PlatformStepfun, tiers, now)
	present := make(map[string]CNQuotaTier, len(tiers))
	for _, tier := range tiers {
		present[tier.Window] = tier
	}
	for _, window := range []string{"5h", "weekly", "monthly"} {
		usedKey := cnExtraKey(PlatformStepfun, window+"_used_percent")
		resetKey := cnExtraKey(PlatformStepfun, window+"_reset_at")
		tier, ok := present[window]
		if !ok {
			updates[usedKey] = nil
			updates[resetKey] = nil
			continue
		}
		if tier.ResetAt == "" {
			updates[resetKey] = nil
		}
	}
	return updates
}

// parseStepFunUsageTiers 解析 GET /step_plan/v1/usage。
//
//	quota.credit_buckets[]  → Credit 月池（total/remaining/used_percent/expires_at_ms）
//	quota.windows[]         → 5h / weekly 滚动窗口（used_percent/resets_at_ms）
//	plan.name               → 套餐名（如 Plus）
//
// Credit 套餐通常没有 5h/weekly 窗口；存在 credits 时不再用同名的 monthly
// window 覆盖月池快照。
func parseStepFunUsageTiers(body []byte) ([]CNQuotaTier, string) {
	quota := gjson.GetBytes(body, "quota")
	planName := strings.TrimSpace(gjson.GetBytes(body, "plan.name").String())

	var tiers []CNQuotaTier
	hasMonthly := false

	if buckets := quota.Get("credit_buckets"); buckets.IsArray() {
		// used_percent 以上游为准（按 total 加权），缺失时才用 total/remaining
		// 反推——与 cc-switch 取 buckets[0].used_percent 一致。
		var total, remaining, percentWeight, percentSum float64
		var resetAt string
		count := 0
		allHaveUsedPercent := true
		buckets.ForEach(func(_, bucket gjson.Result) bool {
			bucketTotal, okTotal := cnParseF64(bucket.Get("total").Value())
			bucketRemaining, okRemaining := cnParseF64(bucket.Get("remaining").Value())
			if !okTotal || !okRemaining || bucketTotal <= 0 {
				return true
			}
			total += bucketTotal
			remaining += bucketRemaining
			if usedPercent, ok := cnParseF64(bucket.Get("used_percent").Value()); ok {
				percentWeight += bucketTotal
				percentSum += usedPercent * bucketTotal
			} else {
				allHaveUsedPercent = false
			}
			if resetAt == "" {
				resetAt = cnNormalizeResetTime(bucket.Get("expires_at_ms").Value())
			}
			count++
			return true
		})
		if count > 0 && total > 0 {
			usedPercent := (total - remaining) / total * 100
			// 只有所有有效 bucket 都带 used_percent 时才能信任加权百分比；
			// 混合缺失时按 total/remaining 统一反推，避免忽略部分 bucket。
			if allHaveUsedPercent && percentWeight > 0 {
				usedPercent = percentSum / percentWeight
			}
			if usedPercent < 0 {
				usedPercent = 0
			}
			tiers = append(tiers, CNQuotaTier{
				Window:      "monthly",
				UsedPercent: usedPercent,
				ResetAt:     resetAt,
			})
			hasMonthly = true
		}
	}

	if windows := quota.Get("windows"); windows.IsArray() {
		windows.ForEach(func(key, window gjson.Result) bool {
			name := stepFunWindowLabel(window.Get("name").String(), window.Get("type").String(), int(key.Int()))
			// credit_buckets 已产出 monthly 档时，跳过同名的 monthly 窗口。
			if name == "monthly" && hasMonthly {
				return true
			}
			usedPercent, ok := cnParseF64(window.Get("used_percent").Value())
			if !ok {
				return true
			}
			if usedPercent < 0 {
				usedPercent = 0
			}
			tiers = append(tiers, CNQuotaTier{
				Window:      name,
				UsedPercent: usedPercent,
				ResetAt:     cnNormalizeResetTime(window.Get("resets_at_ms").Value()),
			})
			return true
		})
	}

	return tiers, planName
}

// stepFunWindowLabel 把上游窗口名归一到 sub2api 的 5h / weekly / monthly
// （调度阈值与 Extra 快照只认这三个）。识别不出时保留原始名的净化形式，
// 对应 cc-switch 的 name || type || "窗口N" 回退，不静默丢窗口；这类标签
// 只参与展示，cnQuotaExtraUpdates 会忽略它们。
func stepFunWindowLabel(name, typ string, index int) string {
	raw := strings.ToLower(strings.TrimSpace(name + " " + typ))
	switch {
	case strings.Contains(raw, "5h"), strings.Contains(raw, "5_hour"), strings.Contains(raw, "five"):
		return "5h"
	case strings.Contains(raw, "week"), strings.Contains(raw, "7d"):
		return "weekly"
	case strings.Contains(raw, "month"), strings.Contains(raw, "credit"):
		return "monthly"
	}
	if label := sanitizeStepFunWindowLabel(name); label != "" {
		return label
	}
	if label := sanitizeStepFunWindowLabel(typ); label != "" {
		return label
	}
	return fmt.Sprintf("window_%d", index+1)
}

func sanitizeStepFunWindowLabel(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			_, _ = b.WriteRune(r)
		case r == ' ', r == '-', r == '_', r == '.':
			_ = b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}
