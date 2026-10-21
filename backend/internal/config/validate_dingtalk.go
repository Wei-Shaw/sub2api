// Package config 包含钉钉连接配置的校验逻辑。
//
// internal_only 模式安全模型（方案 A）：
// 不再要求 admin 填写 InternalCorpID 做二次 corpID 比对。
// 安全边界由钉钉"企业内部应用"类型本身保证——只有应用所属企业的员工才能完成 OAuth，
// 因此 ValidateDingTalkConfig 只要求 app_type=internal（V1），不再要求 InternalCorpID 非空（原 V3 已删除）。
// InternalCorpID 字段保留，admin 可选填；若填写，checkDingTalkCorpAllowed 不会使用它做约束。
package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrDingTalkV1AppTypeMismatch = errors.New("dingtalk: internal_only requires app_type=internal")
	ErrDingTalkV4InvalidAppKind  = errors.New("dingtalk: dingtalk_app_kind must be internal_app")
)

func ValidateDingTalkConfig(cfg DingTalkConnectConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.DingTalkAppKind != "internal_app" {
		return ErrDingTalkV4InvalidAppKind
	}
	if cfg.CorpRestrictionPolicy == "internal_only" {
		if cfg.AppType != "internal" {
			return ErrDingTalkV1AppTypeMismatch
		}
	}
	return ValidateDingTalkApps(cfg.Apps)
}

// ValidateDingTalkApps also validates disabled entries so enabling them cannot
// unexpectedly expose invalid redirect URLs or ambiguous application IDs.
func ValidateDingTalkApps(apps []DingTalkAppConfig) error {
	if len(apps) > 50 {
		return errors.New("dingtalk: at most 50 applications are allowed")
	}
	seen := map[string]bool{}
	clients := map[string]bool{}
	for _, app := range apps {
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`).MatchString(app.ID) || app.ID == "default" {
			return errors.New("dingtalk: invalid or reserved application ID")
		}
		if seen[app.ID] || clients[app.ClientID] {
			return errors.New("dingtalk: duplicate application ID or client ID")
		}
		seen[app.ID], clients[app.ClientID] = true, true
		if strings.TrimSpace(app.Name) == "" || strings.TrimSpace(app.ClientID) == "" || strings.TrimSpace(app.ClientSecret) == "" {
			return fmt.Errorf("dingtalk: name and credentials are required for %s", app.ID)
		}
		if err := ValidateAbsoluteHTTPURL(app.RedirectURL); err != nil {
			return fmt.Errorf("dingtalk: invalid redirect URL for %s", app.ID)
		}
	}
	return nil
}
