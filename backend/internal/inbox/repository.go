package inbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// 约束名，与 184_add_general_inbox.sql 保持一致。用于区分"主键(seq)冲突"（需重试）
// 与"业务 dedup 冲突"（幂等命中，正常返回）。
const (
	constraintDirectPK    = "direct_messages_pkey"
	constraintBroadcastPK = "broadcasts_pkey"
)

// ErrSeqConflict 表示分配到的 seq 与已有记录主键冲突。这是一个内部信号，Publisher
// 捕获后应换下一个 seq 重试（详见 Publisher 的分配-插入循环）。它不是
// ApplicationError，不直接透传给客户端。
var ErrSeqConflict = errors.New("inbox: seq primary key conflict")

// Repository 是通用信箱的持久化访问接口。实现使用原生 SQL（复杂 upsert / GREATEST /
// 范围扫描用 ORM 表达不如原生清晰），底层复用 Ent 暴露的 *sql.DB。
type Repository interface {
	// InsertDirectMessage 落一条单播消息。
	//   - created=true：成功写入新行。
	//   - created=false, err=nil：(user_id, namespace, dedup_key) 幂等命中，未写入。
	//   - err=ErrSeqConflict：seq 主键冲突，调用方应换 seq 重试。
	InsertDirectMessage(ctx context.Context, seq int64, in PublishDirectInput) (created bool, err error)

	// InsertBroadcast 落一条广播消息。语义同上，dedup 维度为 (namespace, dedup_key)。
	InsertBroadcast(ctx context.Context, seq int64, in PublishBroadcastInput) (created bool, err error)

	// GetInboxState 读取用户累积 ack 水位。found=false 表示尚未初始化。
	GetInboxState(ctx context.Context, userID int64) (ackedSeq int64, found bool, err error)

	// InitInboxState 懒初始化用户信箱水位（首次 catchup）。已存在则不覆盖（DO NOTHING）。
	InitInboxState(ctx context.Context, userID, ackedSeq int64) error

	// Ack 累积抬升用户水位到 GREATEST(现值, seq)。行不存在时懒创建。
	Ack(ctx context.Context, userID, seq int64) error

	// ListDirectSince 拉取 seq > sinceSeq 的单播消息，按 seq 升序，最多 limit 条。
	ListDirectSince(ctx context.Context, userID, sinceSeq int64, limit int) ([]Message, error)

	// ListBroadcastsSince 拉取 seq > sinceSeq 且在保留期内的广播候选，按 seq 升序，
	// 最多 limit 条。targeting 匹配由调用方在应用层完成。
	ListBroadcastsSince(ctx context.Context, sinceSeq int64, cutoff time.Time, limit int) ([]Broadcast, error)

	// UnackedDirectSeqs 派生用户在 (ackedSeq, currentSeq] 区间的未 ack 单播 seq 列表。
	UnackedDirectSeqs(ctx context.Context, userID, ackedSeq, currentSeq int64, limit int) ([]int64, error)

	// UnackedBroadcasts 派生 (ackedSeq, currentSeq] 区间的未 ack 广播候选（含 targeting，
	// 由调用方过滤）。
	UnackedBroadcasts(ctx context.Context, ackedSeq, currentSeq int64, cutoff time.Time, limit int) ([]Broadcast, error)

	// DeleteExpiredDirect 删除 created_at < cutoff 的单播消息，单批最多 limit 行，返回删除行数。
	DeleteExpiredDirect(ctx context.Context, cutoff time.Time, limit int) (int64, error)

	// DeleteExpiredBroadcasts 删除 created_at < cutoff 的广播消息，单批最多 limit 行，返回删除行数。
	DeleteExpiredBroadcasts(ctx context.Context, cutoff time.Time, limit int) (int64, error)

	// ListBroadcastsPaged 管理端审计：按 namespace（空串=不过滤）分页倒序列出广播，
	// 返回当页行与匹配总数。
	ListBroadcastsPaged(ctx context.Context, namespace string, limit, offset int) (items []Broadcast, total int64, err error)

	// ListDirectMessagesPaged 管理端审计：按 namespace（空串=不过滤）与 userID（0=不过滤）
	// 分页倒序列出单播消息，返回当页行与匹配总数。
	ListDirectMessagesPaged(ctx context.Context, namespace string, userID int64, limit, offset int) (items []DirectMessage, total int64, err error)
}

type pgRepository struct {
	db *sql.DB
}

// NewRepository 构造基于原生 SQL 的信箱 Repository。
func NewRepository(db *sql.DB) Repository {
	return &pgRepository{db: db}
}

func (r *pgRepository) InsertDirectMessage(ctx context.Context, seq int64, in PublishDirectInput) (bool, error) {
	const q = `
INSERT INTO direct_messages (seq, user_id, namespace, dedup_key, payload)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, namespace, dedup_key) DO NOTHING`
	res, err := r.db.ExecContext(ctx, q, seq, in.RecipientID, in.Namespace, in.DedupKey, []byte(in.Payload))
	if err != nil {
		if isConstraintViolation(err, constraintDirectPK) {
			return false, ErrSeqConflict
		}
		return false, fmt.Errorf("inbox: insert direct message: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *pgRepository) InsertBroadcast(ctx context.Context, seq int64, in PublishBroadcastInput) (bool, error) {
	const q = `
INSERT INTO broadcasts (seq, namespace, dedup_key, targeting, payload)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (namespace, dedup_key) DO NOTHING`
	res, err := r.db.ExecContext(ctx, q, seq, in.Namespace, in.DedupKey, []byte(in.Targeting), []byte(in.Payload))
	if err != nil {
		if isConstraintViolation(err, constraintBroadcastPK) {
			return false, ErrSeqConflict
		}
		return false, fmt.Errorf("inbox: insert broadcast: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *pgRepository) GetInboxState(ctx context.Context, userID int64) (int64, bool, error) {
	const q = `SELECT acked_seq FROM user_inbox_state WHERE user_id = $1`
	var ackedSeq int64
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&ackedSeq)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("inbox: get inbox state for user %d: %w", userID, err)
	default:
		return ackedSeq, true, nil
	}
}

func (r *pgRepository) InitInboxState(ctx context.Context, userID, ackedSeq int64) error {
	const q = `
INSERT INTO user_inbox_state (user_id, acked_seq)
VALUES ($1, $2)
ON CONFLICT (user_id) DO NOTHING`
	if _, err := r.db.ExecContext(ctx, q, userID, ackedSeq); err != nil {
		return fmt.Errorf("inbox: init inbox state for user %d: %w", userID, err)
	}
	return nil
}

func (r *pgRepository) Ack(ctx context.Context, userID, seq int64) error {
	const q = `
INSERT INTO user_inbox_state (user_id, acked_seq, updated_at)
VALUES ($1, $2, NOW())
ON CONFLICT (user_id) DO UPDATE
   SET acked_seq  = GREATEST(user_inbox_state.acked_seq, EXCLUDED.acked_seq),
       updated_at = NOW()`
	if _, err := r.db.ExecContext(ctx, q, userID, seq); err != nil {
		return fmt.Errorf("inbox: ack user %d seq %d: %w", userID, seq, err)
	}
	return nil
}

func (r *pgRepository) ListDirectSince(ctx context.Context, userID, sinceSeq int64, limit int) ([]Message, error) {
	const q = `
SELECT seq, namespace, payload, created_at
  FROM direct_messages
 WHERE user_id = $1 AND seq > $2
 ORDER BY seq
 LIMIT $3`
	rows, err := r.db.QueryContext(ctx, q, userID, sinceSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("inbox: list direct since for user %d: %w", userID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Message
	for rows.Next() {
		var m Message
		m.Scope = ScopeDirect
		if err := rows.Scan(&m.Seq, &m.Namespace, &m.Payload, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("inbox: scan direct message: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inbox: iterate direct messages: %w", err)
	}
	return out, nil
}

func (r *pgRepository) ListBroadcastsSince(ctx context.Context, sinceSeq int64, cutoff time.Time, limit int) ([]Broadcast, error) {
	const q = `
SELECT seq, namespace, dedup_key, targeting, payload, created_at
  FROM broadcasts
 WHERE seq > $1 AND created_at > $2
 ORDER BY seq
 LIMIT $3`
	rows, err := r.db.QueryContext(ctx, q, sinceSeq, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("inbox: list broadcasts since: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanBroadcasts(rows)
}

func (r *pgRepository) UnackedDirectSeqs(ctx context.Context, userID, ackedSeq, currentSeq int64, limit int) ([]int64, error) {
	const q = `
SELECT seq
  FROM direct_messages
 WHERE user_id = $1 AND seq > $2 AND seq <= $3
 ORDER BY seq
 LIMIT $4`
	rows, err := r.db.QueryContext(ctx, q, userID, ackedSeq, currentSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("inbox: unacked direct seqs for user %d: %w", userID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []int64
	for rows.Next() {
		var seq int64
		if err := rows.Scan(&seq); err != nil {
			return nil, fmt.Errorf("inbox: scan unacked direct seq: %w", err)
		}
		out = append(out, seq)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inbox: iterate unacked direct seqs: %w", err)
	}
	return out, nil
}

func (r *pgRepository) UnackedBroadcasts(ctx context.Context, ackedSeq, currentSeq int64, cutoff time.Time, limit int) ([]Broadcast, error) {
	const q = `
SELECT seq, namespace, dedup_key, targeting, payload, created_at
  FROM broadcasts
 WHERE seq > $1 AND seq <= $2 AND created_at > $3
 ORDER BY seq
 LIMIT $4`
	rows, err := r.db.QueryContext(ctx, q, ackedSeq, currentSeq, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("inbox: unacked broadcasts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanBroadcasts(rows)
}

func (r *pgRepository) DeleteExpiredDirect(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	const q = `
DELETE FROM direct_messages
 WHERE seq IN (
   SELECT seq FROM direct_messages WHERE created_at < $1 ORDER BY seq LIMIT $2
 )`
	res, err := r.db.ExecContext(ctx, q, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("inbox: delete expired direct: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *pgRepository) DeleteExpiredBroadcasts(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	const q = `
DELETE FROM broadcasts
 WHERE seq IN (
   SELECT seq FROM broadcasts WHERE created_at < $1 ORDER BY seq LIMIT $2
 )`
	res, err := r.db.ExecContext(ctx, q, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("inbox: delete expired broadcasts: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *pgRepository) ListBroadcastsPaged(ctx context.Context, namespace string, limit, offset int) ([]Broadcast, int64, error) {
	const countQ = `SELECT count(*) FROM broadcasts WHERE ($1 = '' OR namespace = $1)`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQ, namespace).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("inbox: count broadcasts: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	const q = `
SELECT seq, namespace, dedup_key, targeting, payload, created_at
  FROM broadcasts
 WHERE ($1 = '' OR namespace = $1)
 ORDER BY seq DESC
 LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, q, namespace, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("inbox: list broadcasts paged: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items, err := scanBroadcasts(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *pgRepository) ListDirectMessagesPaged(ctx context.Context, namespace string, userID int64, limit, offset int) ([]DirectMessage, int64, error) {
	const countQ = `
SELECT count(*) FROM direct_messages
 WHERE ($1 = '' OR namespace = $1) AND ($2 = 0 OR user_id = $2)`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQ, namespace, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("inbox: count direct messages: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	const q = `
SELECT seq, user_id, namespace, dedup_key, payload, created_at
  FROM direct_messages
 WHERE ($1 = '' OR namespace = $1) AND ($2 = 0 OR user_id = $2)
 ORDER BY seq DESC
 LIMIT $3 OFFSET $4`
	rows, err := r.db.QueryContext(ctx, q, namespace, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("inbox: list direct messages paged: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []DirectMessage
	for rows.Next() {
		var m DirectMessage
		if err := rows.Scan(&m.Seq, &m.UserID, &m.Namespace, &m.DedupKey, &m.Payload, &m.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("inbox: scan direct message row: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("inbox: iterate direct message rows: %w", err)
	}
	return out, total, nil
}

// scanBroadcasts 把结果集扫描为 []Broadcast。
func scanBroadcasts(rows *sql.Rows) ([]Broadcast, error) {
	var out []Broadcast
	for rows.Next() {
		var b Broadcast
		if err := rows.Scan(&b.Seq, &b.Namespace, &b.DedupKey, &b.Targeting, &b.Payload, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("inbox: scan broadcast: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inbox: iterate broadcasts: %w", err)
	}
	return out, nil
}

// isConstraintViolation 判断错误是否为指定约束名的唯一/主键冲突（PostgreSQL 23505）。
func isConstraintViolation(err error, constraint string) bool {
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.Constraint == constraint
	}
	return false
}
