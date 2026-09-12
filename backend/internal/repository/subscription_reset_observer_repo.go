package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type subscriptionResetObserverRepository struct {
	db    *sql.DB
	quota service.SubscriptionQuotaRepository
}

func NewSubscriptionResetObserverRepository(db *sql.DB) service.SubscriptionResetObserverRepository {
	return &subscriptionResetObserverRepository{db: db}
}

const subscriptionResetEligibleAccountSQL = `a.deleted_at IS NULL AND a.platform='openai' AND a.type='oauth'
 AND a.parent_account_id IS NULL AND LOWER(TRIM(COALESCE(a.quota_dimension,''))) IN ('','global')
 AND LOWER(TRIM(COALESCE(a.credentials->>'auth_mode',''))) NOT IN ('agent_identity','personal_access_token','personalaccesstoken')
 AND LOWER(TRIM(COALESCE(a.credentials->>'openai_auth_mode',''))) NOT IN ('agent_identity','personal_access_token','personalaccesstoken')
 AND LOWER(TRIM(COALESCE(a.credentials->>'access_token',''))) NOT LIKE 'at-%'`

func (r *subscriptionResetObserverRepository) GetPolicy(ctx context.Context, groupID int64) (*service.SubscriptionResetPolicy, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT p.policy FROM subscription_reset_policies p JOIN groups g ON g.id=p.group_id WHERE p.group_id=$1 AND g.deleted_at IS NULL`, groupID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return service.DefaultSubscriptionResetPolicy(groupID), nil
	}
	if err != nil {
		return nil, err
	}
	var p service.SubscriptionResetPolicy
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("decode subscription reset policy: %w", err)
	}
	return &p, nil
}

// Version is the caller's expected version, including zero for the first save.
// Reconfiguration always starts a new baseline and invalidates pending batches;
// in-flight queries carrying the former version cannot affect the new policy.
func (r *subscriptionResetObserverRepository) SavePolicy(ctx context.Context, input *service.SubscriptionResetPolicy) (*service.SubscriptionResetPolicy, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	p := *input
	p.AccountIDs = append([]int64{}, input.AccountIDs...)
	p.ResetDimensions = append([]string{}, input.ResetDimensions...)
	sort.Slice(p.AccountIDs, func(i, j int) bool { return p.AccountIDs[i] < p.AccountIDs[j] })
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// Also serializes creation of a group's first policy row.
	var groupType, platform string
	err = tx.QueryRowContext(ctx, `SELECT subscription_type,platform FROM groups WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, p.GroupID).Scan(&groupType, &platform)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGroupNotFound
	}
	if err != nil {
		return nil, err
	}
	if groupType != service.SubscriptionTypeSubscription || platform != service.PlatformOpenAI {
		return nil, infraerrors.BadRequest("INVALID_SUBSCRIPTION_RESET_GROUP", "reset observation requires an OpenAI subscription group")
	}
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT version FROM subscription_reset_policies WHERE group_id=$1 FOR UPDATE`, p.GroupID).Scan(&version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if p.Version != version {
		return nil, service.ErrSubscriptionResetPolicyConflict
	}
	if p.Mode != "off" {
		for _, accountID := range p.AccountIDs {
			var eligible bool
			err = tx.QueryRowContext(ctx, `SELECT (`+subscriptionResetEligibleAccountSQL+`) FROM accounts a JOIN account_groups ag ON ag.account_id=a.id AND ag.group_id=$2 WHERE a.id=$1 FOR SHARE OF a,ag`, accountID, p.GroupID).Scan(&eligible)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && !eligible) {
				return nil, infraerrors.BadRequest("INVALID_SUBSCRIPTION_RESET_ACCOUNTS", "reference accounts must be ordinary OpenAI OAuth accounts belonging to the group")
			}
			if err != nil {
				return nil, err
			}
		}
	}
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&p.UpdatedAt); err != nil {
		return nil, err
	}
	if p.Mode == "auto" {
		if r.quota == nil {
			return nil, service.ErrSubscriptionQuotaUnavailable
		}
		if _, err := r.quota.EnableGroupQuota(WithSubscriptionQuotaSQLTx(ctx, tx), p.GroupID, p.UpdatedAt); err != nil {
			return nil, err
		}
	}
	p.Version = version + 1
	policyJSON, err := json.Marshal(&p)
	if err != nil {
		return nil, err
	}
	stateJSON, err := json.Marshal(service.NewSubscriptionResetObserverState())
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO subscription_reset_policies(group_id,version,mode,policy,state,updated_at)
 VALUES($1,$2,$3,$4::jsonb,$5::jsonb,$6)
 ON CONFLICT(group_id) DO UPDATE SET version=EXCLUDED.version,mode=EXCLUDED.mode,policy=EXCLUDED.policy,state=EXCLUDED.state,updated_at=EXCLUDED.updated_at`, p.GroupID, p.Version, p.Mode, string(policyJSON), string(stateJSON), p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE subscription_reset_events SET updated_at=$2,
 payload=payload || jsonb_build_object('status','config_changed','reason','policy_reconfigured','updated_at',$2::timestamptz)
 WHERE group_id=$1 AND payload->>'status' IN ('pending','needs_review')`, p.GroupID, p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *subscriptionResetObserverRepository) ListEnabled(ctx context.Context) ([]*service.SubscriptionResetPolicy, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT p.policy FROM subscription_reset_policies p JOIN groups g ON g.id=p.group_id WHERE p.mode IN ('observe','auto') AND g.deleted_at IS NULL AND g.subscription_type='subscription' AND g.platform='openai' ORDER BY p.group_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []*service.SubscriptionResetPolicy{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var p service.SubscriptionResetPolicy
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *subscriptionResetObserverRepository) RecordSample(ctx context.Context, groupID, expectedVersion int64, input *service.SubscriptionResetSample) error {
	if input == nil {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Match SavePolicy's group -> policy -> account lock order and prevent a
	// concurrent group deletion/type change from admitting stale observations.
	var groupType, platform string
	err = tx.QueryRowContext(ctx, `SELECT subscription_type,platform FROM groups WHERE id=$1 AND deleted_at IS NULL FOR SHARE`, groupID).Scan(&groupType, &platform)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (groupType != service.SubscriptionTypeSubscription || platform != service.PlatformOpenAI)) {
		return service.ErrGroupNotFound
	}
	if err != nil {
		return err
	}
	var rawPolicy, rawState []byte
	var version int64
	var now time.Time
	err = tx.QueryRowContext(ctx, `SELECT version,policy,state,clock_timestamp() FROM subscription_reset_policies WHERE group_id=$1 FOR UPDATE`, groupID).Scan(&version, &rawPolicy, &rawState, &now)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && version != expectedVersion) {
		return service.ErrSubscriptionResetPolicyConflict
	}
	if err != nil {
		return err
	}
	var policy service.SubscriptionResetPolicy
	state := service.NewSubscriptionResetObserverState()
	if err := json.Unmarshal(rawPolicy, &policy); err != nil {
		return err
	}
	if err := json.Unmarshal(rawState, state); err != nil {
		return err
	}
	if policy.Mode == "off" {
		return nil
	}
	selected := false
	for _, id := range policy.AccountIDs {
		selected = selected || id == input.AccountID
	}
	if !selected {
		return nil
	}
	sample := *input
	// Recheck membership, credential kind, and the local reset marker while the
	// policy is locked. Holding account/membership share locks closes the race
	// between an upstream GET and a concurrent local reset or account move.
	var eligible bool
	var status, marker string
	err = tx.QueryRowContext(ctx, `SELECT (`+subscriptionResetEligibleAccountSQL+`),a.status,COALESCE(a.extra->>'codex_history_reset_at','')
 FROM accounts a JOIN account_groups ag ON ag.account_id=a.id AND ag.group_id=$2 WHERE a.id=$1 FOR SHARE OF a,ag`, sample.AccountID, groupID).Scan(&eligible, &status, &marker)
	if errors.Is(err, sql.ErrNoRows) {
		sample.Error = "account_removed"
	} else if err != nil {
		return err
	} else if !eligible {
		sample.Error = "account_ineligible"
	} else if status != service.StatusActive {
		sample.Error = "account_disabled"
	}
	if marker != "" {
		localAt, parseErr := time.Parse(time.RFC3339Nano, marker)
		if parseErr != nil {
			sample.Error = "invalid_local_reset_marker"
		} else if sample.LocalResetAt == nil || localAt.After(*sample.LocalResetAt) {
			sample.LocalResetAt = &localAt
		}
	}
	changed, err := service.AdvanceSubscriptionResetObserver(&policy, state, &sample, now)
	if err != nil {
		return err
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE subscription_reset_policies SET state=$2::jsonb WHERE group_id=$1`, groupID, string(stateJSON)); err != nil {
		return err
	}
	for _, event := range changed {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO subscription_reset_events(id,group_id,policy_version,opened_at,updated_at,payload)
 VALUES($1,$2,$3,$4,$5,$6::jsonb) ON CONFLICT(id) DO UPDATE SET updated_at=EXCLUDED.updated_at,payload=EXCLUDED.payload`, event.ID, groupID, version, event.OpenedAt, event.UpdatedAt, string(payload))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *subscriptionResetObserverRepository) GetStatus(ctx context.Context, groupID int64, limit int) (*service.SubscriptionResetStatus, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	// A repeatable-read snapshot keeps the policy version and audit list coherent
	// when an administrator saves while the panel is loading.
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	policy := service.DefaultSubscriptionResetPolicy(groupID)
	state := service.NewSubscriptionResetObserverState()
	var rawPolicy, rawState []byte
	err = tx.QueryRowContext(ctx, `SELECT policy,state FROM subscription_reset_policies WHERE group_id=$1`, groupID).Scan(&rawPolicy, &rawState)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(rawPolicy, policy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rawState, state); err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT payload FROM subscription_reset_events WHERE group_id=$1 ORDER BY opened_at DESC,id DESC LIMIT $2`, groupID, limit)
	if err != nil {
		return nil, err
	}
	events := []*service.SubscriptionResetEvent{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		var event service.SubscriptionResetEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			_ = rows.Close()
			return nil, err
		}
		events = append(events, &event)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return service.SubscriptionResetObserverStatus(policy, state, events, time.Now()), nil
}
