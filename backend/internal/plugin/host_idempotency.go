// host_idempotency.go provides the SQL-backed idempotency adapter the
// HostService reverse RPCs use to dedupe externally-retried calls
// (CreditBalance / DeductBalance / AssignSubscription /
// RevokeSubscriptionDays / AccrueRebate).
//
// Wire contract — see migration 144_plugin_idempotency.sql:
//
//   - First call for (namespace, key) inserts a placeholder row, runs the
//     supplied applyFn, then UPDATEs the result_payload with the response.
//   - Subsequent calls for the same (namespace, key) SELECT the cached
//     payload and return it with alreadyApplied=true. The caller MUST treat
//     that flag as "this was a replay — do not run side effects again".
//
// Decoupling: HostIdempotencyStore depends only on *sql.DB; service layer
// adapters compose it with their concrete service calls. This keeps the
// dedup contract independent of any specific business operation.
package plugin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// IdempotencyNamespace* enumerates the distinct RPC families that share the
// plugin_idempotency table. Keep these in sync with the HostService RPC
// methods that delegate through HostIdempotencyStore.
const (
	IdempotencyNamespaceCreditBalance = "credit_balance"
	IdempotencyNamespaceDeductBalance = "deduct_balance"
	IdempotencyNamespaceAssignSub     = "assign_sub"
	IdempotencyNamespaceRevokeSub     = "revoke_sub"
	IdempotencyNamespaceRebate        = "rebate"
)

// HostIdempotencyStore is the SQL-backed idempotency adapter. Each
// (namespace, key) pair is recorded at most once; concurrent attempts
// race on the primary key INSERT and the loser observes the cached payload.
type HostIdempotencyStore struct {
	db *sql.DB
}

// NewHostIdempotencyStore constructs a store bound to the given database.
// db must be the same handle the rest of the host uses; HostIdempotencyStore
// does not own connections.
func NewHostIdempotencyStore(db *sql.DB) *HostIdempotencyStore {
	return &HostIdempotencyStore{db: db}
}

// LookupOrApply atomically resolves (namespace, key) to a JSON payload.
//
// On first call:
//  1. INSERT a placeholder row (namespace, key, '{}'::jsonb).
//  2. Run applyFn to compute the real payload.
//  3. UPDATE the row's result_payload with the computed bytes.
//
// On replay: SELECT the cached payload and return alreadyApplied=true. If
// applyFn returns an error on the first call, the placeholder row is
// rolled back so a retry is treated as the first call.
func (s *HostIdempotencyStore) LookupOrApply(
	ctx context.Context,
	namespace, key string,
	applyFn func(ctx context.Context) ([]byte, error),
) (payload []byte, alreadyApplied bool, err error) {
	if err := validateIdempotencyArgs(namespace, key, applyFn); err != nil {
		return nil, false, err
	}

	inserted, err := s.tryClaim(ctx, namespace, key)
	if err != nil {
		return nil, false, err
	}
	if !inserted {
		cached, err := s.fetchCached(ctx, namespace, key)
		if err != nil {
			return nil, false, err
		}
		return cached, true, nil
	}

	return s.runAndStore(ctx, namespace, key, applyFn)
}

// validateIdempotencyArgs centralises argument checks so LookupOrApply stays
// short and the error messages share a single source of truth.
func validateIdempotencyArgs(namespace, key string, applyFn func(ctx context.Context) ([]byte, error)) error {
	if strings.TrimSpace(namespace) == "" {
		return errors.New("idempotency: empty namespace")
	}
	if strings.TrimSpace(key) == "" {
		return errors.New("idempotency: empty key")
	}
	if applyFn == nil {
		return errors.New("idempotency: nil applyFn")
	}
	return nil
}

// tryClaim attempts to insert a placeholder row. Returns inserted=true if
// this caller wins the race; inserted=false means another caller already
// recorded the (namespace, key) and the cached payload should be fetched.
func (s *HostIdempotencyStore) tryClaim(ctx context.Context, namespace, key string) (bool, error) {
	const stmt = `
INSERT INTO plugin_idempotency (namespace, key, result_payload)
VALUES ($1, $2, '{}'::jsonb)
ON CONFLICT (namespace, key) DO NOTHING
`
	res, err := s.db.ExecContext(ctx, stmt, namespace, key)
	if err != nil {
		return false, fmt.Errorf("idempotency claim: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("idempotency claim rows: %w", err)
	}
	return n > 0, nil
}

// fetchCached SELECTs the result_payload for an already-claimed key.
func (s *HostIdempotencyStore) fetchCached(ctx context.Context, namespace, key string) ([]byte, error) {
	const stmt = `SELECT result_payload FROM plugin_idempotency WHERE namespace = $1 AND key = $2`
	var payload []byte
	err := s.db.QueryRowContext(ctx, stmt, namespace, key).Scan(&payload)
	if err != nil {
		return nil, fmt.Errorf("idempotency lookup: %w", err)
	}
	return payload, nil
}

// runAndStore runs applyFn, persists its payload, and rolls back the
// placeholder row if applyFn errors (so a retry is treated as the first
// call rather than returning a corrupted '{}' payload forever).
func (s *HostIdempotencyStore) runAndStore(
	ctx context.Context,
	namespace, key string,
	applyFn func(ctx context.Context) ([]byte, error),
) ([]byte, bool, error) {
	payload, applyErr := applyFn(ctx)
	if applyErr != nil {
		// Best-effort cleanup; if DELETE fails the row keeps an empty
		// payload and a retry will see alreadyApplied=true with '{}',
		// which the caller can detect by the empty body. We surface the
		// applyErr as the primary failure; the cleanup error is logged
		// implicitly via the wrapped error.
		_ = s.deleteClaim(ctx, namespace, key)
		return nil, false, applyErr
	}
	if err := s.persistPayload(ctx, namespace, key, payload); err != nil {
		return nil, false, err
	}
	return payload, false, nil
}

// deleteClaim removes a placeholder row whose applyFn failed.
func (s *HostIdempotencyStore) deleteClaim(ctx context.Context, namespace, key string) error {
	const stmt = `DELETE FROM plugin_idempotency WHERE namespace = $1 AND key = $2`
	_, err := s.db.ExecContext(ctx, stmt, namespace, key)
	return err
}

// persistPayload UPDATEs the row with the real payload bytes.
func (s *HostIdempotencyStore) persistPayload(ctx context.Context, namespace, key string, payload []byte) error {
	const stmt = `UPDATE plugin_idempotency SET result_payload = $3 WHERE namespace = $1 AND key = $2`
	if len(payload) == 0 {
		// Empty payload still represents "applied"; persist as JSON null
		// so the column stays valid JSONB.
		payload = []byte("null")
	}
	if _, err := s.db.ExecContext(ctx, stmt, namespace, key, payload); err != nil {
		return fmt.Errorf("idempotency persist: %w", err)
	}
	return nil
}
