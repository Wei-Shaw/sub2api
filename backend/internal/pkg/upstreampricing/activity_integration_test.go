package upstreampricing

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpstreamActivityIntegration(t *testing.T) {
	dsn := os.Getenv("FORK_PRICING_TEST_DSN")
	if dsn == "" {
		t.Skip("set FORK_PRICING_TEST_DSN for isolated PostgreSQL tests")
	}
	root, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("cannot open test database")
	}
	defer root.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	schema := "test_fork_activity_" + randomToken()
	_, err = root.ExecContext(ctx, `CREATE SCHEMA `+schema)
	require.NoError(t, err)
	defer func() { _, _ = root.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) }()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("test DSN must be a PostgreSQL URL")
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	if err != nil {
		t.Fatal("cannot open isolated schema")
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `CREATE TABLE accounts(id bigint PRIMARY KEY,type text,platform text,credentials jsonb,deleted_at timestamptz);
 CREATE TABLE usage_logs(id bigint PRIMARY KEY,account_id bigint,user_id bigint,api_key_id bigint,request_id text,upstream_request_id text,model text,upstream_model text,created_at timestamptz,duration_ms bigint,first_token_ms bigint);`)
	require.NoError(t, err)
	for _, file := range []string{"252_fork_upstream_pricing.sql", "254_fork_upstream_activity.sql"} {
		body, readErr := os.ReadFile("../../../migrations/" + file)
		require.NoError(t, readErr)
		_, err = db.ExecContext(ctx, string(body))
		require.NoError(t, err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO accounts VALUES(1,'apikey','openai','{"base_url":"https://station.example","api_key":"synthetic-inference"}',NULL)`)
	require.NoError(t, err)
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, stationTimezone)
	_, err = db.ExecContext(ctx, `INSERT INTO usage_logs VALUES(1,1,1,1,'local-1','remote-1','m','m',$1,2000,300)`, now.Add(-time.Minute))
	require.NoError(t, err)
	store := &Store{DB: db}
	fp, err := store.AccountFingerprint(ctx, 1)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO fork_upstream_pricing_usage(usage_id,payload) VALUES(1,$1::jsonb)`, string(encoded(Usage{AccountID: 1, AccountFingerprint: fp, PricingAt: now.Add(-2 * time.Minute)})))
	require.NoError(t, err)
	detailCalls, walletCalls := 0, 0
	failDetails, failWallet, mutate := false, false, false
	stationItems := []map[string]any{{"id": 1, "request_id": "remote-1", "model": "m", "created_at": now.Add(-time.Minute).Format(time.RFC3339), "duration_ms": 1500, "first_token_ms": 100}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer synthetic-panel", r.Header.Get("Authorization"))
		require.Equal(t, "1", r.Header.Get("X-User-UI-Request"))
		require.Equal(t, "Asia/Shanghai", r.URL.Query().Get("timezone"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/usage":
			detailCalls++
			if failDetails {
				w.WriteHeader(503)
				return
			}
			require.Equal(t, "2026-11-17", r.URL.Query().Get("start_date"))
			require.Equal(t, "desc", r.URL.Query().Get("sort_order"))
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": len(stationItems), "items": stationItems}})
		case "/api/v1/auth/me":
			walletCalls++
			if mutate {
				_, execErr := db.ExecContext(ctx, `UPDATE fork_upstream_pricing_sources SET revision=revision+1 WHERE account_id=1`)
				require.NoError(t, execErr)
			}
			if failWallet {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"balance":0}}`))
		default:
			t.Errorf("unexpected station path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	cfg := Config{Provider: "sub2api", BaseURL: server.URL, Group: "paid", GroupID: "1", IntervalMinutes: 60, Automatic: false}
	_, err = db.ExecContext(ctx, `INSERT INTO fork_upstream_pricing_sources(account_id,revision,config,credential,account_fingerprint,active_version) VALUES(1,1,$1::jsonb,'fixture:synthetic-panel',$2,1)`, string(encoded(cfg)), fp)
	require.NoError(t, err)
	m := NewManager(store, NewRemote(""), timingTestCipher{})
	defer m.Stop()
	due, err := store.activityDue(ctx, now)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, due, "details run independently of price automatic switch")
	require.NoError(t, m.syncActivity(ctx, 1, now))
	state, err := m.Activity(ctx, 1)
	require.NoError(t, err)
	require.True(t, state.Configured)
	require.Equal(t, now.Unix(), state.LastSuccess.Unix())
	require.Equal(t, "0", *state.WalletBalance)
	require.Equal(t, 1, detailCalls)
	require.Equal(t, 1, walletCalls)
	require.ErrorIs(t, m.syncActivity(ctx, 1, now.Add(2*time.Minute)), ErrBusy)
	cached, err := m.CompareTiming(ctx, 1, 1)
	require.NoError(t, err)
	require.Equal(t, "exact", cached.Status)
	require.EqualValues(t, 500, *cached.DurationDiff)
	require.Equal(t, 1, detailCalls, "hover must only read stored evidence")
	// No local calls since the successful cutoff: skip upstream logs, refresh wallet.
	require.NoError(t, m.syncActivity(ctx, 1, now.Add(5*time.Minute)))
	state, err = m.Activity(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, now.Unix(), state.LastSuccess.Unix())
	require.Equal(t, 1, detailCalls)
	require.Equal(t, 2, walletCalls)
	_, err = db.ExecContext(ctx, `INSERT INTO usage_logs VALUES(2,1,1,1,'local-2','remote-2','m','m',$1,2500,300)`, now.Add(6*time.Minute))
	require.NoError(t, err)
	failDetails = true
	require.NoError(t, m.syncActivity(ctx, 1, now.Add(10*time.Minute)))
	state, err = m.Activity(ctx, 1)
	require.NoError(t, err)
	require.NotEmpty(t, state.LastError)
	require.Equal(t, now.Unix(), state.LastSuccess.Unix())
	failDetails = false
	stationItems = append([]map[string]any{{"id": 2, "request_id": "remote-2", "model": "m", "created_at": now.Add(6 * time.Minute).Format(time.RFC3339), "duration_ms": 2000}}, stationItems...)
	require.NoError(t, m.syncActivity(ctx, 1, now.Add(15*time.Minute)))
	state, err = m.Activity(ctx, 1)
	require.NoError(t, err)
	require.Empty(t, state.LastError)
	require.Equal(t, now.Add(15*time.Minute).Unix(), state.LastSuccess.Unix())
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM fork_upstream_activity_records`).Scan(&count))
	require.Equal(t, 2, count, "overlapping retries upsert station records")
	failWallet = true
	require.NoError(t, m.syncActivity(ctx, 1, now.Add(20*time.Minute)))
	state, err = m.Activity(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "0", *state.WalletBalance)
	require.NotEmpty(t, state.WalletError)
	require.Equal(t, now.Add(15*time.Minute).Unix(), state.WalletCheckedAt.Unix(), "failed wallet refresh preserves timestamp and amount")
	failWallet = false
	mutate = true
	require.ErrorIs(t, m.syncActivity(ctx, 1, now.Add(25*time.Minute)), ErrConflict)
	state, err = m.Activity(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, now.Add(15*time.Minute).Unix(), state.LastSuccess.Unix())
	mutate = false
	cfg.Group = "changed"
	_, err = db.ExecContext(ctx, `UPDATE fork_upstream_pricing_sources SET config=$1::jsonb,revision=revision+1 WHERE account_id=1`, string(encoded(cfg)))
	require.NoError(t, err)
	state, err = m.Activity(ctx, 1)
	require.NoError(t, err)
	require.Nil(t, state.WalletBalance, "changed panel identity cannot show old wallet")
	cached, err = m.CompareTiming(ctx, 1, 1)
	require.NoError(t, err)
	require.Equal(t, "unmatched", cached.Status, "changed panel identity cannot reuse another scope's logs")
	// Yesterday's traffic must not trigger a new day's detail download.
	previousCalls := detailCalls
	tomorrow := time.Date(2026, 11, 18, 0, 3, 0, 0, stationTimezone)
	require.NoError(t, m.syncActivity(ctx, 1, tomorrow))
	require.Equal(t, previousCalls, detailCalls)
	state, err = m.Activity(ctx, 1)
	require.NoError(t, err)
	require.Nil(t, state.LastSuccess)
	require.Equal(t, "0", *state.WalletBalance)
	_, err = db.ExecContext(ctx, `UPDATE accounts SET credentials='{"api_key":"changed"}' WHERE id=1`)
	require.NoError(t, err)
	_, err = m.Activity(ctx, 1)
	require.ErrorIs(t, err, ErrConflict)
}
