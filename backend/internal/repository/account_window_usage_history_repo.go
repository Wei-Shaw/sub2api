package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/accountwindowusagehistory"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// accountWindowUsageRepository 实现 service.AccountWindowUsageRepository。
//
// 选型说明（同 channelMonitorRepository）：
//   - 简单读取走 ent，复用项目的事务上下文支持
//   - upsert/清理等集合语义走原生 SQL——局部唯一索引的 ON CONFLICT、
//     GREATEST/单调计数与条件 UPDATE 在 SQL 里表达最直接，也保证多副本并发安全
type accountWindowUsageRepository struct {
	client *dbent.Client
	db     *sql.DB
}

// NewAccountWindowUsageRepository 创建仓储实例。
func NewAccountWindowUsageRepository(client *dbent.Client, db *sql.DB) service.AccountWindowUsageRepository {
	return &accountWindowUsageRepository{client: client, db: db}
}

// accountWindowColumns 读取行的公共列清单（与 scanAccountWindowRows 配套）。
const accountWindowColumns = `id, account_id, window_type, window_start, window_end,
	peak_used_percent, last_used_percent,
	sample_count, last_sample_at,
	requests, tokens_total, tokens_input, tokens_output, tokens_cache_creation, tokens_cache_read,
	finalized_at`

// GetOpenWindow 读取账号指定窗口类型的开放行；无开放行返回 (nil, nil)。
func (r *accountWindowUsageRepository) GetOpenWindow(ctx context.Context, accountID int64, windowType string) (*service.AccountWindowUsageRecord, error) {
	client := clientFromContext(ctx, r.client)
	row, err := client.AccountWindowUsageHistory.Query().
		Where(
			accountwindowusagehistory.AccountIDEQ(accountID),
			accountwindowusagehistory.WindowTypeEQ(windowType),
			accountwindowusagehistory.FinalizedAtIsNil(),
		).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get open window failed: %w", err)
	}
	return entToAccountWindowRecord(row), nil
}

// ListHistorySince 查询账号 window_end >= since 的历史（含开放行）。
func (r *accountWindowUsageRepository) ListHistorySince(ctx context.Context, accountID int64, since time.Time) ([]*service.AccountWindowUsageRecord, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.AccountWindowUsageHistory.Query().
		Where(
			accountwindowusagehistory.AccountIDEQ(accountID),
			accountwindowusagehistory.WindowEndGTE(since),
		).
		Order(dbent.Asc(accountwindowusagehistory.FieldWindowType), dbent.Asc(accountwindowusagehistory.FieldWindowEnd)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list window history failed: %w", err)
	}
	records := make([]*service.AccountWindowUsageRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, entToAccountWindowRecord(row))
	}
	return records, nil
}

// accountWindowUpsertQuery 原子插入/合并开放行（UpsertOpenWindow 与
// ReplaceOpenWindow 共用）。
//
// 冲突目标为局部唯一索引 (account_id, window_type) WHERE finalized_at IS NULL。
// 合并语义：peak 取 GREATEST、last 直接覆盖；sample_count 仅在观测时刻
// （EXCLUDED.last_sample_at）晚于行内已见时刻时累加——同一观测被重复回放
// （多副本读取同一批监控历史 / 重启回填重扫）时恰好计数一次；
// window_end 只前移不回退（上游 reset 抖动/并发乱序均安全），随之前移时
// window_start 一并重算，保证 [start, end) 恰为最终窗口。
const accountWindowUpsertQuery = `
	INSERT INTO account_window_usage_histories
		(account_id, window_type, window_start, window_end,
		 peak_used_percent, last_used_percent, sample_count, last_sample_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	ON CONFLICT (account_id, window_type) WHERE finalized_at IS NULL
	DO UPDATE SET
		peak_used_percent = GREATEST(
			account_window_usage_histories.peak_used_percent, EXCLUDED.peak_used_percent),
		last_used_percent = EXCLUDED.last_used_percent,
		sample_count = account_window_usage_histories.sample_count
			+ CASE WHEN EXCLUDED.last_sample_at > account_window_usage_histories.last_sample_at
				THEN EXCLUDED.sample_count ELSE 0 END,
		last_sample_at = GREATEST(
			account_window_usage_histories.last_sample_at, EXCLUDED.last_sample_at),
		window_end = GREATEST(account_window_usage_histories.window_end, EXCLUDED.window_end),
		window_start = CASE
			WHEN EXCLUDED.window_end > account_window_usage_histories.window_end
			THEN EXCLUDED.window_start
			ELSE account_window_usage_histories.window_start END,
		updated_at = NOW()
`

// UpsertOpenWindow 原子插入/合并开放行。
func (r *accountWindowUsageRepository) UpsertOpenWindow(ctx context.Context, row *service.AccountWindowUsageRecord) error {
	_, err := r.db.ExecContext(ctx, accountWindowUpsertQuery,
		row.AccountID, row.WindowType, row.WindowStart, row.WindowEnd,
		row.PeakUsedPercent, row.LastUsedPercent, row.SampleCount, row.LastSampleAt,
	)
	if err != nil {
		return fmt.Errorf("upsert open window failed: %w", err)
	}
	return nil
}

// FinalizeWindow 幂等关闭窗口并回填 token 明细。
// finalized_at IS NULL 守卫使重复执行/多副本并发关闭均安全（后到者 no-op）。
func (r *accountWindowUsageRepository) FinalizeWindow(ctx context.Context, id int64, stats *usagestats.WindowTokenStats, now time.Time) (bool, error) {
	if stats == nil {
		stats = &usagestats.WindowTokenStats{}
	}
	query := `
		UPDATE account_window_usage_histories
		SET requests = $2, tokens_total = $3, tokens_input = $4, tokens_output = $5,
		    tokens_cache_creation = $6, tokens_cache_read = $7,
		    finalized_at = $8, updated_at = NOW()
		WHERE id = $1 AND finalized_at IS NULL
	`
	res, err := r.db.ExecContext(ctx, query,
		id, stats.Requests, stats.TokensTotal, stats.TokensInput, stats.TokensOutput,
		stats.TokensCacheCreation, stats.TokensCacheRead, now,
	)
	if err != nil {
		return false, fmt.Errorf("finalize window failed: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("finalize window rows affected: %w", err)
	}
	return affected == 1, nil
}

// ReplaceOpenWindow 事务内关闭旧开放行 + 写入新开放行（状态机的旧窗口过期路径）。
// 旧行已被并发关闭时 finalize no-op，仅写入新行。
//
// 注意：本方法自开事务，不得在外层事务上下文中调用（ent 事务上下文中的
// r.db 直连会脱离外层事务，造成半提交）。
func (r *accountWindowUsageRepository) ReplaceOpenWindow(ctx context.Context, oldID int64, stats *usagestats.WindowTokenStats, newRow *service.AccountWindowUsageRecord, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace open window begin tx failed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if stats == nil {
		stats = &usagestats.WindowTokenStats{}
	}
	finalizeQuery := `
		UPDATE account_window_usage_histories
		SET requests = $2, tokens_total = $3, tokens_input = $4, tokens_output = $5,
		    tokens_cache_creation = $6, tokens_cache_read = $7,
		    finalized_at = $8, updated_at = NOW()
		WHERE id = $1 AND finalized_at IS NULL
	`
	if _, err := tx.ExecContext(ctx, finalizeQuery,
		oldID, stats.Requests, stats.TokensTotal, stats.TokensInput, stats.TokensOutput,
		stats.TokensCacheCreation, stats.TokensCacheRead, now,
	); err != nil {
		return fmt.Errorf("replace open window finalize failed: %w", err)
	}

	if _, err := tx.ExecContext(ctx, accountWindowUpsertQuery,
		newRow.AccountID, newRow.WindowType, newRow.WindowStart, newRow.WindowEnd,
		newRow.PeakUsedPercent, newRow.LastUsedPercent, newRow.SampleCount, newRow.LastSampleAt,
	); err != nil {
		return fmt.Errorf("replace open window insert failed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replace open window commit failed: %w", err)
	}
	return nil
}

// ListExpiredOpenWindows 列出 window_end < cutoff 的开放行（finalize 扫描）。
func (r *accountWindowUsageRepository) ListExpiredOpenWindows(ctx context.Context, cutoff time.Time, limit int) ([]*service.AccountWindowUsageRecord, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM account_window_usage_histories
		WHERE finalized_at IS NULL AND window_end < $1
		ORDER BY window_end ASC
		LIMIT $2
	`, accountWindowColumns)
	rows, err := r.db.QueryContext(ctx, query, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("list expired open windows failed: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanAccountWindowRows(rows)
}

// PruneFinalizedBefore 删除保留期外的已关闭行。
func (r *accountWindowUsageRepository) PruneFinalizedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM account_window_usage_histories WHERE finalized_at IS NOT NULL AND finalized_at < $1`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("prune finalized windows failed: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("prune finalized windows rows affected: %w", err)
	}
	return affected, nil
}

// PruneStaleOpenBefore 删除僵尸开放行（账号软删/数据源消失后的兜底清理）。
func (r *accountWindowUsageRepository) PruneStaleOpenBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM account_window_usage_histories WHERE finalized_at IS NULL AND window_end < $1`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("prune stale open windows failed: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("prune stale open windows rows affected: %w", err)
	}
	return affected, nil
}

// ListMonitorQuotaHistorySince 读取渠道监控明细历史中 checked_at > since 的
// 配额快照（按账号），转成观测流供采集器回放。
//
// 被动源之一：监控检测已把按账号抓取的 MonitorQuotaSnapshot 持久化在
// channel_monitor_histories.quota（quota/quota_probe 模式），本查询只是读回，
// 不产生任何上游调用。快照观测时刻优先用快照自带的 fetched_at（真实抓取时间），
// 缺失时退回 checked_at。软删账号跳过。
func (r *accountWindowUsageRepository) ListMonitorQuotaHistorySince(ctx context.Context, since time.Time, limit int) ([]*service.AccountQuotaObservation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.account_id, h.quota, h.checked_at
		FROM channel_monitor_histories h
		JOIN channel_monitors m ON m.id = h.monitor_id
		JOIN accounts a ON a.id = m.account_id AND a.deleted_at IS NULL
		WHERE h.checked_at > $1 AND h.quota IS NOT NULL
		ORDER BY h.checked_at ASC
		LIMIT $2
	`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("list monitor quota history failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	observations := make([]*service.AccountQuotaObservation, 0, 16)
	for rows.Next() {
		var (
			accountID int64
			quotaRaw  []byte
			checkedAt time.Time
		)
		if err := rows.Scan(&accountID, &quotaRaw, &checkedAt); err != nil {
			return nil, fmt.Errorf("scan monitor quota history failed: %w", err)
		}
		snapshot := &domain.MonitorQuotaSnapshot{}
		if err := json.Unmarshal(quotaRaw, snapshot); err != nil {
			// 单行损坏不阻断整批：跳过
			continue
		}
		fetchedAt := snapshot.FetchedAt
		if fetchedAt.IsZero() {
			fetchedAt = checkedAt
		}
		snapshot.FetchedAt = fetchedAt
		observations = append(observations, &service.AccountQuotaObservation{
			AccountID: accountID,
			Snapshot:  snapshot,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate monitor quota history failed: %w", err)
	}
	return observations, nil
}

// ListCodexUsageUpdatesSince 读取 openai 账号 extra 里 codex_* 用量快照的
// 最近更新（更新时间 > since），转成观测流供采集器回放。
//
// 被动源之二：网关已把真实流量响应头（x-codex-primary-*）归一化写入
// accounts.extra（网关侧按账号 30s 节流落库），本查询只是读回，不产生任何
// 上游调用。快照观测时刻 = codex_usage_updated_at（写入时的基准时刻）。
//
// 过滤与排序都在解析后的 timestamptz 上做（RFC3339 文本含混合时区，字典序
// 不可比）；用 >= 而非 >：排序截断时同刻行可能落在 limit 之外，>= 保证下轮
// 重读（重复观测由 last_sample_at 去重），不丢数据。
func (r *accountWindowUsageRepository) ListCodexUsageUpdatesSince(ctx context.Context, since time.Time, limit int) ([]*service.AccountQuotaObservation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id,
		       extra->>'codex_usage_updated_at',
		       extra->>'codex_5h_used_percent',
		       extra->>'codex_5h_reset_at',
		       extra->>'codex_7d_used_percent',
		       extra->>'codex_7d_reset_at'
		FROM accounts
		WHERE platform = $1 AND deleted_at IS NULL
		  AND (extra->>'codex_usage_updated_at')::timestamptz >= $2
		ORDER BY (extra->>'codex_usage_updated_at')::timestamptz ASC, id ASC
		LIMIT $3
	`, domain.PlatformOpenAI, since, limit)
	if err != nil {
		return nil, fmt.Errorf("list codex usage updates failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	observations := make([]*service.AccountQuotaObservation, 0, 16)
	for rows.Next() {
		var (
			accountID    int64
			updatedAtStr string
			used5h       sql.NullString
			reset5h      sql.NullString
			used7d       sql.NullString
			reset7d      sql.NullString
		)
		if err := rows.Scan(&accountID, &updatedAtStr, &used5h, &reset5h, &used7d, &reset7d); err != nil {
			return nil, fmt.Errorf("scan codex usage update failed: %w", err)
		}
		fetchedAt, err := time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			continue // 时间戳异常的行跳过，不阻断整批
		}
		snapshot := &domain.MonitorQuotaSnapshot{Success: true, FetchedAt: fetchedAt}
		appendCodexTier := func(window string, used, reset sql.NullString) {
			usedPercent, err := strconvParseFloat(used)
			if err != nil || !reset.Valid || reset.String == "" {
				return
			}
			snapshot.Tiers = append(snapshot.Tiers, domain.MonitorQuotaTier{
				Window:      window,
				UsedPercent: usedPercent,
				ResetAt:     reset.String,
			})
		}
		// 窗口 token 与 service.recordedWindow 的键一致（"5h"/"7d"）
		appendCodexTier("5h", used5h, reset5h)
		appendCodexTier("7d", used7d, reset7d)
		if len(snapshot.Tiers) == 0 {
			continue
		}
		observations = append(observations, &service.AccountQuotaObservation{
			AccountID: accountID,
			Snapshot:  snapshot,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate codex usage updates failed: %w", err)
	}
	return observations, nil
}

// ---------- helpers ----------

// strconvParseFloat 解析可空文本列里的数值（JSONB number 经 ->> 输出为文本）。
func strconvParseFloat(v sql.NullString) (float64, error) {
	if !v.Valid || v.String == "" {
		return 0, fmt.Errorf("empty value")
	}
	return strconv.ParseFloat(v.String, 64)
}

// scanAccountWindowRows 扫描多行查询结果。
func scanAccountWindowRows(rows *sql.Rows) ([]*service.AccountWindowUsageRecord, error) {
	records := make([]*service.AccountWindowUsageRecord, 0, 16)
	for rows.Next() {
		rec := &service.AccountWindowUsageRecord{}
		if err := rows.Scan(
			&rec.ID, &rec.AccountID, &rec.WindowType, &rec.WindowStart, &rec.WindowEnd,
			&rec.PeakUsedPercent, &rec.LastUsedPercent,
			&rec.SampleCount, &rec.LastSampleAt,
			&rec.Requests, &rec.TokensTotal, &rec.TokensInput, &rec.TokensOutput,
			&rec.TokensCacheCreation, &rec.TokensCacheRead,
			&rec.FinalizedAt,
		); err != nil {
			return nil, fmt.Errorf("scan account window row failed: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account window rows failed: %w", err)
	}
	return records, nil
}

// entToAccountWindowRecord ent 行 → service 记录。
func entToAccountWindowRecord(row *dbent.AccountWindowUsageHistory) *service.AccountWindowUsageRecord {
	return &service.AccountWindowUsageRecord{
		ID:                  row.ID,
		AccountID:           row.AccountID,
		WindowType:          row.WindowType,
		WindowStart:         row.WindowStart,
		WindowEnd:           row.WindowEnd,
		PeakUsedPercent:     row.PeakUsedPercent,
		LastUsedPercent:     row.LastUsedPercent,
		SampleCount:         row.SampleCount,
		LastSampleAt:        row.LastSampleAt,
		Requests:            row.Requests,
		TokensTotal:         row.TokensTotal,
		TokensInput:         row.TokensInput,
		TokensOutput:        row.TokensOutput,
		TokensCacheCreation: row.TokensCacheCreation,
		TokensCacheRead:     row.TokensCacheRead,
		FinalizedAt:         row.FinalizedAt,
	}
}
