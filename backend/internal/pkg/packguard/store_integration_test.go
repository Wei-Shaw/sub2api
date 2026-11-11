//go:build integration

package packguard

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
)

const fixtureKey = "packguard-fixture-not-a-provider-key"
const fixtureBase = "https://ark.ap-southeast.bytepluses.com/api/v3"

type fixture struct {
	db    *sql.DB
	dsn   string
	store *Store
	scope *Scope
}

func setup(t *testing.T) fixture {
	t.Helper()
	dsn := os.Getenv("PACK_GUARD_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL DSN is required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("packguard_%d", time.Now().UnixNano())
	if _, err = admin.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	scoped := parsed.String()
	db, err := sql.Open("postgres", scoped)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(20)
	if _, err = db.Exec(`CREATE TABLE accounts(id BIGINT PRIMARY KEY,credentials JSONB NOT NULL DEFAULT '{}'); INSERT INTO accounts(id) VALUES(11),(12)`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../../migrations/250_byteplus_prepaid_pools.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	hash := HashCredential(fixtureKey)
	if _, err = db.Exec(`WITH scope AS (INSERT INTO byteplus_pack_scopes(id,api_base) VALUES('scope',$2) RETURNING id) INSERT INTO byteplus_pack_credentials(credential_hash,scope_id) SELECT $1,id FROM scope`, hash, fixtureBase); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO byteplus_pack_bindings(account_id,scope_id) VALUES(11,'scope'),(12,'scope')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		_, _ = admin.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`)
		admin.Close()
	})
	return fixture{db, scoped, NewStore(db), &Scope{ID: "scope", CredentialHash: hash, APIBase: fixtureBase}}
}
func (f fixture) grant(t *testing.T, id, model string, units int64, valid time.Duration) {
	t.Helper()
	_, err := f.db.Exec(`INSERT INTO byteplus_pack_grants(id,scope_id,model,capacity_units,expires_at) VALUES($1,'scope',$2,$3,clock_timestamp()+$4::interval)`, id, model, units, fmt.Sprintf("%f seconds", valid.Seconds()))
	if err != nil {
		t.Fatal(err)
	}
}
func (f fixture) balances(t *testing.T) (int64, int64) {
	t.Helper()
	var spent, reserved int64
	if err := f.db.QueryRow(`SELECT COALESCE(SUM(spent_units),0),COALESCE(SUM(reserved_units),0) FROM byteplus_pack_grants`).Scan(&spent, &reserved); err != nil {
		t.Fatal(err)
	}
	return spent, reserved
}
func request(path, body string) *http.Request {
	r, _ := http.NewRequest(http.MethodPost, fixtureBase+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+fixtureKey)
	r.Header.Set("Content-Type", "application/json")
	return r
}
func videoBody() string {
	return `{"model":"dreamina-seedance-2-5-260628","content":[{"type":"text","text":"A cube"}],"duration":4,"resolution":"480p","ratio":"4:3"}`
}
func result(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestConcurrentReservationsRestartAndRepeatedSettlement(t *testing.T) {
	f := setup(t)
	f.grant(t, "pack", SeedanceModel, 100000, 24*time.Hour)
	plan := Plan{Model: SeedanceModel, Units: 1000, Ratio: 64, Lifetime: time.Hour, Async: true}
	var wg sync.WaitGroup
	var accepted atomic.Int64
	var mu sync.Mutex
	var ids []string
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.store.Reserve(context.Background(), f.scope, plan)
			if err == nil {
				accepted.Add(1)
				mu.Lock()
				ids = append(ids, r.ID)
				mu.Unlock()
			} else if !errors.Is(err, ErrExhausted) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 100 {
		t.Fatalf("admitted %d requests against 100 funded slots", accepted.Load())
	}
	if err := f.store.Attach(context.Background(), ids[0], "queued-1"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Uncertain(context.Background(), ids[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE byteplus_pack_reservations SET valid_until=NOW()-INTERVAL '1 hour'`); err != nil {
		t.Fatal(err)
	}
	// A new process/connection has no in-memory reservation state to rely on.
	reopened, err := sql.Open("postgres", f.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	store := NewStore(reopened)
	if _, err = store.Reserve(context.Background(), f.scope, plan); !errors.Is(err, ErrExhausted) {
		t.Fatal("restart or past deadline refunded pending work", err)
	}
	for i := 0; i < 4; i++ {
		if err = store.FinishTask(context.Background(), "scope", "queued-1", 5, true); err != nil {
			t.Fatal(err)
		}
	}
	spent, reserved := f.balances(t)
	if spent != 320 || reserved != 99000 {
		t.Fatalf("duplicate settlement or lost hold: %d %d", spent, reserved)
	}
	exact := plan
	exact.Units = 680
	if _, err = store.Reserve(context.Background(), f.scope, exact); err != nil {
		t.Fatal("unused reservation was not restored", err)
	}
	exact.Units = 1
	if _, err = store.Reserve(context.Background(), f.scope, exact); !errors.Is(err, ErrExhausted) {
		t.Fatal("overspending was admitted", err)
	}
}

func TestExpiryAndGrantAllocationAreExact(t *testing.T) {
	f := setup(t)
	f.grant(t, "too-soon", SeedanceModel, 100000, 30*time.Minute)
	f.grant(t, "old", SeedanceModel, 100, 2*time.Hour)
	f.grant(t, "new", SeedanceModel, 100, 24*time.Hour)
	p := Plan{Model: SeedanceModel, Units: 150, Ratio: 64, Lifetime: time.Hour, Async: true}
	r, err := f.store.Reserve(context.Background(), f.scope, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.Finish(context.Background(), r.ID, 2, true); err != nil {
		t.Fatal(err)
	}
	var old, new int64
	_ = f.db.QueryRow(`SELECT spent_units FROM byteplus_pack_grants WHERE id='old'`).Scan(&old)
	_ = f.db.QueryRow(`SELECT spent_units FROM byteplus_pack_grants WHERE id='new'`).Scan(&new)
	if old != 100 || new != 28 {
		t.Fatalf("wrong expiry priority: %d %d", old, new)
	}
	if _, err = f.db.Exec(`UPDATE byteplus_pack_grants SET expires_at=NOW()-INTERVAL '1 second' WHERE id='old'`); err != nil {
		t.Fatal(err)
	}
	p.Units = 73
	if _, err = f.store.Reserve(context.Background(), f.scope, p); !errors.Is(err, ErrExhausted) {
		t.Fatal("expired/too-short funding was counted", err)
	}
}

func TestTransportRejectsUnfundedExpiredAndQueuedBeforeDispatch(t *testing.T) {
	for _, state := range []string{"unfunded", "expired", "queued"} {
		t.Run(state, func(t *testing.T) {
			f := setup(t)
			plan, _, err := BuildPlan("/contents/generations/tasks", []byte(videoBody()))
			if err != nil {
				t.Fatal(err)
			}
			if state == "expired" {
				f.grant(t, "pack", SeedanceModel, plan.Units, -time.Hour)
			}
			if state == "queued" {
				f.grant(t, "pack", SeedanceModel, plan.Units, 24*time.Hour)
				r, err := f.store.Reserve(context.Background(), f.scope, plan)
				if err != nil {
					t.Fatal(err)
				}
				if err = f.store.Attach(context.Background(), r.ID, "queued-task"); err != nil {
					t.Fatal(err)
				}
			}
			guard := mustTransport(t, f.store)
			calls := 0
			response, err := guard.RoundTrip(request("/contents/generations/tasks", videoBody()), 12, func(*http.Request) (*http.Response, error) {
				calls++
				return result(200, `{"id":"must-not-be-created"}`), nil
			})
			if err != nil || response.StatusCode != 402 || calls != 0 {
				t.Fatalf("%s leaked a request: status=%v calls=%d err=%v", state, response, calls, err)
			}
		})
	}
}

func TestQueuedJobsShareCapacityAcrossAccountsAndPollsSettleOnce(t *testing.T) {
	f := setup(t)
	plan, _, _ := BuildPlan("/contents/generations/tasks", []byte(videoBody()))
	f.grant(t, "pack", SeedanceModel, plan.Units*2+2000, 24*time.Hour)
	guard := mustTransport(t, f.store)
	posts := 0
	next := func(req *http.Request) (*http.Response, error) {
		if req.Method == "POST" {
			posts++
			var body map[string]any
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body["execution_expires_after"] != float64(3600) {
				t.Fatal("queue expiry not enforced")
			}
			return result(200, fmt.Sprintf(`{"id":"task-%d"}`, posts)), nil
		}
		return result(200, `{"id":"task-1","status":"succeeded","usage":{"completion_tokens":10}}`), nil
	}
	for _, account := range []int64{12, 11} {
		res, err := guard.RoundTrip(request("/contents/generations/tasks", videoBody()), account, next)
		if err != nil || res.StatusCode != 200 {
			t.Fatal(err, res)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}
	res, err := guard.RoundTrip(request("/contents/generations/tasks", videoBody()), 12, next)
	if err != nil || res.StatusCode != 402 || posts != 2 {
		t.Fatal("HTTP completion released a queued reservation", err)
	}
	for i := 0; i < 3; i++ {
		req := request("/contents/generations/tasks/task-1", "")
		req.Method = "GET"
		res, err = guard.RoundTrip(req, 12, next)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}
	spent, reserved := f.balances(t)
	if spent != 1070 || reserved != plan.Units {
		t.Fatalf("wrong single billing: %d %d", spent, reserved)
	}
	res, err = guard.RoundTrip(request("/contents/generations/tasks", videoBody()), 12, next)
	if err != nil || res.StatusCode != 200 || posts != 3 {
		t.Fatal("unused funded capacity did not return", err)
	}
}

func TestAmbiguousCreateAndMissingTextUsageKeepFunding(t *testing.T) {
	f := setup(t)
	plan, _, _ := BuildPlan("/contents/generations/tasks", []byte(videoBody()))
	f.grant(t, "video", SeedanceModel, plan.Units, 24*time.Hour)
	guard := mustTransport(t, f.store)
	_, _ = guard.RoundTrip(request("/contents/generations/tasks", videoBody()), 12, func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF })
	_, held := f.balances(t)
	if held != plan.Units {
		t.Fatal("ambiguous POST was refunded")
	}
	f.grant(t, "text", SeedModel, 10000000, 48*time.Hour)
	body := `{"model":"dola-seed-2-1-turbo-260628","messages":[{"role":"user","content":"Hi"}],"max_tokens":16}`
	res, err := guard.RoundTrip(request("/chat/completions", body), 12, func(*http.Request) (*http.Response, error) {
		return result(200, `{"choices":[{"finish_reason":"stop"}]}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	_, after := f.balances(t)
	if after <= held {
		t.Fatal("missing usage was treated as zero consumption")
	}
	var uncertain int
	if err = f.db.QueryRow(`SELECT COUNT(*) FROM byteplus_pack_reservations WHERE state='uncertain'`).Scan(&uncertain); err != nil || uncertain != 2 {
		t.Fatal(err, uncertain)
	}
}

func TestCredentialChangesStorageFailureAndCeilingViolationFailClosed(t *testing.T) {
	f := setup(t)
	f.grant(t, "pack", SeedanceModel, 1000000, 24*time.Hour)
	guard := mustTransport(t, f.store)
	calls := 0
	next := func(*http.Request) (*http.Response, error) { calls++; return result(200, `{}`), nil }
	wrong := request("/contents/generations/tasks", videoBody())
	wrong.Header.Set("Authorization", "Bearer different-key")
	res, _ := guard.RoundTrip(wrong, 12, next)
	if res.StatusCode != 503 || calls != 0 {
		t.Fatal("unverified credential escaped")
	}
	wrong = request("/contents/generations/tasks", videoBody())
	wrong.URL, _ = url.Parse("https://untrusted.example/api/v3/contents/generations/tasks")
	res, _ = guard.RoundTrip(wrong, 12, next)
	if res.StatusCode != 503 || calls != 0 {
		t.Fatal("bound account escaped to a different endpoint")
	}
	p := Plan{Model: SeedanceModel, Units: 1000, Ratio: 64, Lifetime: time.Hour}
	r, err := f.store.Reserve(context.Background(), f.scope, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.Finish(context.Background(), r.ID, 100, true); !errors.Is(err, ErrFrozen) {
		t.Fatal("usage anomaly did not freeze funding", err)
	}
	if _, err = f.store.Reserve(context.Background(), f.scope, p); !errors.Is(err, ErrFrozen) {
		t.Fatal("frozen scope admitted dispatch", err)
	}
	f.db.Close()
	res, _ = guard.RoundTrip(request("/contents/generations/tasks", videoBody()), 12, next)
	if res.StatusCode != 503 || calls != 0 {
		t.Fatal("storage outage failed open")
	}
}

func TestTextTerminalUsageAndEarlyClose(t *testing.T) {
	f := setup(t)
	f.grant(t, "free", SeedModel, 10000000, 48*time.Hour)
	guard := mustTransport(t, f.store)
	body := `{"model":"dola-seed-2-1-turbo-260628","messages":[{"role":"user","content":"Hi"}],"max_tokens":16,"stream":true}`
	stream := "data: {\"choices\":[],\"usage\":{\"total_tokens\":12}}\n\ndata: [DONE]\n\n"
	res, err := guard.RoundTrip(request("/chat/completions", body), 12, func(req *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(req.Body)
		if !bytes.Contains(data, []byte(`"max_completion_tokens":16`)) {
			t.Fatal("output not bounded")
		}
		r := result(200, stream)
		r.Header.Set("Content-Type", "text/event-stream")
		return r, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A normal parser may stop at [DONE] without another Read to receive EOF.
	data := make([]byte, len(stream))
	if _, err = io.ReadFull(res.Body, data); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	spent, held := f.balances(t)
	if spent != 12*64 || held != 0 {
		t.Fatalf("terminal stream accounting failed: %d %d", spent, held)
	}
	res, err = guard.RoundTrip(request("/chat/completions", body), 12, func(*http.Request) (*http.Response, error) {
		r := result(200, "data: {}")
		r.Header.Set("Content-Type", "text/event-stream")
		return r, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	spent, held = f.balances(t)
	if spent != 12*64 || held == 0 {
		t.Fatal("early close refunded uncertain usage")
	}
}

func mustTransport(t *testing.T, ledger Ledger) *Transport {
	t.Helper()
	transport, err := NewTransport(ledger)
	if err != nil {
		t.Fatal(err)
	}
	return transport
}
