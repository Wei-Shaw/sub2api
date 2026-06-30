// Package service ...
//
// oidc_provider_settings.go 集中声明 sub2api 作为 OIDC Provider (`oidc_provider.*`)
// 子系统的所有 admin 可配置 setting key、默认值与值校验函数。
//
// 与 [domain_constants.go] 中的 `SettingKeyOIDCConnect*` 是**两套完全独立**的系统：
//   - `OIDCConnect*`：sub2api 作为客户端 (RP) 接入外部 OIDC 提供者用于登录
//   - `OidcProvider*`(本文件)：sub2api 作为服务端 (OP) 对第三方 RP 颁发身份令牌
//
// 已存在的 [SettingKeyOidcSigningKeyActiveKid] (位于 oidc_signing_service.go) 也属于
// 本命名空间，但因签名服务自包含,出于"绝不动 Stage 1A 已落地代码"的稳态原则保留原位。
package service

import (
	"errors"
	"fmt"
	"strings"
)

// ─── Setting Keys ────────────────────────────────────────────────────────────
//
// 8 个 admin 可配置项 (D9 + spec.md "Admin Configuration Settings")。
// 命名遵循 `oidc_provider.<snake_case>` 模式，与 1A 已落地的
// `oidc_provider.signing_key_active_kid` / `oidc_provider.signing_key.<kid>` 系列对齐。
const (
	// SettingKeyOidcProviderEnabled 主开关。false 时 6 个 OIDC 端点 + discovery 全 404。
	SettingKeyOidcProviderEnabled = "oidc_provider.enabled"

	// SettingKeyOidcProviderIssuerURL OIDC issuer URL，形如 "https://api.sub2api.com"。
	// 启动时若 enabled=true 但本项为空 → fatal。
	SettingKeyOidcProviderIssuerURL = "oidc_provider.issuer_url"

	// SettingKeyOidcProviderAccessTokenTTLSeconds OIDC access token (opaque) TTL，秒。
	SettingKeyOidcProviderAccessTokenTTLSeconds = "oidc_provider.access_token_ttl_seconds"

	// SettingKeyOidcProviderIDTokenTTLSeconds ID Token (RS256 JWT) TTL，秒。
	SettingKeyOidcProviderIDTokenTTLSeconds = "oidc_provider.id_token_ttl_seconds"

	// SettingKeyOidcProviderRefreshTokenTTLSeconds refresh token (opaque) TTL，秒。
	SettingKeyOidcProviderRefreshTokenTTLSeconds = "oidc_provider.refresh_token_ttl_seconds"

	// SettingKeyOidcProviderCodeTTLSeconds 授权码 (opaque) TTL，秒。
	SettingKeyOidcProviderCodeTTLSeconds = "oidc_provider.code_ttl_seconds"

	// SettingKeyOidcProviderSSOCookieMaxAgeSeconds sub2api_sso cookie 的 Max-Age，秒。
	SettingKeyOidcProviderSSOCookieMaxAgeSeconds = "oidc_provider.sso_cookie_max_age_seconds"

	// SettingKeyOidcProviderSSOCookieDomain sub2api_sso cookie 的 Domain 属性。
	// 空字符串表示 host-only cookie。形如 ".sub2api.com" (含前导点用于跨子域)。
	SettingKeyOidcProviderSSOCookieDomain = "oidc_provider.sso_cookie_domain"
)

// ─── 默认值 ──────────────────────────────────────────────────────────────────
//
// 所有 setting 缺省时 service 层应使用本组默认值（与 spec.md 中
// "Defaults applied when settings are unset" 场景严格对齐）。
const (
	// DefaultOidcProviderAccessTokenTTLSeconds 默认 1 小时。
	DefaultOidcProviderAccessTokenTTLSeconds = 3600
	// DefaultOidcProviderIDTokenTTLSeconds 默认 1 小时。
	DefaultOidcProviderIDTokenTTLSeconds = 3600
	// DefaultOidcProviderRefreshTokenTTLSeconds 默认 30 天。
	DefaultOidcProviderRefreshTokenTTLSeconds = 30 * 24 * 60 * 60 // 2592000
	// DefaultOidcProviderCodeTTLSeconds 默认 10 分钟。
	DefaultOidcProviderCodeTTLSeconds = 600
	// DefaultOidcProviderSSOCookieMaxAgeSeconds 默认 30 天，与 refresh TTL 一致。
	DefaultOidcProviderSSOCookieMaxAgeSeconds = 30 * 24 * 60 * 60 // 2592000
	// DefaultOidcProviderEnabled 主开关默认关闭，符合"发版即灰度"策略 (D14)。
	DefaultOidcProviderEnabled = false
	// DefaultOidcProviderSSOCookieDomain 默认空 (host-only)。
	DefaultOidcProviderSSOCookieDomain = ""
)

// AllowedOidcProviderScopes 是 sub2api 作为 OP 全局支持的 scope 集合。
// 每个 client 的 allowed_scopes 必须是本集合的子集。
//
// 与 spec.md / Stage 1B-2 的 client 创建校验对齐。
var AllowedOidcProviderScopes = []string{
	"openid",
	"profile",
	"email",
	"offline_access",
	"sub2api:apikey",
}

// ─── 错误哨兵 ────────────────────────────────────────────────────────────────

// ErrOidcProviderIssuerURLEmpty issuer_url 必填校验失败 (空字符串)。
var ErrOidcProviderIssuerURLEmpty = errors.New("oidc provider: issuer_url must be set")

// ErrOidcProviderIssuerURLNotHTTPS issuer_url 必须以 https:// 开头。
var ErrOidcProviderIssuerURLNotHTTPS = errors.New("oidc provider: issuer_url must start with https://")

// ErrOidcProviderIssuerURLTrailingSlash issuer_url 不能以 / 结尾。
var ErrOidcProviderIssuerURLTrailingSlash = errors.New("oidc provider: issuer_url must not end with /")

// ErrOidcProviderIssuerURLContainsQueryOrFragment issuer_url 不能包含 ? 或 #。
var ErrOidcProviderIssuerURLContainsQueryOrFragment = errors.New("oidc provider: issuer_url must not contain query string or fragment")

// ErrOidcProviderInvalidTTL 某个 TTL 设置项不是正整数。
var ErrOidcProviderInvalidTTL = errors.New("oidc provider: ttl must be a positive integer")

// ─── 校验函数 ────────────────────────────────────────────────────────────────

// ValidateOidcIssuerURL 严格校验 issuer_url 字面值 (与 spec.md "issuer_url format
// is enforced" 场景一一对应)。
//
// 校验规则：
//
//  1. 必须以 "https://" 前缀开头 (允许大写 HTTPS:// 也接受？— 否，与规范一致严格小写)
//  2. 不能以 "/" 结尾
//  3. 不能包含 "?" 或 "#"
//  4. 空字符串 → ErrOidcProviderIssuerURLEmpty (单独错误便于上层区分"未配置")
//
// 返回 nil 表示通过；具体错误为本文件定义的哨兵以便 errors.Is 判断。
//
// 注：本函数**不**做 DNS 解析、不做 HEAD probe；只做格式合规性。
func ValidateOidcIssuerURL(value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return ErrOidcProviderIssuerURLEmpty
	}
	if !strings.HasPrefix(v, "https://") {
		return ErrOidcProviderIssuerURLNotHTTPS
	}
	if strings.HasSuffix(v, "/") {
		return ErrOidcProviderIssuerURLTrailingSlash
	}
	if strings.ContainsAny(v, "?#") {
		return ErrOidcProviderIssuerURLContainsQueryOrFragment
	}
	return nil
}

// IsAllowedOidcProviderScope 判断给定 scope 是否在全局允许集合内。
//
// 用于 client.allowed_scopes 校验、authorize 阶段二次防御、admin UI scope 多选项。
func IsAllowedOidcProviderScope(scope string) bool {
	for _, s := range AllowedOidcProviderScopes {
		if s == scope {
			return true
		}
	}
	return false
}

// ValidateOidcProviderScopeSubset 检查 scopes 是否全部在 [AllowedOidcProviderScopes] 内。
//
// 返回首个非法 scope 的明确错误，便于 admin 表单回显。
func ValidateOidcProviderScopeSubset(scopes []string) error {
	for _, s := range scopes {
		if !IsAllowedOidcProviderScope(s) {
			return fmt.Errorf("oidc provider: scope %q is not in the allowed set %v", s, AllowedOidcProviderScopes)
		}
	}
	return nil
}
