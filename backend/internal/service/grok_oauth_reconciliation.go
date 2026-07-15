package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	defaultGrokOAuthReconcilePageSize = 50
	maxGrokOAuthReconcilePageSize     = 500
	maxGrokOAuthReconcileWindow       = 24 * time.Hour

	GrokOAuthReconcileReasonMissingRefreshToken = "missing_refresh_token"
	GrokOAuthReconcileReasonMissingAccessToken  = "missing_access_token"
	GrokOAuthReconcileReasonMissingExpiry       = "missing_expiry"
	GrokOAuthReconcileReasonInvalidExpiry       = "invalid_expiry"
	GrokOAuthReconcileReasonNearExpiry          = "near_expiry"
	GrokOAuthReconcileReasonCredentialRejected  = "credential_rejected"

	GrokOAuthReconcileActionBlock   = "block_account"
	GrokOAuthReconcileActionRefresh = "refresh_credentials"

	GrokOAuthReconcileOutcomePlanned = "planned"
	GrokOAuthReconcileOutcomeApplied = "applied"
	GrokOAuthReconcileOutcomeSkipped = "skipped"
	GrokOAuthReconcileOutcomeFailed  = "failed"
	GrokOAuthReconcileOutcomePartial = "partial"
)

var (
	ErrGrokOAuthReconcileMode = infraerrors.BadRequest(
		"GROK_OAUTH_RECONCILE_MODE_INVALID",
		"apply requires dry_run=false and apply=true",
	)
	ErrGrokOAuthReconcileCursor = infraerrors.BadRequest(
		"GROK_OAUTH_RECONCILE_CURSOR_INVALID",
		"after_id must be non-negative",
	)
	ErrGrokOAuthReconcileLimit = infraerrors.BadRequest(
		"GROK_OAUTH_RECONCILE_LIMIT_INVALID",
		"limit is outside the allowed reconciliation page range",
	)
	ErrGrokOAuthReconcileWindow = infraerrors.BadRequest(
		"GROK_OAUTH_RECONCILE_WINDOW_INVALID",
		"refresh_window_seconds is outside the allowed range",
	)
)

// GrokOAuthReconciler is the narrow admin-facing reconciliation port.
type GrokOAuthReconciler interface {
	ReconcileGrokOAuth(ctx context.Context, input GrokOAuthReconcileInput) (*GrokOAuthReconcileResult, error)
REDACTED

// GrokOAuthConditionalErrorRepository is the narrow compare-and-set mutation
// used by reconciliation. The repository must only transition an active Grok
// OAuth account when its credential document still exactly matches the state
// observed immediately before the mutation.
type GrokOAuthConditionalErrorRepository interface {
	SetGrokOAuthErrorIfCredentialsUnchanged(ctx context.Context, id int64, expectedCredentials map[string]any, errorMsg string) (bool, error)
REDACTED

type GrokOAuthReconcileInput struct {
	DryRun        bool
	Apply         bool
	AfterID       int64
	Limit         int
	RefreshWindow time.Duration
REDACTED

// GrokOAuthReconcileItem is deliberately metadata-only. Credentials, account
// identity fields, provider response bodies, and raw errors never cross this API.
type GrokOAuthReconcileItem struct {
	AccountID int64  `json:"account_id"`
	Reason    string `json:"reason"`
	Action    string `json:"action"`
	Outcome   string `json:"outcome"`
REDACTED

type GrokOAuthReconcileResult struct {
	DryRun       bool                     `json:"dry_run"`
	Scanned      int                      `json:"scanned"`
	Actionable   int                      `json:"actionable"`
	WouldBlock   int                      `json:"would_block"`
	WouldRefresh int                      `json:"would_refresh"`
	Blocked      int                      `json:"blocked"`
	Refreshed    int                      `json:"refreshed"`
	Skipped      int                      `json:"skipped"`
	Failed       int                      `json:"failed"`
	Partial      int                      `json:"partial"`
	Items        []GrokOAuthReconcileItem `json:"items"`
	NextAfterID  int64                    `json:"next_after_id"`
	HasMore      bool                     `json:"has_more"`
REDACTED

func (s *TokenRefreshService) ReconcileGrokOAuth(ctx context.Context, input GrokOAuthReconcileInput) (*GrokOAuthReconcileResult, error) {
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if input.Apply && input.DryRun {
		return nil, ErrGrokOAuthReconcileMode
REDACTED
	if input.AfterID < 0 {
		return nil, ErrGrokOAuthReconcileCursor
REDACTED
	limit := input.Limit
	maxPageSize := s.grokOAuthReconcileMaxPageSize()
	if limit == 0 {
		limit = min(defaultGrokOAuthReconcilePageSize, maxPageSize)
REDACTED
	if limit < 1 || limit > maxPageSize {
		return nil, ErrGrokOAuthReconcileLimit
REDACTED
	refreshWindow := input.RefreshWindow
	if refreshWindow == 0 {
		refreshWindow = grokTokenRefreshSkew
REDACTED
	if refreshWindow < 0 || refreshWindow > maxGrokOAuthReconcileWindow {
		return nil, ErrGrokOAuthReconcileWindow
REDACTED
	if refreshWindow < grokTokenRefreshSkew {
		refreshWindow = grokTokenRefreshSkew
REDACTED
	dryRun := !input.Apply

	pager := s.candidatePager
	if pager == nil {
		pager, _ = s.accountRepo.(OAuthRefreshCandidatePager)
REDACTED
	if pager == nil {
		return nil, errors.New("OAuth refresh candidate pager is not configured")
REDACTED
	page, err := pager.ListOAuthRefreshCandidatePage(ctx, OAuthRefreshPageOptions{
		Platforms:  []string{PlatformGrokREDACTED,
		AfterID:    input.AfterID,
		Limit:      limit,
		ActiveOnly: true,
		// Reconciliation scans OAuth only and intentionally does not require a
		// refresh token so structurally invalid rows remain discoverable.
		IncludeSetupToken:   false,
		RequireRefreshToken: false,
REDACTED)
	if err != nil {
		return nil, err
REDACTED
	if page == nil {
		return nil, errors.New("OAuth reconciliation repository returned a nil cursor page")
REDACTED
	accounts := page.Accounts
	if !isStrictlyIncreasingAccountPage(accounts, input.AfterID) {
		return nil, errors.New("OAuth reconciliation repository returned an invalid cursor page")
REDACTED

	result := &GrokOAuthReconcileResult{
		DryRun:  dryRun,
		Scanned: len(accounts),
		Items:   make([]GrokOAuthReconcileItem, 0, len(accounts)),
		HasMore: page.HasMore,
REDACTED
	if result.HasMore {
		if page.NextAfterID <= input.AfterID {
			return nil, errors.New("OAuth reconciliation repository returned invalid cursor metadata")
	REDACTED
		result.NextAfterID = page.NextAfterID
REDACTED

	registration, ok := s.grokRegistration()
	if !ok {
		return nil, errors.New("grok OAuth refresher is not registered")
REDACTED
	conditionalErrorRepo, supportsConditionalError := s.accountRepo.(GrokOAuthConditionalErrorRepository)
	if input.Apply && !supportsConditionalError {
		return nil, errors.New("grok OAuth conditional error mutation is not configured")
REDACTED
	providerState := &tokenRefreshProviderState{
		service:      s,
		registration: registration,
		rateGate:     s.providerRateGate(PlatformGrok),
		poolGate:     s.providerConcurrencyGate(PlatformGrok),
REDACTED

	for i := range accounts {
		if err := ctx.Err(); err != nil {
			return nil, err
	REDACTED
		account := &accounts[i]
		reason, action, actionable := classifyGrokOAuthReconcileAccount(account, refreshWindow)
		if !actionable {
			result.Skipped++
			continue
	REDACTED
		result.Actionable++
		item := GrokOAuthReconcileItem{
			AccountID: account.ID,
			Reason:    reason,
			Action:    action,
			Outcome:   GrokOAuthReconcileOutcomePlanned,
	REDACTED
		if action == GrokOAuthReconcileActionBlock {
			result.WouldBlock++
	REDACTED else {
			result.WouldRefresh++
	REDACTED
		if dryRun {
			result.Items = append(result.Items, item)
			continue
	REDACTED

		switch action {
		case GrokOAuthReconcileActionBlock:
			latest, err := s.accountRepo.GetByID(ctx, account.ID)
			if err != nil || latest == nil {
				item.Outcome = GrokOAuthReconcileOutcomeFailed
				result.Failed++
				break
		REDACTED
			latestReason, latestAction, stillActionable := classifyGrokOAuthReconcileAccount(latest, refreshWindow)
			if !stillActionable || latestAction != GrokOAuthReconcileActionBlock {
				// The account changed after page hydration (for example, an admin
				// reauthorized it). Never apply a stale destructive action; the next
				// resumable scan can plan the fresh state.
				item.Outcome = GrokOAuthReconcileOutcomeSkipped
				result.Skipped++
				break
		REDACTED
			account = latest
			item.Reason = latestReason
			applied, err := conditionalErrorRepo.SetGrokOAuthErrorIfCredentialsUnchanged(
				ctx,
				account.ID,
				account.Credentials,
				"Grok OAuth credential reconciliation: missing refresh token",
			)
			if err != nil {
				item.Outcome = GrokOAuthReconcileOutcomeFailed
				result.Failed++
				break
		REDACTED
			if !applied {
				// Reauthorization won the compare-and-set race after the final
				// reread. The runtime fast path is installed only after the CAS
				// succeeds, so the fresh active account remains untouched.
				item.Outcome = GrokOAuthReconcileOutcomeSkipped
				result.Skipped++
				break
		REDACTED
			s.notifyAccountSchedulingBlocked(account, time.Time{REDACTED, "grok_oauth_reconcile_invalid")
			account.Status = StatusError
			account.Schedulable = false
			cacheInvalidationFailed := s.cacheInvalidator == nil
			if s.cacheInvalidator != nil {
				if err := s.cacheInvalidator.InvalidateToken(ctx, account); err != nil {
					cacheInvalidationFailed = true
			REDACTED
		REDACTED
			result.Blocked++
			if cacheInvalidationFailed {
				item.Outcome = GrokOAuthReconcileOutcomePartial
				result.Partial++
		REDACTED else {
				item.Outcome = GrokOAuthReconcileOutcomeApplied
		REDACTED
		case GrokOAuthReconcileActionRefresh:
			if providerState.isTripped() {
				item.Outcome = GrokOAuthReconcileOutcomeSkipped
				result.Skipped++
				break
		REDACTED
			if providerState.isTripped() {
				item.Outcome = GrokOAuthReconcileOutcomeSkipped
				result.Skipped++
				break
		REDACTED
			refreshErr := s.refreshWithRetryWithRateGate(ctx, account, registration.refresher, registration.executor, refreshWindow, providerState)
			providerState.recordResult(refreshErr)
			var permanentErr *accountPermanentRefreshError
			switch {
			case refreshErr == nil:
				item.Outcome = GrokOAuthReconcileOutcomeApplied
				result.Refreshed++
			case errors.Is(refreshErr, errRefreshSkipped):
				item.Outcome = GrokOAuthReconcileOutcomeSkipped
				result.Skipped++
			case errors.As(refreshErr, &permanentErr) && permanentErr.persistentlyBlocked:
				item.Reason = GrokOAuthReconcileReasonCredentialRejected
				item.Action = GrokOAuthReconcileActionBlock
				result.Blocked++
				if permanentErr.cacheInvalidationFailed {
					item.Outcome = GrokOAuthReconcileOutcomePartial
					result.Partial++
			REDACTED else {
					item.Outcome = GrokOAuthReconcileOutcomeApplied
			REDACTED
			default:
				item.Outcome = GrokOAuthReconcileOutcomeFailed
				result.Failed++
		REDACTED
		default:
			return nil, fmt.Errorf("unsupported Grok OAuth reconciliation action")
	REDACTED
		result.Items = append(result.Items, item)
REDACTED
	return result, nil
REDACTED

func (s *TokenRefreshService) grokOAuthReconcileMaxPageSize() int {
	return maxGrokOAuthReconcilePageSize
REDACTED

func (s *TokenRefreshService) grokRegistration() (tokenRefreshRegistration, bool) {
	for _, registration := range s.registrations {
		if registration.platform == PlatformGrok && registration.refresher != nil {
			return registration, true
	REDACTED
REDACTED
	return tokenRefreshRegistration{REDACTED, false
REDACTED

func classifyGrokOAuthReconcileAccount(account *Account, refreshWindow time.Duration) (reason, action string, actionable bool) {
	if account == nil || !account.IsGrokOAuth() || account.Status != StatusActive {
		return "", "", false
REDACTED
	if strings.TrimSpace(account.GetGrokRefreshToken()) == "" {
		return GrokOAuthReconcileReasonMissingRefreshToken, GrokOAuthReconcileActionBlock, true
REDACTED
	if strings.TrimSpace(account.GetGrokAccessToken()) == "" {
		return GrokOAuthReconcileReasonMissingAccessToken, GrokOAuthReconcileActionRefresh, true
REDACTED
	rawExpiry := strings.TrimSpace(account.GetCredential("expires_at"))
	if rawExpiry == "" {
		return GrokOAuthReconcileReasonMissingExpiry, GrokOAuthReconcileActionRefresh, true
REDACTED
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return GrokOAuthReconcileReasonInvalidExpiry, GrokOAuthReconcileActionRefresh, true
REDACTED
	if time.Until(*expiresAt) <= refreshWindow {
		return GrokOAuthReconcileReasonNearExpiry, GrokOAuthReconcileActionRefresh, true
REDACTED
	return "", "", false
REDACTED
