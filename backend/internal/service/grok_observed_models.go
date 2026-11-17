package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const (
	grokObservedModelsExtraKey = "grok_observed_models"
	grokObservedModelsTTL      = 6 * time.Hour
	grokObservedModelsTimeout  = 15 * time.Second
)

type grokObservedModelsSnapshot = GrokObservedModelsSnapshot

var grokObservedModelsFlight sync.Map // accountID -> *singleflight-ish in-flight

// scheduleGrokObservedModelsSync best-effort fetches upstream /v1/models for a
// Grok OAuth account and stores IDs in Extra. Never blocks request path long;
// callers should fire-and-forget after successful auth/probe.
func (s *GrokQuotaService) scheduleGrokObservedModelsSync(account *Account) {
	if s == nil || account == nil || !account.IsGrokOAuth() || s.accountRepo == nil {
		return
	}
	id := account.ID
	if _, loaded := grokObservedModelsFlight.LoadOrStore(id, struct{}{}); loaded {
		return
	}
	// Copy credentials for background use.
	acc := *account
	go func() {
		defer grokObservedModelsFlight.Delete(id)
		ctx, cancel := context.WithTimeout(context.Background(), grokObservedModelsTimeout)
		defer cancel()
		if err := s.syncGrokObservedModels(ctx, &acc); err != nil {
			kind, statusCode := grokObservedModelsSyncLogFields(err)
			slog.Debug("grok_observed_models_sync_failed",
				"account_id", id,
				"error_kind", kind,
				"status_code", statusCode,
			)
		}
	}()
}

func (s *GrokQuotaService) syncGrokObservedModels(ctx context.Context, account *Account) error {
	if s == nil || account == nil {
		return nil
	}
	started := time.Now().UTC()
	req, proxyURL, err := s.prepareGrokObservedModelsRequest(ctx, account)
	if err != nil {
		return s.finishGrokObservedModelsAttempt(ctx, account, nil, started, err)
	}
	if req == nil {
		return nil
	}
	scope := grokCatalogScopeFromRequest(account, req)
	if grokObservedSnapshotFresh(parseGrokObservedModels(account.Extra), scope, time.Now().UTC()) {
		return nil
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return s.finishGrokObservedModelsAttempt(ctx, account, req, started, newUpstreamModelSyncUpstreamError("Failed to request upstream model list", err))
	}
	defer func() { _ = resp.Body.Close() }()

	limit := resolveModelsListReadLimit(s.cfg)
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return s.finishGrokObservedModelsAttempt(ctx, account, req, started, newUpstreamModelSyncUpstreamError("Failed to read upstream model list", err))
	}
	if int64(len(body)) > limit {
		return s.finishGrokObservedModelsAttempt(ctx, account, req, started, newUpstreamModelSyncUpstreamError("Upstream model list response is too large", fmt.Errorf("response exceeds %d bytes", limit)))
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return s.finishGrokObservedModelsAttempt(ctx, account, req, started, &UpstreamModelSyncError{
			Kind:       UpstreamModelSyncErrorUpstream,
			Message:    fmt.Sprintf("Upstream model list request failed with HTTP %d", resp.StatusCode),
			StatusCode: resp.StatusCode,
			Err:        fmt.Errorf("upstream model list returned HTTP %d", resp.StatusCode),
		})
	}
	ids, err := extractGrokModelIDsFromModelsBody(body)
	if err != nil {
		if errors.Is(err, errGrokCatalogEmpty) {
			return s.finishGrokObservedModelsAttempt(ctx, account, req, started, newUpstreamModelSyncUpstreamError("Upstream returned no supported models", err))
		}
		return s.finishGrokObservedModelsAttempt(ctx, account, req, started, newUpstreamModelSyncUpstreamError("Upstream model list response was not valid JSON", err))
	}
	if len(ids) == 0 {
		return s.finishGrokObservedModelsAttempt(ctx, account, req, started, newUpstreamModelSyncUpstreamError("Upstream returned no supported models", errGrokCatalogEmpty))
	}
	observation := buildGrokCatalogObservation(account, req, started, ids, nil)
	if err := s.saveGrokObservedObservation(ctx, account, &observation); err != nil {
		return newUpstreamModelSyncInternalError("Failed to save upstream model catalog", err)
	}
	return nil
}

func (s *GrokQuotaService) prepareGrokObservedModelsRequest(ctx context.Context, account *Account) (*http.Request, string, error) {
	token := strings.TrimSpace(account.GetGrokAccessToken())
	if token == "" && s.tokenProvider != nil {
		// Best-effort warm; avoid forcing refresh storms.
		if at, err := s.tokenProvider.GetAccessToken(ctx, account); err == nil {
			token = strings.TrimSpace(at)
		}
	}
	if token == "" {
		return nil, "", newUpstreamModelSyncConfigError("No Grok access token is available", nil)
	}
	baseURL := strings.TrimSpace(account.GetGrokBaseURL())
	if s.settingService != nil {
		baseURL = strings.TrimSpace(s.settingService.ResolveGrokBaseURL(ctx, account))
	}
	if baseURL == "" {
		baseURL = xai.DefaultCLIBaseURL
	}
	validator, err := grokBaseURLValidator(account, s.cfg)
	if err != nil {
		return nil, "", newUpstreamModelSyncConfigError("Invalid Grok base URL", err)
	}
	validatedBaseURL, err := validator(baseURL)
	if err != nil {
		return nil, "", newUpstreamModelSyncConfigError("Invalid Grok base URL", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildOpenAIModelsURL(validatedBaseURL), nil)
	if err != nil {
		return nil, "", newUpstreamModelSyncConfigError("Invalid Grok model list URL", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultGrokUpstreamUserAgent())
	if account.IsGrokOAuth() {
		applyGrokCLIHeaders(req.Header)
		if isGrokCLIProxyTarget(req.URL.String()) {
			if userID := strings.TrimSpace(account.GetCredential("sub")); userID != "" {
				req.Header.Set("X-UserID", userID)
			}
			if email := strings.TrimSpace(account.GetCredential("email")); email != "" {
				req.Header.Set("X-Email", email)
			}
		}
	}
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if s.proxyRepo != nil && account.ProxyID != nil {
		if p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && p != nil {
			proxyURL = p.URL()
		}
	}
	if s.httpUpstream == nil {
		return nil, "", nil
	}
	return req, proxyURL, nil
}

func (s *GrokQuotaService) finishGrokObservedModelsAttempt(ctx context.Context, account *Account, req *http.Request, started time.Time, cause error) error {
	observation := buildGrokCatalogObservation(account, req, started, nil, cause)
	if err := s.saveGrokObservedObservation(ctx, account, &observation); err != nil {
		slog.Warn("grok model catalog observation was not saved",
			"account_id", account.ID,
			"error", err,
		)
	}
	return cause
}

func (s *GrokQuotaService) saveGrokObservedObservation(ctx context.Context, account *Account, observation *GrokCatalogObservation) error {
	if s == nil || account == nil || account.ID <= 0 || observation == nil || s.accountRepo == nil {
		return nil
	}
	snap, err := PersistGrokCatalogObservation(ctx, s.accountRepo, account.ID, *observation)
	if err != nil {
		return err
	}
	rememberGrokObservedSnapshot(account, snap)
	return nil
}

func extractGrokModelIDsFromModelsBody(body []byte) ([]string, error) {
	return parseGrokCatalogModelIDs(body)
}

func parseGrokObservedModels(extra map[string]any) *grokObservedModelsSnapshot {
	if extra == nil {
		return nil
	}
	raw, ok := extra[grokObservedModelsExtraKey]
	if !ok || raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var snap grokObservedModelsSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil
	}
	if len(snap.Models) == 0 && snap.LastAttempt == nil {
		return nil
	}
	return &snap
}

func buildGrokCatalogObservation(account *Account, req *http.Request, started time.Time, models []string, cause error) GrokCatalogObservation {
	finished := time.Now().UTC()
	if !started.IsZero() && finished.Before(started) {
		finished = started
	}
	if started.IsZero() {
		started = finished
	}
	attempt := GrokCatalogAttempt{
		StartedAt:  started.UTC().Format(time.RFC3339Nano),
		FinishedAt: finished.UTC().Format(time.RFC3339Nano),
		Scope:      grokCatalogScopeFromRequest(account, req),
	}
	if cause == nil {
		attempt.Status = grokCatalogStatusSuccess
		return GrokCatalogObservation{Models: append([]string(nil), models...), Attempt: attempt}
	}
	attempt.Status = grokCatalogStatusFailed
	attempt.ErrorCode = grokCatalogErrorCode(cause)
	return GrokCatalogObservation{Attempt: attempt}
}

func grokCatalogScopeFromRequest(account *Account, req *http.Request) *GrokCatalogScope {
	if account == nil || req == nil || req.URL == nil {
		return nil
	}
	scope, err := NewGrokCatalogScope(account.ID, account.Type, req.URL.String())
	if err != nil {
		return nil
	}
	return scope
}

func grokObservedModelsSyncLogFields(err error) (string, int) {
	var syncErr *UpstreamModelSyncError
	if errors.As(err, &syncErr) {
		return string(syncErr.Kind), syncErr.StatusCode
	}
	return "transport", 0
}

func grokCatalogErrorCode(err error) string {
	if errors.Is(err, errGrokCatalogEmpty) {
		return "empty_catalog"
	}
	var syncErr *UpstreamModelSyncError
	if !errors.As(err, &syncErr) {
		return "transport"
	}
	switch syncErr.Kind {
	case UpstreamModelSyncErrorConfiguration, UpstreamModelSyncErrorUnsupported:
		return "configuration"
	case UpstreamModelSyncErrorInternal:
		return "persistence"
	}
	switch {
	case strings.Contains(syncErr.Message, "too large"):
		return "too_large"
	case strings.Contains(syncErr.Message, "no supported models"):
		return "empty_catalog"
	case strings.Contains(syncErr.Message, "not valid JSON"):
		return "invalid_response"
	case syncErr.StatusCode != 0:
		return "http_error"
	default:
		return "transport"
	}
}

func grokObservedSnapshotFresh(snap *GrokObservedModelsSnapshot, scope *GrokCatalogScope, now time.Time) bool {
	if snap == nil || scope == nil || strings.TrimSpace(scope.Fingerprint) == "" || !grokCatalogHasModelID(snap.Models) {
		return false
	}
	if snap.Scope == nil || snap.Scope.Fingerprint != scope.Fingerprint {
		return false
	}
	fetched, ok := parseGrokCatalogTimestamp(snap.FetchedAt)
	if !ok || fetched.After(now) {
		return false
	}
	return now.Sub(fetched) < grokObservedModelsTTL
}

func parseGrokCatalogTimestamp(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, strings.TrimSpace(value))
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func rememberGrokObservedSnapshot(account *Account, snap *GrokObservedModelsSnapshot) {
	if account == nil || snap == nil {
		return
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[grokObservedModelsExtraKey] = decoded
}
