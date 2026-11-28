//go:build integration

package packguard

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
)

func TestLegacyMigrationPreservesBalancesHoldsAndSafetyLock(t *testing.T) {
	dsn := os.Getenv("PACK_GUARD_TEST_DSN")
	base, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("packguard_legacy_%d", time.Now().UnixNano())
	if _, err = base.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = base.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`)
		_ = base.Close()
	})
	if _, err = db.Exec(`CREATE TABLE accounts(id BIGINT PRIMARY KEY,credentials JSONB NOT NULL DEFAULT '{}'); INSERT INTO accounts(id) VALUES(11)`); err != nil {
		t.Fatal(err)
	}
	legacy, err := os.ReadFile("testdata/legacy_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(legacy)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO byteplus_pack_scopes(id,credential_hash,api_base,frozen,frozen_reason) VALUES('legacy',$1,$2,TRUE,'provider usage exceeded the reserved ceiling')`, HashCredential(fixtureKey), fixtureBase); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO byteplus_pack_bindings VALUES(11,'legacy');
 INSERT INTO byteplus_pack_grants(id,scope_id,model,capacity_units,spent_units,reserved_units,expires_at) VALUES('legacy-pack','legacy','dreamina-seedance-2-5-260628',100000,320,99000,NOW()+INTERVAL '2 hours');
 INSERT INTO byteplus_pack_reservations(id,scope_id,model,ratio_units,reserved_units,task_id,state,valid_until) VALUES('legacy-hold','legacy','dreamina-seedance-2-5-260628',64,99000,'cgt-legacy','uncertain',NOW()-INTERVAL '1 hour');
 INSERT INTO byteplus_pack_allocations VALUES('legacy-hold','legacy-pack',99000)`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../../migrations/250_byteplus_prepaid_pools.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	st, err := store.Status(context.Background(), 11, HashCredential(fixtureKey), fixtureBase)
	if err != nil {
		t.Fatal(err)
	}
	if !st.CredentialMatches || !st.Pool.Enforced || !st.Pool.Frozen || !st.Pool.SafetyLock || st.OpenJobs != 1 || st.Grants[0].Spent != "5" || st.Grants[0].Reserved != "1546.875" {
		raw, _ := json.Marshal(st)
		t.Fatalf("legacy funding changed: %s", raw)
	}
	if err = store.SetState(context.Background(), "legacy", "resume", true); err != ErrSafetyLock {
		t.Fatalf("legacy safety lock was cleared: %v", err)
	}
	if _, err = store.Reserve(context.Background(), &Scope{ID: "legacy"}, Plan{Model: SeedanceModel, Units: 1, Ratio: 64, Lifetime: time.Hour}); err != ErrFrozen {
		t.Fatal("legacy hold or freeze lost")
	}
}
