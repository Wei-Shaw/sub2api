package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const accountRulesDDL = `
CREATE TABLE IF NOT EXISTS upstream_account_rules (
  account_id INTEGER PRIMARY KEY,
  provider TEXT NOT NULL,
  external_key TEXT NOT NULL DEFAULT '',
  multiplier TEXT NOT NULL DEFAULT '',
  version INTEGER NOT NULL DEFAULT 1,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK(provider IN ('a6','subarx'))
);`

type accountRuleSnapshot struct {
	Provider    string
	ExternalKey string
	Multiplier  string
	Version     int64
	CapturedAt  string
}

type accountRule struct {
	AccountID   int64  `json:"account_id"`
	Provider    string `json:"provider"`
	ExternalKey string `json:"token_name"`
	Multiplier  string `json:"multiplier"`
	Version     int64  `json:"version"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type accountRuleActivity struct {
	UsageCount int    `json:"usage_count"`
	FirstSeen  string `json:"first_seen"`
	LastSeen   string `json:"last_seen"`
	Models     string `json:"models"`
}

type accountRuleView struct {
	accountRule
	Configured         bool   `json:"configured"`
	Current            bool   `json:"current"`
	GroupID            int64  `json:"group_id"`
	GroupName          string `json:"group_name"`
	GroupPriority      int    `json:"group_priority"`
	AccountName        string `json:"account_name"`
	AccountPlatform    string `json:"account_platform"`
	AccountStatus      string `json:"account_status"`
	AccountSchedulable bool   `json:"account_schedulable"`
	accountRuleActivity
}

type currentAccountGroup struct {
	GroupID            int64
	GroupName          string
	GroupPriority      int
	AccountID          int64
	AccountName        string
	AccountPlatform    string
	AccountStatus      string
	AccountSchedulable bool
}

type accountRuleInput struct {
	Provider   string `json:"provider"`
	TokenName  string `json:"token_name"`
	Multiplier string `json:"multiplier"`
	Enabled    *bool  `json:"enabled"`
}

func (a *app) initAccountRules() error {
	if _, err := a.db.Exec(accountRulesDDL); err != nil {
		return err
	}
	if _, err := a.ensureColumn("upstream_account_rules", "version", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if a.stateString("account_rules_seeded") != "1" {
		tx, err := a.db.Begin()
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for token, accountID := range a.cfg.A6TokenAccountMap {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO upstream_account_rules(account_id,provider,external_key,multiplier,version,enabled,created_at,updated_at) VALUES(?,?,?,?,1,1,?,?)`, accountID, "a6", token, "", now, now); err != nil {
				tx.Rollback()
				return err
			}
		}
		for accountID, multiplier := range a.cfg.SubarxAccountMultipliers {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO upstream_account_rules(account_id,provider,external_key,multiplier,version,enabled,created_at,updated_at) VALUES(?,?,?,?,1,1,?,?)`, accountID, "subarx", "", multiplier.String(), now, now); err != nil {
				tx.Rollback()
				return err
			}
		}
		if _, err := tx.Exec(`INSERT INTO state(key,value) VALUES('account_rules_seeded','1') ON CONFLICT(key) DO UPDATE SET value=excluded.value`); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	if err := a.reloadAccountRules(); err != nil {
		return err
	}
	return a.backfillRuleSnapshots()
}

func (a *app) reloadAccountRules() error {
	rows, err := a.db.Query(`SELECT account_id,provider,external_key,multiplier,version FROM upstream_account_rules WHERE enabled=1 ORDER BY account_id`)
	if err != nil {
		return err
	}
	a6Rules := map[string]int64{}
	a6Accounts := map[string][]int64{}
	subarxRules := map[int64]decimal.Decimal{}
	providers := map[int64]string{}
	snapshots := map[int64]accountRuleSnapshot{}
	for rows.Next() {
		var accountID int64
		var provider, externalKey, multiplierRaw string
		var version int64
		if err := rows.Scan(&accountID, &provider, &externalKey, &multiplierRaw, &version); err != nil {
			rows.Close()
			return err
		}
		switch provider {
		case "a6":
			if _, exists := a6Rules[externalKey]; !exists {
				a6Rules[externalKey] = accountID
			}
			a6Accounts[externalKey] = append(a6Accounts[externalKey], accountID)
		case "subarx":
			multiplier, err := decimal.NewFromString(multiplierRaw)
			if err != nil || !multiplier.IsPositive() {
				rows.Close()
				return fmt.Errorf("invalid stored Subarx multiplier for account %d", accountID)
			}
			subarxRules[accountID] = multiplier
		}
		providers[accountID] = provider
		snapshots[accountID] = accountRuleSnapshot{Provider: provider, ExternalKey: externalKey, Multiplier: multiplierRaw, Version: version}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	a.rulesMu.Lock()
	a.a6TokenAccountMap = a6Rules
	a.a6TokenAccounts = a6Accounts
	a.subarxAccountMultipliers = subarxRules
	a.accountProviders = providers
	a.accountRuleSnapshots = snapshots
	a.rulesMu.Unlock()
	return nil
}

func (a *app) backfillRuleSnapshots() error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := a.db.Exec(`UPDATE usage_snapshots
SET rule_provider=COALESCE((SELECT provider FROM upstream_account_rules r WHERE r.account_id=usage_snapshots.account_id AND r.enabled=1),''),
    rule_external_key=COALESCE((SELECT external_key FROM upstream_account_rules r WHERE r.account_id=usage_snapshots.account_id AND r.enabled=1),''),
    rule_multiplier=COALESCE((SELECT multiplier FROM upstream_account_rules r WHERE r.account_id=usage_snapshots.account_id AND r.enabled=1),''),
    rule_version=COALESCE((SELECT version FROM upstream_account_rules r WHERE r.account_id=usage_snapshots.account_id AND r.enabled=1),0),
    rule_snapshot_at=?
WHERE rule_provider=''
  AND rule_external_key=''
  AND rule_multiplier=''
  AND rule_version=0
  AND NOT EXISTS (SELECT 1 FROM upstream_usage x WHERE x.usage_source_id=usage_snapshots.source_id)
  AND EXISTS (SELECT 1 FROM upstream_account_rules r WHERE r.account_id=usage_snapshots.account_id AND r.enabled=1)`, now)
	return err
}

func (a *app) accountRuleSnapshotDetails() map[int64]accountRuleSnapshot {
	a.rulesMu.RLock()
	defer a.rulesMu.RUnlock()
	out := make(map[int64]accountRuleSnapshot, len(a.accountRuleSnapshots))
	for id, snapshot := range a.accountRuleSnapshots {
		out[id] = snapshot
	}
	return out
}

func (a *app) a6TokenAccountsSnapshot() map[string][]int64 {
	a.rulesMu.RLock()
	defer a.rulesMu.RUnlock()
	out := make(map[string][]int64, len(a.a6TokenAccounts))
	for token, ids := range a.a6TokenAccounts {
		out[token] = append([]int64(nil), ids...)
	}
	return out
}

// a6TokenAccountIDs includes accounts that used this token in an earlier
// snapshot, so a token rename does not strand historical unmatched bills.
func (a *app) a6TokenAccountIDs(tokenName string) ([]int64, error) {
	seen := map[int64]struct{}{}
	current := a.a6TokenAccountsSnapshot()
	for _, accountID := range current[tokenName] {
		if accountID > 0 {
			seen[accountID] = struct{}{}
		}
	}
	a.rulesMu.RLock()
	historical := append([]int64(nil), a.a6HistoricalTokenAccounts[tokenName]...)
	snapshots := make(map[int64]accountRuleSnapshot, len(a.accountRuleSnapshots))
	for accountID, snapshot := range a.accountRuleSnapshots {
		snapshots[accountID] = snapshot
	}
	a.rulesMu.RUnlock()
	for _, accountID := range historical {
		seen[accountID] = struct{}{}
	}
	// If an account moved from an old token to a new shared token, include all
	// accounts that now share that token. This lets old bills consider channels
	// added later without weakening the model/token/time uniqueness checks.
	for {
		before := len(seen)
		for accountID := range seen {
			snapshot := snapshots[accountID]
			if snapshot.Provider != "a6" || snapshot.ExternalKey == "" {
				continue
			}
			for _, peerID := range current[snapshot.ExternalKey] {
				seen[peerID] = struct{}{}
			}
		}
		if len(seen) == before {
			break
		}
	}
	accountIDs := make([]int64, 0, len(seen))
	for accountID := range seen {
		accountIDs = append(accountIDs, accountID)
	}
	sort.Slice(accountIDs, func(i, j int) bool { return accountIDs[i] < accountIDs[j] })
	return accountIDs, nil
}

func (a *app) loadA6HistoricalTokenAccounts() error {
	rows, err := a.db.Query(`SELECT rule_external_key,account_id FROM usage_snapshots
WHERE rule_provider='a6' AND rule_external_key<>'' AND account_id>0 GROUP BY rule_external_key,account_id`)
	if err != nil {
		return err
	}
	history := map[string][]int64{}
	for rows.Next() {
		var token string
		var accountID int64
		if err := rows.Scan(&token, &accountID); err != nil {
			rows.Close()
			return err
		}
		history[token] = append(history[token], accountID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	a.rulesMu.Lock()
	a.a6HistoricalTokenAccounts = history
	a.rulesMu.Unlock()
	return nil
}

func (a *app) accountRuleSnapshot() (map[string]int64, map[int64]decimal.Decimal, map[int64]string) {
	a.rulesMu.RLock()
	defer a.rulesMu.RUnlock()
	a6Rules := make(map[string]int64, len(a.a6TokenAccountMap))
	for key, value := range a.a6TokenAccountMap {
		a6Rules[key] = value
	}
	subarxRules := make(map[int64]decimal.Decimal, len(a.subarxAccountMultipliers))
	for key, value := range a.subarxAccountMultipliers {
		subarxRules[key] = value
	}
	providers := make(map[int64]string, len(a.accountProviders))
	for key, value := range a.accountProviders {
		providers[key] = value
	}
	return a6Rules, subarxRules, providers
}

func (a *app) sortedA6AccountRules() []accountRule {
	a.rulesMu.RLock()
	defer a.rulesMu.RUnlock()
	out := make([]accountRule, 0)
	for token, accountIDs := range a.a6TokenAccounts {
		for _, accountID := range accountIDs {
			out = append(out, accountRule{AccountID: accountID, Provider: "a6", ExternalKey: token, Enabled: true})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ExternalKey != out[j].ExternalKey {
			return out[i].ExternalKey < out[j].ExternalKey
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out
}

func validateAccountRule(accountID int64, input accountRuleInput) (accountRule, error) {
	if accountID <= 0 {
		return accountRule{}, errors.New("account_id must be a positive integer")
	}
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	rule := accountRule{AccountID: accountID, Provider: provider, Enabled: enabled}
	switch provider {
	case "a6":
		rule.ExternalKey = strings.TrimSpace(input.TokenName)
		if rule.ExternalKey == "" {
			return accountRule{}, errors.New("token_name is required for A6")
		}
	case "subarx":
		multiplier, err := decimal.NewFromString(strings.TrimSpace(input.Multiplier))
		if err != nil || !multiplier.IsPositive() {
			return accountRule{}, errors.New("multiplier must be a positive decimal for Subarx")
		}
		rule.Multiplier = multiplier.String()
	default:
		return accountRule{}, errors.New("provider must be a6 or subarx")
	}
	return rule, nil
}

func (a *app) accountRules(w http.ResponseWriter, r *http.Request) {
	window, err := queryTimeWindow(r, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	views, err := a.accountRuleViews(window.From, window.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	unconfiguredAccounts := map[int64]struct{}{}
	for _, view := range views {
		if !view.Configured && view.UsageCount > 0 {
			unconfiguredAccounts[view.AccountID] = struct{}{}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": views, "unconfigured_accounts": len(unconfiguredAccounts), "from": window.From, "to": window.To})
}

func (a *app) accountRuleViews(from, to string) ([]accountRuleView, error) {
	rules := map[int64]accountRule{}
	rows, err := a.db.Query(`SELECT account_id,provider,external_key,multiplier,version,enabled,created_at,updated_at FROM upstream_account_rules ORDER BY account_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var rule accountRule
		var enabled int
		if err := rows.Scan(&rule.AccountID, &rule.Provider, &rule.ExternalKey, &rule.Multiplier, &rule.Version, &enabled, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		rule.Enabled = enabled == 1
		rules[rule.AccountID] = rule
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	activityByAccount := map[int64]accountRuleActivity{}
	rows, err = a.db.Query(`SELECT account_id,COUNT(*),MIN(created_at),MAX(created_at) FROM usage_snapshots WHERE created_at>=? AND created_at<? GROUP BY account_id`, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var accountID int64
		var activity accountRuleActivity
		if err := rows.Scan(&accountID, &activity.UsageCount, &activity.FirstSeen, &activity.LastSeen); err != nil {
			rows.Close()
			return nil, err
		}
		activityByAccount[accountID] = activity
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	allAccountIDs := map[int64]struct{}{}
	for accountID := range rules {
		allAccountIDs[accountID] = struct{}{}
	}
	for accountID := range activityByAccount {
		allAccountIDs[accountID] = struct{}{}
	}
	for accountID := range allAccountIDs {
		activity := activityByAccount[accountID]
		err := a.db.QueryRow(`SELECT model FROM usage_snapshots WHERE account_id=? ORDER BY created_at DESC,source_id DESC LIMIT 1`, accountID).Scan(&activity.Models)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		activityByAccount[accountID] = activity
	}

	current, err := a.currentAccountGroups()
	if err != nil {
		return nil, err
	}
	out := make([]accountRuleView, 0, len(current)+len(rules))
	seenCurrentAccounts := map[int64]struct{}{}
	for _, item := range current {
		rule, configured := rules[item.AccountID]
		if !configured {
			rule = accountRule{AccountID: item.AccountID, Enabled: true}
		}
		out = append(out, accountRuleView{
			accountRule: rule, Configured: configured && rule.Enabled, Current: true,
			GroupID: item.GroupID, GroupName: item.GroupName, GroupPriority: item.GroupPriority,
			AccountName: item.AccountName, AccountPlatform: item.AccountPlatform,
			AccountStatus: item.AccountStatus, AccountSchedulable: item.AccountSchedulable,
			accountRuleActivity: activityByAccount[item.AccountID],
		})
		seenCurrentAccounts[item.AccountID] = struct{}{}
	}
	for accountID := range allAccountIDs {
		if _, ok := seenCurrentAccounts[accountID]; ok {
			continue
		}
		rule, configured := rules[accountID]
		if !configured {
			rule = accountRule{AccountID: accountID, Enabled: true}
		}
		out = append(out, accountRuleView{
			accountRule: rule, Configured: configured && rule.Enabled,
			GroupName: "未分组或已移除", AccountName: fmt.Sprintf("账号 #%d", accountID),
			accountRuleActivity: activityByAccount[accountID],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Current != out[j].Current {
			return out[i].Current
		}
		if out[i].GroupName != out[j].GroupName {
			return out[i].GroupName < out[j].GroupName
		}
		if out[i].GroupID != out[j].GroupID {
			return out[i].GroupID < out[j].GroupID
		}
		if out[i].GroupPriority != out[j].GroupPriority {
			return out[i].GroupPriority < out[j].GroupPriority
		}
		if out[i].Configured != out[j].Configured {
			return !out[i].Configured
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out, nil
}

func (a *app) currentAccountGroups() ([]currentAccountGroup, error) {
	if a.source == nil {
		return nil, nil
	}
	rows, err := a.source.Query(`
SELECT g.id,g.name,ag.priority,a.id,a.name,a.platform,a.status,a.schedulable
FROM groups g
JOIN account_groups ag ON ag.group_id=g.id
JOIN accounts a ON a.id=ag.account_id
WHERE g.deleted_at IS NULL AND a.deleted_at IS NULL
UNION ALL
SELECT 0,'',0,a.id,a.name,a.platform,a.status,a.schedulable
FROM accounts a
WHERE a.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM account_groups ag
    JOIN groups g ON g.id=ag.group_id AND g.deleted_at IS NULL
    WHERE ag.account_id=a.id
  )
ORDER BY 2,3,4`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]currentAccountGroup, 0)
	for rows.Next() {
		var item currentAccountGroup
		if err := rows.Scan(&item.GroupID, &item.GroupName, &item.GroupPriority, &item.AccountID, &item.AccountName, &item.AccountPlatform, &item.AccountStatus, &item.AccountSchedulable); err != nil {
			return nil, err
		}
		if item.GroupName == "" {
			item.GroupName = "未分组"
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (a *app) putAccountRule(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.ParseInt(r.PathValue("account_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account_id")
		return
	}
	var input accountRuleInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	rule, err := validateAccountRule(accountID, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var oldProvider, oldKey string
	var oldVersion int64
	_ = a.db.QueryRow(`SELECT provider,external_key,version FROM upstream_account_rules WHERE account_id=?`, accountID).Scan(&oldProvider, &oldKey, &oldVersion)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	enabled := 0
	if rule.Enabled {
		enabled = 1
	}
	_, err = a.db.Exec(`INSERT INTO upstream_account_rules(account_id,provider,external_key,multiplier,version,enabled,created_at,updated_at) VALUES(?,?,?,?,1,?,?,?) ON CONFLICT(account_id) DO UPDATE SET provider=excluded.provider,external_key=excluded.external_key,multiplier=excluded.multiplier,version=upstream_account_rules.version+1,enabled=excluded.enabled,updated_at=excluded.updated_at`, accountID, rule.Provider, rule.ExternalKey, rule.Multiplier, enabled, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if oldProvider == "a6" && oldKey != "" && oldKey != rule.ExternalKey {
		_, _ = a.db.Exec(`DELETE FROM state WHERE key=?`, "a6_bootstrap_done:"+oldKey)
	}
	if rule.Provider == "a6" {
		_, _ = a.db.Exec(`DELETE FROM state WHERE key=?`, "a6_bootstrap_done:"+rule.ExternalKey)
	}
	if err := a.reloadAccountRules(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.backfillRuleSnapshots(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = a.db.QueryRow(`SELECT version FROM upstream_account_rules WHERE account_id=?`, accountID).Scan(&rule.Version)
	if rule.Provider == "a6" && rule.Enabled && a.cfg.A6AccessToken != "" {
		go func() {
			if err := a.collect(true); err != nil {
				fmt.Printf("account rule A6 sync: %v\n", err)
			}
		}()
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "rule": rule})
}

func (a *app) deleteAccountRule(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.ParseInt(r.PathValue("account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid account_id")
		return
	}
	result, err := a.db.Exec(`DELETE FROM upstream_account_rules WHERE account_id=?`, accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.reloadAccountRules(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	count, _ := result.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "deleted": count})
}
