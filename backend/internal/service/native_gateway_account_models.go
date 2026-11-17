package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaymodelcatalog"
)

// Model selection is mutable account capability; Route.Model remains the
// immutable qualification/birth anchor. This private catalogue never projects
// provider instructions, tokens, labels, or arbitrary manifest metadata.
type GatewayAccountModelsInput struct {
	GatewayAccountLimitsInput
	ForceRefresh bool `json:"forceRefresh,omitempty"`
}
type GatewayAccountModelChoices = gatewaymodelcatalog.Choices
type GatewayAccountModel struct {
	ModelID   string                      `json:"modelId"`
	Reasoning *GatewayAccountModelChoices `json:"reasoning"`
	Fast      *GatewayAccountModelChoices `json:"fast"`
	Tools     GatewayAccountModelChoices  `json:"tools"`
}
type GatewayAccountModelsResult struct {
	Status             string                 `json:"status"`
	PhysicalGeneration string                 `json:"physicalGeneration,omitempty"`
	PrincipalVersion   string                 `json:"principalVersion,omitempty"`
	Source             string                 `json:"source,omitempty"`
	FetchedAt          string                 `json:"fetchedAt,omitempty"`
	ExpiresAt          string                 `json:"expiresAt,omitempty"`
	Models             *[]GatewayAccountModel `json:"models,omitempty"`
}

var gatewayAccountModelID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)

func GatewayAccountModelIDValid(id string) bool { return gatewayAccountModelID.MatchString(id) }

// Pinned Codex 0.147 converts semantic Ultra to Responses wire Max. Apply
// this only to vendor metadata; submitted request bytes must be canonical.
func gatewayCodexCatalogWireEffort(e string) string {
	if e == "ultra" {
		return "max"
	}
	return e
}

type gatewayModelsEntry struct {
	result  GatewayAccountModelsResult
	expires time.Time
}
type gatewayModelsCache struct {
	sync.Mutex
	entries map[string]gatewayModelsEntry
	pending map[string]chan struct{}
}

type GatewayAccountModelsReader struct {
	dispatch   *GatewayNativeOAuthDispatch
	openRouter *OpenAIGatewayService
	authorize  GatewayAccountLimitsAuthorizer
}

func NewGatewayAccountModelsReader(d *GatewayNativeOAuthDispatch, openRouter *OpenAIGatewayService, a GatewayAccountLimitsAuthorizer) (*GatewayAccountModelsReader, error) {
	if a == nil {
		return nil, ErrGatewayNativeIdentity
	}
	return &GatewayAccountModelsReader{d, openRouter, a}, nil
}
func (s *GatewayAccountModelsReader) Read(ctx context.Context, in GatewayAccountModelsInput) GatewayAccountModelsResult {
	unavailable := GatewayAccountModelsResult{Status: "unavailable"}
	if s == nil || !in.Valid() {
		return unavailable
	}
	before, err := s.authorize(ctx, in.GatewayAccountLimitsInput)
	if err != nil || ctx.Err() != nil {
		return unavailable
	}
	if in.ProfileID == GatewayOpenRouterResponsesProfile {
		return s.openRouter.readGatewayOpenRouterAccountModels(ctx, in, before, s.authorize)
	}
	if GatewayModelCatalogProfile(in.ProfileID) {
		return s.readNativeCatalogAccountModels(ctx, in, before)
	}
	if in.ProfileID != GatewayCodexOAuthResponsesProfile {
		return GatewayAccountModelsResult{Status: "unsupported"}
	}
	if s.dispatch == nil || !gatewayAccountLimitsRouteValid(before, in.GatewayAccountLimitsInput) {
		return unavailable
	}
	ctx, err = WithGatewayNativeOAuthOwner(ctx, in.OwnerRef)
	if err != nil {
		return unavailable
	}
	repository, ok := s.dispatch.repository.(GatewayNativeOAuthCanonicalRepository)
	if !ok {
		return unavailable
	}
	scope, operation, err := repository.ResolveGatewayNativeOAuthCanonical(ctx, before, in.AccountRef)
	if err != nil || scope.Owner != in.OwnerRef || scope.Account != in.AccountRef || scope.Generation != before.Generation || !GatewayNativeOAuthScopeAuthorized(ctx, scope) {
		return unavailable
	}
	a, p, release, err := s.dispatch.repository.LockGatewayNativeOAuthDispatch(ctx, scope, operation, before.AccountID)
	if err != nil || release == nil {
		return unavailable
	}
	if gatewayOAuthPhysicalValid(a, p) != nil || p.QualifiedAt.IsZero() || !SameGatewayNativeDescriptor(before, p.Route) {
		release()
		return unavailable
	}
	a = snapshotGatewayNativeAccount(a)
	release()
	out := s.dispatch.accountModels(ctx, a, p, in.ForceRefresh)
	after, err := s.authorize(ctx, in.GatewayAccountLimitsInput)
	if err != nil || ctx.Err() != nil || !SameGatewayNativeDescriptor(before, after) {
		return unavailable
	}
	fresh, current, release, err := s.dispatch.repository.LockGatewayNativeOAuthDispatch(ctx, scope, operation, before.AccountID)
	if err != nil || release == nil {
		return unavailable
	}
	defer release()
	oldVersion, oldErr := gatewayOAuthCredentialVersion(a)
	version, versionErr := gatewayOAuthCredentialVersion(fresh)
	if oldErr != nil || versionErr != nil || oldVersion != version || gatewayOAuthPhysicalValid(fresh, current) != nil || !sameGatewayOAuthPhysical(p, current) || current.RefreshFence == nil || current.RefreshFence.Check(ctx) != nil {
		return unavailable
	}
	return out
}

// Native alone answers the bounded model catalogue of one exact managed API-key
// account (NVIDIA Build and the coding plans): the static lower bound plus the
// provider's last good live list. This is NOT an upstream entitlement/credit
// observation. It does not decrypt the key itself; a coding plan list is read
// through the read-only quota lease and the public NVIDIA list needs no key.
// Owner, physical generation and current encrypted account are checked again.
func (s *GatewayAccountModelsReader) readNativeCatalogAccountModels(
	ctx context.Context, in GatewayAccountModelsInput, before GatewayNativeRoute,
) GatewayAccountModelsResult {
	unavailable := GatewayAccountModelsResult{Status: "unavailable"}
	if s.openRouter == nil || s.openRouter.accountRepo == nil ||
		before.Profile != in.ProfileID ||
		before.AccountID <= 0 || before.CreatedAt.IsZero() || before.CreatedAt.Nanosecond()%1000 != 0 ||
		!GatewayNativeAPIKeyProfileValid(before.Profile, before.BaseURL, before.Model) {
		return unavailable
	}
	account, err := s.openRouter.accountRepo.GetByID(ctx, before.AccountID)
	if err != nil || validateGatewayNativeRoute(before, account) != nil {
		return unavailable
	}
	scope, err := GatewayNativeCredentialScopeForAccount(account)
	if err != nil || scope.Owner != in.OwnerRef || scope.Account != in.AccountRef ||
		scope.Generation != before.Generation {
		return unavailable
	}
	// The catalogue is built after the account and scope are authorized and before
	// the final fence below; a failed fence discards it.
	models := s.openRouter.liveCatalogueModels(ctx, in, before, account, scope)
	after, err := s.authorize(ctx, in.GatewayAccountLimitsInput)
	if err != nil || ctx.Err() != nil || !SameGatewayNativeDescriptor(before, after) {
		return unavailable
	}
	fresh, err := s.openRouter.accountRepo.GetByID(ctx, before.AccountID)
	if err != nil || validateGatewayNativeRoute(before, fresh) != nil ||
		!reflect.DeepEqual(account.Credentials, fresh.Credentials) ||
		!reflect.DeepEqual(account.Extra, fresh.Extra) {
		return unavailable
	}
	now := time.Now().UTC()
	return GatewayAccountModelsResult{
		Status: "snapshot", Source: gatewayModelCatalogSource(before.Profile), PhysicalGeneration: before.Generation,
		PrincipalVersion: in.AccountRef,
		FetchedAt:        now.Format(time.RFC3339Nano),
		ExpiresAt:        now.Add(time.Minute).Format(time.RFC3339Nano),
		Models:           &models,
	}
}

// A private, custody-bound GET only. It does not create an ordinary credential
// Account, schedule refreshes, mutate account status, or call the admin picker.
func (s *GatewayNativeOAuthDispatch) accountModels(ctx context.Context, a *Account, p GatewayNativeOAuthPhysical, force bool) GatewayAccountModelsResult {
	unavailable := GatewayAccountModelsResult{Status: "unavailable"}
	version, err := gatewayOAuthCredentialVersion(a)
	if err != nil || gatewayOAuthPhysicalValid(a, p) != nil || p.QualifiedAt.IsZero() {
		return unavailable
	}
	if s.modelVersions != nil {
		s.modelVersions.Refresh(ctx, force)
	}
	identity := resolveCodexOutboundIdentity("")
	principal := GatewayAccountLimitsPrincipal(p)
	key := principal + ":" + strconv.FormatInt(version, 10) + ":" + identity.version
	return s.models.read(ctx, key, principal+":", force, func() GatewayAccountModelsResult {
		return s.fetchAccountModels(ctx, a, p, version, principal, identity)
	})
}

// One bounded per-principal snapshot/single-flight mechanism serves both sources.
func (cache *gatewayModelsCache) read(ctx context.Context, key, obsoletePrefix string, force bool, fetch func() GatewayAccountModelsResult) GatewayAccountModelsResult {
	unavailable := GatewayAccountModelsResult{Status: "unavailable"}
	for {
		cache.Lock()
		if cache.entries == nil {
			cache.entries = map[string]gatewayModelsEntry{}
			cache.pending = map[string]chan struct{}{}
		}
		if pending := cache.pending[key]; pending != nil {
			cache.Unlock()
			select {
			case <-ctx.Done():
				return unavailable
			case <-pending:
				force = false
				continue
			}
		}
		if entry, ok := cache.entries[key]; ok && !force && time.Now().Before(entry.expires) {
			cache.Unlock()
			return entry.result
		}
		// Drop obsolete revisions of this physical principal on every new fetch.
		for old, entry := range cache.entries {
			if (obsoletePrefix != "" && strings.HasPrefix(old, obsoletePrefix)) || !time.Now().Before(entry.expires) {
				delete(cache.entries, old)
			}
		}
		if len(cache.entries) >= 256 {
			for old := range cache.entries {
				delete(cache.entries, old)
				break
			}
		}
		pending := make(chan struct{})
		cache.pending[key] = pending
		cache.Unlock()
		out := fetch()
		ttl := 5 * time.Second
		if out.Status == "snapshot" {
			ttl = 60 * time.Second
		}
		cache.Lock()
		expiry := time.Now().Add(ttl)
		if out.Status == "snapshot" {
			expiry, _ = time.Parse(time.RFC3339Nano, out.ExpiresAt)
		}
		cache.entries[key] = gatewayModelsEntry{out, expiry}
		delete(cache.pending, key)
		close(pending)
		cache.Unlock()
		return out
	}
}
func (s *GatewayNativeOAuthDispatch) fetchAccountModels(ctx context.Context, a *Account, p GatewayNativeOAuthPhysical, version int64, principal string, identity codexOutboundIdentity) GatewayAccountModelsResult {
	unavailable := GatewayAccountModelsResult{Status: "unavailable"}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	bundle, err := s.custody.openOAuthVersion(a.GetCredential("oauth_bundle"), p.Scope, version)
	if err != nil {
		return unavailable
	}
	endpoint := GatewayCodexOAuthBaseURL + "/models?client_version=" + identity.version
	request, err := http.NewRequestWithContext(bounded, http.MethodGet, endpoint, nil)
	if err != nil {
		return unavailable
	}
	request.Header.Set("Authorization", "Bearer "+bundle.AccessToken)
	request.Header.Set("ChatGPT-Account-Id", p.ProviderAccount)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Originator", identity.originator)
	request.Header.Set("User-Agent", identity.userAgent)
	request.Header.Set("Version", identity.version)
	bundle = GatewayNativeOAuthBundle{}
	response, err := s.client.Do(request)
	if err != nil {
		return unavailable
	}
	if response.Body == nil {
		return unavailable
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK || response.Request == nil || response.Request.URL.String() != endpoint {
		return unavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return unavailable
	}
	projection := gatewayCodexCatalogProjection{}
	models, err := projectGatewayCodexAccountModels(raw, &projection)
	event := struct {
		Event         string         `json:"event"`
		ClientVersion string         `json:"client_version"`
		RawCount      int            `json:"raw_count"`
		ReturnedCount int            `json:"returned_count"`
		Excluded      map[string]int `json:"excluded_counts"`
	}{"gateway_native_codex_model_catalog", identity.version, projection.raw, len(models), projection.excluded}
	if err != nil {
		event.ReturnedCount = 0
		event.Excluded = map[string]int{"invalid_manifest": 1}
	}
	line, _ := json.Marshal(event)
	_, _ = fmt.Fprintln(os.Stderr, string(line))
	if err != nil {
		return unavailable
	}
	now := time.Now().UTC()
	return GatewayAccountModelsResult{Status: "snapshot", PhysicalGeneration: p.Scope.Generation, PrincipalVersion: principal, Source: "codex", FetchedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano), Models: &models}
}

type gatewayCodexCatalogProjection struct {
	raw      int
	excluded map[string]int
}

func projectGatewayCodexAccountModels(raw []byte, projection *gatewayCodexCatalogProjection) ([]GatewayAccountModel, error) {
	root, ok := GatewayNativePayloadFields(raw, "models")
	if !ok {
		return nil, ErrGatewayNativeIdentity
	}
	var entries []json.RawMessage
	if string(root["models"]) == "null" || json.Unmarshal(root["models"], &entries) != nil || entries == nil || len(entries) > 128 {
		return nil, ErrGatewayNativeIdentity
	}
	projection.raw = len(entries)
	projection.excluded = map[string]int{}
	out := []GatewayAccountModel{}
	seen := map[string]bool{}
	for _, rawModel := range entries {
		fields, ok := GatewayNativePayloadFields(rawModel, "slug", "id", "visibility", "supported_in_api", "supported_endpoints", "input_modalities", "output_modalities", "supported_reasoning_levels", "default_reasoning_level", "disabled", "available", "status")
		if !ok {
			return nil, ErrGatewayNativeIdentity
		}
		var slug, id string
		if value := fields["slug"]; value != nil && json.Unmarshal(value, &slug) != nil {
			return nil, ErrGatewayNativeIdentity
		}
		if value := fields["id"]; value != nil && json.Unmarshal(value, &id) != nil {
			return nil, ErrGatewayNativeIdentity
		}
		if slug == "" {
			slug = id
		}
		if !GatewayAccountModelIDValid(slug) || id != "" && id != slug || seen[slug] {
			return nil, ErrGatewayNativeIdentity
		}
		seen[slug] = true
		exclusion := ""
		exclude := func(reason string) {
			if exclusion == "" {
				exclusion = reason
			}
		}
		// A review token marks a review-only model; preview is an ordinary ID.
		for _, token := range strings.FieldsFunc(strings.ToLower(slug), func(r rune) bool { return r == '-' || r == '_' || r == '.' || r == ':' || r == '/' }) {
			if token == "review" {
				exclude("review_only")
			}
		}
		// supported_in_api describes API-key access. ChatGPT OAuth includes false.
		if value := fields["supported_in_api"]; value != nil {
			var supported bool
			if string(value) == "null" || json.Unmarshal(value, &supported) != nil {
				return nil, ErrGatewayNativeIdentity
			}
		}
		for name, want := range map[string]bool{"available": true, "disabled": false} {
			if value := fields[name]; value != nil {
				var b bool
				if string(value) == "null" || json.Unmarshal(value, &b) != nil {
					return nil, ErrGatewayNativeIdentity
				}
				if b != want {
					exclude("unavailable")
				}
			}
		}
		if value := fields["visibility"]; value != nil {
			var v string
			if json.Unmarshal(value, &v) != nil {
				return nil, ErrGatewayNativeIdentity
			}
			// Visibility controls picker presentation, not account access.
			if v != "list" && v != "hide" && v != "none" {
				return nil, ErrGatewayNativeIdentity
			}
		}
		if value := fields["status"]; value != nil {
			var v string
			if json.Unmarshal(value, &v) != nil {
				return nil, ErrGatewayNativeIdentity
			}
			if v != "active" && v != "available" {
				exclude("unavailable")
			}
		}
		for _, capability := range []struct{ name, want string }{{"supported_endpoints", "responses"}, {"input_modalities", "text"}, {"output_modalities", "text"}} {
			name, want := capability.name, capability.want
			if value := fields[name]; value != nil {
				var values []string
				if json.Unmarshal(value, &values) != nil {
					return nil, ErrGatewayNativeIdentity
				}
				found := false
				for _, v := range values {
					if v == want {
						found = true
					}
				}
				if !found {
					exclude(name)
				}
			}
		}
		if exclusion != "" {
			projection.excluded[exclusion]++
			continue
		}
		m := GatewayAccountModel{ModelID: slug, Fast: &GatewayAccountModelChoices{Allowed: []string{"balanced"}, Default: "balanced"}, Tools: GatewayAccountModelChoices{Allowed: []string{"function"}, Default: "function"}}
		var def string
		if value := fields["default_reasoning_level"]; value != nil && string(value) != "null" {
			if json.Unmarshal(value, &def) != nil {
				return nil, ErrGatewayNativeIdentity
			}
			def = gatewayCodexCatalogWireEffort(def)
		}
		allowed := []string{}
		efforts := map[string]bool{}
		unsupportedLevels := false
		if value := fields["supported_reasoning_levels"]; value != nil && string(value) != "null" {
			var levels []json.RawMessage
			if json.Unmarshal(value, &levels) != nil {
				return nil, ErrGatewayNativeIdentity
			}
			for _, level := range levels {
				var effort string
				if json.Unmarshal(level, &effort) != nil {
					f, ok := GatewayNativePayloadFields(level, "effort")
					if !ok || json.Unmarshal(f["effort"], &effort) != nil {
						return nil, ErrGatewayNativeIdentity
					}
				}
				effort = gatewayCodexCatalogWireEffort(effort)
				if !gatewaymodelcatalog.WireEffortValid(effort) {
					// Explicit none represents disabled reasoning. Unknown
					// advertised levels must never manufacture that meaning.
					if effort != "none" {
						unsupportedLevels = true
					}
					continue
				}
				if !efforts[effort] {
					allowed = append(allowed, effort)
					efforts[effort] = true
				}
			}
		}
		if len(allowed) > 0 {
			if !efforts[def] {
				return nil, ErrGatewayNativeIdentity
			}
			m.Reasoning = &GatewayAccountModelChoices{Allowed: allowed, Default: def}
		} else if unsupportedLevels || def != "" && def != "none" {
			return nil, ErrGatewayNativeIdentity
		}
		out = append(out, m)
	}
	return out, nil
}
func gatewayAccountModelsAllow(result GatewayAccountModelsResult, model string, reasoning json.RawMessage) bool {
	// The sole rollout fallback preserves the already qualified fixed model.
	if result.Status == "unavailable" {
		return model == GatewayCodexOAuthModel && gatewayLegacyCodexReasoning(reasoning)
	}
	if result.Status != "snapshot" || result.Models == nil {
		return false
	}
	for _, m := range *result.Models {
		if m.ModelID == model {
			if len(reasoning) == 0 || string(reasoning) == "null" {
				return true
			}
			fields, ok := GatewayNativePayloadFields(reasoning, "effort")
			if !ok {
				return false
			}
			// Pinned CLI sends {} when reasoning is disabled. No requested
			// field, including summary/context or unknown fields, is allowed.
			if m.Reasoning == nil {
				return len(fields) == 0
			}
			effortRaw := fields["effort"]
			if effortRaw == nil {
				return true
			}
			var effort string
			if json.Unmarshal(effortRaw, &effort) != nil || !gatewaymodelcatalog.WireEffortValid(effort) {
				return false
			}
			for _, allowed := range m.Reasoning.Allowed {
				if effort == allowed {
					return true
				}
			}
			return false
		}
	}
	return false
}
func gatewayLegacyCodexReasoning(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	fields, ok := GatewayNativePayloadFields(raw, "effort")
	if !ok {
		return false
	}
	if fields["effort"] == nil {
		return true
	}
	var effort string
	return json.Unmarshal(fields["effort"], &effort) == nil && gatewaymodelcatalog.WireEffortValid(effort)
}
