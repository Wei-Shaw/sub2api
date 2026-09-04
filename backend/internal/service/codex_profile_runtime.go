package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

type CodexProfileConfidence string

const (
	CodexProfileConfidenceLow    CodexProfileConfidence = "low"
	CodexProfileConfidenceMedium CodexProfileConfidence = "medium"
	CodexProfileConfidenceHigh   CodexProfileConfidence = "high"
)

// CodexClientProfileSignals is an immutable request snapshot used before
// account selection. Evidence contains labels only; raw identity values are not
// retained in the classification result.
type CodexClientProfileSignals struct {
	Headers http.Header
	Body    []byte
}

type CodexClientProfile struct {
	OSClass      CodexOSClass           `json:"os_class"`
	Surface      CodexClientSurface     `json:"surface"`
	Architecture CodexArchitecture      `json:"architecture,omitempty"`
	Confidence   CodexProfileConfidence `json:"confidence"`
	Ambiguous    bool                   `json:"ambiguous,omitempty"`
	Evidence     []string               `json:"evidence,omitempty"`
}

func (p CodexClientProfile) Key() string {
	return codexRuntimeProfileKey(p.OSClass, p.Surface, p.Architecture)
}

type CodexResolvedProfile struct {
	OSClass        CodexOSClass
	Surface        CodexClientSurface
	Architecture   CodexArchitecture
	CatalogVersion int64
	Version        string
	AppBuild       string
	OSLabel        string
	Terminal       string
	ClientName     string
	UserAgent      string
	Originator     string
}

const codexRuntimeCatalogVersion int64 = 1

type codexRuntimeProfileFixture struct {
	appBuild   string
	osLabel    string
	terminal   string
	clientName string
	originator string
}

func (p CodexResolvedProfile) Key() string {
	return codexRuntimeProfileKey(p.OSClass, p.Surface, p.Architecture)
}

func (p CodexResolvedProfile) Supports(client CodexClientProfile) bool {
	// OS and surface are hard compatibility boundaries. Architecture remains
	// an adapter input that converges to this profile's canonical architecture.
	return p.OSClass == client.OSClass && p.Surface == client.Surface
}

type CodexResolvedSlot struct {
	Index   int
	Epoch   int64
	ProxyID *int64
}

// ResolveCodexRuntimeProfile resolves an administrator policy through a closed
// catalog. The policy cannot inject arbitrary versions, user agents, or
// originators.
func ResolveCodexRuntimeProfile(policy CodexOSProfilePolicy) (CodexResolvedProfile, error) {
	return ResolveCodexRuntimeProfileWithVersion(policy, codexCLIVersion)
}

// ResolveCodexRuntimeProfileWithVersion resolves a closed profile using one
// validated engine version. Only the version segment is variable; client name,
// originator, OS/terminal shape, and Desktop app build remain catalog-owned.
func ResolveCodexRuntimeProfileWithVersion(policy CodexOSProfilePolicy, clientVersion string) (CodexResolvedProfile, error) {
	normalized, err := normalizeCodexOSProfile(policy)
	if err != nil {
		return CodexResolvedProfile{}, err
	}
	clientVersion = NormalizeCodexClientVersion(clientVersion)
	if clientVersion == "" {
		return CodexResolvedProfile{}, errors.New("invalid Codex client version")
	}
	if CompareVersions(clientVersion, codexUpstreamMinVersion) < 0 {
		return CodexResolvedProfile{}, fmt.Errorf(
			"codex client version must be at least %s",
			codexUpstreamMinVersion,
		)
	}
	if normalized.CatalogVersion != codexRuntimeCatalogVersion {
		return CodexResolvedProfile{}, fmt.Errorf("unsupported Codex profile catalog version %d", normalized.CatalogVersion)
	}
	fixture, err := codexRuntimeCatalogFixture(normalized.OSClass, normalized.CanonicalSurface)
	if err != nil {
		return CodexResolvedProfile{}, err
	}
	profile := CodexResolvedProfile{
		OSClass:        normalized.OSClass,
		Surface:        normalized.CanonicalSurface,
		Architecture:   normalized.Architecture,
		CatalogVersion: normalized.CatalogVersion,
		Version:        clientVersion,
		AppBuild:       fixture.appBuild,
		OSLabel:        fixture.osLabel,
		Terminal:       fixture.terminal,
		ClientName:     fixture.clientName,
		Originator:     fixture.originator,
	}

	if profile.OSClass == CodexOSGeneric {
		profile.UserAgent = fixture.clientName + "/" + clientVersion
	} else {
		osLabel := fixture.osLabel + "; " + string(profile.Architecture)
		profile.UserAgent = fmt.Sprintf("%s/%s (%s) %s", fixture.clientName, clientVersion, osLabel, fixture.terminal)
		if fixture.appBuild != "" {
			profile.UserAgent += fmt.Sprintf(" (%s; %s)", fixture.originator, fixture.appBuild)
		}
	}
	pairedOriginator, pairedUA, paired := openai.PairCodexClientIdentity(profile.UserAgent)
	if !paired || pairedOriginator != profile.Originator || pairedUA != profile.UserAgent || openai.CodexUserAgentVersion(profile.UserAgent) != profile.Version {
		return CodexResolvedProfile{}, fmt.Errorf("invalid closed Codex profile fixture %s", profile.Key())
	}
	return profile, nil
}

func codexRuntimeCatalogFixture(osClass CodexOSClass, surface CodexClientSurface) (codexRuntimeProfileFixture, error) {
	if osClass == CodexOSGeneric {
		switch surface {
		case CodexSurfaceSDK:
			return codexRuntimeProfileFixture{clientName: "codex_sdk_ts", originator: "codex_sdk_ts"}, nil
		case CodexSurfaceThirdParty:
			return codexRuntimeProfileFixture{clientName: "codex_exec", originator: "codex_exec"}, nil
		default:
			return codexRuntimeProfileFixture{}, errors.New("generic Codex profile requires sdk or third_party surface")
		}
	}
	if surface != CodexSurfaceDesktop && surface != CodexSurfaceCLI {
		return codexRuntimeProfileFixture{}, fmt.Errorf("%s Codex profile requires desktop or cli surface", osClass)
	}

	fixture := codexRuntimeProfileFixture{}
	if surface == CodexSurfaceDesktop {
		// Engine protocol and desktop application build are distinct fields.
		// The app build is pinned from the repository's captured first-party UA.
		fixture.appBuild = "26.616.71553"
		fixture.clientName = "Codex Desktop"
		fixture.originator = "Codex Desktop"
		fixture.terminal = "unknown"
	} else {
		fixture.clientName = "codex_cli_rs"
		fixture.originator = "codex_cli_rs"
	}
	switch osClass {
	case CodexOSWindows:
		fixture.osLabel = "Windows 11"
		if surface == CodexSurfaceCLI {
			fixture.terminal = "WindowsTerminal"
		}
	case CodexOSMacOS:
		fixture.osLabel = "Mac OS X 26.0.1"
		if surface == CodexSurfaceCLI {
			fixture.terminal = "Terminal.app"
		}
	case CodexOSLinux:
		fixture.osLabel = "Ubuntu 22.04"
		if surface == CodexSurfaceCLI {
			fixture.terminal = "xterm-256color"
		}
	default:
		return codexRuntimeProfileFixture{}, fmt.Errorf("unsupported Codex OS class %q", osClass)
	}
	return fixture, nil
}

func codexRuntimeProfileKey(osClass CodexOSClass, surface CodexClientSurface, arch CodexArchitecture) string {
	parts := []string{string(osClass), string(surface)}
	if arch != "" {
		parts = append(parts, string(arch))
	}
	return strings.Join(parts, "/")
}

// ClassifyCodexClientProfile classifies only into the closed policy catalog.
// Unknown or script-like callers intentionally fall back to Generic instead of
// being guessed into an OS-specific adapter.
func ClassifyCodexClientProfile(signals CodexClientProfileSignals) CodexClientProfile {
	ua := strings.ToLower(strings.TrimSpace(signals.Headers.Get("user-agent")))
	originator := strings.ToLower(strings.TrimSpace(signals.Headers.Get("originator")))
	bodySignals := codexProfileBodySignals(signals.Body)
	headerSignals := codexProfileTurnMetadataSignals(signals.Headers.Get("x-codex-turn-metadata"))
	allSignals := append([]string{ua, originator}, bodySignals...)
	allSignals = append(allSignals, headerSignals...)
	all := strings.Join(allSignals, " ")
	explicitOS, osConflict := explicitCodexOS(signals)
	uaOS := classifyCodexOS(ua)
	explicitArch, archConflict := explicitCodexArchitecture(signals)
	uaArch := classifyCodexArchitecture(ua)
	explicitSurface, surfaceConflict := explicitCodexSurface(signals)
	uaSurface := classifyCodexSurface(ua, originator, strings.TrimSpace(ua+" "+originator))
	if osConflict || archConflict || surfaceConflict || (explicitOS != "" && uaOS != "" && explicitOS != uaOS) ||
		(explicitArch != "" && uaArch != "" && explicitArch != uaArch) ||
		(explicitSurface != "" && uaSurface != "" && explicitSurface != uaSurface) {
		return CodexClientProfile{
			Surface: classifyCodexSurface(ua, originator, all), Confidence: CodexProfileConfidenceLow,
			Ambiguous: true, Evidence: []string{"conflicting_strong_profile_signals"},
		}
	}

	profile := CodexClientProfile{Confidence: CodexProfileConfidenceLow}
	if explicitSurface == CodexSurfaceSDK || explicitSurface == CodexSurfaceThirdParty {
		return CodexClientProfile{
			OSClass: CodexOSGeneric, Surface: explicitSurface, Confidence: CodexProfileConfidenceHigh,
			Evidence: []string{"explicit_generic_surface"},
		}
	}
	strongSDK := hasAnyCodexProfileToken(all,
		"openai-python", "openai-python/", "python-requests", "python/",
		"openai-node", "node-fetch", "node.js", "curl/", "postman",
		"codex_sdk_ts/",
	)
	strongThirdParty := hasAnyCodexProfileToken(all, "mozilla/", "opencode", "cursor/", "codex_vscode/", "codex_vscode_copilot/")
	if strongSDK || strongThirdParty {
		profile.OSClass = CodexOSGeneric
		if strongSDK {
			profile.Surface = CodexSurfaceSDK
			profile.Evidence = append(profile.Evidence, "sdk_client_signal")
		} else {
			profile.Surface = CodexSurfaceThirdParty
			profile.Evidence = append(profile.Evidence, "third_party_client_signal")
		}
		profile.Confidence = CodexProfileConfidenceHigh
		return profile
	}

	profile.OSClass = explicitOS
	if profile.OSClass == "" {
		profile.OSClass = classifyCodexOS(all)
	}
	profile.Architecture = explicitArch
	if profile.Architecture == "" {
		profile.Architecture = classifyCodexArchitecture(all)
	}
	profile.Surface = explicitSurface
	if profile.Surface == "" {
		profile.Surface = classifyCodexSurface(ua, originator, all)
	}

	if profile.OSClass == "" || profile.Surface == "" {
		return CodexClientProfile{
			OSClass:    CodexOSGeneric,
			Surface:    CodexSurfaceThirdParty,
			Confidence: CodexProfileConfidenceLow,
			Evidence:   []string{"insufficient_profile_evidence"},
		}
	}

	profile.Evidence = append(profile.Evidence, "os_signal", "surface_signal")
	if profile.Architecture != "" {
		profile.Evidence = append(profile.Evidence, "architecture_signal")
		profile.Confidence = CodexProfileConfidenceHigh
	} else {
		profile.Confidence = CodexProfileConfidenceMedium
	}
	return profile
}

func explicitCodexOS(signals CodexClientProfileSignals) (CodexOSClass, bool) {
	values := codexExplicitProfileValues(signals, "os", "platform")
	seen := make(map[CodexOSClass]struct{})
	for _, value := range values {
		if osClass := classifyCodexOS(value); osClass != "" {
			seen[osClass] = struct{}{}
		}
	}
	if len(seen) > 1 {
		return "", true
	}
	for osClass := range seen {
		return osClass, false
	}
	return "", false
}

func explicitCodexArchitecture(signals CodexClientProfileSignals) (CodexArchitecture, bool) {
	values := codexExplicitProfileValues(signals, "arch", "architecture")
	seen := make(map[CodexArchitecture]struct{})
	for _, value := range values {
		if architecture := classifyCodexArchitecture(value); architecture != "" {
			seen[architecture] = struct{}{}
		}
	}
	if len(seen) > 1 {
		return "", true
	}
	for architecture := range seen {
		return architecture, false
	}
	return "", false
}

func explicitCodexSurface(signals CodexClientProfileSignals) (CodexClientSurface, bool) {
	values := codexExplicitProfileValues(signals, "surface", "client_surface")
	seen := make(map[CodexClientSurface]struct{})
	for _, value := range values {
		value = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
		var surface CodexClientSurface
		switch value {
		case "desktop":
			surface = CodexSurfaceDesktop
		case "cli", "command_line":
			surface = CodexSurfaceCLI
		case "sdk":
			surface = CodexSurfaceSDK
		case "third_party", "thirdparty":
			surface = CodexSurfaceThirdParty
		}
		if surface != "" {
			seen[surface] = struct{}{}
		}
	}
	if len(seen) > 1 {
		return "", true
	}
	for surface := range seen {
		return surface, false
	}
	return "", false
}

func codexExplicitProfileValues(signals CodexClientProfileSignals, fields ...string) []string {
	values := make([]string, 0, len(fields)*5)
	if len(signals.Body) > 0 && gjson.ValidBytes(signals.Body) {
		for _, field := range fields {
			for _, prefix := range []string{"client_metadata.", "metadata.", "turn_metadata.", "request.client_metadata."} {
				if value := strings.ToLower(strings.TrimSpace(gjson.GetBytes(signals.Body, prefix+field).String())); value != "" {
					values = append(values, value)
				}
			}
		}
		for _, raw := range []string{
			gjson.GetBytes(signals.Body, "client_metadata.x-codex-turn-metadata").String(),
			gjson.GetBytes(signals.Body, "client_metadata.turn_metadata").String(),
		} {
			for _, field := range fields {
				if value := strings.ToLower(strings.TrimSpace(gjson.Get(raw, field).String())); value != "" {
					values = append(values, value)
				}
			}
		}
	}
	headerMetadata := signals.Headers.Get("x-codex-turn-metadata")
	if gjson.Valid(headerMetadata) {
		for _, field := range fields {
			if value := strings.ToLower(strings.TrimSpace(gjson.Get(headerMetadata, field).String())); value != "" {
				values = append(values, value)
			}
		}
	}
	return values
}

func codexProfileBodySignals(body []byte) []string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil
	}
	paths := [...]string{
		"client_metadata.os",
		"client_metadata.platform",
		"client_metadata.arch",
		"client_metadata.architecture",
		"client_metadata.client",
		"client_metadata.client_name",
		"client_metadata.originator",
		"client_metadata.cwd",
		"client_metadata.workspace",
		"client_metadata.workspace_path",
	}
	values := make([]string, 0, len(paths)+4)
	for _, path := range paths {
		if value := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, path).String())); value != "" {
			values = append(values, value)
		}
	}
	for _, raw := range []string{
		gjson.GetBytes(body, "client_metadata.x-codex-turn-metadata").String(),
		gjson.GetBytes(body, "client_metadata.turn_metadata").String(),
	} {
		values = append(values, codexProfileTurnMetadataSignals(raw)...)
	}
	return values
}

func codexProfileTurnMetadataSignals(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var object map[string]any
	if json.Unmarshal([]byte(raw), &object) != nil || object == nil {
		return nil
	}
	values := make([]string, 0, 8)
	for _, path := range []string{"os", "platform", "arch", "architecture", "cwd", "workspace", "workspace_path", "client", "client_name", "originator"} {
		if value := strings.ToLower(strings.TrimSpace(gjson.Get(raw, path).String())); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func classifyCodexOS(value string) CodexOSClass {
	switch {
	case hasAnyCodexProfileToken(value, "windows", "win32", "win64", `c:\`):
		return CodexOSWindows
	case hasAnyCodexProfileToken(value, "mac os x", "macos", "macintosh", "darwin"):
		return CodexOSMacOS
	case hasAnyCodexProfileToken(value, "ubuntu", "debian", "fedora", "linux", "/home/", "/workspace/"):
		return CodexOSLinux
	default:
		return ""
	}
}

func classifyCodexArchitecture(value string) CodexArchitecture {
	switch {
	case hasAnyCodexProfileToken(value, "arm64", "aarch64"):
		return CodexArchARM64
	case hasAnyCodexProfileToken(value, "x86_64", "amd64", "win64", "x64"):
		return CodexArchX8664
	default:
		return ""
	}
}

func classifyCodexSurface(ua, originator, aggregate string) CodexClientSurface {
	switch {
	case hasAnyCodexProfileToken(ua, "codex desktop/", "codex_chatgpt_desktop/", "codex_app/"),
		hasAnyCodexProfileToken(originator, "codex desktop", "codex_chatgpt_desktop", "codex_app"):
		return CodexSurfaceDesktop
	case hasAnyCodexProfileToken(ua, "codex_cli_rs/", "codex-tui/", "codex_exec/"),
		hasAnyCodexProfileToken(originator, "codex_cli_rs", "codex-tui", "codex_exec"):
		return CodexSurfaceCLI
	case strings.Contains(aggregate, "desktop"):
		return CodexSurfaceDesktop
	case hasAnyCodexProfileToken(aggregate, "terminal", "xterm", "iterm", "shell"):
		return CodexSurfaceCLI
	default:
		return ""
	}
}

func hasAnyCodexProfileToken(value string, tokens ...string) bool {
	for _, token := range tokens {
		if token != "" && strings.Contains(value, token) {
			return true
		}
	}
	return false
}

type CodexIdentityKind string

const (
	CodexIdentityInstallation CodexIdentityKind = "installation_id"
	CodexIdentitySession      CodexIdentityKind = "session_id"
	CodexIdentityConversation CodexIdentityKind = "conversation_id"
	CodexIdentityThread       CodexIdentityKind = "thread_id"
	CodexIdentityTurn         CodexIdentityKind = "turn_id"
	CodexIdentityWindow       CodexIdentityKind = "window_id"
	CodexIdentityPromptCache  CodexIdentityKind = "prompt_cache_key"
	CodexIdentityWorkspace    CodexIdentityKind = "workspace"
	CodexIdentityGitRemote    CodexIdentityKind = "git_remote"
	CodexIdentityGitCommit    CodexIdentityKind = "git_commit"
)

type CodexIdentitySource struct {
	InstallationID string
	SessionID      string
	ConversationID string
	ThreadID       string
	TurnID         string
	WindowID       string
	PromptCacheKey string
	Workspace      string
	GitRemote      string
	GitCommit      string
	ProfileFields  map[string]CodexProfileFieldSource
}

type CodexProfileFieldSource struct {
	Value   string
	Present bool
}

type CodexProfileFieldMapping struct {
	Field         string
	ClientValue   string
	ClientPresent bool
	UpstreamValue string
}

type CodexIdentityMapping struct {
	Kind          CodexIdentityKind
	ClientValue   string
	UpstreamValue string
}

type CodexSessionRuntimeConstraints struct {
	MaxActiveConversationsPerSlot int
	DisableCrossKeyContinuation   bool
}

type CodexIdentityAttemptInput struct {
	Mode            CodexIdentityPolicyMode
	AccountID       int64
	APIKeyScope     string
	AccountSeed     string
	Profile         CodexResolvedProfile
	Slot            CodexResolvedSlot
	SessionPolicy   CodexSessionPolicySpec
	SessionRuntime  CodexSessionRuntimeConstraints
	ConversationKey string
	RequestNonce    string
	Source          CodexIdentitySource
}

type CodexIdentityAttemptPlan struct {
	AccountID         int64
	APIKeyScope       string
	Profile           CodexResolvedProfile
	Slot              CodexResolvedSlot
	SessionPolicy     CodexSessionPolicySpec
	SessionSlotIndex  int
	RequestMappings   []CodexIdentityMapping
	ProfileMappings   []CodexProfileFieldMapping
	conversationLease *CodexDeviceConversationLease
}

func (p *CodexIdentityAttemptPlan) UpstreamValue(kind CodexIdentityKind) string {
	if p == nil {
		return ""
	}
	for _, mapping := range p.RequestMappings {
		if mapping.Kind == kind {
			return mapping.UpstreamValue
		}
	}
	return ""
}

// BuildCodexIdentityAttemptPlan is hard-gated by the additive policy mode. An
// empty or off mode returns nil before validating any new-mode input, preserving
// all legacy request behavior.
func BuildCodexIdentityAttemptPlan(input CodexIdentityAttemptInput) (*CodexIdentityAttemptPlan, error) {
	if input.Mode == "" || input.Mode == CodexIdentityPolicyOff {
		return nil, nil
	}
	if input.Mode != CodexIdentityPolicyOSProfileDevicePool {
		return nil, fmt.Errorf("unsupported Codex identity attempt mode %q", input.Mode)
	}
	if input.AccountID <= 0 {
		return nil, errors.New("codex identity attempt requires a positive account ID")
	}
	input.APIKeyScope = strings.TrimSpace(input.APIKeyScope)
	if input.APIKeyScope == "" {
		return nil, errors.New("codex identity attempt requires an API key scope")
	}
	seed, ok := canonicalCodexFingerprintSeed(input.AccountSeed)
	if !ok {
		return nil, errors.New("codex identity attempt requires a canonical account seed")
	}
	if input.Profile.OSClass == "" || input.Profile.Surface == "" || input.Profile.UserAgent == "" || input.Profile.Originator == "" {
		return nil, errors.New("codex identity attempt requires a resolved profile")
	}
	if input.Slot.Index < 0 || input.Slot.Epoch < 1 {
		return nil, errors.New("codex identity attempt requires a valid resolved slot")
	}
	if err := validateCodexSessionPolicy(input.SessionPolicy); err != nil {
		return nil, err
	}
	if input.SessionPolicy.Mode == CodexSessionDeviceShared &&
		(input.SessionRuntime.MaxActiveConversationsPerSlot != 1 || !input.SessionRuntime.DisableCrossKeyContinuation) {
		return nil, errors.New("device_shared requires one active conversation per slot and disabled cross-key continuation")
	}

	conversation := firstNonEmptyCodexIdentityValue(
		input.ConversationKey,
		input.Source.ConversationID,
		input.Source.SessionID,
		input.Source.PromptCacheKey,
		input.Source.ThreadID,
		input.RequestNonce,
	)
	if conversation == "" {
		return nil, errors.New("codex identity attempt requires a conversation key or request nonce")
	}
	requestNonce := firstNonEmptyCodexIdentityValue(input.RequestNonce, input.Source.TurnID, conversation)
	profileKey := input.Profile.Key()
	slotParts := []string{profileKey, strconv.Itoa(input.Slot.Index), strconv.FormatInt(input.Slot.Epoch, 10)}

	plan := &CodexIdentityAttemptPlan{
		AccountID:        input.AccountID,
		APIKeyScope:      input.APIKeyScope,
		Profile:          input.Profile,
		Slot:             input.Slot,
		SessionPolicy:    input.SessionPolicy,
		SessionSlotIndex: -1,
	}
	deviceID := deriveCodexIdentityUUID(seed, "device-slot:v1", slotParts...)
	plan.addMapping(CodexIdentityInstallation, input.Source.InstallationID, deviceID)

	sessionID := ""
	switch input.SessionPolicy.Mode {
	case CodexSessionConversationIsolated:
		sessionID = deriveCodexIdentityUUID(seed, "session-conversation:v1", append(slotParts, input.APIKeyScope, conversation)...)
	case CodexSessionAPIKeyShared:
		sessionID = deriveCodexIdentityUUID(seed, "session-api-key:v1", append(slotParts, input.APIKeyScope)...)
	case CodexSessionPool:
		choice := deriveCodexIdentityDigest(seed, "session-pool-choice:v1", input.APIKeyScope, conversation)
		plan.SessionSlotIndex = int(binary.BigEndian.Uint64(choice[:8]) % uint64(input.SessionPolicy.SessionsPerDevice))
		sessionID = deriveCodexIdentityUUID(seed, "session-pool-slot:v1", append(slotParts, strconv.Itoa(plan.SessionSlotIndex))...)
	case CodexSessionDeviceShared:
		sessionID = deriveCodexIdentityUUID(seed, "session-device:v1", slotParts...)
	default:
		return nil, fmt.Errorf("unsupported Codex session policy %q", input.SessionPolicy.Mode)
	}
	plan.addMapping(CodexIdentitySession, input.Source.SessionID, sessionID)
	plan.addMapping(CodexIdentityConversation, input.Source.ConversationID, sessionID)

	threadID := deriveCodexIdentityUUID(seed, "thread-conversation:v1", append(slotParts, input.APIKeyScope, conversation, input.Source.ThreadID)...)
	turnID := deriveCodexIdentityUUID(seed, "turn-attempt:v1", append(slotParts, input.APIKeyScope, conversation, requestNonce, input.Source.TurnID)...)
	windowID := deriveCodexIdentityUUID(seed, "window-conversation:v1", append(slotParts, input.APIKeyScope, conversation, input.Source.WindowID)...)
	plan.addMapping(CodexIdentityThread, input.Source.ThreadID, threadID)
	plan.addMapping(CodexIdentityTurn, input.Source.TurnID, turnID)
	plan.addMapping(CodexIdentityWindow, input.Source.WindowID, windowID+":0")

	promptCacheID := sessionID
	if input.SessionPolicy.Mode == CodexSessionConversationIsolated {
		promptCacheID = deriveCodexIdentityUUID(seed, "prompt-cache-conversation:v1", append(slotParts, input.APIKeyScope, conversation)...)
	}
	plan.addMapping(CodexIdentityPromptCache, input.Source.PromptCacheKey, promptCacheID)

	if input.Source.Workspace != "" {
		plan.addMapping(CodexIdentityWorkspace, input.Source.Workspace, codexWorkspaceAlias(seed, input.Profile.OSClass, slotParts, input.Source.Workspace))
	}
	if input.Source.GitRemote != "" {
		digest := deriveCodexIdentityDigest(seed, "git-remote:v1", append(slotParts, input.Source.GitRemote)...)
		alias := hex.EncodeToString(digest[:10])
		plan.addMapping(CodexIdentityGitRemote, input.Source.GitRemote, "https://github.com/codex-workspace/"+alias+".git")
	}
	if input.Source.GitCommit != "" {
		digest := deriveCodexIdentityDigest(seed, "git-commit:v1", append(slotParts, input.Source.GitCommit)...)
		alias := hex.EncodeToString(digest[:20])
		plan.addMapping(CodexIdentityGitCommit, input.Source.GitCommit, alias)
	}
	for field, upstreamValue := range codexResolvedProfileMetadataValues(input.Profile) {
		source := input.Source.ProfileFields[field]
		plan.ProfileMappings = append(plan.ProfileMappings, CodexProfileFieldMapping{
			Field: field, ClientValue: source.Value, ClientPresent: source.Present, UpstreamValue: upstreamValue,
		})
	}
	return plan, nil
}

func codexResolvedProfileMetadataValues(profile CodexResolvedProfile) map[string]string {
	values := map[string]string{
		"os": string(profile.OSClass), "platform": string(profile.OSClass),
		"surface": string(profile.Surface), "client": profile.ClientName,
		"client_name": profile.ClientName, "originator": profile.Originator,
		"version": profile.Version, "user_agent": profile.UserAgent,
		"terminal": profile.Terminal, "os_label": profile.OSLabel, "app_build": profile.AppBuild,
		"arch": "", "architecture": "",
	}
	if profile.Architecture != "" {
		values["arch"] = string(profile.Architecture)
		values["architecture"] = string(profile.Architecture)
	}
	return values
}

func (p *CodexIdentityAttemptPlan) addMapping(kind CodexIdentityKind, clientValue, upstreamValue string) {
	if p == nil || strings.TrimSpace(upstreamValue) == "" {
		return
	}
	p.RequestMappings = append(p.RequestMappings, CodexIdentityMapping{
		Kind:          kind,
		ClientValue:   strings.TrimSpace(clientValue),
		UpstreamValue: strings.TrimSpace(upstreamValue),
	})
}

func deriveCodexIdentityUUID(seed, domain string, parts ...string) string {
	digest := deriveCodexIdentityDigest(seed, domain, parts...)
	var id uuid.UUID
	copy(id[:], digest[:16])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
}

func deriveCodexIdentityDigest(seed, domain string, parts ...string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, []byte(seed))
	writeCodexIdentityHMACPart(mac, domain)
	for _, part := range parts {
		writeCodexIdentityHMACPart(mac, part)
	}
	var digest [sha256.Size]byte
	copy(digest[:], mac.Sum(nil))
	return digest
}

type codexIdentityHMACWriter interface {
	Write([]byte) (int, error)
}

func writeCodexIdentityHMACPart(dst codexIdentityHMACWriter, value string) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	_, _ = dst.Write(length[:])
	_, _ = dst.Write([]byte(value))
}

func codexWorkspaceAlias(seed string, osClass CodexOSClass, slotParts []string, original string) string {
	digest := deriveCodexIdentityDigest(seed, "workspace:v1", append(slotParts, original)...)
	suffix := hex.EncodeToString(digest[:8])
	switch osClass {
	case CodexOSWindows:
		return `C:\Users\codex\workspace\` + suffix
	case CodexOSMacOS:
		return "/Users/codex/workspace/" + suffix
	case CodexOSLinux:
		return "/home/codex/workspace/" + suffix
	default:
		return "/workspace/" + suffix
	}
}

func firstNonEmptyCodexIdentityValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// ExtractCodexIdentitySource reads only known identity fields. It never scans
// prompt or output text.
func ExtractCodexIdentitySource(headers http.Header, body []byte) CodexIdentitySource {
	source := CodexIdentitySource{}
	if headers != nil {
		source.InstallationID = strings.TrimSpace(headers.Get("x-codex-installation-id"))
		source.SessionID = firstNonEmptyCodexIdentityValue(headers.Get("session-id"), headers.Get("session_id"))
		source.ConversationID = strings.TrimSpace(headers.Get("conversation_id"))
		source.ThreadID = firstNonEmptyCodexIdentityValue(headers.Get("thread-id"), headers.Get("thread_id"), headers.Get("x-client-request-id"))
		source.WindowID = strings.TrimSpace(headers.Get("x-codex-window-id"))
		mergeCodexIdentityTurnMetadata(&source, headers.Get("x-codex-turn-metadata"))
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return source
	}
	source.InstallationID = firstNonEmptyCodexIdentityValue(source.InstallationID,
		gjson.GetBytes(body, "client_metadata.x-codex-installation-id").String(),
		gjson.GetBytes(body, "client_metadata.installation_id").String(),
		codexBodyIdentityString(body, "metadata.installation_id", "turn_metadata.installation_id", "request.client_metadata.installation_id"))
	source.SessionID = firstNonEmptyCodexIdentityValue(source.SessionID,
		codexBodyIdentityString(body, "client_metadata.session_id", "metadata.session_id", "turn_metadata.session_id", "request.client_metadata.session_id"))
	source.ConversationID = firstNonEmptyCodexIdentityValue(source.ConversationID,
		codexBodyIdentityString(body, "client_metadata.conversation_id", "metadata.conversation_id", "turn_metadata.conversation_id", "request.client_metadata.conversation_id"))
	source.ThreadID = firstNonEmptyCodexIdentityValue(source.ThreadID,
		codexBodyIdentityString(body, "client_metadata.thread_id", "metadata.thread_id", "turn_metadata.thread_id", "request.client_metadata.thread_id"))
	source.TurnID = firstNonEmptyCodexIdentityValue(source.TurnID,
		codexBodyIdentityString(body, "client_metadata.turn_id", "metadata.turn_id", "turn_metadata.turn_id", "request.client_metadata.turn_id"))
	source.WindowID = firstNonEmptyCodexIdentityValue(source.WindowID,
		gjson.GetBytes(body, "client_metadata.x-codex-window-id").String(),
		codexBodyIdentityString(body, "client_metadata.window_id", "metadata.window_id", "turn_metadata.window_id", "request.client_metadata.window_id"))
	source.PromptCacheKey = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	source.Workspace = firstNonEmptyCodexIdentityValue(
		codexBodyIdentityString(body,
			"client_metadata.workspace_path", "client_metadata.workspace", "client_metadata.cwd",
			"metadata.workspace_path", "metadata.workspace", "metadata.cwd",
			"turn_metadata.workspace_path", "turn_metadata.workspace", "turn_metadata.cwd",
			"request.client_metadata.workspace_path", "request.client_metadata.workspace", "request.client_metadata.cwd"),
	)
	source.GitRemote = firstNonEmptyCodexIdentityValue(
		codexBodyIdentityString(body,
			"client_metadata.git_remote_url", "client_metadata.git_remote",
			"metadata.git_remote_url", "metadata.git_remote",
			"turn_metadata.git_remote_url", "turn_metadata.git_remote",
			"request.client_metadata.git_remote_url", "request.client_metadata.git_remote"),
	)
	source.GitCommit = firstNonEmptyCodexIdentityValue(
		codexBodyIdentityString(body,
			"client_metadata.git_commit", "client_metadata.git_sha",
			"metadata.git_commit", "metadata.git_sha",
			"turn_metadata.git_commit", "turn_metadata.git_sha",
			"request.client_metadata.git_commit", "request.client_metadata.git_sha"),
	)
	if source.ProfileFields == nil {
		source.ProfileFields = make(map[string]CodexProfileFieldSource)
	}
	for _, field := range []string{
		"os", "platform", "arch", "architecture", "surface", "client", "client_name",
		"originator", "version", "user_agent", "terminal", "os_label", "app_build",
	} {
		for _, prefix := range []string{"client_metadata.", "metadata.", "turn_metadata.", "request.client_metadata."} {
			value := gjson.GetBytes(body, prefix+field)
			if value.Exists() {
				source.ProfileFields[field] = CodexProfileFieldSource{Value: value.String(), Present: true}
				break
			}
		}
	}
	mergeCodexIdentityTurnMetadata(&source, gjson.GetBytes(body, "client_metadata.x-codex-turn-metadata").String())
	return source
}

func codexBodyIdentityString(body []byte, paths ...string) string {
	for _, path := range paths {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func mergeCodexIdentityTurnMetadata(source *CodexIdentitySource, raw string) {
	if source == nil || strings.TrimSpace(raw) == "" || !json.Valid([]byte(raw)) {
		return
	}
	source.InstallationID = firstNonEmptyCodexIdentityValue(source.InstallationID, gjson.Get(raw, "installation_id").String())
	source.SessionID = firstNonEmptyCodexIdentityValue(source.SessionID, gjson.Get(raw, "session_id").String())
	source.ConversationID = firstNonEmptyCodexIdentityValue(source.ConversationID, gjson.Get(raw, "conversation_id").String())
	source.ThreadID = firstNonEmptyCodexIdentityValue(source.ThreadID, gjson.Get(raw, "thread_id").String())
	source.TurnID = firstNonEmptyCodexIdentityValue(source.TurnID, gjson.Get(raw, "turn_id").String())
	source.WindowID = firstNonEmptyCodexIdentityValue(source.WindowID, gjson.Get(raw, "window_id").String())
	source.Workspace = firstNonEmptyCodexIdentityValue(source.Workspace,
		gjson.Get(raw, "workspace_path").String(), gjson.Get(raw, "workspace").String(), gjson.Get(raw, "cwd").String())
	source.GitRemote = firstNonEmptyCodexIdentityValue(source.GitRemote,
		gjson.Get(raw, "git_remote_url").String(), gjson.Get(raw, "git_remote").String())
	source.GitCommit = firstNonEmptyCodexIdentityValue(source.GitCommit,
		gjson.Get(raw, "git_commit").String(), gjson.Get(raw, "git_sha").String())
	if source.ProfileFields == nil {
		source.ProfileFields = make(map[string]CodexProfileFieldSource)
	}
	for _, field := range []string{
		"os", "platform", "arch", "architecture", "surface", "client", "client_name",
		"originator", "version", "user_agent", "terminal", "os_label", "app_build",
	} {
		if _, exists := source.ProfileFields[field]; exists {
			continue
		}
		value := gjson.Get(raw, field)
		if value.Exists() {
			source.ProfileFields[field] = CodexProfileFieldSource{Value: value.String(), Present: true}
		}
	}
}
