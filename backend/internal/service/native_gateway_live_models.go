package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// Live model lists of the catalogue profiles: the API-key coding plans (GLM,
// MiniMax, Kimi), read with the key furnished only by the read-only quota
// credential lease, and NVIDIA Build, whose list is public and read without any
// key. The provider's own /models answer is read through Sub2API's existing
// builders and parser. The list is never evidence of plan membership: it is the
// owner's decision that the provider decides, with the static catalogue as the
// lower bound. Dispatch never reads the network and never takes the lease (see
// peek).
const (
	gatewayLiveModelsMax         = 512 // the Gateway contract bounds a catalogue snapshot to 512 models
	gatewayLiveModelsFreshFor    = 5 * time.Minute
	gatewayLiveModelsAdmitFor    = 24 * time.Hour
	gatewayLiveModelsEntriesMax  = 256
	gatewayLiveModelsFetchSlots  = 2
	gatewayLiveModelsTimeout     = 2 * time.Second
	gatewayLiveModelsFailBackoff = time.Minute // a failed read is not retried sooner, even when forced
	gatewayLiveModelsForceAfter  = time.Minute // a forced refresh is not honoured sooner after a good read
	gatewayLiveModelsLogMax      = 64          // excluded IDs named in one catalogue log line

	// gatewayLiveModelsPublicKey prefixes the one shared entry of a public list.
	gatewayLiveModelsPublicKey = "public_models_"
)

// gatewayLiveModelsEntry keeps the last SUCCESSFUL provider list; a failed fetch
// never replaces it.
type gatewayLiveModelsEntry struct {
	models  []string
	fetched time.Time // last SUCCESSFUL read; zero when there never was one
	failed  time.Time // last failed read; spaces out the next attempt
}

// GatewayLiveModels is a small in-memory store: one entry per encrypted physical
// principal (one per profile for a public list), single-flight refresh, bounded
// concurrency.
type GatewayLiveModels struct {
	mu      sync.Mutex
	entries map[string]gatewayLiveModelsEntry
	pending map[string]chan struct{}
	slots   chan struct{}
	now     func() time.Time
	client  *GatewayProviderQuotaCollector
}

// NewGatewayLiveModels builds the store. A supplied transport is a trusted
// fixture composition only; the host, deadline and redirect policy stay fixed in
// the collector.
func NewGatewayLiveModels(transport http.RoundTripper) *GatewayLiveModels {
	return &GatewayLiveModels{
		entries: map[string]gatewayLiveModelsEntry{},
		pending: map[string]chan struct{}{},
		slots:   make(chan struct{}, gatewayLiveModelsFetchSlots),
		now:     time.Now,
		client:  NewGatewayProviderQuotaCollector(transport),
	}
}

// liveModels returns the store, creating the default one on first use.
func (s *OpenAIGatewayService) liveModels() *GatewayLiveModels {
	s.gatewayLiveModelsOnce.Do(func() {
		if s.gatewayLiveModels == nil {
			s.gatewayLiveModels = NewGatewayLiveModels(nil)
		}
	})
	return s.gatewayLiveModels
}

// gatewayLiveModelsKey binds an entry to the exact encrypted account row: a
// rotated key or a new generation is a new principal. A public list is the same
// for every account of the profile, so it has one entry per profile.
func gatewayLiveModelsKey(profile string, scope GatewayNativeCredentialScope, envelope string) string {
	if _, public := gatewayLiveModelsSource(profile); public {
		return gatewayLiveModelsPublicKey + profile
	}
	encoded, _ := json.Marshal(scope)
	digest := sha256.Sum256(append(append(encoded, 0), envelope...))
	return "coding_plan_models_" + hex.EncodeToString(digest[:])
}

// peek never waits and never fetches: it is the only live read dispatch uses.
func (l *GatewayLiveModels) peek(key string) []string {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok || l.now().Sub(entry.fetched) >= gatewayLiveModelsAdmitFor {
		return nil
	}
	return slices.Clone(entry.models)
}

func (l *GatewayLiveModels) fresh(key string, force bool) ([]string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok {
		return nil, false
	}
	now := l.now()
	// A recent failure counts as fresh so that a broken endpoint is asked once a
	// minute, not once per request, whether or not the caller forces a refresh.
	recentFailure := !entry.failed.IsZero() && now.Sub(entry.failed) < gatewayLiveModelsFailBackoff
	// A forced refresh is honoured at most once a minute per entry, so repeated
	// refreshes (of the shared public entry in particular) cannot occupy the
	// fetch slots that every account uses.
	age := now.Sub(entry.fetched)
	recentSuccess := !entry.fetched.IsZero() && age < gatewayLiveModelsFreshFor && (!force || age < gatewayLiveModelsForceAfter)
	return slices.Clone(entry.models), recentFailure || recentSuccess
}

// evictLocked keeps the store bounded: expired entries first, then one arbitrary
// account entry (a dropped entry only narrows admission back to anchor and
// static). The shared entry of a public list is never the arbitrary victim: one
// account's read must not narrow admission for every other account.
func (l *GatewayLiveModels) evictLocked(key string) {
	if _, ok := l.entries[key]; ok || len(l.entries) < gatewayLiveModelsEntriesMax {
		return
	}
	for old, entry := range l.entries {
		if l.now().Sub(entry.fetched) >= gatewayLiveModelsAdmitFor && l.now().Sub(entry.failed) >= gatewayLiveModelsFailBackoff {
			delete(l.entries, old)
		}
	}
	if len(l.entries) >= gatewayLiveModelsEntriesMax {
		for old := range l.entries {
			if !strings.HasPrefix(old, gatewayLiveModelsPublicKey) {
				delete(l.entries, old)
				break
			}
		}
	}
}

func (l *GatewayLiveModels) store(key string, models []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.evictLocked(key)
	l.entries[key] = gatewayLiveModelsEntry{models: slices.Clone(models), fetched: l.now()}
}

// fail records a failed read and keeps the last good list untouched.
func (l *GatewayLiveModels) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.evictLocked(key)
	entry := l.entries[key]
	entry.failed = l.now()
	l.entries[key] = entry
}

// refresh updates the entry from the provider when it is not fresh. A busy slot
// or a concurrent refresh returns at once with whatever is stored; failure keeps
// the last good list and is remembered so it is not retried per request.
func (l *GatewayLiveModels) refresh(ctx context.Context, key string, force bool, fetch func(context.Context) ([]string, error)) {
	if l == nil || fetch == nil {
		return
	}
	if _, fresh := l.fresh(key, force); fresh {
		return
	}
	l.mu.Lock()
	if wait := l.pending[key]; wait != nil {
		l.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-wait:
		}
		return
	}
	select {
	case l.slots <- struct{}{}:
	default:
		l.mu.Unlock()
		return
	}
	done := make(chan struct{})
	l.pending[key] = done
	l.mu.Unlock()
	failed := true
	defer func() {
		<-l.slots
		l.mu.Lock()
		delete(l.pending, key)
		l.mu.Unlock()
		close(done)
		if failed {
			l.fail(key)
		}
	}()
	models, err := fetch(ctx)
	if err == nil {
		l.store(key, models)
		failed = false
	}
}

// gatewayLiveModelsSource is source-owned: the endpoint and whether the list is
// public come from the profile, never from the account, the request or a caller.
func gatewayLiveModelsSource(profile string) (base string, public bool) {
	switch profile {
	case GatewayKimiCodeResponsesProfile:
		return GatewayKimiCodeResponsesBaseURL, false
	case GatewayGLMCodingResponsesProfile:
		return GatewayGLMCodingResponsesBaseURL, false
	case GatewayMiniMaxMPlanResponsesProfile:
		return GatewayMiniMaxMPlanResponsesBaseURL, false
	case GatewayNVIDIABuildChatProfile:
		return GatewayNVIDIABuildBaseURL, true
	}
	return "", false
}

// FetchModels reads the provider's own model list. A keyed list takes the key
// furnished by the current scoped lease; a public list takes no key at all and
// sends no Authorization header. It reuses Sub2API's models URL builder and
// parser; only the entry id is used (no name fallback) and models that cannot
// review are dropped. Any non-2xx, redirect, malformed or empty answer is an
// error.
func (c *GatewayProviderQuotaCollector) FetchModels(ctx context.Context, profile string, credential []byte) ([]string, error) {
	base, public := gatewayLiveModelsSource(profile)
	if c == nil || c.client == nil || base == "" || public != (len(credential) == 0) || !public && !gatewayCredentialSafeForHeader(credential) {
		return nil, ErrGatewayProviderQuota
	}
	bounded, cancel := context.WithTimeout(ctx, gatewayLiveModelsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(bounded, http.MethodGet, buildOpenAIModelsURL(base), nil)
	if err != nil {
		return nil, ErrGatewayProviderQuota
	}
	if !public {
		req.Header.Set("Authorization", "Bearer "+string(credential))
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", xai.ProtectedUserAgent)
	resp, err := c.client.Do(req)
	if err != nil || resp == nil || resp.Body == nil {
		return nil, ErrGatewayProviderQuota
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ErrGatewayProviderQuota
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, gatewayProviderQuotaMaxBytes+1))
	if err != nil || len(body) > gatewayProviderQuotaMaxBytes || bounded.Err() != nil {
		return nil, ErrGatewayProviderQuota
	}
	ids, err := extractUpstreamModelIDsWithSelector(body, func(e upstreamModelEntry) string { return e.ID })
	if err != nil {
		return nil, ErrGatewayProviderQuota
	}
	// Filter before the bound so excluded models never take a place in it.
	valid := make([]string, 0, len(ids))
	excluded, invalid := []string{}, 0
	for _, id := range ids {
		switch {
		case !GatewayAccountModelIDValid(id):
			invalid++
		case !gatewayReviewModelID(id):
			excluded = append(excluded, id)
		case !slices.Contains(valid, id):
			valid = append(valid, id)
		}
	}
	sort.Strings(valid)
	if len(valid) > gatewayLiveModelsMax {
		valid = valid[:gatewayLiveModelsMax]
	}
	// Model IDs only: no key, URL or body. The excluded IDs show what the filter drops.
	event := struct {
		Event         string   `json:"event"`
		Profile       string   `json:"profile"`
		RawCount      int      `json:"raw_count"`
		ReturnedCount int      `json:"returned_count"`
		InvalidCount  int      `json:"invalid_count"`
		Excluded      []string `json:"excluded"`
	}{"gateway_native_live_model_catalog", profile, len(ids), len(valid), invalid, excluded[:min(len(excluded), gatewayLiveModelsLogMax)]}
	line, _ := json.Marshal(event)
	_, _ = fmt.Fprintln(os.Stderr, string(line))
	if len(valid) == 0 {
		return nil, ErrGatewayProviderQuota
	}
	return valid, nil
}

func gatewayCredentialSafeForHeader(credential []byte) bool {
	text := string(credential)
	return len(credential) > 0 && len(credential) <= 16384 && strings.TrimSpace(text) == text && !strings.ContainsAny(text, "\r\n")
}

// gatewayModelCatalogue is the account-models answer: the anchor first, the
// static models, then the last good live list, deduplicated and bounded. It is
// never empty and never an error status.
func gatewayModelCatalogue(profile, anchor string, live []string) []string {
	out := []string{anchor}
	add := func(id string) {
		if len(out) < gatewayLiveModelsMax && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	for _, id := range gatewayCatalogStaticModelIDs(profile) {
		add(id)
	}
	for _, id := range live {
		add(id)
	}
	return out
}

// gatewayModelCatalogueAdmitted is the one dispatch-time membership rule: the
// anchor, the static models or the last good live list (kept for 24 hours). It
// is read-only: no network, no lease, no waiting.
func (s *OpenAIGatewayService) gatewayModelCatalogueAdmitted(scope GatewayNativeCredentialScope, envelope, profile, model string) bool {
	if !GatewayModelCatalogProfile(profile) || !GatewayAccountModelIDValid(model) {
		return false
	}
	// Admit exactly what the account catalogue publishes: anchor, static, last good live list.
	static := gatewayCatalogStaticModelIDs(profile)
	return slices.Contains(gatewayModelCatalogue(profile, static[0], s.liveModels().peek(gatewayLiveModelsKey(profile, scope, envelope))), model)
}

// liveCatalogueModels builds the catalogue for one authorized account, refreshing
// the live list when it is not fresh. Failure is invisible to callers.
func (s *OpenAIGatewayService) liveCatalogueModels(ctx context.Context, in GatewayAccountModelsInput, route GatewayNativeRoute, account *Account, scope GatewayNativeCredentialScope) []GatewayAccountModel {
	live := s.liveModels()
	key := gatewayLiveModelsKey(route.Profile, scope, account.GetCredential("api_key"))
	_, public := gatewayLiveModelsSource(route.Profile)
	custody := gatewayNativeCustody(ctx)
	live.refresh(ctx, key, in.ForceRefresh, func(fetchCtx context.Context) ([]string, error) {
		// One bound for the whole read: waiting for the row lock counts too.
		fetchCtx, cancel := context.WithTimeout(fetchCtx, gatewayLiveModelsTimeout)
		defer cancel()
		if public {
			// A public list needs no key: nothing is decoded and no lease is taken.
			return live.client.FetchModels(fetchCtx, route.Profile, nil)
		}
		var models []string
		err := s.WithGatewayNativeQuotaCredential(fetchCtx, scope, route, custody, func(credential []byte, _ GatewayNativeOAuthPhysical) error {
			var e error
			models, e = live.client.FetchModels(fetchCtx, route.Profile, credential)
			return e
		})
		return models, err
	})
	return gatewayCatalogAccountModels(route.Profile, route.Model, live.peek(key))
}
