package service

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDingTalkQuotaCents(t *testing.T) {
	for _, n := range []float64{-1, math.NaN(), math.Inf(1), 1.001, 1000000001} {
		_, err := DingTalkQuotaCents(n)
		require.Error(t, err)
	}
	n, err := DingTalkQuotaCents(123.45)
	require.NoError(t, err)
	require.EqualValues(t, 12345, n)
}
func TestDingTalkDepartmentScope(t *testing.T) {
	ds := []DingTalkDepartment{{ID: 1}, {ID: 2, ParentID: 1}, {ID: 3, ParentID: 2}, {ID: 4, ParentID: 1}, {ID: 5, ParentID: 6}, {ID: 6, ParentID: 5}}
	require.Equal(t, map[int64]bool{2: true, 3: true}, DingTalkDepartmentScope(ds, []int64{2, 99}))
	require.Equal(t, map[int64]bool{5: true, 6: true}, DingTalkDepartmentScope(ds, []int64{5}))
	require.NotEqual(t, DingTalkProviderKey("a"), DingTalkProviderKey("b"))
}

// Uses a dedicated schema in the explicitly supplied test database. No existing
// application tables are touched. Exercises real PostgreSQL locks/transactions.
func TestDingTalkOrganizationPostgres(t *testing.T) {
	dsn := os.Getenv("DINGTALK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set DINGTALK_TEST_DATABASE_URL to run PostgreSQL transaction tests")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	schema := fmt.Sprintf("dingtalk_test_%d", time.Now().UnixNano())
	_, err = db.Exec(`CREATE SCHEMA ` + schema)
	require.NoError(t, err)
	defer db.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
	scoped, err := sql.Open("postgres", dsn+"&search_path="+schema)
	require.NoError(t, err)
	defer scoped.Close()
	_, err = scoped.Exec(`CREATE TABLE users(id BIGINT PRIMARY KEY,username TEXT,status TEXT DEFAULT 'active',role TEXT DEFAULT 'user',balance NUMERIC(20,8) DEFAULT 0,deleted_at TIMESTAMPTZ,updated_at TIMESTAMPTZ);CREATE TABLE auth_identities(user_id BIGINT,provider_type TEXT,provider_key TEXT,provider_subject TEXT);INSERT INTO users(id,username) VALUES(1,'manager'),(2,'member'),(3,'outsider'),(4,'second manager');INSERT INTO auth_identities VALUES(2,'dingtalk','dingtalk:a','union2'),(3,'dingtalk','dingtalk:b','union3')`)
	require.NoError(t, err)
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "239_dingtalk_organizations.sql"))
	require.NoError(t, err)
	_, err = scoped.Exec(string(migration))
	require.NoError(t, err)
	s := NewDingTalkOrganizationService(scoped, nil)
	ctx := context.Background()
	ds := []DingTalkDepartment{{ID: 1, Name: "Corp"}, {ID: 2, ParentID: 1, Name: "Team"}, {ID: 3, ParentID: 2, Name: "Child"}, {ID: 4, ParentID: 1, Name: "Other"}}
	ms := []DingTalkDirectoryMember{{DepartmentID: 3, UnionID: "union2", StaffID: "2", Name: "Member"}, {DepartmentID: 4, UnionID: "union3", StaffID: "3", Name: "Other"}}
	require.NoError(t, s.ReplaceDirectory(ctx, "a", ds, ms))
	require.NoError(t, s.SaveManager(ctx, DingTalkManager{UserID: 1, LimitCents: 10000, Enabled: true, Departments: []DingTalkManagerDepartment{{AppID: "a", DepartmentID: 2}}}))
	dir, err := s.Directory(ctx, "a", 1, false)
	require.NoError(t, err)
	require.Len(t, dir.Departments, 2)
	require.Len(t, dir.Members, 1)
	require.EqualValues(t, 2, dir.Members[0].UserID)
	grant := DingTalkQuotaGrant{ActorID: 1, TargetID: 2, AppID: "a", DepartmentID: 3, AmountCents: 6000, RequestID: "allocation-test-0001"}
	outside := grant
	outside.DepartmentID = 4
	outside.RequestID = "outside-dept-0001"
	_, err = s.Grant(ctx, outside, false)
	require.Error(t, err)
	crossed := grant
	crossed.TargetID = 3
	crossed.RequestID = "cross-app-test-0001"
	_, err = s.Grant(ctx, crossed, false)
	require.Error(t, err)
	self := grant
	self.TargetID = 1
	_, err = s.Grant(ctx, self, false)
	require.Error(t, err)
	// Concurrent creation cannot replace an application identity behind a scope.
	settingSvc := &SettingService{settingRepo: &panelRateLimitSettingRepo{values: map[string]string{}}, cfg: &config.Config{}}
	var configSuccess atomic.Int32
	var configWG sync.WaitGroup
	for i := 0; i < 2; i++ {
		configWG.Add(1)
		go func(i int) {
			defer configWG.Done()
			app := config.DingTalkAppConfig{ID: "same", Name: "App", ClientID: fmt.Sprintf("client-%d", i), ClientSecret: "secret", RedirectURL: "https://example.com/callback", Enabled: true}
			if e := s.SaveApps(ctx, settingSvc, []config.DingTalkAppConfig{app}); e == nil {
				configSuccess.Add(1)
			}
		}(i)
	}
	configWG.Wait()
	require.EqualValues(t, 1, configSuccess.Load())
	// Two competing writes exceed the shared budget: exactly one must commit.
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g := grant
			g.RequestID = fmt.Sprintf("concurrent-grant-%d", i)
			if _, e := s.Grant(ctx, g, false); e == nil {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.EqualValues(t, 1, successes.Load())
	var balance float64
	require.NoError(t, scoped.QueryRow(`SELECT balance FROM users WHERE id=2`).Scan(&balance))
	require.Equal(t, 60.0, balance)
	managers, err := s.Managers(ctx, 1, false)
	require.NoError(t, err)
	require.EqualValues(t, 6000, managers[0].UsedCents)
	require.Error(t, s.SaveManager(ctx, DingTalkManager{UserID: 1, LimitCents: 100, Enabled: true}))
	// Raising the cap preserves spending. Concurrent retries must credit once.
	require.NoError(t, s.SaveManager(ctx, DingTalkManager{UserID: 1, LimitCents: 20000, Enabled: true, Departments: []DingTalkManagerDepartment{{AppID: "a", DepartmentID: 2}}}))
	successes.Store(0)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.Grant(ctx, grant, false); e == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 4, successes.Load())
	require.NoError(t, scoped.QueryRow(`SELECT balance FROM users WHERE id=2`).Scan(&balance))
	require.Equal(t, 120.0, balance)
	changed := grant
	changed.AmountCents = 1
	_, err = s.Grant(ctx, changed, false)
	require.Error(t, err)
	records, err := s.Grants(ctx, 1, false)
	require.NoError(t, err)
	require.Len(t, records, 2)
	// Removed memberships become inaccessible on the next complete snapshot.
	require.NoError(t, s.ReplaceDirectory(ctx, "a", ds, nil))
	g := grant
	g.RequestID = "removed-member-0001"
	_, err = s.Grant(ctx, g, false)
	require.Error(t, err)
	require.NoError(t, s.ReplaceDirectory(ctx, "a", ds, ms))
	// Transaction rollback: audit insertion failure must undo balance AND budget.
	_, err = scoped.Exec(`ALTER TABLE dingtalk_quota_grants ADD CONSTRAINT reject_test CHECK(request_id <> 'rollback-grant-0001')`)
	require.NoError(t, err)
	g.RequestID = "rollback-grant-0001"
	_, err = s.Grant(ctx, g, false)
	require.Error(t, err)
	require.NoError(t, scoped.QueryRow(`SELECT balance FROM users WHERE id=2`).Scan(&balance))
	require.Equal(t, 120.0, balance)
	managers, err = s.Managers(ctx, 1, false)
	require.NoError(t, err)
	require.EqualValues(t, 12000, managers[0].UsedCents)
	// Revocation and stale directories fail closed.
	m := managers[0]
	m.Enabled = false
	require.NoError(t, s.SaveManager(ctx, m))
	g.RequestID = "revoked-grant-0001"
	_, err = s.Grant(ctx, g, false)
	require.Error(t, err)
	m.Enabled = true
	require.NoError(t, s.SaveManager(ctx, m))
	_, err = scoped.Exec(`UPDATE dingtalk_directory_snapshots SET synced_at=NOW()-INTERVAL '25 hours'`)
	require.NoError(t, err)
	g.RequestID = "stale-grant-00001"
	_, err = s.Grant(ctx, g, false)
	require.Error(t, err)
}
