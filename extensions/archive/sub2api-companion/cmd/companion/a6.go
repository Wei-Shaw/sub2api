package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const a6Provider = "a6"

type a6LogItem struct {
	CreatedAt        int64           `json:"created_at"`
	RequestID        string          `json:"request_id"`
	TokenName        string          `json:"token_name"`
	Group            string          `json:"group"`
	ModelName        string          `json:"model_name"`
	PromptTokens     int64           `json:"prompt_tokens"`
	CompletionTokens int64           `json:"completion_tokens"`
	Quota            json.Number     `json:"quota"`
	Channel          int64           `json:"channel"`
	ChannelName      string          `json:"channel_name"`
	Other            json.RawMessage `json:"other"`
}

type a6BillingDetails struct {
	CacheTokens     int64       `json:"cache_tokens"`
	ModelRatio      json.Number `json:"model_ratio"`
	CacheRatio      json.Number `json:"cache_ratio"`
	CompletionRatio json.Number `json:"completion_ratio"`
	GroupRatio      json.Number `json:"group_ratio"`
	BillingSource   string      `json:"billing_source"`
	RequestPath     string      `json:"request_path"`
}

type a6LogPage struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Page     int         `json:"page"`
		PageSize int         `json:"page_size"`
		Total    int         `json:"total"`
		Items    []a6LogItem `json:"items"`
	} `json:"data"`
}

func loadA6Config(c *config) error {
	c.A6BaseURL = strings.TrimRight(env("A6_BASE_URL", "https://a6api.com"), "/")
	c.A6UserID = strings.TrimSpace(osEnv("A6_USER_ID"))
	c.A6AccessToken = strings.TrimSpace(osEnv("A6_ACCESS_TOKEN"))
	c.A6TokenAccountMap = map[string]int64{}
	if c.A6AccessToken == "" {
		return nil
	}
	parsedURL, err := url.Parse(c.A6BaseURL)
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return errors.New("A6_BASE_URL must be an absolute HTTP(S) URL")
	}
	userID, err := strconv.ParseInt(c.A6UserID, 10, 64)
	if err != nil || userID <= 0 {
		return errors.New("A6_USER_ID must be a positive integer when A6_ACCESS_TOKEN is set")
	}
	c.A6TokenAccountMap, err = parseA6TokenAccountMap(osEnv("A6_TOKEN_ACCOUNT_MAP"))
	if err != nil {
		return err
	}
	if len(c.A6TokenAccountMap) == 0 {
		return errors.New("A6_TOKEN_ACCOUNT_MAP is required when A6_ACCESS_TOKEN is set")
	}
	c.A6USDToCNY, err = decimal.NewFromString(strings.TrimSpace(osEnv("A6_USD_TO_CNY_RATE")))
	if err != nil || !c.A6USDToCNY.IsPositive() {
		return errors.New("A6_USD_TO_CNY_RATE must be a positive decimal when A6_ACCESS_TOKEN is set")
	}
	c.FXPrimaryURL = env("FX_PRIMARY_URL", "https://api.coinbase.com/v2/exchange-rates?currency=USD")
	c.FXFallbackURL = env("FX_FALLBACK_URL", "https://open.er-api.com/v6/latest/USD")
	c.FXRefreshInterval, err = time.ParseDuration(env("FX_REFRESH_INTERVAL", "1h"))
	if err != nil || c.FXRefreshInterval <= 0 {
		return errors.New("FX_REFRESH_INTERVAL must be a positive duration")
	}
	c.FXHTTPTimeout, err = time.ParseDuration(env("FX_HTTP_TIMEOUT", "15s"))
	if err != nil || c.FXHTTPTimeout <= 0 {
		return errors.New("FX_HTTP_TIMEOUT must be a positive duration")
	}
	c.FXMaxStaleness, err = time.ParseDuration(env("FX_MAX_STALENESS", "6h"))
	if err != nil || c.FXMaxStaleness <= 0 {
		return errors.New("FX_MAX_STALENESS must be a positive duration")
	}
	c.A6BootstrapWindow, err = time.ParseDuration(env("A6_BOOTSTRAP_LOOKBACK", "24h"))
	if err != nil || c.A6BootstrapWindow <= 0 {
		return errors.New("A6_BOOTSTRAP_LOOKBACK must be a positive duration")
	}
	c.A6HistoricalTokenLookback, err = time.ParseDuration(env("A6_HISTORICAL_TOKEN_LOOKBACK", "48h"))
	if err != nil || c.A6HistoricalTokenLookback <= 0 {
		return errors.New("A6_HISTORICAL_TOKEN_LOOKBACK must be a positive duration")
	}
	c.A6SyncInterval, err = time.ParseDuration(env("A6_SYNC_INTERVAL", "5m"))
	if err != nil || c.A6SyncInterval <= 0 {
		return errors.New("A6_SYNC_INTERVAL must be a positive duration")
	}
	c.A6HTTPTimeout, err = time.ParseDuration(env("A6_HTTP_TIMEOUT", "90s"))
	if err != nil || c.A6HTTPTimeout <= 0 {
		return errors.New("A6_HTTP_TIMEOUT must be a positive duration")
	}
	c.A6PageSize, err = positiveIntEnv("A6_PAGE_SIZE", 100, 100)
	if err != nil {
		return err
	}
	c.A6MaxPages, err = positiveIntEnv("A6_MAX_PAGES", 50, 500)
	if err != nil {
		return err
	}
	c.A6BackfillChunk, err = time.ParseDuration(env("A6_BACKFILL_CHUNK", "24h"))
	if err != nil || c.A6BackfillChunk <= 0 {
		return errors.New("A6_BACKFILL_CHUNK must be a positive duration")
	}
	c.A6BackfillPageDelay, err = time.ParseDuration(env("A6_BACKFILL_PAGE_DELAY", "200ms"))
	if err != nil || c.A6BackfillPageDelay < 0 {
		return errors.New("A6_BACKFILL_PAGE_DELAY must be a non-negative duration")
	}
	return nil
}

func osEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func positiveIntEnv(key string, fallback, maximum int) (int, error) {
	raw := env(key, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 || value > maximum {
		return 0, fmt.Errorf("%s must be between 1 and %d", key, maximum)
	}
	return value, nil
}

func parseA6TokenAccountMap(raw string) (map[string]int64, error) {
	out := map[string]int64{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid A6_TOKEN_ACCOUNT_MAP entry %q", entry)
		}
		accountID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || accountID <= 0 {
			return nil, fmt.Errorf("invalid account id in A6_TOKEN_ACCOUNT_MAP entry %q", entry)
		}
		out[strings.TrimSpace(parts[0])] = accountID
	}
	return out, nil
}

func (a *app) a6HTTPClient() *http.Client {
	a.httpMu.Lock()
	defer a.httpMu.Unlock()
	if a.http == nil {
		a.http = &http.Client{Timeout: a.cfg.A6HTTPTimeout}
	}
	return a.http
}

func (a *app) a6GET(path string, target any) error {
	req, err := http.NewRequest(http.MethodGet, a.cfg.A6BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.A6AccessToken)
	req.Header.Set("New-API-User", a.cfg.A6UserID)
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("User-Agent", "sub2api-companion/1.0")
	resp, err := a.a6HTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("A6 GET %s: %w", pathName(path), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("A6 GET %s returned HTTP %d", pathName(path), resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 16<<20))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode A6 %s: %w", pathName(path), err)
	}
	return nil
}

func pathName(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[:i]
	}
	return path
}

func (a *app) collectA6(force bool) error {
	if a.cfg.A6AccessToken == "" {
		return nil
	}
	// Keep polling token names captured by historical rule snapshots as well as
	// currently configured names. A rename must not strand bills that arrive
	// after the account rule changed.
	if err := a.loadA6HistoricalTokenAccounts(); err != nil {
		return err
	}
	if !force {
		lastSync := a.stateInt64("a6_last_sync_unix")
		if lastSync > 0 && time.Since(time.Unix(lastSync, 0)) < a.cfg.A6SyncInterval {
			return nil
		}
	}
	quotaPerUnit, err := a.a6QuotaPerUnit()
	if err != nil {
		a.recordA6SyncError(err)
		return err
	}
	tokenAccounts := a.a6TokenAccountsSnapshot()
	historicalOnly := make(map[string]bool)
	a.rulesMu.RLock()
	for token, accountIDs := range a.a6HistoricalTokenAccounts {
		if _, exists := tokenAccounts[token]; exists {
			continue
		}
		tokenAccounts[token] = append([]int64(nil), accountIDs...)
		historicalOnly[token] = true
	}
	a.rulesMu.RUnlock()
	tokens := make([]string, 0, len(tokenAccounts))
	for token := range tokenAccounts {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	var tokenErrors []error
	for _, token := range tokens {
		accountID := int64(0)
		if len(tokenAccounts[token]) == 1 {
			accountID = tokenAccounts[token][0]
		}
		if err := a.collectA6Token(token, accountID, quotaPerUnit, historicalOnly[token]); err != nil {
			tokenErrors = append(tokenErrors, fmt.Errorf("token %q: %w", token, err))
		}
	}
	if err := a.setState("a6_last_sync_unix", strconv.FormatInt(time.Now().Unix(), 10)); err != nil {
		return err
	}
	if len(tokenErrors) > 0 {
		err := errors.Join(tokenErrors...)
		a.recordA6SyncError(err)
		return err
	}
	return a.setState("a6_last_sync_error", "")
}

func (a *app) recordA6SyncError(err error) {
	if err == nil {
		return
	}
	_ = a.setState("a6_last_sync_error", err.Error())
	_ = a.setState("a6_last_sync_error_at", time.Now().UTC().Format(time.RFC3339Nano))
}

func (a *app) a6QuotaPerUnit() (decimal.Decimal, error) {
	var status struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			QuotaPerUnit json.Number `json:"quota_per_unit"`
		} `json:"data"`
	}
	if err := a.a6GET("/api/status", &status); err != nil {
		return decimal.Zero, err
	}
	if !status.Success {
		return decimal.Zero, fmt.Errorf("A6 status rejected request: %s", status.Message)
	}
	quotaPerUnit, err := decimal.NewFromString(status.Data.QuotaPerUnit.String())
	if err != nil || !quotaPerUnit.IsPositive() {
		return decimal.Zero, errors.New("A6 returned an invalid quota_per_unit")
	}
	return quotaPerUnit, nil
}

func (a *app) sortedA6Tokens() []string {
	rules := a.sortedA6AccountRules()
	tokens := make([]string, 0, len(rules))
	for _, rule := range rules {
		tokens = append(tokens, rule.ExternalKey)
	}
	return tokens
}

func (a *app) collectA6Token(tokenName string, accountID int64, quotaPerUnit decimal.Decimal, historicalOnly bool) error {
	lookback := a.cfg.A6BootstrapWindow
	if historicalOnly && a.cfg.A6HistoricalTokenLookback > 0 {
		lookback = a.cfg.A6HistoricalTokenLookback
	}
	now := time.Now().UTC()
	cutoff := now.Add(-lookback)
	bootstrapKey := "a6_bootstrap_done:" + tokenName
	bootstrapDone := a.stateString(bootstrapKey) == "1"
	reachedBoundary := false
	for page := 1; page <= a.cfg.A6MaxPages; page++ {
		query := url.Values{
			"p":               {strconv.Itoa(page)},
			"page_size":       {strconv.Itoa(a.cfg.A6PageSize)},
			"type":            {"2"},
			"token_name":      {tokenName},
			"model_name":      {""},
			"start_timestamp": {strconv.FormatInt(cutoff.Unix(), 10)},
			"end_timestamp":   {strconv.FormatInt(now.Add(time.Minute).Unix(), 10)},
			"group":           {""},
			"request_id":      {""},
		}
		var response a6LogPage
		if err := a.a6GET("/api/log/self?"+query.Encode(), &response); err != nil {
			return err
		}
		if !response.Success {
			return fmt.Errorf("A6 log rejected request: %s", response.Message)
		}
		if len(response.Data.Items) == 0 {
			reachedBoundary = true
			break
		}
		knownSeen := false
		oldSeen := false
		for _, item := range response.Data.Items {
			if item.RequestID == "" || item.TokenName != tokenName {
				continue
			}
			occurredAt := time.Unix(item.CreatedAt, 0).UTC()
			if occurredAt.Before(cutoff) {
				oldSeen = true
				continue
			}
			known, err := a.hasUpstreamRecord(a6Provider, item.RequestID)
			if err != nil {
				return err
			}
			if known {
				knownSeen = true
				continue
			}
			if err := a.storeA6Item(item, accountID, quotaPerUnit, occurredAt); err != nil {
				return err
			}
		}
		if (bootstrapDone && knownSeen) || oldSeen || len(response.Data.Items) < a.cfg.A6PageSize {
			reachedBoundary = true
			break
		}
	}
	if !reachedBoundary {
		return fmt.Errorf("A6 bootstrap for token %q exceeded A6_MAX_PAGES", tokenName)
	}
	if !bootstrapDone {
		if err := a.setState(bootstrapKey, "1"); err != nil {
			return err
		}
	}
	return nil
}

const (
	a6BackfillStatusKey    = "a6_backfill_status"
	a6BackfillFromKey      = "a6_backfill_from"
	a6BackfillToKey        = "a6_backfill_to"
	a6BackfillCursorKey    = "a6_backfill_cursor"
	a6BackfillProcessedKey = "a6_backfill_processed"
	a6BackfillErrorKey     = "a6_backfill_error"
)

type a6BackfillRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func parseBackfillTime(raw string, endOfDay bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		if endOfDay {
			return t.Add(24*time.Hour - time.Nanosecond).UTC(), nil
		}
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q", raw)
}

func (a *app) earliestA6UsageTime() (time.Time, error) {
	rules := a.sortedA6AccountRules()
	accounts := make([]int64, 0, len(rules))
	seen := map[int64]bool{}
	for _, rule := range rules {
		accountID := rule.AccountID
		if !seen[accountID] {
			accounts = append(accounts, accountID)
			seen[accountID] = true
		}
	}
	if len(accounts) == 0 {
		return time.Time{}, errors.New("A6 account mapping is empty")
	}
	placeholders := make([]string, len(accounts))
	args := make([]any, len(accounts))
	for i, accountID := range accounts {
		placeholders[i] = "?"
		args[i] = accountID
	}
	var raw string
	err := a.db.QueryRow(`SELECT COALESCE(MIN(created_at),'') FROM usage_snapshots WHERE account_id IN (`+strings.Join(placeholders, ",")+`)`, args...).Scan(&raw)
	if err != nil {
		return time.Time{}, err
	}
	if raw == "" {
		return time.Time{}, errors.New("no A6 usage snapshots found")
	}
	return time.Parse(time.RFC3339Nano, raw)
}

func (a *app) startA6Backfill(w http.ResponseWriter, r *http.Request) {
	if a.cfg.A6AccessToken == "" {
		writeError(w, http.StatusServiceUnavailable, "A6 synchronization is not configured")
		return
	}
	var request a6BackfillRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
	}
	from, err := parseBackfillTime(request.From, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if from.IsZero() {
		from, err = a.earliestA6UsageTime()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	to, err := parseBackfillTime(request.To, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if to.IsZero() || to.After(time.Now().UTC()) {
		to = time.Now().UTC()
	}
	if !from.Before(to) {
		writeError(w, http.StatusBadRequest, "from must be earlier than to")
		return
	}

	a.backfillMu.Lock()
	if a.backfillRunning {
		a.backfillMu.Unlock()
		writeError(w, http.StatusConflict, "A6 backfill is already running")
		return
	}
	a.backfillRunning = true
	a.backfillMu.Unlock()
	if err := a.setStates(map[string]string{
		a6BackfillStatusKey: "running", a6BackfillFromKey: from.Format(time.RFC3339Nano),
		a6BackfillToKey: to.Format(time.RFC3339Nano), a6BackfillCursorKey: from.Format(time.RFC3339Nano),
		a6BackfillProcessedKey: "0", a6BackfillErrorKey: "",
	}); err != nil {
		a.backfillMu.Lock()
		a.backfillRunning = false
		a.backfillMu.Unlock()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	go a.runA6Backfill(from, to, 0)
	writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339)})
}

func (a *app) a6BackfillStatus(w http.ResponseWriter, _ *http.Request) {
	a.backfillMu.Lock()
	running := a.backfillRunning
	a.backfillMu.Unlock()
	processed, _ := strconv.Atoi(a.stateString(a6BackfillProcessedKey))
	writeJSON(w, http.StatusOK, map[string]any{
		"status": a.stateString(a6BackfillStatusKey), "running": running,
		"from": a.stateString(a6BackfillFromKey), "to": a.stateString(a6BackfillToKey),
		"cursor": a.stateString(a6BackfillCursorKey), "processed": processed,
		"error": a.stateString(a6BackfillErrorKey),
	})
}

func (a *app) resumeA6Backfill() {
	if a.cfg.A6AccessToken == "" || a.stateString(a6BackfillStatusKey) != "running" {
		return
	}
	cursor, err := time.Parse(time.RFC3339Nano, a.stateString(a6BackfillCursorKey))
	if err != nil {
		_ = a.setStates(map[string]string{a6BackfillStatusKey: "failed", a6BackfillErrorKey: "invalid persisted cursor"})
		return
	}
	to, err := time.Parse(time.RFC3339Nano, a.stateString(a6BackfillToKey))
	if err != nil {
		_ = a.setStates(map[string]string{a6BackfillStatusKey: "failed", a6BackfillErrorKey: "invalid persisted end time"})
		return
	}
	processed, _ := strconv.Atoi(a.stateString(a6BackfillProcessedKey))
	a.backfillMu.Lock()
	a.backfillRunning = true
	a.backfillMu.Unlock()
	go a.runA6Backfill(cursor, to, processed)
}

func (a *app) runA6Backfill(cursor, to time.Time, processed int) {
	defer func() {
		a.backfillMu.Lock()
		a.backfillRunning = false
		a.backfillMu.Unlock()
	}()
	quotaPerUnit, err := a.a6QuotaPerUnitWithRetry()
	if err != nil {
		a.failA6Backfill(err)
		return
	}
	for cursor.Before(to) {
		chunkEnd := cursor.Add(a.cfg.A6BackfillChunk)
		if chunkEnd.After(to) {
			chunkEnd = to
		}
		chunkProcessed := 0
		a.mu.Lock()
		var collectErr error
		tokenAccounts := a.a6TokenAccountsSnapshot()
		tokens := make([]string, 0, len(tokenAccounts))
		for token := range tokenAccounts {
			tokens = append(tokens, token)
		}
		sort.Strings(tokens)
		for _, token := range tokens {
			accountID := int64(0)
			if len(tokenAccounts[token]) == 1 {
				accountID = tokenAccounts[token][0]
			}
			var count int
			count, collectErr = a.collectA6RangeAdaptive(token, accountID, quotaPerUnit, cursor, chunkEnd)
			if collectErr != nil {
				break
			}
			chunkProcessed += count
		}
		if collectErr == nil {
			collectErr = a.reconcileUpstream()
		}
		a.mu.Unlock()
		if collectErr != nil {
			a.failA6Backfill(collectErr)
			return
		}
		processed += chunkProcessed
		cursor = chunkEnd
		if err := a.setStates(map[string]string{a6BackfillCursorKey: cursor.Format(time.RFC3339Nano), a6BackfillProcessedKey: strconv.Itoa(processed)}); err != nil {
			a.failA6Backfill(err)
			return
		}
	}
	_ = a.setStates(map[string]string{a6BackfillStatusKey: "completed", a6BackfillCursorKey: to.Format(time.RFC3339Nano), a6BackfillProcessedKey: strconv.Itoa(processed), a6BackfillErrorKey: ""})
}

func (a *app) failA6Backfill(err error) {
	logMessage := err.Error()
	if len(logMessage) > 500 {
		logMessage = logMessage[:500]
	}
	_ = a.setStates(map[string]string{a6BackfillStatusKey: "failed", a6BackfillErrorKey: logMessage})
}

func (a *app) setStates(values map[string]string) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	for key, value := range values {
		if _, err := tx.Exec(`INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (a *app) a6QuotaPerUnitWithRetry() (decimal.Decimal, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		value, err := a.a6QuotaPerUnit()
		if err == nil {
			return value, nil
		}
		lastErr = err
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}
	return decimal.Zero, lastErr
}

func (a *app) collectA6RangeAdaptive(tokenName string, accountID int64, quotaPerUnit decimal.Decimal, start, end time.Time) (int, error) {
	first, err := a.fetchA6RangePage(tokenName, start, end, 1)
	if err != nil {
		return 0, err
	}
	capacity := a.cfg.A6PageSize * a.cfg.A6MaxPages
	if first.Data.Total > capacity {
		if end.Sub(start) <= time.Minute {
			return 0, fmt.Errorf("A6 range %s to %s has %d rows, exceeding page capacity", start.Format(time.RFC3339), end.Format(time.RFC3339), first.Data.Total)
		}
		mid := start.Add(end.Sub(start) / 2)
		left, err := a.collectA6RangeAdaptive(tokenName, accountID, quotaPerUnit, start, mid)
		if err != nil {
			return 0, err
		}
		right, err := a.collectA6RangeAdaptive(tokenName, accountID, quotaPerUnit, mid, end)
		return left + right, err
	}
	processed, err := a.storeA6RangeItems(first.Data.Items, tokenName, accountID, quotaPerUnit, start, end)
	if err != nil {
		return 0, err
	}
	pages := (first.Data.Total + a.cfg.A6PageSize - 1) / a.cfg.A6PageSize
	for page := 2; page <= pages; page++ {
		if a.cfg.A6BackfillPageDelay > 0 {
			time.Sleep(a.cfg.A6BackfillPageDelay)
		}
		response, err := a.fetchA6RangePage(tokenName, start, end, page)
		if err != nil {
			return processed, err
		}
		count, err := a.storeA6RangeItems(response.Data.Items, tokenName, accountID, quotaPerUnit, start, end)
		if err != nil {
			return processed, err
		}
		processed += count
	}
	return processed, nil
}

func (a *app) fetchA6RangePage(tokenName string, start, end time.Time, page int) (a6LogPage, error) {
	query := url.Values{
		"p": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(a.cfg.A6PageSize)}, "type": {"2"},
		"token_name": {tokenName}, "model_name": {""}, "start_timestamp": {strconv.FormatInt(start.Unix(), 10)},
		"end_timestamp": {strconv.FormatInt(end.Unix(), 10)}, "group": {""}, "request_id": {""},
	}
	var response a6LogPage
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		lastErr = a.a6GET("/api/log/self?"+query.Encode(), &response)
		if lastErr == nil {
			if !response.Success {
				return a6LogPage{}, fmt.Errorf("A6 log rejected request: %s", response.Message)
			}
			return response, nil
		}
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}
	return a6LogPage{}, lastErr
}

func (a *app) storeA6RangeItems(items []a6LogItem, tokenName string, accountID int64, quotaPerUnit decimal.Decimal, start, end time.Time) (int, error) {
	processed := 0
	for _, item := range items {
		if item.RequestID == "" || item.TokenName != tokenName {
			continue
		}
		occurredAt := time.Unix(item.CreatedAt, 0).UTC()
		if occurredAt.Before(start) || occurredAt.After(end) {
			continue
		}
		known, err := a.hasUpstreamRecord(a6Provider, item.RequestID)
		if err != nil {
			return processed, err
		}
		if known {
			continue
		}
		if err := a.storeA6Item(item, accountID, quotaPerUnit, occurredAt); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (a *app) hasUpstreamRecord(provider, requestID string) (bool, error) {
	var exists bool
	err := a.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM upstream_usage WHERE provider=? AND upstream_request_id=?)`, provider, requestID).Scan(&exists)
	return exists, err
}

func parseA6Details(raw json.RawMessage) a6BillingDetails {
	var details a6BillingDetails
	if len(raw) == 0 || string(raw) == "null" {
		return details
	}
	if raw[0] == '"' {
		var encoded string
		if json.Unmarshal(raw, &encoded) == nil {
			_ = json.Unmarshal([]byte(encoded), &details)
		}
		return details
	}
	_ = json.Unmarshal(raw, &details)
	return details
}

func (a *app) storeA6Item(item a6LogItem, accountID int64, quotaPerUnit decimal.Decimal, occurredAt time.Time) error {
	quota, err := decimal.NewFromString(item.Quota.String())
	if err != nil || quota.IsNegative() {
		return fmt.Errorf("A6 request %s has invalid quota", item.RequestID)
	}
	details := parseA6Details(item.Other)
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}
	costUSD := quota.Div(quotaPerUnit)
	fx := a.fxRateForTime(occurredAt)
	_, err = a.db.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,fx_rate_source,fx_rate_effective_at,fx_rate_fetched_at,fx_rate_stale,occurred_at,model,input_tokens,output_tokens,cache_tokens,token_name,account_id_hint,channel_id,channel_name,billing_details,match_method,source,imported_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(provider,upstream_request_id) DO UPDATE SET occurred_at=excluded.occurred_at,model=excluded.model,input_tokens=excluded.input_tokens,output_tokens=excluded.output_tokens,cache_tokens=excluded.cache_tokens,token_name=excluded.token_name,account_id_hint=excluded.account_id_hint,channel_id=excluded.channel_id,channel_name=excluded.channel_name,billing_details=excluded.billing_details,source=excluded.source,imported_at=excluded.imported_at`,
		a6Provider, item.RequestID, costUSD.String(), "USD", fx.Rate.String(), costUSD.Mul(fx.Rate).String(), fx.Source, formatOptionalTime(fx.EffectiveAt), formatOptionalTime(fx.FetchedAt), boolInt(fx.Stale), occurredAt.Format(time.RFC3339Nano), item.ModelName, item.PromptTokens, item.CompletionTokens, details.CacheTokens, item.TokenName, accountID, item.Channel, item.ChannelName, string(detailsJSON), a6StagingMatchMethod, "a6_api", time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

type matchCandidate struct {
	sourceID int64
	delta    time.Duration
}

func (a *app) reconcileUpstream() error {
	if err := a.loadA6HistoricalTokenAccounts(); err != nil {
		return err
	}
	rows, err := a.db.Query(`SELECT provider,upstream_request_id,occurred_at,model,input_tokens,output_tokens,cache_tokens,account_id_hint,token_name,match_method
FROM upstream_usage WHERE usage_source_id IS NULL AND allocation_scope='' ORDER BY occurred_at`)
	if err != nil {
		return err
	}
	type pendingRecord struct {
		provider, requestID, occurredAt, model, matchMethod string
		inputTokens, outputTokens, cacheTokens              int64
		accountID                                           int64
		tokenName                                           string
	}
	pending := make([]pendingRecord, 0)
	for rows.Next() {
		var record pendingRecord
		if err := rows.Scan(&record.provider, &record.requestID, &record.occurredAt, &record.model, &record.inputTokens, &record.outputTokens, &record.cacheTokens, &record.accountID, &record.tokenName, &record.matchMethod); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, record)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, record := range pending {
		sourceID, method, err := a.matchUpstreamRecordForToken(record.requestID, record.occurredAt, record.model, record.inputTokens, record.outputTokens, record.cacheTokens, record.accountID, record.tokenName)
		if err != nil {
			return err
		}
		if sourceID == 0 {
			if record.matchMethod == a6StagingMatchMethod {
				if _, err := a.db.Exec(`UPDATE upstream_usage SET match_method='' WHERE provider=? AND upstream_request_id=? AND usage_source_id IS NULL AND match_method=?`, record.provider, record.requestID, a6StagingMatchMethod); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := a.db.Exec(`UPDATE upstream_usage SET usage_source_id=?,match_method=? WHERE provider=? AND upstream_request_id=? AND usage_source_id IS NULL`, sourceID, method, record.provider, record.requestID); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) matchUpstreamRecord(requestID, occurredRaw, model string, inputTokens, outputTokens, cacheTokens, accountID int64) (int64, string, error) {
	return a.matchUpstreamRecordForToken(requestID, occurredRaw, model, inputTokens, outputTokens, cacheTokens, accountID, "")
}

func (a *app) matchUpstreamRecordForToken(requestID, occurredRaw, model string, inputTokens, outputTokens, cacheTokens, accountID int64, tokenName string) (int64, string, error) {
	var directID int64
	directErr := a.db.QueryRow(`SELECT u.source_id FROM usage_snapshots u
JOIN request_maps m ON m.client_request_id=CASE WHEN u.request_id LIKE 'client:%' THEN substr(u.request_id,8) ELSE u.request_id END
WHERE m.upstream_request_id=? AND NOT EXISTS(SELECT 1 FROM upstream_usage x WHERE x.usage_source_id=u.source_id) LIMIT 1`, requestID).Scan(&directID)
	if directErr == nil {
		return directID, "direct_request_id", nil
	}
	if !errors.Is(directErr, sql.ErrNoRows) {
		return 0, "", directErr
	}
	if model == "" {
		return 0, "", nil
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, occurredRaw)
	if err != nil {
		return 0, "", err
	}
	windowStart := occurredAt.Add(-2 * time.Minute).Format(time.RFC3339Nano)
	windowEnd := occurredAt.Add(2 * time.Minute).Format(time.RFC3339Nano)
	accountIDs := []int64{}
	if tokenName != "" {
		var err error
		accountIDs, err = a.a6TokenAccountIDs(tokenName)
		if err != nil {
			return 0, "", err
		}
	}
	if len(accountIDs) == 0 && accountID > 0 {
		accountIDs = []int64{accountID}
	}
	if len(accountIDs) == 0 {
		return 0, "", nil
	}
	placeholders := make([]string, len(accountIDs))
	args := make([]any, 0, len(accountIDs)+8)
	for i := range accountIDs {
		placeholders[i] = "?"
	}
	accountCondition := "u.account_id IN (" + strings.Join(placeholders, ",") + ")"
	for _, id := range accountIDs {
		args = append(args, id)
	}
	args = append(args, model, outputTokens, cacheTokens, inputTokens, inputTokens, windowStart, windowEnd)
	candidates, err := a.compositeMatchCandidates(occurredAt, `SELECT u.source_id,u.created_at FROM usage_snapshots u
WHERE `+accountCondition+`
AND (CASE WHEN u.upstream_model<>'' THEN u.upstream_model ELSE u.model END)=?
AND u.output_tokens=?
AND (u.cache_read_tokens+u.cache_creation_tokens)=?
AND (u.input_tokens=? OR u.input_tokens+u.cache_read_tokens+u.cache_creation_tokens=?)
AND u.created_at BETWEEN ? AND ?
AND NOT EXISTS(SELECT 1 FROM upstream_usage x WHERE x.usage_source_id=u.source_id)`, args...)
	if err != nil {
		return 0, "", err
	}
	if sourceID := closestUniqueCandidate(candidates); sourceID != 0 {
		return sourceID, "composite_account_model_tokens_time", nil
	}
	if len(candidates) > 0 || cacheTokens <= 0 {
		return 0, "", nil
	}

	// A6's cache_tokens is normally the downstream cache total, but Claude
	// billing can report only cache-read tokens when cache creation is present.
	// Accept that representation only when the same strict model/token/time
	// constraints leave one candidate, so shared-token traffic stays safe.
	readArgs := make([]any, 0, len(accountIDs)+9)
	for _, id := range accountIDs {
		readArgs = append(readArgs, id)
	}
	readArgs = append(readArgs, model, outputTokens, cacheTokens, inputTokens, inputTokens, inputTokens, windowStart, windowEnd)
	readCandidates, err := a.compositeMatchCandidates(occurredAt, `SELECT u.source_id,u.created_at FROM usage_snapshots u
WHERE `+accountCondition+`
AND (CASE WHEN u.upstream_model<>'' THEN u.upstream_model ELSE u.model END)=?
AND u.output_tokens=?
AND u.cache_read_tokens=?
AND (u.input_tokens=? OR u.input_tokens+u.cache_read_tokens=? OR u.input_tokens+u.cache_read_tokens+u.cache_creation_tokens=?)
AND u.created_at BETWEEN ? AND ?
AND NOT EXISTS(SELECT 1 FROM upstream_usage x WHERE x.usage_source_id=u.source_id)`, readArgs...)
	if err != nil {
		return 0, "", err
	}
	if len(readCandidates) == 1 {
		return readCandidates[0].sourceID, "composite_account_model_tokens_time_cache_read", nil
	}

	// Some A6 Claude bills report one fewer cached token than the downstream log.
	// Keep this fallback narrow so shared upstream traffic cannot be broadly paired.
	tightStart := occurredAt.Add(-2 * time.Second).Format(time.RFC3339Nano)
	tightEnd := occurredAt.Add(2 * time.Second).Format(time.RFC3339Nano)
	args = args[:0]
	for _, id := range accountIDs {
		args = append(args, id)
	}
	args = append(args, model, outputTokens, cacheTokens, inputTokens, inputTokens, tightStart, tightEnd)
	candidates, err = a.compositeMatchCandidates(occurredAt, `SELECT u.source_id,u.created_at FROM usage_snapshots u
WHERE `+accountCondition+`
AND (CASE WHEN u.upstream_model<>'' THEN u.upstream_model ELSE u.model END)=?
AND u.output_tokens=?
AND ABS((u.cache_read_tokens+u.cache_creation_tokens)-?)=1
AND (u.cache_read_tokens+u.cache_creation_tokens)>0
AND (u.input_tokens=? OR u.input_tokens+u.cache_read_tokens+u.cache_creation_tokens=?)
AND u.created_at BETWEEN ? AND ?
AND NOT EXISTS(SELECT 1 FROM upstream_usage x WHERE x.usage_source_id=u.source_id)`, args...)
	if err != nil {
		return 0, "", err
	}
	if len(candidates) != 1 {
		return 0, "", nil
	}
	return candidates[0].sourceID, "composite_account_model_tokens_time_cache_tolerance", nil
}

func (a *app) compositeMatchCandidates(occurredAt time.Time, query string, args ...any) ([]matchCandidate, error) {
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	candidates := make([]matchCandidate, 0)
	for rows.Next() {
		var sourceID int64
		var createdRaw string
		if err := rows.Scan(&sourceID, &createdRaw); err != nil {
			rows.Close()
			return nil, err
		}
		createdAt, err := time.Parse(time.RFC3339Nano, createdRaw)
		if err != nil {
			continue
		}
		delta := occurredAt.Sub(createdAt)
		if delta < 0 {
			delta = -delta
		}
		candidates = append(candidates, matchCandidate{sourceID: sourceID, delta: delta})
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return candidates, nil
}

func closestUniqueCandidate(candidates []matchCandidate) int64 {
	if len(candidates) == 0 {
		return 0
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].delta < candidates[j].delta })
	if len(candidates) > 1 && candidates[0].delta == candidates[1].delta {
		return 0
	}
	return candidates[0].sourceID
}
