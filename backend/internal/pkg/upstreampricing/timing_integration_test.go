package upstreampricing

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type timingTestCipher struct{}

func (timingTestCipher) Encrypt(v string) (string, error) { return "fixture:" + v, nil }
func (timingTestCipher) Decrypt(v string) (string, error) { return v[len("fixture:"):], nil }

func TestStationTimingManagerIntegration(t *testing.T) {
	dsn := os.Getenv("FORK_PRICING_TEST_DSN")
	if dsn == "" {
		t.Skip("set FORK_PRICING_TEST_DSN for isolated PostgreSQL tests")
	}
	root, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("cannot open integration database")
	}
	defer root.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	if root.PingContext(ctx) != nil {
		t.Fatal("integration database unavailable")
	}
	schema := "test_fork_timing_" + randomToken()
	_, err = root.ExecContext(ctx, `CREATE SCHEMA `+schema)
	require.NoError(t, err)
	defer func() { _, _ = root.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) }()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("integration DSN must be a PostgreSQL URL")
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal("cannot open isolated schema")
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `CREATE TABLE accounts(id bigint PRIMARY KEY,type text,platform text,credentials jsonb,deleted_at timestamptz);
 CREATE TABLE usage_logs(id bigint PRIMARY KEY,account_id bigint,user_id bigint,api_key_id bigint,request_id text,upstream_request_id text,
 model text,upstream_model text,created_at timestamptz,duration_ms bigint,first_token_ms bigint);`)
	require.NoError(t, err)
	migration, err := os.ReadFile("../../../migrations/252_fork_upstream_pricing.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO accounts VALUES (1,'apikey','openai','{"base_url":"https://example.com","api_key":"synthetic-key"}',NULL),(2,'apikey','openai','{}',NULL);
 INSERT INTO usage_logs VALUES(7,1,1,1,'client-id','station-id','m','m','2026-11-12T08:01:00Z',2500,300);`)
	require.NoError(t, err)
	store := &Store{DB: db}
	fp, err := store.AccountFingerprint(ctx, 1)
	require.NoError(t, err)
	stamp, err := time.Parse(time.RFC3339, "2026-11-12T08:00:58Z")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO fork_upstream_pricing_usage(usage_id,payload) VALUES(7,$1::jsonb)`, string(encoded(Usage{AccountID: 1, AccountFingerprint: fp, PricingAt: stamp})))
	require.NoError(t, err)
	activityMigration, err := os.ReadFile("../../../migrations/254_fork_upstream_activity.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(activityMigration))
	require.NoError(t, err)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer server.Close()
	c := Config{Provider: "newapi", BaseURL: server.URL, UserID: "42", Group: "private", IntervalMinutes: 60}
	_, err = db.ExecContext(ctx, `INSERT INTO fork_upstream_pricing_sources(account_id,revision,config,credential,account_fingerprint) VALUES(1,1,$1::jsonb,'fixture:synthetic-panel',$2)`, string(encoded(c)), fp)
	require.NoError(t, err)
	m := NewManager(store, NewRemote(""), timingTestCipher{})
	defer m.cancel()
	out, err := m.CompareTiming(ctx, 1, 7)
	require.NoError(t, err)
	require.Equal(t, "unmatched", out.Status)
	require.Zero(t, calls, "hover never queries the station")
	src, err := store.Source(ctx, 1)
	require.NoError(t, err)
	duration, first := int64(2000), int64(100)
	record := TimingRecord{ID: "99", RequestID: "station-id", Model: "m", CreatedAt: stamp, DurationMs: &duration, FirstTokenMs: &first, PrecisionMs: 1000}
	_, err = db.ExecContext(ctx, `INSERT INTO fork_upstream_activity_records VALUES($1,$2,$3,$4,$5::jsonb,$6)`, 1, activityIdentity(src), record.ID, record.RequestID, string(encoded(record)), stamp)
	require.NoError(t, err)
	out, err = m.CompareTiming(ctx, 1, 7)
	require.NoError(t, err)
	require.Equal(t, "exact", out.Status)
	require.EqualValues(t, 500, *out.DurationDiff)
	require.EqualValues(t, 200, *out.FirstDiff)
	_, err = m.CompareTiming(ctx, 2, 7)
	require.Error(t, err, "cross-account cache isolation")
	require.Zero(t, calls)
	_, err = db.ExecContext(ctx, `UPDATE accounts SET credentials='{"api_key":"changed"}' WHERE id=1`)
	require.NoError(t, err)
	_, err = m.CompareTiming(ctx, 1, 7)
	require.ErrorIs(t, err, ErrConflict)
	require.Zero(t, calls)
	_, err = db.ExecContext(ctx, `UPDATE accounts SET credentials='{"base_url":"https://example.com","api_key":"synthetic-key"}' WHERE id=1; DELETE FROM fork_upstream_pricing_usage WHERE usage_id=7`)
	require.NoError(t, err)
	_, err = m.CompareTiming(ctx, 1, 7)
	require.Error(t, err, "old requests without immutable upstream identity are not guessed")
	require.Zero(t, calls)
}
