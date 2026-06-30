// Package model 定义服务层使用的数据模型。
package model

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// TLSRouterMatchContains 表示 User-Agent 包含 pattern 即命中。
	TLSRouterMatchContains = "contains"
	// TLSRouterMatchPrefix 表示 User-Agent 以 pattern 开头即命中。
	TLSRouterMatchPrefix = "prefix"
	// TLSRouterMatchExact 表示 User-Agent 与 pattern 完全相等才命中。
	TLSRouterMatchExact = "exact"
	// TLSRouterMatchRegex 表示使用正则匹配 User-Agent。
	TLSRouterMatchRegex = "regex"
)

// TLSFingerprintRouterRule 表示 TLS 路由器中的一条 UA 匹配规则。
type TLSFingerprintRouterRule struct {
	Name                    string `json:"name"`
	Enabled                 bool   `json:"enabled"`
	MatchType               string `json:"match_type"`
	Pattern                 string `json:"pattern"`
	CaseSensitive           bool   `json:"case_sensitive"`
	TLSFingerprintProfileID int64  `json:"tls_fingerprint_profile_id"`
	UpstreamUserAgent       string `json:"upstream_user_agent,omitempty"`
	UpstreamOriginator      string `json:"upstream_originator,omitempty"`
}

// TLSFingerprintRouter TLS 指纹路由器。
type TLSFingerprintRouter struct {
	ID                                       int64                      `json:"id"`
	Name                                     string                     `json:"name"`
	Description                              *string                    `json:"description"`
	Enabled                                  bool                       `json:"enabled"`
	ChatGPTOAuthTokenUserAgent               string                     `json:"chatgpt_oauth_token_user_agent"`
	ChatGPTOAuthTokenTLSFingerprintProfileID *int64                     `json:"chatgpt_oauth_token_tls_fingerprint_profile_id"`
	CodexInviteResetUserAgent                string                     `json:"codex_invite_reset_user_agent"`
	CodexInviteResetTLSFingerprintProfileID  *int64                     `json:"codex_invite_reset_tls_fingerprint_profile_id"`
	Rules                                    []TLSFingerprintRouterRule `json:"rules"`
	CreatedAt                                time.Time                  `json:"created_at"`
	UpdatedAt                                time.Time                  `json:"updated_at"`
}

// Validate 验证路由器配置的有效性。
func (r *TLSFingerprintRouter) Validate() error {
	if r.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if len(r.ChatGPTOAuthTokenUserAgent) > 512 {
		return &ValidationError{Field: "chatgpt_oauth_token_user_agent", Message: "chatgpt_oauth_token_user_agent must be at most 512 characters"}
	}
	if r.ChatGPTOAuthTokenTLSFingerprintProfileID != nil && *r.ChatGPTOAuthTokenTLSFingerprintProfileID < -1 {
		return &ValidationError{Field: "chatgpt_oauth_token_tls_fingerprint_profile_id", Message: "chatgpt_oauth_token_tls_fingerprint_profile_id must be null, 0, -1, or a positive profile ID"}
	}
	if len(r.CodexInviteResetUserAgent) > 512 {
		return &ValidationError{Field: "codex_invite_reset_user_agent", Message: "codex_invite_reset_user_agent must be at most 512 characters"}
	}
	if r.CodexInviteResetTLSFingerprintProfileID != nil && *r.CodexInviteResetTLSFingerprintProfileID < -1 {
		return &ValidationError{Field: "codex_invite_reset_tls_fingerprint_profile_id", Message: "codex_invite_reset_tls_fingerprint_profile_id must be null, 0, -1, or a positive profile ID"}
	}
	for i, rule := range r.Rules {
		if err := rule.Validate(); err != nil {
			return &ValidationError{Field: "rules", Message: "rule " + strconv.Itoa(i+1) + " " + err.Error()}
		}
	}
	return nil
}

// Validate 验证路由规则配置的有效性。
func (r TLSFingerprintRouterRule) Validate() error {
	if strings.TrimSpace(r.Pattern) == "" {
		return &ValidationError{Field: "pattern", Message: "pattern is required"}
	}
	switch NormalizeTLSRouterMatchType(r.MatchType) {
	case TLSRouterMatchContains, TLSRouterMatchPrefix, TLSRouterMatchExact:
		return nil
	case TLSRouterMatchRegex:
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return &ValidationError{Field: "pattern", Message: "regex is invalid"}
		}
		return nil
	default:
		return &ValidationError{Field: "match_type", Message: "match_type must be contains, prefix, exact, or regex"}
	}
}

// NormalizeTLSRouterMatchType 归一化 TLS 路由器匹配类型。
func NormalizeTLSRouterMatchType(matchType string) string {
	switch strings.ToLower(strings.TrimSpace(matchType)) {
	case TLSRouterMatchPrefix:
		return TLSRouterMatchPrefix
	case TLSRouterMatchExact:
		return TLSRouterMatchExact
	case TLSRouterMatchRegex:
		return TLSRouterMatchRegex
	default:
		return TLSRouterMatchContains
	}
}
