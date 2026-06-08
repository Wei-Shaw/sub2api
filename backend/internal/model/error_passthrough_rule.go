// Package model 定义服务层使用的数据模型。
package model

import "time"

// ErrorPassthroughRule 全局错误透传规则
// 用于控制上游错误如何返回给客户端
type ErrorPassthroughRule struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`             // 规则名称
	Enabled         bool      `json:"enabled"`          // 是否启用
	Priority        int       `json:"priority"`         // 优先级（数字越小优先级越高）
	ErrorCodes      []int     `json:"error_codes"`      // 匹配的错误码列表（OR关系）
	Keywords        []string  `json:"keywords"`         // 匹配的关键词列表（OR关系）
	MatchMode       string    `json:"match_mode"`       // "any"(任一条件) 或 "all"(所有条件)
	Platforms       []string  `json:"platforms"`        // 适用平台列表
	PassthroughCode bool      `json:"passthrough_code"` // 是否透传原始状态码
	ResponseCode    *int      `json:"response_code"`    // 自定义状态码（passthrough_code=false 时使用）
	PassthroughBody bool      `json:"passthrough_body"` // 是否透传原始错误信息
	CustomMessage   *string   `json:"custom_message"`   // 自定义错误信息（passthrough_body=false 时使用）
	SkipMonitoring  bool      `json:"skip_monitoring"`  // 是否跳过运维监控记录
	Description     *string   `json:"description"`      // 规则描述
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// MatchModeAny 表示任一条件匹配即可
const MatchModeAny = "any"

// MatchModeAll 表示所有条件都必须匹配
const MatchModeAll = "all"

// 支持的平台常量
const (
	PlatformAnthropic   = "anthropic"
	PlatformOpenAI      = "openai"
	PlatformGemini      = "gemini"
	PlatformAntigravity = "antigravity"
)

// AllPlatforms 返回所有支持的平台列表
func AllPlatforms() []string {
	return []string{PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity}
}

// Validate 验证规则配置的有效性
func (r *ErrorPassthroughRule) Validate() error {
	if r.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if r.MatchMode != MatchModeAny && r.MatchMode != MatchModeAll {
		return &ValidationError{Field: "match_mode", Message: "match_mode must be 'any' or 'all'"}
	}
	// 至少需要配置一个匹配条件（错误码或关键词）
	if len(r.ErrorCodes) == 0 && len(r.Keywords) == 0 {
		return &ValidationError{Field: "conditions", Message: "at least one error_code or keyword is required"}
	}
	if !r.PassthroughCode && (r.ResponseCode == nil || *r.ResponseCode <= 0) {
		return &ValidationError{Field: "response_code", Message: "response_code is required when passthrough_code is false"}
	}
	if !r.PassthroughBody && (r.CustomMessage == nil || *r.CustomMessage == "") {
		return &ValidationError{Field: "custom_message", Message: "custom_message is required when passthrough_body is false"}
	}
	return nil
}

// ValidationError 表示验证错误
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
