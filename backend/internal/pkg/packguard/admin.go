package packguard

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalid       = errors.New("invalid prepaid funding input")
	ErrConflict      = errors.New("funding state changed or this provider resource is already credited")
	ErrSafetyLock    = errors.New("funding is safety-locked; provider reconciliation is required before resuming")
	ErrPauseRequired = errors.New("pause the funding pool before changing verified funding or linking credentials")
	ErrPendingJobs   = errors.New("unsettled jobs still hold funding; reconcile them before disabling pack-only enforcement")
	ErrRecheck       = errors.New("recheck each pack's remaining balance after leaving pack-only mode")
	ErrRegion        = errors.New("pack-only billing policies are currently qualified for the Johor/AP endpoint only")
)

const QualifiedAPIBase = "https://ark.ap-southeast.bytepluses.com/api/v3"

type Policy struct {
	Model                  string `json:"model"`
	Name                   string `json:"name"`
	MinimumValiditySeconds int64  `json:"minimum_validity_seconds"`
}

// Policies are qualified billing versions, not user-supplied price estimates.
// Future model versions require their own bounds and provider qualification.
func Policies() []Policy {
	return []Policy{{SeedanceModel, "Seedance 2.5 (2026-06-28)", int64((VideoLifetime + ExpiryMargin) / time.Second)},
		{SeedModel, "Seed 2.1 turbo (2026-06-28)", int64((5*time.Minute + ExpiryMargin) / time.Second)}}
}
func policyFor(model string) (Policy, bool) {
	for _, p := range Policies() {
		if p.Model == model {
			return p, true
		}
	}
	return Policy{}, false
}

func NormalizeBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || !isArkHost(u.Hostname()) || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") || strings.TrimRight(u.Path, "/") != "/api/v3" {
		return "", ErrInvalid
	}
	u.Host = strings.ToLower(u.Hostname())
	u.Path = "/api/v3"
	u.RawPath = ""
	return u.String(), nil
}

// Decimal strings avoid rounding a remaining provider balance upwards.
func ParseTokens(raw string) (int64, error) { return parseTokens(raw, false) }
func parseTokens(raw string, allowZero bool) (int64, error) {
	if !regexp.MustCompile(`^[0-9]+(?:\.[0-9]{1,6})?$`).MatchString(raw) || len(raw) > 24 {
		return 0, ErrInvalid
	}
	r, ok := new(big.Rat).SetString(raw)
	if !ok {
		return 0, ErrInvalid
	}
	r.Mul(r, big.NewRat(UnitsPerToken, 1))
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, ErrInvalid
	}
	u := r.Num().Int64()
	if u < 0 || (!allowZero && u == 0) || u > 1_000_000_000_000_000 {
		return 0, ErrInvalid
	}
	return u, nil
}
func FormatTokens(units int64) string {
	if units%64 == 0 {
		return fmt.Sprint(units / 64)
	}
	return strings.TrimRight(fmt.Sprintf("%d.%06d", units/64, (units%64)*15625), "0")
}

type Pool struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	APIBase        string     `json:"api_base"`
	Enforced       bool       `json:"enforced"`
	Frozen         bool       `json:"frozen"`
	SafetyLock     bool       `json:"safety_lock"`
	Reason         string     `json:"reason"`
	ReconcileAfter *time.Time `json:"reconcile_after"`
}
type ModelStatus struct {
	Model      string     `json:"model"`
	Spendable  string     `json:"spendable_tokens"`
	Held       string     `json:"held_tokens"`
	NextExpiry *time.Time `json:"next_expiry"`
}
type Grant struct {
	VerifiedAt        time.Time `json:"verified_at"`
	EligibilityReason string    `json:"eligibility_reason"`
	ID                string    `json:"id"`
	Model             string    `json:"model"`
	Capacity          string    `json:"capacity_tokens"`
	Spent             string    `json:"spent_tokens"`
	Reserved          string    `json:"reserved_tokens"`
	Available         string    `json:"available_tokens"`
	ExpiresAt         time.Time `json:"expires_at"`
	Eligible          bool      `json:"eligible"`
	NeedsVerification bool      `json:"needs_verification"`
}
type HeldJob struct {
	ID         string    `json:"id"`
	Model      string    `json:"model"`
	State      string    `json:"state"`
	TaskID     *string   `json:"task_id"`
	Reserved   string    `json:"reserved_tokens"`
	Ceiling    string    `json:"ceiling_tokens"`
	Actual     *string   `json:"actual_tokens"`
	CreatedAt  time.Time `json:"created_at"`
	ValidUntil time.Time `json:"valid_until"`
}
type Status struct {
	ObservedAt        time.Time     `json:"observed_at"`
	Models            []ModelStatus `json:"models"`
	Pool              *Pool         `json:"pool"`
	CredentialMatches bool          `json:"credential_matches"`
	Policies          []Policy      `json:"policies"`
	Grants            []Grant       `json:"grants"`
	Jobs              []HeldJob     `json:"jobs"`
	OpenJobs          int64         `json:"open_jobs"`
	BoundAccounts     int64         `json:"bound_accounts"`
	Credentials       int64         `json:"credentials"`
}

func (s *Store) Pools(ctx context.Context) ([]Pool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name,api_base,enforced,frozen,safety_lock,frozen_reason,reconcile_after FROM byteplus_pack_scopes ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Pool{}
	for rows.Next() {
		var p Pool
		if err = rows.Scan(&p.ID, &p.Name, &p.APIBase, &p.Enforced, &p.Frozen, &p.SafetyLock, &p.Reason, &p.ReconcileAfter); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Status(ctx context.Context, accountID int64, hash, base string) (*Status, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	st := &Status{Models: []ModelStatus{}, Policies: []Policy{}, Grants: []Grant{}, Jobs: []HeldJob{}}
	if base == QualifiedAPIBase {
		st.Policies = Policies()
	}
	if err = tx.QueryRowContext(ctx, `SELECT transaction_timestamp()`).Scan(&st.ObservedAt); err != nil {
		return nil, err
	}
	p := &Pool{}
	err = tx.QueryRowContext(ctx, `SELECT s.id,s.name,s.api_base,s.enforced,s.frozen,s.safety_lock,s.frozen_reason,s.reconcile_after
 FROM byteplus_pack_scopes s WHERE s.id=(SELECT scope_id FROM byteplus_pack_bindings WHERE account_id=$1)
 OR s.id=(SELECT scope_id FROM byteplus_pack_credentials WHERE credential_hash=$2)
 ORDER BY CASE WHEN s.enforced THEN 0 ELSE 1 END,CASE WHEN s.id=(SELECT scope_id FROM byteplus_pack_bindings WHERE account_id=$1) THEN 0 ELSE 1 END LIMIT 1`, accountID, hash).
		Scan(&p.ID, &p.Name, &p.APIBase, &p.Enforced, &p.Frozen, &p.SafetyLock, &p.Reason, &p.ReconcileAfter)
	if errors.Is(err, sql.ErrNoRows) {
		st.CredentialMatches = true
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	st.Pool = p
	if canonical, e := NormalizeBase(p.APIBase); e == nil {
		p.APIBase = canonical
	}
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM byteplus_pack_credentials WHERE scope_id=$1 AND credential_hash=$2),
 (SELECT count(*) FROM byteplus_pack_bindings WHERE scope_id=$1),
 (SELECT count(*) FROM byteplus_pack_credentials WHERE scope_id=$1),
 (SELECT count(*) FROM byteplus_pack_reservations WHERE scope_id=$1 AND state IN ('reserved','pending','uncertain'))`, p.ID, hash).
		Scan(&st.CredentialMatches, &st.BoundAccounts, &st.Credentials, &st.OpenJobs); err != nil {
		return nil, err
	}
	st.CredentialMatches = st.CredentialMatches && p.APIBase == base
	now := st.ObservedAt
	rows, err := tx.QueryContext(ctx, `SELECT id,model,capacity_units,spent_units,reserved_units,expires_at,verified_at FROM byteplus_pack_grants WHERE scope_id=$1 ORDER BY expires_at,id`, p.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g Grant
		var capacity, spent, reserved int64
		var verified time.Time
		if err = rows.Scan(&g.ID, &g.Model, &capacity, &spent, &reserved, &g.ExpiresAt, &verified); err != nil {
			_ = rows.Close()
			return nil, err
		}
		g.Capacity = FormatTokens(capacity)
		g.Spent = FormatTokens(spent)
		g.Reserved = FormatTokens(reserved)
		g.Available = FormatTokens(capacity - spent - reserved)
		policy, known := policyFor(g.Model)
		g.Eligible = p.APIBase == QualifiedAPIBase && known && capacity > spent+reserved && !g.ExpiresAt.Before(now.Add(time.Duration(policy.MinimumValiditySeconds)*time.Second))
		g.NeedsVerification = p.ReconcileAfter != nil && verified.Before(*p.ReconcileAfter) && g.ExpiresAt.After(now) && capacity > spent
		g.VerifiedAt = verified
		g.EligibilityReason = grantReason(st, g, known, now)
		st.Grants = append(st.Grants, g)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT id,model,state,task_id,reserved_units,actual_units,created_at,valid_until
 FROM byteplus_pack_reservations WHERE scope_id=$1 ORDER BY CASE WHEN state IN ('reserved','pending','uncertain') THEN 0 ELSE 1 END,created_at DESC LIMIT 100`, p.ID)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var j HeldJob
		var reserved int64
		var actual sql.NullInt64
		if err = rows.Scan(&j.ID, &j.Model, &j.State, &j.TaskID, &reserved, &actual, &j.CreatedAt, &j.ValidUntil); err != nil {
			return nil, err
		}
		j.Ceiling = FormatTokens(reserved)
		j.Reserved = j.Ceiling
		if j.State == "succeeded" || j.State == "failed" {
			j.Reserved = "0"
		}
		if actual.Valid {
			v := FormatTokens(actual.Int64)
			j.Actual = &v
		}
		st.Jobs = append(st.Jobs, j)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	st.Models = summarizeGrants(st)
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *Store) Enroll(ctx context.Context, accountID int64, hash, base, name, poolID string) error {
	if accountID <= 0 || len(hash) != 64 || len(name) > 100 {
		return ErrInvalid
	}
	canonical, err := NormalizeBase(base)
	if err != nil {
		return err
	}
	if canonical != QualifiedAPIBase {
		return ErrRegion
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var currentHash string
	if err = tx.QueryRowContext(ctx, `SELECT encode(sha256(convert_to(btrim(COALESCE(credentials->>'api_key','')),'UTF8')),'hex') FROM accounts WHERE id=$1 FOR UPDATE`, accountID).Scan(&currentHash); err != nil {
		return err
	}
	if currentHash != hash {
		return ErrScope
	}
	// Serialize first enrollment of a provider key. The unique alias also protects replicas.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, hash); err != nil {
		return err
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT scope_id FROM byteplus_pack_credentials WHERE credential_hash=$1`, hash).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var priorBinding string
	err = tx.QueryRowContext(ctx, `SELECT scope_id FROM byteplus_pack_bindings WHERE account_id=$1`, accountID).Scan(&priorBinding)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if poolID == "" {
		poolID = existing
	}
	if poolID == "" {
		b := make([]byte, 16)
		if _, err = rand.Read(b); err != nil {
			return err
		}
		poolID = hex.EncodeToString(b)
		if strings.TrimSpace(name) == "" {
			name = "BytePlus prepaid funding"
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO byteplus_pack_scopes(id,name,api_base,frozen,frozen_reason) VALUES($1,$2,$3,TRUE,'Initial setup: verify funding and isolate provider consumption')`, poolID, name, canonical); err != nil {
			return err
		}
	}
	// Lock multiple pools in a stable order before merging a mistaken empty pool.
	locks, err := tx.QueryContext(ctx, `SELECT id FROM byteplus_pack_scopes WHERE id=$1 OR id=$2 OR id=$3 ORDER BY id FOR UPDATE`, poolID, existing, priorBinding)
	if err != nil {
		return err
	}
	for locks.Next() {
		var locked string
		if err = locks.Scan(&locked); err != nil {
			_ = locks.Close()
			return err
		}
	}
	err = locks.Err()
	_ = locks.Close()
	if err != nil {
		return err
	}
	var actualBase string
	var enforced, safety, frozen bool
	if err = tx.QueryRowContext(ctx, `SELECT api_base,enforced,safety_lock,frozen FROM byteplus_pack_scopes WHERE id=$1`, poolID).Scan(&actualBase, &enforced, &safety, &frozen); err != nil {
		return err
	}
	if actualBase != canonical {
		normalized, e := NormalizeBase(actualBase)
		if e != nil || normalized != canonical {
			return ErrScope
		}
	}
	if safety {
		return ErrSafetyLock
	}
	if existing != poolID && !frozen {
		return ErrPauseRequired
	}
	if existing != "" && existing != poolID {
		var empty bool
		if err = tx.QueryRowContext(ctx, `SELECT frozen AND NOT EXISTS(SELECT 1 FROM byteplus_pack_grants WHERE scope_id=$1) AND NOT EXISTS(SELECT 1 FROM byteplus_pack_reservations WHERE scope_id=$1) FROM byteplus_pack_scopes WHERE id=$1`, existing).Scan(&empty); err != nil {
			return err
		}
		if !empty {
			return ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_credentials SET scope_id=$1 WHERE scope_id=$2`, poolID, existing); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_bindings SET scope_id=$1 WHERE scope_id=$2`, poolID, existing); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_scopes SET enforced=FALSE,frozen=TRUE,frozen_reason='Empty unfunded pool merged into another pool' WHERE id=$1`, existing); err != nil {
			return err
		}
	}
	var bound string
	err = tx.QueryRowContext(ctx, `SELECT scope_id FROM byteplus_pack_bindings WHERE account_id=$1`, accountID).Scan(&bound)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if bound != "" && bound != poolID {
		var canMove bool
		if err = tx.QueryRowContext(ctx, `SELECT frozen AND NOT EXISTS(SELECT 1 FROM byteplus_pack_reservations WHERE scope_id=$1 AND state IN ('reserved','pending','uncertain')) FROM byteplus_pack_scopes WHERE id=$1`, bound).Scan(&canMove); err != nil {
			return err
		}
		if !canMove {
			return ErrPauseRequired
		}
		if _, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_bindings SET scope_id=$1 WHERE account_id=$2`, poolID, accountID); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO byteplus_pack_credentials(credential_hash,scope_id) VALUES($1,$2) ON CONFLICT(credential_hash) DO NOTHING`, hash, poolID); err != nil {
		return err
	}
	if !enforced {
		if _, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_scopes SET enforced=TRUE,frozen=TRUE,frozen_reason='Re-enabled: verify remaining funding and isolate provider consumption' WHERE id=$1`, poolID); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO byteplus_pack_bindings(account_id,scope_id)
 SELECT id,$1 FROM accounts WHERE encode(sha256(convert_to(btrim(COALESCE(credentials->>'api_key','')),'UTF8')),'hex')=$2
 ON CONFLICT(account_id) DO UPDATE SET scope_id=EXCLUDED.scope_id
 WHERE byteplus_pack_bindings.scope_id=EXCLUDED.scope_id OR NOT EXISTS
 (SELECT 1 FROM byteplus_pack_scopes old WHERE old.id=byteplus_pack_bindings.scope_id AND old.enforced)`, poolID, hash); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AddGrant(ctx context.Context, poolID, id, model, tokens string, expiry time.Time) error {
	id = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(id), "ModelArk_resource_packages"))
	if !regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,160}$`).MatchString(id) {
		return ErrInvalid
	}
	if _, ok := policyFor(model); !ok {
		return ErrInvalid
	}
	units, err := ParseTokens(tokens)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var frozen, enforced bool
	var now time.Time
	if err = tx.QueryRowContext(ctx, `SELECT frozen,enforced,clock_timestamp() FROM byteplus_pack_scopes WHERE id=$1 FOR UPDATE`, poolID).Scan(&frozen, &enforced, &now); err != nil {
		return err
	}
	if !frozen || !enforced {
		return ErrPauseRequired
	}
	if !expiry.After(now) {
		return ErrInvalid
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO byteplus_pack_grants(id,scope_id,model,capacity_units,expires_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(id) DO NOTHING`, id, poolID, model, units, expiry)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return tx.Commit()
}

// VerifyGrant never resets spent usage. After consumption, only a reduction or
// shorter expiry is accepted, so a delayed provider counter cannot refill funds.
func (s *Store) VerifyGrant(ctx context.Context, poolID, id, tokens string, expiry time.Time) error {
	units, err := parseTokens(tokens, true)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var frozen bool
	var after *time.Time
	if err = tx.QueryRowContext(ctx, `SELECT frozen,reconcile_after FROM byteplus_pack_scopes WHERE id=$1 FOR UPDATE`, poolID).Scan(&frozen, &after); err != nil {
		return err
	}
	if !frozen {
		return ErrPauseRequired
	}
	var pending int64
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM byteplus_pack_reservations WHERE scope_id=$1 AND state IN ('reserved','pending','uncertain')`, poolID).Scan(&pending); err != nil {
		return err
	}
	if pending != 0 {
		return ErrPendingJobs
	}
	var capacity, spent, reserved int64
	var oldExpiry time.Time
	if err = tx.QueryRowContext(ctx, `SELECT capacity_units,spent_units,reserved_units,expires_at FROM byteplus_pack_grants WHERE id=$1 AND scope_id=$2 FOR UPDATE`, id, poolID).Scan(&capacity, &spent, &reserved, &oldExpiry); err != nil {
		return err
	}
	if reserved != 0 {
		return ErrConflict
	}
	// Correct an unused initial entry before the first dispatch, but never refill
	// a consumed pack or an existing balance after an untracked PAYG period.
	var used bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM byteplus_pack_allocations WHERE grant_id=$1)`, id).Scan(&used); err != nil {
		return err
	}
	if (used || after != nil) && (units > capacity-spent || expiry.After(oldExpiry)) {
		return ErrInvalid
	}
	if expiry.IsZero() {
		return ErrInvalid
	}
	if _, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_grants SET capacity_units=$1,expires_at=$2,verified_at=clock_timestamp() WHERE id=$3 AND scope_id=$4`, spent+units, expiry, id, poolID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetState(ctx context.Context, poolID, action string, confirmed bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var enforced, safety bool
	if err = tx.QueryRowContext(ctx, `SELECT enforced,safety_lock FROM byteplus_pack_scopes WHERE id=$1 FOR UPDATE`, poolID).Scan(&enforced, &safety); err != nil {
		return err
	}
	switch action {
	case "freeze":
		_, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_scopes SET frozen=TRUE,frozen_reason=CASE WHEN safety_lock THEN frozen_reason ELSE 'Paused by administrator' END WHERE id=$1`, poolID)
	case "resume":
		if !confirmed || !enforced {
			return ErrInvalid
		}
		if safety {
			return ErrSafetyLock
		}
		var stale bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM byteplus_pack_grants g JOIN byteplus_pack_scopes s ON s.id=g.scope_id WHERE s.id=$1 AND s.reconcile_after IS NOT NULL AND g.verified_at<s.reconcile_after AND g.expires_at>clock_timestamp() AND g.capacity_units>g.spent_units)`, poolID).Scan(&stale); err != nil {
			return err
		}
		if stale {
			return ErrRecheck
		}
		var eligible bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM byteplus_pack_grants WHERE scope_id=$1 AND capacity_units>spent_units+reserved_units AND
 ((model=$2 AND expires_at>=clock_timestamp()+INTERVAL '70 minutes') OR (model=$3 AND expires_at>=clock_timestamp()+INTERVAL '15 minutes')))`, poolID, SeedanceModel, SeedModel).Scan(&eligible); err != nil {
			return err
		}
		if !eligible {
			return ErrExhausted
		}
		_, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_scopes SET frozen=FALSE,frozen_reason='',reconcile_after=NULL WHERE id=$1`, poolID)
	case "disable":
		if !confirmed {
			return ErrInvalid
		}
		if safety {
			return ErrSafetyLock
		}
		var pending int64
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM byteplus_pack_reservations WHERE scope_id=$1 AND state IN ('reserved','pending','uncertain')`, poolID).Scan(&pending); err != nil {
			return err
		}
		if pending != 0 {
			return ErrPendingJobs
		}
		_, err = tx.ExecContext(ctx, `UPDATE byteplus_pack_scopes SET enforced=FALSE,frozen=TRUE,reconcile_after=clock_timestamp(),frozen_reason='Pack-only disabled: administrator explicitly allowed pay-as-you-go' WHERE id=$1`, poolID)
	default:
		return ErrInvalid
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Task(ctx context.Context, poolID, id string) (string, error) {
	var task sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT task_id FROM byteplus_pack_reservations WHERE id=$1 AND scope_id=$2`, id, poolID).Scan(&task); err != nil {
		return "", err
	}
	if !task.Valid || !regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`).MatchString(task.String) {
		return "", ErrInvalid
	}
	return task.String, nil
}

// Reasons describe local admission funding, not provider account credit.
func grantReason(st *Status, g Grant, known bool, now time.Time) string {
	switch {
	case st.Pool.APIBase != QualifiedAPIBase || !known:
		return "unsupported"
	case !st.CredentialMatches:
		return "credential_mismatch"
	case !g.ExpiresAt.After(now):
		return "expired"
	case g.Available == "0":
		return "exhausted"
	case g.NeedsVerification:
		return "verification_required"
	case !st.Pool.Enforced:
		return "enforcement_off"
	case st.Pool.SafetyLock:
		return "safety_locked"
	case st.Pool.Frozen:
		return "paused"
	case !g.Eligible:
		return "too_close_to_expiry"
	default:
		return "eligible"
	}
}
func summarizeGrants(st *Status) []ModelStatus {
	out := []ModelStatus{}
	for _, p := range st.Policies {
		available, held := new(big.Int), new(big.Int)
		var expiry *time.Time
		for _, g := range st.Grants {
			if g.Model != p.Model {
				continue
			}
			n, _ := parseTokens(g.Reserved, true)
			held.Add(held, big.NewInt(n))
			if g.EligibilityReason != "eligible" {
				continue
			}
			n, _ = parseTokens(g.Available, true)
			available.Add(available, big.NewInt(n))
			if expiry == nil || g.ExpiresAt.Before(*expiry) {
				t := g.ExpiresAt
				expiry = &t
			}
		}
		out = append(out, ModelStatus{Model: p.Model, Spendable: formatBigUnits(available), Held: formatBigUnits(held), NextExpiry: expiry})
	}
	return out
}

func formatBigUnits(units *big.Int) string {
	text := new(big.Rat).SetFrac(units, big.NewInt(UnitsPerToken)).FloatString(6)
	return strings.TrimRight(strings.TrimRight(text, "0"), ".")
}
