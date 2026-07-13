// Package service — support_chat_rag_retriever.go 提供 RAG 检索编排层。
//
// 工作流（add-support-knowledge-rag, 见 design.md D5/D7）：
//
//  1. caller（chat handler）拿到用户最新一条 message → 调用 RetrieveTopK(ctx, query)
//  2. 本服务调 EmbeddingService.Embed(query) 拿到 q 向量
//  3. 调 SupportChatRAGRetriever.RetrieveTopK 跑 UNION ALL SQL 一次性拉两表 top-K 候选
//  4. 在 Go 层应用 0.3 cosine 阈值（避免给 LLM 喂噪声片段）
//  5. 按 score DESC 排序后返回
//
// 阈值与 top-K 限制都在 Go 层完成，不下推到 SQL：
//   - 阈值过滤：表达力一致，Go 端读到结果再丢，可控；
//   - top-K：传入 SQL 的是 K，但 Go 端再 cap 一次防止 repository 实现忽视 limit。
//
// 错误传播：embedding 失败 / SQL 失败都会冒泡到 caller，由 caller 决定是否降级（chat
// handler 在 RAG 失败时会 silent skip 整个 RAG 段，按 design D7 走"无相关知识"路径）。
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// SupportChatRAGRetrieveSourceFAQ / SupportChatRAGRetrieveSourceDoc：Hit.Source 取值集合。
const (
	SupportChatRAGRetrieveSourceFAQ = "faq"
	SupportChatRAGRetrieveSourceDoc = "doc"
)

// SupportChatRAGScoreThreshold 是 cosine similarity 注入阈值（design D7）。
// 低于该值的 Hit 视作"无相关知识"，不进入 prompt 注入。
const SupportChatRAGScoreThreshold = 0.3

// SupportChatRAGHit 是单条检索命中。
//
// Title 仅 FAQ 路径有值（=question）；Doc 路径为空。
// SourceURL 仅 Doc 路径有值（=source_url）；FAQ 路径为空。
// Body 是用于注入 prompt 的正文（FAQ 是 answer；Doc 是 chunk_text）。
type SupportChatRAGHit struct {
	Source    string  // "faq" | "doc"
	ID        int64   // 对应表 id
	Title     string  // FAQ.question；Doc 为空
	Body      string  // FAQ.answer / Doc.chunk_text
	SourceURL string  // Doc.source_url；FAQ 为空
	Score     float64 // cosine similarity，范围 0..1
}

// SupportChatRAGRetriever 是底层向量检索接口，由 repository 包实现。
//
// 实现方法应当：
//   - 接收 query 向量（长度 == SupportChatRAGEmbedDimension），运行 design D5 的
//     UNION ALL SQL，限制 LIMIT $2 = limit。
//   - 仅返回 embedding IS NOT NULL 的行（与 enabled=true 联合过滤）。
//   - 不做阈值过滤（service 层负责）。
//   - 不做排序后处理（SQL 已 ORDER BY score DESC）；service 层会再排一次以兜底。
type SupportChatRAGRetriever interface {
	RetrieveTopK(ctx context.Context, qVec []float32, limit int) ([]SupportChatRAGHit, error)
}

// SupportChatRAGRetrievalService 是检索编排层；chat handler 直接依赖它即可。
type SupportChatRAGRetrievalService struct {
	embedding EmbeddingService
	retriever SupportChatRAGRetriever
	settings  *SettingService
}

// NewSupportChatRAGRetrievalService 注入 embedding + retriever + settings 三个依赖。
func NewSupportChatRAGRetrievalService(
	embedding EmbeddingService,
	retriever SupportChatRAGRetriever,
	settings *SettingService,
) *SupportChatRAGRetrievalService {
	return &SupportChatRAGRetrievalService{
		embedding: embedding,
		retriever: retriever,
		settings:  settings,
	}
}

// RetrieveTopK 编排"embed query → 跑 UNION ALL → 阈值过滤 → 截断到 topK"。
//
// 入参 query 是用户最新一条消息（caller 已截到合理长度）。
// 入参 topK 通常来自 SupportChatRAGRuntime.TopK；本方法不读 settings，避免重复 round-trip。
//
// 返回切片长度 ≤ topK；空切片表示"没有相关知识"。
//
// 错误：
//   - query 为空 → ErrEmbeddingEmptyText（embedding service 抛）
//   - embedding 失败 → 直接返回错误，由 caller 决定降级
//   - retriever SQL 失败 → 直接返回错误
func (s *SupportChatRAGRetrievalService) RetrieveTopK(
	ctx context.Context,
	query string,
	topK int,
) ([]SupportChatRAGHit, error) {
	if s == nil {
		return nil, fmt.Errorf("support chat rag retrieval: service nil")
	}
	if topK <= 0 {
		topK = SupportChatRAGTopKDefault
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, ErrEmbeddingEmptyText
	}

	vec, err := s.embedding.Embed(ctx, q)
	if err != nil {
		// 凭据缺失：转成"空检索结果"语义，让 chat handler 走"无相关知识"分支，
		// 而不是把"凭据没配置"当成对话失败。由 change-support-chat-external-llm 引入。
		if errors.Is(err, ErrEmbeddingDisabled) {
			return nil, nil
		}
		return nil, fmt.Errorf("support chat rag retrieve embed: %w", err)
	}

	// 拉比 topK 略多一些（cap 32），让阈值过滤后仍有足够样本；
	// 但 SQL LIMIT 仍受 topK 控制以减少传输量；这里保守地直接传 topK。
	hits, err := s.retriever.RetrieveTopK(ctx, vec, topK)
	if err != nil {
		return nil, fmt.Errorf("support chat rag retrieve sql: %w", err)
	}

	// 阈值过滤 + 兜底排序（防止实现层未排序）。
	filtered := hits[:0]
	for _, h := range hits {
		if h.Score >= SupportChatRAGScoreThreshold {
			filtered = append(filtered, h)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	if len(filtered) > topK {
		filtered = filtered[:topK]
	}
	return filtered, nil
}
