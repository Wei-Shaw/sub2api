// Package service — support_chat_rag_settings.go 提供 add-support-knowledge-rag
// change 引入的 8 个 admin-only setting 的 Parse / Normalize / Clamp / Validate helper。
//
// 模式与 support_chat_settings.go 完全一致：
//   - Parse* 用于 GET 路径，损坏 / 缺失时回退到安全默认值。
//   - Normalize* / Validate* 用于 PUT 严格校验（admin 错误输入返 400）。
//
// 8 个 key 见 domain_constants.go：
//   support_chat_rag_enabled / doc_url / doc_depth / doc_cron /
//   embed_model / top_k / chunk_size / chunk_overlap
package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ====================================================================
// doc_url (string, 0..SupportChatRAGDocURLMaxLen 字符)
// ====================================================================

// ParseSupportChatRAGDocURL trim、不严格校验 URL 格式（admin 可能填子路径）；
// 仅长度兜底（库里残留过长字符串时硬截）。GET 永远返回合法值。
func ParseSupportChatRAGDocURL(raw string) string {
	v := strings.TrimSpace(raw)
	if utf8.RuneCountInString(v) > SupportChatRAGDocURLMaxLen {
		// 已持久化的过长内容：硬截而不是覆盖回空字符串（避免把 admin 已配置的关键值丢掉）
		return string([]rune(v)[:SupportChatRAGDocURLMaxLen])
	}
	return v
}

// NormalizeSupportChatRAGDocURL 严格校验：trim 后 0..MaxLen；
// 允许空字符串（admin 可清空 → pipeline 会以 empty_doc_url 状态短路退出）。
// 当非空时必须以 http:// 或 https:// 开头。
func NormalizeSupportChatRAGDocURL(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	if utf8.RuneCountInString(v) > SupportChatRAGDocURLMaxLen {
		return "", infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_RAG_DOC_URL",
			fmt.Sprintf("doc_url must be at most %d characters", SupportChatRAGDocURLMaxLen),
		)
	}
	if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
		return "", infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_RAG_DOC_URL",
			"doc_url must start with http:// or https://",
		)
	}
	return v, nil
}

// ====================================================================
// doc_depth (int, 0..2)
// ====================================================================

// ClampSupportChatRAGDocDepth 把任意整数裁剪到 [Min, Max]；
// 负数 → Default。GET 永远返回合法值。
func ClampSupportChatRAGDocDepth(v int) int {
	if v < SupportChatRAGDocDepthMin {
		return SupportChatRAGDocDepthDefault
	}
	if v > SupportChatRAGDocDepthMax {
		return SupportChatRAGDocDepthMax
	}
	return v
}

// ValidateSupportChatRAGDocDepth 严格校验，PUT 路径用。
func ValidateSupportChatRAGDocDepth(v int) (int, error) {
	if v < SupportChatRAGDocDepthMin || v > SupportChatRAGDocDepthMax {
		return 0, infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_RAG_DOC_DEPTH",
			fmt.Sprintf("doc_depth must be in [%d, %d]", SupportChatRAGDocDepthMin, SupportChatRAGDocDepthMax),
		)
	}
	return v, nil
}

// ====================================================================
// doc_cron (enum: daily-03 / weekly / manual)
// ====================================================================

// ParseSupportChatRAGDocCron 从持久化字符串读取，未知值回退到 daily-03。
func ParseSupportChatRAGDocCron(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" || !IsSupportChatRAGAllowedDocCron(v) {
		return SupportChatRAGDocCronDaily03
	}
	return v
}

// NormalizeSupportChatRAGDocCron PUT 严格校验。
func NormalizeSupportChatRAGDocCron(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if !IsSupportChatRAGAllowedDocCron(v) {
		return "", infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_RAG_DOC_CRON",
			fmt.Sprintf("doc_cron must be one of: %s", strings.Join(SupportChatRAGAllowedDocCrons, ", ")),
		)
	}
	return v, nil
}

// ====================================================================
// embed_model (string, 1..200 字符)
// ====================================================================

// ParseSupportChatRAGEmbedModel trim 后非空返回，否则用默认。
func ParseSupportChatRAGEmbedModel(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return SupportChatRAGEmbedModelDefault
	}
	return v
}

// NormalizeSupportChatRAGEmbedModel PUT 严格校验：trim 后必须非空，长度 ≤ 200。
func NormalizeSupportChatRAGEmbedModel(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_RAG_EMBED_MODEL",
			"embed_model must not be blank",
		)
	}
	if utf8.RuneCountInString(v) > 200 {
		return "", infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_RAG_EMBED_MODEL",
			"embed_model must be at most 200 characters",
		)
	}
	return v, nil
}

// ====================================================================
// top_k (int, 1..20)
// ====================================================================

// ClampSupportChatRAGTopK 0 / 负数 → 默认；超界 → 边界。
func ClampSupportChatRAGTopK(v int) int {
	if v <= 0 {
		return SupportChatRAGTopKDefault
	}
	if v < SupportChatRAGTopKMin {
		return SupportChatRAGTopKMin
	}
	if v > SupportChatRAGTopKMax {
		return SupportChatRAGTopKMax
	}
	return v
}

// ValidateSupportChatRAGTopK 严格校验。
func ValidateSupportChatRAGTopK(v int) (int, error) {
	if v < SupportChatRAGTopKMin || v > SupportChatRAGTopKMax {
		return 0, infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_RAG_TOP_K",
			fmt.Sprintf("top_k must be in [%d, %d]", SupportChatRAGTopKMin, SupportChatRAGTopKMax),
		)
	}
	return v, nil
}

// ====================================================================
// chunk_size (int, 200..2000)
// ====================================================================

// ClampSupportChatRAGChunkSize 0/负 → 默认；超界 → 边界。
func ClampSupportChatRAGChunkSize(v int) int {
	if v <= 0 {
		return SupportChatRAGChunkSizeDefault
	}
	if v < SupportChatRAGChunkSizeMin {
		return SupportChatRAGChunkSizeMin
	}
	if v > SupportChatRAGChunkSizeMax {
		return SupportChatRAGChunkSizeMax
	}
	return v
}

// ValidateSupportChatRAGChunkSize 严格校验。
func ValidateSupportChatRAGChunkSize(v int) (int, error) {
	if v < SupportChatRAGChunkSizeMin || v > SupportChatRAGChunkSizeMax {
		return 0, infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_RAG_CHUNK_SIZE",
			fmt.Sprintf("chunk_size must be in [%d, %d]", SupportChatRAGChunkSizeMin, SupportChatRAGChunkSizeMax),
		)
	}
	return v, nil
}

// ====================================================================
// chunk_overlap (int, 0..500)
// ====================================================================

// ClampSupportChatRAGChunkOverlap 负值 → 默认；超界 → 边界。
func ClampSupportChatRAGChunkOverlap(v int) int {
	if v < SupportChatRAGChunkOverlapMin {
		return SupportChatRAGChunkOverlapDefault
	}
	if v > SupportChatRAGChunkOverlapMax {
		return SupportChatRAGChunkOverlapMax
	}
	return v
}

// ValidateSupportChatRAGChunkOverlap 严格校验。
// 注意 caller 应再做 chunk_overlap < chunk_size 的交叉校验（见 setting_handler）。
func ValidateSupportChatRAGChunkOverlap(v int) (int, error) {
	if v < SupportChatRAGChunkOverlapMin || v > SupportChatRAGChunkOverlapMax {
		return 0, infraerrors.BadRequest(
			"INVALID_SUPPORT_CHAT_RAG_CHUNK_OVERLAP",
			fmt.Sprintf("chunk_overlap must be in [%d, %d]", SupportChatRAGChunkOverlapMin, SupportChatRAGChunkOverlapMax),
		)
	}
	return v, nil
}

// ====================================================================
// Doc index status (JSON 状态对象)
// ====================================================================
//
// SupportChatRAGDocIndexStatus 是 doc pipeline 持久化到 setting 的状态对象。
// 不是用户可输入字段——pipeline 写、admin GET /api/admin/support/doc-index/status 读。
type SupportChatRAGDocIndexStatus struct {
	// LastRunAt 最近一次完整流程结束的时间（成功或失败都更新）；零值表示从未运行。
	LastRunAt string `json:"last_run_at,omitempty"`
	// Running 表示当前 pipeline 是否在跑（goroutine 启动 / 结束时由 advisory lock 守卫更新）。
	Running bool `json:"running,omitempty"`
	// Status 概要：success / partial / failed / empty_doc_url / already_running。
	Status string `json:"status,omitempty"`
	// ChunksTotal 当前 support_doc_chunks 表行数（pipeline 结束时记录）。
	ChunksTotal int `json:"chunks_total"`
	// ChunksAdded 本次运行新插入的 chunk 数。
	ChunksAdded int `json:"chunks_added"`
	// ChunksRemoved 本次运行删除的 orphan chunk 数。
	ChunksRemoved int `json:"chunks_removed"`
	// ChunksFailedEmbed 本次运行 row 已写入但 embedding 仍为 NULL 的数量。
	ChunksFailedEmbed int `json:"chunks_failed_embed"`
	// PagesFetched 本次运行成功抓取的 URL 数。
	PagesFetched int `json:"pages_fetched"`
	// Errors 各 URL / 各阶段的错误信息（用于 admin 卡片"最近一次错误"展示）。
	Errors []SupportChatRAGDocIndexError `json:"errors,omitempty"`
}

// SupportChatRAGDocIndexError 是 status 对象内的错误条目。
type SupportChatRAGDocIndexError struct {
	URL     string `json:"url,omitempty"`
	Stage   string `json:"stage"` // fetch / parse / chunk / embed / persist / cap_hit
	Message string `json:"message"`
}
