package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var ErrCodexDeviceProfileUnsupported = ErrDeviceProfileUnsupported

type CodexDeviceProfileUnsupportedError struct {
	Profile CodexClientProfile
}

func (e *CodexDeviceProfileUnsupportedError) Error() string {
	if e == nil {
		return ErrCodexDeviceProfileUnsupported.Error()
	}
	return fmt.Sprintf("no available OAuth account supports Codex profile %s", e.Profile.Key())
}

func (e *CodexDeviceProfileUnsupportedError) Unwrap() error {
	return ErrDeviceProfileUnsupported
}

func (e *CodexDeviceProfileUnsupportedError) Code() string {
	return "DEVICE_PROFILE_UNSUPPORTED"
}

type codexProfileRequestContextKey struct{}
type codexProfileAffinityActiveContextKey struct{}

type codexProfileRequest struct {
	Profile          CodexClientProfile
	APIKeyScope      string
	ConversationHash string
	state            *codexProfileRequestState
}

type codexProfileRequestState struct {
	affinityActive atomic.Bool
}

const codexIdentityAttemptPlanContextKey = "codex_identity_attempt_plan"

func withCodexProfileRequest(ctx context.Context, request codexProfileRequest) context.Context {
	if ctx == nil {
		return nil
	}
	request.APIKeyScope = strings.TrimSpace(request.APIKeyScope)
	request.ConversationHash = strings.TrimSpace(request.ConversationHash)
	if request.state == nil {
		request.state = &codexProfileRequestState{}
	}
	return context.WithValue(ctx, codexProfileRequestContextKey{}, request)
}

func codexProfileRequestFromContext(ctx context.Context) (codexProfileRequest, bool) {
	if ctx == nil {
		return codexProfileRequest{}, false
	}
	request, ok := ctx.Value(codexProfileRequestContextKey{}).(codexProfileRequest)
	if !ok || strings.TrimSpace(request.APIKeyScope) == "" ||
		(request.Profile.OSClass == "" && !request.Profile.Ambiguous) {
		return codexProfileRequest{}, false
	}
	return request, true
}

func withCodexProfileAffinityActive(ctx context.Context) context.Context {
	if ctx == nil {
		return nil
	}
	markCodexProfileAffinityActive(ctx)
	return context.WithValue(ctx, codexProfileAffinityActiveContextKey{}, true)
}

func markCodexProfileAffinityActive(ctx context.Context) {
	if request, ok := codexProfileRequestFromContext(ctx); ok && request.state != nil {
		request.state.affinityActive.Store(true)
	}
}

func codexProfileAffinityActive(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	active, _ := ctx.Value(codexProfileAffinityActiveContextKey{}).(bool)
	if active {
		return true
	}
	request, ok := codexProfileRequestFromContext(ctx)
	return ok && request.state != nil && request.state.affinityActive.Load()
}

func stageCodexProfileRequest(c *gin.Context, body []byte, conversationHash string) {
	if c == nil || c.Request == nil {
		return
	}
	if !isCodexProfileIdentityRoute(c.Request.URL.Path) {
		return
	}
	scope := HTTPUpstreamIsolationScopeFromContext(c.Request.Context())
	if scope == "" {
		return
	}
	profile := ClassifyCodexClientProfile(CodexClientProfileSignals{
		Headers: c.Request.Header,
		Body:    body,
	})
	var state *codexProfileRequestState
	if existing, ok := codexProfileRequestFromContext(c.Request.Context()); ok {
		state = existing.state
	}
	c.Request = c.Request.WithContext(withCodexProfileRequest(c.Request.Context(), codexProfileRequest{
		Profile:          profile,
		APIKeyScope:      scope,
		ConversationHash: conversationHash,
		state:            state,
	}))
}

func isCodexProfileIdentityRoute(path string) bool {
	path = strings.TrimSuffix(strings.TrimSpace(path), "/")
	return strings.HasSuffix(path, "/responses") || strings.HasSuffix(path, "/responses/compact")
}

func stagedCodexIdentityAttemptPlan(c *gin.Context, account *Account) *CodexIdentityAttemptPlan {
	if c == nil || account == nil {
		return nil
	}
	value, ok := c.Get(codexIdentityAttemptPlanContextKey)
	if !ok {
		return nil
	}
	plan, ok := value.(*CodexIdentityAttemptPlan)
	if !ok || plan == nil || plan.AccountID != account.ID {
		return nil
	}
	return plan
}

func stageCodexIdentityAttemptPlan(c *gin.Context, plan *CodexIdentityAttemptPlan) {
	if c != nil {
		c.Set(codexIdentityAttemptPlanContextKey, plan)
	}
}

func refreshCodexProfileTurnPlan(c *gin.Context, account *Account, payload []byte) (*CodexIdentityAttemptPlan, error) {
	current := stagedCodexIdentityAttemptPlan(c, account)
	if current == nil {
		return nil, nil
	}
	if c == nil || c.Request == nil {
		return nil, errors.New("codex profile turn requires an HTTP request context")
	}
	seed, ok := codexFingerprintSeed(account.Extra)
	if !ok {
		return nil, errors.New("codex profile turn requires an account fingerprint seed")
	}
	request, ok := codexProfileRequestFromContext(c.Request.Context())
	if !ok {
		return nil, errors.New("codex profile turn requires a classified request context")
	}
	frameSource := ExtractCodexIdentitySource(nil, payload)
	source := preferCodexIdentitySource(frameSource, codexIdentitySourceFromPlan(current))
	constraints := CodexSessionRuntimeConstraints{
		MaxActiveConversationsPerSlot: current.SessionPolicy.MaxActiveConversationsPerSlot,
		DisableCrossKeyContinuation:   current.SessionPolicy.DisableCrossKeyContinuation,
	}
	next, err := BuildCodexIdentityAttemptPlan(CodexIdentityAttemptInput{
		Mode:            account.CodexIdentityPolicy.Mode,
		AccountID:       account.ID,
		APIKeyScope:     current.APIKeyScope,
		AccountSeed:     seed,
		Profile:         current.Profile,
		Slot:            current.Slot,
		SessionPolicy:   current.SessionPolicy,
		SessionRuntime:  constraints,
		ConversationKey: request.ConversationHash,
		RequestNonce:    uuid.NewString(),
		Source:          source,
	})
	if err != nil {
		return nil, err
	}
	next.conversationLease = current.conversationLease
	current.conversationLease = nil
	stageCodexIdentityAttemptPlan(c, next)
	return next, nil
}

func preferCodexIdentitySource(primary, fallback CodexIdentitySource) CodexIdentitySource {
	profileFields := make(map[string]CodexProfileFieldSource, len(fallback.ProfileFields)+len(primary.ProfileFields))
	for field, value := range fallback.ProfileFields {
		profileFields[field] = value
	}
	for field, value := range primary.ProfileFields {
		profileFields[field] = value
	}
	return CodexIdentitySource{
		InstallationID: firstNonEmptyCodexIdentityValue(primary.InstallationID, fallback.InstallationID),
		SessionID:      firstNonEmptyCodexIdentityValue(primary.SessionID, fallback.SessionID),
		ConversationID: firstNonEmptyCodexIdentityValue(primary.ConversationID, fallback.ConversationID),
		ThreadID:       firstNonEmptyCodexIdentityValue(primary.ThreadID, fallback.ThreadID),
		TurnID:         firstNonEmptyCodexIdentityValue(primary.TurnID, fallback.TurnID),
		WindowID:       firstNonEmptyCodexIdentityValue(primary.WindowID, fallback.WindowID),
		PromptCacheKey: firstNonEmptyCodexIdentityValue(primary.PromptCacheKey, fallback.PromptCacheKey),
		Workspace:      firstNonEmptyCodexIdentityValue(primary.Workspace, fallback.Workspace),
		GitRemote:      firstNonEmptyCodexIdentityValue(primary.GitRemote, fallback.GitRemote),
		GitCommit:      firstNonEmptyCodexIdentityValue(primary.GitCommit, fallback.GitCommit),
		ProfileFields:  profileFields,
	}
}

func codexIdentitySourceFromPlan(plan *CodexIdentityAttemptPlan) CodexIdentitySource {
	source := CodexIdentitySource{ProfileFields: make(map[string]CodexProfileFieldSource)}
	if plan == nil {
		return source
	}
	for _, mapping := range plan.RequestMappings {
		switch mapping.Kind {
		case CodexIdentityInstallation:
			source.InstallationID = mapping.ClientValue
		case CodexIdentitySession:
			source.SessionID = mapping.ClientValue
		case CodexIdentityConversation:
			source.ConversationID = mapping.ClientValue
		case CodexIdentityThread:
			source.ThreadID = mapping.ClientValue
		case CodexIdentityTurn:
			source.TurnID = mapping.ClientValue
		case CodexIdentityWindow:
			source.WindowID = mapping.ClientValue
		case CodexIdentityPromptCache:
			source.PromptCacheKey = mapping.ClientValue
		case CodexIdentityWorkspace:
			source.Workspace = mapping.ClientValue
		case CodexIdentityGitRemote:
			source.GitRemote = mapping.ClientValue
		case CodexIdentityGitCommit:
			source.GitCommit = mapping.ClientValue
		}
	}
	for _, mapping := range plan.ProfileMappings {
		source.ProfileFields[mapping.Field] = CodexProfileFieldSource{
			Value: mapping.ClientValue, Present: mapping.ClientPresent,
		}
	}
	return source
}

func codexProfilePolicyForClient(account *Account, client CodexClientProfile) (CodexOSProfilePolicy, CodexResolvedProfile, bool) {
	if account == nil || client.Ambiguous || account.CodexIdentityPolicy.Mode != CodexIdentityPolicyOSProfileDevicePool {
		return CodexOSProfilePolicy{}, CodexResolvedProfile{}, false
	}
	for _, policy := range account.CodexIdentityPolicy.Profiles {
		if policy.OSClass != client.OSClass || policy.CanonicalSurface != client.Surface {
			continue
		}
		resolved, err := ResolveCodexRuntimeProfile(policy)
		if err != nil || !resolved.Supports(client) {
			return CodexOSProfilePolicy{}, CodexResolvedProfile{}, false
		}
		return policy, resolved, true
	}
	return CodexOSProfilePolicy{}, CodexResolvedProfile{}, false
}

func (s *OpenAIGatewayService) codexProfileAccountCompatible(ctx context.Context, account *Account) bool {
	if account == nil {
		return false
	}
	if account.CodexIdentityPolicy.Mode == CodexIdentityPolicyOff || account.CodexIdentityPolicy.Mode == "" {
		return !codexProfileAffinityActive(ctx)
	}
	request, ok := codexProfileRequestFromContext(ctx)
	if !ok {
		// Non-Codex endpoints share this scheduler but do not stage a client
		// profile. They retain the pre-feature account eligibility behavior.
		return true
	}
	_, _, compatible := codexProfilePolicyForClient(account, request.Profile)
	return compatible
}

func codexProfileUnsupportedFromContext(ctx context.Context) error {
	request, ok := codexProfileRequestFromContext(ctx)
	if !ok {
		return ErrCodexDeviceProfileUnsupported
	}
	return &CodexDeviceProfileUnsupportedError{Profile: request.Profile}
}

// SelectCodexDeviceSlotIndex uses rendezvous hashing so increasing a profile's
// slot count moves only keys that prefer the new slot.
func SelectCodexDeviceSlotIndex(seed, apiKeyScope string, profile CodexResolvedProfile, slotCount int) (int, error) {
	if _, ok := canonicalCodexFingerprintSeed(seed); !ok {
		return 0, errors.New("codex slot selection requires a canonical account seed")
	}
	apiKeyScope = strings.TrimSpace(apiKeyScope)
	if apiKeyScope == "" || profile.Key() == "" || slotCount < 1 || slotCount > maxCodexDeviceSlotsPerProfile {
		return 0, errors.New("invalid Codex slot selection input")
	}
	selected := 0
	var selectedScore uint64
	for index := 0; index < slotCount; index++ {
		digest := deriveCodexIdentityDigest(seed, "device-slot-choice:v1", profile.Key(), apiKeyScope, strconv.Itoa(index))
		score := binary.BigEndian.Uint64(digest[:8])
		if index == 0 || score > selectedScore {
			selected = index
			selectedScore = score
		}
	}
	return selected, nil
}

func effectiveCodexProfileProxyID(account *Account, profile CodexOSProfilePolicy, slotIndex int) *int64 {
	for _, slot := range profile.Slots {
		if slot.Index != slotIndex {
			continue
		}
		switch slot.ProxyMode {
		case CodexProxyDirect:
			return nil
		case CodexProxyExplicit, "":
			if slot.ProxyID != nil {
				id := *slot.ProxyID
				return &id
			}
		}
	}
	switch profile.ProxyMode {
	case CodexProxyDirect:
		return nil
	case CodexProxyExplicit, "":
		if profile.ProxyID != nil {
			id := *profile.ProxyID
			return &id
		}
	}
	if account != nil && account.ProxyID != nil {
		id := *account.ProxyID
		return &id
	}
	return nil
}

func codexProfileSlotClientVersion(profile CodexOSProfilePolicy, slotIndex int) (CodexClientVersionMode, string) {
	for _, slot := range profile.Slots {
		if slot.Index == slotIndex {
			return slot.ClientVersionMode, slot.ClientVersion
		}
	}
	return CodexClientVersionInherit, ""
}

func (s *OpenAIGatewayService) resolveCodexDeviceSlotClientVersion(ctx context.Context, slot *CodexResolvedDeviceSlot) (string, error) {
	if slot == nil {
		return "", errors.New("codex client version requires a resolved device slot")
	}
	var settings *SettingService
	if s != nil {
		settings = s.settingService
	}
	return resolveEffectiveCodexClientVersion(ctx, settings, slot.ClientVersionMode, slot.ClientVersion)
}

func (s *OpenAIGatewayService) prepareCodexProfileAttempt(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*Account, error) {
	previousPlan, _ := func() (*CodexIdentityAttemptPlan, bool) {
		if c == nil {
			return nil, false
		}
		value, ok := c.Get(codexIdentityAttemptPlanContextKey)
		plan, typed := value.(*CodexIdentityAttemptPlan)
		return plan, ok && typed && plan != nil
	}()
	stageCodexIdentityAttemptPlan(c, nil)
	if account == nil || account.CodexIdentityPolicy.Mode == "" || account.CodexIdentityPolicy.Mode == CodexIdentityPolicyOff {
		return account, nil
	}
	if account.CodexIdentityPolicy.Mode != CodexIdentityPolicyOSProfileDevicePool {
		return nil, fmt.Errorf("unsupported Codex identity policy mode %q", account.CodexIdentityPolicy.Mode)
	}
	if c != nil && c.Request != nil && !isCodexProfileIdentityRoute(c.Request.URL.Path) {
		return account, nil
	}

	request, ok := codexProfileRequestFromContext(ctx)
	if !ok && c != nil && c.Request != nil {
		stageCodexProfileRequest(c, body, "")
		request, ok = codexProfileRequestFromContext(c.Request.Context())
	}
	if !ok {
		return nil, codexProfileUnsupportedFromContext(ctx)
	}
	profilePolicy, resolvedProfile, compatible := codexProfilePolicyForClient(account, request.Profile)
	if !compatible {
		return nil, &CodexDeviceProfileUnsupportedError{Profile: request.Profile}
	}
	seed, ok := codexFingerprintSeed(account.Extra)
	if !ok {
		return nil, errors.New("codex OS profile device pool requires an account fingerprint seed")
	}
	_, apiKeyID, ok := HTTPUpstreamIsolationIDsFromContext(ctx)
	if !ok && c != nil && c.Request != nil {
		_, apiKeyID, ok = HTTPUpstreamIsolationIDsFromContext(c.Request.Context())
	}
	if !ok || apiKeyID <= 0 {
		return nil, errors.New("codex OS profile device pool requires an authenticated API key")
	}
	slotIndex, err := SelectCodexDeviceSlotIndex(seed, request.APIKeyScope, resolvedProfile, profilePolicy.SlotCount)
	if err != nil {
		return nil, err
	}
	effectiveProxyID := effectiveCodexProfileProxyID(account, profilePolicy, slotIndex)
	resolvedBinding := &CodexResolvedDeviceSlot{
		AccountID:        account.ID,
		APIKeyID:         apiKeyID,
		OSClass:          request.Profile.OSClass,
		CanonicalSurface: profilePolicy.CanonicalSurface,
		Architecture:     profilePolicy.Architecture,
		CatalogVersion:   profilePolicy.CatalogVersion,
		SlotIndex:        slotIndex,
		Epoch:            profilePolicy.Epoch,
		PolicyVersion:    account.CodexIdentityPolicy.Version,
		ProxyID:          effectiveProxyID,
	}
	if bindingRepo, supported := s.accountRepo.(CodexDeviceBindingRepository); supported {
		if previousPlan != nil && previousPlan.AccountID != account.ID {
			resolvedBinding, err = bindingRepo.RebindCodexDeviceBinding(
				ctx,
				previousPlan.AccountID,
				account.ID,
				apiKeyID,
				request.Profile.OSClass,
				request.Profile.Surface,
			)
		} else {
			resolvedBinding, err = bindingRepo.ResolveCodexDeviceBinding(ctx, account.ID, apiKeyID, request.Profile.OSClass, request.Profile.Surface)
		}
		if err != nil {
			return nil, fmt.Errorf("resolve Codex device binding: %w", err)
		}
	} else if effectiveProxyID != nil && (account.ProxyID == nil || *effectiveProxyID != *account.ProxyID) {
		return nil, errors.New("codex profile proxy override requires a runtime binding repository")
	}
	if resolvedBinding == nil {
		return nil, errors.New("codex device binding resolver returned nil")
	}
	var conversationLease *CodexDeviceConversationLease
	if account.CodexIdentityPolicy.SessionPolicy.Mode == CodexSessionDeviceShared {
		resolvedBinding, conversationLease, err = s.acquireCodexConversationSlot(ctx, account, request, resolvedBinding, body)
		if err != nil {
			return nil, err
		}
		// A failed version/profile/identity adaptation must not strand capacity.
		defer func() {
			if conversationLease != nil {
				conversationLease.Release()
			}
		}()
	}
	clientVersionMode, clientVersion := codexProfileSlotClientVersion(profilePolicy, resolvedBinding.SlotIndex)
	if strings.TrimSpace(string(resolvedBinding.ClientVersionMode)) == "" {
		resolvedBinding.ClientVersionMode = clientVersionMode
	}
	if strings.TrimSpace(resolvedBinding.ClientVersion) == "" {
		resolvedBinding.ClientVersion = clientVersion
	}
	effectiveClientVersion, err := s.resolveCodexDeviceSlotClientVersion(ctx, resolvedBinding)
	if err != nil {
		return nil, fmt.Errorf("resolve bound Codex client version: %w", err)
	}
	attemptProfile, err := ResolveCodexRuntimeProfileWithVersion(CodexOSProfilePolicy{
		OSClass:          resolvedBinding.OSClass,
		CanonicalSurface: resolvedBinding.CanonicalSurface,
		Architecture:     resolvedBinding.Architecture,
		SlotCount:        1,
		Epoch:            resolvedBinding.Epoch,
		CatalogVersion:   resolvedBinding.CatalogVersion,
	}, effectiveClientVersion)
	if err != nil {
		return nil, fmt.Errorf("resolve bound Codex profile: %w", err)
	}
	resolvedAccount := *account
	if resolvedBinding.Proxy != nil {
		proxy := *resolvedBinding.Proxy
		resolvedAccount.Proxy = &proxy
		resolvedAccount.ProxyID = &proxy.ID
	} else if resolvedBinding.ProxyID == nil {
		resolvedAccount.Proxy = nil
		resolvedAccount.ProxyID = nil
	} else if account.ProxyID == nil || *account.ProxyID != *resolvedBinding.ProxyID || account.Proxy == nil {
		return nil, errors.New("resolved Codex profile proxy was not hydrated")
	}

	source := ExtractCodexIdentitySource(nil, body)
	if c != nil && c.Request != nil {
		source = ExtractCodexIdentitySource(c.Request.Header, body)
	}
	constraints := CodexSessionRuntimeConstraints{
		MaxActiveConversationsPerSlot: account.CodexIdentityPolicy.SessionPolicy.MaxActiveConversationsPerSlot,
		DisableCrossKeyContinuation:   account.CodexIdentityPolicy.SessionPolicy.DisableCrossKeyContinuation,
	}
	plan, err := BuildCodexIdentityAttemptPlan(CodexIdentityAttemptInput{
		Mode:        account.CodexIdentityPolicy.Mode,
		AccountID:   account.ID,
		APIKeyScope: request.APIKeyScope,
		AccountSeed: seed,
		Profile:     attemptProfile,
		Slot: CodexResolvedSlot{
			Index:   resolvedBinding.SlotIndex,
			Epoch:   resolvedBinding.Epoch,
			ProxyID: resolvedBinding.ProxyID,
		},
		SessionPolicy:   account.CodexIdentityPolicy.SessionPolicy,
		SessionRuntime:  constraints,
		ConversationKey: request.ConversationHash,
		RequestNonce:    uuid.NewString(),
		Source:          source,
	})
	if err != nil {
		return nil, err
	}
	plan.conversationLease = conversationLease
	conversationLease = nil // Ownership transfers to the attempt lifecycle.

	if previousPlan != nil && previousPlan.AccountID != account.ID {
		if store := s.getOpenAIWSStateStore(); store != nil && request.ConversationHash != "" {
			groupID := getOpenAIGroupIDFromContext(c)
			stateKey := scopedOpenAIWSStateKey(ctx, request.ConversationHash)
			store.DeleteSessionTurnState(groupID, stateKey)
			store.DeleteSessionConn(groupID, stateKey)
		}
	}
	stageCodexIdentityAttemptPlan(c, plan)
	markCodexProfileAffinityActive(ctx)
	if c != nil && c.Request != nil {
		markCodexProfileAffinityActive(c.Request.Context())
	}
	return &resolvedAccount, nil
}

// PrepareCodexProfileAttempt is the handler boundary for direct WS ingress,
// whose account attempts do not pass through Forward.
func (s *OpenAIGatewayService) PrepareCodexProfileAttempt(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*Account, error) {
	return s.prepareCodexProfileAttempt(ctx, c, account, body)
}

func (s *OpenAIGatewayService) ReleaseCodexProfileAttempt(c *gin.Context, account *Account) {
	plan := stagedCodexIdentityAttemptPlan(c, account)
	if plan == nil || plan.conversationLease == nil {
		return
	}
	plan.conversationLease.Release()
	plan.conversationLease = nil
}

func (s *OpenAIGatewayService) RestoreCodexProfileRetryPayload(c *gin.Context, account *Account, payload []byte) ([]byte, error) {
	plan := stagedCodexIdentityAttemptPlan(c, account)
	restored, _, err := RestoreCodexIdentityJSON(payload, plan)
	return restored, err
}

func (s *OpenAIGatewayService) CodexProfileAttemptContext(c *gin.Context, account *Account, fallback context.Context) context.Context {
	plan := stagedCodexIdentityAttemptPlan(c, account)
	if plan != nil && plan.conversationLease != nil {
		return withCodexDeviceConversationLeaseContext(
			plan.conversationLease.Context(),
			plan.conversationLease,
		)
	}
	if fallback == nil {
		return context.Background()
	}
	return fallback
}

func applyStagedCodexProfileHeaders(c *gin.Context, account *Account, headers http.Header) bool {
	plan := stagedCodexIdentityAttemptPlan(c, account)
	if plan == nil || headers == nil {
		return false
	}
	headers.Set("x-codex-installation-id", plan.UpstreamValue(CodexIdentityInstallation))
	headers.Set("session-id", plan.UpstreamValue(CodexIdentitySession))
	headers.Set("session_id", plan.UpstreamValue(CodexIdentitySession))
	headers.Set("conversation_id", plan.UpstreamValue(CodexIdentityConversation))
	headers.Set("thread-id", plan.UpstreamValue(CodexIdentityThread))
	headers.Set("x-client-request-id", plan.UpstreamValue(CodexIdentityThread))
	headers.Set("x-codex-window-id", plan.UpstreamValue(CodexIdentityWindow))
	rewriteCodexProfileTurnMetadataHeader(headers, plan)
	return true
}

func rewriteCodexProfileTurnMetadataHeader(headers http.Header, plan *CodexIdentityAttemptPlan) {
	if headers == nil || plan == nil {
		return
	}
	raw := strings.TrimSpace(headers.Get("x-codex-turn-metadata"))
	if raw == "" {
		return
	}
	metadata := make(map[string]any)
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
		metadata = make(map[string]any)
	}
	fields := map[string]string{
		"installation_id": plan.UpstreamValue(CodexIdentityInstallation),
		"session_id":      plan.UpstreamValue(CodexIdentitySession),
		"conversation_id": plan.UpstreamValue(CodexIdentityConversation),
		"thread_id":       plan.UpstreamValue(CodexIdentityThread),
		"turn_id":         plan.UpstreamValue(CodexIdentityTurn),
		"window_id":       plan.UpstreamValue(CodexIdentityWindow),
		"os":              string(plan.Profile.OSClass),
		"platform":        string(plan.Profile.OSClass),
		"surface":         string(plan.Profile.Surface),
		"client":          plan.Profile.ClientName,
		"client_name":     plan.Profile.ClientName,
		"originator":      plan.Profile.Originator,
		"version":         plan.Profile.Version,
		"user_agent":      plan.Profile.UserAgent,
		"terminal":        plan.Profile.Terminal,
		"os_label":        plan.Profile.OSLabel,
		"app_build":       plan.Profile.AppBuild,
	}
	if plan.Profile.Architecture != "" {
		fields["arch"] = string(plan.Profile.Architecture)
		fields["architecture"] = string(plan.Profile.Architecture)
	} else {
		delete(metadata, "arch")
		delete(metadata, "architecture")
	}
	for key, value := range fields {
		if value == "" {
			delete(metadata, key)
			continue
		}
		metadata[key] = value
	}
	for kind, names := range map[CodexIdentityKind][]string{
		CodexIdentityWorkspace: {"workspace", "workspace_path", "cwd", "working_directory"},
		CodexIdentityGitRemote: {"git_remote", "git_remote_url", "remote_url"},
		CodexIdentityGitCommit: {"git_commit", "git_sha", "commit_sha"},
	} {
		value := plan.UpstreamValue(kind)
		if value == "" {
			continue
		}
		for _, name := range names {
			if _, exists := metadata[name]; exists {
				metadata[name] = value
			}
		}
		metadata[names[0]] = value
	}
	if rebuilt, err := json.Marshal(metadata); err == nil {
		headers.Set("x-codex-turn-metadata", string(rebuilt))
	}
}

func enforceCodexIdentityHeadersForAttempt(c *gin.Context, account *Account, headers http.Header, fallbackUA string) {
	if plan := stagedCodexIdentityAttemptPlan(c, account); plan != nil {
		headers.Set("user-agent", plan.Profile.UserAgent)
		headers.Set("originator", plan.Profile.Originator)
		headers.Set("version", plan.Profile.Version)
		return
	}
	enforceCodexIdentityHeadersWithUA(headers, fallbackUA)
}

func restoreStagedCodexIdentityJSON(c *gin.Context, account *Account, payload []byte) ([]byte, error) {
	plan := stagedCodexIdentityAttemptPlan(c, account)
	restored, _, err := RestoreCodexIdentityJSON(payload, plan)
	return restored, err
}

func restoreStagedCodexIdentitySSE(c *gin.Context, account *Account, payload []byte) ([]byte, error) {
	plan := stagedCodexIdentityAttemptPlan(c, account)
	restored, _, err := RestoreCodexIdentitySSE(payload, plan)
	return restored, err
}

func codexProfileAffinityKey(request codexProfileRequest, sessionHash string, policyVersion int64, index bool) string {
	base := strings.Join([]string{
		"codex-profile-affinity:v2",
		request.APIKeyScope,
		string(request.Profile.OSClass),
		string(request.Profile.Surface),
		strings.TrimSpace(sessionHash),
	}, "|")
	sum := sha256.Sum256([]byte(base))
	key := "codex-profile:"
	if index {
		key += "index:"
	} else {
		key += "binding:"
	}
	key += hex.EncodeToString(sum[:16])
	if !index {
		key += ":policy:" + strconv.FormatInt(policyVersion, 10)
	}
	return key
}

func (s *OpenAIGatewayService) codexProfileAccountByID(ctx context.Context, accountID int64) *Account {
	if s == nil || accountID <= 0 {
		return nil
	}
	if s.schedulerSnapshot != nil {
		if account, err := s.schedulerSnapshot.GetAccount(ctx, accountID); err == nil && account != nil {
			return account
		}
	}
	if s.accountRepo == nil {
		return nil
	}
	account, _ := s.accountRepo.GetByID(ctx, accountID)
	return account
}

func (s *OpenAIGatewayService) getCodexProfileAffinityAccountID(ctx context.Context, groupID *int64, sessionHash string) (int64, bool, error) {
	request, ok := codexProfileRequestFromContext(ctx)
	if !ok || s == nil || s.cache == nil || strings.TrimSpace(sessionHash) == "" {
		return 0, false, nil
	}
	indexKey := codexProfileAffinityKey(request, sessionHash, 0, true)
	accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), indexKey)
	if err != nil {
		if errors.Is(err, ErrStickySessionNotFound) {
			return 0, false, nil
		}
		return 0, true, err
	}
	if accountID <= 0 {
		return 0, false, nil
	}
	account := s.codexProfileAccountByID(ctx, accountID)
	if account == nil || !account.IsProvisioned() || !account.IsActive() || !account.Schedulable ||
		account.CodexIdentityPolicy.Mode != CodexIdentityPolicyOSProfileDevicePool || !s.codexProfileAccountCompatible(ctx, account) {
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), indexKey)
		if account != nil {
			bindingKey := codexProfileAffinityKey(request, sessionHash, account.CodexIdentityPolicy.Version, false)
			_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), bindingKey)
		}
		return 0, true, nil
	}
	markCodexProfileAffinityActive(ctx)
	bindingKey := codexProfileAffinityKey(request, sessionHash, account.CodexIdentityPolicy.Version, false)
	boundID, bindErr := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), bindingKey)
	if bindErr != nil || boundID != accountID {
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), indexKey)
		return 0, true, bindErr
	}
	return accountID, true, nil
}

func (s *OpenAIGatewayService) resolveCodexAwareStickyAccountID(ctx context.Context, groupID *int64, sessionHash string) (int64, error) {
	accountID, handled, err := s.getCodexProfileAffinityAccountID(ctx, groupID, sessionHash)
	if handled {
		return accountID, err
	}
	legacyAccountID, legacyErr := s.getStickySessionAccountID(ctx, groupID, sessionHash)
	if legacyErr != nil || legacyAccountID <= 0 {
		return legacyAccountID, legacyErr
	}
	legacyAccount := s.codexProfileAccountByID(ctx, legacyAccountID)
	if legacyAccount == nil {
		// Narrow tests and degraded instances without an account reader retain the
		// pre-feature behavior; production readers validate the target below.
		if s.accountRepo == nil && s.schedulerSnapshot == nil {
			return legacyAccountID, nil
		}
		_ = s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
		return 0, nil
	}
	if legacyAccount.CodexIdentityPolicy.Mode == "" || legacyAccount.CodexIdentityPolicy.Mode == CodexIdentityPolicyOff {
		return legacyAccountID, nil
	}
	_ = s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
	return 0, nil
}

func (s *OpenAIGatewayService) setCodexProfileAffinityAccountID(ctx context.Context, groupID *int64, sessionHash string, accountID int64) (bool, error) {
	request, ok := codexProfileRequestFromContext(ctx)
	if !ok || s == nil || s.cache == nil || strings.TrimSpace(sessionHash) == "" || accountID <= 0 {
		return false, nil
	}
	account := s.codexProfileAccountByID(ctx, accountID)
	if account == nil || !account.IsProvisioned() || !account.IsActive() || !account.Schedulable || account.CodexIdentityPolicy.Mode != CodexIdentityPolicyOSProfileDevicePool {
		return false, nil
	}
	if !s.codexProfileAccountCompatible(ctx, account) {
		return true, codexProfileUnsupportedFromContext(ctx)
	}
	markCodexProfileAffinityActive(ctx)
	ttl := time.Duration(account.CodexIdentityPolicy.AffinityTTLSeconds) * time.Second
	bindingKey := codexProfileAffinityKey(request, sessionHash, account.CodexIdentityPolicy.Version, false)
	indexKey := codexProfileAffinityKey(request, sessionHash, 0, true)
	oldBindingKey := ""
	if oldAccountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), indexKey); err == nil && oldAccountID > 0 {
		if oldAccount := s.codexProfileAccountByID(ctx, oldAccountID); oldAccount != nil {
			oldBindingKey = codexProfileAffinityKey(request, sessionHash, oldAccount.CodexIdentityPolicy.Version, false)
		}
	}

	if atomicCache, ok := s.cache.(CodexProfileAffinityCache); ok {
		err := atomicCache.RebindCodexProfileAffinity(
			ctx,
			derefGroupID(groupID),
			oldBindingKey,
			bindingKey,
			indexKey,
			accountID,
			ttl,
		)
		if err != nil {
			// A transport error can make the Lua result ambiguous. Best-effort
			// removal makes either outcome fail closed once Redis is reachable.
			_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), indexKey)
			_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), bindingKey)
			if oldBindingKey != "" && oldBindingKey != bindingKey {
				_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), oldBindingKey)
			}
		}
		return true, err
	}

	// Test/degraded cache implementations may not provide the atomic extension.
	// Remove the published index first so any later partial write fails closed
	// instead of routing the next request back to the previous account.
	if err := s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), indexKey); err != nil {
		return true, err
	}
	if oldBindingKey != "" && oldBindingKey != bindingKey {
		if err := s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), oldBindingKey); err != nil {
			return true, err
		}
	}
	if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), bindingKey, accountID, ttl); err != nil {
		return true, err
	}
	if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), indexKey, accountID, ttl); err != nil {
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), bindingKey)
		return true, err
	}
	return true, nil
}

func (s *OpenAIGatewayService) deleteCodexProfileAffinity(ctx context.Context, groupID *int64, sessionHash string) {
	request, ok := codexProfileRequestFromContext(ctx)
	if !ok || s == nil || s.cache == nil || strings.TrimSpace(sessionHash) == "" {
		return
	}
	indexKey := codexProfileAffinityKey(request, sessionHash, 0, true)
	accountID, _ := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), indexKey)
	if account := s.codexProfileAccountByID(ctx, accountID); account != nil {
		bindingKey := codexProfileAffinityKey(request, sessionHash, account.CodexIdentityPolicy.Version, false)
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), bindingKey)
	}
	_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), indexKey)
}

func (s *OpenAIGatewayService) refreshCodexProfileAffinity(ctx context.Context, groupID *int64, sessionHash string, ttl time.Duration) {
	request, ok := codexProfileRequestFromContext(ctx)
	if !ok || s == nil || s.cache == nil || strings.TrimSpace(sessionHash) == "" {
		return
	}
	indexKey := codexProfileAffinityKey(request, sessionHash, 0, true)
	accountID, _ := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), indexKey)
	account := s.codexProfileAccountByID(ctx, accountID)
	if account == nil {
		return
	}
	if configured := time.Duration(account.CodexIdentityPolicy.AffinityTTLSeconds) * time.Second; configured > 0 {
		ttl = configured
	}
	_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), indexKey, ttl)
	bindingKey := codexProfileAffinityKey(request, sessionHash, account.CodexIdentityPolicy.Version, false)
	_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), bindingKey, ttl)
}
