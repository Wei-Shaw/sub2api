package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// ChannelMonitorRepository 渠道监控数据访问接口。
// 入参/返回的指针类型均使用 service 包的 ChannelMonitor 模型，
// repository 实现负责与 ent 模型互转，并保持 api_key_encrypted 字段为密文。
type ChannelMonitorRepository interface {
	// CRUD
	Create(ctx context.Context, m *ChannelMonitor) error
	GetByID(ctx context.Context, id int64) (*ChannelMonitor, error)
	Update(ctx context.Context, m *ChannelMonitor) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params ChannelMonitorListParams) ([]*ChannelMonitor, int64, error)

	// 调度器辅助
	ListEnabled(ctx context.Context) ([]*ChannelMonitor, error)
	MarkChecked(ctx context.Context, id int64, checkedAt time.Time) error
	InsertHistoryBatch(ctx context.Context, rows []*ChannelMonitorHistoryRow) error
	DeleteHistoryBefore(ctx context.Context, before time.Time) (int64, error)

	// 历史记录
	ListHistory(ctx context.Context, monitorID int64, model string, limit int) ([]*ChannelMonitorHistoryEntry, error)

	// 用户视图聚合
	ListLatestPerModel(ctx context.Context, monitorID int64) ([]*ChannelMonitorLatest, error)
	ComputeAvailability(ctx context.Context, monitorID int64, windowDays int) ([]*ChannelMonitorAvailability, error)

	// 批量聚合（admin/user list 用，避免 N+1）
	ListLatestForMonitorIDs(ctx context.Context, ids []int64) (map[int64][]*ChannelMonitorLatest, error)
	ComputeAvailabilityForMonitors(ctx context.Context, ids []int64, windowDays int) (map[int64][]*ChannelMonitorAvailability, error)
	// ListRecentHistoryForMonitors 批量取多个 monitor 各自主模型（primaryModels[monitorID]）最近 perMonitorLimit 条历史。
	// 返回的 entry 已按 checked_at DESC 排序（最新在前），不含 message 字段。
	ListRecentHistoryForMonitors(ctx context.Context, ids []int64, primaryModels map[int64]string, perMonitorLimit int) (map[int64][]*ChannelMonitorHistoryEntry, error)
}

// ChannelMonitorService 渠道监控管理服务。
type ChannelMonitorService struct {
	repo      ChannelMonitorRepository
	encryptor SecretEncryptor
}

// NewChannelMonitorService 创建渠道监控服务实例。
func NewChannelMonitorService(repo ChannelMonitorRepository, encryptor SecretEncryptor) *ChannelMonitorService {
	return &ChannelMonitorService{repo: repo, encryptor: encryptor}
}

// ---------- CRUD ----------

// List 列表查询（支持 provider/enabled/search 过滤 + 分页）。
// 返回的 ChannelMonitor.APIKey 已解密为明文，handler 层负责脱敏。
func (s *ChannelMonitorService) List(ctx context.Context, params ChannelMonitorListParams) ([]*ChannelMonitor, int64, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 || params.PageSize > 200 {
		params.PageSize = 20
	}
	items, total, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list channel monitors: %w", err)
	}
	for _, it := range items {
		s.decryptInPlace(it)
	}
	return items, total, nil
}

// Get 查询单个监控（解密 API Key）。
func (s *ChannelMonitorService) Get(ctx context.Context, id int64) (*ChannelMonitor, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.decryptInPlace(m)
	return m, nil
}

// Create 创建监控（内部加密 api_key）。
func (s *ChannelMonitorService) Create(ctx context.Context, p ChannelMonitorCreateParams) (*ChannelMonitor, error) {
	if err := validateCreateParams(p); err != nil {
		return nil, err
	}
	encrypted, err := s.encryptor.Encrypt(p.APIKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt api key: %w", err)
	}
	m := &ChannelMonitor{
		Name:            strings.TrimSpace(p.Name),
		Provider:        p.Provider,
		Endpoint:        normalizeEndpoint(p.Endpoint),
		APIKey:          encrypted, // 注意：传入 repository 时该字段为密文
		PrimaryModel:    strings.TrimSpace(p.PrimaryModel),
		ExtraModels:     normalizeModels(p.ExtraModels),
		GroupName:       strings.TrimSpace(p.GroupName),
		Enabled:         p.Enabled,
		IntervalSeconds: p.IntervalSeconds,
		CreatedBy:       p.CreatedBy,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("create channel monitor: %w", err)
	}
	// 不再调 s.Get 重走解密链：已知刚加密的明文，直接构造响应。
	// 这样可避免 SecretEncryptor 解密失败时 APIKey 被静默清空的问题（见 Fix 4）。
	m.APIKey = strings.TrimSpace(p.APIKey)
	return m, nil
}

// validateCreateParams 把 Create 入参的所有校验聚拢为一个函数，避免 Create 主体超过 30 行。
func validateCreateParams(p ChannelMonitorCreateParams) error {
	if err := validateProvider(p.Provider); err != nil {
		return err
	}
	if err := validateInterval(p.IntervalSeconds); err != nil {
		return err
	}
	if err := validateEndpoint(p.Endpoint); err != nil {
		return err
	}
	if strings.TrimSpace(p.APIKey) == "" {
		return ErrChannelMonitorMissingAPIKey
	}
	if strings.TrimSpace(p.PrimaryModel) == "" {
		return ErrChannelMonitorMissingPrimaryModel
	}
	return nil
}

// Update 更新监控。APIKey 字段：nil 或空字符串 = 不修改；非空 = 加密后覆盖。
func (s *ChannelMonitorService) Update(ctx context.Context, id int64, p ChannelMonitorUpdateParams) (*ChannelMonitor, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := applyMonitorUpdate(existing, p); err != nil {
		return nil, err
	}

	newPlainAPIKey, apiKeyUpdated, err := s.applyAPIKeyUpdate(existing, p.APIKey)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update channel monitor: %w", err)
	}

	// 不再调 s.Get 重走解密链：避免二次解密带来的"密文被静默清空"风险（与 Create 一致）。
	if apiKeyUpdated {
		existing.APIKey = newPlainAPIKey
	} else {
		s.decryptInPlace(existing)
	}
	return existing, nil
}

// applyAPIKeyUpdate 处理 Update 中的 APIKey 字段：
//   - 入参 raw 为 nil 或空白：不修改 existing.APIKey（仍为密文），返回 updated=false
//   - 非空：加密后写入 existing.APIKey；同时把明文返回给调用方，
//     供写库成功后塞回 existing 避免把密文吐回客户端
func (s *ChannelMonitorService) applyAPIKeyUpdate(existing *ChannelMonitor, raw *string) (plain string, updated bool, err error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return "", false, nil
	}
	plain = strings.TrimSpace(*raw)
	encrypted, encErr := s.encryptor.Encrypt(plain)
	if encErr != nil {
		return "", false, fmt.Errorf("encrypt api key: %w", encErr)
	}
	existing.APIKey = encrypted
	return plain, true, nil
}

// Delete 删除监控（历史通过外键 CASCADE 自动清理）。
func (s *ChannelMonitorService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete channel monitor: %w", err)
	}
	return nil
}

// ListHistory 列出某个监控最近的检测历史。
// model 为空表示返回所有模型；limit <= 0 时使用默认值，超过上限会被截断。
func (s *ChannelMonitorService) ListHistory(ctx context.Context, id int64, model string, limit int) ([]*ChannelMonitorHistoryEntry, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = MonitorHistoryDefaultLimit
	}
	if limit > MonitorHistoryMaxLimit {
		limit = MonitorHistoryMaxLimit
	}
	entries, err := s.repo.ListHistory(ctx, id, strings.TrimSpace(model), limit)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	return entries, nil
}

// ---------- 业务 ----------

// RunCheck 同步触发对一个监控的检测：并发跑 primary + extra 模型，
// 写历史记录并更新 last_checked_at。返回每个模型的检测结果。
func (s *ChannelMonitorService) RunCheck(ctx context.Context, id int64) ([]*CheckResult, error) {
	m, err := s.Get(ctx, id) // 已解密 APIKey
	if err != nil {
		return nil, err
	}
	if m.APIKeyDecryptFailed {
		return nil, ErrChannelMonitorAPIKeyDecryptFailed
	}
	results := s.runChecksConcurrent(ctx, m)
	s.persistCheckResults(ctx, m, results)
	return results, nil
}

// persistCheckResults 写入本次检测的历史记录并更新 last_checked_at。
// 任一写库失败都只记日志，不影响调用方拿到 results（与 MVP 期望一致：宁可漏记历史也要先返回结果）。
func (s *ChannelMonitorService) persistCheckResults(ctx context.Context, m *ChannelMonitor, results []*CheckResult) {
	rows := make([]*ChannelMonitorHistoryRow, 0, len(results))
	for _, r := range results {
		rows = append(rows, &ChannelMonitorHistoryRow{
			MonitorID:     m.ID,
			Model:         r.Model,
			Status:        r.Status,
			LatencyMs:     r.LatencyMs,
			PingLatencyMs: r.PingLatencyMs,
			Message:       r.Message,
			CheckedAt:     r.CheckedAt,
		})
	}
	if err := s.repo.InsertHistoryBatch(ctx, rows); err != nil {
		slog.Error("channel_monitor: insert history failed",
			"monitor_id", m.ID, "name", m.Name, "error", err)
	}
	if err := s.repo.MarkChecked(ctx, m.ID, time.Now()); err != nil {
		slog.Error("channel_monitor: mark checked failed",
			"monitor_id", m.ID, "error", err)
	}
}

// runChecksConcurrent 对 primary + extra 模型并发执行检测。
// errgroup 仅用于等待，不传播错误（每个 model 失败都已打包进 CheckResult）。
func (s *ChannelMonitorService) runChecksConcurrent(ctx context.Context, m *ChannelMonitor) []*CheckResult {
	models := append([]string{m.PrimaryModel}, m.ExtraModels...)
	results := make([]*CheckResult, len(models))

	// ping 共享一次，所有模型记录同一个 ping 延迟。
	pingMs := pingEndpointOrigin(ctx, m.Endpoint)

	var eg errgroup.Group
	var mu sync.Mutex
	for i, model := range models {
		i, model := i, model
		eg.Go(func() error {
			r := runCheckForModel(ctx, m.Provider, m.Endpoint, m.APIKey, model)
			r.PingLatencyMs = pingMs
			mu.Lock()
			results[i] = r
			mu.Unlock()
			return nil
		})
	}
	_ = eg.Wait()
	return results
}

// ---------- 调度器内部 ----------

// listDueForCheck 返回需要立即检测的监控列表：
// enabled=true AND (last_checked_at IS NULL OR last_checked_at + interval <= now)。
// 实现下沉到 repository（用 SQL 表达式比较），减少应用层数据传输。
func (s *ChannelMonitorService) listDueForCheck(ctx context.Context) ([]*ChannelMonitor, error) {
	all, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	due := make([]*ChannelMonitor, 0, len(all))
	for _, m := range all {
		if m.LastCheckedAt == nil {
			due = append(due, m)
			continue
		}
		nextAt := m.LastCheckedAt.Add(time.Duration(m.IntervalSeconds) * time.Second)
		if !nextAt.After(now) {
			due = append(due, m)
		}
	}
	return due, nil
}

// cleanupOldHistory 删除 monitorHistoryRetentionDays 天之前的历史记录。
func (s *ChannelMonitorService) cleanupOldHistory(ctx context.Context) error {
	before := time.Now().AddDate(0, 0, -monitorHistoryRetentionDays)
	deleted, err := s.repo.DeleteHistoryBefore(ctx, before)
	if err != nil {
		return fmt.Errorf("delete history before %s: %w", before.Format(time.RFC3339), err)
	}
	if deleted > 0 {
		slog.Info("channel_monitor: history cleanup",
			"deleted_rows", deleted, "before", before.Format(time.RFC3339))
	}
	return nil
}

// ---------- helpers ----------

// decryptInPlace 把 ChannelMonitor.APIKey 从密文解密为明文。
// 解密失败时把字段清空 + 设置 APIKeyDecryptFailed=true（不返回错误，避免阻断列表渲染）。
// runner / RunCheck 必须读取该标志位并拒绝执行检测。
func (s *ChannelMonitorService) decryptInPlace(m *ChannelMonitor) {
	if m == nil || m.APIKey == "" {
		return
	}
	plain, err := s.encryptor.Decrypt(m.APIKey)
	if err != nil {
		slog.Warn("channel_monitor: decrypt api key failed",
			"monitor_id", m.ID, "error", err)
		m.APIKey = ""
		m.APIKeyDecryptFailed = true
		return
	}
	m.APIKey = plain
}

// applyMonitorUpdate 把 update params 中非 nil 的字段应用到 existing 上。
// APIKey 字段在调用方单独处理（涉及加密）。
//
// 行数稍超过 30：这是逐字段平铺的 dispatcher，每个 if 都是 1-3 行的"非 nil 则覆盖"模式，
// 拆分反而会增加跳转噪音、影响可读性，故保留为单函数。
func applyMonitorUpdate(existing *ChannelMonitor, p ChannelMonitorUpdateParams) error {
	if p.Name != nil {
		existing.Name = strings.TrimSpace(*p.Name)
	}
	if p.Provider != nil {
		if err := validateProvider(*p.Provider); err != nil {
			return err
		}
		existing.Provider = *p.Provider
	}
	if p.Endpoint != nil {
		if err := validateEndpoint(*p.Endpoint); err != nil {
			return err
		}
		existing.Endpoint = normalizeEndpoint(*p.Endpoint)
	}
	if p.PrimaryModel != nil {
		existing.PrimaryModel = strings.TrimSpace(*p.PrimaryModel)
	}
	if p.ExtraModels != nil {
		existing.ExtraModels = normalizeModels(*p.ExtraModels)
	}
	if p.GroupName != nil {
		existing.GroupName = strings.TrimSpace(*p.GroupName)
	}
	if p.Enabled != nil {
		existing.Enabled = *p.Enabled
	}
	if p.IntervalSeconds != nil {
		if err := validateInterval(*p.IntervalSeconds); err != nil {
			return err
		}
		existing.IntervalSeconds = *p.IntervalSeconds
	}
	return nil
}
