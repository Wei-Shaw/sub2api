package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
)

type billingUsageEvidence struct {
	Usage              *ProviderBillUsage `json:"usage,omitempty"`
	UsageStatus        string             `json:"usage_status"`
	TokenStatus        string             `json:"token_status"`
	LocalMatchedTokens *int64             `json:"local_matched_tokens,omitempty"`
	TokenDifference    *int64             `json:"token_difference,omitempty"`
}

type BillingBillSource struct {
	ID             int64           `json:"id"`
	ProviderBillID string          `json:"provider_bill_id"`
	Provider       string          `json:"provider"`
	Description    string          `json:"description"`
	SourceAmount   string          `json:"source_amount"`
	Currency       string          `json:"currency"`
	PeriodStart    time.Time       `json:"period_start"`
	PeriodEnd      time.Time       `json:"period_end"`
	RawSource      json.RawMessage `json:"raw_source"`
}

// RawBillSource is deliberately separate from Report: original invoice rows
// can contain large component lists and are never returned to ordinary users.
// The HTTP handler must require administrator authorization for this method.
func (s *UpstreamReconciliationService) RawBillSource(ctx context.Context, id int64) (*BillingBillSource, error) {
	if id <= 0 {
		return nil, billingInvalid("invalid invoice source identifier")
	}
	var b BillingBillSource
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT b.id,b.provider_bill_id,c.provider,b.description,b.source_amount,b.currency,b.period_start,b.period_end,b.raw_source FROM upstream_billing_bills b JOIN upstream_billing_runs r ON r.id=b.run_id JOIN upstream_billing_connections c ON c.id=r.connection_id WHERE b.id=$1`, id).Scan(&b.ID, &b.ProviderBillID, &b.Provider, &b.Description, &b.SourceAmount, &b.Currency, &b.PeriodStart, &b.PeriodEnd, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBillingNotFound
	}
	if err != nil || len(raw) > 131072 {
		return nil, ErrBillingStorage
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil || compact.Len() > 65536 {
		return nil, ErrBillingStorage
	}
	b.RawSource = json.RawMessage(compact.Bytes())
	return &b, nil
}

func normalizeBillingEvidence(b *ProviderBill) error {
	if len(b.RawSource) == 0 {
		b.RawSource = json.RawMessage(`{}`)
	}
	trimmed := bytes.TrimSpace(b.RawSource)
	if len(b.RawSource) > 65536 || !json.Valid(b.RawSource) || len(trimmed) == 0 || trimmed[0] != '{' {
		return billingInvalid("provider original invoice row is invalid or exceeds 64 KiB")
	}
	var compact bytes.Buffer
	if json.Compact(&compact, b.RawSource) != nil {
		return billingInvalid("provider original invoice row is invalid")
	}
	b.RawSource = json.RawMessage(compact.Bytes())
	if b.UsageStatus == "" {
		b.UsageStatus = "unsupported"
	}
	if b.UsageStatus != "available" && b.UsageStatus != "unavailable" && b.UsageStatus != "unsupported" {
		return billingInvalid("provider token evidence status is invalid")
	}
	if b.UsageStatus == "available" && b.Usage == nil {
		return billingInvalid("provider marked missing token evidence as available")
	}
	if b.Usage != nil {
		u := b.Usage
		if u.Tokens < 0 || len(u.Model) > 256 || len(u.TokenType) > 64 || len(u.ContextWindow) > 64 || len(u.ServiceTier) > 64 || len(u.InferenceGeo) > 64 {
			return billingInvalid("provider token evidence is invalid")
		}
	}
	return nil
}

func checkBillingImportOverlap(ctx context.Context, tx *sql.Tx, c *BillingConnection, bills []ProviderBill) error {
	var locked bool
	if tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "sub2api:upstream-billing:import:"+c.Provider).Scan(&locked) != nil {
		return ErrBillingStorage
	}
	if !locked {
		return ErrBillingBusy
	}
	if len(bills) == 0 {
		return nil
	}
	type window struct {
		ResourceID  string    `json:"resource_id"`
		PeriodStart time.Time `json:"period_start"`
		PeriodEnd   time.Time `json:"period_end"`
	}
	windows := make([]window, 0, len(bills))
	seen := map[string]bool{}
	for _, b := range bills {
		key := billingPeriodKey(b.ResourceID, b.PeriodStart, b.PeriodEnd)
		if !seen[key] {
			windows = append(windows, window{b.ResourceID, b.PeriodStart, b.PeriodEnd})
			seen[key] = true
		}
	}
	data, err := json.Marshal(windows)
	if err != nil {
		return ErrBillingStorage
	}
	var overlaps bool
	query := `SELECT EXISTS(SELECT 1 FROM upstream_billing_months m
	 JOIN upstream_billing_connections c ON c.id=m.connection_id JOIN upstream_billing_bills b ON b.run_id=m.active_run_id
	 JOIN jsonb_to_recordset($3::jsonb) AS incoming(resource_id TEXT,period_start TIMESTAMPTZ,period_end TIMESTAMPTZ)
	 ON b.resource_id=incoming.resource_id AND b.period_start<incoming.period_end AND b.period_end>incoming.period_start
	 WHERE c.provider=$1 AND m.connection_id<>$2`
	args := []any{c.Provider, c.ID, string(data)}
	if billingHasAccountScope(c.Provider) {
		query += ` AND c.settings->>'account_id'=$4`
		args = append(args, c.Settings["account_id"])
	}
	err = tx.QueryRowContext(ctx, query+`)`, args...).Scan(&overlaps)
	if err != nil {
		return ErrBillingStorage
	}
	if overlaps {
		return infraerrors.Conflict("UPSTREAM_BILLING_OVERLAPPING_SCOPE", "another connection already contains overlapping official bills for this provider resource; reuse that connection. Generic or organization-wide resource identifiers cannot safely distinguish separate cloud accounts")
	}
	return nil
}

func checkBillingEvidenceRegression(ctx context.Context, tx *sql.Tx, connectionID int64, month string, bills []ProviderBill) error {
	var unavailable []string
	for _, b := range bills {
		if b.UsageStatus == "unavailable" {
			unavailable = append(unavailable, b.ID)
		}
	}
	if len(unavailable) == 0 {
		return nil
	}
	var previouslyReconciled bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_billing_months m
	 JOIN upstream_billing_bills b ON b.run_id=m.active_run_id JOIN upstream_billing_allocations a ON a.bill_id=b.id
	 WHERE m.connection_id=$1 AND m.month=$2 AND b.provider_bill_id=ANY($3) AND a.official_tokens IS NOT NULL)`, connectionID, month, pq.Array(unavailable)).Scan(&previouslyReconciled)
	if err != nil {
		return ErrBillingStorage
	}
	if previouslyReconciled {
		return billingInvalid("official usage evidence is temporarily unavailable; the previous reconciled snapshot has been retained and synchronization will retry")
	}
	return nil
}

func billingEvidenceFor(b ProviderBill) (billingUsageEvidence, bool) {
	e := billingUsageEvidence{Usage: b.Usage, UsageStatus: b.UsageStatus, TokenStatus: b.UsageStatus}
	if e.TokenStatus == "" {
		e.TokenStatus = "unsupported"
		e.UsageStatus = "unsupported"
	}
	if b.UsageStatus != "available" || b.Usage == nil {
		return e, false
	}
	u := b.Usage
	if !u.Complete {
		e.TokenStatus = "incomplete"
		return e, false
	}
	// Historical input/cache counters can have been changed by ForceCacheBilling
	// and cache TTL overrides. Only output_tokens are unmodified upstream usage.
	if u.TokenType != "output" {
		e.TokenStatus = "local_category_unverified"
		return e, false
	}
	// There is no historical inference_geo column. Only a provider bucket with
	// no geographic billing dimension can be compared without assuming a region.
	if u.Model == "" || u.ServiceTier != "standard" || u.InferenceGeo != "not_available" || (u.ContextWindow != "0-200k" && u.ContextWindow != "200k-1M") {
		e.TokenStatus = "dimensions_unverified"
		return e, false
	}
	e.TokenStatus = "verified"
	return e, true
}

func billingOfficialUsageKey(b ProviderBill) string {
	u := b.Usage
	if u == nil {
		return ""
	}
	return billingPeriodKey(b.ResourceID, b.PeriodStart, b.PeriodEnd) + "\x00" + u.Model + "\x00" + u.TokenType + "\x00" + u.ContextWindow + "\x00" + u.ServiceTier + "\x00" + u.InferenceGeo
}

func loadComparableBillingUsage(ctx context.Context, tx *sql.Tx, accountIDs []int64, b ProviderBill) ([]billingUsageWeight, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT u.account_id,COALESCE(a.name,''),u.api_key_id,COALESCE(k.name,''),u.user_id,
	 COUNT(*),COALESCE(SUM(u.input_tokens::bigint+u.output_tokens::bigint+u.cache_creation_tokens::bigint+u.cache_read_tokens::bigint),0),COALESCE(SUM(u.total_cost),0)::text,COALESCE(SUM(u.output_tokens::bigint),0)
	 FROM usage_logs u LEFT JOIN accounts a ON a.id=u.account_id LEFT JOIN api_keys k ON k.id=u.api_key_id
	 WHERE u.account_id=ANY($1) AND u.created_at >= $2 AND u.created_at < $3
	 AND COALESCE(NULLIF(u.upstream_response_model,''),NULLIF(u.upstream_model,''),u.model)=$4
	 AND COALESCE(NULLIF(u.service_tier,''),'standard') IN ('standard','default')
	 AND CASE WHEN (u.input_tokens::bigint+u.cache_read_tokens::bigint+u.cache_creation_tokens::bigint)<=200000 THEN '0-200k'
	          WHEN (u.input_tokens::bigint+u.cache_read_tokens::bigint+u.cache_creation_tokens::bigint)<=1000000 THEN '200k-1M' ELSE 'unsupported' END=$5
	 GROUP BY u.account_id,a.name,u.api_key_id,k.name,u.user_id
	 ORDER BY u.account_id,u.api_key_id,u.user_id LIMIT $6`, pq.Array(accountIDs), b.PeriodStart, b.PeriodEnd, b.Usage.Model, b.Usage.ContextWindow, billingMaxAllocations+1)
	if err != nil {
		return nil, ErrBillingStorage
	}
	return scanComparableBillingUsage(rows)
}

func scanComparableBillingUsage(rows *sql.Rows) ([]billingUsageWeight, error) {
	defer rows.Close()
	result := []billingUsageWeight{}
	for rows.Next() {
		var u billingUsageWeight
		var matched int64
		if rows.Scan(&u.AccountID, &u.AccountName, &u.APIKeyID, &u.APIKeyName, &u.UserID, &u.Requests, &u.Tokens, &u.LocalCost, &matched) != nil {
			return nil, ErrBillingStorage
		}
		if matched < 0 {
			return nil, billingInvalid("local comparable token count is invalid")
		}
		u.MatchedTokens = &matched
		result = append(result, u)
		if len(result) > billingMaxAllocations {
			return nil, billingInvalid("comparable usage exceeds the allocation limit")
		}
	}
	if rows.Err() != nil {
		return nil, ErrBillingStorage
	}
	return result, nil
}

func loadPriorComparableBillingUsage(ctx context.Context, tx *sql.Tx, connectionID int64, month string, b ProviderBill) ([]billingUsageWeight, error) {
	u := b.Usage
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT ON (a.account_id,a.api_key_id,a.user_id)
	 a.account_id,a.account_name,a.api_key_id,a.api_key_name,a.user_id,a.requests,a.tokens,a.local_cost::text,a.local_matched_tokens
	 FROM upstream_billing_months m JOIN upstream_billing_bills b ON b.run_id=m.active_run_id
	 JOIN upstream_billing_allocations a ON a.bill_id=b.id
	 WHERE m.connection_id=$1 AND m.month=$2 AND b.resource_id=$3 AND b.period_start=$4 AND b.period_end=$5
	 AND b.usage_evidence->'usage'->>'model'=$6 AND b.usage_evidence->'usage'->>'token_type'=$7
	 AND b.usage_evidence->'usage'->>'context_window'=$8 AND b.usage_evidence->'usage'->>'service_tier'=$9
	 AND b.usage_evidence->'usage'->>'inference_geo'=$10 AND a.local_matched_tokens IS NOT NULL AND a.method='official_token_weighted'
	 ORDER BY a.account_id,a.api_key_id,a.user_id,a.requests DESC LIMIT $11`, connectionID, month, b.ResourceID, b.PeriodStart, b.PeriodEnd, u.Model, u.TokenType, u.ContextWindow, u.ServiceTier, u.InferenceGeo, billingMaxAllocations+1)
	if err != nil {
		return nil, ErrBillingStorage
	}
	return scanComparableBillingUsage(rows)
}

func allocateOfficialBillingAmount(amount string, official int64, usage []billingUsageWeight) ([]billingAllocation, int64, error) {
	var local int64
	for _, u := range usage {
		if u.MatchedTokens == nil || *u.MatchedTokens < 0 || *u.MatchedTokens > math.MaxInt64-local {
			return nil, 0, billingInvalid("local comparable token counts are invalid")
		}
		local += *u.MatchedTokens
	}
	if local > official {
		return nil, local, nil
	}
	if official == 0 || local == 0 {
		unmatched, err := allocateBillingAmount(amount, nil)
		return unmatched, local, err
	}
	weights := make([]billingUsageWeight, len(usage), len(usage)+1)
	copy(weights, usage)
	for i := range weights {
		weights[i].LocalCost = strconv.FormatInt(*weights[i].MatchedTokens, 10)
	}
	if official > local {
		weights = append(weights, billingUsageWeight{LocalCost: strconv.FormatInt(official-local, 10)})
	}
	allocations, err := allocateBillingAmount(amount, weights)
	if err != nil {
		return nil, local, err
	}
	for i := range allocations {
		if i < len(usage) {
			allocations[i].billingUsageWeight = usage[i]
			allocations[i].Method = "official_token_weighted"
		} else {
			allocations[i].billingUsageWeight = billingUsageWeight{LocalCost: "0"}
			allocations[i].Method = "unmatched"
		}
	}
	return allocations, local, nil
}
