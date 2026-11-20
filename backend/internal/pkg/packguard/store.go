// Package packguard gates ModelArk dispatch on durable, expiring prepaid funds.
package packguard

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var (
	ErrExhausted = errors.New("no prepaid pack capacity can safely cover this request")
	ErrFrozen    = errors.New("prepaid funding is frozen pending reconciliation")
	ErrScope     = errors.New("upstream credentials or endpoint differ from the verified prepaid funding")
)

const UnitsPerToken int64 = 64
const ExpiryMargin = 10 * time.Minute

type Scope struct {
	ID, CredentialHash, APIBase string
	Frozen                      bool
}
type Plan struct {
	Model        string
	Units, Ratio int64
	Lifetime     time.Duration
	Async        bool
}
type Reservation struct {
	ID, ScopeID, Model string
	Units, Ratio       int64
}

type Ledger interface {
	Ready(context.Context) error
	Lookup(context.Context, int64, string, *url.URL) (*Scope, error)
	Reserve(context.Context, *Scope, Plan) (*Reservation, error)
	Attach(context.Context, string, string) error
	Uncertain(context.Context, string) error
	Finish(context.Context, string, int64, bool) error
	FinishTask(context.Context, string, string, int64, bool) error
}

type Store struct{ DB *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{DB: db} }
func HashCredential(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// Ready validates every required table/column before application startup. It
// returns no account data and does not cache mutable financial policy.
func (s *Store) Ready(ctx context.Context) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("prepaid funding database is required")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT b.account_id,s.name,s.enforced,s.safety_lock,s.reconcile_after,c.credential_hash,
 g.capacity_units,g.spent_units,g.reserved_units,g.expires_at,g.verified_at,
 r.reserved_units,r.ratio_units,r.state,r.valid_until,a.units
 FROM byteplus_pack_bindings b JOIN byteplus_pack_scopes s ON s.id=b.scope_id
 LEFT JOIN byteplus_pack_credentials c ON c.scope_id=s.id
 LEFT JOIN byteplus_pack_grants g ON g.scope_id=s.id
 LEFT JOIN byteplus_pack_reservations r ON r.scope_id=s.id
 LEFT JOIN byteplus_pack_allocations a ON a.reservation_id=r.id WHERE FALSE`)
	if err != nil {
		return err
	}
	_ = rows.Close()
	return rows.Err()
}

func (s *Store) Lookup(ctx context.Context, accountID int64, hash string, target *url.URL) (*Scope, error) {
	var scope Scope
	var matches bool
	var binding sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT s.id,$2,s.api_base,s.frozen,
 EXISTS(SELECT 1 FROM byteplus_pack_credentials c WHERE c.scope_id=s.id AND c.credential_hash=$2),
 (SELECT scope_id FROM byteplus_pack_bindings WHERE account_id=$1)
 FROM byteplus_pack_scopes s WHERE s.enforced AND (
 s.id=(SELECT scope_id FROM byteplus_pack_bindings WHERE account_id=$1)
 OR s.id=(SELECT scope_id FROM byteplus_pack_credentials WHERE credential_hash=$2))
 ORDER BY CASE WHEN s.id=(SELECT scope_id FROM byteplus_pack_bindings WHERE account_id=$1) THEN 0 ELSE 1 END LIMIT 1`, accountID, hash).
		Scan(&scope.ID, &scope.CredentialHash, &scope.APIBase, &scope.Frozen, &matches, &binding)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	canonical, err := NormalizeBase(scope.APIBase)
	if err != nil {
		return nil, ErrScope
	}
	scope.APIBase = canonical
	base, err := url.Parse(scope.APIBase)
	if err != nil || !matches || target.Scheme != "https" || target.User != nil || !strings.EqualFold(base.Hostname(), target.Hostname()) || (target.Port() != "" && target.Port() != "443") || !strings.HasPrefix(target.Path, strings.TrimRight(base.Path, "/")+"/") {
		return nil, ErrScope
	}
	if accountID > 0 && (!binding.Valid || binding.String != scope.ID) {
		if _, err = s.DB.ExecContext(ctx, `INSERT INTO byteplus_pack_bindings(account_id,scope_id) VALUES($1,$2)
 ON CONFLICT(account_id) DO UPDATE SET scope_id=EXCLUDED.scope_id
 WHERE byteplus_pack_bindings.scope_id<>EXCLUDED.scope_id AND NOT EXISTS
 (SELECT 1 FROM byteplus_pack_scopes old WHERE old.id=byteplus_pack_bindings.scope_id AND old.enforced)`, accountID, scope.ID); err != nil {
			return nil, err
		}
	}
	return &scope, nil
}

func (s *Store) Reserve(ctx context.Context, scope *Scope, plan Plan) (*Reservation, error) {
	if plan.Units <= 0 || plan.Ratio <= 0 || plan.Lifetime <= 0 {
		return nil, ErrExhausted
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(nonce)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var frozen, enforced bool
	var now time.Time
	if err = tx.QueryRowContext(ctx, `SELECT frozen,enforced,clock_timestamp() FROM byteplus_pack_scopes WHERE id=$1 FOR UPDATE`, scope.ID).Scan(&frozen, &enforced, &now); err != nil {
		return nil, err
	}
	if frozen || !enforced {
		return nil, ErrFrozen
	}
	deadline := now.Add(plan.Lifetime + ExpiryMargin)
	rows, err := tx.QueryContext(ctx, `SELECT id,capacity_units-spent_units-reserved_units FROM byteplus_pack_grants
        WHERE scope_id=$1 AND model=$2 AND expires_at >= $3
        AND capacity_units>spent_units+reserved_units ORDER BY expires_at,id FOR UPDATE`, scope.ID, plan.Model, deadline)
	if err != nil {
		return nil, err
	}
	type allocation struct {
		id    string
		units int64
	}
	var allocations []allocation
	remaining := plan.Units
	for rows.Next() {
		var a allocation
		if err = rows.Scan(&a.id, &a.units); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if remaining > 0 {
			a.units = min(a.units, remaining)
			allocations = append(allocations, a)
			remaining -= a.units
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if remaining > 0 {
		return nil, ErrExhausted
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO byteplus_pack_reservations(id,scope_id,model,ratio_units,reserved_units,valid_until)
        VALUES($1,$2,$3,$4,$5,$6)`, id, scope.ID, plan.Model, plan.Ratio, plan.Units, deadline); err != nil {
		return nil, err
	}
	for _, a := range allocations {
		if _, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_grants SET reserved_units=reserved_units+$1 WHERE id=$2`, a.units, a.id); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO byteplus_pack_allocations(reservation_id,grant_id,units) VALUES($1,$2,$3)`, id, a.id, a.units); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &Reservation{ID: id, ScopeID: scope.ID, Model: plan.Model, Units: plan.Units, Ratio: plan.Ratio}, nil
}

func (s *Store) Attach(ctx context.Context, id, task string) error {
	if task == "" || len(task) > 256 {
		return ErrScope
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE byteplus_pack_reservations SET task_id=$1,state='pending'
        WHERE id=$2 AND state IN ('reserved','uncertain','pending') AND (task_id IS NULL OR task_id=$1)`, task, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrScope
	}
	return err
}
func (s *Store) Uncertain(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE byteplus_pack_reservations SET state='uncertain' WHERE id=$1 AND state IN ('reserved','pending','uncertain')`, id)
	return err
}

func (s *Store) FinishTask(ctx context.Context, scope, task string, tokens int64, success bool) error {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM byteplus_pack_reservations WHERE scope_id=$1 AND task_id=$2`, scope, task).Scan(&id)
	// Read-only access to historical tasks cannot create funding or refund holds.
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.Finish(ctx, id, tokens, success)
}

func (s *Store) Finish(ctx context.Context, id string, tokens int64, success bool) error {
	var scopeID string
	if err := s.DB.QueryRowContext(ctx, `SELECT scope_id FROM byteplus_pack_reservations WHERE id=$1`, id).Scan(&scopeID); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var ignored bool
	if err = tx.QueryRowContext(ctx, `SELECT frozen FROM byteplus_pack_scopes WHERE id=$1 FOR UPDATE`, scopeID).Scan(&ignored); err != nil {
		return err
	}
	var ratio, reserved int64
	var state string
	if err = tx.QueryRowContext(ctx, `SELECT state,ratio_units,reserved_units FROM byteplus_pack_reservations WHERE id=$1 FOR UPDATE`, id).Scan(&state, &ratio, &reserved); err != nil {
		return err
	}
	if state == "succeeded" || state == "failed" {
		return tx.Commit()
	}
	actual := int64(0)
	if success {
		// Missing usage is ambiguous; retain every allocated unit.
		if tokens <= 0 {
			return ErrFrozen
		}
		if tokens > reserved/ratio {
			_, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_scopes SET frozen=TRUE,safety_lock=TRUE,frozen_reason='provider usage exceeded the reserved ceiling' WHERE id=$1`, scopeID)
			if err != nil {
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			return ErrFrozen
		}
		actual = tokens * ratio
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.grant_id,a.units FROM byteplus_pack_allocations a
        JOIN byteplus_pack_grants g ON g.id=a.grant_id WHERE a.reservation_id=$1 ORDER BY g.expires_at,g.id FOR UPDATE OF g`, id)
	if err != nil {
		return err
	}
	type allocation struct {
		id    string
		units int64
	}
	var allocations []allocation
	for rows.Next() {
		var a allocation
		if err = rows.Scan(&a.id, &a.units); err != nil {
			_ = rows.Close()
			return err
		}
		allocations = append(allocations, a)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	remaining := actual
	for _, a := range allocations {
		spent := min(remaining, a.units)
		remaining -= spent
		if _, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_grants SET reserved_units=reserved_units-$1,spent_units=spent_units+$2 WHERE id=$3`, a.units, spent, a.id); err != nil {
			return err
		}
	}
	if remaining != 0 {
		return fmt.Errorf("prepaid allocation invariant violated")
	}
	state = "failed"
	if success {
		state = "succeeded"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_reservations SET state=$1,actual_units=$2 WHERE id=$3`, state, actual, id); err != nil {
		return err
	}
	return tx.Commit()
}
