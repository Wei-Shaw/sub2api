//go:build unit

package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const (
	insertClaimSQL = `
INSERT INTO plugin_idempotency (namespace, key, result_payload)
VALUES ($1, $2, '{}'::jsonb)
ON CONFLICT (namespace, key) DO NOTHING
`
	updatePayloadSQL = `UPDATE plugin_idempotency SET result_payload = $3 WHERE namespace = $1 AND key = $2`
	selectCachedSQL  = `SELECT result_payload FROM plugin_idempotency WHERE namespace = $1 AND key = $2`
	deleteClaimSQL   = `DELETE FROM plugin_idempotency WHERE namespace = $1 AND key = $2`
)

// TestHostIdempotency_FirstCallApplies covers the happy path: claim row is
// inserted, applyFn runs, payload is persisted, alreadyApplied=false.
func TestHostIdempotency_FirstCallApplies(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectExec(insertClaimSQL).
		WithArgs("credit_balance", "order:123").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(updatePayloadSQL).
		WithArgs("credit_balance", "order:123", []byte(`{"balance":"42"}`)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := NewHostIdempotencyStore(db)
	called := 0
	payload, applied, err := store.LookupOrApply(context.Background(),
		"credit_balance", "order:123",
		func(ctx context.Context) ([]byte, error) {
			called++
			return []byte(`{"balance":"42"}`), nil
		})
	if err != nil {
		t.Fatalf("LookupOrApply: %v", err)
	}
	if applied {
		t.Fatalf("alreadyApplied=true on first call")
	}
	if string(payload) != `{"balance":"42"}` {
		t.Fatalf("payload = %s", payload)
	}
	if called != 1 {
		t.Fatalf("applyFn calls = %d, want 1", called)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestHostIdempotency_SecondCallReturnsCached covers the dedup path: the
// INSERT is a no-op (RowsAffected=0), so applyFn is NOT called and the
// cached payload is SELECTed.
func TestHostIdempotency_SecondCallReturnsCached(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectExec(insertClaimSQL).
		WithArgs("credit_balance", "order:123").
		WillReturnResult(sqlmock.NewResult(0, 0)) // no row inserted
	mock.ExpectQuery(selectCachedSQL).
		WithArgs("credit_balance", "order:123").
		WillReturnRows(sqlmock.NewRows([]string{"result_payload"}).
			AddRow([]byte(`{"balance":"42"}`)))

	store := NewHostIdempotencyStore(db)
	called := 0
	payload, applied, err := store.LookupOrApply(context.Background(),
		"credit_balance", "order:123",
		func(ctx context.Context) ([]byte, error) {
			called++
			return []byte("should not run"), nil
		})
	if err != nil {
		t.Fatalf("LookupOrApply: %v", err)
	}
	if !applied {
		t.Fatalf("alreadyApplied=false on replay")
	}
	if string(payload) != `{"balance":"42"}` {
		t.Fatalf("payload = %s", payload)
	}
	if called != 0 {
		t.Fatalf("applyFn calls = %d, want 0", called)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestHostIdempotency_ApplyFnError rolls back the placeholder so a retry is
// treated as the first call.
func TestHostIdempotency_ApplyFnError(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectExec(insertClaimSQL).
		WithArgs("credit_balance", "order:err").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deleteClaimSQL).
		WithArgs("credit_balance", "order:err").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := NewHostIdempotencyStore(db)
	wantErr := errors.New("apply boom")
	_, _, err = store.LookupOrApply(context.Background(),
		"credit_balance", "order:err",
		func(ctx context.Context) ([]byte, error) {
			return nil, wantErr
		})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want wraps %v", err, wantErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestHostIdempotency_ValidatesArgs rejects empty namespace / key / nil fn.
func TestHostIdempotency_ValidatesArgs(t *testing.T) {
	t.Parallel()
	store := NewHostIdempotencyStore(nil)
	cases := []struct {
		name, ns, key string
		fn            func(ctx context.Context) ([]byte, error)
	}{
		{"empty namespace", "", "k", func(ctx context.Context) ([]byte, error) { return nil, nil }},
		{"empty key", "ns", "", func(ctx context.Context) ([]byte, error) { return nil, nil }},
		{"nil fn", "ns", "k", nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := store.LookupOrApply(context.Background(), tc.ns, tc.key, tc.fn); err == nil {
				t.Fatalf("LookupOrApply: expected error")
			}
		})
	}
}
