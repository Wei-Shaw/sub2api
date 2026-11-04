//go:build integration

package routes

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/packguard"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const prepaidFixtureKey = "fixture-ark-key-not-a-provider-credential"
const prepaidFixtureBase = "https://ark.ap-southeast.bytepluses.com/api/v3"

type prepaidAccounts struct {
	service.AccountRepository
	db *sql.DB
}

func (a prepaidAccounts) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	var raw []byte
	if err := a.db.QueryRowContext(ctx, `SELECT credentials FROM accounts WHERE id=$1`, id).Scan(&raw); err != nil {
		return nil, service.ErrAccountNotFound
	}
	var credentials map[string]any
	if err := json.Unmarshal(raw, &credentials); err != nil {
		return nil, err
	}
	return &service.Account{ID: id, Name: fmt.Sprintf("Fixture Ark %d", id), Platform: "openai", Type: service.AccountTypeAPIKey, Credentials: credentials, Concurrency: 1}, nil
}

type prepaidVendor struct {
	mu    sync.Mutex
	calls int
	jobs  map[string]string
}

func (v *prepaidVendor) Do(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.calls++
	body := `{"data":[{"id":"dreamina-seedance-2-5-260628"},{"id":"dola-seed-2-1-turbo-260628"}]}`
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/contents/generations/tasks") {
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			return nil, err
		}
		if in["execution_expires_after"] != float64(3600) {
			return nil, fmt.Errorf("guard did not enforce the execution lifetime")
		}
		id := fmt.Sprintf("cgt-fixture-%d", len(v.jobs)+1)
		v.jobs[id] = "queued"
		body = fmt.Sprintf(`{"id":%q}`, id)
	} else if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/generations/tasks/") {
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		state := v.jobs[id]
		body = fmt.Sprintf(`{"id":%q,"status":%q,"usage":{"completion_tokens":40000},"content":{"video_url":"https://private.invalid/signed?secret=fixture-only"}}`, id, state)
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}
func (v *prepaidVendor) DoWithTLS(r *http.Request, p string, id int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return v.Do(r, p, id, n)
}
func (v *prepaidVendor) count() int { v.mu.Lock(); defer v.mu.Unlock(); return v.calls }
func (v *prepaidVendor) succeed() {
	v.mu.Lock()
	defer v.mu.Unlock()
	for id := range v.jobs {
		v.jobs[id] = "succeeded"
	}
}

type prepaidHTTPFixture struct {
	server *httptest.Server
	db     *sql.DB
	vendor *prepaidVendor
}

func prepaidHTTPDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PACK_GUARD_TEST_DSN")
	if dsn == "" {
		ctx := context.Background()
		pg, err := tcpostgres.Run(ctx, "postgres:18-alpine", tcpostgres.WithDatabase("packguard"), tcpostgres.WithUsername("packguard"), tcpostgres.WithPassword("fixture-only-not-a-provider-credential"), tcpostgres.BasicWaitStrategies())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = pg.Terminate(ctx) })
		dsn, err = pg.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatal(err)
		}
	}
	base, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("prepaid_http_%d", time.Now().UnixNano())
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
	if _, err = db.Exec(`CREATE TABLE accounts(id BIGINT PRIMARY KEY,credentials JSONB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{11, 12} {
		creds, _ := json.Marshal(map[string]string{"api_key": prepaidFixtureKey, "base_url": prepaidFixtureBase})
		if _, err = db.Exec(`INSERT INTO accounts VALUES($1,$2)`, id, creds); err != nil {
			t.Fatal(err)
		}
	}
	migration, err := os.ReadFile("../../../migrations/250_byteplus_prepaid_pools.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	return db
}

func prepaidHTTP(t *testing.T, browser bool) *prepaidHTTPFixture {
	t.Helper()
	db := prepaidHTTPDatabase(t)
	vendor := &prepaidVendor{jobs: map[string]string{}}
	guarded, err := service.NewBytePlusPackHTTP(vendor, packguard.NewStore(db))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.NewBytePlusPrepaidService(prepaidAccounts{db: db}, db, guarded)
	if err != nil {
		t.Fatal(err)
	}
	h := admin.NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetBytePlusPrepaidService(svc)
	r := gin.New()
	root := r.Group("/api/v1/admin/accounts")
	root.Use(func(c *gin.Context) {
		switch c.GetHeader("Authorization") {
		case "Bearer fixture-admin-not-production":
			c.Set(string(middleware.ContextKeyUserRole), "admin")
		case "Bearer fixture-scoped-not-production":
			c.Set(string(middleware.ContextKeyUserRole), "admin")
			c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{}}}))
		case "Bearer fixture-user-not-production":
			c.Set(string(middleware.ContextKeyUserRole), "user")
		default:
			c.AbortWithStatus(401)
		}
	})
	root.Use(h.ScopedAdminKeyAccounts)
	registerBytePlusPrepaidRoutes(root, h)
	// Test-only model entry point: real ledger/transport, deterministic vendor.
	// It cannot reach a provider or charge anything.
	r.POST("/fixture/submit", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer fixture-admin-not-production" {
			c.AbortWithStatus(401)
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatus(400)
			return
		}
		if len(body) == 0 {
			body = []byte(`{"model":"dreamina-seedance-2-5-260628","content":[{"type":"text","text":"Fixture"}],"duration":4,"resolution":"480p"}`)
		}
		req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, prepaidFixtureBase+"/contents/generations/tasks", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+prepaidFixtureKey)
		resp, err := guarded.Do(req, "", 11, 1)
		if err != nil {
			c.AbortWithStatus(502)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			c.AbortWithStatus(502)
			return
		}
		c.Data(resp.StatusCode, "application/json", data)
	})
	r.POST("/fixture/succeed", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer fixture-admin-not-production" {
			c.AbortWithStatus(401)
			return
		}
		vendor.succeed()
		c.JSON(200, gin.H{"ok": true})
	})
	r.GET("/fixture/counter", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer fixture-admin-not-production" {
			c.AbortWithStatus(401)
			return
		}
		c.JSON(200, gin.H{"upstream_calls": vendor.count()})
	})
	r.POST("/fixture/expire", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer fixture-admin-not-production" {
			c.AbortWithStatus(401)
			return
		}
		if _, err := db.Exec(`UPDATE byteplus_pack_grants SET expires_at=clock_timestamp()-INTERVAL '1 second'`); err != nil {
			c.AbortWithStatus(500)
			return
		}
		c.JSON(200, gin.H{"ok": true})
	})
	server := httptest.NewUnstartedServer(r)
	if browser {
		bind := os.Getenv("PACK_GUARD_BROWSER_BIND")
		if bind == "" {
			bind = "127.0.0.1:18991"
		}
		listener, err := net.Listen("tcp", bind)
		if err != nil {
			t.Fatal(err)
		}
		server.Listener = listener
	}
	server.Start()
	t.Cleanup(server.Close)
	return &prepaidHTTPFixture{server: server, db: db, vendor: vendor}
}

func (f *prepaidHTTPFixture) request(t *testing.T, method, path, token string, input any) (int, []byte) {
	t.Helper()
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, f.server.URL+path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}
func statusFrom(t *testing.T, data []byte) *packguard.Status {
	t.Helper()
	var out struct {
		Data packguard.Status `json:"data"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return &out.Data
}

func TestBytePlusPrepaidAdminGatewayIntegration(t *testing.T) {
	t.Setenv("BYTEPLUS_BILLING_CONFIG_FILE", "")
	f := prepaidHTTP(t, false)
	const adminToken = "fixture-admin-not-production"
	const path = "/api/v1/admin/accounts/11/byteplus-prepaid"
	for _, token := range []string{"fixture-scoped-not-production", "fixture-user-not-production"} {
		code, data := f.request(t, "GET", path, token, nil)
		if code != 403 || strings.Contains(string(data), "grants") {
			t.Fatalf("delegated funding access: %d %s", code, data)
		}
	}
	for _, method := range []string{"GET", "POST"} {
		suffix := "/provider-status"
		if method == "POST" {
			suffix = "/provider-query"
		}
		for _, token := range []string{"fixture-scoped-not-production", "fixture-user-not-production"} {
			code, body := f.request(t, method, path+suffix, token, nil)
			if code != 403 || strings.Contains(string(body), "balance") {
				t.Fatalf("billing access escaped full-admin gate: %d", code)
			}
		}
		code, body := f.request(t, method, path+suffix, adminToken, nil)
		if code != 200 || !strings.Contains(string(body), `"setup":"pool_required"`) || f.vendor.count() != 0 {
			t.Fatalf("unenrolled provider query dispatched inference: %d %s", code, body)
		}
	}
	code, data := f.request(t, "GET", path, adminToken, nil)
	if code != 200 || statusFrom(t, data).Pool != nil {
		t.Fatal("default mode must stay unenrolled")
	}
	code, _ = f.request(t, "POST", path+"/enable", adminToken, map[string]any{"confirmed": false})
	if code != 400 {
		t.Fatal("setup confirmation missing")
	}
	code, data = f.request(t, "POST", path+"/enable", adminToken, map[string]any{"confirmed": true, "name": "HTTP fixture"})
	st := statusFrom(t, data)
	for _, suffix := range []string{"/provider-status", "/provider-query"} {
		method := "GET"
		if suffix == "/provider-query" {
			method = "POST"
		}
		billingCode, body := f.request(t, method, path+suffix, adminToken, nil)
		if billingCode != 200 || !strings.Contains(string(body), `"setup":"missing_credentials"`) || f.vendor.count() != 0 {
			t.Fatalf("missing billing credentials submitted inference: %d %s", billingCode, body)
		}
	}
	if code != 200 || st.Pool == nil || !st.Pool.Frozen || st.BoundAccounts != 2 {
		t.Fatalf("invalid initial state: %d %s", code, data)
	}
	code, _ = f.request(t, "POST", "/fixture/submit", adminToken, nil)
	if code != 402 || f.vendor.count() != 0 {
		t.Fatal("unfunded HTTP request reached upstream")
	}
	grant := map[string]any{"pack_id": "http-fixture-pack", "model": packguard.SeedanceModel, "remaining_tokens": "1200000", "expires_at": time.Now().Add(3 * time.Hour).Format(time.RFC3339), "confirmed": true}
	code, data = f.request(t, "POST", path+"/grant", adminToken, grant)
	if code != 200 {
		t.Fatalf("grant failed: %d %s", code, data)
	}
	code, _ = f.request(t, "POST", path+"/grant", adminToken, grant)
	if code != 409 {
		t.Fatal("pack duplicated through HTTP")
	}
	code, data = f.request(t, "POST", path+"/resume", adminToken, map[string]any{"confirmed": true})
	if code != 200 {
		t.Fatalf("resume failed: %s", data)
	}
	for i := 0; i < 2; i++ {
		code, data = f.request(t, "POST", "/fixture/submit", adminToken, nil)
		if code != 200 {
			t.Fatalf("funded create: %d %s", code, data)
		}
	}
	code, _ = f.request(t, "POST", "/fixture/submit", adminToken, nil)
	if code != 402 || f.vendor.count() != 2 {
		t.Fatal("fully reserved HTTP pool dispatched another job")
	}
	code, _ = f.request(t, "POST", path+"/disable", adminToken, map[string]any{"allow_pay_as_you_go": true})
	if code != 409 {
		t.Fatal("queued holds lost through opt-out")
	}
	code, _ = f.request(t, "POST", path+"/freeze", adminToken, map[string]any{})
	if code != 200 {
		t.Fatal("freeze failed")
	}
	code, data = f.request(t, "GET", path, adminToken, nil)
	st = statusFrom(t, data)
	if st.ObservedAt.IsZero() || len(st.Models) != 2 || st.Grants[0].VerifiedAt.IsZero() {
		t.Fatal("display freshness/summary contract missing")
	}
	if st.OpenJobs != 2 {
		t.Fatal("queued holds missing")
	}
	_, _ = f.request(t, "POST", "/fixture/succeed", adminToken, nil)
	for _, job := range st.Jobs {
		code, data = f.request(t, "POST", path+"/reservations/"+job.ID+"/reconcile", adminToken, nil)
		if code != 200 || strings.Contains(string(data), "private.invalid") || strings.Contains(string(data), prepaidFixtureKey) {
			t.Fatalf("reconcile leaked data or failed: %d %s", code, data)
		}
	}
	st = statusFrom(t, data)
	if st.OpenJobs != 0 || st.Grants[0].Reserved != "0" || st.Grants[0].Spent != "133750" {
		t.Fatal("HTTP settlement or held-token display incorrect")
	}
	for _, job := range st.Jobs {
		_, _ = f.request(t, "POST", path+"/reservations/"+job.ID+"/reconcile", adminToken, nil)
	}
	code, data = f.request(t, "GET", path, adminToken, nil)
	if statusFrom(t, data).Grants[0].Spent != "133750" {
		t.Fatal("repeated poll double-charged")
	}
	code, _ = f.request(t, "POST", path+"/disable", adminToken, map[string]any{"confirmed": true})
	if code != 400 {
		t.Fatal("generic confirmation enabled PAYG")
	}
	code, _ = f.request(t, "POST", path+"/disable", adminToken, map[string]any{"allow_pay_as_you_go": true})
	if code != 200 {
		t.Fatal("explicit opt-out failed")
	}
	code, data = f.request(t, "POST", path+"/enable", adminToken, map[string]any{"confirmed": true})
	if code != 200 {
		t.Fatal("reenable failed")
	}
	code, _ = f.request(t, "POST", path+"/resume", adminToken, map[string]any{"confirmed": true})
	if code != 409 {
		t.Fatal("stale balance reused after PAYG")
	}
}

func TestBytePlusPrepaidBrowserHarness(t *testing.T) {
	if os.Getenv("PACK_GUARD_BROWSER_HARNESS") != "1" {
		t.Skip("manual isolated browser harness; automated HTTP flow runs above")
	}
	f := prepaidHTTP(t, true)
	t.Log("isolated browser API on " + f.server.URL + "; no real provider credentials or outbound model calls")
	seconds := 7200
	if raw := os.Getenv("PACK_GUARD_BROWSER_SECONDS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			seconds = n
		}
	}
	<-time.After(time.Duration(seconds) * time.Second)
}
