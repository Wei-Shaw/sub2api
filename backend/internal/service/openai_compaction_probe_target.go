package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	OpenAICompactionProbeTargetsExtraKey          = "openai_compact_probe_targets"
	OpenAICompactionCredentialFingerprintExtraKey = "__openai_compact_credential_fingerprint"
	openAICompactionProbeRecordVersion            = 1
	openAICompactionProbeMaxTargets               = 32
)

const (
	OpenAICompactionProbeVerdictSupported   = "supported"
	OpenAICompactionProbeVerdictUnsupported = "unsupported"
	OpenAICompactionProbeVerdictUnknown     = "unknown"
	OpenAICompactionKindV2                  = openAICompactionKindV2
	OpenAICompactionKindLegacy              = openAICompactionKindLegacy
)

const (
	openAICompactionErrorNone                = ""
	openAICompactionErrorAuth                = "auth_invalid"
	openAICompactionErrorRateLimit           = "rate_limited"
	openAICompactionErrorUpstream5xx         = "upstream_5xx"
	openAICompactionErrorTimeout             = "timeout"
	openAICompactionErrorTransport           = "transport"
	openAICompactionErrorTruncated           = "truncated"
	openAICompactionErrorModelNotFound       = "model_not_found"
	openAICompactionErrorEndpointUnsupported = "endpoint_unsupported"
	openAICompactionErrorInvalidContract     = "invalid_contract"
	openAICompactionErrorRequest             = "request_error"
)

var (
	ErrOpenAICompactionProbeIdentityChanged = errors.New("openai compaction probe target identity changed")
	ErrOpenAICompactionProtocolUnknown      = errors.New("openai compaction protocol unknown")
)

type OpenAICompactionProbeTargetWriter interface {
	UpdateOpenAICompactionProbeTarget(context.Context, *Account, OpenAICompactionProbeTarget, OpenAICompactionProbeObservation) error
}

// OpenAICompactionProbeTarget is the bounded identity of one actual upstream
// target. It contains no raw credential material.
type OpenAICompactionProbeTarget struct {
	Kind                  string `json:"kind"`
	RequestedModel        string `json:"requested_model,omitempty"`
	Model                 string `json:"model"`
	Protocol              string `json:"protocol"`
	Endpoint              string `json:"endpoint"`
	EndpointHash          string `json:"endpoint_hash"`
	LogicalAccountID      int64  `json:"logical_account_id"`
	CredentialAccountID   int64  `json:"credential_account_id"`
	CredentialSource      string `json:"credential_source"`
	CredentialFingerprint string `json:"credential_fingerprint"`
	AuthMode              string `json:"auth_mode"`
	ConfigFingerprint     string `json:"config_fingerprint"`
	FingerprintVersion    int    `json:"fingerprint_version"`
	outboundEndpoint      string
}

func (t OpenAICompactionProbeTarget) ID() string {
	return strings.Join([]string{t.Kind, t.Model, t.Protocol, t.EndpointHash}, "|")
}

type OpenAICompactionProbeRecord struct {
	Version       int                         `json:"version"`
	Target        OpenAICompactionProbeTarget `json:"target"`
	Verdict       string                      `json:"verdict"`
	LastCheckedAt string                      `json:"last_checked_at"`
	VerifiedAt    string                      `json:"verified_at,omitempty"`
	LastStatus    int                         `json:"last_status,omitempty"`
	ErrorClass    string                      `json:"error_class,omitempty"`
	Error         string                      `json:"error,omitempty"`
}

type OpenAICompactionProbeObservation struct {
	Verdict    string
	CheckedAt  time.Time
	Status     int
	ErrorClass string
	Error      string
}

const (
	openAICompactionTierUnsupported = 0
	openAICompactionTierUnknown     = 1
	openAICompactionTierDeclared    = 2
	openAICompactionTierVerified    = 3
)

// OpenAICompactionRequestDecision is the shared, request-target-scoped result
// consumed by scheduling and forwarding. Unknown native support remains an
// attempt, while verified evidence outranks provider declarations.
type OpenAICompactionRequestDecision struct {
	Kind               string
	Execution          string
	Tier               int
	Mode               string
	Target             OpenAICompactionProbeTarget
	Verdict            string
	VerdictKnown       bool
	ProviderCapability ProviderCapabilityStatus
	Reason             string
}

func (d OpenAICompactionRequestDecision) Allowed() bool {
	return d.Execution == openAICompactionExecutionNative || d.Execution == openAICompactionExecutionSummary
}

func openAICompactionAuthMode(account, credentialAccount *Account) string {
	if account == nil {
		return ""
	}
	if account.IsShadow() {
		return "credential_shadow"
	}
	if credentialAccount != nil && strings.TrimSpace(credentialAccount.Type) != "" {
		return strings.TrimSpace(credentialAccount.Type)
	}
	return strings.TrimSpace(account.Type)
}

func openAICompactionCredentialSource(account, credentialAccount *Account) (string, int64) {
	if account == nil || credentialAccount == nil {
		return "", 0
	}
	if account.IsShadow() {
		return "credential_shadow", credentialAccount.ID
	}
	return "direct", credentialAccount.ID
}

func openAICompactionTargetExtraMaterial(extra map[string]any) map[string]any {
	out := make(map[string]any)
	for _, key := range []string{
		"api_protocol", "api_base_urls", "protocol_rules", "openai_responses_mode",
		"openai_responses_supported", "openai_capabilities", "base_url", "openai_base_url",
		"openai_passthrough", "openai_oauth_passthrough",
	} {
		if value, ok := extra[key]; ok && value != nil {
			out[key] = value
		}
	}
	return out
}

func OpenAICompactionProbeRouteMaterialEqual(left, right map[string]any) bool {
	leftJSON, _ := json.Marshal(openAICompactionTargetExtraMaterial(left))
	rightJSON, _ := json.Marshal(openAICompactionTargetExtraMaterial(right))
	return string(leftJSON) == string(rightJSON)
}

func openAICompactionRouteMaterial(account *Account) map[string]any {
	if account == nil {
		return nil
	}
	out := openAICompactionTargetExtraMaterial(account.Extra)
	if account.ProxyID != nil {
		out["proxy_id"] = *account.ProxyID
	}
	if account.Credentials != nil {
		for _, key := range []string{
			"api_protocol", "api_base_urls", "protocol_rules", "model_mapping", "compact_model_mapping",
			"base_url", "openai_base_url", "account_mode", "project_id",
		} {
			if value, ok := account.Credentials[key]; ok && value != nil {
				out["credential."+key] = value
			}
		}
	}
	return out
}

func hashOpenAICompactionFingerprintMaterial(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// OpenAICompactionCredentialFingerprint hashes only stable credential identity
// material. OAuth token rotation stays stable; setup-token and API-key changes
// rotate the fingerprint. Raw credential values never leave this function.
func OpenAICompactionCredentialFingerprint(account *Account) string {
	if account == nil || len(account.Credentials) == 0 {
		return ""
	}
	identityKeys := []string{
		"chatgpt_account_id", "chatgpt_user_id", "organization_id", "chatgpt_account_is_fedramp",
		"task_id", "agent_runtime_id",
	}
	identity := make(map[string]any)
	for _, key := range identityKeys {
		if value, ok := account.Credentials[key]; ok && value != nil {
			identity[key] = value
		}
	}
	switch account.Type {
	case AccountTypeOAuth:
		if account.IsOpenAIAgentIdentity() {
			if openAICompactionIdentityString(identity, "task_id") == "" ||
				openAICompactionIdentityString(identity, "agent_runtime_id") == "" {
				return ""
			}
			privateKey := strings.TrimSpace(account.GetCredential("agent_private_key"))
			if privateKey == "" {
				return ""
			}
			sum := sha256.Sum256([]byte(privateKey))
			identity["agent_private_key_sha256"] = hex.EncodeToString(sum[:])
		} else if openAICompactionIdentityString(identity, "chatgpt_account_id") == "" {
			return ""
		}
	case AccountTypeSetupToken:
		token := strings.TrimSpace(account.GetOpenAIAccessToken())
		if token == "" {
			return ""
		}
		sum := sha256.Sum256([]byte(token))
		identity["access_token_sha256"] = hex.EncodeToString(sum[:])
	case AccountTypeAPIKey:
		apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
		if apiKey == "" {
			return ""
		}
		sum := sha256.Sum256([]byte(apiKey))
		identity["api_key_sha256"] = hex.EncodeToString(sum[:])
	default:
		identity["type"] = account.Type
	}
	if overrides := account.GetHeaderOverrides(); len(overrides) > 0 {
		identity["header_overrides"] = overrides
	}
	return hashOpenAICompactionFingerprintMaterial(identity)
}

func openAICompactionIdentityString(material map[string]any, key string) string {
	value, ok := material[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func openAICompactionTargetFingerprintWithDigest(account *Account, target OpenAICompactionProbeTarget, credentialFingerprint string) string {
	if account == nil || strings.TrimSpace(credentialFingerprint) == "" {
		return ""
	}
	credentialSource, credentialAccountID := openAICompactionCredentialSource(account, &Account{ID: target.CredentialAccountID})
	if target.CredentialSource != "" {
		credentialSource = target.CredentialSource
	}
	material := map[string]any{
		"version":                1,
		"logical_account_id":     account.ID,
		"credential_account_id":  credentialAccountID,
		"credential_source":      credentialSource,
		"platform":               account.Platform,
		"type":                   account.Type,
		"parent_account_id":      account.ParentAccountID,
		"route":                  openAICompactionRouteMaterial(account),
		"credential_fingerprint": strings.TrimSpace(credentialFingerprint),
		"kind":                   target.Kind,
		"model":                  target.Model,
		"protocol":               target.Protocol,
		"endpoint_hash":          target.EndpointHash,
		"auth_mode":              target.AuthMode,
	}
	return hashOpenAICompactionFingerprintMaterial(material)
}

func openAICompactionTargetFingerprint(account, credentialAccount *Account, target OpenAICompactionProbeTarget) string {
	return openAICompactionTargetFingerprintWithDigest(account, target, OpenAICompactionCredentialFingerprint(credentialAccount))
}

func OpenAICompactionProbeTargetFingerprintFromProjection(account *Account, target OpenAICompactionProbeTarget) string {
	if account == nil {
		return ""
	}
	digest, _ := account.Extra[OpenAICompactionCredentialFingerprintExtraKey].(string)
	return openAICompactionTargetFingerprintWithDigest(account, target, digest)
}

func OpenAICompactionProbeTargetFingerprint(account, credentialAccount *Account, target OpenAICompactionProbeTarget) string {
	if account == nil || credentialAccount == nil {
		return ""
	}
	return openAICompactionTargetFingerprint(account, credentialAccount, target)
}

func resolveOpenAICompactionTargetProtocolCached(account *Account, model string) string {
	var catalog []string
	if account != nil && account.routesByModel() && account.IsAdaptiveAPIProtocol() {
		profile := account.providerProfile()
		if profile != nil && profile.ModelCatalog {
			rules, _ := account.configuredProtocolRules()
			if _, matched := matchProtocolRuleSet(model, rules); !matched {
				catalog = cachedModelCatalogProtocols(account, model)
			}
		}
	}
	return resolveUpstreamProtocol(account, APIProtocolResponses, model, catalog)
}

func resolveOpenAICompactionTargetProtocol(ctx context.Context, s *OpenAIGatewayService, account *Account, model string, refreshCatalog bool) (string, bool) {
	if account != nil && account.routesByModel() && account.IsAdaptiveAPIProtocol() {
		profile := account.providerProfile()
		if profile != nil && profile.ModelCatalog {
			rules, _ := account.configuredProtocolRules()
			if _, matched := matchProtocolRuleSet(model, rules); !matched {
				var catalog []string
				var known bool
				if refreshCatalog && s != nil {
					catalog, known = s.modelCatalogProtocolsKnown(ctx, account, model)
				} else {
					catalog, known = cachedModelCatalogProtocolsKnown(account, model)
				}
				if !known {
					return "", false
				}
				return resolveUpstreamProtocol(account, APIProtocolResponses, model, catalog), true
			}
		}
	}
	if refreshCatalog && s != nil {
		return s.resolveUpstreamProtocolFor(ctx, account, APIProtocolResponses, model), true
	}
	return resolveOpenAICompactionTargetProtocolCached(account, model), true
}

func (s *OpenAIGatewayService) resolveOpenAICompactionTarget(ctx context.Context, account *Account, requestedModel, kind string, refreshCatalog bool) (OpenAICompactionProbeTarget, error) {
	if account == nil {
		return OpenAICompactionProbeTarget{}, fmt.Errorf("account is unavailable")
	}
	credentialAccount := account
	if account.IsShadow() {
		if s == nil || s.accountRepo == nil {
			return OpenAICompactionProbeTarget{}, fmt.Errorf("credential source facts are unavailable")
		}
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return OpenAICompactionProbeTarget{}, err
		}
		credentialAccount = resolved
	}
	return s.resolveOpenAICompactionTargetWithCredential(ctx, account, credentialAccount, requestedModel, kind, refreshCatalog)
}

func (s *OpenAIGatewayService) resolveOpenAICompactionTargetRoute(ctx context.Context, account *Account, requestedModel, kind string, refreshCatalog bool, protocolOverride string) (OpenAICompactionProbeTarget, bool, error) {
	if account == nil {
		return OpenAICompactionProbeTarget{}, false, fmt.Errorf("account is unavailable")
	}
	routeModel := strings.TrimSpace(resolveOpenAIAccountUpstreamModelForRequest(account, requestedModel, false))
	if routeModel == "" {
		return OpenAICompactionProbeTarget{}, false, fmt.Errorf("compaction target model is unavailable")
	}
	protocol := strings.TrimSpace(protocolOverride)
	known := protocol != ""
	if !known {
		protocol, known = resolveOpenAICompactionTargetProtocol(ctx, s, account, routeModel, refreshCatalog)
	}
	model := routeModel
	if known && kind == openAICompactionKindLegacy && protocol == APIProtocolResponses {
		model = strings.TrimSpace(resolveOpenAIAccountUpstreamModelForRequest(account, requestedModel, true))
		if s != nil {
			if compactModel := strings.TrimSpace(s.resolveOpenAICompactFallbackModel(account, requestedModel)); compactModel != "" {
				model = compactModel
			}
		}
		if model == "" {
			model = routeModel
		}
	}
	target := OpenAICompactionProbeTarget{
		Kind:               kind,
		RequestedModel:     strings.TrimSpace(requestedModel),
		Model:              model,
		Protocol:           protocol,
		LogicalAccountID:   account.ID,
		FingerprintVersion: 1,
	}
	if !known {
		target.Protocol = ""
		return target, false, nil
	}
	endpoint, err := s.openAICompactionTargetEndpoint(account, protocol, kind)
	if err != nil {
		return OpenAICompactionProbeTarget{}, false, err
	}
	target.Endpoint = sanitizeOpenAICompactionEndpoint(endpoint)
	target.EndpointHash = hashOpenAICompactionFingerprintMaterial([]string{endpoint})
	target.outboundEndpoint = endpoint
	return target, true, nil
}

func (s *OpenAIGatewayService) resolveOpenAICompactionTargetWithCredential(ctx context.Context, account, credentialAccount *Account, requestedModel, kind string, refreshCatalog bool) (OpenAICompactionProbeTarget, error) {
	return s.resolveOpenAICompactionTargetWithCredentialProtocol(ctx, account, credentialAccount, requestedModel, kind, refreshCatalog, "")
}

func (s *OpenAIGatewayService) resolveOpenAICompactionTargetWithCredentialProtocol(ctx context.Context, account, credentialAccount *Account, requestedModel, kind string, refreshCatalog bool, protocolOverride string) (OpenAICompactionProbeTarget, error) {
	if account == nil || credentialAccount == nil {
		return OpenAICompactionProbeTarget{}, fmt.Errorf("account or credential source is unavailable")
	}
	target, known, err := s.resolveOpenAICompactionTargetRoute(ctx, account, requestedModel, kind, refreshCatalog, protocolOverride)
	if err != nil {
		return OpenAICompactionProbeTarget{}, err
	}
	if !known {
		return OpenAICompactionProbeTarget{}, fmt.Errorf("compaction target protocol is unknown: %w", ErrOpenAICompactionProtocolUnknown)
	}
	credentialSource, credentialAccountID := openAICompactionCredentialSource(account, credentialAccount)
	target.CredentialAccountID = credentialAccountID
	target.CredentialSource = credentialSource
	target.CredentialFingerprint = OpenAICompactionCredentialFingerprint(credentialAccount)
	target.AuthMode = openAICompactionAuthMode(account, credentialAccount)
	target.ConfigFingerprint = openAICompactionTargetFingerprint(account, credentialAccount, target)
	if target.ConfigFingerprint == "" {
		return OpenAICompactionProbeTarget{}, fmt.Errorf("credential identity facts are unavailable")
	}
	return target, nil
}

func (s *OpenAIGatewayService) resolveOpenAICompactionTargetForProtocol(ctx context.Context, account *Account, requestedModel, kind, protocol string) (OpenAICompactionProbeTarget, error) {
	credentialAccount := account
	if account != nil && account.IsShadow() {
		if s == nil || s.accountRepo == nil {
			return OpenAICompactionProbeTarget{}, fmt.Errorf("credential source facts are unavailable")
		}
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return OpenAICompactionProbeTarget{}, err
		}
		credentialAccount = resolved
	}
	return s.resolveOpenAICompactionTargetWithCredentialProtocol(ctx, account, credentialAccount, requestedModel, kind, false, protocol)
}

// resolveOpenAICompactionTargetFromProjection resolves only facts present in the
// scheduler projection. It never reads the database or fetches a model catalog.
func (s *OpenAIGatewayService) resolveOpenAICompactionTargetFromProjection(account *Account, requestedModel, kind string) (OpenAICompactionProbeTarget, bool) {
	target, known, err := s.resolveOpenAICompactionTargetRoute(context.Background(), account, requestedModel, kind, false, "")
	if err != nil || account == nil {
		return OpenAICompactionProbeTarget{}, false
	}
	credentialSource := "direct"
	credentialAccountID := account.ID
	var digest string
	projectedDigestAvailable := false
	if account.Extra != nil {
		if value, ok := account.Extra[OpenAICompactionCredentialFingerprintExtraKey]; ok {
			projectedDigestAvailable = true
			digest, _ = value.(string)
		}
	}
	if account.IsShadow() {
		credentialSource = "credential_shadow"
		credentialAccountID = 0
		if account.ParentAccountID != nil {
			credentialAccountID = *account.ParentAccountID
		}
	} else if !projectedDigestAvailable {
		// Full accounts without a scheduler projection still carry their real
		// credential identity. Projected accounts always include this key, even
		// when the identity could not be derived, and must remain unknown.
		digest = OpenAICompactionCredentialFingerprint(account)
	}
	target.CredentialAccountID = credentialAccountID
	target.CredentialSource = credentialSource
	target.CredentialFingerprint = strings.TrimSpace(digest)
	target.AuthMode = openAICompactionAuthMode(account, &Account{ID: credentialAccountID})
	if target.CredentialFingerprint != "" {
		target.ConfigFingerprint = openAICompactionTargetFingerprintWithDigest(account, target, target.CredentialFingerprint)
	}
	return target, known
}

func openAICompactionKindForScheduling(requireCompact bool, requiredCapability OpenAIEndpointCapability) string {
	if requireCompact {
		return openAICompactionKindLegacy
	}
	if requiredCapability == OpenAIEndpointCapabilityRemoteCompactionV2 {
		return openAICompactionKindV2
	}
	return ""
}

func openAICompactionProviderCapability(account *Account, kind string) ProviderCapabilityStatus {
	if account == nil {
		return ProviderCapabilityUnknown
	}
	if kind != openAICompactionKindV2 {
		return ProviderCapabilityUnknown
	}
	return account.remoteCompactionV2Capability()
}

func (s *OpenAIGatewayService) openAICompactionRequestDecision(ctx context.Context, account *Account, requestedModel, kind string, selected bool) OpenAICompactionRequestDecision {
	return s.openAICompactionRequestDecisionWithProtocol(ctx, account, requestedModel, kind, selected, "")
}

func (s *OpenAIGatewayService) openAICompactionRequestDecisionWithProtocol(ctx context.Context, account *Account, requestedModel, kind string, selected bool, actualProtocol string) OpenAICompactionRequestDecision {
	decision := OpenAICompactionRequestDecision{
		Kind:      kind,
		Execution: openAICompactionExecutionUnsupported,
		Tier:      openAICompactionTierUnsupported,
		Mode:      OpenAICompactModeAuto,
	}
	if account == nil {
		decision.Reason = "account_unavailable"
		return decision
	}
	decision.Mode = account.GetOpenAICompactMode()
	if account.IsGrok() {
		decision.Execution = openAICompactionExecutionNative
		decision.Tier = openAICompactionTierVerified
		decision.Reason = "grok_native"
		return decision
	}
	if !account.IsOpenAICompatible() {
		decision.Reason = "account_not_openai_compatible"
		return decision
	}

	target, protocolKnown := s.resolveOpenAICompactionTargetFromProjection(account, requestedModel, kind)
	if selected {
		var resolved OpenAICompactionProbeTarget
		var err error
		if strings.TrimSpace(actualProtocol) != "" {
			resolved, err = s.resolveOpenAICompactionTargetForProtocol(ctx, account, requestedModel, kind, actualProtocol)
		} else {
			resolved, err = s.resolveOpenAICompactionTarget(ctx, account, requestedModel, kind, false)
		}
		switch {
		case err == nil:
			target = resolved
			protocolKnown = strings.TrimSpace(resolved.Protocol) != ""
		case errors.Is(err, ErrOpenAICompactionProtocolUnknown):
			// A cold catalog leaves the projected target unknown. Existing exact
			// evidence remains usable only when its fingerprint already matched.
		default:
			// A selected account whose credential source cannot be resolved must
			// not reuse a previously cached fingerprint or verdict.
			target.ConfigFingerprint = ""
			target.CredentialFingerprint = ""
		}
	}
	decision.Target = target
	decision.ProviderCapability = openAICompactionProviderCapability(account, kind)
	if target.ConfigFingerprint != "" {
		if verdict, ok := OpenAICompactionProbeVerdictForTarget(account, target); ok {
			decision.Verdict = verdict
			decision.VerdictKnown = true
		}
	}
	summaryEnabled := account.OpenAICompactSummaryEnabled()
	protocolAllowsNative := true
	if protocolKnown {
		protocolAllowsNative = target.Protocol == APIProtocolResponses
	}
	var nativeAllowed bool
	switch decision.Mode {
	case OpenAICompactModeForceOff:
		nativeAllowed = false
	case OpenAICompactModeForceOn:
		// The explicit override permits an attempt, but never overrides the
		// selected account's non-Responses wire protocol.
		nativeAllowed = protocolAllowsNative
	default:
		switch {
		case !protocolAllowsNative:
			nativeAllowed = false
		case decision.VerdictKnown && decision.Verdict == OpenAICompactionProbeVerdictSupported:
			nativeAllowed = true
		case decision.VerdictKnown && decision.Verdict == OpenAICompactionProbeVerdictUnsupported:
			nativeAllowed = false
		case decision.ProviderCapability == ProviderCapabilitySupported:
			nativeAllowed = true
		case decision.ProviderCapability == ProviderCapabilityUnsupported:
			nativeAllowed = false
		case summaryEnabled:
			// Unknown native support with an explicit summary opt-in uses the
			// deterministic server-side path instead of gambling on a 4xx.
			nativeAllowed = false
		default:
			nativeAllowed = true
		}
	}
	if nativeAllowed {
		decision.Execution = openAICompactionExecutionNative
		switch {
		case decision.VerdictKnown && decision.Verdict == OpenAICompactionProbeVerdictSupported:
			decision.Tier = openAICompactionTierVerified
			decision.Reason = "probe_supported"
		case decision.ProviderCapability == ProviderCapabilitySupported:
			decision.Tier = openAICompactionTierDeclared
			decision.Reason = "provider_supported"
		default:
			decision.Tier = openAICompactionTierUnknown
			decision.Reason = "unknown_native_attempt"
		}
		return decision
	}
	if summaryEnabled {
		decision.Execution = openAICompactionExecutionSummary
		decision.Tier = openAICompactionTierUnknown
		decision.Reason = "summary_fallback"
		return decision
	}
	decision.Reason = "native_compaction_unsupported"
	return decision
}

func (s *OpenAIGatewayService) openAICompactionTargetEndpoint(account *Account, protocol, kind string) (string, error) {
	if s == nil {
		s = &OpenAIGatewayService{}
	}
	switch protocol {
	case APIProtocolResponses:
		endpoint, err := s.openAIResponsesTargetURL(account)
		if err != nil {
			return "", err
		}
		if kind == openAICompactionKindLegacy {
			endpoint = appendOpenAIResponsesRequestPathSuffix(endpoint, "/compact")
		}
		return endpoint, nil
	case APIProtocolChatCompletions:
		return s.openAIChatCompletionsTargetURL(account)
	case APIProtocolAnthropic:
		return s.nativeAnthropicTargetURL(account)
	default:
		return "", fmt.Errorf("unsupported compaction target protocol %q", protocol)
	}
}

func sanitizeOpenAICompactionEndpoint(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

func classifyOpenAICompactionProbeObservation(resp *http.Response, body []byte, probeErr error, result openAICompactProbeResult, now time.Time) OpenAICompactionProbeObservation {
	obs := OpenAICompactionProbeObservation{CheckedAt: now, Verdict: OpenAICompactionProbeVerdictUnknown, ErrorClass: openAICompactionErrorTransport}
	if resp != nil {
		obs.Status = resp.StatusCode
	}
	if probeErr != nil {
		if errors.Is(probeErr, context.DeadlineExceeded) {
			obs.ErrorClass = openAICompactionErrorTimeout
		} else {
			var netErr net.Error
			if errors.As(probeErr, &netErr) && netErr.Timeout() {
				obs.ErrorClass = openAICompactionErrorTimeout
			}
		}
		obs.Error = openAICompactionProbeSafeError(obs.ErrorClass, obs.Status)
		return obs
	}
	if resp == nil {
		obs.Error = openAICompactionProbeSafeError(obs.ErrorClass, obs.Status)
		return obs
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		obs.ErrorClass = openAICompactionErrorAuth
	case resp.StatusCode == http.StatusTooManyRequests:
		obs.ErrorClass = openAICompactionErrorRateLimit
	case resp.StatusCode >= 500:
		obs.ErrorClass = openAICompactionErrorUpstream5xx
	case isResponsesProbeModelUnavailable(resp.StatusCode, body):
		obs.ErrorClass = openAICompactionErrorModelNotFound
	case resp.StatusCode == http.StatusNotFound:
		obs.Verdict = OpenAICompactionProbeVerdictUnsupported
		obs.ErrorClass = openAICompactionErrorEndpointUnsupported
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		switch result {
		case openAICompactProbeSupported:
			obs.Verdict = OpenAICompactionProbeVerdictSupported
			obs.ErrorClass = openAICompactionErrorNone
			obs.Error = ""
		case openAICompactProbeUnsupported:
			obs.Verdict = OpenAICompactionProbeVerdictUnsupported
			obs.ErrorClass = openAICompactionErrorInvalidContract
			if obs.Error == "" {
				obs.Error = "upstream terminal did not contain exactly one consumable compaction item"
			}
		default:
			obs.ErrorClass = openAICompactionErrorTruncated
		}
	case shouldMarkOpenAICompactUnsupported(resp.StatusCode, body):
		obs.Verdict = OpenAICompactionProbeVerdictUnsupported
		obs.ErrorClass = openAICompactionErrorEndpointUnsupported
	default:
		obs.ErrorClass = openAICompactionErrorRequest
	}
	obs.Error = openAICompactionProbeSafeError(obs.ErrorClass, obs.Status)
	return obs
}

func openAICompactionProbeSafeError(errorClass string, status int) string {
	switch errorClass {
	case openAICompactionErrorAuth:
		return "upstream authentication failed"
	case openAICompactionErrorRateLimit:
		return "upstream rate limited the probe"
	case openAICompactionErrorUpstream5xx:
		return fmt.Sprintf("upstream returned HTTP %d", status)
	case openAICompactionErrorTimeout:
		return "compact probe timed out"
	case openAICompactionErrorTransport:
		return "compact probe transport failed"
	case openAICompactionErrorTruncated:
		return "upstream response ended without a successful terminal"
	case openAICompactionErrorModelNotFound:
		return "model is unavailable on this target"
	case openAICompactionErrorEndpointUnsupported:
		return "target endpoint does not support native compaction"
	case openAICompactionErrorInvalidContract:
		return "target terminal did not contain exactly one consumable compaction item"
	case openAICompactionErrorRequest:
		return fmt.Sprintf("target rejected the compact probe with HTTP %d", status)
	default:
		return ""
	}
}

func decodeOpenAICompactionProbeRecords(extra map[string]any) map[string]OpenAICompactionProbeRecord {
	raw, ok := extra[OpenAICompactionProbeTargetsExtraKey]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var records map[string]OpenAICompactionProbeRecord
	if err := json.Unmarshal(encoded, &records); err != nil || len(records) > openAICompactionProbeMaxTargets*2 {
		return nil
	}
	return records
}

func BuildOpenAICompactionProbeExtraUpdates(extra map[string]any, target OpenAICompactionProbeTarget, obs OpenAICompactionProbeObservation) (map[string]any, error) {
	if strings.TrimSpace(target.ID()) == "" || strings.TrimSpace(target.ConfigFingerprint) == "" {
		return nil, fmt.Errorf("invalid compaction probe target")
	}
	records := decodeOpenAICompactionProbeRecords(extra)
	if records == nil {
		records = make(map[string]OpenAICompactionProbeRecord)
	}
	key := target.ID()
	record := OpenAICompactionProbeRecord{Version: openAICompactionProbeRecordVersion, Target: target}
	if obs.Verdict == OpenAICompactionProbeVerdictUnknown {
		if previous, ok := records[key]; ok && previous.Target.ConfigFingerprint == target.ConfigFingerprint {
			record = previous
			record.Target = target
		}
	} else {
		record.Verdict = obs.Verdict
		record.VerifiedAt = obs.CheckedAt.UTC().Format(time.RFC3339Nano)
	}
	if record.Verdict == "" {
		record.Verdict = OpenAICompactionProbeVerdictUnknown
	}
	record.LastCheckedAt = obs.CheckedAt.UTC().Format(time.RFC3339Nano)
	record.LastStatus = obs.Status
	record.ErrorClass = obs.ErrorClass
	record.Error = truncateString(obs.Error, 512)
	records[key] = record
	pruneOpenAICompactionProbeRecords(records)
	return map[string]any{OpenAICompactionProbeTargetsExtraKey: records}, nil
}

func pruneOpenAICompactionProbeRecords(records map[string]OpenAICompactionProbeRecord) {
	if len(records) <= openAICompactionProbeMaxTargets {
		return
	}
	type entry struct{ key, checked string }
	entries := make([]entry, 0, len(records))
	for key, record := range records {
		entries = append(entries, entry{key: key, checked: record.LastCheckedAt})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].checked == entries[j].checked {
			return entries[i].key < entries[j].key
		}
		return entries[i].checked < entries[j].checked
	})
	for i := 0; i < len(entries)-openAICompactionProbeMaxTargets; i++ {
		delete(records, entries[i].key)
	}
}

func OpenAICompactionProbeVerdictForTarget(account *Account, target OpenAICompactionProbeTarget) (string, bool) {
	if account == nil {
		return OpenAICompactionProbeVerdictUnknown, false
	}
	record, ok := decodeOpenAICompactionProbeRecords(account.Extra)[target.ID()]
	if !ok || record.Target.ConfigFingerprint != target.ConfigFingerprint || record.Version != openAICompactionProbeRecordVersion {
		return OpenAICompactionProbeVerdictUnknown, false
	}
	switch record.Verdict {
	case OpenAICompactionProbeVerdictSupported, OpenAICompactionProbeVerdictUnsupported, OpenAICompactionProbeVerdictUnknown:
		return record.Verdict, true
	default:
		return OpenAICompactionProbeVerdictUnknown, false
	}
}
