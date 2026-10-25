package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type schedulerOutboxRepository struct {
	db *sql.DB
}

type schedulerOutboxCleanupLease struct {
	conn *sql.Conn
}

const schedulerOutboxDefaultCleanSize = 5000

// schedulerOutboxCleanupLockNameMySQL 是清理锁在 MySQL 下的命名锁名称
// （对应 PG 版 pg_try_advisory_lock(hashtext('scheduler_outbox_cleanup'))）。
const schedulerOutboxCleanupLockNameMySQL = "scheduler_outbox_cleanup"

func NewSchedulerOutboxRepository(db *sql.DB) service.SchedulerOutboxRepository {
	return &schedulerOutboxRepository{db: db}
}

func (r *schedulerOutboxRepository) ListAfterAndReleaseDedup(ctx context.Context, afterID int64, limit int) ([]service.SchedulerOutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if IsMySQL() {
		return r.listAfterAndReleaseDedupMySQL(ctx, afterID, limit)
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH selected AS MATERIALIZED (
			SELECT id, event_type, account_id, group_id, payload, created_at
			FROM scheduler_outbox
			WHERE id > $1
			ORDER BY id ASC
			LIMIT $2
			FOR UPDATE
		), released AS (
			UPDATE scheduler_outbox AS o
			SET dedup_key = NULL
			FROM selected AS s
			WHERE o.id = s.id
				AND o.dedup_key IS NOT NULL
			RETURNING o.id
		)
		SELECT s.id, s.event_type, s.account_id, s.group_id, s.payload, s.created_at
		FROM selected AS s
		CROSS JOIN (SELECT COUNT(*) FROM released) AS release_barrier
		ORDER BY s.id ASC
	`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	events := make([]service.SchedulerOutboxEvent, 0, limit)
	for rows.Next() {
		var (
			payloadRaw []byte
			accountID  sql.NullInt64
			groupID    sql.NullInt64
			event      service.SchedulerOutboxEvent
		)
		if err := rows.Scan(&event.ID, &event.EventType, &accountID, &groupID, &payloadRaw, &event.CreatedAt); err != nil {
			return nil, err
		}
		if accountID.Valid {
			v := accountID.Int64
			event.AccountID = &v
		}
		if groupID.Valid {
			v := groupID.Int64
			event.GroupID = &v
		}
		if len(payloadRaw) > 0 {
			var payload map[string]any
			if err := json.Unmarshal(payloadRaw, &payload); err != nil {
				return nil, err
			}
			event.Payload = payload
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *schedulerOutboxRepository) FirstCreatedAtAfter(ctx context.Context, afterID int64) (time.Time, bool, error) {
	if IsMySQL() {
		return r.firstCreatedAtAfterMySQL(ctx, afterID)
	}
	var createdAt time.Time
	err := r.db.QueryRowContext(ctx, `
		SELECT created_at
		FROM scheduler_outbox
		WHERE id > $1
		ORDER BY id ASC
		LIMIT 1
	`, afterID).Scan(&createdAt)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return createdAt, true, nil
}

func (r *schedulerOutboxRepository) MaxID(ctx context.Context) (int64, error) {
	var maxID int64
	if err := r.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM scheduler_outbox").Scan(&maxID); err != nil {
		return 0, err
	}
	return maxID, nil
}

func (r *schedulerOutboxRepository) DeleteConsumedUpTo(ctx context.Context, watermark int64, limit int) (int64, error) {
	if watermark <= 0 {
		return 0, nil
	}
	if limit <= 0 {
		limit = schedulerOutboxDefaultCleanSize
	}
	if IsMySQL() {
		return r.deleteConsumedUpToMySQL(ctx, watermark, limit)
	}
	// created_at < NOW() - INTERVAL '10 seconds' 防御 PG 序列号在事务内提前分配但
	// 提交延迟的竞争：若某 Tx 在 watermark 推进前持有 id=N（未提交），watermark
	// 跨过 N 后该 Tx 才提交，此时 row N 已经"低于 watermark"但从未被 poll；10s
	// 宽限期让此类慢事务有机会提交后被消费，再被 cleanup 删除。
	result, err := r.db.ExecContext(ctx, `
		WITH doomed AS (
			SELECT id
			FROM scheduler_outbox
			WHERE id <= $1
				AND created_at < NOW() - INTERVAL '10 seconds'
			ORDER BY id ASC
			LIMIT $2
		)
		DELETE FROM scheduler_outbox o
		USING doomed d
		WHERE o.id = d.id
	`, watermark, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *schedulerOutboxRepository) TryAcquireCleanupLock(ctx context.Context) (service.SchedulerOutboxCleanupLease, bool, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, false, err
	}

	if IsMySQL() {
		// MySQL 版：GET_LOCK 是连接级命名锁，与 pg_try_advisory_lock 语义对等；
		// timeout 0 表示尝试一次立即返回（1=成功，0=被占用）。
		var acquired sql.NullInt64
		if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", schedulerOutboxCleanupLockNameMySQL).Scan(&acquired); err != nil {
			_ = conn.Close()
			return nil, false, err
		}
		if !acquired.Valid || acquired.Int64 != 1 {
			_ = conn.Close()
			return nil, false, nil
		}
		return &schedulerOutboxCleanupLease{conn: conn}, true, nil
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtext('scheduler_outbox_cleanup'))").Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	if !acquired {
		_ = conn.Close()
		return nil, false, nil
	}
	return &schedulerOutboxCleanupLease{conn: conn}, true, nil
}

func (l *schedulerOutboxCleanupLease) Release() {
	if l == nil || l.conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if IsMySQL() {
		_, _ = l.conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", schedulerOutboxCleanupLockNameMySQL)
		_ = l.conn.Close()
		l.conn = nil
		return
	}
	_, _ = l.conn.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext('scheduler_outbox_cleanup'))")
	_ = l.conn.Close()
	l.conn = nil
}

func enqueueSchedulerOutbox(ctx context.Context, exec sqlExecutor, eventType string, accountID *int64, groupID *int64, payload any) error {
	if exec == nil {
		return nil
	}
	if IsMySQL() {
		return enqueueSchedulerOutboxMySQL(ctx, exec, eventType, accountID, groupID, payload)
	}
	var payloadArg any
	var payloadJSON []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		payloadArg = encoded
		payloadJSON = encoded
	}
	query := `
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		VALUES ($1, $2, $3, $4)
	`
	args := []any{eventType, accountID, groupID, payloadArg}
	if schedulerOutboxEventSupportsDedup(eventType) {
		dedupKey := schedulerOutboxDedupKey(eventType, accountID, groupID, payloadJSON)
		query = `
			INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload, dedup_key)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (dedup_key) WHERE dedup_key IS NOT NULL DO NOTHING
		`
		args = append(args, dedupKey)
	}
	_, err := exec.ExecContext(ctx, query, args...)
	return err
}

// enqueueSchedulerOutboxMySQL 是 enqueueSchedulerOutbox 的 MySQL 实现：
// $n→?；PG 的部分唯一索引 `ON CONFLICT (dedup_key) WHERE dedup_key IS NOT NULL DO NOTHING`
// 对应 MySQL 唯一索引 idx_scheduler_outbox_pending_dedup_key（唯一索引天然允许多个 NULL，
// 无需 WHERE 谓词）+ ODKU no-op（冲突即静默丢弃，与 DO NOTHING 语义一致）。
func enqueueSchedulerOutboxMySQL(ctx context.Context, exec sqlExecutor, eventType string, accountID *int64, groupID *int64, payload any) error {
	if exec == nil {
		return nil
	}
	var payloadArg any
	var payloadJSON []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		payloadArg = encoded
		payloadJSON = encoded
	}
	query := `
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		VALUES (?, ?, ?, ?)
	`
	args := []any{eventType, accountID, groupID, payloadArg}
	if schedulerOutboxEventSupportsDedup(eventType) {
		dedupKey := schedulerOutboxDedupKey(eventType, accountID, groupID, payloadJSON)
		query = `
			INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload, dedup_key)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE id = id
		`
		args = append(args, dedupKey)
	}
	_, err := exec.ExecContext(ctx, query, args...)
	return err
}

func schedulerOutboxDedupKey(eventType string, accountID *int64, groupID *int64, payloadJSON []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(eventType))
	_, _ = h.Write([]byte{0})
	if accountID != nil {
		_, _ = h.Write([]byte(strconv.FormatInt(*accountID, 10)))
	}
	_, _ = h.Write([]byte{0})
	if groupID != nil {
		_, _ = h.Write([]byte(strconv.FormatInt(*groupID, 10)))
	}
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(payloadJSON)
	return fmt.Sprintf("scheduler_outbox:%s", hex.EncodeToString(h.Sum(nil)))
}

func schedulerOutboxEventSupportsDedup(eventType string) bool {
	switch eventType {
	case service.SchedulerOutboxEventAccountChanged,
		service.SchedulerOutboxEventGroupChanged,
		service.SchedulerOutboxEventFullRebuild:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。
// ---------------------------------------------------------------------------

// listAfterAndReleaseDedupMySQL 是 ListAfterAndReleaseDedup 的 MySQL 实现。
// PG 版是可写 CTE（selected FOR UPDATE + UPDATE..RETURNING + release_barrier），
// MySQL 无可写 CTE，拆为同事务两步：
//  1. SELECT ... FOR UPDATE 取行集合（MySQL 要求 LIMIT 在 FOR UPDATE 之前）；
//  2. UPDATE dedup_key = NULL WHERE id IN (...) AND dedup_key IS NOT NULL。
//
// 返回第 1 步读到的行：UPDATE 只清空 dedup_key、不改动任何返回列，与 PG
// UPDATE..RETURNING 语义一致；PG 的 release_barrier 仅保证 UPDATE 先于最终
// SELECT 完成，此处以同事务顺序执行对齐。
func (r *schedulerOutboxRepository) listAfterAndReleaseDedupMySQL(ctx context.Context, afterID int64, limit int) ([]service.SchedulerOutboxEvent, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	rollback := func(err error) ([]service.SchedulerOutboxEvent, error) {
		_ = tx.Rollback()
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, event_type, account_id, group_id, payload, created_at
		FROM scheduler_outbox
		WHERE id > ?
		ORDER BY id ASC
		LIMIT ?
		FOR UPDATE
	`, afterID, limit)
	if err != nil {
		return rollback(err)
	}

	events := make([]service.SchedulerOutboxEvent, 0, limit)
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var (
			payloadRaw []byte
			accountID  sql.NullInt64
			groupID    sql.NullInt64
			event      service.SchedulerOutboxEvent
		)
		if err := rows.Scan(&event.ID, &event.EventType, &accountID, &groupID, &payloadRaw, &event.CreatedAt); err != nil {
			_ = rows.Close()
			return rollback(err)
		}
		if accountID.Valid {
			v := accountID.Int64
			event.AccountID = &v
		}
		if groupID.Valid {
			v := groupID.Int64
			event.GroupID = &v
		}
		if len(payloadRaw) > 0 {
			var payload map[string]any
			if err := json.Unmarshal(payloadRaw, &payload); err != nil {
				_ = rows.Close()
				return rollback(err)
			}
			event.Payload = payload
		}
		events = append(events, event)
		ids = append(ids, event.ID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return rollback(err)
	}
	if err := rows.Close(); err != nil {
		return rollback(err)
	}

	if len(ids) > 0 {
		idArgs := make([]any, 0, len(ids))
		for _, id := range ids {
			idArgs = append(idArgs, id)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE scheduler_outbox
			SET dedup_key = NULL
			WHERE id IN (`+Placeholders(len(ids))+`)
				AND dedup_key IS NOT NULL
		`, idArgs...); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}

// firstCreatedAtAfterMySQL 是 FirstCreatedAtAfter 的 MySQL 实现（$1→?）。
func (r *schedulerOutboxRepository) firstCreatedAtAfterMySQL(ctx context.Context, afterID int64) (time.Time, bool, error) {
	var createdAt time.Time
	err := r.db.QueryRowContext(ctx, `
		SELECT created_at
		FROM scheduler_outbox
		WHERE id > ?
		ORDER BY id ASC
		LIMIT 1
	`, afterID).Scan(&createdAt)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return createdAt, true, nil
}

// deleteConsumedUpToMySQL 是 DeleteConsumedUpTo 的 MySQL 实现：
// PG 版为 doomed CTE + DELETE..USING，MySQL 单表 DELETE 原生支持 ORDER BY/LIMIT，
// 一条语句等价（id <= ?、宽限期条件、按 id 升序限量删除）；NOW()→NOW(3)。
func (r *schedulerOutboxRepository) deleteConsumedUpToMySQL(ctx context.Context, watermark int64, limit int) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM scheduler_outbox
		WHERE id <= ?
			AND created_at < NOW(3) - INTERVAL 10 SECOND
		ORDER BY id ASC
		LIMIT ?
	`, watermark, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
