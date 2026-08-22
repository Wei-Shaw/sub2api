package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// 账号滚动窗口用量历史（纯被动统计，无开关、无主动探测）。
//
// 数据分两块：
//   - 窗口限额（使用率/重置时间）：由 AccountWindowUsageIngester 回放两条既有
//     观测流——①渠道监控明细历史（channel_monitor_histories 已持久化的按账号
//     配额快照）；②OpenAI/Codex 真实流量响应头归一化落库的 accounts.extra
//     快照（codex_5h/7d_used_percent + reset_at）。采集器只读不取（fetch），
//     不产生任何新的上游调用
//   - token 明细：窗口关闭后由 usage_logs 在 [window_start, window_end) 内
//     聚合重建（GetAccountWindowStatsRange），不依赖上游
//
// 管理端在「账号管理 → 查看统计」弹窗按窗口类型展示两者叠加的图表，
// 推算限额（token ÷ 最终使用率）随时间下降即为限额缩水信号。
// 注意：token 分子只含经本代理的用量，账号若同时在代理外消费，推算限额
// 会系统性偏低。

// 滚动窗口类型 token（与 domain.MonitorQuotaTier.Window 取值一致）。
// 仅记录带滚动窗口语义的类型；Gemini/Grok 的 daily、30d 与 Antigravity 的
// total 不在本功能语义内（未来需要时可扩展 duration 映射）。
const (
	windowTypeFiveHour       = "5h"
	windowTypeSevenDay       = "7d"
	windowTypeSevenDaySonnet = "7d-sonnet"
	windowTypeSevenDayFable  = "7d-fable"
	windowTypeWeekly         = "weekly"
)

// windowTypeDuration 窗口类型的窗口时长（用于推导 window_start 与 token 聚合边界）。
var windowTypeDuration = map[string]time.Duration{
	windowTypeFiveHour:       5 * time.Hour,
	windowTypeSevenDay:       7 * 24 * time.Hour,
	windowTypeSevenDaySonnet: 7 * 24 * time.Hour,
	windowTypeSevenDayFable:  7 * 24 * time.Hour,
	windowTypeWeekly:         7 * 24 * time.Hour,
}

// recordedWindow 判断 tier 窗口是否纳入记录。
func recordedWindow(windowType string) bool {
	_, ok := windowTypeDuration[windowType]
	return ok
}

// AccountWindowUsageRecord 单个滚动窗口的用量历史记录（一行）。
//
// finalized_at 为空表示「开放行」：当前窗口仍在滑动/使用中，quota 字段随采样
// 持续更新；窗口关闭后回填 token 明细并定格（局部唯一索引保证每账号每窗口
// 类型至多一行开放行）。
type AccountWindowUsageRecord struct {
	ID          int64     `json:"id"`
	AccountID   int64     `json:"account_id"`
	WindowType  string    `json:"window_type"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	// Peak/LastUsedPercent 窗口内峰值/最新使用率（0-100+，不截断）
	PeakUsedPercent float64 `json:"peak_used_percent"`
	LastUsedPercent float64 `json:"last_used_percent"`
	SampleCount     int     `json:"sample_count"`
	// LastSampleAt 行内最新采样的观测时刻：同一观测（时刻相同或更早）重复
	// 回放时 sample_count 恰好计数一次（多副本/重启回填幂等），单调前移
	LastSampleAt *time.Time `json:"last_sample_at"`
	// Requests/Tokens* finalize 后由 usage_logs 聚合回填；开放行为 nil
	Requests            *int64     `json:"requests"`
	TokensTotal         *int64     `json:"tokens_total"`
	TokensInput         *int64     `json:"tokens_input"`
	TokensOutput        *int64     `json:"tokens_output"`
	TokensCacheCreation *int64     `json:"tokens_cache_creation"`
	TokensCacheRead     *int64     `json:"tokens_cache_read"`
	FinalizedAt         *time.Time `json:"finalized_at"`
}

// AccountQuotaObservation 一次按账号的配额观测（被动源读出的最小单元）。
type AccountQuotaObservation struct {
	AccountID int64
	Snapshot  *domain.MonitorQuotaSnapshot
}

// AccountWindowUsageRepository 账号滚动窗口用量历史仓储。
//
// 刻意做成窄接口（而非往 AccountRepository 加方法）：AccountRepository 有多个
// 测试文件手写全量实现，加方法会连锁要求补 stub；本接口仅采集器与查询服务使用。
type AccountWindowUsageRepository interface {
	// GetOpenWindow 读取账号指定窗口类型的开放行；无开放行返回 (nil, nil)。
	GetOpenWindow(ctx context.Context, accountID int64, windowType string) (*AccountWindowUsageRecord, error)
	// UpsertOpenWindow 原子插入/合并开放行（ON CONFLICT 局部唯一索引）：
	// peak 取 GREATEST、last_sample_at 单调前移且相同时刻的重复观测不累加
	// sample_count、window_end 只前移不回退，并发/多副本回放天然幂等合并。
	UpsertOpenWindow(ctx context.Context, row *AccountWindowUsageRecord) error
	// FinalizeWindow 幂等关闭窗口：回填 token 明细并设置 finalized_at。
	// 行不存在或已关闭时返回 false（不报错）。
	FinalizeWindow(ctx context.Context, id int64, stats *usagestats.WindowTokenStats, now time.Time) (bool, error)
	// ReplaceOpenWindow 事务内「关闭旧开放行 + 写入新开放行」：状态机的
	// 旧窗口过期 → 新窗口路径，避免新窗口数据在并发下误并入旧窗口行。
	// 旧行已被并发关闭时静默跳过 finalize，仅写入新行。
	ReplaceOpenWindow(ctx context.Context, oldID int64, stats *usagestats.WindowTokenStats, newRow *AccountWindowUsageRecord, now time.Time) error
	// ListExpiredOpenWindows 列出 window_end < cutoff 的开放行（finalize 扫描）。
	// 按 window_end 升序。
	ListExpiredOpenWindows(ctx context.Context, cutoff time.Time, limit int) ([]*AccountWindowUsageRecord, error)
	// ListHistorySince 查询账号 window_end >= since 的历史（含开放行），
	// 按 window_type、window_end 升序。
	ListHistorySince(ctx context.Context, accountID int64, since time.Time) ([]*AccountWindowUsageRecord, error)
	// PruneFinalizedBefore 删除 finalized_at < cutoff 的已关闭行（保留期清理）。
	PruneFinalizedBefore(ctx context.Context, cutoff time.Time) (int64, error)
	// PruneStaleOpenBefore 删除 window_end < cutoff 的僵尸开放行
	// （账号软删/数据源消失后的兜底清理）。
	PruneStaleOpenBefore(ctx context.Context, cutoff time.Time) (int64, error)
	// ListMonitorQuotaHistorySince 读取渠道监控明细历史中 checked_at > since
	// 的按账号配额快照（被动源①，只读不探测），按 checked_at 升序。
	ListMonitorQuotaHistorySince(ctx context.Context, since time.Time, limit int) ([]*AccountQuotaObservation, error)
	// ListCodexUsageUpdatesSince 读取 openai 账号 extra 里 codex_* 快照的
	// 最近更新（被动源②，只读不探测）。
	ListCodexUsageUpdatesSince(ctx context.Context, since time.Time, limit int) ([]*AccountQuotaObservation, error)
}

// AccountWindowUsageEntry 管理端窗口历史接口的单窗口条目。
type AccountWindowUsageEntry struct {
	WindowStart         time.Time `json:"window_start"`
	WindowEnd           time.Time `json:"window_end"`
	Requests            *int64    `json:"requests"` // finalize 前为 null
	TokensTotal         *int64    `json:"tokens_total"`
	TokensInput         *int64    `json:"tokens_input"`
	TokensOutput        *int64    `json:"tokens_output"`
	TokensCacheCreation *int64    `json:"tokens_cache_creation"`
	TokensCacheRead     *int64    `json:"tokens_cache_read"`
	PeakUsedPercent     float64   `json:"peak_used_percent"`
	FinalUsedPercent    *float64  `json:"final_used_percent"` // 开放行为 null
	SampleCount         int       `json:"sample_count"`
	Finalized           bool      `json:"finalized"`
}

// AccountWindowHistoryResponse 管理端窗口历史接口响应。
// Windows 按窗口类型分组（key = "5h"/"7d"/...），组内旧 → 新。
type AccountWindowHistoryResponse struct {
	Windows map[string][]*AccountWindowUsageEntry `json:"windows"`
}

// AccountWindowUsageHistoryService 账号滚动窗口用量历史查询服务（管理端）。
type AccountWindowUsageHistoryService struct {
	windowRepo  AccountWindowUsageRepository
	accountRepo AccountRepository
}

// NewAccountWindowUsageHistoryService 构造查询服务。
func NewAccountWindowUsageHistoryService(
	windowRepo AccountWindowUsageRepository,
	accountRepo AccountRepository,
) *AccountWindowUsageHistoryService {
	return &AccountWindowUsageHistoryService{
		windowRepo:  windowRepo,
		accountRepo: accountRepo,
	}
}

// GetWindowHistory 查询账号近 days 天的滚动窗口用量历史。
// 账号不存在（含软删）返回空 Windows（宽松语义，弹窗在账号被并发删除时
// 静默收起本区块）。
func (s *AccountWindowUsageHistoryService) GetWindowHistory(ctx context.Context, accountID int64, days int) (*AccountWindowHistoryResponse, error) {
	resp := &AccountWindowHistoryResponse{Windows: map[string][]*AccountWindowUsageEntry{}}

	if _, err := s.accountRepo.GetByID(ctx, accountID); err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return resp, nil
		}
		return nil, fmt.Errorf("get account failed: %w", err)
	}

	since := time.Now().AddDate(0, 0, -days)
	records, err := s.windowRepo.ListHistorySince(ctx, accountID, since)
	if err != nil {
		return nil, fmt.Errorf("list window history failed: %w", err)
	}

	for _, rec := range records {
		if !recordedWindow(rec.WindowType) {
			continue
		}
		resp.Windows[rec.WindowType] = append(resp.Windows[rec.WindowType], windowRecordToEntry(rec))
	}
	return resp, nil
}

func windowRecordToEntry(rec *AccountWindowUsageRecord) *AccountWindowUsageEntry {
	entry := &AccountWindowUsageEntry{
		WindowStart:         rec.WindowStart,
		WindowEnd:           rec.WindowEnd,
		PeakUsedPercent:     rec.PeakUsedPercent,
		SampleCount:         rec.SampleCount,
		Requests:            rec.Requests,
		TokensTotal:         rec.TokensTotal,
		TokensInput:         rec.TokensInput,
		TokensOutput:        rec.TokensOutput,
		TokensCacheCreation: rec.TokensCacheCreation,
		TokensCacheRead:     rec.TokensCacheRead,
		Finalized:           rec.FinalizedAt != nil,
	}
	if rec.FinalizedAt != nil {
		final := rec.LastUsedPercent
		entry.FinalUsedPercent = &final
	}
	return entry
}
