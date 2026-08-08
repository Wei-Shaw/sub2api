package service

import (
	"context"
	"strings"
	"time"
)

// Spending-limit is recoverable at the end of the observed billing period.
// When no billing snapshot is available, use a short probe rather than
// fabricating a 24h boundary from the error arrival time.
const grokSpendingLimitProbeCooldown = 10 * time.Minute

func grokSpendingLimitResetAt(account *Account, now time.Time) time.Time {
	if account != nil {
		if billing, err := grokBillingSnapshotFromExtra(account.Extra); err == nil && billing != nil {
			for _, raw := range []string{billing.PeriodEnd, billing.BillingPeriodEndREDACTED {
				if resetAt, err := time.Parse(time.RFC3339, strings.TrimSpace(raw)); err == nil && resetAt.After(now) {
					return resetAt
			REDACTED
		REDACTED
	REDACTED
REDACTED
	return now.Add(grokSpendingLimitProbeCooldown)
REDACTED

// clearGrokNeedsReauthExtra drops the soft reauth flag after successful refresh
// or reauth. Best-effort; never fails the request path.
func clearGrokNeedsReauthExtra(ctx context.Context, repo AccountRepository, accountID int64) {
	if repo == nil || accountID <= 0 {
		return
REDACTED
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	_ = repo.UpdateExtra(stateCtx, accountID, map[string]any{
		"grok_needs_reauth":        false,
		"grok_needs_reauth_reason": "",
		"grok_needs_reauth_at":     "",
REDACTED)
REDACTED

func accountGrokNeedsReauth(account *Account) bool {
	if account == nil {
		return false
REDACTED
	if account.Status == StatusError {
		msg := strings.ToLower(account.ErrorMessage)
		if strings.Contains(msg, "spending limit") || strings.Contains(msg, "reauthorize") {
			return true
	REDACTED
REDACTED
	if v, ok := account.Extra["grok_needs_reauth"].(bool); ok && v {
		return true
REDACTED
	if s, ok := account.Extra["grok_needs_reauth"].(string); ok {
		return strings.EqualFold(s, "true") || s == "1"
REDACTED
	return false
REDACTED
