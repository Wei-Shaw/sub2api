package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const GatewayGenerationExtraKey = "gateway_generation_v1"
const GatewayProfileExtraKey = "gateway_profile_v1"
const GatewayModelExtraKey = "gateway_model_v1"
const GatewayMiMoResponsesProfile = "openai-responses-apikey-v1"
const GatewayLegacyBridgeProfile = "mimo-token-plan-responses-chat-bridge-v1"
const GatewayOpenRouterResponsesProfile = "openrouter-openai-responses-apikey-v1"
const GatewayOpenRouterBaseURL = "https://openrouter.ai/api/v1"
const GatewayOpenRouterModel = "openai/gpt-5.4-mini"
const GatewayOpenRouterNanoModel = "openai/gpt-5.4-nano"

// NVIDIA Build is a separate API-key principal and a fixed upstream, not
// an alias for other direct plans or an OpenRouter credential.
const GatewayNVIDIABuildChatProfile = "nvidia-build-chat-apikey-v1"
const GatewayNVIDIABuildBaseURL = "https://integrate.api.nvidia.com/v1"
const GatewayNVIDIABuildAnchorModel = "nvidia/nemotron-3.5-super-vl-120b-a12b"

func GatewayNVIDIABuildModelValid(model string) bool {
	return model == GatewayNVIDIABuildAnchorModel
}

func GatewayOpenRouterModelValid(model string) bool {
	// This validates descriptor syntax only. Candidate qualification and every
	// requested model are separately admitted by the fixed provider catalogue.
	return GatewayAccountModelIDValid(model) && strings.Contains(model, "/") && !strings.Contains(model, "://")
}

// Finite identities only; endpoints/models remain trusted composition fields.
func GatewayNativeAPIKeyProfileValid(profile, baseURL, model string) bool {
	return (profile == GatewayMiMoResponsesProfile || profile == GatewayLegacyBridgeProfile) && baseURL != "" && strings.TrimSpace(model) != "" ||
		profile == GatewayOpenRouterResponsesProfile && baseURL == GatewayOpenRouterBaseURL && GatewayOpenRouterModelValid(model) ||
		profile == GatewayNVIDIABuildChatProfile && baseURL == GatewayNVIDIABuildBaseURL && model == GatewayNVIDIABuildAnchorModel ||
		GatewayNativeCodingPlanProfileValid(profile, baseURL, model)
}

var ErrGatewayNativeIdentity = errors.New("gateway native identity rejected")
var ErrGatewayNativeReplay = errors.New("gateway native dispatch already entered")
var ErrGatewayNativeEffectUnknown = errors.New("gateway native effect unknown")

// This is PRIVATE server routing. It carries no authorization. The host must
// persist its admission/effect claim before calling ForwardGatewayRoute.
type GatewayNativeRoute struct {
	AccountID  int64     `json:"account_id"`
	Generation string    `json:"generation"`
	CreatedAt  time.Time `json:"created_at"`
	Profile    string    `json:"profile"`
	BaseURL    string    `json:"base_url"`
	Model      string    `json:"model"`
}

// Wire identity compares the birth instant, not time.Time's location/cache
// representation. Every other frozen descriptor field remains exact.
func SameGatewayNativeDescriptor(a, b GatewayNativeRoute) bool {
	return a.AccountID == b.AccountID && a.Generation == b.Generation &&
		a.CreatedAt.Equal(b.CreatedAt) && a.Profile == b.Profile &&
		a.BaseURL == b.BaseURL && a.Model == b.Model
}

// This API selects a backend key domain inaccessible to unrestricted ordinary
// item IDs. Do not encode a generation into the old Get/SetReasoningContent ID.
type GatewayNativeReasoningCache interface {
	GetGatewayNativeReasoningContent(context.Context, string, string) (string, error)
	SetGatewayNativeReasoningContent(context.Context, string, string, string, time.Duration) error
}

// Implemented by the real PostgreSQL repository. A mere GetByID cannot close
// the fresh-check/delete/reuse race. The release function MUST always be called.
type GatewayNativeAccountLocker interface {
	LockGatewayNativeAccount(context.Context, int64) (*Account, func(), error)
}

type gatewayNativeContextKey struct{}
type gatewayNativeControlContextKey struct{}

// Server composition only; public admin handlers never receive this context.
func WithGatewayNativeControl(ctx context.Context) context.Context {
	return context.WithValue(ctx, gatewayNativeControlContextKey{}, true)
}
func isGatewayNativeControl(ctx context.Context) bool {
	allowed, _ := ctx.Value(gatewayNativeControlContextKey{}).(bool)
	return allowed
}

type GatewayNativeAccountEraser interface {
	EraseGatewayNativeAccount(context.Context, GatewayNativeRoute) error
}

func (s *OpenAIGatewayService) EraseGatewayCandidate(ctx context.Context, route GatewayNativeRoute) error {
	eraser, ok := s.accountRepo.(GatewayNativeAccountEraser)
	id, err := uuid.Parse(route.Generation)
	if !ok || err != nil || id == uuid.Nil || id.String() != route.Generation ||
		route.AccountID <= 0 || route.CreatedAt.IsZero() || route.BaseURL == "" || route.Model == "" ||
		!GatewayNativeAPIKeyProfileValid(route.Profile, route.BaseURL, route.Model) {
		return ErrGatewayNativeIdentity
	}
	return eraser.EraseGatewayNativeAccount(ctx, route)
}

type gatewayNativeDispatch struct {
	route          GatewayNativeRoute
	account        *Account
	scope          GatewayNativeCredentialScope
	custody        *GatewayNativeCredentialCustody
	oauth          *GatewayNativeOAuthPhysical
	entered        atomic.Bool
	requestedModel string
}

func HasGatewayNativeIdentity(a *Account) bool {
	if a == nil {
		return false
	}
	_, generation := a.Extra[GatewayGenerationExtraKey]
	_, profile := a.Extra[GatewayProfileExtraKey]
	_, custody := a.Extra[GatewayCredentialScopeExtraKey]
	_, claudeGeneration := a.Extra["gateway_claude_generation_v1"]
	_, claudeProfile := a.Extra["gateway_claude_profile_v1"]
	_, claudeCustody := a.Extra["gateway_claude_scope_v1"]
	_, grokGeneration := a.Extra["gateway_grok_generation_v1"]
	_, grokProfile := a.Extra["gateway_grok_profile_v1"]
	_, grokCustody := a.Extra["gateway_grok_scope_v1"]
	return generation || profile || custody || claudeGeneration || claudeProfile || claudeCustody || grokGeneration || grokProfile || grokCustody
}

func validateGatewayNativeShape(a *Account) error {
	if a == nil || a.Platform != PlatformOpenAI || a.Type != AccountTypeAPIKey ||
		a.ParentAccountID != nil || a.ProxyID != nil || a.Schedulable ||
		len(a.GroupIDs) != 0 || len(a.AccountGroups) != 0 {
		return ErrGatewayNativeIdentity
	}
	generation, ok := a.Extra[GatewayGenerationExtraKey].(string)
	id, err := uuid.Parse(generation)
	if !ok || err != nil || id == uuid.Nil || id.String() != generation {
		return ErrGatewayNativeIdentity
	}
	profile, profileOK := a.Extra[GatewayProfileExtraKey].(string)
	mode, passthrough := "force_responses", true
	if profile == GatewayLegacyBridgeProfile || profile == GatewayNVIDIABuildChatProfile {
		mode, passthrough = "force_chat_completions", false
	}
	if !profileOK || (profile != GatewayMiMoResponsesProfile && profile != GatewayLegacyBridgeProfile && profile != GatewayOpenRouterResponsesProfile && profile != GatewayNVIDIABuildChatProfile && !GatewayNativeCodingPlanProfileIDValid(profile)) ||
		a.Extra["openai_responses_mode"] != mode ||
		a.Extra["openai_passthrough"] != passthrough ||
		a.Extra["native_api_key_cancel_on_disconnect"] != true ||
		a.Extra["openai_preserve_compatible_reasoning"] != true {
		return ErrGatewayNativeIdentity
	}
	model, modelOK := a.Extra[GatewayModelExtraKey].(string)
	if !modelOK || strings.TrimSpace(model) == "" {
		return ErrGatewayNativeIdentity
	}
	envelope, envelopeOK := a.Credentials["api_key"].(string)
	baseURL, baseURLOK := a.Credentials["base_url"].(string)
	if len(a.Credentials) != 2 || !envelopeOK || !baseURLOK || envelope == "" ||
		envelope != strings.TrimSpace(envelope) || baseURL == "" {
		return ErrGatewayNativeIdentity
	}
	if !GatewayNativeAPIKeyProfileValid(profile, baseURL, model) {
		return ErrGatewayNativeIdentity
	}
	if _, _, _, err := gatewayNativeParseEnvelope(envelope); err != nil {
		return ErrGatewayNativeIdentity
	}
	if _, err := GatewayNativeCredentialScopeForAccount(a); err != nil {
		return ErrGatewayNativeIdentity
	}
	// Extra is an allowlist, so no WS, pool, header override, probe, quota reset,
	// provider endpoint/protocol or model remapping knob can alter this profile.
	for key := range a.Extra {
		switch key {
		case GatewayGenerationExtraKey, GatewayProfileExtraKey, GatewayModelExtraKey, GatewayCredentialScopeExtraKey,
			"openai_responses_mode", "openai_passthrough", "native_api_key_cancel_on_disconnect", "openai_preserve_compatible_reasoning":
		default:
			return ErrGatewayNativeIdentity
		}
	}
	return nil
}

func validateGatewayNativeRoute(r GatewayNativeRoute, a *Account) error {
	if validateGatewayNativeShape(a) != nil || r.AccountID <= 0 || r.CreatedAt.IsZero() ||
		r.AccountID != a.ID || r.Generation != a.Extra[GatewayGenerationExtraKey] ||
		r.Profile != a.Extra[GatewayProfileExtraKey] || !r.CreatedAt.Equal(a.CreatedAt) ||
		r.BaseURL == "" || r.BaseURL != a.GetCredential("base_url") ||
		r.Model == "" || r.Model != a.Extra[GatewayModelExtraKey] || !a.IsActive() ||
		(a.ExpiresAt != nil && !a.ExpiresAt.After(time.Now())) {
		return ErrGatewayNativeIdentity
	}
	return nil
}

func GatewayNativeDescriptor(a *Account) (GatewayNativeRoute, error) {
	if validateGatewayNativeShape(a) != nil || a.ID <= 0 || a.CreatedAt.IsZero() {
		return GatewayNativeRoute{}, ErrGatewayNativeIdentity
	}
	generation, generationOK := a.Extra[GatewayGenerationExtraKey].(string)
	profile, profileOK := a.Extra[GatewayProfileExtraKey].(string)
	model, modelOK := a.Extra[GatewayModelExtraKey].(string)
	if !generationOK || !profileOK || !modelOK {
		return GatewayNativeRoute{}, ErrGatewayNativeIdentity
	}
	return GatewayNativeRoute{a.ID, generation, a.CreatedAt, profile, a.GetCredential("base_url"), model}, nil
}

// This is an exact candidate query, never name/latest/ID-only recovery. Zero,
// multiple, malformed or deleted candidates are quarantined by the facade.
func (s *OpenAIGatewayService) ResolveGatewayCandidate(ctx context.Context, generation string) (*Account, error) {
	a, err := s.ResolveGatewayCandidateMetadata(ctx, generation)
	if err != nil || gatewayNativeCustody(ctx).ValidateEnvelope(a.GetCredential("api_key")) != nil {
		return nil, ErrGatewayNativeIdentity
	}
	return a, nil
}

// Safe control metadata remains available after key retirement so the owner can
// disable and erase an exact candidate. This does not qualify activation or entry.
func (s *OpenAIGatewayService) ResolveGatewayCandidateMetadata(ctx context.Context, generation string) (*Account, error) {
	consumer, consumerErr := GatewayNativeConsumer(ctx)
	custody := gatewayNativeCustody(ctx)
	id, err := uuid.Parse(generation)
	if consumerErr != nil || custody == nil || err != nil || id == uuid.Nil || id.String() != generation || s.accountRepo == nil {
		return nil, ErrGatewayNativeIdentity
	}
	accounts, err := s.accountRepo.FindByExtraField(ctx, GatewayGenerationExtraKey, generation)
	if err != nil || len(accounts) != 1 {
		return nil, ErrGatewayNativeIdentity
	}
	a := &accounts[0]
	if _, err := GatewayNativeDescriptor(a); err != nil {
		return nil, err
	}
	scope, err := GatewayNativeCredentialScopeForAccount(a)
	_, _, _, envelopeErr := gatewayNativeParseEnvelope(a.GetCredential("api_key"))
	if err != nil || scope.Consumer != consumer || envelopeErr != nil {
		return nil, ErrGatewayNativeIdentity
	}
	return snapshotGatewayNativeAccount(a), nil
}

// Snapshot accepted primitives before the next repository/transport wait. No
// decrypted Account clone exists: both original and fresh comparisons remain
// encrypted throughout Authorization construction and provider entry.
func snapshotGatewayNativeAccount(a *Account) *Account {
	snapshot := *a
	snapshot.Credentials = make(map[string]any, len(a.Credentials))
	for key, value := range a.Credentials {
		snapshot.Credentials[key] = value
	}
	snapshot.Extra = make(map[string]any, len(a.Extra))
	for key, value := range a.Extra {
		snapshot.Extra[key] = value
	}
	scope, _ := GatewayNativeCredentialScopeForAccount(a)
	if a.Type == AccountTypeOAuth {
		scope, _ = gatewayOAuthScopeForAccount(a)
	}
	snapshot.Extra[GatewayCredentialScopeExtraKey] = scope.Metadata()
	if a.ExpiresAt != nil {
		expires := *a.ExpiresAt
		snapshot.ExpiresAt = &expires
	}
	return &snapshot
}

func (s *OpenAIGatewayService) ForwardGatewayRoute(ctx context.Context, c *gin.Context, route GatewayNativeRoute, body []byte) (*OpenAIForwardResult, bool, error) {
	if ctx != nil {
		if lifetime := gatewayNativeLifetime(ctx); lifetime != nil {
			defer lifetime.finish()
		}
	}
	if ctx != nil && route.Profile == GatewayGrokOAuthResponsesProfile {
		return s.forwardGatewayGrokRoute(ctx, c, route, body)
	}
	if ctx != nil && route.Profile == GatewayClaudeOAuthMessagesProfile {
		return s.forwardGatewayClaudeRoute(ctx, c, route, body)
	}
	if ctx != nil && route.Profile == GatewayCodexOAuthResponsesProfile {
		return s.forwardGatewayOAuthRoute(ctx, c, route, body)
	}
	if ctx == nil || c == nil || c.Request == nil || s.accountRepo == nil || len(body) > 4<<20 || !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() || !gatewayNativeUnambiguousPolicyFields(body) ||
		(route.Profile != GatewayOpenRouterResponsesProfile && route.Profile != GatewayNVIDIABuildChatProfile && !GatewayCodingPlanCatalogProfile(route.Profile) && gjson.GetBytes(body, "model").String() != route.Model) ||
		(GatewayCodingPlanCatalogProfile(route.Profile) && !GatewayCodingPlanModelAdmitted(route.Profile, gjson.GetBytes(body, "model").String())) || !GatewayNativeAPIKeyProfileValid(route.Profile, route.BaseURL, route.Model) ||
		gjson.GetBytes(body, "previous_response_id").String() != "" ||
		(gjson.GetBytes(body, "service_tier").Exists() && gjson.GetBytes(body, "service_tier").String() != "default") ||
		(gjson.GetBytes(body, "store").Exists() && gjson.GetBytes(body, "store").Bool()) {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionPayload)
		return nil, false, ErrGatewayNativeIdentity
	}
	if route.Profile == GatewayOpenRouterResponsesProfile {
		fields, valid := gatewayOpenRouterResponsesFields(body)
		if !valid {
			gatewayNativeAdmissionDiagnostic(gatewayAdmissionPayload)
			return nil, false, ErrGatewayNativeIdentity
		}
		if !gatewayNativeProviderTextInput(route.Profile, fields["input"]) {
			gatewayNativeAdmissionDiagnostic(gatewayAdmissionInput)
			return nil, false, ErrGatewayNativeIdentity
		}
		if fields["tools"] != nil && !gatewayNativeProviderTools(fields["tools"]) {
			gatewayNativeAdmissionDiagnostic(gatewayAdmissionTools)
			return nil, false, ErrGatewayNativeIdentity
		}
		prepared, mapping, prepareErr := adaptResponsesClientToolsForFunctionUpstream(body, "OpenRouter")
		if prepareErr != nil || len(prepared) > 4<<20 {
			gatewayNativeAdmissionDiagnostic(gatewayAdmissionMapping)
			return nil, false, ErrGatewayNativeIdentity
		}
		body = prepared
		setGrokResponsesClientToolMapping(c, mapping)
		defer clearGrokResponsesClientToolMapping(c)
	}
	if route.Profile == GatewayNVIDIABuildChatProfile {
		// Validate the bounded provider subset before custody, but do NOT
		// pre-lower custom/namespace tools. The existing protected Chat bridge
		// owns the single Responses-to-Chat conversion and reverse restoration.
		// Double lowering loses the caller's original tool identity.
		if ValidateGatewayNativeProviderResponsesBounded(route.Profile, body, 4<<20) != nil {
			gatewayNativeAdmissionDiagnostic(gatewayAdmissionPayload)
			return nil, false, ErrGatewayNativeIdentity
		}
	}
	if GatewayNativeCodingPlanProfileIDValid(route.Profile) {
		prepared, mapping, prepareErr := gatewayNativePrepareProviderResponses(route.Profile, body, 4<<20)
		if prepareErr != nil {
			gatewayNativeAdmissionDiagnostic(gatewayAdmissionMapping)
			return nil, false, ErrGatewayNativeIdentity
		}
		body = prepared
		setGrokResponsesClientToolMapping(c, mapping)
		defer clearGrokResponsesClientToolMapping(c)
	}
	if _, ok := s.accountRepo.(GatewayNativeAccountLocker); !ok {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionRuntime)
		return nil, false, ErrGatewayNativeIdentity
	}
	if s.GatewayNativeLifetimeReady() && gatewayNativeLifetime(ctx) == nil {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionLifetime)
		return nil, false, ErrGatewayNativeIdentity
	}
	consumer, err := GatewayNativeConsumer(ctx)
	custody := gatewayNativeCustody(ctx)
	if err != nil || custody == nil {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionCredential)
		return nil, false, ErrGatewayNativeIdentity
	}
	a, err := s.accountRepo.GetByID(ctx, route.AccountID)
	if err != nil || validateGatewayNativeRoute(route, a) != nil {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionDescriptor)
		return nil, false, ErrGatewayNativeIdentity
	}
	scope, scopeErr := GatewayNativeCredentialScopeForAccount(a)
	if scopeErr != nil || scope.Consumer != consumer || custody.ValidateEnvelope(a.GetCredential("api_key")) != nil {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionCredential)
		return nil, false, ErrGatewayNativeIdentity
	}
	if lifetime := gatewayNativeLifetime(ctx); lifetime != nil && !lifetime.matchesScope(scope) {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionLifetime)
		return nil, false, ErrGatewayNativeIdentity
	}
	a = snapshotGatewayNativeAccount(a)
	state := &gatewayNativeDispatch{route: route, account: a, scope: scope, custody: custody}
	if route.Profile == GatewayNVIDIABuildChatProfile || GatewayCodingPlanCatalogProfile(route.Profile) {
		state.requestedModel = gjson.GetBytes(body, "model").String()
	}
	if route.Profile == GatewayOpenRouterResponsesProfile {
		state.requestedModel = gjson.GetBytes(body, "model").String()
		if s.gatewayOpenRouterModels == nil {
			gatewayNativeAdmissionDiagnostic(gatewayAdmissionRuntime)
			return nil, false, ErrGatewayNativeIdentity
		}
		catalogue := s.gatewayOpenRouterModels.read(ctx, gatewayOpenRouterModelsPrincipal(scope, a.GetCredential("api_key")), false)
		if !gatewayOpenRouterModelsAllow(catalogue, route.Model, state.requestedModel, body) {
			gatewayNativeCatalogAdmissionDiagnostic(catalogue)
			return nil, false, ErrGatewayNativeIdentity
		}
	}
	ctx = WithHTTPUpstreamRedirectsDisabled(context.WithValue(ctx, gatewayNativeContextKey{}, state))
	// Restrict this private seam to a Responses request; never pass public route
	// path suffixes or authentication and affinity headers into upstream routing.
	request := c.Request.Clone(ctx)
	request.Header = make(http.Header)
	c.Request = request
	// Native Responses is the first qualification path. The separately selected
	// historical bridge is never an automatic fallback after any dispatch.
	var result *OpenAIForwardResult
	if route.Profile == GatewayMiMoResponsesProfile || route.Profile == GatewayOpenRouterResponsesProfile || GatewayNativeCodingPlanProfileIDValid(route.Profile) {
		result, err = s.forwardGatewayNativeResponses(ctx, c, a, body)
	} else {
		result, err = s.forwardResponsesViaRawChatCompletions(ctx, c, a, body)
	}
	if err != nil && err != ErrGatewayNativeIdentity && err != ErrGatewayNativeReplay {
		if state.entered.Load() {
			err = ErrGatewayNativeEffectUnknown
		} else {
			err = ErrGatewayNativeIdentity
		}
	}
	if err == nil {
		if lifetime := gatewayNativeLifetime(ctx); lifetime != nil {
			lifetime.success()
		}
	}
	return result, state.entered.Load(), err
}

// Last common HTTP seam. The pointer is the EXACT credential-bearing Account
// used to construct Authorization and URL. The fresh row is locked until Do
// returns headers/error; status changes after that boundary are accepted effects.
func (s *OpenAIGatewayService) checkGatewayNativeDispatch(request *http.Request, a *Account) (func(), error) {
	noop := func() {}
	state, _ := request.Context().Value(gatewayNativeContextKey{}).(*gatewayNativeDispatch)
	if state == nil {
		if HasGatewayNativeIdentity(a) {
			return noop, ErrGatewayNativeIdentity
		}
		return noop, nil
	}
	if state.route.Profile == GatewayGrokOAuthResponsesProfile {
		return s.checkGatewayGrokDispatch(request, a, state)
	}
	if state.route.Profile == GatewayCodexOAuthResponsesProfile {
		return s.checkGatewayOAuthDispatch(request, a, state)
	}
	if state.account != a || validateGatewayNativeRoute(state.route, a) != nil || request.Method != http.MethodPost ||
		request.Header.Get("Authorization") != "Bearer "+a.GetCredential("api_key") {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionDescriptor)
		return noop, ErrGatewayNativeIdentity
	}
	target, err := s.gatewayNativeTargetURL(a)
	if err != nil || request.URL.String() != target {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionDispatchTarget)
		return noop, ErrGatewayNativeIdentity
	}
	locker, ok := s.accountRepo.(GatewayNativeAccountLocker)
	if !ok {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionAccountLock)
		return noop, ErrGatewayNativeIdentity
	}
	// Observe membership before taking the credential row lock. The same
	// encrypted row and unexpired snapshot are checked again at entry below.
	var catalogue GatewayAccountModelsResult
	var payload []byte
	if state.route.Profile == GatewayOpenRouterResponsesProfile {
		if s.gatewayOpenRouterModels == nil || request.GetBody == nil {
			gatewayNativeAdmissionDiagnostic(gatewayAdmissionRuntime)
			return noop, ErrGatewayNativeIdentity
		}
		reader, readErr := request.GetBody()
		if readErr != nil {
			gatewayNativeAdmissionDiagnostic(gatewayAdmissionPayload)
			return noop, ErrGatewayNativeIdentity
		}
		payload, readErr = io.ReadAll(io.LimitReader(reader, (4<<20)+1))
		_ = reader.Close()
		if readErr != nil || len(payload) > 4<<20 || gjson.GetBytes(payload, "model").String() != state.requestedModel {
			gatewayNativeAdmissionDiagnostic(gatewayAdmissionPayload)
			return noop, ErrGatewayNativeIdentity
		}
		catalogue = s.gatewayOpenRouterModels.read(request.Context(), gatewayOpenRouterModelsPrincipal(state.scope, a.GetCredential("api_key")), false)
	}
	fresh, release, err := locker.LockGatewayNativeAccount(request.Context(), state.route.AccountID)
	if err != nil {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionAccountLock)
		return noop, ErrGatewayNativeIdentity
	}
	if release == nil {
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionAccountLock)
		return noop, ErrGatewayNativeIdentity
	}
	if validateGatewayNativeRoute(state.route, fresh) != nil || !reflect.DeepEqual(a.Credentials, fresh.Credentials) || !reflect.DeepEqual(a.Extra, fresh.Extra) {
		release()
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionAccountChanged)
		return noop, ErrGatewayNativeIdentity
	}
	scope, err := GatewayNativeCredentialScopeForAccount(fresh)
	consumer, consumerErr := GatewayNativeConsumer(request.Context())
	if err != nil || consumerErr != nil || scope != state.scope || scope.Consumer != consumer || state.custody == nil {
		release()
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionCredential)
		return noop, ErrGatewayNativeIdentity
	}
	if state.route.Profile == GatewayNVIDIABuildChatProfile || GatewayCodingPlanCatalogProfile(state.route.Profile) {
		// The bound provider/model tuple and the actual outgoing body must still
		// agree at the final locked credential entry boundary.
		modelAdmitted := GatewayNVIDIABuildModelValid(state.requestedModel)
		if state.route.Profile != GatewayNVIDIABuildChatProfile {
			modelAdmitted = GatewayCodingPlanModelAdmitted(state.route.Profile, state.requestedModel)
		}
		if request.GetBody == nil || !modelAdmitted {
			release()
			return noop, ErrGatewayNativeIdentity
		}
		reader, readErr := request.GetBody()
		if readErr != nil {
			release()
			return noop, ErrGatewayNativeIdentity
		}
		payload, readErr := io.ReadAll(io.LimitReader(reader, (4<<20)+1))
		_ = reader.Close()
		if readErr != nil || len(payload) > 4<<20 ||
			gjson.GetBytes(payload, "model").String() != state.requestedModel {
			release()
			return noop, ErrGatewayNativeIdentity
		}
	}
	if state.route.Profile == GatewayOpenRouterResponsesProfile && !gatewayOpenRouterModelsAllow(catalogue, state.route.Model, state.requestedModel, payload) {
		release()
		gatewayNativeCatalogAdmissionDiagnostic(catalogue)
		return noop, ErrGatewayNativeIdentity
	}
	// The ONLY decode boundary. The row stays encrypted, and no plaintext enters
	// Account credentials, cache/history, readback or errors. Authentication fails
	// before the entered CAS, so malformed/tampered/unavailable-key rows make zero
	// upstream entries. Caller Authorization is replaced, never forwarded.
	authorization, err := state.custody.authorization(fresh.GetCredential("api_key"), scope)
	if err != nil {
		release()
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionCredential)
		return noop, ErrGatewayNativeIdentity
	}
	if !gatewayNativeEntered(request.Context(), &state.entered) {
		release()
		gatewayNativeAdmissionDiagnostic(gatewayAdmissionReplay)
		return noop, ErrGatewayNativeReplay
	}
	request.Header.Set("Authorization", authorization)
	request.GetBody = nil
	request.Header.Del("Idempotency-Key")
	request.Header.Del("X-Idempotency-Key")
	return release, nil
}

func gatewayNativeReasoningScope(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	state, _ := c.Request.Context().Value(gatewayNativeContextKey{}).(*gatewayNativeDispatch)
	if state == nil {
		return ""
	}
	return state.route.Generation
}

// Different JSON readers choose different occurrences of duplicate keys. Scan
// decoded top-level names (including escapes), rejecting duplicate policy fields
// before any transport entry. Nested and noncritical fields retain native bytes.
func gatewayNativeUnambiguousPolicyFields(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	seen := make(map[string]bool, 4)
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return false
		}
		key, ok := token.(string)
		if !ok {
			return false
		}
		for _, canonical := range []string{"model", "store", "previous_response_id", "service_tier"} {
			// encoding/json matches struct fields with EqualFold, including
			// Unicode aliases. Only canonical decoded names are safe here.
			if strings.EqualFold(key, canonical) {
				if key != canonical || seen[canonical] {
					return false
				}
				seen[canonical] = true
			}
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return false
		}
	}
	token, err = decoder.Token()
	return err == nil && token == json.Delim('}')
}

// GatewayNativePayloadFields reuses the repaired private protocol parser.
// Tool and user values remain opaque; the caller names only critical fields.
func GatewayNativePayloadFields(body []byte, critical ...string) (map[string]json.RawMessage, bool) {
	return gatewayNativeCanonicalObject(body, critical...)
}

// Required private bootstrap custody input must be a constructed active adapter.
func GatewayNativeCredentialCustodyReady(c *GatewayNativeCredentialCustody) bool {
	return c != nil && c.active != "" && c.keys[c.active] != nil
}
