package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// CyberPolicyReplayResult 是一次把留存请求重放到 moderation 审核接口的结果。
// 重放不写入任何日志表、不计违规计数、不发送邮件。
type CyberPolicyReplayResult struct {
	ScannedText     string             `json:"scanned_text"`
	ImageCount      int                `json:"image_count"`
	Flagged         bool               `json:"flagged"`
	HighestCategory string             `json:"highest_category"`
	HighestScore    float64            `json:"highest_score"`
	CategoryScores  map[string]float64 `json:"category_scores"`
	Thresholds      map[string]float64 `json:"thresholds"`
	Model           string             `json:"model"`
	LatencyMS       int                `json:"latency_ms"`
	// ChunkCount 实际送审的分片数；TokenCount 为全文 token 数；
	// ChunkTokens 为本次生效的分片上限，便于前端解释「为什么分了这么多片」。
	ChunkCount  int `json:"chunk_count"`
	TokenCount  int `json:"token_count"`
	ChunkTokens int `json:"chunk_tokens"`
}

// GetCyberPolicyRequest 取回 cyber_policy 命中留存的原始请求体；未留存时返回 NotFound。
func (s *ContentModerationService) GetCyberPolicyRequest(ctx context.Context, moderationLogID int64) (*CyberPolicyRequestPayload, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("CONTENT_MODERATION_REPOSITORY_UNAVAILABLE", "风控仓储不可用")
	}
	payload, err := s.repo.GetRequestPayload(ctx, moderationLogID)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, infraerrors.NotFound("CYBER_POLICY_REQUEST_NOT_FOUND", "该记录未留存请求体")
	}
	return payload, nil
}

// ReplayCyberPolicyRequest 取回留存的请求体，按当前生效配置重新送 moderation 审核接口打分。
// 使用当前配置而非记录当时的快照——重放的意义是回答"按现在的配置会怎么判"。
//
// 注意：moderation 接口只返回内容分类分数，不会返回 cyber_policy（那是上游 LLM 的独立
// 安全策略），因此重放结果为"未命中"是常见且正常的。
func (s *ContentModerationService) ReplayCyberPolicyRequest(ctx context.Context, moderationLogID int64) (*CyberPolicyReplayResult, error) {
	payload, err := s.GetCyberPolicyRequest(ctx, moderationLogID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	// 用不截断的全文：分片审核需要完整内容，截断会让超出部分从未被审核。
	content := ExtractContentModerationInputFull(payload.Protocol, []byte(payload.RequestBody))
	if content.IsEmpty() {
		return nil, infraerrors.BadRequest("CYBER_POLICY_REPLAY_EMPTY_INPUT", "该请求没有可审核的文本内容")
	}
	if len(cfg.apiKeys()) == 0 {
		return nil, infraerrors.BadRequest("CONTENT_MODERATION_API_KEY_MISSING", "请先在风控配置中填写 moderation API Key")
	}
	start := time.Now()
	chunked, err := s.moderateChunked(ctx, cfg, content, false)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, infraerrors.InternalServer("CONTENT_MODERATION_REPLAY_FAILED", "重放审核失败: "+err.Error())
	}
	flagged, highestCategory, highestScore := evaluateModerationScores(chunked.CategoryScores, cfg.Thresholds)
	return &CyberPolicyReplayResult{
		ScannedText:     content.Text,
		ImageCount:      len(content.Images),
		Flagged:         flagged,
		HighestCategory: highestCategory,
		HighestScore:    highestScore,
		CategoryScores:  cloneFloatMap(chunked.CategoryScores),
		Thresholds:      cloneFloatMap(cfg.Thresholds),
		Model:           cfg.Model,
		LatencyMS:       latency,
		ChunkCount:      chunked.ChunkCount,
		TokenCount:      chunked.TokenCount,
		ChunkTokens:     cfg.ChunkTokens,
	}, nil
}
