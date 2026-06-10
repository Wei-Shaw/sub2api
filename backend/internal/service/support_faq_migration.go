// Package service — support_faq_migration.go
//
// add-support-knowledge-rag 启动数据迁移：把 add-support-chat-widget 时代存在
// `support_chat_faqs` JSON setting 中的 FAQ 条目搬到 `support_faq_items` 新表
// （design.md D8 / tasks 11.1-11.3）。
//
// 幂等条件（避免重复迁移）：
//
//   - 表 support_faq_items 行数 > 0 → 直接 return（视作已迁移过 / 已有 admin 录入）；
//   - 表为空 + setting 也为空 → 无需做任何事；
//   - 表为空 + setting 非空 → 执行迁移：
//     a) 把 setting 中每一条 FAQ INSERT 到表（embedding=NULL）；
//     b) 起一个后台 goroutine 跑 EmbedBatch 批量补 embedding，失败保留 NULL；
//     c) 不删 setting key（保留作为回滚兜底，详见 design D8 README）。
//
// 调用时机：服务启动后、cron 启动前。由 cmd/server/main.go 显式调一次。
package service

import (
	"context"
	"log/slog"
)

// SupportFaqMigrationService 封装一次性迁移逻辑。
type SupportFaqMigrationService struct {
	settings  *SettingService
	repo      SupportFaqRepository
	embedding EmbeddingService
}

// NewSupportFaqMigrationService 构造迁移服务。
func NewSupportFaqMigrationService(
	settings *SettingService,
	repo SupportFaqRepository,
	embedding EmbeddingService,
) *SupportFaqMigrationService {
	return &SupportFaqMigrationService{
		settings:  settings,
		repo:      repo,
		embedding: embedding,
	}
}

// Run 执行迁移（同步部分：写表）+ 异步部分（embed 补回）。
//
// 任何错误只 log，不冒泡 —— 启动迁移失败不应阻塞服务启动；admin 后台后续可手动
// 触发 reindex 补救。
//
// 返回 (migratedCount, asyncEmbeddingScheduled)：
//   - migratedCount 是本次实际写入的 FAQ 行数；
//   - asyncEmbeddingScheduled = true 时表示 embed goroutine 已 spawned。
func (s *SupportFaqMigrationService) Run(ctx context.Context) (int, bool) {
	if s == nil || s.settings == nil || s.repo == nil {
		return 0, false
	}

	// 1. 表已有数据 → 跳过。
	count, err := s.repo.CountAll(ctx)
	if err != nil {
		slog.WarnContext(ctx, "support_faq_migration: count failed, skipping",
			slog.Any("err", err))
		return 0, false
	}
	if count > 0 {
		slog.Info("support_faq_migration: skip - table not empty",
			slog.Int("rows", count))
		return 0, false
	}

	// 2. 读 legacy setting（add-support-chat-widget 时代写入的 JSON）。
	legacyValue, lerr := s.settings.settingRepo.Get(ctx, SettingKeySupportChatFAQs)
	if lerr != nil {
		slog.WarnContext(ctx, "support_faq_migration: read legacy setting failed",
			slog.Any("err", lerr))
		return 0, false
	}
	if legacyValue == nil || legacyValue.Value == "" {
		slog.Info("support_faq_migration: skip - legacy setting empty")
		return 0, false
	}
	legacy := ParseSupportChatFAQs(legacyValue.Value)
	if len(legacy) == 0 {
		slog.Info("support_faq_migration: skip - legacy setting parse to empty")
		return 0, false
	}

	// 3. 同步写表（每条单独 INSERT；表是新表，行数极小 ≤ 数十条）。
	created := make([]int64, 0, len(legacy))
	for i, f := range legacy {
		item := SupportFaqItem{
			Question:  f.Question,
			Answer:    f.Answer,
			Tags:      []string{},
			Enabled:   f.Enabled,
			SortOrder: f.SortOrder,
		}
		if item.SortOrder == 0 {
			item.SortOrder = (i + 1) * 10 // 给个稳定的初始排序
		}
		if err := s.repo.Create(ctx, &item); err != nil {
			slog.WarnContext(ctx, "support_faq_migration: insert failed, continuing",
				slog.String("question", f.Question),
				slog.Any("err", err))
			continue
		}
		created = append(created, item.ID)
	}
	slog.Info("support_faq_migration: legacy faqs migrated",
		slog.Int("inserted", len(created)),
		slog.Int("legacy_total", len(legacy)))

	// 4. 异步补 embedding（不阻塞启动）。
	if len(created) == 0 || s.embedding == nil {
		return len(created), false
	}
	go s.backfillEmbeddings(created)
	return len(created), true
}

// backfillEmbeddings 把刚迁移的行批量补 embedding。
//
// 失败时单条日志即可，不重试 —— admin 可在后台手动触发 reindex 补救。
func (s *SupportFaqMigrationService) backfillEmbeddings(ids []int64) {
	bgctx := context.Background()
	for _, id := range ids {
		item, err := s.repo.GetByID(bgctx, id)
		if err != nil || item == nil {
			continue
		}
		text := buildFaqEmbeddingText(item.Question, item.Answer)
		vec, eerr := s.embedding.Embed(bgctx, text)
		if eerr != nil {
			slog.Warn("support_faq_migration: embed failed for migrated row",
				slog.Int64("id", id),
				slog.Any("err", eerr))
			continue
		}
		if err := s.repo.SetEmbedding(bgctx, id, vec); err != nil {
			slog.Warn("support_faq_migration: set embedding failed",
				slog.Int64("id", id),
				slog.Any("err", err))
		}
	}
	slog.Info("support_faq_migration: backfill embeddings finished",
		slog.Int("count", len(ids)))
}

// ProvideSupportFaqMigrationService wire helper：构造 + 启动后台 Run。
//
// 与项目其他启动钩子（NewSchedulerSnapshotService → Start）一致：把启动副作用封进
// wire 图，避免 main.go 显式调用而漏调。
//
// 用 goroutine 包裹 Run 是因为：
//   - 迁移本身是 IO（DB + embedding self-call），不应阻塞依赖图初始化；
//   - 若失败也只 log，不影响其他服务。
func ProvideSupportFaqMigrationService(
	settings *SettingService,
	repo SupportFaqRepository,
	embedding EmbeddingService,
) *SupportFaqMigrationService {
	svc := NewSupportFaqMigrationService(settings, repo, embedding)
	go func() {
		// 用 background ctx；启动时 root context 还未建立。
		_, _ = svc.Run(context.Background())
	}()
	return svc
}
