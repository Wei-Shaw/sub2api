//go:build integration

package packguard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func accountKey(t *testing.T, f fixture, id int64, key string) {
	t.Helper()
	creds, _ := json.Marshal(map[string]string{"api_key": key, "base_url": fixtureBase})
	if _, err := f.db.Exec(`INSERT INTO accounts(id,credentials) VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET credentials=EXCLUDED.credentials`, id, creds); err != nil {
		t.Fatal(err)
	}
}
func enrollFixture(t *testing.T, f fixture) (int64, string, string) {
	t.Helper()
	const key = "synthetic-admin-key-not-a-provider-credential"
	accountKey(t, f, 13, key)
	accountKey(t, f, 14, key)
	if err := f.store.Enroll(context.Background(), 13, HashCredential(key), fixtureBase, "Fixture pool", ""); err != nil {
		t.Fatal(err)
	}
	st, err := f.store.Status(context.Background(), 13, HashCredential(key), fixtureBase)
	if err != nil {
		t.Fatal(err)
	}
	if st.Pool == nil || !st.Pool.Frozen || !st.Pool.Enforced || st.BoundAccounts != 2 {
		t.Fatalf("unsafe enrollment: %+v", st)
	}
	return 13, key, st.Pool.ID
}

func TestOptInDefaultPAYGThenFrozenUntilVerifiedFunding(t *testing.T) {
	f := setup(t)
	guard := mustTransport(t, f.store)
	calls := 0
	next := func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"cgt-default"}`))}, nil
	}
	req, _ := http.NewRequest(http.MethodPost, fixtureBase+"/contents/generations/tasks", strings.NewReader(`{"model":"future-model"}`))
	req.Header.Set("Authorization", "Bearer unenrolled-test-key")
	resp, err := guard.RoundTrip(req, 13, next)
	if err != nil || resp.StatusCode != 200 || calls != 1 {
		t.Fatal("ordinary PAYG behavior changed")
	}
	_ = resp.Body.Close()
	id, key, pool := enrollFixture(t, f)
	makeRequest := func() *http.Request {
		r, _ := http.NewRequest(http.MethodPost, fixtureBase+"/contents/generations/tasks", strings.NewReader(`{"model":"dreamina-seedance-2-5-260628","content":[{"type":"text","text":"Fixture"}],"resolution":"480p","duration":4}`))
		r.Header.Set("Authorization", "Bearer "+key)
		return r
	}
	resp, err = guard.RoundTrip(makeRequest(), id, next)
	if err != nil || resp.StatusCode != 402 || calls != 1 {
		t.Fatal("new enrollment dispatched before funding verification")
	}
	_ = resp.Body.Close()
	if err = f.store.SetState(context.Background(), pool, "resume", true); !errors.Is(err, ErrExhausted) {
		t.Fatalf("unfunded resume: %v", err)
	}
	if err = f.store.AddGrant(context.Background(), pool, "ModelArk_resource_packages70000000000000001", SeedanceModel, "1200000", time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = f.store.AddGrant(context.Background(), pool, "70000000000000001", SeedanceModel, "1200000", time.Now().Add(2*time.Hour)); !errors.Is(err, ErrConflict) {
		t.Fatal("display prefix bypassed pack deduplication")
	}
	if err = f.store.SetState(context.Background(), pool, "resume", false); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing operator acknowledgement accepted")
	}
	if err = f.store.SetState(context.Background(), pool, "resume", true); err != nil {
		t.Fatal(err)
	}
	resp, err = guard.RoundTrip(makeRequest(), id, next)
	if err != nil || resp.StatusCode != 200 || calls != 2 {
		t.Fatalf("funded request failed: %v", err)
	}
	_ = resp.Body.Close()
	if err = f.store.SetState(context.Background(), pool, "disable", true); !errors.Is(err, ErrPendingJobs) {
		t.Fatalf("queued reservation lost on disable: %v", err)
	}
	if err = f.store.FinishTask(context.Background(), pool, "cgt-default", 39891, true); err != nil {
		t.Fatal(err)
	}
	st, err := f.store.Status(context.Background(), id, HashCredential(key), fixtureBase)
	if err != nil {
		t.Fatal(err)
	}
	if st.OpenJobs != 0 || st.Grants[0].Spent != "66692.765625" || st.Jobs[0].Reserved != "0" || st.Jobs[0].Ceiling == "0" {
		t.Fatal("settlement display or available funds incorrect")
	}
	if err = f.store.SetState(context.Background(), pool, "disable", false); !errors.Is(err, ErrInvalid) {
		t.Fatal("PAYG opt-out needs distinct acknowledgement")
	}
	if err = f.store.SetState(context.Background(), pool, "disable", true); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Enroll(context.Background(), id, HashCredential(key), fixtureBase, "", pool); err != nil {
		t.Fatal(err)
	}
	if err = f.store.SetState(context.Background(), pool, "resume", true); !errors.Is(err, ErrRecheck) {
		t.Fatalf("reenabling reused a stale pack snapshot: %v", err)
	}
	if err = f.store.VerifyGrant(context.Background(), pool, "70000000000000001", "1200000", time.Now().Add(time.Hour)); !errors.Is(err, ErrInvalid) {
		t.Fatal("consumed pack was refilled")
	}
	if err = f.store.VerifyGrant(context.Background(), pool, "70000000000000001", "1000000", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = f.store.SetState(context.Background(), pool, "resume", true); !errors.Is(err, ErrExhausted) {
		t.Fatal("shortened expiry was ignored")
	}
	if err = f.store.AddGrant(context.Background(), pool, "new-long-pack", SeedanceModel, "1000000", time.Now().Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = f.store.SetState(context.Background(), pool, "resume", true); err != nil {
		t.Fatal(err)
	}
}

func TestAliasedCredentialsAndAccountEditsCannotDetachProtection(t *testing.T) {
	f := setup(t)
	id, key, pool := enrollFixture(t, f)
	accountKey(t, f, 15, key)
	var bound string
	if err := f.db.QueryRow(`SELECT scope_id FROM byteplus_pack_bindings WHERE account_id=15`).Scan(&bound); err != nil || bound != pool {
		t.Fatal("cloned account did not inherit protection")
	}
	const replacement = "replacement-fixture-key-not-a-provider-credential"
	accountKey(t, f, id, replacement)
	guard := mustTransport(t, f.store)
	r, _ := http.NewRequest(http.MethodPost, "https://proxy.invalid/v1/chat/completions", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer "+replacement)
	calls := 0
	resp, err := guard.RoundTrip(r, id, func(*http.Request) (*http.Response, error) { calls++; return nil, nil })
	if err != nil || resp.StatusCode != 503 || calls != 0 {
		t.Fatal("changed endpoint or key escaped enforcement")
	}
	_ = resp.Body.Close()
	if err = f.store.Enroll(context.Background(), id, HashCredential(replacement), fixtureBase, "", pool); err != nil {
		t.Fatal(err)
	}
	st, err := f.store.Status(context.Background(), id, HashCredential(replacement), fixtureBase)
	if err != nil {
		t.Fatal(err)
	}
	if !st.CredentialMatches || st.Credentials != 2 || !st.Pool.Frozen {
		t.Fatal("replacement reset the pool or left the old key unprotected")
	}
	accountKey(t, f, 16, key)
	r, _ = http.NewRequest(http.MethodPost, fixtureBase+"/responses", strings.NewReader(`{"model":"dola-seed-2-1-turbo-260628","input":"Hi"}`))
	r.Header.Set("Authorization", "bearer "+key)
	resp, err = guard.RoundTrip(r, 16, func(*http.Request) (*http.Response, error) { calls++; return nil, nil })
	if err != nil || resp.StatusCode != 402 || calls != 0 {
		t.Fatal("old credential alias bypassed the shared frozen pool")
	}
	_ = resp.Body.Close()
	accountKey(t, f, 17, "other-fixture-key")
	if err = f.store.Enroll(context.Background(), 17, HashCredential("other-fixture-key"), fixtureBase, "Wrong empty pool", ""); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Enroll(context.Background(), 17, HashCredential("other-fixture-key"), fixtureBase, "", pool); err != nil {
		t.Fatalf("empty mistaken pool could not be repaired: %v", err)
	}
}

func TestConcurrentEnrollmentAndImmutablePackCredit(t *testing.T) {
	f := setup(t)
	id, key, pool := enrollFixture(t, f)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f.store.Enroll(context.Background(), id, HashCredential(key), fixtureBase, "", pool); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	credits := 0
	var mu sync.Mutex
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := f.store.AddGrant(context.Background(), pool, "one-provider-pack", SeedanceModel, "1000000", time.Now().Add(2*time.Hour))
			if err == nil {
				mu.Lock()
				credits++
				mu.Unlock()
			} else if !errors.Is(err, ErrConflict) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if credits != 1 {
		t.Fatalf("duplicate credit count: %d", credits)
	}
	var pools int
	if err := f.db.QueryRow(`SELECT count(*) FROM byteplus_pack_credentials WHERE credential_hash=$1`, HashCredential(key)).Scan(&pools); err != nil || pools != 1 {
		t.Fatal("credential credited to multiple pools")
	}
}

func TestFundingDisplayExcludesExpiredAndPausedRemainders(t *testing.T) {
	f := setup(t)
	f.grant(t, "usable", SeedanceModel, 128000001, 24*time.Hour)
	f.grant(t, "expired", SeedanceModel, 64000000, -time.Hour)
	f.grant(t, "short", SeedanceModel, 64000000, time.Minute)
	st, err := f.store.Status(context.Background(), 11, HashCredential(fixtureKey), fixtureBase)
	if err != nil {
		t.Fatal(err)
	}
	if st.ObservedAt.IsZero() || len(st.Models) != 2 || st.Models[0].Spendable != "2000000.015625" {
		t.Fatalf("incorrect spendable summary: %+v", st)
	}
	for _, g := range st.Grants {
		if g.VerifiedAt.IsZero() {
			t.Fatal("missing recorded verification")
		}
		if g.ID == "expired" && (g.Available != "1000000" || g.EligibilityReason != "expired") {
			t.Fatal("expired numeric remainder treated as spendable")
		}
	}
	beforeSpent, beforeHeld := f.balances(t)
	if _, err = f.db.Exec(`UPDATE byteplus_pack_scopes SET frozen=TRUE WHERE id='scope'`); err != nil {
		t.Fatal(err)
	}
	paused, err := f.store.Status(context.Background(), 12, HashCredential(fixtureKey), fixtureBase)
	if err != nil || paused.Models[0].Spendable != "0" {
		t.Fatal("shared paused pool still spendable")
	}
	afterSpent, afterHeld := f.balances(t)
	if beforeSpent != afterSpent || beforeHeld != afterHeld {
		t.Fatal("status reads changed funding")
	}
}

func TestFundingStatusIsOneSnapshotDuringConcurrentSettlement(t *testing.T) {
	f := setup(t)
	f.grant(t, "concurrent-status", SeedanceModel, 1000000000, 24*time.Hour)
	ctx := context.Background()
	finished := make(chan error, 1)
	go func() {
		for i := 0; i < 50; i++ {
			r, err := f.store.Reserve(ctx, f.scope, Plan{Model: SeedanceModel, Units: 1000, Ratio: 64, Lifetime: time.Hour})
			if err != nil {
				finished <- err
				return
			}
			if err = f.store.Finish(ctx, r.ID, 1, true); err != nil {
				finished <- err
				return
			}
		}
		finished <- nil
	}()
	for i := 0; i < 30; i++ {
		st, err := f.store.Status(ctx, 11, HashCredential(fixtureKey), fixtureBase)
		if err != nil {
			t.Fatal(err)
		}
		var held int64
		var open int64
		for _, job := range st.Jobs {
			if job.State == "reserved" || job.State == "pending" || job.State == "uncertain" {
				open++
				n, e := parseTokens(job.Reserved, true)
				if e != nil {
					t.Fatal(e)
				}
				held += n
			}
		}
		grantHeld, err := parseTokens(st.Grants[0].Reserved, true)
		if err != nil || grantHeld != held || st.OpenJobs != open {
			t.Fatalf("mixed funding snapshot: grants=%d jobs=%d count=%d/%d", grantHeld, held, st.OpenJobs, open)
		}
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}
