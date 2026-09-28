package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/shopspring/decimal"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type museProviderRepository struct{ db *sql.DB }

func NewMuseProviderRepository(db *sql.DB) service.MuseProviderStore {
	return &museProviderRepository{db: db}
}

func (r *museProviderRepository) SaveProfile(ctx context.Context, a *service.Account, o *muse.Observation) error {
	if a == nil || o == nil || a.Platform != service.PlatformMuse || o.Identity.AccountID != a.ID || o.Identity.OwnerUserID != service.MuseOwnerUserID(a.Extra) {
		return muse.ErrOwner
	}
	encoded, err := json.Marshal(o)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `INSERT INTO muse_account_profiles(account_id,verified_account_updated_at,capabilities,verified_at)
		SELECT id,updated_at,$3::jsonb,NOW() FROM accounts WHERE id=$1 AND updated_at=$2 AND status='active' AND deleted_at IS NULL
		ON CONFLICT(account_id) DO UPDATE SET verified_account_updated_at=EXCLUDED.verified_account_updated_at,
		capabilities=EXCLUDED.capabilities,verified_at=NOW(),renewal_not_before=NULL`, a.ID, a.UpdatedAt, string(encoded))
	if err != nil {
		return err
	}
	return museOneRow(result, muse.ErrGeneration)
}

func (r *museProviderRepository) Profile(ctx context.Context, a *service.Account) (*muse.Observation, error) {
	if a == nil || !a.IsMuse() {
		return nil, muse.ErrOwner
	}
	var encoded []byte
	err := r.db.QueryRowContext(ctx, `SELECT p.capabilities FROM muse_account_profiles p JOIN accounts a ON a.id=p.account_id LEFT JOIN proxies px ON px.id=a.proxy_id
  WHERE a.id=$1 AND a.updated_at=p.verified_account_updated_at AND a.status='active' AND a.deleted_at IS NULL
  AND (a.proxy_id IS NULL OR (px.deleted_at IS NULL AND px.updated_at=(p.capabilities->>'proxy_updated_at')::timestamptz))`, a.ID).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, muse.ErrTransportUnqualified
	}
	if err != nil {
		return nil, err
	}
	var result muse.Observation
	if err = json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

type museFrozenCharge struct {
	Command service.UsageBillingCommand `json:"command"`
	Log     service.UsageLog            `json:"log"`
}

func (r *museProviderRepository) FreezeCharge(ctx context.Context, id string, cmd *service.UsageBillingCommand, log *service.UsageLog) error {
	if cmd == nil || log == nil || cmd.RequestID != id || log.RequestID != id {
		return muse.ErrInvalid
	}
	var pricingData []byte
	if err := r.db.QueryRowContext(ctx, `SELECT pricing_snapshot FROM muse_turns WHERE id=$1`, id).Scan(&pricingData); err != nil {
		return err
	}
	var pricing muse.Pricing
	if err := json.Unmarshal(pricingData, &pricing); err != nil {
		return err
	}
	if err := validateMuseCharge(pricing, cmd, log); err != nil {
		return err
	}
	cmd.Normalize()
	copyLog := *log
	copyLog.User = nil
	copyLog.APIKey = nil
	copyLog.Account = nil
	copyLog.Group = nil
	copyLog.Subscription = nil
	encoded, err := json.Marshal(museFrozenCharge{Command: *cmd, Log: copyLog})
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE muse_turns SET billing_command=$2::jsonb,requested_model=$3,group_id=$4,subscription_id=$5
		WHERE id=$1 AND state='reserved' AND billing_command IS NULL AND owner_user_id=$6 AND api_key_id=$7 AND account_id=$8`,
		id, string(encoded), log.Model, log.GroupID, log.SubscriptionID, cmd.UserID, cmd.APIKeyID, cmd.AccountID)
	if err != nil {
		return err
	}
	return museOneRow(result, muse.ErrTransition)
}

func (r *museProviderRepository) EnableVerified(ctx context.Context, a *service.Account) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE accounts a SET schedulable=TRUE WHERE a.id=$1 AND a.updated_at=$2 AND a.status='active' AND a.deleted_at IS NULL
		AND EXISTS(SELECT 1 FROM muse_account_profiles p WHERE p.account_id=a.id AND p.verified_account_updated_at=a.updated_at)
		AND EXISTS(SELECT 1 FROM muse_account_workspace_bindings b WHERE b.account_id=a.id AND b.verified_account_updated_at=a.updated_at)`, a.ID, a.UpdatedAt)
	if err != nil {
		return err
	}
	if err = museOneRow(result, muse.ErrGeneration); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO scheduler_outbox(event_type,account_id,created_at) VALUES($1,$2,NOW())`, service.SchedulerOutboxEventAccountChanged, a.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Lock the canonical workspace before invoking renewal. This prevents any replica
// or alias from submitting while a remote session is being rotated.
func (r *museProviderRepository) RenewSession(ctx context.Context, a *service.Account, renew func(context.Context) (map[string]any, error)) error {
	if a == nil || renew == nil {
		return muse.ErrInvalid
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var workspaceID int64
	err = tx.QueryRowContext(ctx, `SELECT workspace_id FROM muse_account_workspace_bindings WHERE account_id=$1`, a.ID).Scan(&workspaceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		w, e := lockMuseWorkspace(ctx, tx, workspaceID)
		if e != nil {
			return e
		}
		if w.ActiveTurnID != "" {
			return muse.ErrBusy
		}
	}
	var current bool
	err = tx.QueryRowContext(ctx, `SELECT updated_at=$2 AND credentials=$3::jsonb AND proxy_id IS NOT DISTINCT FROM $4 AND status='active' AND deleted_at IS NULL FROM accounts WHERE id=$1 FOR UPDATE`, a.ID, a.UpdatedAt, mustMuseJSON(a.Credentials), a.ProxyID).Scan(&current)
	if err != nil {
		return err
	}
	if !current {
		return muse.ErrGeneration
	}
	if a.ProxyID != nil {
		if a.Proxy == nil {
			return muse.ErrGeneration
		}
		var at time.Time
		if err = tx.QueryRowContext(ctx, `SELECT updated_at FROM proxies WHERE id=$1 AND deleted_at IS NULL FOR SHARE`, *a.ProxyID).Scan(&at); err != nil {
			return err
		}
		if !at.Equal(a.Proxy.UpdatedAt) {
			return muse.ErrGeneration
		}
	}
	probe, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	document, err := renew(probe)
	if err != nil {
		return err
	}
	credentials := map[string]any{"muse_session": document}
	if mapping, ok := a.Credentials["model_mapping"]; ok {
		credentials["model_mapping"] = mapping
	}
	if err := service.ValidateMuseAccount(a.Platform, a.Type, credentials, a.Extra); err != nil {
		return err
	}
	old, err := json.Marshal(a.Credentials)
	if err != nil {
		return err
	}
	next, err := json.Marshal(credentials)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE accounts a SET credentials=$4::jsonb,updated_at=clock_timestamp(),schedulable=FALSE
		WHERE id=$1 AND updated_at=$2 AND credentials=$3::jsonb AND proxy_id IS NOT DISTINCT FROM $5
		AND status='active' AND deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM muse_account_workspace_bindings b JOIN muse_workspace_bindings w ON w.id=b.workspace_id WHERE b.account_id=a.id AND w.active_turn_id IS NOT NULL)`, a.ID, a.UpdatedAt, string(old), string(next), a.ProxyID)
	if err != nil {
		return err
	}
	if err = museOneRow(result, muse.ErrGeneration); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO scheduler_outbox(event_type,account_id,created_at) VALUES($1,$2,NOW())`, service.SchedulerOutboxEventAccountChanged, a.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *museProviderRepository) AdminTurn(ctx context.Context, id string) (*muse.Turn, error) {
	return scanMuseTurn(r.db.QueryRowContext(ctx, `SELECT `+museTurnColumns+` FROM muse_turns WHERE id=$1`, id))
}

// Settlement uses the existing billing primitives and log serializer inside one
// transaction, together with the native turn link. No second ledger is introduced.
func (r *museProviderRepository) Settle(ctx context.Context, id string) (*service.MuseSettlement, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var state muse.State
	var encoded []byte
	var settled sql.NullTime
	var observed sql.NullString
	var reported []byte
	var providerID string
	err = tx.QueryRowContext(ctx, `SELECT state,billing_command,settled_at,observed_model,reported_usage,provider_turn_id FROM muse_turns WHERE id=$1 FOR UPDATE`, id).Scan(&state, &encoded, &settled, &observed, &reported, &providerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, muse.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if !state.Terminal() || len(encoded) == 0 {
		return nil, muse.ErrTransition
	}
	var frozen museFrozenCharge
	if err = json.Unmarshal(encoded, &frozen); err != nil {
		return nil, err
	}
	if observed.Valid {
		frozen.Log.UpstreamResponseModel = &observed.String
	}
	if providerID != "" {
		frozen.Log.UpstreamRequestID = &providerID
	}
	if len(reported) > 0 && string(reported) != "null" {
		var usage apicompat.ResponsesUsage
		if err = json.Unmarshal(reported, &usage); err != nil {
			return nil, err
		}
		frozen.Log.InputTokens = usage.InputTokens
		frozen.Log.OutputTokens = usage.OutputTokens
	}
	if settled.Valid {
		return &service.MuseSettlement{Command: frozen.Command, Log: frozen.Log}, nil
	}
	if state != muse.Completed {
		frozen.Command.BalanceCost = 0
		frozen.Command.SubscriptionCost = 0
		frozen.Command.APIKeyQuotaCost = 0
		frozen.Command.APIKeyRateLimitCost = 0
		frozen.Command.AccountQuotaCost = 0
		frozen.Command.RequestFingerprint = ""
		frozen.Command.Normalize()
		frozen.Log.ActualCost = 0
		frozen.Log.TotalCost = 0
	}
	billing := &usageBillingRepository{db: r.db}
	applied, err := billing.claimUsageBillingKey(ctx, tx, &frozen.Command)
	if err != nil {
		return nil, err
	}
	billingResult := &service.UsageBillingApplyResult{Applied: applied}
	if applied {
		if err = billing.applyUsageBillingEffects(ctx, tx, &frozen.Command, billingResult); err != nil {
			return nil, err
		}
	}
	if applied && frozen.Command.BalanceCost > 0 {
		now := time.Now()
		// Muse uses DB-authoritative platform quotas so a crash between settlement
		// and cache invalidation cannot lose a charge or a quota increment.
		_, err = tx.ExecContext(ctx, `UPDATE user_platform_quotas SET
   daily_usage_usd=CASE WHEN daily_window_start IS NULL OR daily_window_start<$3 THEN $5 ELSE daily_usage_usd+$5 END,
   weekly_usage_usd=CASE WHEN weekly_window_start IS NULL OR weekly_window_start<$4 THEN $5 ELSE weekly_usage_usd+$5 END,
   monthly_usage_usd=CASE WHEN monthly_window_start IS NULL OR monthly_window_start<=$6 THEN $5 ELSE monthly_usage_usd+$5 END,
   daily_window_start=$3,weekly_window_start=$4,
   monthly_window_start=CASE WHEN monthly_window_start IS NULL OR monthly_window_start<=$6 THEN $7 ELSE monthly_window_start END,updated_at=NOW()
   WHERE user_id=$1 AND platform=$2 AND deleted_at IS NULL`, frozen.Command.UserID, muse.Platform, timezone.StartOfDay(now), timezone.StartOfWeek(now), frozen.Command.BalanceCost, now.AddDate(0, 0, -30), now)
		if err != nil {
			return nil, err
		}
	}
	logs := newUsageLogRepositoryWithSQL(nil, tx)
	if _, err = logs.createSingle(ctx, tx, &frozen.Log); err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE muse_turns SET usage_log_id=$2,settled_at=NOW() WHERE id=$1`, id, frozen.Log.ID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &service.MuseSettlement{Applied: applied, Result: billingResult, Command: frozen.Command, Log: frozen.Log}, nil
}

func (r *museProviderRepository) PendingRecovery(ctx context.Context, limit int) ([]int64, error) {
	if limit < 1 || limit > 100 {
		return nil, muse.ErrInvalid
	}
	rows, err := r.db.QueryContext(ctx, `SELECT w.id FROM muse_workspace_bindings w JOIN muse_turns t ON t.id=w.active_turn_id
		WHERE w.lease_until<clock_timestamp() AND t.state<>'owner_review' ORDER BY w.lease_until LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *museProviderRepository) ResolveOwnerReview(ctx context.Context, id string, actor muse.Actor, outcome muse.State) error {
	if outcome != muse.Completed && outcome != muse.Failed && outcome != muse.Cancelled {
		return muse.ErrTransition
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var workspaceID int64
	err = tx.QueryRowContext(ctx, `SELECT workspace_id FROM muse_turns WHERE id=$1 AND owner_user_id=$2 AND api_key_id=$3`, id, actor.UserID, actor.APIKeyID).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return muse.ErrNotFound
	}
	if err != nil {
		return err
	}
	w, err := lockMuseWorkspace(ctx, tx, workspaceID)
	if err != nil {
		return err
	}
	if w.ActiveTurnID != id {
		return muse.ErrTransition
	}
	result, err := tx.ExecContext(ctx, `UPDATE muse_turns SET state=$2,updated_at=NOW() WHERE id=$1 AND state='owner_review'`, id, outcome)
	if err != nil {
		return err
	}
	if err = museOneRow(result, muse.ErrTransition); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE muse_workspace_bindings SET active_turn_id=NULL,lease_owner=NULL,lease_until=NULL,lease_fence=lease_fence+1 WHERE id=$1`, workspaceID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func mustMuseJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func validateMuseCharge(p muse.Pricing, cmd *service.UsageBillingCommand, log *service.UsageLog) error {
	if err := p.Validate(); err != nil {
		return err
	}
	unit, _ := decimal.NewFromString(p.UnitPrice)
	multiplier, _ := decimal.NewFromString(p.Multiplier)
	actual := unit.Mul(multiplier).Round(8)
	if unit.GreaterThanOrEqual(decimal.New(1, 10)) || actual.GreaterThanOrEqual(decimal.New(1, 10)) {
		return muse.ErrInvalid
	}
	equal := func(v float64, want decimal.Decimal) bool {
		return !math.IsNaN(v) && !math.IsInf(v, 0) && decimal.NewFromFloat(v).Equal(want)
	}
	if cmd.BillingType != log.BillingType || log.InputTokens != 0 || log.OutputTokens != 0 || cmd.UserID != log.UserID || cmd.APIKeyID != log.APIKeyID || cmd.AccountID != log.AccountID || cmd.Model != log.Model || cmd.AccountType != service.AccountTypeSession ||
		!equal(log.TotalCost, unit) || !equal(log.ActualCost, actual) || !equal(log.RateMultiplier, multiplier) ||
		!equal(cmd.BalanceCost+cmd.SubscriptionCost, actual) || cmd.AccountQuotaCost != 0 || cmd.InputTokens != 0 || cmd.OutputTokens != 0 ||
		(cmd.APIKeyQuotaCost != 0 && !equal(cmd.APIKeyQuotaCost, actual)) || (cmd.APIKeyRateLimitCost != 0 && !equal(cmd.APIKeyRateLimitCost, actual)) ||
		cmd.BalanceCost < 0 || cmd.SubscriptionCost < 0 || (cmd.BalanceCost > 0 && cmd.SubscriptionCost > 0) ||
		(cmd.SubscriptionCost > 0 && (cmd.SubscriptionID == nil || log.SubscriptionID == nil || *cmd.SubscriptionID != *log.SubscriptionID)) {
		return muse.ErrInvalid
	}
	return nil
}

func (r *museProviderRepository) PendingSettlement(ctx context.Context, limit int) ([]string, error) {
	if limit < 1 || limit > 100 {
		return nil, muse.ErrInvalid
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM muse_turns WHERE state IN ('completed','failed','cancelled','rejected') AND billing_command IS NOT NULL AND settled_at IS NULL AND settlement_attempts<8 AND (settlement_not_before IS NULL OR settlement_not_before<clock_timestamp()) ORDER BY updated_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *museProviderRepository) PendingTurns(ctx context.Context, accountID int64) ([]*muse.Turn, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+museTurnColumns+` FROM muse_turns WHERE workspace_id IN (SELECT workspace_id FROM muse_account_workspace_bindings WHERE account_id=$1) AND (state NOT IN ('completed','failed','cancelled','rejected') OR (billing_command IS NOT NULL AND settled_at IS NULL)) ORDER BY created_at LIMIT 25`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	turns := []*muse.Turn{}
	for rows.Next() {
		turn, e := scanMuseTurn(rows)
		if e != nil {
			return nil, e
		}
		turns = append(turns, turn)
	}
	return turns, rows.Err()
}

// Only normalized attribution and provider-reported usage are persisted. Remote
// output, reasoning, session documents, and browser state never enter this row.
func (r *museProviderRepository) RecordResult(ctx context.Context, id string, result *muse.Result) error {
	if result == nil || result.Response == nil || result.ProviderTurnID == "" {
		return muse.ErrInvalid
	}
	var usage any
	if result.Response.Usage != nil {
		usage = result.Response.Usage
	}
	encoded, err := json.Marshal(usage)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE muse_turns SET observed_model=$2,reported_usage=$3::jsonb WHERE id=$1 AND provider_turn_id=$4 AND state IN ('accepted','running','ambiguous','cancel_pending')`, id, result.Response.Model, string(encoded), result.ProviderTurnID)
	if err != nil {
		return err
	}
	return museOneRow(res, muse.ErrTransition)
}

// Unknown expiries are never scheduled using a guessed refresh interval.
func (r *museProviderRepository) DueRenewal(ctx context.Context, limit int) ([]int64, error) {
	if limit < 1 || limit > 100 {
		return nil, muse.ErrInvalid
	}
	rows, err := r.db.QueryContext(ctx, `SELECT p.account_id FROM muse_account_profiles p JOIN accounts a ON a.id=p.account_id
 WHERE a.updated_at=p.verified_account_updated_at AND a.status='active' AND a.deleted_at IS NULL
 AND p.capabilities->>'session_expires_at' IS NOT NULL
 AND (p.capabilities->>'session_expires_at')::timestamptz<=clock_timestamp()+INTERVAL '5 minutes'
 AND (p.renewal_not_before IS NULL OR p.renewal_not_before<=clock_timestamp())
 ORDER BY p.verified_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (r *museProviderRepository) DeferRenewal(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE muse_account_profiles SET renewal_not_before=clock_timestamp()+INTERVAL '5 minutes' WHERE account_id=$1`, id)
	return err
}

func (r *museProviderRepository) DeferSettlement(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE muse_turns SET settlement_attempts=settlement_attempts+1,settlement_not_before=clock_timestamp()+LEAST(60,POWER(2,LEAST(settlement_attempts,6)))*INTERVAL '1 minute' WHERE id=$1 AND settled_at IS NULL`, id)
	return err
}
