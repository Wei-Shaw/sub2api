package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	subarxProvider            = "subarx"
	subarxUsageSource         = "subarx_usage_api"
	subarxChargeDecimalPlaces = int32(5)
)

func (a *app) subarxAPICoverageStart(now time.Time) time.Time {
	lookbackDays := a.cfg.SubarxLookbackDays
	if lookbackDays < 1 {
		lookbackDays = 1
	}
	location, err := time.LoadLocation(a.cfg.SubarxTimezone)
	if err != nil {
		location = time.UTC
	}
	localNow := now.In(location)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	return start.AddDate(0, 0, -(lookbackDays - 1)).UTC()
}

func (a *app) subarxAPICovers(accountID int64, createdRaw string) bool {
	if strings.TrimSpace(a.cfg.SubarxAccountAPIKeys[accountID]) == "" {
		return false
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdRaw)
	if err != nil {
		return false
	}
	return !createdAt.Before(a.subarxAPICoverageStart(time.Now()))
}

const subarxDDL = `
CREATE TABLE IF NOT EXISTS subarx_daily_usage (
  account_id INTEGER NOT NULL,
  usage_date TEXT NOT NULL,
  model TEXT NOT NULL,
  requests INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens INTEGER NOT NULL DEFAULT 0,
  cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
  standard_cost TEXT NOT NULL DEFAULT '0',
  actual_cost TEXT NOT NULL DEFAULT '0',
  account_cost TEXT NOT NULL DEFAULT '0',
  currency TEXT NOT NULL DEFAULT 'USD',
  observed_at TEXT NOT NULL,
  source TEXT NOT NULL,
  PRIMARY KEY(account_id, usage_date, model)
);
CREATE INDEX IF NOT EXISTS subarx_daily_usage_date_idx
ON subarx_daily_usage(usage_date, account_id);
CREATE TABLE IF NOT EXISTS subarx_billing_snapshots (
  account_id INTEGER PRIMARY KEY,
  schema_version TEXT NOT NULL DEFAULT '',
  billing_scope TEXT NOT NULL DEFAULT '',
  group_rate_multiplier TEXT NOT NULL,
  resolved_rate_multiplier TEXT NOT NULL,
  effective_rate_multiplier TEXT NOT NULL,
  peak_rate_enabled INTEGER NOT NULL DEFAULT 0,
  observed_at TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  source TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS subarx_usage_scope_status (
  usage_source_id INTEGER PRIMARY KEY,
  account_id INTEGER NOT NULL,
  usage_date TEXT NOT NULL,
  model TEXT NOT NULL,
  allocation_status TEXT NOT NULL,
  observed_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS subarx_scope_date_idx
ON subarx_usage_scope_status(account_id, usage_date, model);`

type subarxDailyUsage struct {
	Date                string      `json:"date"`
	Requests            int64       `json:"requests"`
	InputTokens         int64       `json:"input_tokens"`
	OutputTokens        int64       `json:"output_tokens"`
	CacheReadTokens     int64       `json:"cache_read_tokens"`
	CacheCreationTokens int64       `json:"cache_write_tokens"`
	Cost                json.Number `json:"cost"`
	ActualCost          json.Number `json:"actual_cost"`
}

type subarxModelUsage struct {
	Model               string      `json:"model"`
	Requests            int64       `json:"requests"`
	InputTokens         int64       `json:"input_tokens"`
	OutputTokens        int64       `json:"output_tokens"`
	CacheReadTokens     int64       `json:"cache_read_tokens"`
	CacheCreationTokens int64       `json:"cache_creation_tokens"`
	Cost                json.Number `json:"cost"`
	ActualCost          json.Number `json:"actual_cost"`
	AccountCost         json.Number `json:"account_cost"`
}

type subarxUsageResponse struct {
	IsValid    bool               `json:"isValid"`
	Unit       string             `json:"unit"`
	DailyUsage []subarxDailyUsage `json:"daily_usage"`
	ModelStats []subarxModelUsage `json:"model_stats"`
}

type subarxBillingResponse struct {
	SchemaVersion           json.RawMessage `json:"schema_version"`
	BillingScope            string          `json:"billing_scope"`
	GroupRateMultiplier     json.Number     `json:"group_rate_multiplier"`
	ResolvedRateMultiplier  json.Number     `json:"resolved_rate_multiplier"`
	PeakRateEnabled         bool            `json:"peak_rate_enabled"`
	EffectiveRateMultiplier json.Number     `json:"effective_rate_multiplier"`
	ObservedAt              string          `json:"observed_at"`
}

type subarxLocalUsage struct {
	SourceID     int64
	OccurredAt   string
	InputTokens  int64
	OutputTokens int64
	CacheTokens  int64
	Weight       decimal.Decimal
}

func subarxCharge(value decimal.Decimal) decimal.Decimal {
	return value.Shift(subarxChargeDecimalPlaces).Ceil().Shift(-subarxChargeDecimalPlaces)
}

func loadSubarxConfig(c *config) error {
	c.SubarxBaseURL = strings.TrimRight(env("SUBARX_BASE_URL", "https://www.subarx.com"), "/")
	keys, err := parseSubarxAccountAPIKeys(osEnv("SUBARX_ACCOUNT_API_KEYS"))
	if err != nil {
		return err
	}
	c.SubarxAccountAPIKeys = keys
	c.SubarxSyncInterval, err = time.ParseDuration(env("SUBARX_SYNC_INTERVAL", "5m"))
	if err != nil || c.SubarxSyncInterval <= 0 {
		return errors.New("SUBARX_SYNC_INTERVAL must be a positive duration")
	}
	c.SubarxHTTPTimeout, err = time.ParseDuration(env("SUBARX_HTTP_TIMEOUT", "30s"))
	if err != nil || c.SubarxHTTPTimeout <= 0 {
		return errors.New("SUBARX_HTTP_TIMEOUT must be a positive duration")
	}
	c.SubarxLookbackDays, err = positiveIntEnv("SUBARX_LOOKBACK_DAYS", 90, 90)
	if err != nil {
		return err
	}
	c.SubarxTimezone = env("SUBARX_TIMEZONE", "Asia/Shanghai")
	if _, err := time.LoadLocation(c.SubarxTimezone); err != nil {
		return fmt.Errorf("invalid SUBARX_TIMEZONE: %w", err)
	}
	if len(keys) == 0 {
		return nil
	}
	parsedURL, err := url.Parse(c.SubarxBaseURL)
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return errors.New("SUBARX_BASE_URL must be an absolute HTTP(S) URL")
	}
	return nil
}

func parseSubarxAccountAPIKeys(raw string) (map[int64]string, error) {
	out := map[int64]string{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid SUBARX_ACCOUNT_API_KEYS entry for account mapping")
		}
		accountID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		apiKey := strings.TrimSpace(parts[1])
		if err != nil || accountID <= 0 || apiKey == "" {
			return nil, fmt.Errorf("invalid SUBARX_ACCOUNT_API_KEYS entry for account mapping")
		}
		if _, exists := out[accountID]; exists {
			return nil, fmt.Errorf("duplicate account %d in SUBARX_ACCOUNT_API_KEYS", accountID)
		}
		out[accountID] = apiKey
	}
	return out, nil
}

func parseSubarxAccountMultipliers(raw string) (map[int64]decimal.Decimal, error) {
	out := map[int64]decimal.Decimal{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid SUBARX_ACCOUNT_MULTIPLIERS entry %q", entry)
		}
		accountID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil || accountID <= 0 {
			return nil, fmt.Errorf("invalid account id in SUBARX_ACCOUNT_MULTIPLIERS entry %q", entry)
		}
		multiplier, err := decimal.NewFromString(strings.TrimSpace(parts[1]))
		if err != nil || !multiplier.IsPositive() {
			return nil, fmt.Errorf("invalid multiplier in SUBARX_ACCOUNT_MULTIPLIERS entry %q", entry)
		}
		out[accountID] = multiplier
	}
	return out, nil
}

func (a *app) subarxHTTPClient() *http.Client {
	a.httpMu.Lock()
	defer a.httpMu.Unlock()
	if a.subarxHTTP == nil {
		a.subarxHTTP = &http.Client{Timeout: a.cfg.SubarxHTTPTimeout}
	}
	return a.subarxHTTP
}

func (a *app) subarxGET(apiKey, path string, target any) error {
	req, err := http.NewRequest(http.MethodGet, a.cfg.SubarxBaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("User-Agent", "sub2api-companion/1.0")
	resp, err := a.subarxHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("Subarx GET %s: %w", pathName(path), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Subarx GET %s returned HTTP %d", pathName(path), resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 16<<20))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode Subarx %s: %w", pathName(path), err)
	}
	return nil
}

func (a *app) collectSubarx(force bool) error {
	if len(a.cfg.SubarxAccountAPIKeys) == 0 {
		return nil
	}
	accountIDs := make([]int64, 0, len(a.cfg.SubarxAccountAPIKeys))
	for accountID := range a.cfg.SubarxAccountAPIKeys {
		accountIDs = append(accountIDs, accountID)
	}
	sort.Slice(accountIDs, func(i, j int) bool { return accountIDs[i] < accountIDs[j] })
	var errs []error
	for _, accountID := range accountIDs {
		stateKey := fmt.Sprintf("subarx_last_attempt_unix:%d", accountID)
		if !force {
			lastAttempt := a.stateInt64(stateKey)
			if lastAttempt > 0 && time.Since(time.Unix(lastAttempt, 0)) < a.cfg.SubarxSyncInterval {
				continue
			}
		}
		_ = a.setState(stateKey, strconv.FormatInt(time.Now().Unix(), 10))
		if err := a.collectSubarxAccount(accountID, a.cfg.SubarxAccountAPIKeys[accountID]); err != nil {
			errs = append(errs, fmt.Errorf("Subarx account %d: %w", accountID, err))
			continue
		}
		_ = a.setState(fmt.Sprintf("subarx_last_success_unix:%d", accountID), strconv.FormatInt(time.Now().Unix(), 10))
	}
	return errors.Join(errs...)
}

func (a *app) collectSubarxAccount(accountID int64, apiKey string) error {
	var errs []error
	var billing subarxBillingResponse
	if err := a.subarxGET(apiKey, "/v1/sub2api/billing", &billing); err != nil {
		errs = append(errs, err)
	} else if err := a.storeSubarxBilling(accountID, billing); err != nil {
		errs = append(errs, err)
	}

	query := url.Values{}
	query.Set("days", strconv.Itoa(a.cfg.SubarxLookbackDays))
	query.Set("timezone", a.cfg.SubarxTimezone)
	var usage subarxUsageResponse
	if err := a.subarxGET(apiKey, "/v1/usage?"+query.Encode(), &usage); err != nil {
		errs = append(errs, err)
		return errors.Join(errs...)
	}
	if !usage.IsValid || !strings.EqualFold(usage.Unit, "USD") {
		errs = append(errs, errors.New("Subarx usage response is invalid or not denominated in USD"))
		return errors.Join(errs...)
	}
	location, _ := time.LoadLocation(a.cfg.SubarxTimezone)
	today := time.Now().In(location).Format("2006-01-02")
	yesterday := time.Now().In(location).AddDate(0, 0, -1).Format("2006-01-02")
	for _, daily := range usage.DailyUsage {
		if daily.Requests == 0 && decimalJSON(daily.Cost).IsZero() && decimalJSON(daily.ActualCost).IsZero() {
			continue
		}
		if _, err := time.Parse("2006-01-02", daily.Date); err != nil {
			errs = append(errs, fmt.Errorf("invalid usage date %q", daily.Date))
			continue
		}
		if daily.Date != today && daily.Date != yesterday && a.hasSubarxDailySnapshot(accountID, daily.Date) {
			continue
		}
		if err := a.collectSubarxDay(accountID, apiKey, daily); err != nil {
			errs = append(errs, fmt.Errorf("date %s: %w", daily.Date, err))
		}
	}
	return errors.Join(errs...)
}

func (a *app) collectSubarxDay(accountID int64, apiKey string, expected subarxDailyUsage) error {
	query := url.Values{}
	query.Set("start_date", expected.Date)
	query.Set("end_date", expected.Date)
	query.Set("days", "1")
	query.Set("timezone", a.cfg.SubarxTimezone)
	var usage subarxUsageResponse
	if err := a.subarxGET(apiKey, "/v1/usage?"+query.Encode(), &usage); err != nil {
		return err
	}
	if !usage.IsValid || !strings.EqualFold(usage.Unit, "USD") {
		return errors.New("daily usage response is invalid or not denominated in USD")
	}
	// Historical queries return the requested date in model_stats while
	// daily_usage remains the current dashboard day. For the active day, prefer
	// the matching row from this response so live traffic cannot stale the
	// broader snapshot used to discover dates.
	daily := expected
	for i := range usage.DailyUsage {
		if usage.DailyUsage[i].Date == expected.Date {
			daily = usage.DailyUsage[i]
			break
		}
	}
	if err := validateSubarxDay(daily, usage.ModelStats); err != nil {
		return err
	}
	return a.storeSubarxDay(accountID, daily, usage.ModelStats)
}

func validateSubarxDay(daily subarxDailyUsage, models []subarxModelUsage) error {
	var requests int64
	var inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int64
	standardCost := decimal.Zero
	actualCost := decimal.Zero
	for _, model := range models {
		if strings.TrimSpace(model.Model) == "" || model.Requests < 0 || model.InputTokens < 0 || model.OutputTokens < 0 || model.CacheReadTokens < 0 || model.CacheCreationTokens < 0 {
			return errors.New("daily model_stats contains an invalid model row")
		}
		requests += model.Requests
		inputTokens += model.InputTokens
		outputTokens += model.OutputTokens
		cacheReadTokens += model.CacheReadTokens
		cacheCreationTokens += model.CacheCreationTokens
		standardCost = standardCost.Add(decimalJSON(model.Cost))
		actualCost = actualCost.Add(decimalJSON(model.ActualCost))
	}
	if requests != daily.Requests {
		return fmt.Errorf("daily request total %d does not match model total %d", daily.Requests, requests)
	}
	if inputTokens != daily.InputTokens || outputTokens != daily.OutputTokens || cacheReadTokens != daily.CacheReadTokens || cacheCreationTokens != daily.CacheCreationTokens {
		return fmt.Errorf("daily token totals do not match model totals")
	}
	if !decimalClose(standardCost, decimalJSON(daily.Cost)) || !decimalClose(actualCost, decimalJSON(daily.ActualCost)) {
		return errors.New("daily cost totals do not match model totals")
	}
	return nil
}

func (a *app) storeSubarxBilling(accountID int64, billing subarxBillingResponse) error {
	group := decimalJSON(billing.GroupRateMultiplier)
	resolved := decimalJSON(billing.ResolvedRateMultiplier)
	effective := decimalJSON(billing.EffectiveRateMultiplier)
	if !effective.IsPositive() || group.IsNegative() || resolved.IsNegative() {
		return errors.New("Subarx billing response contains an invalid multiplier")
	}
	observedAt := strings.TrimSpace(billing.ObservedAt)
	if observedAt == "" {
		observedAt = time.Now().UTC().Format(time.RFC3339Nano)
	} else if parsed, err := time.Parse(time.RFC3339Nano, observedAt); err == nil {
		observedAt = parsed.UTC().Format(time.RFC3339Nano)
	}
	schemaVersion := strings.Trim(strings.TrimSpace(string(billing.SchemaVersion)), "\"")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO subarx_billing_snapshots(account_id,schema_version,billing_scope,group_rate_multiplier,resolved_rate_multiplier,effective_rate_multiplier,peak_rate_enabled,observed_at,fetched_at,source)
VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(account_id) DO UPDATE SET schema_version=excluded.schema_version,billing_scope=excluded.billing_scope,group_rate_multiplier=excluded.group_rate_multiplier,resolved_rate_multiplier=excluded.resolved_rate_multiplier,effective_rate_multiplier=excluded.effective_rate_multiplier,peak_rate_enabled=excluded.peak_rate_enabled,observed_at=excluded.observed_at,fetched_at=excluded.fetched_at,source=excluded.source`, accountID, schemaVersion, billing.BillingScope, group.String(), resolved.String(), effective.String(), boolInt(billing.PeakRateEnabled), observedAt, now, subarxUsageSource); err != nil {
		tx.Rollback()
		return err
	}
	if _, err = tx.Exec(`INSERT INTO upstream_account_rules(account_id,provider,external_key,multiplier,version,enabled,created_at,updated_at)
VALUES(?,?,'',?,1,1,?,?) ON CONFLICT(account_id) DO UPDATE SET multiplier=excluded.multiplier,version=upstream_account_rules.version+1,updated_at=excluded.updated_at WHERE upstream_account_rules.provider='subarx'`, accountID, subarxProvider, effective.String(), now, now); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return a.reloadAccountRules()
}

func (a *app) storeSubarxDay(accountID int64, daily subarxDailyUsage, models []subarxModelUsage) error {
	location, err := time.LoadLocation(a.cfg.SubarxTimezone)
	if err != nil {
		return err
	}
	day, err := time.ParseInLocation("2006-01-02", daily.Date, location)
	if err != nil {
		return err
	}
	from := day.UTC().Format(time.RFC3339Nano)
	to := day.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	rollback := func(err error) error {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM upstream_usage WHERE provider=? AND source=? AND account_id_hint=? AND billing_date=? AND usage_source_id IS NULL`, subarxProvider, subarxUsageSource, accountID, daily.Date); err != nil {
		return rollback(err)
	}
	if _, err := tx.Exec(`DELETE FROM subarx_usage_scope_status WHERE account_id=? AND usage_date=?`, accountID, daily.Date); err != nil {
		return rollback(err)
	}
	if _, err := tx.Exec(`DELETE FROM subarx_daily_usage WHERE account_id=? AND usage_date=?`, accountID, daily.Date); err != nil {
		return rollback(err)
	}
	seenModels := map[string]bool{}
	for _, model := range models {
		seenModels[model.Model] = true
		if _, err := tx.Exec(`INSERT INTO subarx_daily_usage(account_id,usage_date,model,requests,input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,standard_cost,actual_cost,account_cost,currency,observed_at,source) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, accountID, daily.Date, model.Model, model.Requests, model.InputTokens, model.OutputTokens, model.CacheReadTokens, model.CacheCreationTokens, decimalJSON(model.Cost).String(), decimalJSON(model.ActualCost).String(), decimalJSON(model.AccountCost).String(), "USD", now, subarxUsageSource); err != nil {
			return rollback(err)
		}
		if err := allocateSubarxModel(tx, accountID, daily.Date, from, to, model, now); err != nil {
			return rollback(err)
		}
	}
	rows, err := tx.Query(`SELECT source_id,model FROM usage_snapshots WHERE account_id=? AND created_at>=? AND created_at<? ORDER BY source_id`, accountID, from, to)
	if err != nil {
		return rollback(err)
	}
	for rows.Next() {
		var sourceID int64
		var model string
		if err := rows.Scan(&sourceID, &model); err != nil {
			rows.Close()
			return rollback(err)
		}
		if seenModels[model] {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO subarx_usage_scope_status(usage_source_id,account_id,usage_date,model,allocation_status,observed_at) VALUES(?,?,?,?,?,?)`, sourceID, accountID, daily.Date, model, "missing_upstream_model", now); err != nil {
			rows.Close()
			return rollback(err)
		}
	}
	if err := rows.Close(); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

func allocateSubarxModel(tx *sql.Tx, accountID int64, usageDate, from, to string, model subarxModelUsage, observedAt string) error {
	rows, err := tx.Query(`SELECT source_id,created_at,input_tokens,output_tokens,(cache_read_tokens+cache_creation_tokens),input_cost,output_cost,cache_read_cost,cache_creation_cost
FROM usage_snapshots WHERE account_id=? AND model=? AND created_at>=? AND created_at<? ORDER BY source_id`, accountID, model.Model, from, to)
	if err != nil {
		return err
	}
	locals := make([]subarxLocalUsage, 0)
	totalWeight := decimal.Zero
	for rows.Next() {
		var local subarxLocalUsage
		var inputCost, outputCost, cacheReadCost, cacheCreateCost string
		if err := rows.Scan(&local.SourceID, &local.OccurredAt, &local.InputTokens, &local.OutputTokens, &local.CacheTokens, &inputCost, &outputCost, &cacheReadCost, &cacheCreateCost); err != nil {
			rows.Close()
			return err
		}
		for _, raw := range []string{inputCost, outputCost, cacheReadCost, cacheCreateCost} {
			value, err := decimal.NewFromString(raw)
			if err != nil {
				rows.Close()
				return fmt.Errorf("invalid standard cost for usage %d", local.SourceID)
			}
			local.Weight = local.Weight.Add(value)
		}
		totalWeight = totalWeight.Add(local.Weight)
		locals = append(locals, local)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	standardCost := decimalJSON(model.Cost)
	actualCost := decimalJSON(model.ActualCost)
	status := "allocated_full"
	allocatedTotal := decimal.Zero
	switch {
	case len(locals) == 0:
		status = "upstream_only"
	case int64(len(locals)) > model.Requests:
		status = "local_requests_exceed_upstream"
	case int64(len(locals)) == model.Requests:
		allocatedTotal = actualCost
	case !standardCost.IsPositive():
		status = "partial_without_standard_cost"
	case totalWeight.GreaterThan(standardCost.Add(decimalTolerance(standardCost))):
		status = "local_standard_cost_exceeds_upstream"
	default:
		status = "allocated_partial"
		allocatedTotal = actualCost.Mul(totalWeight).Div(standardCost)
	}
	if allocatedTotal.GreaterThan(actualCost) {
		allocatedTotal = actualCost
	}

	scope := subarxScope(accountID, usageDate, model.Model)
	allocations := distributeSubarxCost(locals, totalWeight, allocatedTotal)
	for i, local := range locals {
		if _, err := tx.Exec(`INSERT INTO subarx_usage_scope_status(usage_source_id,account_id,usage_date,model,allocation_status,observed_at) VALUES(?,?,?,?,?,?)`, local.SourceID, accountID, usageDate, model.Model, status, observedAt); err != nil {
			return err
		}
		if allocatedTotal.IsZero() && actualCost.IsPositive() {
			continue
		}
		details, _ := json.Marshal(map[string]any{"scope": scope, "status": status, "upstream_requests": model.Requests, "local_requests": len(locals), "upstream_standard_cost": standardCost.String(), "local_standard_cost": totalWeight.String()})
		requestID := fmt.Sprintf("%s:%d", scope, local.SourceID)
		if _, err := tx.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,fx_rate_source,fx_rate_effective_at,fx_rate_fetched_at,fx_rate_stale,occurred_at,model,input_tokens,output_tokens,cache_tokens,account_id_hint,billing_details,usage_source_id,match_method,allocation_scope,billing_date,source,imported_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(provider,upstream_request_id) DO NOTHING`, subarxProvider, requestID, allocations[i].String(), "USD", "1", allocations[i].String(), "subarx_native", usageDate, observedAt, 0, local.OccurredAt, model.Model, local.InputTokens, local.OutputTokens, local.CacheTokens, accountID, string(details), local.SourceID, "subarx_daily_model_allocation", scope, usageDate, subarxUsageSource, observedAt); err != nil {
			return err
		}
	}

	unallocated := actualCost.Sub(allocatedTotal)
	if unallocated.IsNegative() || decimalClose(unallocated, decimal.Zero) {
		unallocated = decimal.Zero
	}
	if unallocated.IsPositive() {
		details, _ := json.Marshal(map[string]any{"scope": scope, "status": status, "upstream_requests": model.Requests, "local_requests": len(locals), "upstream_standard_cost": standardCost.String(), "local_standard_cost": totalWeight.String(), "unallocated_cost": unallocated.String()})
		if _, err := tx.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,fx_rate_source,fx_rate_effective_at,fx_rate_fetched_at,fx_rate_stale,occurred_at,model,input_tokens,output_tokens,cache_tokens,account_id_hint,billing_details,match_method,allocation_scope,billing_date,source,imported_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, subarxProvider, scope+":unallocated", unallocated.String(), "USD", "1", unallocated.String(), "subarx_native", usageDate, observedAt, 0, from, model.Model, model.InputTokens, model.OutputTokens, model.CacheReadTokens+model.CacheCreationTokens, accountID, string(details), "subarx_daily_model_unallocated", scope, usageDate, subarxUsageSource, observedAt); err != nil {
			return err
		}
	}
	return nil
}

func distributeSubarxCost(locals []subarxLocalUsage, totalWeight, total decimal.Decimal) []decimal.Decimal {
	out := make([]decimal.Decimal, len(locals))
	if len(locals) == 0 || total.IsZero() {
		return out
	}
	allocated := decimal.Zero
	for i, local := range locals {
		if i == len(locals)-1 {
			out[i] = total.Sub(allocated)
			break
		}
		if totalWeight.IsPositive() {
			out[i] = total.Mul(local.Weight).Div(totalWeight)
		} else {
			out[i] = total.Div(decimal.NewFromInt(int64(len(locals))))
		}
		allocated = allocated.Add(out[i])
	}
	return out
}

func subarxScope(accountID int64, usageDate, model string) string {
	encodedModel := base64.RawURLEncoding.EncodeToString([]byte(model))
	return fmt.Sprintf("subarx-daily:%d:%s:%s", accountID, usageDate, encodedModel)
}

func (a *app) hasSubarxDailySnapshot(accountID int64, usageDate string) bool {
	var exists bool
	_ = a.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM subarx_daily_usage WHERE account_id=? AND usage_date=?)`, accountID, usageDate).Scan(&exists)
	if !exists {
		return false
	}
	var unresolved bool
	_ = a.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM subarx_usage_scope_status WHERE account_id=? AND usage_date=? AND allocation_status IN ('local_requests_exceed_upstream','local_standard_cost_exceeds_upstream','partial_without_standard_cost','missing_upstream_model'))`, accountID, usageDate).Scan(&unresolved)
	return !unresolved
}

func decimalJSON(value json.Number) decimal.Decimal {
	raw := strings.TrimSpace(value.String())
	if raw == "" {
		return decimal.Zero
	}
	parsed, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Zero
	}
	return parsed
}

func decimalTolerance(value decimal.Decimal) decimal.Decimal {
	relative := value.Abs().Mul(decimal.RequireFromString("0.000001"))
	minimum := decimal.RequireFromString("0.000000001")
	if relative.LessThan(minimum) {
		return minimum
	}
	return relative
}

func decimalClose(left, right decimal.Decimal) bool {
	diff := left.Sub(right).Abs()
	base := left.Abs()
	if right.Abs().GreaterThan(base) {
		base = right.Abs()
	}
	return diff.LessThanOrEqual(decimalTolerance(base))
}
