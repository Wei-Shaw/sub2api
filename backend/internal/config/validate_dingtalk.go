// Package config 包含钉钉连接配置的校验逻辑。
//
// internal_only 模式安全模型（方案 A）：
// 不再要求 admin 填写 InternalCorpID 做二次 corpID 比对。
// 安全边界由钉钉"企业内部应用"类型本身保证——只有应用所属企业的员工才能完成 OAuth，
// 因此 ValidateDingTalkConfig 只要求 app_type=internal（V1），不再要求 InternalCorpID 非空（原 V3 已删除）。
// InternalCorpID 字段保留，admin 可选填；若填写，checkDingTalkCorpAllowed 不会使用它做约束。
package config

import "errors"

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
	return nil
}
