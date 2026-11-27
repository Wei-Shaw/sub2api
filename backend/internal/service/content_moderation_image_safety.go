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
// 描述的是账号/上游故障而不是用户内容。带 error.param 的 400 是参数校验错误
// （例如 images 接口自身的 moderation 参数取值非法），即便文案含 moderation
// 也不是内容拒绝，一律排除。
func IsOpenAIImagesSafetyRefusal(err *OpenAIImagesUpstreamError) bool {
	if err == nil || err.StatusCode != http.StatusBadRequest {
		return false
	}
	if strings.TrimSpace(err.Param) != "" {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(err.Code))
	if code == "moderation_blocked" ||
		strings.Contains(code, "content_policy") ||
		strings.Contains(code, "policy_violation") ||
		strings.Contains(code, "safety_violation") {
		return true
	}
	return isOpenAIImagesContentPolicyRefusal(err.Message)
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
