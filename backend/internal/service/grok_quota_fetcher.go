package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const grokQuotaSnapshotExtraKey = "grok_usage_snapshot"

type GrokQuotaFetcher struct{REDACTED

func NewGrokQuotaFetcher() *GrokQuotaFetcher {
	return &GrokQuotaFetcher{REDACTED
REDACTED

func (f *GrokQuotaFetcher) BuildUsageInfo(account *Account) *UsageInfo {
	now := time.Now()
	usage := &UsageInfo{
		Source:    "passive",
		UpdatedAt: &now,
REDACTED
	if account == nil {
		usage.ErrorCode = "quota_unknown"
		usage.Error = "Grok quota is unknown until billing is probed or an upstream response includes xAI rate-limit headers"
		return usage
REDACTED

	billing, _ := grokBillingSnapshotFromExtra(account.Extra)
	snapshot, err := grokQuotaSnapshotFromExtra(account.Extra)
	if billing != nil {
		usage.GrokBilling = billing
		if billing.Plan != "" {
			usage.SubscriptionTier = billing.Plan
			usage.SubscriptionTierRaw = billing.Plan
	REDACTED
		if parsedAt, parseErr := time.Parse(time.RFC3339, billing.UpdatedAt); parseErr == nil {
			usage.UpdatedAt = &parsedAt
	REDACTED
		if billing.FetchedAt != "" {
			usage.GrokLastQuotaProbeAt = billing.FetchedAt
	REDACTED
		usage.GrokQuotaSnapshotState = "billing_observed"
		usage.GrokLastStatusCode = billing.StatusCode
		switch billing.StatusCode {
		case 401:
			usage.NeedsReauth = true
			usage.ErrorCode = "unauthenticated"
		case 403:
			usage.IsForbidden = true
			usage.ForbiddenType = "forbidden"
			usage.ErrorCode = "forbidden"
		case 429:
			usage.ErrorCode = "rate_limited"
	REDACTED
REDACTED

	if err != nil || snapshot == nil {
		applyGrokCredentialUsageFallback(usage, account)
		if billing == nil {
			usage.ErrorCode = "quota_unknown"
			usage.Error = "Grok quota is unknown until billing is probed or an upstream response includes xAI rate-limit headers"
	REDACTED
		return usage
REDACTED

	if parsedAt, parseErr := time.Parse(time.RFC3339, snapshot.UpdatedAt); parseErr == nil {
		if billing == nil || usage.UpdatedAt == nil || parsedAt.After(*usage.UpdatedAt) {
			usage.UpdatedAt = &parsedAt
	REDACTED
REDACTED
	usage.GrokRequestQuota = snapshot.Requests
	usage.GrokTokenQuota = snapshot.Tokens
	usage.GrokRetryAfterSeconds = snapshot.RetryAfterSeconds
	if usage.SubscriptionTier == "" {
		usage.SubscriptionTier = snapshot.SubscriptionTier
		usage.SubscriptionTierRaw = snapshot.SubscriptionTier
REDACTED
	if usage.GrokEntitlementStatus == "" {
		usage.GrokEntitlementStatus = snapshot.EntitlementStatus
REDACTED
	if usage.GrokLastQuotaProbeAt == "" {
		usage.GrokLastQuotaProbeAt = snapshot.LastProbeAt
REDACTED
	usage.GrokLastHeadersSeenAt = snapshot.LastHeadersSeenAt
	if snapshot.StatusCode >= http.StatusBadRequest || usage.GrokLastStatusCode == 0 {
		usage.GrokLastStatusCode = snapshot.StatusCode
REDACTED
	if snapshot.HasObservedHeaders() {
		if usage.GrokQuotaSnapshotState == "" {
			usage.GrokQuotaSnapshotState = "observed"
	REDACTED
REDACTED else if billing == nil {
		usage.GrokQuotaSnapshotState = "no_headers"
		usage.ErrorCode = "quota_unknown"
		usage.Error = "No xAI quota headers observed on the latest Grok probe"
REDACTED

	if usage.ErrorCode == "" {
		switch snapshot.StatusCode {
		case 401:
			usage.NeedsReauth = true
			usage.ErrorCode = "unauthenticated"
		case 403:
			usage.IsForbidden = true
			usage.ForbiddenType = "forbidden"
			usage.ErrorCode = "forbidden"
			if usage.GrokEntitlementStatus == "" {
				usage.GrokEntitlementStatus = "forbidden"
		REDACTED
		case 429:
			usage.ErrorCode = "rate_limited"
	REDACTED
REDACTED
	applyGrokCredentialUsageFallback(usage, account)
	return usage
REDACTED

func applyGrokCredentialUsageFallback(usage *UsageInfo, account *Account) {
	if usage == nil || account == nil {
		return
REDACTED
	if usage.SubscriptionTier == "" {
		tier := strings.TrimSpace(account.GetCredential("subscription_tier"))
		usage.SubscriptionTier = tier
		usage.SubscriptionTierRaw = tier
REDACTED
	if usage.GrokEntitlementStatus == "" {
		usage.GrokEntitlementStatus = strings.TrimSpace(account.GetCredential("entitlement_status"))
REDACTED
REDACTED

func grokBillingSnapshotFromExtra(extra map[string]any) (*xai.BillingSummary, error) {
	if extra == nil {
		return nil, nil
REDACTED
	raw, ok := extra[grokBillingExtraKey]
	if !ok || raw == nil {
		return nil, nil
REDACTED
	switch snapshot := raw.(type) {
	case *xai.BillingSummary:
		return snapshot, nil
	case xai.BillingSummary:
		return &snapshot, nil
	case map[string]any:
		data, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
	REDACTED
		var out xai.BillingSummary
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
	REDACTED
		return &out, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal grok billing snapshot: %w", err)
	REDACTED
		var out xai.BillingSummary
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
	REDACTED
		return &out, nil
REDACTED
REDACTED

func grokQuotaSnapshotFromExtra(extra map[string]any) (*xai.QuotaSnapshot, error) {
	if extra == nil {
		return nil, nil
REDACTED
	raw, ok := extra[grokQuotaSnapshotExtraKey]
	if !ok || raw == nil {
		return nil, nil
REDACTED
	switch snapshot := raw.(type) {
	case *xai.QuotaSnapshot:
		return snapshot, nil
	case xai.QuotaSnapshot:
		return &snapshot, nil
	case map[string]any:
		data, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
	REDACTED
		var out xai.QuotaSnapshot
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
	REDACTED
		return &out, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal grok quota snapshot: %w", err)
	REDACTED
		var out xai.QuotaSnapshot
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
	REDACTED
		return &out, nil
REDACTED
REDACTED
