// Package service — support_faq_service.go
//
// 客服知识库 FAQ 业务服务（admin-only CRUD + embedding 同步写入）。
//
// 设计要点：
//
//  1. 写入路径（Create / Update）默认在保存后同步重算 embedding：
//
//     row 持久化   →   嵌入服务调用   →   SetEmbedding（成功）
//                                  ↓ 失败
//                              row 仍可见但 Indexed = false；
//                              返回值带 EmbeddingWarning，admin UI 显示"未索引"。
//
//     这样 admin 不会因为 OpenAI 短暂失败而无法保存 FAQ，与 design D9 一致。
//
//  2. Validate 在 service 层做（trim + 长度 / 数量 / tag 单条长度）。重复 question
//     **不**强行去重 —— admin 可能故意保留多条相似 FAQ 应对不同语境。
//
//  3. Reindex（重新嵌入所有 enabled 行）走单独入口，不卡 admin 的 UI 操作；
//     调用方可以选 listIDsWithoutEmbedding（仅补 NULL）或全量。
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"
)

// SupportFaqService 提供 FAQ admin CRUD 与 embedding 同步。
type SupportFaqService struct {
	repo      SupportFaqRepository
	embedding EmbeddingService
}

// NewSupportFaqService 构造 FAQ service。embedding 可为 nil（embedding 关闭场景），
// 但 admin 路径在 RAG 启用时建议非 nil。
func NewSupportFaqService(repo SupportFaqRepository, embedding EmbeddingService) *SupportFaqService {
	return &SupportFaqService{repo: repo, embedding: embedding}
}

// SupportFaqMutationResult 是 Create / Update 的返回值：包含 row + 可选 warning。
//
// EmbeddingWarning 非空表示 row 已持久化但 embedding 失败；调用方应显示给 admin。
type SupportFaqMutationResult struct {
	Item             SupportFaqItem
	EmbeddingWarning string // "" = 成功；非空 = 失败原因（用户可读）
}

// Create 新建 FAQ。流程：validate → ent.Create → embed → SetEmbedding。
//
// 即使 embedding 失败，Create 仍 return nil（row 已经入库）；warning 走结果里。
// 完全不调 embedding（embedding == nil 或 ErrEmbeddingDisabled）也算成功，
// Indexed = false。
func (s *SupportFaqService) Create(ctx context.Context, in SupportFaqItem) (*SupportFaqMutationResult, error) {
	question, answer, tags, err := normalizeFaqFields(in.Question, in.Answer, in.Tags)
	if err != nil {
		return nil, err
	}

	row := SupportFaqItem{
		Question:  question,
		Answer:    answer,
		Tags:      tags,
		Enabled:   in.Enabled,
		SortOrder: in.SortOrder,
	}
	if err := s.repo.Create(ctx, &row); err != nil {
		return nil, err
	}

	res := &SupportFaqMutationResult{Item: row}
	if !row.Enabled {
		// disabled FAQ 不参与检索，没必要算 embedding；保持 Indexed=false。
		return res, nil
	}
	res.EmbeddingWarning = s.tryEmbed(ctx, row.ID, row.Question, row.Answer)
	if res.EmbeddingWarning == "" {
		// 重读一次 Indexed 状态；避免上层在 UI 上看到不一致。
		fresh, err := s.repo.GetByID(ctx, row.ID)
		if err == nil && fresh != nil {
			res.Item = *fresh
		}
	}
	return res, nil
}

// Update 部分更新 FAQ。如果 question / answer / enabled 任一变化，重算 embedding。
//
// 同样：embedding 失败不阻塞写入；warning 走结果里。
func (s *SupportFaqService) Update(ctx context.Context, id int64, patch SupportFaqItemPatch) (*SupportFaqMutationResult, error) {
	if id <= 0 {
		return nil, ErrSupportFaqNotFound
	}

	// 校验：仅校验非 nil 字段。
	normPatch, err := normalizeFaqPatch(patch)
	if err != nil {
		return nil, err
	}

	if err := s.repo.UpdatePartial(ctx, id, normPatch); err != nil {
		return nil, err
	}

	// 重读 row（含 Indexed）。
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	res := &SupportFaqMutationResult{Item: *row}

	// 仅当影响 embedding 内容（question / answer）或启用状态变化时才重算。
	needReembed := normPatch.Question != nil || normPatch.Answer != nil || normPatch.Enabled != nil
	if !needReembed {
		return res, nil
	}

	if !row.Enabled {
		// 切换为 disabled：清掉 embedding，避免在检索集合里残留。
		if err := s.repo.ClearEmbedding(ctx, id); err != nil {
			slog.WarnContext(ctx, "support_faq_clear_embedding_failed",
				slog.Int64("id", id), slog.String("err", err.Error()))
			res.EmbeddingWarning = "清除向量失败：" + err.Error()
		}
		// 重读 Indexed 状态。
		if fresh, err := s.repo.GetByID(ctx, id); err == nil && fresh != nil {
			res.Item = *fresh
		}
		return res, nil
	}

	// enabled = true：重算 embedding。
	res.EmbeddingWarning = s.tryEmbed(ctx, row.ID, row.Question, row.Answer)
	if res.EmbeddingWarning == "" {
		if fresh, err := s.repo.GetByID(ctx, id); err == nil && fresh != nil {
			res.Item = *fresh
		}
	}
	return res, nil
}

// Delete 物理删除 FAQ。
func (s *SupportFaqService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrSupportFaqNotFound
	}
	return s.repo.Delete(ctx, id)
}

// Get 单条读取。
func (s *SupportFaqService) Get(ctx context.Context, id int64) (*SupportFaqItem, error) {
	if id <= 0 {
		return nil, ErrSupportFaqNotFound
	}
	return s.repo.GetByID(ctx, id)
}

// List 列表。onlyEnabled = true 时仅返回 enabled，公开 FAQ 列表用。
func (s *SupportFaqService) List(ctx context.Context, onlyEnabled bool) ([]SupportFaqItem, error) {
	return s.repo.List(ctx, onlyEnabled)
}

// CountAll 总行数（数据迁移启动钩子的 "is empty" 判断）。
func (s *SupportFaqService) CountAll(ctx context.Context) (int, error) {
	return s.repo.CountAll(ctx)
}

// ReindexMissing 给所有 embedding 为空的启用 row 补 embedding。
//
// 返回 (succeeded, failed, error)：error 仅在不可恢复故障时非 nil（embedding service
// 配置错误等）。单条失败不中断流程，记录到 failed 计数。
func (s *SupportFaqService) ReindexMissing(ctx context.Context) (int, int, error) {
	if s.embedding == nil {
		return 0, 0, ErrEmbeddingDisabled
	}
	ids, err := s.repo.ListIDsWithoutEmbedding(ctx)
	if err != nil {
		return 0, 0, err
	}
	succeeded, failed := 0, 0
	for _, id := range ids {
		row, err := s.repo.GetByID(ctx, id)
		if err != nil {
			failed++
			continue
		}
		if !row.Enabled {
			continue
		}
		if w := s.tryEmbed(ctx, row.ID, row.Question, row.Answer); w == "" {
			succeeded++
		} else {
			failed++
		}
	}
	return succeeded, failed, nil
}

// ReindexAll 强制重算所有 enabled FAQ 的 embedding（admin 改了 embed_model 时用）。
func (s *SupportFaqService) ReindexAll(ctx context.Context) (int, int, error) {
	if s.embedding == nil {
		return 0, 0, ErrEmbeddingDisabled
	}
	rows, err := s.repo.List(ctx, true) // onlyEnabled
	if err != nil {
		return 0, 0, err
	}
	succeeded, failed := 0, 0
	for i := range rows {
		row := rows[i]
		if w := s.tryEmbed(ctx, row.ID, row.Question, row.Answer); w == "" {
			succeeded++
		} else {
			failed++
		}
	}
	return succeeded, failed, nil
}

// tryEmbed 尝试给一行算 embedding 并写库；返回 "" 表示成功，非空表示 user-readable warning。
func (s *SupportFaqService) tryEmbed(ctx context.Context, id int64, question, answer string) string {
	if s.embedding == nil {
		return "" // 没配 embedding service：保持 Indexed=false 但不视作错误。
	}
	// FAQ embedding 输入采用 "Q:.../A:..." 模式，提升语义相似度的覆盖面。
	text := buildFaqEmbeddingText(question, answer)
	vec, err := s.embedding.Embed(ctx, text)
	if err != nil {
		if errors.Is(err, ErrEmbeddingDisabled) {
			// admin 没配客服 api key：不视作 warning（FAQ 仍可保存）。
			return ""
		}
		slog.WarnContext(ctx, "support_faq_embed_failed",
			slog.Int64("id", id), slog.String("err", err.Error()))
		return "向量生成失败：" + err.Error()
	}
	if err := s.repo.SetEmbedding(ctx, id, vec); err != nil {
		slog.WarnContext(ctx, "support_faq_set_embedding_failed",
			slog.Int64("id", id), slog.String("err", err.Error()))
		return "向量写入失败：" + err.Error()
	}
	return ""
}

// normalizeFaqFields 校验 + trim + tag 归一化。
func normalizeFaqFields(question, answer string, tags []string) (string, string, []string, error) {
	q := strings.TrimSpace(question)
	if q == "" {
		return "", "", nil, ErrSupportFaqQuestionRequired
	}
	if utf8.RuneCountInString(q) > SupportFaqQuestionMaxLen {
		return "", "", nil, ErrSupportFaqQuestionTooLong
	}
	a := strings.TrimSpace(answer)
	if a == "" {
		return "", "", nil, ErrSupportFaqAnswerRequired
	}
	if utf8.RuneCountInString(a) > SupportFaqAnswerMaxLen {
		return "", "", nil, ErrSupportFaqAnswerTooLong
	}

	normTags, err := normalizeFaqTags(tags)
	if err != nil {
		return "", "", nil, err
	}
	return q, a, normTags, nil
}

// normalizeFaqTags 校验 + trim + 去重 + 数量限制。
func normalizeFaqTags(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return []string{}, nil
	}
	if len(tags) > SupportFaqTagsMaxItems {
		return nil, ErrSupportFaqTagsTooMany
	}
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, raw := range tags {
		t := strings.TrimSpace(raw)
		if t == "" {
			return nil, ErrSupportFaqTagInvalid
		}
		if utf8.RuneCountInString(t) > SupportFaqTagMaxLen {
			return nil, ErrSupportFaqTagInvalid
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out, nil
}

// normalizeFaqPatch 校验 patch 中非 nil 字段。
func normalizeFaqPatch(p SupportFaqItemPatch) (SupportFaqItemPatch, error) {
	out := p

	if p.Question != nil {
		q := strings.TrimSpace(*p.Question)
		if q == "" {
			return out, ErrSupportFaqQuestionRequired
		}
		if utf8.RuneCountInString(q) > SupportFaqQuestionMaxLen {
			return out, ErrSupportFaqQuestionTooLong
		}
		out.Question = &q
	}
	if p.Answer != nil {
		a := strings.TrimSpace(*p.Answer)
		if a == "" {
			return out, ErrSupportFaqAnswerRequired
		}
		if utf8.RuneCountInString(a) > SupportFaqAnswerMaxLen {
			return out, ErrSupportFaqAnswerTooLong
		}
		out.Answer = &a
	}
	if p.Tags != nil {
		norm, err := normalizeFaqTags(*p.Tags)
		if err != nil {
			return out, err
		}
		out.Tags = &norm
	}
	if p.SortOrder != nil {
		// sort_order 不限范围；但负数允许（admin 用负数把某条置顶）。无需校验。
		_ = fmt.Sprintf
	}
	return out, nil
}

// buildFaqEmbeddingText 构造 FAQ 的 embedding 输入文本（"Q:.../A:..." 模式）。
//
// 提取为 package-level 函数以便迁移服务（support_faq_migration.go）和重建逻辑
// 复用同样的格式 —— 否则两处文本格式漂移会导致检索分数计算口径不一致。
func buildFaqEmbeddingText(question, answer string) string {
	return "Q: " + question + "\nA: " + answer
}
