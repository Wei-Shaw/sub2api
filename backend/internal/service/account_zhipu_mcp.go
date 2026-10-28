package service

import (
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ZhipuMCPCapabilityExtraKey 智谱账号远程 MCP Server 转发能力开关（accounts.extra.zhipu_mcp_enabled）。
const ZhipuMCPCapabilityExtraKey = "zhipu_mcp_enabled"

// IsZhipuMCPCapable 返回智谱账号是否已开启 MCP 转发能力。
// 仅 Coding Plan 订阅模式的智谱账号可开启；字段缺失或类型不正确按 false（关闭）处理。
func (a *Account) IsZhipuMCPCapable() bool {
	if a == nil || a.Platform != PlatformZhipu || a.Extra == nil {
		return false
	}
	if a.GetAccountMode() != AccountModeCoding {
		return false
	}
	enabled, ok := a.Extra[ZhipuMCPCapabilityExtraKey].(bool)
	return ok && enabled
}

// zhipuMCPCredentialsAccountMode 从原始 credentials map 读取归一化后的 account_mode，
// 语义与 Account.GetAccountMode 一致（trim 后仅接受 payg / coding，其余视为空）。
func zhipuMCPCredentialsAccountMode(credentials map[string]any) string {
	if credentials == nil {
		return ""
	}
	mode, _ := credentials["account_mode"].(string)
	mode = strings.TrimSpace(mode)
	if mode == AccountModePayG || mode == AccountModeCoding {
		return mode
	}
	return ""
}

// ValidateZhipuMCPExtra 校验智谱账号 MCP 转发开关：仅 platform=zhipu 且提供该 key 时生效。
// 置 true 要求账号处于 Coding Plan 订阅模式（credentials["account_mode"] = coding）。
func ValidateZhipuMCPExtra(platform string, credentials map[string]any, extra map[string]any) error {
	if platform != PlatformZhipu {
		return nil
	}
	raw, exists := extra[ZhipuMCPCapabilityExtraKey]
	if !exists {
		return nil
	}
	enabled, ok := raw.(bool)
	if !ok {
		return infraerrors.BadRequest(
			"ZHIPU_MCP_ENABLED_INVALID",
			"zhipu_mcp_enabled must be a boolean",
		)
	}
	if enabled && zhipuMCPCredentialsAccountMode(credentials) != AccountModeCoding {
		return infraerrors.BadRequest(
			"ZHIPU_MCP_NOT_CODING_PLAN",
			"仅 Coding Plan 订阅模式的智谱账号可开启 MCP 转发",
		)
	}
	return nil
}

// normalizeZhipuMCPUpdateExtra 处理账号更新时的 zhipu_mcp_enabled。
// Extra 在 update 时整体替换：未提供该 key 时保留账号现值，避免普通编辑误关已开启的开关。
// account_mode 可能同时被新 credentials 覆盖，校验按"新 credentials 里的 account_mode 优先，
// 否则用账号现值"判定。
func normalizeZhipuMCPUpdateExtra(account *Account, input *UpdateAccountInput, normalized map[string]any) (map[string]any, error) {
	if account == nil || account.Platform != PlatformZhipu {
		return normalized, nil
	}
	credentials := account.Credentials
	if len(input.Credentials) > 0 {
		if mode := zhipuMCPCredentialsAccountMode(input.Credentials); mode != "" {
			credentials = input.Credentials
		}
	}
	if err := ValidateZhipuMCPExtra(account.Platform, credentials, input.Extra); err != nil {
		return nil, err
	}
	if normalized == nil {
		normalized = make(map[string]any)
	} else {
		normalized = shallowCopyMap(normalized)
	}
	if _, provided := input.Extra[ZhipuMCPCapabilityExtraKey]; !provided {
		if current, ok := account.Extra[ZhipuMCPCapabilityExtraKey].(bool); ok {
			normalized[ZhipuMCPCapabilityExtraKey] = current
		}
	}
	return normalized, nil
}
