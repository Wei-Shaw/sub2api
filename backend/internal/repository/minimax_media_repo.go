package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type miniMaxMediaRepository struct{ db *sql.DB }

func NewMiniMaxMediaRepository(db *sql.DB) service.MiniMaxMediaRepository {
	return &miniMaxMediaRepository{db: db}
}

const mediaColumns = `id,user_id,api_key_id,group_id,account_id,subscription_id,account_type,upstream_id,origin,kind,idempotency_key,request_hash,snapshot,reserved_usd,reservation_windows,status,billing_state,bill,result,created_at`

type mediaWindows struct {
	Subscription [3]*time.Time `json:"subscription"`
	Key          [3]*time.Time `json:"key"`
}
type mediaScanner interface{ Scan(...any) error }

func scanMedia(row mediaScanner) (*service.MiniMaxMediaTask, error) {
	t := &service.MiniMaxMediaTask{}
	var snapshot, windows, bill, result []byte
	var upstream sql.NullString
	err := row.Scan(&t.ID, &t.UserID, &t.APIKeyID, &t.GroupID, &t.AccountID, &t.SubscriptionID, &t.AccountType, &upstream, &t.Origin, &t.Kind, &t.IdempotencyKey, &t.RequestHash, &snapshot, &t.ReservedUSD, &windows, &t.Status, &t.BillingState, &bill, &result, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrMiniMaxMediaNotFound
	}
	if err != nil {
		return nil, err
	}
	var saved service.MiniMaxMediaTask
	if err = json.Unmarshal(snapshot, &saved); err != nil {
		return nil, err
	}
	t.Model, t.Resolution, t.Duration, t.Stream, t.Pricing = saved.Model, saved.Resolution, saved.Duration, saved.Stream, saved.Pricing
	t.RequestInfo = saved.RequestInfo
	t.BillingPolicy, t.RequestedModel = saved.BillingPolicy, saved.RequestedModel
	t.UpstreamID = upstream.String
	var w mediaWindows
	if err = json.Unmarshal(windows, &w); err != nil {
		return nil, err
	}
	t.SubscriptionWindows, t.KeyWindows = w.Subscription, w.Key
	if len(bill) > 0 {
		if err = json.Unmarshal(bill, &t.Bill); err != nil {
			return nil, err
		}
	}
	t.Result = result
	return t, nil
}
func (r *miniMaxMediaRepository) Get(ctx context.Context, id string, user, key int64) (*service.MiniMaxMediaTask, error) {
	return scanMedia(r.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM minimax_media_tasks WHERE id=$1 AND ($2::bigint=0 OR user_id=$2) AND ($3::bigint=0 OR api_key_id=$3)`, id, user, key))
}
func (r *miniMaxMediaRepository) FindByIdempotencyKey(ctx context.Context, key int64, idem string) (*service.MiniMaxMediaTask, error) {
	return scanMedia(r.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM minimax_media_tasks WHERE api_key_id=$1 AND idempotency_key=$2`, key, idem))
}
func (r *miniMaxMediaRepository) List(ctx context.Context, user, key int64, limit int) ([]*service.MiniMaxMediaTask, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+mediaColumns+` FROM minimax_media_tasks WHERE ($1::bigint=0 OR user_id=$1) AND ($2::bigint=0 OR api_key_id=$2) ORDER BY created_at DESC LIMIT $3`, user, key, min(max(limit, 1), 100))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tasks := make([]*service.MiniMaxMediaTask, 0)
	for rows.Next() {
		t, e := scanMedia(rows)
		if e != nil {
			return nil, e
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
func (r *miniMaxMediaRepository) Reserve(ctx context.Context, t *service.MiniMaxMediaTask) (*service.MiniMaxMediaTask, bool, error) {
	t.ReservedUSD = service.QuantizeUsageBillingAmount(t.ReservedUSD)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	snapshot, err := json.Marshal(t)
	if err != nil {
		return nil, false, err
	}
	var id string
	err = tx.QueryRowContext(ctx, `INSERT INTO minimax_media_tasks
 (id,user_id,api_key_id,group_id,account_id,subscription_id,account_type,origin,kind,idempotency_key,request_hash,snapshot,reserved_usd)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(api_key_id,idempotency_key) DO NOTHING RETURNING id`, t.ID, t.UserID, t.APIKeyID, t.GroupID, t.AccountID, t.SubscriptionID, t.AccountType, t.Origin, t.Kind, t.IdempotencyKey, t.RequestHash, snapshot, t.ReservedUSD).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		old, e := scanMedia(tx.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM minimax_media_tasks WHERE api_key_id=$1 AND idempotency_key=$2`, t.APIKeyID, t.IdempotencyKey))
		if e != nil {
			return nil, false, e
		}
		if old.UserID != t.UserID || old.RequestHash != t.RequestHash || old.Kind != t.Kind {
			return nil, false, service.ErrMiniMaxMediaConflict
		}
		return old, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !t.UsesStandardBilling() {
		if err = reserveMiniMaxSimple(ctx, tx, t); err != nil {
			return nil, false, err
		}
	} else {
		// Common lock order is user, subscription, key, consistent with the ledger.
		// Moving funds out of balance
		// makes the reservation visible to concurrent text and media consumption.
		var lockID int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL AND status='active' FOR UPDATE`, t.UserID).Scan(&lockID)
		if err != nil {
			return nil, false, err
		}
		if t.SubscriptionID == nil {
			res, e := tx.ExecContext(ctx, `UPDATE users SET balance=balance-$1,frozen_balance=frozen_balance+$1,updated_at=NOW() WHERE id=$2 AND balance >= $1`, t.ReservedUSD, t.UserID)
			if e != nil {
				return nil, false, e
			}
			n, e := res.RowsAffected()
			if e != nil {
				return nil, false, e
			}
			if n != 1 {
				return nil, false, service.ErrMiniMaxMediaFunds
			}
		}
		if t.SubscriptionID != nil {
			if err = tx.QueryRowContext(ctx, `SELECT id FROM user_subscriptions WHERE id=$1 FOR UPDATE`, *t.SubscriptionID).Scan(&lockID); err != nil {
				return nil, false, err
			}
		}
		err = tx.QueryRowContext(ctx, `SELECT id FROM api_keys WHERE id=$1 AND user_id=$2 AND group_id=$3 AND deleted_at IS NULL AND status='active' AND (expires_at IS NULL OR expires_at>NOW()) FOR UPDATE`, t.APIKeyID, t.UserID, t.GroupID).Scan(&lockID)
		if err != nil {
			return nil, false, err
		}
		if err = incrementUsageBillingAPIKeyRateLimit(ctx, tx, t.APIKeyID, 0); err != nil {
			return nil, false, err
		}
		var ok bool
		err = tx.QueryRowContext(ctx, `SELECT (quota<=0 OR quota_used+$1<=quota)
 AND (rate_limit_5h<=0 OR usage_5h+$1<=rate_limit_5h)
 AND (rate_limit_1d<=0 OR usage_1d+$1<=rate_limit_1d)
 AND (rate_limit_7d<=0 OR usage_7d+$1<=rate_limit_7d),window_5h_start,window_1d_start,window_7d_start FROM api_keys WHERE id=$2`, t.ReservedUSD, t.APIKeyID).Scan(&ok, &t.KeyWindows[0], &t.KeyWindows[1], &t.KeyWindows[2])
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, service.ErrMiniMaxMediaFunds
		}
		if _, err = tx.ExecContext(ctx, `UPDATE api_keys SET quota_used=quota_used+$1 WHERE id=$2`, t.ReservedUSD, t.APIKeyID); err != nil {
			return nil, false, err
		}
		if err = incrementUsageBillingAPIKeyRateLimit(ctx, tx, t.APIKeyID, t.ReservedUSD); err != nil {
			return nil, false, err
		}
		if t.SubscriptionID != nil {
			err = tx.QueryRowContext(ctx, `SELECT
 (g.daily_limit_usd IS NULL OR g.daily_limit_usd<=0 OR us.daily_usage_usd+$1<=g.daily_limit_usd)
 AND (g.weekly_limit_usd IS NULL OR g.weekly_limit_usd<=0 OR us.weekly_usage_usd+$1<=g.weekly_limit_usd)
 AND (g.monthly_limit_usd IS NULL OR g.monthly_limit_usd<=0 OR us.monthly_usage_usd+$1<=g.monthly_limit_usd),
 us.daily_window_start,us.weekly_window_start,us.monthly_window_start
 FROM user_subscriptions us JOIN groups g ON g.id=us.group_id
 WHERE us.id=$2 AND us.user_id=$3 AND us.group_id=$4 AND us.deleted_at IS NULL AND g.deleted_at IS NULL
 AND us.status='active' AND us.starts_at<=NOW() AND us.expires_at>NOW() FOR UPDATE OF us`, t.ReservedUSD, *t.SubscriptionID, t.UserID, t.GroupID).Scan(&ok, &t.SubscriptionWindows[0], &t.SubscriptionWindows[1], &t.SubscriptionWindows[2])
			if err != nil {
				return nil, false, err
			}
			if !ok {
				return nil, false, service.ErrMiniMaxMediaFunds
			}
			if err = incrementUsageBillingSubscription(ctx, tx, *t.SubscriptionID, t.ReservedUSD); err != nil {
				return nil, false, err
			}
		}
	}
	windows, _ := json.Marshal(mediaWindows{t.SubscriptionWindows, t.KeyWindows})
	if _, err = tx.ExecContext(ctx, `UPDATE minimax_media_tasks SET reservation_windows=$1 WHERE id=$2`, windows, t.ID); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	t.Status, t.BillingState = "submitting", "reserved"
	return t, true, nil
}
func (r *miniMaxMediaRepository) Submitted(ctx context.Context, id, upstream string) error {
	var saved string
	err := r.db.QueryRowContext(ctx, `UPDATE minimax_media_tasks SET upstream_id=$1,status='queued',updated_at=NOW(),next_poll_at=NOW()+INTERVAL '10 seconds' WHERE id=$2 AND upstream_id IS NULL AND billing_state='reserved' AND bill IS NULL RETURNING upstream_id`, upstream, id).Scan(&saved)
	if errors.Is(err, sql.ErrNoRows) {
		// The first write can commit even if its response is lost. Accept only
		// the same immutable provider identity when retrying that database write.
		err = r.db.QueryRowContext(ctx, `SELECT upstream_id FROM minimax_media_tasks WHERE id=$1`, id).Scan(&saved)
	}
	if err != nil {
		return err
	}
	if saved != upstream {
		return errors.New("media submission identity conflict")
	}
	return nil
}
func (r *miniMaxMediaRepository) Record(ctx context.Context, id, status string, bill *service.MiniMaxMediaBill, result json.RawMessage) error {
	var b, res any
	if bill != nil {
		v, err := json.Marshal(bill)
		if err != nil {
			return err
		}
		b = v
	}
	if len(result) > 0 {
		if !json.Valid(result) {
			return errors.New("invalid media result")
		}
		res = []byte(result)
	}
	// A terminal receipt is immutable even when settlement needs retrying.
	_, err := r.db.ExecContext(ctx, `UPDATE minimax_media_tasks SET status=$1,bill=$2,result=COALESCE($3,result),updated_at=NOW(),next_poll_at=NOW()+INTERVAL '10 seconds' WHERE id=$4 AND billing_state='reserved' AND bill IS NULL`, status, b, res, id)
	return err
}
func (r *miniMaxMediaRepository) Claim(ctx context.Context, limit int) ([]*service.MiniMaxMediaTask, error) {
	columns := strings.Split(mediaColumns, ",")
	for i := range columns {
		columns[i] = "t." + columns[i]
	}
	rows, err := r.db.QueryContext(ctx, `WITH candidates AS (
 SELECT id FROM minimax_media_tasks WHERE billing_state='reserved' AND next_poll_at<=NOW()
 AND (bill IS NOT NULL OR (kind='video' AND upstream_id IS NOT NULL AND created_at>NOW()-INTERVAL '7 days'))
 ORDER BY next_poll_at LIMIT $1 FOR UPDATE SKIP LOCKED)
 UPDATE minimax_media_tasks t SET next_poll_at=NOW()+INTERVAL '60 seconds' FROM candidates c WHERE t.id=c.id RETURNING `+strings.Join(columns, ","), min(max(limit, 1), 100))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var tasks []*service.MiniMaxMediaTask
	for rows.Next() {
		t, e := scanMedia(rows)
		if e != nil {
			return nil, e
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (r *miniMaxMediaRepository) RefreshResult(ctx context.Context, id string, result json.RawMessage) error {
	if !json.Valid(result) {
		return errors.New("invalid media result")
	}
	_, err := r.db.ExecContext(ctx, `UPDATE minimax_media_tasks SET result=$1,updated_at=NOW() WHERE id=$2 AND billing_state='settled' AND status='succeeded'`, []byte(result), id)
	return err
}
func (r *miniMaxMediaRepository) Settle(ctx context.Context, id string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	t, err := scanMedia(tx.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM minimax_media_tasks WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return false, err
	}
	if t.BillingState != "reserved" {
		return false, nil
	}
	if t.Bill == nil {
		return false, errors.New("media receipt is required before settlement")
	}
	b, err := reconcileMiniMaxVoiceFees(ctx, tx, t)
	if err != nil {
		return false, err
	}
	if t.Status != "succeeded" && t.Status != "failed" && t.Status != "cancelled" {
		return false, errors.New("media is not terminal")
	}
	if t.Status != "succeeded" && (b.ChargedUSD != 0 || b.UpstreamUSD != 0) {
		return false, errors.New("failed media has usage; reconciliation required")
	}
	b.MeterSource, b.CompletedAt = t.Bill.MeterSource, t.Bill.CompletedAt
	windowCost := b.ChargedUSD
	if !t.UsesStandardBilling() {
		b.ChargedUSD = 0
	}
	if t.UsesStandardBilling() {
		var lockID int64
		if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, t.UserID).Scan(&lockID); err != nil {
			return false, err
		}
		if t.SubscriptionID == nil {
			res, e := tx.ExecContext(ctx, `UPDATE users SET balance=balance+$1,frozen_balance=frozen_balance-$1,updated_at=NOW() WHERE id=$2 AND frozen_balance >= $1`, t.ReservedUSD, t.UserID)
			if e != nil {
				return false, e
			}
			n, e := res.RowsAffected()
			if e != nil {
				return false, e
			}
			if n != 1 {
				return false, errors.New("media hold is missing; reconciliation required")
			}
		}
		// Refund only original spend windows. A reset must never receive a refund
		// for an older window; the actual usage is applied by the normal ledger.
		if t.SubscriptionID != nil {
			if err = tx.QueryRowContext(ctx, `SELECT id FROM user_subscriptions WHERE id=$1 FOR UPDATE`, *t.SubscriptionID).Scan(&lockID); err != nil {
				return false, err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE api_keys SET quota_used=GREATEST(0,quota_used-$1),
 status=CASE WHEN status='quota_exhausted' AND quota_used-$1<quota THEN 'active' ELSE status END,
 usage_5h=CASE WHEN window_5h_start IS NOT DISTINCT FROM $3::timestamptz THEN GREATEST(0,usage_5h-$1) ELSE usage_5h END,
 usage_1d=CASE WHEN window_1d_start IS NOT DISTINCT FROM $4::timestamptz THEN GREATEST(0,usage_1d-$1) ELSE usage_1d END,
 usage_7d=CASE WHEN window_7d_start IS NOT DISTINCT FROM $5::timestamptz THEN GREATEST(0,usage_7d-$1) ELSE usage_7d END,updated_at=NOW() WHERE id=$2`, t.ReservedUSD, t.APIKeyID, t.KeyWindows[0], t.KeyWindows[1], t.KeyWindows[2])
		if err != nil {
			return false, err
		}
		if t.SubscriptionID != nil {
			_, err = tx.ExecContext(ctx, `UPDATE user_subscriptions SET
 daily_usage_usd=CASE WHEN daily_window_start IS NOT DISTINCT FROM $3::timestamptz THEN GREATEST(0,daily_usage_usd-$1) ELSE daily_usage_usd END,
 weekly_usage_usd=CASE WHEN weekly_window_start IS NOT DISTINCT FROM $4::timestamptz THEN GREATEST(0,weekly_usage_usd-$1) ELSE weekly_usage_usd END,
 monthly_usage_usd=CASE WHEN monthly_window_start IS NOT DISTINCT FROM $5::timestamptz THEN GREATEST(0,monthly_usage_usd-$1) ELSE monthly_usage_usd END,updated_at=NOW() WHERE id=$2`, t.ReservedUSD, *t.SubscriptionID, t.SubscriptionWindows[0], t.SubscriptionWindows[1], t.SubscriptionWindows[2])
			if err != nil {
				return false, err
			}
		}
	} else if t.TracksKeyWindows() {
		if err = refundMiniMaxSimpleWindows(ctx, tx, t); err != nil {
			return false, err
		}
	}
	cmd := &service.UsageBillingCommand{RequestID: t.ID, APIKeyID: t.APIKeyID, UserID: t.UserID, AccountID: t.AccountID, SubscriptionID: t.SubscriptionID, AccountType: t.AccountType, Model: t.Model, MediaType: t.Kind, RequestPayloadHash: t.RequestHash}
	if t.UsesStandardBilling() {
		cmd.APIKeyQuotaCost, cmd.APIKeyRateLimitCost, cmd.AccountQuotaCost = b.ChargedUSD, b.ChargedUSD, b.UpstreamUSD
		if t.SubscriptionID == nil {
			cmd.BalanceCost = b.ChargedUSD
		} else {
			cmd.SubscriptionCost = b.ChargedUSD
			cmd.BillingType = service.BillingTypeSubscription
		}
	} else if t.TracksKeyWindows() {
		cmd.APIKeyRateLimitCost = windowCost
	}
	cmd.Normalize()
	ledger := &usageBillingRepository{db: r.db}
	claimed, err := ledger.claimUsageBillingKey(ctx, tx, cmd)
	if err != nil {
		return false, err
	}
	if !claimed {
		return false, errors.New("media ledger exists without terminal task")
	}
	if err = ledger.applyUsageBillingEffects(ctx, tx, cmd, &service.UsageBillingApplyResult{}); err != nil {
		return false, err
	}
	mode, endpoint, videoCount := "per_request", "/v1/t2a_v2", 0
	if t.Kind == "video" {
		mode, endpoint, videoCount = "video", "/v2/video_generation", 1
	}
	if t.Kind == "image" {
		mode, endpoint = "image", "/v1/image_generation"
	}
	if t.Kind == "voice_clone" || t.Kind == "voice_design" {
		endpoint = "/v1/" + t.Kind
	}
	if t.Status == "succeeded" {
		var duration any
		var imageSize, imageSizeSource any
		if t.Kind == "image" {
			imageSize, imageSizeSource = service.ImageBillingSizePerImage, service.ImageSizeSourceDefault
		}
		if !t.RequestInfo.StartedAt.IsZero() {
			completedAt := b.CompletedAt
			if completedAt.IsZero() {
				completedAt = time.Now()
			}
			duration = max(completedAt.Sub(t.RequestInfo.StartedAt).Milliseconds(), 0)
		}
		requestedModel := t.RequestedModel
		if requestedModel == "" {
			requestedModel = t.Model
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,requested_model,group_id,subscription_id,total_cost,actual_cost,rate_multiplier,billing_type,billing_mode,stream,request_type,inbound_endpoint,upstream_endpoint,video_count,video_resolution,video_duration_seconds,account_stats_cost,account_rate_multiplier,image_count,duration_ms,user_agent,ip_address,image_size,image_size_source,created_at)
 VALUES($1,$2,$3,$4,$5,$26,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$15,$16,$17,$18,$19,1,$20,$21,$22,$23,$24,$25,NOW())`, t.UserID, t.APIKeyID, t.AccountID, t.ID, t.Model, t.GroupID, t.SubscriptionID, b.RetailUSD, b.ChargedUSD, *t.Pricing.Multiplier, cmd.BillingType, mode, t.Stream, service.RequestTypeFromLegacy(t.Stream, false), endpoint, videoCount, t.Resolution, t.Duration, b.UpstreamUSD, b.Usage.OutputImages, duration, t.RequestInfo.UserAgent, t.RequestInfo.IPAddress, imageSize, imageSizeSource, requestedModel)
		if err != nil {
			return false, err
		}
	}
	state := "settled"
	if t.Status != "succeeded" {
		state = "released"
	}
	savedBill, err := json.Marshal(b)
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE minimax_media_tasks SET billing_state=$1,bill=$3,updated_at=NOW() WHERE id=$2`, state, t.ID, savedBill); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// Simple mode never reserves wallet, subscription, lifetime key or account quota.
// Only its explicit key-window option reserves and applies monetary windows.
func reserveMiniMaxSimple(ctx context.Context, tx *sql.Tx, t *service.MiniMaxMediaTask) error {
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL AND status='active' FOR UPDATE`, t.UserID).Scan(&id); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM api_keys WHERE id=$1 AND user_id=$2 AND group_id=$3 AND deleted_at IS NULL AND status IN ('active','expired','quota_exhausted') FOR UPDATE`, t.APIKeyID, t.UserID, t.GroupID).Scan(&id); err != nil {
		return err
	}
	if !t.TracksKeyWindows() {
		return nil
	}
	if err := incrementUsageBillingAPIKeyRateLimit(ctx, tx, t.APIKeyID, 0); err != nil {
		return err
	}
	var ok bool
	err := tx.QueryRowContext(ctx, `SELECT
 (rate_limit_5h<=0 OR usage_5h+$1<=rate_limit_5h)
 AND (rate_limit_1d<=0 OR usage_1d+$1<=rate_limit_1d)
 AND (rate_limit_7d<=0 OR usage_7d+$1<=rate_limit_7d),window_5h_start,window_1d_start,window_7d_start FROM api_keys WHERE id=$2`, t.ReservedUSD, t.APIKeyID).Scan(&ok, &t.KeyWindows[0], &t.KeyWindows[1], &t.KeyWindows[2])
	if err != nil {
		return err
	}
	if !ok {
		return service.ErrMiniMaxMediaFunds
	}
	return incrementUsageBillingAPIKeyRateLimit(ctx, tx, t.APIKeyID, t.ReservedUSD)
}

func refundMiniMaxSimpleWindows(ctx context.Context, tx *sql.Tx, t *service.MiniMaxMediaTask) error {
	_, err := tx.ExecContext(ctx, `UPDATE api_keys SET
 usage_5h=CASE WHEN window_5h_start IS NOT DISTINCT FROM $3::timestamptz THEN GREATEST(0,usage_5h-$1) ELSE usage_5h END,
 usage_1d=CASE WHEN window_1d_start IS NOT DISTINCT FROM $4::timestamptz THEN GREATEST(0,usage_1d-$1) ELSE usage_1d END,
 usage_7d=CASE WHEN window_7d_start IS NOT DISTINCT FROM $5::timestamptz THEN GREATEST(0,usage_7d-$1) ELSE usage_7d END,updated_at=NOW() WHERE id=$2`, t.ReservedUSD, t.APIKeyID, t.KeyWindows[0], t.KeyWindows[1], t.KeyWindows[2])
	return err
}
