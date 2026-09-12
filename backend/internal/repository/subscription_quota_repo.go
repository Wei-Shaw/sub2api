package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type subscriptionQuotaRepository struct{ client *dbent.Client }

func NewSubscriptionQuotaRepository(client *dbent.Client) service.SubscriptionQuotaRepository {
	return &subscriptionQuotaRepository{client: client}
}

type quotaSQL interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// WithSubscriptionQuotaSQLTx lets an observer repository publish quota changes
// and its own audit/outbox using the same native SQL transaction. Subscription
// lifecycle callers normally use the existing Ent transaction context instead.
func WithSubscriptionQuotaSQLTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, quotaSQLTxKey{}, tx)
}

type quotaSQLTxKey struct{}

func (r *subscriptionQuotaRepository) executor(ctx context.Context) quotaSQL {
	if tx, ok := ctx.Value(quotaSQLTxKey{}).(*sql.Tx); ok && tx != nil {
		return tx
	}
	return clientFromContext(ctx, r.client)
}

type quotaGroupLockKey struct{}
type quotaGroupLock struct {
	groupID   int64
	exclusive bool
}

func quotaQueryOne(ctx context.Context, exec quotaSQL, query string, args []any, dest ...any) error {
	rows, err := exec.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	return rows.Scan(dest...)
}

func (r *subscriptionQuotaRepository) WithGroupQuotaTx(ctx context.Context, groupID int64, exclusive bool, fn func(context.Context) error) error {
	if groupID <= 0 || fn == nil {
		return service.ErrInvalidInput
	}
	if held, ok := ctx.Value(quotaGroupLockKey{}).(quotaGroupLock); ok {
		if held.groupID != groupID || (exclusive && !held.exclusive) {
			return errors.New("subscription quota lock order violation")
		}
		return fn(ctx)
	}
	lockAndApply := func(txCtx context.Context) error {
		mode := "FOR SHARE"
		if exclusive {
			mode = "FOR UPDATE"
		}
		var id int64
		if err := quotaQueryOne(txCtx, r.executor(txCtx), `SELECT id FROM groups WHERE id=$1 `+mode, []any{groupID}, &id); err != nil {
			return err
		}
		return fn(context.WithValue(txCtx, quotaGroupLockKey{}, quotaGroupLock{groupID, exclusive}))
	}
	if tx, ok := ctx.Value(quotaSQLTxKey{}).(*sql.Tx); (ok && tx != nil) || dbent.TxFromContext(ctx) != nil {
		return lockAndApply(ctx)
	}
	tx, err := r.client.Tx(ctx)
	if errors.Is(err, dbent.ErrTxStarted) {
		// Repositories may be constructed with tx.Client() directly. In that
		// case the caller owns commit/rollback even without a context marker.
		return lockAndApply(ctx)
	}
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockAndApply(dbent.NewTxContext(ctx, tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *subscriptionQuotaRepository) GetGroupQuotaState(ctx context.Context, groupID int64) (*service.SubscriptionGroupQuotaState, error) {
	return readGroupQuotaState(ctx, r.executor(ctx), groupID)
}

func readGroupQuotaState(ctx context.Context, exec quotaSQL, groupID int64) (*service.SubscriptionGroupQuotaState, error) {
	state := &service.SubscriptionGroupQuotaState{GroupID: groupID}
	err := quotaQueryOne(ctx, exec, `SELECT enabled_at,revision FROM subscription_group_quota_state WHERE group_id=$1`, []any{groupID}, &state.EnabledAt, &state.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return state, err
}

func quotaDatabaseTime(ctx context.Context, exec quotaSQL, requested time.Time) (time.Time, error) {
	if !requested.IsZero() {
		return requested.UTC().Truncate(time.Microsecond), nil
	}
	var now time.Time
	err := quotaQueryOne(ctx, exec, `SELECT clock_timestamp()`, nil, &now)
	return now, err
}

func (r *subscriptionQuotaRepository) EnableGroupQuota(ctx context.Context, groupID int64, at time.Time) (*service.SubscriptionGroupQuotaState, error) {
	var state *service.SubscriptionGroupQuotaState
	err := r.WithGroupQuotaTx(ctx, groupID, true, func(txCtx context.Context) error {
		exec := r.executor(txCtx)
		var err error
		at, err = quotaDatabaseTime(txCtx, exec, at)
		if err != nil {
			return err
		}
		if _, err := exec.ExecContext(txCtx, `INSERT INTO subscription_group_quota_state(group_id,enabled_at)
 SELECT id,$2 FROM groups WHERE id=$1 AND deleted_at IS NULL
 ON CONFLICT(group_id) DO NOTHING`, groupID, at); err != nil {
			return err
		}
		state, err = readGroupQuotaState(txCtx, exec, groupID)
		if err != nil {
			return err
		}
		if state == nil {
			return service.ErrInvalidInput
		}
		return seedSubscriptionQuotaStates(txCtx, exec, groupID, 0)
	})
	return state, err
}

// Lock subscriptions before inserting their initial buckets. The migration is
// lazy and preserves every counter/window, including inactive subscriptions.
func seedSubscriptionQuotaStates(ctx context.Context, exec quotaSQL, groupID, subscriptionID int64) error {
	_, err := exec.ExecContext(ctx, `WITH eligible AS MATERIALIZED (
 SELECT us.* FROM user_subscriptions us
 WHERE us.group_id=$1 AND ($2::bigint=0 OR us.id=$2) AND (us.deleted_at IS NULL OR $2<>0)
 AND NOT EXISTS(SELECT 1 FROM subscription_quota_state s WHERE s.subscription_id=us.id)
 ORDER BY us.id FOR UPDATE OF us
 ), inserted AS (
 INSERT INTO subscription_usage_buckets(subscription_id,dimension,term_epoch,group_revision,reason,window_start,term_starts_at,legacy_identity,used_usd)
 SELECT us.id,d.dimension,1,g.revision,'initial',d.window_start,us.starts_at,
 (FLOOR(EXTRACT(EPOCH FROM us.starts_at)*1000000)::bigint)::text || ':' ||
 COALESCE((FLOOR(EXTRACT(EPOCH FROM d.window_start)*1000000)::bigint)::text,'null'),d.used_usd
 FROM eligible us JOIN subscription_group_quota_state g ON g.group_id=us.group_id
 CROSS JOIN LATERAL (VALUES ('daily',us.daily_window_start,us.daily_usage_usd),
 ('weekly',us.weekly_window_start,us.weekly_usage_usd),('monthly',us.monthly_window_start,us.monthly_usage_usd)) d(dimension,window_start,used_usd)
 RETURNING id,subscription_id,dimension
 ) INSERT INTO subscription_quota_state(subscription_id,daily_bucket_id,weekly_bucket_id,monthly_bucket_id)
 SELECT subscription_id,MAX(id) FILTER(WHERE dimension='daily'),MAX(id) FILTER(WHERE dimension='weekly'),MAX(id) FILTER(WHERE dimension='monthly')
 FROM inserted GROUP BY subscription_id`, groupID, subscriptionID)
	return err
}

const subscriptionQuotaStateSQL = `SELECT json_build_object(
 'subscription_id',us.id,'user_id',us.user_id,'group_id',us.group_id,
 'status',us.status,'starts_at',us.starts_at,'expires_at',us.expires_at,'deleted_at',us.deleted_at,
 'term_epoch',s.term_epoch,'group_revision',g.revision,'state_version',s.state_version,
 'daily_bucket_id',s.daily_bucket_id,'weekly_bucket_id',s.weekly_bucket_id,'monthly_bucket_id',s.monthly_bucket_id,
 'term_starts_at',d.term_starts_at,
 'daily_window_start',d.window_start,'weekly_window_start',w.window_start,'monthly_window_start',m.window_start,
 'daily_usage_usd',d.used_usd,'weekly_usage_usd',w.used_usd,'monthly_usage_usd',m.used_usd)
 FROM subscription_quota_state s JOIN user_subscriptions us ON us.id=s.subscription_id
 JOIN subscription_group_quota_state g ON g.group_id=us.group_id
 JOIN subscription_usage_buckets d ON d.id=s.daily_bucket_id
 JOIN subscription_usage_buckets w ON w.id=s.weekly_bucket_id
 JOIN subscription_usage_buckets m ON m.id=s.monthly_bucket_id
 WHERE s.subscription_id=$1`

func readSubscriptionQuotaState(ctx context.Context, exec quotaSQL, subscriptionID int64) (*service.SubscriptionQuotaState, error) {
	var raw []byte
	err := quotaQueryOne(ctx, exec, subscriptionQuotaStateSQL, []any{subscriptionID}, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state service.SubscriptionQuotaState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *subscriptionQuotaRepository) GetSubscriptionQuotaState(ctx context.Context, id int64) (*service.SubscriptionQuotaState, error) {
	return readSubscriptionQuotaState(ctx, r.executor(ctx), id)
}

func (r *subscriptionQuotaRepository) subscriptionGroupID(ctx context.Context, id int64) (int64, error) {
	var groupID int64
	err := quotaQueryOne(ctx, r.executor(ctx), `SELECT group_id FROM user_subscriptions WHERE id=$1`, []any{id}, &groupID)
	return groupID, err
}

func lockQuotaSubscription(ctx context.Context, exec quotaSQL, id, groupID int64) error {
	var found int64
	return quotaQueryOne(ctx, exec, `SELECT id FROM user_subscriptions WHERE id=$1 AND group_id=$2 FOR UPDATE`, []any{id, groupID}, &found)
}

func (r *subscriptionQuotaRepository) EnsureSubscriptionQuotaState(ctx context.Context, id int64) (*service.SubscriptionQuotaState, error) {
	groupID, err := r.subscriptionGroupID(ctx, id)
	if err != nil {
		return nil, err
	}
	var state *service.SubscriptionQuotaState
	err = r.WithGroupQuotaTx(ctx, groupID, false, func(txCtx context.Context) error {
		exec := r.executor(txCtx)
		if err := lockQuotaSubscription(txCtx, exec, id, groupID); err != nil {
			return err
		}
		if err := seedSubscriptionQuotaStates(txCtx, exec, groupID, id); err != nil {
			return err
		}
		var err error
		state, err = readSubscriptionQuotaState(txCtx, exec, id)
		return err
	})
	return state, err
}

func (r *subscriptionQuotaRepository) RotateSubscriptionQuota(ctx context.Context, input service.RotateSubscriptionQuotaInput) (*service.SubscriptionQuotaState, error) {
	if !input.Dimensions.Any() || strings.TrimSpace(input.Reason) == "" {
		return nil, service.ErrInvalidInput
	}
	if input.NewTerm && (!input.Dimensions.Daily || !input.Dimensions.Weekly || !input.Dimensions.Monthly || input.TermStartsAt.IsZero()) {
		return nil, service.ErrInvalidInput
	}
	groupID, err := r.subscriptionGroupID(ctx, input.SubscriptionID)
	if err != nil {
		return nil, err
	}
	var result *service.SubscriptionQuotaState
	err = r.WithGroupQuotaTx(ctx, groupID, false, func(txCtx context.Context) error {
		exec := r.executor(txCtx)
		if err := lockQuotaSubscription(txCtx, exec, input.SubscriptionID, groupID); err != nil {
			return err
		}
		if err := seedSubscriptionQuotaStates(txCtx, exec, groupID, input.SubscriptionID); err != nil {
			return err
		}
		state, err := readSubscriptionQuotaState(txCtx, exec, input.SubscriptionID)
		if err != nil || state == nil {
			return err
		}
		if expected := input.Expected; expected != nil &&
			(expected.SubscriptionID != input.SubscriptionID || expected.TermEpoch != state.TermEpoch ||
				(input.Dimensions.Daily && expected.DailyBucketID != state.DailyBucketID) ||
				(input.Dimensions.Weekly && expected.WeeklyBucketID != state.WeeklyBucketID) ||
				(input.Dimensions.Monthly && expected.MonthlyBucketID != state.MonthlyBucketID)) {
			result = state
			return nil
		}
		at, err := quotaDatabaseTime(txCtx, exec, input.At)
		if err != nil {
			return err
		}
		termEpoch, termStart := state.TermEpoch, state.TermStartsAt
		if input.NewTerm {
			termEpoch++
			termStart = input.TermStartsAt
		}
		_, err = exec.ExecContext(txCtx, `WITH inserted AS (
 INSERT INTO subscription_usage_buckets(subscription_id,dimension,term_epoch,group_revision,reason,window_start,term_starts_at,created_at)
 SELECT $1,d.dimension,$2,$3,$4,d.window_start,$5,$6
 FROM (VALUES ('daily',$7::boolean,$10::timestamptz),('weekly',$8::boolean,$11::timestamptz),('monthly',$9::boolean,$12::timestamptz)) d(dimension,selected,window_start)
 WHERE d.selected RETURNING id,dimension
 ), changed AS (
 UPDATE subscription_quota_state SET term_epoch=$2,state_version=nextval('subscription_quota_state_version'),
 daily_bucket_id=COALESCE((SELECT id FROM inserted WHERE dimension='daily'),daily_bucket_id),
 weekly_bucket_id=COALESCE((SELECT id FROM inserted WHERE dimension='weekly'),weekly_bucket_id),
 monthly_bucket_id=COALESCE((SELECT id FROM inserted WHERE dimension='monthly'),monthly_bucket_id)
 WHERE subscription_id=$1 RETURNING subscription_id
 ) UPDATE user_subscriptions SET
 daily_usage_usd=CASE WHEN $7 THEN 0 ELSE daily_usage_usd END,
 weekly_usage_usd=CASE WHEN $8 THEN 0 ELSE weekly_usage_usd END,
 monthly_usage_usd=CASE WHEN $9 THEN 0 ELSE monthly_usage_usd END,
 daily_window_start=CASE WHEN $7 THEN $10 ELSE daily_window_start END,
 weekly_window_start=CASE WHEN $8 THEN $11 ELSE weekly_window_start END,
 monthly_window_start=CASE WHEN $9 THEN $12 ELSE monthly_window_start END,updated_at=$6
 WHERE id IN (SELECT subscription_id FROM changed)`, input.SubscriptionID, termEpoch, state.GroupRevision, input.Reason, termStart, at,
			input.Dimensions.Daily, input.Dimensions.Weekly, input.Dimensions.Monthly, input.DailyWindowStart, input.WeeklyWindowStart, input.MonthlyWindowStart)
		if err != nil {
			return err
		}
		result, err = readSubscriptionQuotaState(txCtx, exec, input.SubscriptionID)
		return err
	})
	return result, err
}

func (r *subscriptionQuotaRepository) RotateGroupQuota(ctx context.Context, input service.RotateGroupQuotaInput) (*service.RotateGroupQuotaResult, error) {
	if !input.Dimensions.Any() || strings.TrimSpace(input.Reason) == "" {
		return nil, service.ErrInvalidInput
	}
	var result *service.RotateGroupQuotaResult
	err := r.WithGroupQuotaTx(ctx, input.GroupID, true, func(txCtx context.Context) error {
		exec := r.executor(txCtx)
		at, err := quotaDatabaseTime(txCtx, exec, input.At)
		if err != nil {
			return err
		}
		if err := seedSubscriptionQuotaStates(txCtx, exec, input.GroupID, 0); err != nil {
			return err
		}
		var revision int64
		if err := quotaQueryOne(txCtx, exec, `UPDATE subscription_group_quota_state SET revision=revision+1 WHERE group_id=$1 RETURNING revision`, []any{input.GroupID}, &revision); err != nil {
			return err
		}
		updated, err := exec.ExecContext(txCtx, `WITH eligible AS MATERIALIZED (
 SELECT us.id,us.starts_at,s.term_epoch FROM user_subscriptions us
 JOIN subscription_quota_state s ON s.subscription_id=us.id
 WHERE us.group_id=$1 AND us.deleted_at IS NULL AND us.status='active' AND us.starts_at<=$2 AND us.expires_at>$2
 AND (us.daily_window_start IS NOT NULL OR us.weekly_window_start IS NOT NULL OR us.monthly_window_start IS NOT NULL)
 ORDER BY us.id FOR UPDATE OF us
 ), inserted AS (
 INSERT INTO subscription_usage_buckets(subscription_id,dimension,term_epoch,group_revision,reason,window_start,term_starts_at,created_at)
 SELECT e.id,d.dimension,e.term_epoch,$3,$4,d.window_start,e.starts_at,$2 FROM eligible e
 CROSS JOIN (VALUES ('daily',$5::boolean,$8::timestamptz),('weekly',$6::boolean,$2::timestamptz),('monthly',$7::boolean,$2::timestamptz)) d(dimension,selected,window_start)
 WHERE d.selected RETURNING id,subscription_id,dimension
 ), ids AS (
 SELECT subscription_id,MAX(id) FILTER(WHERE dimension='daily') AS daily,
 MAX(id) FILTER(WHERE dimension='weekly') AS weekly,MAX(id) FILTER(WHERE dimension='monthly') AS monthly FROM inserted GROUP BY subscription_id
 ), changed AS (
 UPDATE subscription_quota_state s SET state_version=nextval('subscription_quota_state_version'),
 daily_bucket_id=COALESCE(ids.daily,s.daily_bucket_id),weekly_bucket_id=COALESCE(ids.weekly,s.weekly_bucket_id),monthly_bucket_id=COALESCE(ids.monthly,s.monthly_bucket_id)
 FROM ids WHERE s.subscription_id=ids.subscription_id RETURNING s.subscription_id
 ) UPDATE user_subscriptions SET daily_usage_usd=CASE WHEN $5 THEN 0 ELSE daily_usage_usd END,
 weekly_usage_usd=CASE WHEN $6 THEN 0 ELSE weekly_usage_usd END,monthly_usage_usd=CASE WHEN $7 THEN 0 ELSE monthly_usage_usd END,
 daily_window_start=CASE WHEN $5 THEN $8 ELSE daily_window_start END,
 weekly_window_start=CASE WHEN $6 THEN $2 ELSE weekly_window_start END,monthly_window_start=CASE WHEN $7 THEN $2 ELSE monthly_window_start END,
 updated_at=$2 WHERE id IN(SELECT subscription_id FROM changed)`, input.GroupID, at, revision, input.Reason, input.Dimensions.Daily, input.Dimensions.Weekly, input.Dimensions.Monthly, input.DailyWindowStart)
		if err != nil {
			return err
		}
		count, err := updated.RowsAffected()
		if err != nil {
			return err
		}
		result = &service.RotateGroupQuotaResult{GroupID: input.GroupID, Revision: revision, EffectiveAt: at, AffectedSubscriptions: count}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("rotate group subscription quota: %w", err)
	}
	return result, nil
}
