// Package service: P0-3 §4.4 task 2 — 长期身份画像热路径注入。
//
// 该文件提供把 (sub2api_user, platform) 维度的伪指纹画像（IdentityProfile）
// 注入到出站请求关键字段的工具函数，让同一 sub2api 用户跨多次请求/多个上游
// 账号呈现稳定的 device_id / originator / session_id 用户段。
//
// 设计原则（按 handoff §4.4 慎重原则）：
//
//   - 默认关闭：通过 cfg.Gateway.IdentityProfileInjectEnabled 控制，需 admin
//     显式开启，便于灰度回滚。
//
//   - **绝不动 stainless / UA 头**：per-account fingerprint 已经稳定 mimic 了
//     Anthropic SDK 的 stainless 链路，盲覆盖会破坏现网行为。本切片只覆盖
//     metadata.user_id 中的 device_id 段（Anthropic）和 originator/session_id
//     的用户混合段（OpenAI），其他全部保留。
//
//   - **失败降级为 noop**：任一前置条件不满足（service 为空、user_id 为 0、
//     metadata.user_id 解析失败）都直接返回原始 body / 原值，不阻断请求。

package service

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// resolveSubjectUserID 从 gin.Context 中取出已认证的 sub2api 用户 id。
// handler 在 auth 中间件之后会把 subject 写入 gin.Context，service 层读取。
func resolveSubjectUserID(c *gin.Context) int64 {
	if c == nil {
		return 0
	}
	// gin.Context 的 Get 与 Request.Context 的 Value 都需要尝试，
	// 因为目前 handler 注入到的是 gin.Context（通过 c.Set 或 c.Request.Context）。
	if v, ok := c.Get(string(ctxkey.SubjectUserID)); ok {
		if uid, ok := v.(int64); ok {
			return uid
		}
	}
	if c.Request != nil {
		if v := c.Request.Context().Value(ctxkey.SubjectUserID); v != nil {
			if uid, ok := v.(int64); ok {
				return uid
			}
		}
	}
	return 0
}

// applyIdentityProfileToAnthropicBody 在 Anthropic OAuth 路径上把 metadata.user_id
// 中的 device_id 段替换为 IdentityProfile.DeviceIDHex64。
//
//   - body 必须是已经经过 RewriteUserIDWithMasking 处理之后的 body（保证
//     metadata.user_id 已存在且格式合法）。
//   - 当 inject 关闭、profile 为空、userID==0 或解析失败时返回原 body。
//   - 该替换只动 device_id 字段；session_id 由 P0-2 / 会话 mask 控制，
//     account_uuid 必须保留（上游用它判账号身份）。
func (s *GatewayService) applyIdentityProfileToAnthropicBody(c *gin.Context, body []byte) []byte {
	if !s.identityProfileInjectEnabled() || s.identityProfileService == nil {
		return body
	}
	userID := resolveSubjectUserID(c)
	if userID <= 0 {
		return body
	}

	uidValue := gjson.GetBytes(body, "metadata.user_id")
	if !uidValue.Exists() || uidValue.Type != gjson.String {
		return body
	}
	parsed := ParseMetadataUserID(uidValue.String())
	if parsed == nil {
		return body
	}

	profile := s.identityProfileService.Profile(userID, PlatformAnthropic, time.Now())
	newDeviceID := profile.DeviceIDHex64
	if newDeviceID == "" || newDeviceID == parsed.DeviceID {
		return body
	}

	// 重建 metadata.user_id：新格式 → JSON；旧格式 → user_xxx_account_yyy_session_zzz。
	// 统一用 FormatMetadataUserID 处理格式选择，避免手动拼装。
	//
	// uaVersion 选型：
	//   - 新格式：必须 >= NewMetadataFormatMinVersion（"2.1.78"）才会产出 JSON；
	//     传入 NewMetadataFormatMinVersion 即可。
	//   - 旧格式：传 "" 让 FormatMetadataUserID 走 legacy 分支。
	uaVersion := ""
	if parsed.IsNewFormat {
		uaVersion = NewMetadataFormatMinVersion
	}
	rebuilt := FormatMetadataUserID(newDeviceID, parsed.AccountUUID, parsed.SessionID, uaVersion)
	updated, err := sjson.SetBytes(body, "metadata.user_id", rebuilt)
	if err != nil || len(updated) == 0 {
		return body
	}
	return updated
}

// identityProfileInjectEnabled 返回是否启用 P0-3 注入。配置缺失时默认 false。
func (s *GatewayService) identityProfileInjectEnabled() bool {
	if s == nil || s.cfg == nil {
		return false
	}
	return s.cfg.Gateway.IdentityProfileInjectEnabled
}

// stableOpenAISessionUserSeed 给 OpenAI 路径返回一个稳定的"用户级混合种子"，
// 用于 isolateOpenAISessionID 派生 session_id / conversation_id。
//
// 设计动机：现网 isolateOpenAISessionID 只用 apiKeyID 做隔离，导致同一 sub2api
// 用户在 apiKey 轮换 / 多 key 共用时，对外看到的 session_id 会跨 apiKey 翻新，
// 与"真实用户单设备长 session"模式不一致——上游可以用这个差异关联出"代理"。
//
// 注入开启后，把 (sub2api_user, OpenAI 平台) 的 MachineID 混入隔离种子，让同一
// 用户跨 apiKey 的 session_id 派生出稳定但仍互不冲突的值。空字符串 = noop
// （维持原 isolateOpenAISessionID 行为，向后兼容）。
//
// 故意不动 originator：originator 字段是 codex_cli_rs / chatgpt_web 这种官方
// client 强字符串标识，乱拼会被上游立即识破，比合流暴露风险更大；P0-3 此切片
// 只覆盖会随真实用户行为天然变化的 session_id。
func (s *OpenAIGatewayService) stableOpenAISessionUserSeed(c *gin.Context) string {
	if s == nil || s.cfg == nil || !s.cfg.Gateway.IdentityProfileInjectEnabled {
		return ""
	}
	if s.identityProfileService == nil {
		return ""
	}
	userID := resolveSubjectUserID(c)
	if userID <= 0 {
		return ""
	}
	profile := s.identityProfileService.Profile(userID, PlatformOpenAI, time.Now())
	return strings.TrimSpace(profile.MachineID)
}
