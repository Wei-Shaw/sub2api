// Package service — support_doc_indexer_cron.go
//
// add-support-knowledge-rag 的文档抓取定时调度（design.md D4）。
//
// 调度策略：
//   - 在 cron 上挂一条 "0 3 * * *"（每天 03:00）固定 entry；
//   - 触发时再从 settings 读取最新的 support_chat_rag_doc_cron 值决定是否真的跑：
//   - "daily-03" → 每次都跑（直接通过）；
//   - "weekly"   → 仅周一通过；
//   - "manual"   → 永远跳过（admin 后台按钮触发，不进 cron）。
//   - RAG 关闭 / doc_url 为空时也跳过（防御性 short-circuit）。
//
// 之所以用"单一 03:00 entry + fire-time 决策"而非"动态重载 cron 表达式"：
//   - 对 admin 修改设置无需 Reload 钩子，下一次 03:00 自然生效；
//   - 实现简单，cron 实例只创建一次（避免 Reload 的并发竞态）；
//   - 三种枚举值的语义都能精确表达（每天 / 每周一 / 手动）。
//
// 跨进程并发由 pipeline.Run 内部的 PG advisory lock 保证；本服务不再额外加 leader lock
// （即使两个实例同时 03:00 触发，第二个会因 advisory lock 失败而立即返回）。
package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// SupportDocIndexerCron 是文档抓取定时调度服务。
type SupportDocIndexerCron struct {
	settings *SettingService
	pipeline *SupportDocPipeline

	mu      sync.Mutex
	cron    *cron.Cron
	started bool
}

// NewSupportDocIndexerCron 构造调度器。
func NewSupportDocIndexerCron(settings *SettingService, pipeline *SupportDocPipeline) *SupportDocIndexerCron {
	return &SupportDocIndexerCron{
		settings: settings,
		pipeline: pipeline,
	}
}

// Start 注册 03:00 cron entry 并启动调度器。
//
// 多次调用安全：started=true 时直接返回。
func (c *SupportDocIndexerCron) Start() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return
	}

	// robfig/cron/v3 默认 5 字段（min hour dom mon dow）。03:00 每天触发。
	c.cron = cron.New()
	if _, err := c.cron.AddFunc("0 3 * * *", c.fireDailyEntry); err != nil {
		slog.Warn("support_doc_indexer_cron: register entry failed", slog.Any("err", err))
		return
	}
	c.cron.Start()
	c.started = true
	slog.Info("support_doc_indexer_cron: started",
		slog.String("schedule", "0 3 * * * (daily 03:00 fire-time gated)"))
}

// Stop 停止 cron 调度器。
func (c *SupportDocIndexerCron) Stop() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return
	}
	if c.cron != nil {
		ctx := c.cron.Stop()
		// 等待最多 30 秒让 in-flight job 收尾；超时不阻塞关闭。
		select {
		case <-ctx.Done():
		case <-time.After(30 * time.Second):
			slog.Warn("support_doc_indexer_cron: shutdown timeout, abandoning in-flight job")
		}
	}
	c.cron = nil
	c.started = false
}

// fireDailyEntry 是 03:00 cron entry 的触发函数。
//
// 决策树：
//  1. 读 settings：RAG 启用？doc_url 非空？doc_cron 值是什么？
//  2. doc_cron = manual → 跳过（仅手动按钮触发）。
//  3. doc_cron = weekly → 仅周一通过。
//  4. doc_cron = daily-03（默认） → 通过。
//  5. RAG 关闭或 doc_url 空 → 跳过。
//  6. 通过 → pipeline.RunAsync（advisory lock 自然兜底跨进程并发）。
func (c *SupportDocIndexerCron) fireDailyEntry() {
	if c == nil || c.settings == nil || c.pipeline == nil {
		return
	}
	bgctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rt := c.settings.GetSupportChatRAGRuntime(bgctx)

	if !rt.Enabled {
		slog.Info("support_doc_indexer_cron: skip - RAG disabled")
		return
	}
	if rt.DocURL == "" {
		slog.Info("support_doc_indexer_cron: skip - empty doc_url")
		return
	}

	switch rt.DocCron {
	case SupportChatRAGDocCronManual:
		slog.Info("support_doc_indexer_cron: skip - manual mode")
		return
	case SupportChatRAGDocCronWeekly:
		if time.Now().Weekday() != time.Monday {
			slog.Info("support_doc_indexer_cron: skip - weekly mode, today is not Monday")
			return
		}
	case SupportChatRAGDocCronDaily03:
		// 通过。
	default:
		// 未知值（可能是脏数据）：保守按 daily-03 处理。
		slog.Warn("support_doc_indexer_cron: unknown cron value, falling back to daily",
			slog.String("doc_cron", rt.DocCron))
	}

	slog.Info("support_doc_indexer_cron: firing pipeline async run",
		slog.String("doc_cron", rt.DocCron),
		slog.String("doc_url", rt.DocURL))
	c.pipeline.RunAsync()
}

// ProvideSupportDocIndexerCron wire helper：构造后立即 Start。
//
// 与项目其他 cron 类服务（OpsCleanupService / BackupService）保持一致：
// 在 wire ProviderSet 中通过 ProvideXxx 把 Start 副作用拉进 wire 图，
// 避免 cmd/server/main.go 显式调 Start 而漏调。
func ProvideSupportDocIndexerCron(settings *SettingService, pipeline *SupportDocPipeline) *SupportDocIndexerCron {
	c := NewSupportDocIndexerCron(settings, pipeline)
	c.Start()
	return c
}
