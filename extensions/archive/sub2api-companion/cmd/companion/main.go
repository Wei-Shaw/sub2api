package main

import (
	"bufio"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	_ "modernc.org/sqlite"
)

//go:embed web/*
var webFS embed.FS

const a6StagingMatchMethod = "a6_import_staging"

const usageSnapshotsDDL = `
CREATE TABLE IF NOT EXISTS usage_snapshots (
  source_id INTEGER PRIMARY KEY, request_id TEXT NOT NULL, user_id INTEGER NOT NULL,
  user_email TEXT NOT NULL DEFAULT '', api_key_id INTEGER NOT NULL, account_id INTEGER NOT NULL,
  group_id INTEGER NOT NULL DEFAULT 0, group_name TEXT NOT NULL DEFAULT '', model TEXT NOT NULL,
  upstream_model TEXT NOT NULL DEFAULT '', actual_cost TEXT NOT NULL,
  input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
  input_cost TEXT NOT NULL DEFAULT '0', output_cost TEXT NOT NULL DEFAULT '0',
  cache_read_cost TEXT NOT NULL DEFAULT '0', cache_creation_cost TEXT NOT NULL DEFAULT '0',
  source_currency TEXT NOT NULL DEFAULT 'PLATFORM_UNIT', fx_rate_to_cny TEXT NOT NULL DEFAULT '1',
  actual_cost_cny TEXT NOT NULL DEFAULT '0', created_at TEXT NOT NULL,
  rule_provider TEXT NOT NULL DEFAULT '', rule_external_key TEXT NOT NULL DEFAULT '',
  rule_multiplier TEXT NOT NULL DEFAULT '', rule_version INTEGER NOT NULL DEFAULT 0,
  rule_snapshot_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS usage_request_idx ON usage_snapshots(request_id);
CREATE INDEX IF NOT EXISTS usage_created_idx ON usage_snapshots(created_at);
CREATE INDEX IF NOT EXISTS usage_created_source_idx ON usage_snapshots(created_at, source_id DESC);
CREATE INDEX IF NOT EXISTS usage_account_created_source_idx ON usage_snapshots(account_id, created_at DESC, source_id DESC);`

type config struct {
	ListenAddr                string
	DataDir                   string
	PricingFile               string
	CaddyLogPath              string
	SourceDatabaseURL         string
	AdminUser                 string
	AdminPassword             string
	SyncInterval              time.Duration
	Sub2APIUnitToCNY          decimal.Decimal
	A6BaseURL                 string
	A6UserID                  string
	A6AccessToken             string
	A6TokenAccountMap         map[string]int64
	A6USDToCNY                decimal.Decimal
	FXPrimaryURL              string
	FXFallbackURL             string
	FXRefreshInterval         time.Duration
	FXHTTPTimeout             time.Duration
	FXMaxStaleness            time.Duration
	A6BootstrapWindow         time.Duration
	A6HistoricalTokenLookback time.Duration
	A6SyncInterval            time.Duration
	A6HTTPTimeout             time.Duration
	A6PageSize                int
	A6MaxPages                int
	A6BackfillChunk           time.Duration
	A6BackfillPageDelay       time.Duration
	SubarxAccountMultipliers  map[int64]decimal.Decimal
	SubarxBaseURL             string
	SubarxAccountAPIKeys      map[int64]string
	SubarxSyncInterval        time.Duration
	SubarxHTTPTimeout         time.Duration
	SubarxLookbackDays        int
	SubarxTimezone            string
}

type app struct {
	cfg                       config
	db                        *sql.DB
	source                    *sql.DB
	http                      *http.Client
	subarxHTTP                *http.Client
	fxHTTP                    *http.Client
	httpMu                    sync.Mutex
	mu                        sync.Mutex
	backfillMu                sync.Mutex
	backfillRunning           bool
	rulesMu                   sync.RWMutex
	a6TokenAccountMap         map[string]int64
	a6TokenAccounts           map[string][]int64
	a6HistoricalTokenAccounts map[string][]int64
	subarxAccountMultipliers  map[int64]decimal.Decimal
	accountProviders          map[int64]string
	accountRuleSnapshots      map[int64]accountRuleSnapshot
}

type pricingDocument struct {
	SchemaVersion string         `json:"schema_version"`
	Currency      string         `json:"currency"`
	PriceUnit     string         `json:"price_unit"`
	SiteName      string         `json:"site_name"`
	SiteDomain    string         `json:"site_domain"`
	Models        []pricingModel `json:"models"`
}

type pricingModel struct {
	ModelName          string   `json:"model_name"`
	GroupName          string   `json:"group_name"`
	InputPrice         *float64 `json:"input_price,omitempty"`
	OutputPrice        *float64 `json:"output_price,omitempty"`
	CacheInputPrice    *float64 `json:"cache_input_price,omitempty"`
	CacheCreatePrice   *float64 `json:"cache_create_price"`
	CacheCreatePrice1H *float64 `json:"cache_create_price_1h"`
	Enabled            bool     `json:"enabled"`
	Note               string   `json:"note,omitempty"`
}

type upstreamRecord struct {
	Provider          string `json:"provider"`
	UpstreamRequestID string `json:"upstream_request_id"`
	Cost              string `json:"cost"`
	Currency          string `json:"currency"`
	FXRateToCNY       string `json:"fx_rate_to_cny"`
	OccurredAt        string `json:"occurred_at"`
	Model             string `json:"model"`
	InputTokens       int64  `json:"input_tokens"`
	OutputTokens      int64  `json:"output_tokens"`
	Source            string `json:"source"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(cfg.DataDir, "companion.db"))
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	a := &app{cfg: cfg, db: db}
	if err := a.migrate(); err != nil {
		log.Fatal(err)
	}
	if cfg.SourceDatabaseURL != "" {
		a.source, err = sql.Open("postgres", cfg.SourceDatabaseURL)
		if err != nil {
			log.Fatal(err)
		}
		a.source.SetMaxOpenConns(2)
	}
	go a.runCollectors()
	a.resumeA6Backfill()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /api/provider/pricing", a.pricing)
	mux.Handle("GET /ops/profit", a.basicAuth(http.HandlerFunc(a.dashboard)))
	mux.Handle("GET /ops/", a.basicAuth(http.HandlerFunc(a.dashboard)))
	mux.Handle("GET /ops/api/summary", a.basicAuth(http.HandlerFunc(a.summary)))
	mux.Handle("GET /ops/api/timeseries", a.basicAuth(http.HandlerFunc(a.timeseries)))
	mux.Handle("GET /ops/api/requests", a.basicAuth(http.HandlerFunc(a.requests)))
	mux.Handle("POST /ops/api/upstream/import", a.basicAuth(http.HandlerFunc(a.importUpstream)))
	mux.Handle("POST /ops/api/collect", a.basicAuth(http.HandlerFunc(a.collectNow)))
	mux.Handle("GET /ops/api/a6/backfill", a.basicAuth(http.HandlerFunc(a.a6BackfillStatus)))
	mux.Handle("POST /ops/api/a6/backfill", a.basicAuth(http.HandlerFunc(a.startA6Backfill)))
	mux.Handle("GET /ops/api/account-rules", a.basicAuth(http.HandlerFunc(a.accountRules)))
	mux.Handle("PUT /ops/api/account-rules/{account_id}", a.basicAuth(http.HandlerFunc(a.putAccountRule)))
	mux.Handle("DELETE /ops/api/account-rules/{account_id}", a.basicAuth(http.HandlerFunc(a.deleteAccountRule)))

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: securityHeaders(mux), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("companion listening on %s", cfg.ListenAddr)
	log.Fatal(srv.ListenAndServe())
}

func loadConfig() (config, error) {
	interval, err := time.ParseDuration(env("SYNC_INTERVAL", "30s"))
	if err != nil {
		return config{}, fmt.Errorf("invalid SYNC_INTERVAL: %w", err)
	}
	unitRate, err := decimal.NewFromString(env("SUB2API_UNIT_TO_CNY_RATE", "1"))
	if err != nil || !unitRate.IsPositive() {
		return config{}, errors.New("SUB2API_UNIT_TO_CNY_RATE must be a positive decimal")
	}
	c := config{
		ListenAddr: env("LISTEN_ADDR", ":8090"), DataDir: env("DATA_DIR", "./data"),
		PricingFile: env("PRICING_FILE", "./config/pricing.json"), CaddyLogPath: os.Getenv("CADDY_LOG_PATH"),
		SourceDatabaseURL: os.Getenv("SOURCE_DATABASE_URL"), AdminUser: os.Getenv("ADMIN_USER"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"), SyncInterval: interval, Sub2APIUnitToCNY: unitRate,
	}
	if c.AdminUser == "" || c.AdminPassword == "" {
		return config{}, errors.New("ADMIN_USER and ADMIN_PASSWORD are required")
	}
	if err := loadA6Config(&c); err != nil {
		return config{}, err
	}
	c.SubarxAccountMultipliers, err = parseSubarxAccountMultipliers(osEnv("SUBARX_ACCOUNT_MULTIPLIERS"))
	if err != nil {
		return config{}, err
	}
	if err := loadSubarxConfig(&c); err != nil {
		return config{}, err
	}
	return c, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func (a *app) migrate() error {
	_, err := a.db.Exec(`
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS state (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS request_maps (
  client_request_id TEXT PRIMARY KEY, upstream_request_id TEXT NOT NULL,
  observed_at TEXT NOT NULL, path TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS request_maps_upstream_idx ON request_maps(upstream_request_id);
CREATE TABLE IF NOT EXISTS upstream_usage (
  provider TEXT NOT NULL, upstream_request_id TEXT NOT NULL, cost TEXT NOT NULL,
  currency TEXT NOT NULL, fx_rate_to_cny TEXT NOT NULL, cost_cny TEXT NOT NULL,
	fx_rate_source TEXT NOT NULL DEFAULT 'legacy_fixed',
	fx_rate_effective_at TEXT NOT NULL DEFAULT '', fx_rate_fetched_at TEXT NOT NULL DEFAULT '',
	fx_rate_stale INTEGER NOT NULL DEFAULT 0,
  occurred_at TEXT NOT NULL, model TEXT NOT NULL DEFAULT '',
  input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
  cache_tokens INTEGER NOT NULL DEFAULT 0, token_name TEXT NOT NULL DEFAULT '',
  account_id_hint INTEGER NOT NULL DEFAULT 0, channel_id INTEGER NOT NULL DEFAULT 0,
  channel_name TEXT NOT NULL DEFAULT '', billing_details TEXT NOT NULL DEFAULT '',
  usage_source_id INTEGER, match_method TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL, imported_at TEXT NOT NULL,
  PRIMARY KEY(provider, upstream_request_id)
);
CREATE INDEX IF NOT EXISTS upstream_occurred_idx ON upstream_usage(occurred_at);
CREATE TABLE IF NOT EXISTS fx_rates (
  base_currency TEXT NOT NULL, quote_currency TEXT NOT NULL, rate TEXT NOT NULL,
  effective_at TEXT NOT NULL, fetched_at TEXT NOT NULL, source TEXT NOT NULL,
  stale INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(base_currency, quote_currency, effective_at)
);
CREATE INDEX IF NOT EXISTS fx_rates_lookup_idx ON fx_rates(base_currency, quote_currency, effective_at DESC);
` + subarxDDL + `
`)
	if err != nil {
		return err
	}
	const usageSchemaVersion = "billing_only_v3"
	if a.stateString("usage_schema_version") != usageSchemaVersion {
		tx, err := a.db.Begin()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`DROP TABLE IF EXISTS usage_snapshots;` + usageSnapshotsDDL + `
DELETE FROM state WHERE key='usage_source_id';
INSERT INTO state(key,value) VALUES('usage_schema_version','billing_only_v3') ON CONFLICT(key) DO UPDATE SET value=excluded.value;
INSERT INTO state(key,value) VALUES('usage_currency_semantics','platform_unit_v1') ON CONFLICT(key) DO UPDATE SET value=excluded.value;`); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	} else if _, err := a.db.Exec(usageSnapshotsDDL); err != nil {
		return err
	}
	upstreamColumns := []struct{ name, definition string }{
		{"cache_tokens", "INTEGER NOT NULL DEFAULT 0"},
		{"token_name", "TEXT NOT NULL DEFAULT ''"},
		{"account_id_hint", "INTEGER NOT NULL DEFAULT 0"},
		{"channel_id", "INTEGER NOT NULL DEFAULT 0"},
		{"channel_name", "TEXT NOT NULL DEFAULT ''"},
		{"billing_details", "TEXT NOT NULL DEFAULT ''"},
		{"usage_source_id", "INTEGER"},
		{"match_method", "TEXT NOT NULL DEFAULT ''"},
		{"fx_rate_source", "TEXT NOT NULL DEFAULT 'legacy_fixed'"},
		{"fx_rate_effective_at", "TEXT NOT NULL DEFAULT ''"},
		{"fx_rate_fetched_at", "TEXT NOT NULL DEFAULT ''"},
		{"fx_rate_stale", "INTEGER NOT NULL DEFAULT 0"},
		{"allocation_scope", "TEXT NOT NULL DEFAULT ''"},
		{"billing_date", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range upstreamColumns {
		if _, err := a.ensureColumn("upstream_usage", column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := a.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS upstream_usage_source_idx ON upstream_usage(usage_source_id) WHERE usage_source_id IS NOT NULL`); err != nil {
		return err
	}
	if _, err := a.db.Exec(`CREATE INDEX IF NOT EXISTS upstream_usage_subarx_scope_idx ON upstream_usage(provider,source,account_id_hint,billing_date,allocation_scope)`); err != nil {
		return err
	}
	if _, err := a.db.Exec(`CREATE INDEX IF NOT EXISTS usage_rule_token_idx ON usage_snapshots(rule_provider,rule_external_key,account_id,created_at)`); err != nil {
		return err
	}
	if _, err := a.db.Exec(`DROP INDEX IF EXISTS upstream_account_rules_a6_key_idx`); err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{
		{"rule_provider", "TEXT NOT NULL DEFAULT ''"},
		{"rule_external_key", "TEXT NOT NULL DEFAULT ''"},
		{"rule_multiplier", "TEXT NOT NULL DEFAULT ''"},
		{"rule_version", "INTEGER NOT NULL DEFAULT 0"},
		{"rule_snapshot_at", "TEXT NOT NULL DEFAULT ''"},
	} {
		if _, err := a.ensureColumn("usage_snapshots", column.name, column.definition); err != nil {
			return err
		}
	}
	return a.initAccountRules()
}

func (a *app) ensureColumn(table, column, definition string) (bool, error) {
	rows, err := a.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return false, err
		}
		if name == column {
			return false, rows.Close()
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	_, err = a.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err == nil, err
}

func (a *app) runCollectors() {
	ticker := time.NewTicker(a.cfg.SyncInterval)
	defer ticker.Stop()
	for {
		if a.cfg.A6AccessToken != "" {
			if _, err := a.ensureCurrentFXRate(time.Now().UTC()); err != nil {
				log.Printf("fx collector: %v", err)
			}
		}
		if err := a.collect(false); err != nil {
			log.Printf("collector: %v", err)
		}
		<-ticker.C
	}
}

func (a *app) collect(forceA6 bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.collectCaddy(); err != nil {
		return err
	}
	if err := a.collectUsage(); err != nil {
		return err
	}
	var errs []error
	if err := a.reconcileUpstream(); err != nil {
		errs = append(errs, err)
	}
	if err := a.collectA6(forceA6); err != nil {
		errs = append(errs, err)
	}
	if err := a.reconcileUpstream(); err != nil {
		errs = append(errs, err)
	}
	if err := a.collectSubarx(forceA6); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (a *app) collectCaddy() error {
	if a.cfg.CaddyLogPath == "" {
		return nil
	}
	f, err := os.Open(a.cfg.CaddyLogPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open caddy log: %w", err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	offset := a.stateInt64("caddy_offset")
	if offset > stat.Size() {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		var entry struct {
			Timestamp float64 `json:"ts"`
			Request   struct {
				URI string `json:"uri"`
			} `json:"request"`
			Headers map[string][]string `json:"resp_headers"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			continue
		}
		clientID := firstHeader(entry.Headers, "X-Client-Request-Id")
		upstreamID := lastHeader(entry.Headers, "X-Request-Id")
		if clientID == "" || upstreamID == "" || clientID == upstreamID {
			continue
		}
		observed := time.Unix(0, int64(entry.Timestamp*float64(time.Second))).UTC().Format(time.RFC3339Nano)
		_, err = a.db.Exec(`INSERT INTO request_maps(client_request_id, upstream_request_id, observed_at, path)
VALUES(?,?,?,?) ON CONFLICT(client_request_id) DO UPDATE SET upstream_request_id=excluded.upstream_request_id, observed_at=excluded.observed_at, path=excluded.path`, clientID, upstreamID, observed, entry.Request.URI)
		if err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	return a.setState("caddy_offset", strconv.FormatInt(pos, 10))
}

func firstHeader(headers map[string][]string, name string) string {
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

func lastHeader(headers map[string][]string, name string) string {
	for key, values := range headers {
		if !strings.EqualFold(key, name) {
			continue
		}
		for i := len(values) - 1; i >= 0; i-- {
			if value := strings.TrimSpace(values[i]); value != "" {
				return value
			}
		}
	}
	return ""
}

func (a *app) collectUsage() error {
	if a.source == nil {
		return nil
	}
	lastID := a.stateInt64("usage_source_id")
	rows, err := a.source.Query(`SELECT l.id, l.request_id, l.user_id, COALESCE(u.email,''), l.api_key_id, l.account_id,
COALESCE(l.group_id,0), COALESCE(g.name,''), l.model, COALESCE(l.upstream_model,''), l.actual_cost::text,
l.input_tokens, l.output_tokens, l.cache_read_tokens, l.cache_creation_tokens,
l.input_cost::text, l.output_cost::text, l.cache_read_cost::text, l.cache_creation_cost::text, l.created_at
FROM usage_logs l JOIN users u ON u.id=l.user_id LEFT JOIN groups g ON g.id=l.group_id
WHERE l.id > $1 ORDER BY l.id LIMIT 1000`, lastID)
	if err != nil {
		return fmt.Errorf("query source usage: %w", err)
	}
	defer rows.Close()
	ruleSnapshots := a.accountRuleSnapshotDetails()
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	maxID := lastID
	for rows.Next() {
		var id, userID, apiKeyID, accountID, groupID, inTok, outTok, cacheRead, cacheCreate int64
		var requestID, userEmail, groupName, model, upstreamModel, cost, inputCost, outputCost, cacheReadCost, cacheCreateCost string
		var created time.Time
		if err := rows.Scan(&id, &requestID, &userID, &userEmail, &apiKeyID, &accountID, &groupID, &groupName, &model, &upstreamModel, &cost, &inTok, &outTok, &cacheRead, &cacheCreate, &inputCost, &outputCost, &cacheReadCost, &cacheCreateCost, &created); err != nil {
			tx.Rollback()
			return err
		}
		actualUnit, err := decimal.NewFromString(cost)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("invalid source actual_cost for usage %d: %w", id, err)
		}
		snapshot := ruleSnapshots[accountID]
		snapshotAt := time.Now().UTC().Format(time.RFC3339Nano)
		_, err = tx.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,user_email,api_key_id,account_id,group_id,group_name,model,upstream_model,actual_cost,input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,input_cost,output_cost,cache_read_cost,cache_creation_cost,source_currency,fx_rate_to_cny,actual_cost_cny,created_at,rule_provider,rule_external_key,rule_multiplier,rule_version,rule_snapshot_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(source_id) DO UPDATE SET request_id=excluded.request_id,user_id=excluded.user_id,user_email=excluded.user_email,api_key_id=excluded.api_key_id,account_id=excluded.account_id,group_id=excluded.group_id,group_name=excluded.group_name,model=excluded.model,upstream_model=excluded.upstream_model,actual_cost=excluded.actual_cost,input_tokens=excluded.input_tokens,output_tokens=excluded.output_tokens,cache_read_tokens=excluded.cache_read_tokens,cache_creation_tokens=excluded.cache_creation_tokens,input_cost=excluded.input_cost,output_cost=excluded.output_cost,cache_read_cost=excluded.cache_read_cost,cache_creation_cost=excluded.cache_creation_cost,source_currency=excluded.source_currency,fx_rate_to_cny=excluded.fx_rate_to_cny,actual_cost_cny=excluded.actual_cost_cny,created_at=excluded.created_at`, id, requestID, userID, userEmail, apiKeyID, accountID, groupID, groupName, model, upstreamModel, cost, inTok, outTok, cacheRead, cacheCreate, inputCost, outputCost, cacheReadCost, cacheCreateCost, "PLATFORM_UNIT", a.cfg.Sub2APIUnitToCNY.String(), actualUnit.Mul(a.cfg.Sub2APIUnitToCNY).String(), created.UTC().Format(time.RFC3339Nano), snapshot.Provider, snapshot.ExternalKey, snapshot.Multiplier, snapshot.Version, snapshotAt)
		if err != nil {
			tx.Rollback()
			return err
		}
		if id > maxID {
			maxID = id
		}
	}
	if err := rows.Err(); err != nil {
		tx.Rollback()
		return err
	}
	if maxID != lastID {
		if _, err := tx.Exec(`INSERT INTO state(key,value) VALUES('usage_source_id',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strconv.FormatInt(maxID, 10)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (a *app) stateInt64(key string) int64 {
	raw := a.stateString(key)
	if raw == "" {
		return 0
	}
	v, _ := strconv.ParseInt(raw, 10, 64)
	return v
}

func (a *app) stateString(key string) string {
	var raw string
	if a.db.QueryRow(`SELECT value FROM state WHERE key=?`, key).Scan(&raw) != nil {
		return ""
	}
	return raw
}

func (a *app) setState(key, value string) error {
	_, err := a.db.Exec(`INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (a *app) health(w http.ResponseWriter, _ *http.Request) {
	a6Rules, subarxRules, _ := a.accountRuleSnapshot()
	a6LastSync := a.stateInt64("a6_last_sync_unix")
	a6LastError := a.stateString("a6_last_sync_error")
	a6LastSyncAt := ""
	if a6LastSync > 0 {
		a6LastSyncAt = time.Unix(a6LastSync, 0).UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "source_database": a.source != nil, "caddy_log": a.cfg.CaddyLogPath != "",
		"a6_sync": a.cfg.A6AccessToken != "", "a6_rule_accounts": len(a6Rules),
		"a6_sync_ok":      a.cfg.A6AccessToken != "" && a6LastSync > 0 && a6LastError == "",
		"a6_last_sync_at": a6LastSyncAt, "a6_last_sync_error": a6LastError,
		"subarx_sync": len(a.cfg.SubarxAccountAPIKeys) > 0, "subarx_api_accounts": len(a.cfg.SubarxAccountAPIKeys),
		"subarx_rule_accounts": len(subarxRules),
	})
}

func (a *app) pricing(w http.ResponseWriter, _ *http.Request) {
	b, err := os.ReadFile(a.cfg.PricingFile)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "pricing unavailable")
		return
	}
	var doc pricingDocument
	if err := json.Unmarshal(b, &doc); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid pricing configuration")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": doc.SchemaVersion, "success": true, "message": "", "data": map[string]any{"currency": doc.Currency, "price_unit": doc.PriceUnit, "site_name": doc.SiteName, "site_domain": doc.SiteDomain, "updated_at": time.Now().UTC().Format(time.RFC3339), "models": doc.Models}})
}

func (a *app) dashboard(w http.ResponseWriter, _ *http.Request) {
	b, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "dashboard unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}

func (a *app) summary(w http.ResponseWriter, r *http.Request) {
	window, err := queryTimeWindow(r, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	totals, err := a.profitSummary(window.From, window.To)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	_, billedCount, upstreamUnmatched, err := a.upstreamTotals(window.From, window.To)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	subarxUnallocatedCost, subarxUnallocatedCount, err := a.subarxUnallocatedTotals(window.From, window.To)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	profit := totals.MatchedRevenue.Sub(totals.MatchedCost)
	fx := a.latestFXRate()
	margin := decimal.Zero
	if !totals.MatchedRevenue.IsZero() {
		margin = profit.Div(totals.MatchedRevenue).Mul(decimal.NewFromInt(100))
	}
	writeJSON(w, 200, map[string]any{
		"from":                     window.From,
		"to":                       window.To,
		"revenue":                  totals.Revenue.StringFixed(8),
		"matched_revenue":          totals.MatchedRevenue.StringFixed(8),
		"upstream_cost":            totals.MatchedCost.StringFixed(8),
		"billed_upstream_cost":     totals.MatchedCost.StringFixed(8),
		"gross_profit":             profit.StringFixed(8),
		"margin_percent":           margin.StringFixed(2),
		"matched":                  totals.Matched,
		"unmatched":                totals.Unmatched,
		"downstream_matched":       totals.Matched,
		"downstream_unmatched":     totals.Unmatched,
		"upstream_unmatched":       upstreamUnmatched,
		"record_total":             totals.Matched + totals.Unmatched + upstreamUnmatched,
		"billed_count":             billedCount,
		"cost_policy":              "billed_or_subarx_api_or_rule",
		"calculated_count":         totals.Calculated,
		"subarx_unallocated_cost":  subarxUnallocatedCost.StringFixed(8),
		"subarx_unallocated_count": subarxUnallocatedCount,
		"profit_scope":             "matched_only",
		"currency":                 "CNY",
		"fx_usd_cny":               fx.Rate.String(),
		"fx_source":                fx.Source,
		"fx_effective_at":          formatOptionalTime(fx.EffectiveAt),
		"fx_stale":                 fx.Stale,
	})
}

func (a *app) subarxUnallocatedTotals(from, to string) (decimal.Decimal, int, error) {
	rows, err := a.db.Query(`SELECT cost_cny FROM upstream_usage WHERE provider=? AND source=? AND usage_source_id IS NULL AND occurred_at>=? AND occurred_at<?`, subarxProvider, subarxUsageSource, from, to)
	if err != nil {
		return decimal.Zero, 0, err
	}
	defer rows.Close()
	total := decimal.Zero
	count := 0
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return decimal.Zero, 0, err
		}
		value, err := decimal.NewFromString(raw)
		if err != nil {
			return decimal.Zero, 0, err
		}
		total = total.Add(value)
		count++
	}
	return total, count, rows.Err()
}

type summaryTotals struct {
	Revenue        decimal.Decimal
	MatchedRevenue decimal.Decimal
	MatchedCost    decimal.Decimal
	Matched        int
	Unmatched      int
	Calculated     int
}

type timeSeriesBucket struct {
	Start             time.Time
	Revenue           decimal.Decimal
	MatchedRevenue    decimal.Decimal
	UpstreamCost      decimal.Decimal
	Matched           int
	Unmatched         int
	UpstreamUnmatched int
}

type timeSeriesPoint struct {
	Start             string `json:"start"`
	Revenue           string `json:"revenue"`
	UpstreamCost      string `json:"upstream_cost"`
	GrossProfit       string `json:"gross_profit"`
	Matched           int    `json:"matched"`
	Unmatched         int    `json:"unmatched"`
	UpstreamUnmatched int    `json:"upstream_unmatched"`
	RecordTotal       int    `json:"record_total"`
}

func (a *app) timeseries(w http.ResponseWriter, r *http.Request) {
	window, err := queryTimeWindow(r, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	points, bucketLabel, err := a.buildTimeSeries(window.From, window.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": window.From, "to": window.To, "bucket": bucketLabel, "points": points})
}

func (a *app) buildTimeSeries(from, to string) ([]timeSeriesPoint, string, error) {
	_, subarxRules, _ := a.accountRuleSnapshot()
	fromTime, err := time.Parse(time.RFC3339Nano, from)
	if err != nil {
		return nil, "", err
	}
	toTime, err := time.Parse(time.RFC3339Nano, to)
	if err != nil {
		return nil, "", err
	}
	bucketSize, bucketLabel := timeSeriesBucketSize(toTime.Sub(fromTime))
	bucketCount := int((toTime.Sub(fromTime) + bucketSize - 1) / bucketSize)
	buckets := make([]timeSeriesBucket, bucketCount)
	for i := range buckets {
		buckets[i] = timeSeriesBucket{Start: fromTime.Add(time.Duration(i) * bucketSize), Revenue: decimal.Zero, MatchedRevenue: decimal.Zero, UpstreamCost: decimal.Zero}
	}
	rows, err := a.db.Query(`SELECT u.source_id,u.created_at,u.account_id,u.actual_cost_cny,
COALESCE(x.cost_cny,''),COALESCE(s.allocation_status,''),u.input_cost,u.output_cost,u.cache_read_cost,u.cache_creation_cost
FROM usage_snapshots u LEFT JOIN upstream_usage x ON x.usage_source_id=u.source_id
LEFT JOIN subarx_usage_scope_status s ON s.usage_source_id=u.source_id
WHERE u.created_at>=? AND u.created_at<?`, from, to)
	if err != nil {
		return nil, "", err
	}
	for rows.Next() {
		var sourceID, accountID int64
		var createdRaw, revenueRaw, billedRaw, scopeStatus, inputRaw, outputRaw, cacheReadRaw, cacheCreateRaw string
		if err := rows.Scan(&sourceID, &createdRaw, &accountID, &revenueRaw, &billedRaw, &scopeStatus, &inputRaw, &outputRaw, &cacheReadRaw, &cacheCreateRaw); err != nil {
			rows.Close()
			return nil, "", err
		}
		createdAt, err := time.Parse(time.RFC3339Nano, createdRaw)
		if err != nil {
			rows.Close()
			return nil, "", fmt.Errorf("invalid stored time for usage %d: %w", sourceID, err)
		}
		index := int(createdAt.Sub(fromTime) / bucketSize)
		if index < 0 || index >= len(buckets) {
			continue
		}
		revenue, err := decimal.NewFromString(revenueRaw)
		if err != nil {
			rows.Close()
			return nil, "", fmt.Errorf("invalid stored revenue for usage %d: %w", sourceID, err)
		}
		bucket := &buckets[index]
		bucket.Revenue = bucket.Revenue.Add(revenue)
		cost := decimal.Zero
		matched := false
		if billedRaw != "" {
			cost, err = decimal.NewFromString(billedRaw)
			if err != nil {
				rows.Close()
				return nil, "", fmt.Errorf("invalid stored upstream cost for usage %d: %w", sourceID, err)
			}
			matched = true
		} else if scopeStatus == "" && !a.subarxAPICovers(accountID, createdRaw) {
			if multiplier, ok := subarxRules[accountID]; ok {
				standardCost := decimal.Zero
				for _, raw := range []string{inputRaw, outputRaw, cacheReadRaw, cacheCreateRaw} {
					value, parseErr := decimal.NewFromString(raw)
					if parseErr != nil {
						rows.Close()
						return nil, "", fmt.Errorf("invalid stored standard cost for usage %d: %w", sourceID, parseErr)
					}
					standardCost = standardCost.Add(value)
				}
				cost = subarxCharge(standardCost.Mul(multiplier))
				matched = true
			}
		}
		if matched {
			bucket.Matched++
			bucket.MatchedRevenue = bucket.MatchedRevenue.Add(revenue)
			bucket.UpstreamCost = bucket.UpstreamCost.Add(cost)
		} else {
			bucket.Unmatched++
		}
	}
	if err := rows.Close(); err != nil {
		return nil, "", err
	}
	rows, err = a.db.Query(`SELECT occurred_at FROM upstream_usage WHERE usage_source_id IS NULL AND match_method<>'`+a6StagingMatchMethod+`' AND occurred_at>=? AND occurred_at<?`, from, to)
	if err != nil {
		return nil, "", err
	}
	for rows.Next() {
		var occurredRaw string
		if err := rows.Scan(&occurredRaw); err != nil {
			rows.Close()
			return nil, "", err
		}
		occurredAt, err := time.Parse(time.RFC3339Nano, occurredRaw)
		if err != nil {
			continue
		}
		index := int(occurredAt.Sub(fromTime) / bucketSize)
		if index >= 0 && index < len(buckets) {
			buckets[index].UpstreamUnmatched++
		}
	}
	if err := rows.Close(); err != nil {
		return nil, "", err
	}
	points := make([]timeSeriesPoint, len(buckets))
	targetRevenue, targetCost, targetProfit := decimal.Zero, decimal.Zero, decimal.Zero
	displayedRevenue, displayedCost, displayedProfit := decimal.Zero, decimal.Zero, decimal.Zero
	lastRevenue, lastMatched := -1, -1
	for i, bucket := range buckets {
		profit := bucket.MatchedRevenue.Sub(bucket.UpstreamCost)
		points[i] = timeSeriesPoint{
			Start: bucket.Start.UTC().Format(time.RFC3339Nano), Revenue: bucket.Revenue.StringFixed(8),
			UpstreamCost: bucket.UpstreamCost.StringFixed(8), GrossProfit: profit.StringFixed(8),
			Matched: bucket.Matched, Unmatched: bucket.Unmatched, UpstreamUnmatched: bucket.UpstreamUnmatched,
			RecordTotal: bucket.Matched + bucket.Unmatched + bucket.UpstreamUnmatched,
		}
		targetRevenue = targetRevenue.Add(bucket.Revenue)
		targetCost = targetCost.Add(bucket.UpstreamCost)
		targetProfit = targetProfit.Add(profit)
		displayedRevenue = displayedRevenue.Add(decimal.RequireFromString(points[i].Revenue))
		displayedCost = displayedCost.Add(decimal.RequireFromString(points[i].UpstreamCost))
		displayedProfit = displayedProfit.Add(decimal.RequireFromString(points[i].GrossProfit))
		if !bucket.Revenue.IsZero() {
			lastRevenue = i
		}
		if bucket.Matched > 0 {
			lastMatched = i
		}
	}
	if lastRevenue >= 0 {
		diff := decimal.RequireFromString(targetRevenue.StringFixed(8)).Sub(displayedRevenue)
		points[lastRevenue].Revenue = decimal.RequireFromString(points[lastRevenue].Revenue).Add(diff).StringFixed(8)
	}
	if lastMatched >= 0 {
		costDiff := decimal.RequireFromString(targetCost.StringFixed(8)).Sub(displayedCost)
		profitDiff := decimal.RequireFromString(targetProfit.StringFixed(8)).Sub(displayedProfit)
		points[lastMatched].UpstreamCost = decimal.RequireFromString(points[lastMatched].UpstreamCost).Add(costDiff).StringFixed(8)
		points[lastMatched].GrossProfit = decimal.RequireFromString(points[lastMatched].GrossProfit).Add(profitDiff).StringFixed(8)
	}
	return points, bucketLabel, nil
}

func timeSeriesBucketSize(window time.Duration) (time.Duration, string) {
	switch {
	case window <= 48*time.Hour:
		return time.Hour, "1小时"
	case window <= 14*24*time.Hour:
		return 6 * time.Hour, "6小时"
	case window <= 120*24*time.Hour:
		return 24 * time.Hour, "1天"
	default:
		return 7 * 24 * time.Hour, "1周"
	}
}

func (a *app) profitSummary(from, to string) (summaryTotals, error) {
	_, subarxRules, _ := a.accountRuleSnapshot()
	rows, err := a.db.Query(`SELECT u.source_id,u.created_at,u.account_id,u.actual_cost_cny,
COALESCE(x.cost_cny,''),COALESCE(s.allocation_status,''),u.input_cost,u.output_cost,u.cache_read_cost,u.cache_creation_cost
FROM usage_snapshots u
LEFT JOIN upstream_usage x ON x.usage_source_id=u.source_id
LEFT JOIN subarx_usage_scope_status s ON s.usage_source_id=u.source_id
WHERE u.created_at>=? AND u.created_at<?`, from, to)
	if err != nil {
		return summaryTotals{}, err
	}
	defer rows.Close()

	totals := summaryTotals{Revenue: decimal.Zero, MatchedRevenue: decimal.Zero, MatchedCost: decimal.Zero}
	for rows.Next() {
		var sourceID, accountID int64
		var createdRaw, revenueRaw, billedRaw, scopeStatus, inputRaw, outputRaw, cacheReadRaw, cacheCreateRaw string
		if err := rows.Scan(&sourceID, &createdRaw, &accountID, &revenueRaw, &billedRaw, &scopeStatus, &inputRaw, &outputRaw, &cacheReadRaw, &cacheCreateRaw); err != nil {
			return summaryTotals{}, err
		}
		revenue, err := decimal.NewFromString(revenueRaw)
		if err != nil {
			return summaryTotals{}, fmt.Errorf("invalid stored revenue for usage %d: %w", sourceID, err)
		}
		totals.Revenue = totals.Revenue.Add(revenue)

		cost := decimal.Zero
		matched := false
		if billedRaw != "" {
			cost, err = decimal.NewFromString(billedRaw)
			if err != nil {
				return summaryTotals{}, fmt.Errorf("invalid stored upstream cost for usage %d: %w", sourceID, err)
			}
			matched = true
		} else if scopeStatus == "" && !a.subarxAPICovers(accountID, createdRaw) {
			if multiplier, ok := subarxRules[accountID]; ok {
				standardCost := decimal.Zero
				for _, raw := range []string{inputRaw, outputRaw, cacheReadRaw, cacheCreateRaw} {
					value, parseErr := decimal.NewFromString(raw)
					if parseErr != nil {
						return summaryTotals{}, fmt.Errorf("invalid stored standard cost for usage %d: %w", sourceID, parseErr)
					}
					standardCost = standardCost.Add(value)
				}
				cost = subarxCharge(standardCost.Mul(multiplier))
				matched = true
				totals.Calculated++
			}
		}
		if matched {
			totals.Matched++
			totals.MatchedRevenue = totals.MatchedRevenue.Add(revenue)
			totals.MatchedCost = totals.MatchedCost.Add(cost)
		} else {
			totals.Unmatched++
		}
	}
	return totals, rows.Err()
}

func (a *app) upstreamTotals(from, to string) (decimal.Decimal, int, int, error) {
	rows, err := a.db.Query(`SELECT x.cost_cny, x.usage_source_id IS NOT NULL
FROM upstream_usage x WHERE x.occurred_at>=? AND x.occurred_at<?
AND NOT (x.usage_source_id IS NULL AND x.match_method='`+a6StagingMatchMethod+`')`, from, to)
	if err != nil {
		return decimal.Zero, 0, 0, err
	}
	defer rows.Close()
	total := decimal.Zero
	count := 0
	unmatched := 0
	for rows.Next() {
		var raw string
		var mapped bool
		if err := rows.Scan(&raw, &mapped); err != nil {
			return decimal.Zero, 0, 0, err
		}
		value, err := decimal.NewFromString(raw)
		if err != nil {
			return decimal.Zero, 0, 0, fmt.Errorf("invalid stored upstream cost: %w", err)
		}
		if mapped {
			total = total.Add(value)
			count++
		} else {
			unmatched++
		}
	}
	return total, count, unmatched, rows.Err()
}

type profitRow struct {
	RecordType        string          `json:"record_type"`
	SourceID          int64           `json:"source_id"`
	CreatedAt         string          `json:"created_at"`
	RequestID         string          `json:"request_id"`
	UpstreamRequestID string          `json:"upstream_request_id"`
	UserID            int64           `json:"user_id"`
	UserEmail         string          `json:"user_email"`
	APIKeyID          int64           `json:"api_key_id"`
	AccountID         int64           `json:"account_id"`
	GroupID           int64           `json:"group_id"`
	GroupName         string          `json:"group_name"`
	Model             string          `json:"model"`
	InputTokens       int64           `json:"input_tokens"`
	OutputTokens      int64           `json:"output_tokens"`
	CacheTokens       int64           `json:"cache_tokens"`
	Revenue           decimal.Decimal `json:"-"`
	Cost              decimal.Decimal `json:"-"`
	BilledCost        decimal.Decimal `json:"-"`
	RevenueText       string          `json:"revenue"`
	CostText          string          `json:"upstream_cost"`
	BilledCostText    string          `json:"billed_upstream_cost"`
	UpstreamCostText  string          `json:"upstream_cost_original"`
	UpstreamCurrency  string          `json:"upstream_currency"`
	ProfitText        string          `json:"gross_profit"`
	CostSource        string          `json:"cost_source"`
	FXRateToCNY       string          `json:"fx_rate_to_cny"`
	CostSourceLabel   string          `json:"cost_source_label"`
	Matched           bool            `json:"matched"`
}

func (a *app) profitRows(from string, limit int) ([]profitRow, error) {
	return a.profitRowsPage(from, "9999-12-31T23:59:59.999999999Z", "downstream_all", limit, 0)
}

func (a *app) profitRowsPage(from, to, status string, limit, offset int) ([]profitRow, error) {
	_, subarxRules, providers := a.accountRuleSnapshot()
	base := `SELECT 'downstream',u.source_id,u.created_at,u.request_id,COALESCE(x.upstream_request_id,m.upstream_request_id,''),u.user_id,u.user_email,u.api_key_id,u.account_id,u.group_id,u.group_name,u.model,u.input_tokens,u.output_tokens,(u.cache_read_tokens+u.cache_creation_tokens),u.input_cost,u.output_cost,u.cache_read_cost,u.cache_creation_cost,u.actual_cost_cny,COALESCE(x.cost_cny,''),COALESCE(x.cost,''),COALESCE(x.currency,''),COALESCE(x.fx_rate_to_cny,''),COALESCE(x.source,''),COALESCE(s.allocation_status,''),u.rule_provider,u.rule_external_key,u.rule_multiplier,u.rule_version,u.rule_snapshot_at
FROM usage_snapshots u LEFT JOIN request_maps m ON m.client_request_id=CASE WHEN u.request_id LIKE 'client:%' THEN substr(u.request_id,8) ELSE u.request_id END
LEFT JOIN upstream_usage x ON x.usage_source_id=u.source_id
LEFT JOIN subarx_usage_scope_status s ON s.usage_source_id=u.source_id
WHERE u.created_at>=? AND u.created_at<?`
	args := []any{from, to}
	if status == "upstream_unmatched" {
		base = `SELECT 'upstream_unmatched',0 AS source_id,x.occurred_at AS created_at,'',x.upstream_request_id,0,'',0,x.account_id_hint,0,'',x.model,x.input_tokens,x.output_tokens,x.cache_tokens,'0','0','0','0','',x.cost_cny,x.cost,x.currency,x.fx_rate_to_cny,x.source,'','','','',0,''
FROM upstream_usage x WHERE x.usage_source_id IS NULL AND x.match_method<>'` + a6StagingMatchMethod + `' AND x.occurred_at>=? AND x.occurred_at<?`
	} else if status == "matched" || status == "unmatched" {
		condition, conditionArgs := a.downstreamMatchedCondition()
		if status == "unmatched" {
			condition = "NOT " + condition
		}
		base += " AND " + condition
		args = append(args, conditionArgs...)
	}
	query := base
	if status == "all" {
		query = `SELECT * FROM (` + base + ` UNION ALL
SELECT 'upstream_unmatched',0 AS source_id,x.occurred_at AS created_at,'',x.upstream_request_id,0,'',0,x.account_id_hint,0,'',x.model,x.input_tokens,x.output_tokens,x.cache_tokens,'0','0','0','0','',x.cost_cny,x.cost,x.currency,x.fx_rate_to_cny,x.source,'','','','',0,''
FROM upstream_usage x WHERE x.usage_source_id IS NULL AND x.match_method<>'` + a6StagingMatchMethod + `' AND x.occurred_at>=? AND x.occurred_at<?)`
		args = append(args, from, to)
	}
	query += " ORDER BY created_at DESC, source_id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]profitRow, 0)
	for rows.Next() {
		var row profitRow
		var inputCostRaw, outputCostRaw, cacheReadCostRaw, cacheCreateCostRaw, revenueRaw, billedRaw, upstreamRaw, upstreamSource, scopeStatus string
		var snapshot accountRuleSnapshot
		if err := rows.Scan(&row.RecordType, &row.SourceID, &row.CreatedAt, &row.RequestID, &row.UpstreamRequestID, &row.UserID, &row.UserEmail, &row.APIKeyID, &row.AccountID, &row.GroupID, &row.GroupName, &row.Model, &row.InputTokens, &row.OutputTokens, &row.CacheTokens, &inputCostRaw, &outputCostRaw, &cacheReadCostRaw, &cacheCreateCostRaw, &revenueRaw, &billedRaw, &upstreamRaw, &row.UpstreamCurrency, &row.FXRateToCNY, &upstreamSource, &scopeStatus, &snapshot.Provider, &snapshot.ExternalKey, &snapshot.Multiplier, &snapshot.Version, &snapshot.CapturedAt); err != nil {
			return nil, err
		}
		if row.RecordType == "upstream_unmatched" {
			row.CostSource = "upstream_unmatched"
			if upstreamSource == subarxUsageSource {
				row.CostSource = "subarx_unallocated"
			}
			row.CostText = fixedDecimal(billedRaw)
			row.BilledCostText = row.CostText
			row.UpstreamCostText = fixedDecimal(upstreamRaw)
			row.CostSourceLabel = "上游待匹配"
			if upstreamSource == subarxUsageSource {
				row.CostSourceLabel = "Subarx 账单差额"
			}
			out = append(out, row)
			continue
		}
		row.Revenue, _ = decimal.NewFromString(revenueRaw)
		row.BilledCost, _ = decimal.NewFromString(billedRaw)
		row.Matched = billedRaw != ""
		row.CostSource = "pending"
		if row.Matched {
			row.Cost = row.BilledCost
			row.CostSource = "billed"
			if upstreamCost, err := decimal.NewFromString(upstreamRaw); err == nil {
				row.UpstreamCostText = upstreamCost.StringFixed(8)
			}
			row.CostSourceLabel = "账单实扣"
			if upstreamSource == subarxUsageSource {
				row.CostSource = "subarx_billed_allocation"
				row.CostSourceLabel = "已对账"
			}
		} else if scopeStatus != "" {
			row.CostSource = "subarx_pending"
			row.CostSourceLabel = "Subarx 账单待分配"
		} else if a.subarxAPICovers(row.AccountID, row.CreatedAt) {
			row.CostSource = "subarx_waiting"
			row.CostSourceLabel = "等待 Subarx 账单"
		} else if multiplier, ok := a.snapshotSubarxMultiplier(row.AccountID, snapshot, subarxRules); ok {
			standardCost := decimal.Zero
			for _, raw := range []string{inputCostRaw, outputCostRaw, cacheReadCostRaw, cacheCreateCostRaw} {
				value, err := decimal.NewFromString(raw)
				if err != nil {
					return nil, fmt.Errorf("invalid stored standard cost for usage %d: %w", row.SourceID, err)
				}
				standardCost = standardCost.Add(value)
			}
			row.BilledCost = subarxCharge(standardCost.Mul(multiplier))
			row.Cost = row.BilledCost
			row.Matched = true
			row.CostSource = "subarx_rule"
			row.CostSourceLabel = "规则实扣"
			row.UpstreamCurrency = "USD"
			row.FXRateToCNY = "1"
			row.UpstreamCostText = row.BilledCost.StringFixed(8)
		} else if snapshot.Provider == "" {
			if _, configured := providers[row.AccountID]; !configured {
				row.CostSource = "rule_unconfigured"
				row.CostSourceLabel = "规则待配置"
			}
		} else if snapshot.Provider == "a6" {
			if row.UpstreamRequestID != "" {
				row.CostSource = "a6_waiting"
				row.CostSourceLabel = "等待上游账单"
			} else {
				row.CostSource = "a6_pending"
				row.CostSourceLabel = "上游账单待匹配"
			}
		}
		row.RevenueText = row.Revenue.StringFixed(8)
		if row.Matched {
			row.CostText = row.Cost.StringFixed(8)
			row.BilledCostText = row.BilledCost.StringFixed(8)
			row.ProfitText = row.Revenue.Sub(row.Cost).StringFixed(8)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (a *app) snapshotSubarxMultiplier(accountID int64, snapshot accountRuleSnapshot, current map[int64]decimal.Decimal) (decimal.Decimal, bool) {
	if snapshot.Provider == "subarx" {
		multiplier, err := decimal.NewFromString(snapshot.Multiplier)
		return multiplier, err == nil && multiplier.IsPositive()
	}
	// Compatibility for test/imported rows created before rule snapshots existed.
	multiplier, ok := current[accountID]
	return multiplier, ok
}

func fixedDecimal(raw string) string {
	value, err := decimal.NewFromString(raw)
	if err != nil {
		return ""
	}
	return value.StringFixed(8)
}

func (a *app) downstreamMatchedCondition() (string, []any) {
	_, subarxRules, _ := a.accountRuleSnapshot()
	condition := "(x.usage_source_id IS NOT NULL"
	args := make([]any, 0, len(subarxRules)+1)
	legacyIDs := make([]int64, 0, len(subarxRules))
	apiIDs := make([]int64, 0, len(subarxRules))
	for accountID := range subarxRules {
		if _, ok := a.cfg.SubarxAccountAPIKeys[accountID]; ok {
			apiIDs = append(apiIDs, accountID)
		} else {
			legacyIDs = append(legacyIDs, accountID)
		}
	}
	sort.Slice(legacyIDs, func(i, j int) bool { return legacyIDs[i] < legacyIDs[j] })
	sort.Slice(apiIDs, func(i, j int) bool { return apiIDs[i] < apiIDs[j] })
	fallbackConditions := make([]string, 0, 2)
	legacyPlaceholders := make([]string, len(legacyIDs))
	for i, accountID := range legacyIDs {
		legacyPlaceholders[i] = "?"
		args = append(args, accountID)
	}
	if len(legacyIDs) > 0 {
		fallbackConditions = append(fallbackConditions, "u.account_id IN ("+strings.Join(legacyPlaceholders, ",")+")")
	}
	apiPlaceholders := make([]string, len(apiIDs))
	for i, accountID := range apiIDs {
		apiPlaceholders[i] = "?"
		args = append(args, accountID)
	}
	if len(apiIDs) > 0 {
		args = append(args, a.subarxAPICoverageStart(time.Now()).Format(time.RFC3339Nano))
		fallbackConditions = append(fallbackConditions, "(u.account_id IN ("+strings.Join(apiPlaceholders, ",")+") AND u.created_at<?)")
	}
	if len(fallbackConditions) > 0 {
		condition += " OR (s.usage_source_id IS NULL AND (" + strings.Join(fallbackConditions, " OR ") + "))"
	}
	// New usage rows carry an immutable rule snapshot. Keep status/count queries
	// stable even when the current account rule is later edited or removed.
	condition += " OR (s.usage_source_id IS NULL AND u.rule_provider='subarx' AND u.rule_multiplier<>''"
	if len(apiIDs) > 0 {
		placeholders := make([]string, len(apiIDs))
		for i, accountID := range apiIDs {
			placeholders[i] = "?"
			args = append(args, accountID)
		}
		args = append(args, a.subarxAPICoverageStart(time.Now()).Format(time.RFC3339Nano))
		condition += " AND (u.account_id NOT IN (" + strings.Join(placeholders, ",") + ") OR u.created_at<?)"
	}
	condition += ")"
	return condition + ")", args
}

func (a *app) requests(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize == 0 {
		pageSize, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	window, err := queryTimeWindow(r, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "matched" && status != "unmatched" && status != "upstream_unmatched" {
		writeError(w, http.StatusBadRequest, "status must be all, matched, unmatched, or upstream_unmatched")
		return
	}
	var total int
	if err := a.recordCount(window.From, window.To, status).Scan(&total); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
		if page > totalPages {
			page = totalPages
		}
	}
	rows, err := a.profitRowsPage(window.From, window.To, status, pageSize, (page-1)*pageSize)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": rows, "page": page, "page_size": pageSize, "total": total, "total_pages": totalPages, "from": window.From, "to": window.To, "status": status})
}

func (a *app) recordCount(from, to, status string) *sql.Row {
	if status == "upstream_unmatched" {
		return a.db.QueryRow(`SELECT COUNT(*) FROM upstream_usage WHERE usage_source_id IS NULL AND match_method<>'`+a6StagingMatchMethod+`' AND occurred_at>=? AND occurred_at<?`, from, to)
	}
	if status == "all" {
		return a.db.QueryRow(`SELECT
(SELECT COUNT(*) FROM usage_snapshots WHERE created_at>=? AND created_at<?)+
(SELECT COUNT(*) FROM upstream_usage WHERE usage_source_id IS NULL AND match_method<>'`+a6StagingMatchMethod+`' AND occurred_at>=? AND occurred_at<?)`, from, to, from, to)
	}
	condition, args := a.downstreamMatchedCondition()
	if status == "unmatched" {
		condition = "NOT " + condition
	}
	query := `SELECT COUNT(*) FROM usage_snapshots u LEFT JOIN upstream_usage x ON x.usage_source_id=u.source_id
LEFT JOIN subarx_usage_scope_status s ON s.usage_source_id=u.source_id
WHERE u.created_at>=? AND u.created_at<? AND ` + condition
	return a.db.QueryRow(query, append([]any{from, to}, args...)...)
}

type timeWindow struct {
	From string
	To   string
}

func queryTimeWindow(r *http.Request, now time.Time) (timeWindow, error) {
	to := now.UTC()
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return timeWindow{}, errors.New("to must be an RFC3339 timestamp")
		}
		to = parsed.UTC()
	}
	from := to.Add(-24 * time.Hour)
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return timeWindow{}, errors.New("from must be an RFC3339 timestamp")
		}
		from = parsed.UTC()
	}
	if !from.Before(to) {
		return timeWindow{}, errors.New("from must be earlier than to")
	}
	return timeWindow{From: from.Format(time.RFC3339Nano), To: to.Format(time.RFC3339Nano)}, nil
}

func (a *app) importUpstream(w http.ResponseWriter, r *http.Request) {
	var records []upstreamRecord
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/csv") {
		parsed, err := parseCSV(r.Body)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		records = parsed
	} else if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20)).Decode(&records); err != nil {
		writeError(w, 400, "invalid JSON")
		return
	}
	if len(records) == 0 {
		writeError(w, 400, "no records")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	count := 0
	for _, rec := range records {
		if rec.Provider == "" {
			rec.Provider = "a6"
		}
		if rec.Currency == "" {
			rec.Currency = "USD"
		}
		if rec.Source == "" {
			rec.Source = "manual_import"
		}
		if rec.UpstreamRequestID == "" {
			tx.Rollback()
			writeError(w, 400, "upstream_request_id is required")
			return
		}
		originalCost, err := decimal.NewFromString(rec.Cost)
		if err != nil {
			tx.Rollback()
			writeError(w, 400, "invalid cost")
			return
		}
		fxRate := decimal.NewFromInt(1)
		if !strings.EqualFold(rec.Currency, "CNY") {
			if strings.TrimSpace(rec.FXRateToCNY) == "" {
				tx.Rollback()
				writeError(w, 400, "fx_rate_to_cny is required for non-CNY cost")
				return
			}
			fxRate, err = decimal.NewFromString(rec.FXRateToCNY)
			if err != nil || !fxRate.IsPositive() {
				tx.Rollback()
				writeError(w, 400, "invalid fx_rate_to_cny")
				return
			}
		} else {
			rec.FXRateToCNY = "1"
		}
		costCNY := originalCost.Mul(fxRate)
		if rec.OccurredAt == "" {
			rec.OccurredAt = time.Now().UTC().Format(time.RFC3339)
		} else if t, err := time.Parse(time.RFC3339, rec.OccurredAt); err == nil {
			rec.OccurredAt = t.UTC().Format(time.RFC3339Nano)
		} else {
			tx.Rollback()
			writeError(w, 400, "invalid occurred_at")
			return
		}
		_, err = tx.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,fx_rate_source,fx_rate_effective_at,fx_rate_fetched_at,occurred_at,model,input_tokens,output_tokens,source,imported_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(provider,upstream_request_id) DO UPDATE SET model=excluded.model,input_tokens=excluded.input_tokens,output_tokens=excluded.output_tokens,source=excluded.source,imported_at=excluded.imported_at`, rec.Provider, rec.UpstreamRequestID, rec.Cost, strings.ToUpper(rec.Currency), fxRate.String(), costCNY.String(), "manual_import", rec.OccurredAt, time.Now().UTC().Format(time.RFC3339Nano), rec.OccurredAt, rec.Model, rec.InputTokens, rec.OutputTokens, rec.Source, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			tx.Rollback()
			writeError(w, 500, err.Error())
			return
		}
		count++
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if err := a.reconcileUpstream(); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "imported": count})
}

func parseCSV(body io.Reader) ([]upstreamRecord, error) {
	r := csv.NewReader(body)
	headers, err := r.Read()
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i, h := range headers {
		index[strings.TrimSpace(strings.ToLower(h))] = i
	}
	required := []string{"upstream_request_id", "cost"}
	for _, h := range required {
		if _, ok := index[h]; !ok {
			return nil, fmt.Errorf("missing CSV column %s", h)
		}
	}
	var out []upstreamRecord
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		get := func(k string) string {
			i, ok := index[k]
			if !ok || i >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[i])
		}
		inTok, _ := strconv.ParseInt(get("input_tokens"), 10, 64)
		outTok, _ := strconv.ParseInt(get("output_tokens"), 10, 64)
		out = append(out, upstreamRecord{Provider: get("provider"), UpstreamRequestID: get("upstream_request_id"), Cost: get("cost"), Currency: get("currency"), FXRateToCNY: get("fx_rate_to_cny"), OccurredAt: get("occurred_at"), Model: get("model"), InputTokens: inTok, OutputTokens: outTok, Source: get("source")})
	}
	return out, nil
}

func (a *app) collectNow(w http.ResponseWriter, _ *http.Request) {
	if err := a.collect(true); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *app) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(u), []byte(a.cfg.AdminUser)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(p), []byte(a.cfg.AdminPassword)) == 1
		if !ok || !userOK || !passOK {
			w.Header().Set("WWW-Authenticate", `Basic realm="Sub2API Profit"`)
			writeError(w, 401, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"success": false, "message": msg})
}
