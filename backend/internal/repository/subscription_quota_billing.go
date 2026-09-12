package repository

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func applySubscriptionQuotaCharge(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand) (*service.SubscriptionQuotaState, error) {
	if cmd.SubscriptionID == nil {
		return nil, service.ErrSubscriptionQuotaInvalid
	}
	id := *cmd.SubscriptionID
	var groupID, userID int64
	// Lock the pre-existing group row even before accounting is enabled. A
	// legacy settlement and initial bucket seeding must have a strict order.
	if err := quotaQueryOne(ctx, tx, `SELECT us.group_id,us.user_id FROM user_subscriptions us
 JOIN groups g ON g.id=us.group_id WHERE us.id=$1 FOR SHARE OF g`, []any{id}, &groupID, &userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrSubscriptionNotFound
		}
		return nil, err
	}
	group, err := readGroupQuotaState(ctx, tx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		if cmd.SubscriptionQuota != nil && !cmd.SubscriptionQuota.IsLegacy() {
			return nil, service.ErrSubscriptionQuotaInvalid
		}
		return nil, incrementUsageBillingSubscription(ctx, tx, id, cmd.SubscriptionCost)
	}
	quota := cmd.SubscriptionQuota
	if quota == nil {
		return nil, service.ErrSubscriptionQuotaRequired
	}
	if quota.SubscriptionID != id || quota.UserID != userID || quota.UserID != cmd.UserID || quota.GroupID != groupID || quota.AdmittedAt.IsZero() {
		return nil, service.ErrSubscriptionQuotaInvalid
	}
	if err := lockQuotaSubscription(ctx, tx, id, groupID); err != nil {
		return nil, err
	}
	if err := seedSubscriptionQuotaStates(ctx, tx, groupID, id); err != nil {
		return nil, err
	}
	var stateID int64
	if err := quotaQueryOne(ctx, tx, `SELECT subscription_id FROM subscription_quota_state WHERE subscription_id=$1 FOR UPDATE`, []any{id}, &stateID); err != nil {
		return nil, err
	}
	ids := [3]int64{quota.DailyBucketID, quota.WeeklyBucketID, quota.MonthlyBucketID}
	if quota.IsLegacy() {
		if !quota.AdmittedAt.Before(group.EnabledAt) || quota.TermStartsAt.IsZero() {
			return nil, service.ErrSubscriptionQuotaInvalid
		}
		windows := [3]*time.Time{quota.DailyWindowStart, quota.WeeklyWindowStart, quota.MonthlyWindowStart}
		for i, dimension := range []string{"daily", "weekly", "monthly"} {
			identity := legacyQuotaIdentity(quota.TermStartsAt, windows[i])
			if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_usage_buckets(subscription_id,dimension,term_epoch,group_revision,reason,window_start,term_starts_at,legacy_identity)
 VALUES($1,$2,1,1,'legacy_transition',$3,$4,$5) ON CONFLICT(subscription_id,dimension,legacy_identity) DO NOTHING`, id, dimension, windows[i], quota.TermStartsAt, identity); err != nil {
				return nil, err
			}
			if err := quotaQueryOne(ctx, tx, `SELECT id FROM subscription_usage_buckets WHERE subscription_id=$1 AND dimension=$2 AND legacy_identity=$3`, []any{id, dimension, identity}, &ids[i]); err != nil {
				return nil, err
			}
		}
	} else if ids[0] <= 0 || ids[1] <= 0 || ids[2] <= 0 || quota.TermEpoch <= 0 || quota.GroupRevision <= 0 {
		return nil, service.ErrSubscriptionQuotaInvalid
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,subscription_id,dimension,term_epoch,group_revision FROM subscription_usage_buckets
 WHERE id IN($1,$2,$3) ORDER BY id FOR UPDATE`, ids[0], ids[1], ids[2])
	if err != nil {
		return nil, err
	}
	valid := 0
	want := map[string]int64{"daily": ids[0], "weekly": ids[1], "monthly": ids[2]}
	for rows.Next() {
		var bucketID, subscriptionID, termEpoch, groupRevision int64
		var dimension string
		if err := rows.Scan(&bucketID, &subscriptionID, &dimension, &termEpoch, &groupRevision); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if subscriptionID != id || want[dimension] != bucketID || (!quota.IsLegacy() && (termEpoch != quota.TermEpoch || groupRevision > quota.GroupRevision)) {
			_ = rows.Close()
			return nil, service.ErrSubscriptionQuotaInvalid
		}
		valid++
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if valid != 3 {
		return nil, service.ErrSubscriptionQuotaInvalid
	}
	if _, err := tx.ExecContext(ctx, `UPDATE subscription_usage_buckets SET used_usd=used_usd+$1 WHERE id IN($2,$3,$4)`, cmd.SubscriptionCost, ids[0], ids[1], ids[2]); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_subscriptions us SET
 daily_usage_usd=us.daily_usage_usd+CASE WHEN s.daily_bucket_id=$3 THEN $2 ELSE 0 END,
 weekly_usage_usd=us.weekly_usage_usd+CASE WHEN s.weekly_bucket_id=$4 THEN $2 ELSE 0 END,
 monthly_usage_usd=us.monthly_usage_usd+CASE WHEN s.monthly_bucket_id=$5 THEN $2 ELSE 0 END,updated_at=NOW()
 FROM subscription_quota_state s WHERE us.id=$1 AND s.subscription_id=us.id
 AND (s.daily_bucket_id=$3 OR s.weekly_bucket_id=$4 OR s.monthly_bucket_id=$5)`, id, cmd.SubscriptionCost, ids[0], ids[1], ids[2]); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE subscription_quota_state SET state_version=nextval('subscription_quota_state_version')
 WHERE subscription_id=$1 AND (daily_bucket_id=$2 OR weekly_bucket_id=$3 OR monthly_bucket_id=$4)`, id, ids[0], ids[1], ids[2]); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_quota_charges(request_id,api_key_id,subscription_id,daily_bucket_id,weekly_bucket_id,monthly_bucket_id,admitted_at,cost_usd)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, cmd.RequestID, cmd.APIKeyID, id, ids[0], ids[1], ids[2], quota.AdmittedAt, cmd.SubscriptionCost); err != nil {
		return nil, err
	}
	state, err := readSubscriptionQuotaState(ctx, tx, id)
	if err == nil && state == nil {
		err = errors.New("subscription quota current state disappeared")
	}
	return state, err
}

func legacyQuotaIdentity(termStart time.Time, windowStart *time.Time) string {
	window := "null"
	if windowStart != nil {
		window = strconv.FormatInt(windowStart.UnixMicro(), 10)
	}
	return strconv.FormatInt(termStart.UnixMicro(), 10) + ":" + window
}
