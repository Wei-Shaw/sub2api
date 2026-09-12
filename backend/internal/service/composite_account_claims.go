package service

import "strings"

// CompositeClaimStrength 表达账号对某模型的声明强度。ownership 据此分层：
// 存在强声明平台时池/单平台仅由强声明平台构成，完全无强声明才回退通配命中
// 平台集合，避免 catch-all 通配账号与显式声明平台混池（如 grok 精确别名被
// openai 的 "*" 通配冒领改写模型）。
type CompositeClaimStrength int

const (
	// CompositeClaimNone 无声明。
	CompositeClaimNone CompositeClaimStrength = iota
	// CompositeClaimWildcard 弱声明：model_mapping 通配命中（fallback 语义，
	// 仅在无任何强声明平台时参与池/单平台判定）。
	CompositeClaimWildcard
	// CompositeClaimExplicit 强声明：mapping 精确键命中（含平台既有归一化后的
	// 精确键），或受控 native（空 mapping 且 detector 平台一致且 IsModelSupported
	// 通过）。精确与受控 native 同为强等级。
	CompositeClaimExplicit
)

// CompositeAccountClaimStrength 返回 account 在「配置态」对 model 的声明强度。
// 只依赖持久化配置（mapping / 平台 / 目录），不读取限流等瞬态运行状态。
func CompositeAccountClaimStrength(account *Account, model string) CompositeClaimStrength {
	if account == nil {
		return CompositeClaimNone
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return CompositeClaimNone
	}

	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		// 规则B：空 mapping 仅受控 native 声明。OpenAI OAuth 空 mapping 仍受
		// isOpenAIOAuthServableModel 约束（IsModelSupported 内部处理）。
		if platform, ok := DetectModelPlatform(model); !ok || platform != account.Platform {
			return CompositeClaimNone
		}
		if account.IsModelSupported(model) {
			return CompositeClaimExplicit
		}
		return CompositeClaimNone
	}
	return compositeMappingClaimStrength(account, mapping, model)
}

// compositeMappingClaimStrength 按账号映射的实际命中路径分级，与
// ResolveMappedModel 的匹配顺序一致：原始模型精确 → 原始通配 → 归一化精确 →
// 归一化通配；命中即终止。映射目标为空的条目沿用既有 strict claim 语义：不构
// 成声明，也不回落通配（显式条目对通配具有遮蔽性，转发阶段同样走不到通配）。
func compositeMappingClaimStrength(account *Account, mapping map[string]string, model string) CompositeClaimStrength {
	if mapped, exists := mapping[model]; exists {
		return compositeClaimStrengthForTarget(mapped, true)
	}
	if mapped, matched := matchWildcardMappingResult(mapping, model); matched {
		return compositeClaimStrengthForTarget(mapped, false)
	}
	normalized := normalizeRequestedModelForLookup(account.Platform, model)
	if normalized != model {
		if mapped, exists := mapping[normalized]; exists {
			return compositeClaimStrengthForTarget(mapped, true)
		}
		if mapped, matched := matchWildcardMappingResult(mapping, normalized); matched {
			return compositeClaimStrengthForTarget(mapped, false)
		}
	}
	return CompositeClaimNone
}

func compositeClaimStrengthForTarget(mapped string, exact bool) CompositeClaimStrength {
	if strings.TrimSpace(mapped) == "" {
		return CompositeClaimNone
	}
	if exact {
		return CompositeClaimExplicit
	}
	return CompositeClaimWildcard
}

// CompositeAccountClaimsModel 报告 account 在「配置态」是否能提供 model，是
// composite 候选平台选择期与后续构建期共用的统一能力谓词（任意强度命中均为
// true：通配账号在其平台入池后仍可被 selector 选中，与转发阶段映射一致）。
//
// 规则（评审收敛版）：
//   - A. 有效 model_mapping 非空：精确 / 既有通配（最长优先，含平台既有归一化
//     回退）命中且映射目标非空才可 claim；未命中不 native fallback。
//     Grok / Antigravity / Gemini GoogleOne 的默认映射经 GetModelMapping 生效，
//     归入本规则，与转发阶段账号级映射保持一致。
//   - B. 有效 model_mapping 为空：仅当 DetectModelPlatform(model) 与账号平台
//     一致且既有 IsModelSupported 通过才可 claim。opencode_go 无 detector 分支，
//     空 mapping 永不入池，仅显式 mapping 命中可入池。
//
// 空 mapping 账号不得凭 IsModelSupported 的 allow-all 语义冒领其他平台的模型。
// ownership 的池构成按 CompositeAccountClaimStrength 分层：有强声明平台时仅强
// 声明平台入池，完全无强声明才使用通配命中平台集合。
func CompositeAccountClaimsModel(account *Account, model string) bool {
	return CompositeAccountClaimStrength(account, model) != CompositeClaimNone
}
