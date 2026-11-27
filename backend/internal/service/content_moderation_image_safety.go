package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ContentModerationActionImageSafety 是上游图像安全系统拒绝生图时写入风控中心的 action。
const ContentModerationActionImageSafety = "image_safety"

// IsOpenAIImagesSafetyRefusal 判断一次终态图像上游错误是否为上游安全系统拒绝。
//
// 只认 400：上游安全拒绝（如 "Your request was rejected by the safety system"）
// 与 Responses 生图兜底合成的 content_policy_violation 都是 400；其余状态码
// 描述的是账号/上游故障而不是用户内容。
//
// 判定顺序：
//  1. 明确的安全类 error.code 直接认定（包括 Azure 带 param=prompt 的
//     content_filter）；response.incomplete 只有在 content_filter/moderation
//     原因下才会是 400，同样认定。
//  2. 只能看文案时更保守：带 error.param 的 400 是参数校验错误，排除；文案
//     是"未知/不支持的参数"之类时排除；关键词表不含单独的 "moderation"——
//     images 接口自身有 moderation 参数，兼容上游常回
//     "Unrecognized request argument supplied: moderation" 且 param 为 null。
//     误报会留存无辜用户的提示词，宁可漏记也不误记。
func IsOpenAIImagesSafetyRefusal(err *OpenAIImagesUpstreamError) bool {
	if err == nil || err.StatusCode != http.StatusBadRequest {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(err.Code))
	if code == "response_incomplete" ||
		code == "moderation_blocked" ||
		code == "contentfilter" ||
		strings.Contains(code, "content_filter") ||
		strings.Contains(code, "content_policy") ||
		strings.Contains(code, "policy_violation") ||
		strings.Contains(code, "safety_violation") ||
		strings.Contains(code, "responsibleaipolicyviolation") {
		return true
	}
	if strings.TrimSpace(err.Param) != "" {
		return false
	}
	return isImageSafetyRefusalMessage(err.Message)
}

var imageSafetyParameterErrorMarkers = []string{
	"unrecognized request argument",
	"unknown parameter",
	"unsupported parameter",
	"unexpected parameter",
	"invalid parameter",
	"invalid value",
	"unrecognized parameter",
}

var imageSafetyRefusalMarkers = []string{
	"safety system", "safety policy", "safety violation",
	"content policy", "content_policy", "content filter", "content_filter",
	"content management policy", "moderation_blocked",
	"安全系统", "安全策略", "安全政策", "内容政策", "内容审核", "违规内容", "不适合生成",
}

func isImageSafetyRefusalMessage(message string) bool {
	lower := strings.ToLower(message)
	for _, marker := range imageSafetyParameterErrorMarkers {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	for _, marker := range imageSafetyRefusalMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// ImageSafetyRecordInput 是一次图像上游安全拒绝的风控记录入参。
type ImageSafetyRecordInput struct {
	RequestID         string
	UserID            int64
	UserEmail         string
	APIKeyID          int64
	APIKeyName        string
	GroupID           *int64
	GroupName         string
	Endpoint          string
	Provider          string
	Model             string
	Prompt            string
	UpstreamStatus    int
	UpstreamType      string
	UpstreamCode      string
	UpstreamMessage   string
	UpstreamRequestID string
}

// RecordImageSafetyEvent 把一次图像上游安全拒绝写入风控中心日志，仅作留证。
//
// 拒绝已由网关原样返回给用户；本方法是事后记录，不改变响应。上游安全系统对
// 图像存在误判，因此记录固定为 risk_control_log_only：不写哈希黑名单、不计入
// 自动封号次数、不发邮件。受 risk_control_enabled 总开关与内容审核
// group/model scope 约束，不受内容审核 Enabled/Mode/sample 约束（与
// cyber_policy 记录一致）。
func (s *ContentModerationService) RecordImageSafetyEvent(ctx context.Context, in ImageSafetyRecordInput) {
	if s == nil || s.repo == nil {
		return
	}
	runtimeSnapshot, err := s.loadRuntimeSnapshot(ctx)
	if err != nil {
		slog.Warn("content_moderation.image_safety_runtime_snapshot_load_failed", "error", err)
		return
	}
	if !runtimeSnapshot.riskControlEnabled {
		return
	}
	cfg := runtimeSnapshot.config
	if !cfg.includesGroup(in.GroupID) || !cfg.includesModel(in.Model) {
		return
	}
	var userID *int64
	if in.UserID > 0 {
		userID = &in.UserID
	}
	var apiKeyID *int64
	if in.APIKeyID > 0 {
		apiKeyID = &in.APIKeyID
	}
	provider := strings.TrimSpace(in.Provider)
	if provider == "" {
		provider = PlatformOpenAI
	}
	log := &ContentModerationLog{
		RequestID:       in.RequestID,
		UserID:          userID,
		UserEmail:       in.UserEmail,
		APIKeyID:        apiKeyID,
		APIKeyName:      in.APIKeyName,
		GroupID:         cloneInt64Ptr(in.GroupID),
		GroupName:       in.GroupName,
		Endpoint:        in.Endpoint,
		Provider:        provider,
		Model:           in.Model,
		Mode:            ContentModerationModeRiskControlLogOnly,
		Action:          ContentModerationActionImageSafety,
		Flagged:         true,
		HighestCategory: ContentModerationActionImageSafety,
		HighestScore:    1.0,
		InputExcerpt:    trimRunes(redactContentModerationSecrets(strings.TrimSpace(in.Prompt)), maxModerationExcerptRunes),
		Error:           trimRunes(redactContentModerationSecrets(formatImageSafetyUpstreamDetail(in)), maxModerationExcerptRunes*4),
		CreatedAt:       time.Now(),
	}
	if err := s.repo.CreateLog(ctx, log); err != nil {
		slog.Warn("content_moderation.image_safety_create_log_failed", "user_id", in.UserID, "endpoint", in.Endpoint, "error", err)
	}
}

func formatImageSafetyUpstreamDetail(in ImageSafetyRecordInput) string {
	parts := make([]string, 0, 4)
	if in.UpstreamStatus > 0 {
		parts = append(parts, fmt.Sprintf("upstream_status=%d", in.UpstreamStatus))
	}
	if v := strings.TrimSpace(in.UpstreamType); v != "" {
		parts = append(parts, "type="+v)
	}
	if v := strings.TrimSpace(in.UpstreamCode); v != "" {
		parts = append(parts, "code="+v)
	}
	if v := strings.TrimSpace(in.UpstreamRequestID); v != "" {
		parts = append(parts, "upstream_request_id="+v)
	}
	detail := strings.Join(parts, " ")
	if msg := strings.TrimSpace(in.UpstreamMessage); msg != "" {
		if detail != "" {
			detail += "\n"
		}
		detail += msg
	}
	return detail
}
