package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	auditLogQueueCapacity = 4096
	auditLogBatchSize     = 100
	auditLogFlushInterval = time.Second

	auditRetentionCheckInterval = 24 * time.Hour
	auditRetentionStartupDelay  = 5 * time.Minute
	auditRetentionBatchSize     = 5000
)

// AuditLogService 管理面操作审计日志服务。
// 写入端为非阻塞异步批量落库（不拖慢管理请求）；
// 读取端提供分页查询；清空端点由 handler 层做 TOTP 强校验后调用 ClearAll。
type AuditLogService struct {
	repo           AuditLogRepository
	settingService *SettingService

	queue chan auditLogCommand

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started atomic.Bool

	droppedCount uint64
	writeFailed  uint64
	writtenCount uint64
}

type auditLogCommand struct {
	entry *AuditLog
	clear *auditLogClearCommand
}

type auditLogClearCommand struct {
	ctx    context.Context
	trace  *AuditLog
	result chan auditLogClearResult
}

type auditLogClearResult struct {
	deleted int64
	err     error
}

type auditLogBatchWriter struct {
	service *AuditLogService
	batch   []*AuditLog
}

func NewAuditLogService(repo AuditLogRepository, settingService *SettingService) *AuditLogService {
	ctx, cancel := context.WithCancel(context.Background())
	return &AuditLogService{
		repo:           repo,
		settingService: settingService,
		queue:          make(chan auditLogCommand, auditLogQueueCapacity),
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start 启动异步写入与保留期清理协程。
func (s *AuditLogService) Start() {
	if s == nil || s.repo == nil || !s.started.CompareAndSwap(false, true) {
		return
	}
	s.wg.Add(2)
	go s.runWriter()
	go s.runRetentionLoop()
}

// Stop 停止服务并尽量落盘队列中剩余记录。
func (s *AuditLogService) Stop() {
	if s == nil {
		return
	}
	s.cancel()
	s.wg.Wait()
}

// Record 非阻塞入队一条审计记录；队列打满时丢弃并计数（管理面流量下几乎不可能发生）。
func (s *AuditLogService) Record(entry *AuditLog) {
	if s == nil || entry == nil {
		return
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	select {
	case <-s.ctx.Done():
		return
	default:
	}
	select {
	case s.queue <- auditLogCommand{entry: entry}:
	default:
		atomic.AddUint64(&s.droppedCount, 1)
	}
}

// List 分页查询审计日志。
func (s *AuditLogService) List(ctx context.Context, filter *AuditLogFilter) (*AuditLogList, error) {
	return s.repo.List(ctx, filter)
}

// GetByID 查询单条详情。
func (s *AuditLogService) GetByID(ctx context.Context, id int64) (*AuditLog, error) {
	return s.repo.GetByID(ctx, id)
}

// ClearAll 全量清空审计日志并写入留痕记录。
// 调用方（handler）必须先完成 TOTP 验证；本方法负责：
//  1. 排在本命令之前的异步记录先落库
//  2. repository 在单一事务中统计、清空并写入 "audit_log.clear" 留痕
func (s *AuditLogService) ClearAll(ctx context.Context, trace *AuditLog) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, errors.New("audit log service is unavailable")
	}
	if !s.started.Load() {
		return 0, errors.New("audit log service is not started")
	}
	if trace == nil {
		return 0, errors.New("audit clear trace is required")
	}
	trace.Action = AuditActionAuditLogClear
	if trace.CreatedAt.IsZero() {
		trace.CreatedAt = time.Now().UTC()
	}
	result := make(chan auditLogClearResult, 1)
	command := auditLogCommand{clear: &auditLogClearCommand{ctx: ctx, trace: trace, result: result}}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-s.ctx.Done():
		return 0, errors.New("audit log service is stopping")
	case s.queue <- command:
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-s.ctx.Done():
		return 0, errors.New("audit log service is stopping")
	case response := <-result:
		return response.deleted, response.err
	}
}

func (s *AuditLogService) runWriter() {
	defer s.wg.Done()

	ticker := time.NewTicker(auditLogFlushInterval)
	defer ticker.Stop()
	writer := auditLogBatchWriter{service: s, batch: make([]*AuditLog, 0, auditLogBatchSize)}

	for {
		select {
		case <-s.ctx.Done():
			// 停机前排空队列。
			for {
				select {
				case command := <-s.queue:
					writer.handle(command)
				default:
					_ = writer.flush()
					return
				}
			}
		case command := <-s.queue:
			writer.handle(command)
		case <-ticker.C:
			_ = writer.flush()
		}
	}
}

func (w *auditLogBatchWriter) flush() error {
	if len(w.batch) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	inserted, err := w.service.repo.BatchInsert(ctx, w.batch)
	cancel()
	if err != nil {
		atomic.AddUint64(&w.service.writeFailed, uint64(len(w.batch)))
		_, _ = fmt.Fprintf(os.Stderr, "time=%s level=WARN msg=\"audit log flush failed\" err=%v batch=%d\n",
			time.Now().Format(time.RFC3339Nano), err, len(w.batch))
	} else {
		atomic.AddUint64(&w.service.writtenCount, uint64(inserted))
	}
	w.batch = w.batch[:0]
	return err
}

func (w *auditLogBatchWriter) handle(command auditLogCommand) {
	if command.entry != nil {
		w.batch = append(w.batch, command.entry)
		if len(w.batch) >= auditLogBatchSize {
			_ = w.flush()
		}
		return
	}
	if command.clear == nil {
		return
	}
	if err := w.flush(); err != nil {
		command.clear.result <- auditLogClearResult{err: fmt.Errorf("flush pending audit logs: %w", err)}
		return
	}
	deleted, err := w.service.repo.ClearAll(command.clear.ctx, command.clear.trace)
	if err != nil {
		err = fmt.Errorf("clear audit logs: %w", err)
	}
	command.clear.result <- auditLogClearResult{deleted: deleted, err: err}
}

// runRetentionLoop 按保留期定期删除过期审计日志。
// 删除操作幂等，多实例并发执行无害，因此无需选主。
func (s *AuditLogService) runRetentionLoop() {
	defer s.wg.Done()

	startupTimer := time.NewTimer(auditRetentionStartupDelay)
	defer startupTimer.Stop()
	select {
	case <-s.ctx.Done():
		return
	case <-startupTimer.C:
	}

	ticker := time.NewTicker(auditRetentionCheckInterval)
	defer ticker.Stop()

	s.runRetentionOnce()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.runRetentionOnce()
		}
	}
}

func (s *AuditLogService) runRetentionOnce() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
	defer cancel()

	days := 0
	if s.settingService != nil {
		days = s.settingService.GetAuditLogRetentionDays(ctx)
	}
	if days <= 0 {
		return // 0 或负值表示永久保留，仅支持手动清空
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	for {
		deleted, err := s.repo.DeleteBefore(ctx, cutoff, auditRetentionBatchSize)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "time=%s level=WARN msg=\"audit log retention cleanup failed\" err=%v\n",
				time.Now().Format(time.RFC3339Nano), err)
			return
		}
		if deleted == 0 {
			return
		}
	}
}
