package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const (
	grokModelDiagnosticStateUnknown       = "unknown"
	grokModelDiagnosticStateFresh         = "fresh"
	grokModelDiagnosticStateStale         = "stale"
	grokModelDiagnosticStateSourceChanged = "source_changed"

	grokModelDiagnosticStatusListed       = "listed"
	grokModelDiagnosticStatusNotListed    = "not_listed"
	grokModelDiagnosticStatusUnverifiable = "unverifiable"
	grokModelDiagnosticStatusOutOfScope   = "out_of_scope"
)

// GrokModelDiagnosticItem is one local mapping compared with an account catalog.
type GrokModelDiagnosticItem struct {
	Requested string `json:"requested"`
	Upstream  string `json:"upstream,omitempty"`
	Status    string `json:"status"`
}

// GrokModelDiagnostics is an admin-only view of catalog trust and mapping
// differences. It never changes routing or the stored account.
type GrokModelDiagnostics struct {
	State       string                    `json:"state"`
	Endpoint    string                    `json:"endpoint,omitempty"`
	FetchedAt   string                    `json:"fetched_at,omitempty"`
	ExpiresAt   string                    `json:"expires_at,omitempty"`
	Models      []string                  `json:"models"`
	LastAttempt *GrokCatalogAttempt       `json:"last_attempt,omitempty"`
	Mappings    []GrokModelDiagnosticItem `json:"mappings"`
}

// EvaluateGrokModelDiagnostics compares one account's saved mapping with a
// catalog observation. account and snapshot are not modified. Only a fresh,
// same-scope text catalog can report not_listed; other states may show the
// historical model list without claiming that something is currently absent.
func EvaluateGrokModelDiagnostics(account *Account, current *GrokCatalogScope, snapshot *GrokObservedModelsSnapshot, now time.Time) *GrokModelDiagnostics {
	diag := &GrokModelDiagnostics{
		State:    grokModelDiagnosticStateUnknown,
		Models:   []string{},
		Mappings: []GrokModelDiagnosticItem{},
	}
	if current != nil {
		diag.Endpoint = current.Endpoint
	}
	if snapshot != nil {
		if snapshot.Models != nil {
			diag.Models = append([]string{}, snapshot.Models...)
		}
		diag.FetchedAt = strings.TrimSpace(snapshot.FetchedAt)
		diag.LastAttempt = cloneGrokCatalogAttempt(snapshot.LastAttempt)
	}
	state, fetched, haveFetched := grokModelDiagnosticState(current, snapshot, now)
	diag.State = state
	if haveFetched {
		diag.ExpiresAt = fetched.UTC().Add(grokObservedModelsTTL).Format(time.RFC3339Nano)
	}
	diag.Mappings = grokDiagnosticMappings(account, state, grokDiagnosticCatalogSet(diag.Models))
	return diag
}

// GetGrokModelDiagnostics builds the models URL that a sync would use and
// compares it with the stored observation. It does not fetch a token, send
// HTTP, or write the account. Non-Grok accounts return nil.
func (s *AccountTestService) GetGrokModelDiagnostics(ctx context.Context, account *Account) *GrokModelDiagnostics {
	if s == nil || account == nil || !account.IsGrok() {
		return nil
	}
	return EvaluateGrokModelDiagnostics(account, s.grokModelDiagnosticsScope(ctx, account), parseGrokObservedModels(account.Extra), time.Now().UTC())
}

func (s *AccountTestService) grokModelDiagnosticsScope(ctx context.Context, account *Account) *GrokCatalogScope {
	modelsURL, err := s.grokModelDiagnosticsModelsURL(ctx, account)
	if err != nil || strings.TrimSpace(modelsURL) == "" {
		return nil
	}
	scope, err := NewGrokCatalogScope(account.ID, account.Type, modelsURL)
	if err != nil {
		return nil
	}
	return scope
}

func (s *AccountTestService) grokModelDiagnosticsModelsURL(ctx context.Context, account *Account) (string, error) {
	if s == nil || account == nil {
		return "", errGrokCatalogInvalidModelsURL
	}
	baseURL, err := s.grokModelDiagnosticsBaseURL(ctx, account)
	if err != nil {
		return "", err
	}
	return buildOpenAIModelsURL(baseURL), nil
}

func (s *AccountTestService) grokModelDiagnosticsBaseURL(ctx context.Context, account *Account) (string, error) {
	switch account.Type {
	case AccountTypeAPIKey:
		baseURL := strings.TrimSpace(account.GetCredential("base_url"))
		if baseURL == "" {
			baseURL = "https://api.x.ai"
		}
		return s.validateUpstreamBaseURL(baseURL)
	case AccountTypeOAuth:
		validator, err := grokBaseURLValidator(account, s.cfg)
		if err != nil {
			return "", err
		}
		baseURL := account.GetGrokBaseURL()
		if s.settingService != nil {
			baseURL = s.settingService.ResolveGrokBaseURL(ctx, account)
		}
		return validator(baseURL)
	default:
		return "", errGrokCatalogInvalidModelsURL
	}
}

func grokModelDiagnosticState(current *GrokCatalogScope, snapshot *GrokObservedModelsSnapshot, now time.Time) (string, time.Time, bool) {
	fetched, fetchedOK := time.Time{}, false
	if snapshot != nil {
		if parsed, ok := parseGrokCatalogTimestamp(snapshot.FetchedAt); ok && !parsed.After(now) {
			fetched, fetchedOK = parsed, true
		}
	}
	if current == nil || strings.TrimSpace(current.Fingerprint) == "" {
		return grokModelDiagnosticStateUnknown, fetched, fetchedOK
	}
	if snapshot == nil || snapshot.Scope == nil || strings.TrimSpace(snapshot.Scope.Fingerprint) == "" || !grokCatalogHasModelID(snapshot.Models) {
		return grokModelDiagnosticStateUnknown, fetched, fetchedOK
	}
	if !fetchedOK || fetched.After(now) {
		return grokModelDiagnosticStateUnknown, time.Time{}, false
	}
	if snapshot.Scope.Fingerprint != current.Fingerprint {
		return grokModelDiagnosticStateSourceChanged, fetched, true
	}
	if now.Sub(fetched) >= grokObservedModelsTTL {
		return grokModelDiagnosticStateStale, fetched, true
	}
	return grokModelDiagnosticStateFresh, fetched, true
}

func grokDiagnosticCatalogSet(models []string) map[string]struct{} {
	set := make(map[string]struct{}, len(models))
	for _, model := range models {
		observed := grokDiagnosticObservedID(model)
		if observed == "" {
			continue
		}
		set[observed] = struct{}{}
	}
	return set
}

func grokDiagnosticMappings(account *Account, state string, catalog map[string]struct{}) []GrokModelDiagnosticItem {
	items := []GrokModelDiagnosticItem{}
	if account == nil {
		return items
	}
	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		return items
	}
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		if strings.TrimSpace(key) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		items = append(items, grokDiagnosticItem(key, mapping[key], state, catalog))
	}
	return items
}

func grokDiagnosticItem(requested, target, state string, catalog map[string]struct{}) GrokModelDiagnosticItem {
	item := GrokModelDiagnosticItem{Requested: requested, Status: grokModelDiagnosticStatusUnverifiable}
	if strings.Contains(requested, "*") || strings.Contains(target, "*") {
		return item
	}
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return item
	}
	if grokDiagnosticIndependentCapability(trimmed) {
		item.Upstream = grokDiagnosticObservedID(trimmed)
		item.Status = grokModelDiagnosticStatusOutOfScope
		return item
	}
	// Responses rewrites bare grok / grok-latest to grok-4.5. Chat, Messages,
	// and the websocket bridge keep the literal alias. Neither generation is
	// a catalog fact, so do not compare grok-4.5 or grok-4.6.
	if grokDiagnosticBareAlias(trimmed) {
		item.Upstream = trimmed
		item.Status = grokModelDiagnosticStatusUnverifiable
		return item
	}
	canonical := grokDiagnosticObservedID(trimmed)
	item.Upstream = canonical
	_, inCatalog := catalog[canonical]
	switch {
	case inCatalog && (state == grokModelDiagnosticStateFresh || state == grokModelDiagnosticStateStale):
		item.Status = grokModelDiagnosticStatusListed
	case state == grokModelDiagnosticStateFresh && xai.IsGrokTextResponsesModelID(trimmed):
		item.Status = grokModelDiagnosticStatusNotListed
	default:
		item.Status = grokModelDiagnosticStatusUnverifiable
	}
	return item
}

// grokDiagnosticObservedID uses the text alias table only. Runtime audit treats
// some -build responses as the same model; a catalog check must not. Bare
// grok / grok-latest and unknown IDs stay as written: the alias table's
// fallback generation is not unique across forwards, and case is not evidence
// that a custom id is listed.
func grokDiagnosticObservedID(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" || grokDiagnosticBareAlias(trimmed) {
		return trimmed
	}
	return strings.TrimSpace(xai.ResolveGrokTextResponsesModelID(trimmed))
}

func grokDiagnosticBareAlias(model string) bool {
	normalized := strings.ToLower(xai.StripGrokProviderPrefix(strings.TrimSpace(model)))
	return normalized == "grok" || normalized == "grok-latest"
}

func grokDiagnosticIndependentCapability(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(xai.StripGrokProviderPrefix(model)))
	if normalized == "" {
		return false
	}
	if xai.IsGrokImagineModel(normalized) {
		return true
	}
	switch {
	case strings.Contains(normalized, "voice"),
		strings.Contains(normalized, "realtime"),
		strings.Contains(normalized, "audio"),
		strings.Contains(normalized, "video"),
		strings.HasPrefix(normalized, "tts"),
		strings.HasPrefix(normalized, "stt"),
		strings.Contains(normalized, "-tts"),
		strings.Contains(normalized, "-stt"),
		strings.Contains(normalized, "/tts"),
		strings.Contains(normalized, "/stt"):
		return true
	default:
		return false
	}
}
