package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// ManagedLinkRecomputer 可选依赖：owner 账号 plan 变更后重算 managed 分组链接。
// *AccountGroupRecomputer 实现此接口；nil 时 ApplyProbedPlan 跳过 recompute。
type ManagedLinkRecomputer interface {
	RecomputeManagedLinks(ctx context.Context, account *Account) error
}

// NormalizeUpstreamPlanFromProbe 将平台探测到的原始套餐字符串映射为 groups/accounts.upstream_plan code。
// 未知或不支持的 raw → 空串（无法匹配共享池）。
// 与 DefaultGroupUpstreamPlansSeed 的 code 体系对齐，并复用 NormalizeUpstreamPlanCode。
func NormalizeUpstreamPlanFromProbe(platform, raw string) string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	raw = strings.TrimSpace(raw)
	if platform == "" || raw == "" {
		return ""
	}

	var code string
	switch platform {
	case PlatformOpenAI:
		code = NormalizeUpstreamPlanCode(raw)
	case PlatformGrok:
		code = normalizeGrokUpstreamPlanFromProbe(raw)
	case PlatformAntigravity:
		code = normalizeAntigravityUpstreamPlanFromProbe(raw)
	case PlatformAnthropic, PlatformGemini:
		// v1 无稳定 probe 映射；仅当 raw 已是种子 code（当前种子为空）时才保留
		code = NormalizeUpstreamPlanCode(raw)
	default:
		return ""
	}

	if code == "" || !isSeedUpstreamPlanCode(platform, code) {
		return ""
	}
	return code
}

func normalizeGrokUpstreamPlanFromProbe(raw string) string {
	code := NormalizeUpstreamPlanCode(raw)
	// 去掉空格/下划线/连字符后再匹配常见变体
	compact := strings.NewReplacer(" ", "", "_", "", "-", "").Replace(code)
	switch compact {
	case "supergrok":
		return "supergrok"
	case "supergrokheavy":
		return "supergrokheavy"
	case "free", "basic":
		return compact
	default:
		return code
	}
}

func normalizeAntigravityUpstreamPlanFromProbe(raw string) string {
	code := NormalizeUpstreamPlanCode(raw)
	switch code {
	case "free", "free_tier", "freetier":
		return "free-tier"
	case "pro", "g1_pro", "g1_pro_tier", "g1pro", "g1pro-tier":
		return "g1-pro-tier"
	case "ultra", "g1_ultra", "g1_ultra_tier", "g1ultra", "g1ultra-tier":
		return "g1-ultra-tier"
	case "abnormal":
		return ""
	default:
		return code
	}
}

func isSeedUpstreamPlanCode(platform, code string) bool {
	seed := DefaultGroupUpstreamPlansSeed()
	opts, ok := seed[platform]
	if !ok {
		return false
	}
	for _, o := range opts {
		if o.Code == code {
			return true
		}
	}
	return false
}

// planCredentialKey 返回该平台 credentials 中用于审计的 plan 字段名。
func planCredentialKey(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformGrok:
		return "subscription_tier"
	default:
		return "plan_type"
	}
}

// ExtractProbedPlanRaw 从账号 credentials 提取可用于 ApplyProbedPlan 的原始 plan 串。
func ExtractProbedPlanRaw(account *Account) string {
	if account == nil {
		return ""
	}
	key := planCredentialKey(account.Platform)
	if v := strings.TrimSpace(account.GetCredential(key)); v != "" {
		return v
	}
	// OpenAI 兼容：credentials 可能只有 chatgpt_plan_type
	if strings.EqualFold(account.Platform, PlatformOpenAI) {
		if v := strings.TrimSpace(account.GetCredential("chatgpt_plan_type")); v != "" {
			return v
		}
	}
	// 其它平台兜底 plan_type
	if key != "plan_type" {
		return strings.TrimSpace(account.GetCredential("plan_type"))
	}
	return ""
}

// ApplyProbedPlan 是 accounts.upstream_plan 与 credentials 中 plan 类字段的唯一持久化入口（K16）。
// 1) Normalize(platform, raw) → code（未知为空）
// 2) 更新 credentials 审计键 + accounts.upstream_plan 列
// 3) 若 owner_user_id != nil 则 RecomputeManagedLinks
// recomputer 可为 nil（系统号路径或未注入时跳过 recompute；Recomputer 自身也对 owner 空 no-op）。
func ApplyProbedPlan(ctx context.Context, accountRepo AccountRepository, recomputer ManagedLinkRecomputer, account *Account, rawPlan string) error {
	if accountRepo == nil || account == nil {
		return nil
	}
	// spark 影子不持凭据；plan 由母账号维护
	if account.IsCredentialShadow() {
		return nil
	}

	rawPlan = strings.TrimSpace(rawPlan)
	code := NormalizeUpstreamPlanFromProbe(account.Platform, rawPlan)
	currentCode := NormalizeUpstreamPlanCode(account.UpstreamPlan)

	credKey := planCredentialKey(account.Platform)
	// 审计：优先保留探测原串；原串空则写规范化 code（便于空探测清字段时同步）
	auditVal := rawPlan
	if auditVal == "" {
		auditVal = code
	}
	currentCred := strings.TrimSpace(account.GetCredential(credKey))

	planUnchanged := currentCode == code
	credUnchanged := auditVal == "" && currentCred == "" || strings.EqualFold(currentCred, auditVal)
	if planUnchanged && credUnchanged {
		return nil
	}

	// 内存态先更新，再落库
	if account.Credentials == nil {
		account.Credentials = make(map[string]any, 1)
	}
	if auditVal == "" {
		delete(account.Credentials, credKey)
	} else {
		account.Credentials[credKey] = auditVal
	}
	account.UpstreamPlan = code

	credsPatch := map[string]any{}
	if auditVal == "" {
		// BulkUpdate 使用 JSONB || 合并，无法删除键；空审计值时走完整 Update 覆盖 credentials
		if err := accountRepo.Update(ctx, account); err != nil {
			return fmt.Errorf("apply probed plan update account=%d: %w", account.ID, err)
		}
	} else {
		credsPatch[credKey] = auditVal
		planCopy := code
		if _, err := accountRepo.BulkUpdate(ctx, []int64{account.ID}, AccountBulkUpdate{
			Credentials:  credsPatch,
			UpstreamPlan: &planCopy,
		}); err != nil {
			return fmt.Errorf("apply probed plan bulk update account=%d: %w", account.ID, err)
		}
	}

	if account.OwnerUserID != nil && *account.OwnerUserID > 0 && recomputer != nil {
		if err := recomputer.RecomputeManagedLinks(ctx, account); err != nil {
			return fmt.Errorf("apply probed plan recompute account=%d: %w", account.ID, err)
		}
	}

	slog.Info("upstream_plan_applied",
		"account_id", account.ID,
		"platform", account.Platform,
		"raw", rawPlan,
		"upstream_plan", code,
		"previous_upstream_plan", currentCode,
	)
	return nil
}

// ApplyProbedPlanFromCredentials 在 OAuth/凭证已写入 plan 类字段后同步列 + recompute。
// raw 为空则 no-op（不因缺少探测结果清空已有 plan）。
func ApplyProbedPlanFromCredentials(ctx context.Context, accountRepo AccountRepository, recomputer ManagedLinkRecomputer, account *Account) error {
	raw := ExtractProbedPlanRaw(account)
	if raw == "" {
		return nil
	}
	return ApplyProbedPlan(ctx, accountRepo, recomputer, account, raw)
}
