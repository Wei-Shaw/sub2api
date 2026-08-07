package service

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// Spending-limit cools stay long (billing period), and the account is marked
// StatusError with a stable reauth-oriented message so the admin UI surfaces
// needs_reauth / ReAuth entry points. We do not hard-delete accounts.
const grokSpendingLimitCooldown = 24 * time.Hour

const grokSpendingLimitErrorMessage = "Grok spending limit reached; reauthorize or wait for billing reset"

// markGrokSpendingLimitReauth applies a long temp-unsched cool and durable
// SetError so ops sees reauth-required without wiping OAuth credentials.
func (s *OpenAIGatewayService) markGrokSpendingLimitReauth(ctx context.Context, account *Account) {
	if s == nil || account == nil || account.IsPoolMode() {
		return
REDACTED
	s.tempUnscheduleGrok(ctx, account, grokSpendingLimitCooldown, "grok spending limit")
	if s.accountRepo == nil {
		return
REDACTED
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if err := s.accountRepo.SetError(stateCtx, account.ID, grokSpendingLimitErrorMessage); err != nil {
		slog.Warn("grok_spending_limit_set_error_failed", "account_id", account.ID, "error", err)
REDACTED
	// Soft flag in extra for UI / usage fetcher without requiring status poll.
	_ = s.accountRepo.UpdateExtra(stateCtx, account.ID, map[string]any{
		"grok_needs_reauth":        true,
		"grok_needs_reauth_reason": "spending_limit",
		"grok_needs_reauth_at":     time.Now().UTC().Format(time.RFC3339),
REDACTED)
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
