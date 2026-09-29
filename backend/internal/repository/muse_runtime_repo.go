package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
)

type museRuntimeRepository struct{ db *sql.DB }

func NewMuseRuntimeRepository(db *sql.DB) muse.RuntimeStore {
	return &museRuntimeRepository{db: db}
}

var _ muse.RuntimeStore = (*museRuntimeRepository)(nil)

func (r *museRuntimeRepository) Bind(ctx context.Context, identity muse.Identity) (*muse.Workspace, error) {
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var accountValid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM accounts WHERE id=$1
		AND platform='muse' AND status='active' AND deleted_at IS NULL AND updated_at=$2)`, identity.AccountID, identity.AccountUpdatedAt).Scan(&accountValid)
	if err != nil {
		return nil, err
	}
	if !accountValid {
		return nil, muse.ErrGeneration
	}
	w := &muse.Workspace{}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO muse_workspace_bindings (principal_id, remote_workspace_id, owner_user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (principal_id, remote_workspace_id) DO UPDATE
		SET principal_id = EXCLUDED.principal_id
		WHERE muse_workspace_bindings.owner_user_id = EXCLUDED.owner_user_id
		RETURNING id, identity_generation, lease_fence`, identity.PrincipalID, identity.WorkspaceID, identity.OwnerUserID).
		Scan(&w.ID, &w.Generation, &w.Fence)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, muse.ErrOwner
	}
	if err != nil {
		return nil, err
	}
	var idle bool
	err = tx.QueryRowContext(ctx, `SELECT active_turn_id IS NULL FROM muse_workspace_bindings WHERE id=$1`, w.ID).Scan(&idle)
	if err != nil {
		return nil, err
	}
	if !idle {
		return nil, muse.ErrBusy
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO muse_account_workspace_bindings(account_id,workspace_id,verified_account_updated_at)
		VALUES($1,$2,$3) ON CONFLICT(account_id) DO UPDATE SET verified_account_updated_at=EXCLUDED.verified_account_updated_at
		WHERE muse_account_workspace_bindings.workspace_id=EXCLUDED.workspace_id`, identity.AccountID, w.ID, identity.AccountUpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := museOneRow(result, muse.ErrGeneration); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	w.Identity = identity
	return w, nil
}

func (r *museRuntimeRepository) Reserve(ctx context.Context, input muse.Reservation) (*muse.Turn, muse.Lease, error) {
	if err := input.Validate(); err != nil {
		return nil, muse.Lease{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, muse.Lease{}, err
	}
	defer func() { _ = tx.Rollback() }()
	w, err := lockMuseWorkspace(ctx, tx, input.WorkspaceID)
	if err != nil {
		return nil, muse.Lease{}, err
	}
	if w.Identity.OwnerUserID != input.Actor.UserID {
		return nil, muse.Lease{}, muse.ErrOwner
	}
	if w.Generation != input.Generation {
		return nil, muse.Lease{}, muse.ErrGeneration
	}
	admissible, err := museAdmissionValid(ctx, tx, input.Actor, input.AccountID, w.ID, input.AccountUpdatedAt, input.ProxyUpdatedAt)
	if err != nil {
		return nil, muse.Lease{}, err
	}
	if !admissible {
		return nil, muse.Lease{}, muse.ErrOwner
	}
	// Keep billing failures from admitting an unbounded stream of uncharged
	// requests after remote completion. This check shares the workspace row lock.
	var unsettled bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM muse_turns WHERE workspace_id=$1 AND billing_command IS NOT NULL AND settled_at IS NULL AND state IN ('completed','failed','cancelled','rejected'))`, w.ID).Scan(&unsettled)
	if err != nil {
		return nil, muse.Lease{}, err
	}
	if unsettled {
		return nil, muse.Lease{}, muse.ErrBusy
	}
	// Expired leases remain occupied until the original work is reconciled.
	if w.ActiveTurnID != "" {
		return nil, muse.Lease{}, muse.ErrBusy
	}
	pricing, err := json.Marshal(input.Pricing)
	if err != nil {
		return nil, muse.Lease{}, err
	}
	turn, err := scanMuseTurn(tx.QueryRowContext(ctx, `
		INSERT INTO muse_turns (id, workspace_id, identity_generation, owner_user_id, api_key_id, account_id, pricing_snapshot, account_updated_at, proxy_updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9) RETURNING `+museTurnColumns,
		input.TurnID, w.ID, w.Generation, input.Actor.UserID, input.Actor.APIKeyID, input.AccountID, string(pricing), input.AccountUpdatedAt, input.ProxyUpdatedAt))
	if err != nil {
		return nil, muse.Lease{}, err
	}
	lease := muse.Lease{WorkspaceID: w.ID, TurnID: turn.ID, Owner: input.LeaseOwner, Fence: w.Fence + 1}
	_, err = tx.ExecContext(ctx, `UPDATE muse_workspace_bindings SET active_turn_id=$2,
		lease_owner=$3, lease_fence=$4, lease_until=clock_timestamp()+($5 * INTERVAL '1 second'), updated_at=NOW()
		WHERE id=$1`, w.ID, turn.ID, lease.Owner, lease.Fence, input.LeaseDuration.Seconds())
	if err != nil {
		return nil, muse.Lease{}, err
	}
	if err := tx.Commit(); err != nil {
		return nil, muse.Lease{}, err
	}
	return turn, lease, nil
}

func (r *museRuntimeRepository) Advance(ctx context.Context, lease muse.Lease, from, to muse.State, providerID string) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	if !muse.CanTransition(from, to) || len(providerID) > 256 {
		return muse.ErrTransition
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := verifyMuseLease(ctx, tx, lease); err != nil {
		return err
	}
	if to == muse.Submitting {
		turn, err := scanMuseTurn(tx.QueryRowContext(ctx, `SELECT `+museTurnColumns+` FROM muse_turns WHERE id=$1`, lease.TurnID))
		if err != nil {
			return err
		}
		valid, err := museAdmissionValid(ctx, tx, turn.Actor, turn.AccountID, turn.WorkspaceID, turn.AccountUpdatedAt, turn.ProxyUpdatedAt)
		if err != nil {
			return err
		}
		if !valid {
			return muse.ErrGeneration
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE muse_turns SET state=$3,
		provider_turn_id=CASE WHEN $4='' THEN provider_turn_id ELSE $4 END, updated_at=NOW()
		WHERE id=$1 AND workspace_id=$2 AND state=$5
		AND (provider_turn_id='' OR $4='' OR provider_turn_id=$4)`, lease.TurnID, lease.WorkspaceID, to, providerID, from)
	if err != nil {
		return err
	}
	if err := museOneRow(result, muse.ErrTransition); err != nil {
		return err
	}
	if to.Terminal() {
		_, err = tx.ExecContext(ctx, `UPDATE muse_workspace_bindings SET active_turn_id=NULL,
			lease_owner=NULL, lease_until=NULL, updated_at=NOW() WHERE id=$1`, lease.WorkspaceID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *museRuntimeRepository) Renew(ctx context.Context, lease muse.Lease, duration time.Duration) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	if err := muse.ValidateLease(duration); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE muse_workspace_bindings
		SET lease_until=clock_timestamp()+($5 * INTERVAL '1 second'), updated_at=NOW()
		WHERE id=$1 AND active_turn_id=$2 AND lease_owner=$3 AND lease_fence=$4 AND lease_until>clock_timestamp()`,
		lease.WorkspaceID, lease.TurnID, lease.Owner, lease.Fence, duration.Seconds())
	if err != nil {
		return err
	}
	return museOneRow(result, muse.ErrLease)
}

// ClaimRecovery acquires the same operation; it never creates or resubmits work.
func (r *museRuntimeRepository) ClaimRecovery(ctx context.Context, workspaceID int64, owner string, duration time.Duration) (*muse.Turn, muse.Lease, error) {
	if workspaceID <= 0 || (muse.Lease{WorkspaceID: workspaceID, TurnID: "recovery", Owner: owner, Fence: 1}).Validate() != nil {
		return nil, muse.Lease{}, muse.ErrInvalid
	}
	if err := muse.ValidateLease(duration); err != nil {
		return nil, muse.Lease{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, muse.Lease{}, err
	}
	defer func() { _ = tx.Rollback() }()
	w, err := lockMuseWorkspace(ctx, tx, workspaceID)
	if err != nil {
		return nil, muse.Lease{}, err
	}
	var expired bool
	err = tx.QueryRowContext(ctx, `SELECT lease_until <= clock_timestamp() FROM muse_workspace_bindings WHERE id=$1`, workspaceID).Scan(&expired)
	if w.ActiveTurnID == "" {
		return nil, muse.Lease{}, muse.ErrNotFound
	}
	if err != nil {
		return nil, muse.Lease{}, err
	}
	if !expired {
		return nil, muse.Lease{}, muse.ErrBusy
	}
	turn, err := scanMuseTurn(tx.QueryRowContext(ctx, `SELECT `+museTurnColumns+` FROM muse_turns WHERE id=$1`, w.ActiveTurnID))
	if err != nil {
		return nil, muse.Lease{}, err
	}
	if turn.State == muse.OwnerReview {
		return nil, muse.Lease{}, muse.ErrOwnerReview
	}
	if turn.State != muse.Reserved && turn.State != muse.OwnerReview {
		_, err = tx.ExecContext(ctx, `UPDATE muse_turns SET state='ambiguous', updated_at=NOW() WHERE id=$1`, turn.ID)
		if err != nil {
			return nil, muse.Lease{}, err
		}
		turn.State = muse.Ambiguous
	}
	lease := muse.Lease{WorkspaceID: w.ID, TurnID: turn.ID, Owner: owner, Fence: w.Fence + 1}
	_, err = tx.ExecContext(ctx, `UPDATE muse_workspace_bindings SET lease_owner=$2, lease_fence=$3,
		lease_until=clock_timestamp()+($4 * INTERVAL '1 second'), updated_at=NOW() WHERE id=$1`, w.ID, owner, lease.Fence, duration.Seconds())
	if err != nil {
		return nil, muse.Lease{}, err
	}
	if err := tx.Commit(); err != nil {
		return nil, muse.Lease{}, err
	}
	return turn, lease, nil
}

func (r *museRuntimeRepository) GetTurn(ctx context.Context, id string, actor muse.Actor) (*muse.Turn, error) {
	if actor.UserID <= 0 || actor.APIKeyID <= 0 {
		return nil, muse.ErrInvalid
	}
	return scanMuseTurn(r.db.QueryRowContext(ctx, `SELECT `+museTurnColumns+` FROM muse_turns WHERE id=$1 AND owner_user_id=$2 AND api_key_id=$3`, id, actor.UserID, actor.APIKeyID))
}

func lockMuseWorkspace(ctx context.Context, tx *sql.Tx, id int64) (*muse.Workspace, error) {
	w := &muse.Workspace{}
	var turn, owner sql.NullString
	var until sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT id, principal_id, remote_workspace_id, owner_user_id,
		identity_generation, lease_fence, active_turn_id, lease_owner, lease_until
		FROM muse_workspace_bindings WHERE id=$1 FOR UPDATE`, id).
		Scan(&w.ID, &w.Identity.PrincipalID, &w.Identity.WorkspaceID, &w.Identity.OwnerUserID, &w.Generation, &w.Fence, &turn, &owner, &until)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, muse.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	w.ActiveTurnID = turn.String
	w.LeaseOwner = owner.String
	w.LeaseUntil = until.Time
	return w, nil
}

func verifyMuseLease(ctx context.Context, tx *sql.Tx, lease muse.Lease) error {
	w, err := lockMuseWorkspace(ctx, tx, lease.WorkspaceID)
	if err != nil {
		return err
	}
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT lease_until>clock_timestamp() FROM muse_workspace_bindings WHERE id=$1`, w.ID).Scan(&valid)
	if err != nil {
		return muse.ErrLease
	}
	if !valid || w.ActiveTurnID != lease.TurnID || w.LeaseOwner != lease.Owner || w.Fence != lease.Fence {
		return muse.ErrLease
	}
	return nil
}

const museTurnColumns = `id, workspace_id, identity_generation, owner_user_id, api_key_id, account_id,
	state, provider_turn_id, pricing_snapshot, created_at, updated_at, account_updated_at, proxy_updated_at`

func scanMuseTurn(row interface{ Scan(...any) error }) (*muse.Turn, error) {
	t := &muse.Turn{}
	var pricing []byte
	var accountAt, proxyAt sql.NullTime
	err := row.Scan(&t.ID, &t.WorkspaceID, &t.Generation, &t.Actor.UserID, &t.Actor.APIKeyID, &t.AccountID,
		&t.State, &t.ProviderTurnID, &pricing, &t.CreatedAt, &t.UpdatedAt, &accountAt, &proxyAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, muse.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(pricing, &t.Pricing); err != nil {
		return nil, err
	}
	if accountAt.Valid {
		t.AccountUpdatedAt = accountAt.Time
	}
	if proxyAt.Valid {
		t.ProxyUpdatedAt = &proxyAt.Time
	}
	return t, nil
}

func museOneRow(result sql.Result, conflict error) error {
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return conflict
	}
	return nil
}

func museAdmissionValid(ctx context.Context, tx *sql.Tx, actor muse.Actor, accountID, workspaceID int64, accountAt time.Time, proxyAt *time.Time) (bool, error) {
	var valid bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM api_keys k JOIN users u ON u.id=k.user_id
		JOIN accounts a ON a.id=$3
 LEFT JOIN proxies px ON px.id=a.proxy_id
 JOIN muse_account_workspace_bindings b ON b.account_id=a.id AND b.workspace_id=$4
		WHERE k.id=$1 AND k.user_id=$2 AND k.status='active' AND k.deleted_at IS NULL
		AND (k.expires_at IS NULL OR k.expires_at>clock_timestamp())
		AND u.status='active' AND u.deleted_at IS NULL
		AND a.platform='muse' AND a.status='active' AND a.schedulable AND a.deleted_at IS NULL
		AND a.updated_at=b.verified_account_updated_at AND a.updated_at=$5
 AND ((a.proxy_id IS NULL AND $6::timestamptz IS NULL) OR (a.proxy_id IS NOT NULL AND px.deleted_at IS NULL AND px.updated_at=$6::timestamptz))
 )`, actor.APIKeyID, actor.UserID, accountID, workspaceID, accountAt, proxyAt).Scan(&valid)
	return valid, err
}
