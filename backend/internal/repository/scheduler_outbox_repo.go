package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

func NewSchedulerOutboxRepository(db *sql.DB) service.SchedulerOutboxRepository {
	return &schedulerOutboxRepository{db: db}
}

func (r *schedulerOutboxRepository) ListAfterAndReleaseDedup(ctx context.Context, afterID int64, limit int) ([]service.SchedulerOutboxEvent, error) {
	if limit <= 0 {
		limit = 100
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
	_, _ = l.conn.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext('scheduler_outbox_cleanup'))")
	_ = l.conn.Close()
	l.conn = nil
}

func enqueueSchedulerOutbox(ctx context.Context, exec sqlExecutor, eventType string, accountID *int64, groupID *int64, payload any) error {
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

// decodeSchedulerOutboxRows turns a result set of the queue projection into events.
func decodeSchedulerOutboxRows(rows *sql.Rows) ([]service.SchedulerOutboxEvent, error) {
	events := make([]service.SchedulerOutboxEvent, 0, 64)
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
	return events, rows.Err()
}

// Claim takes the head of the queue and records ownership on the rows.
//
// Two properties matter and neither existed before:
//   - the read has no `id > watermark` predicate, so a row that becomes visible
//     after the watermark already moved past it is still consumed;
//   - ownership lives in claimed_at/claimed_by, so the claim stays effective for
//     the whole handling phase (a statement-level FOR UPDATE would release its
//     locks as soon as this query returned), and an abandoned claim is
//     reclaimed by another consumer once the lease expires.
//
// FOR UPDATE SKIP LOCKED still guards the instant two consumers run this
// statement at the same time.
func (r *schedulerOutboxRepository) Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]service.SchedulerOutboxEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil scheduler outbox database")
	}
	if workerID == "" {
		return nil, errors.New("empty scheduler outbox worker id")
	}
	if limit <= 0 {
		limit = 100
	}
	leaseSeconds := int64(lease / time.Second)
	if leaseSeconds < 1 {
		leaseSeconds = 300
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH busy AS (
			-- Accounts whose events are currently owned by another consumer. Their
			-- later events must wait: claiming them in parallel lets a second consumer
			-- finish event N+1 before event N and reorders one account's changes.
			-- Rows claimed by this worker are excluded, because this worker handles
			-- its own claims strictly in id order.
			SELECT DISTINCT account_id
			FROM scheduler_outbox
			WHERE account_id IS NOT NULL
			  AND claimed_by IS NOT NULL
			  AND claimed_by <> $1
			  AND claimed_at IS NOT NULL
			  AND claimed_at >= NOW() - ($3 * INTERVAL '1 second')
		), candidates AS (
			SELECT id
			FROM scheduler_outbox
			WHERE (available_at IS NULL OR available_at <= NOW())
			  AND (claimed_at IS NULL OR claimed_at < NOW() - ($3 * INTERVAL '1 second'))
			  AND (account_id IS NULL OR account_id NOT IN (SELECT account_id FROM busy))
			ORDER BY id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		), taken AS (
			UPDATE scheduler_outbox AS o
			SET claimed_at = NOW(), claimed_by = $1
			FROM candidates AS c
			WHERE o.id = c.id
			RETURNING o.id, o.event_type, o.account_id, o.group_id, o.payload, o.created_at
		), released AS (
			UPDATE scheduler_outbox AS o
			SET dedup_key = NULL
			FROM taken AS t
			WHERE o.id = t.id
			  AND o.dedup_key IS NOT NULL
			RETURNING o.id
		)
		SELECT t.id, t.event_type, t.account_id, t.group_id, t.payload, t.created_at
		FROM taken AS t
		CROSS JOIN (SELECT COUNT(*) FROM released) AS release_barrier
		ORDER BY t.id ASC
	`, workerID, limit, leaseSeconds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return decodeSchedulerOutboxRows(rows)
}

// DeleteClaimed acknowledges a single event, and only for the consumer that
// still owns it. Deleting by `id <= watermark` (the previous behaviour) could
// destroy rows that were never claimed.
func (r *schedulerOutboxRepository) DeleteClaimed(ctx context.Context, id int64, workerID string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM scheduler_outbox
		WHERE id = $1 AND claimed_by = $2
	`, id, workerID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("scheduler outbox claim %d is no longer owned by %s", id, workerID)
	}
	return nil
}

// RetryClaimed releases one event back to the queue with a not-before time.
func (r *schedulerOutboxRepository) RetryClaimed(ctx context.Context, id int64, workerID string, availableAt time.Time, lastError string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE scheduler_outbox
		SET claimed_at = NULL, claimed_by = NULL, available_at = $3
		WHERE id = $1 AND claimed_by = $2
	`, id, workerID, availableAt)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("scheduler outbox claim %d is no longer owned by %s", id, workerID)
	}
	return nil
}

// ReleaseClaimed hands a whole tail of an in-flight batch back to the queue at
// once, so ordering is preserved without waiting for the lease to expire.
func (r *schedulerOutboxRepository) ReleaseClaimed(ctx context.Context, ids []int64, availableAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)+1))
		args = append(args, id)
	}
	args = append(args, availableAt)
	query := fmt.Sprintf(
		"UPDATE scheduler_outbox SET claimed_at = NULL, claimed_by = NULL, available_at = $%d WHERE id IN (%s)",
		len(args), strings.Join(placeholders, ","),
	)
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	return nil
}

// PendingStats reports the real backlog. Before this, the backlog was derived
// as `max(id) - watermark`, which reported "no backlog" for exactly the rows
// the watermark had jumped over.
func (r *schedulerOutboxRepository) PendingStats(ctx context.Context) (service.SchedulerOutboxPendingStats, error) {
	var (
		stats  service.SchedulerOutboxPendingStats
		oldest sql.NullTime
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*), MIN(created_at) FROM scheduler_outbox
	`).Scan(&stats.Pending, &oldest)
	if err != nil {
		return stats, err
	}
	if oldest.Valid {
		value := oldest.Time
		stats.OldestCreatedAt = &value
	}
	return stats, nil
}
