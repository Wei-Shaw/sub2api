package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const codexRuntimeTestSeed = "11111111-1111-4111-8111-111111111111"

func TestClassifyCodexClientProfileClosedCatalog(t *testing.T) {
	tests := []struct {
		name        string
		headers     http.Header
		body        string
		wantOS      CodexOSClass
		wantSurface CodexClientSurface
		wantArch    CodexArchitecture
		wantLevel   CodexProfileConfidence
	}{
		{
			name:        "windows cli",
			headers:     http.Header{"User-Agent": {"codex_cli_rs/0.146.0 (Windows 11; x86_64) WindowsTerminal"}},
			wantOS:      CodexOSWindows,
			wantSurface: CodexSurfaceCLI,
			wantArch:    CodexArchX8664,
			wantLevel:   CodexProfileConfidenceHigh,
		},
		{
			name:        "windows desktop",
			headers:     http.Header{"User-Agent": {"Codex Desktop/0.146.0 (Windows 11; arm64) CodexDesktop"}},
			wantOS:      CodexOSWindows,
			wantSurface: CodexSurfaceDesktop,
			wantArch:    CodexArchARM64,
			wantLevel:   CodexProfileConfidenceHigh,
		},
		{
			name:        "windows desktop from body metadata enum",
			headers:     http.Header{},
			body:        `{"client_metadata":{"os":"windows","arch":"x86_64","surface":"desktop"}}`,
			wantOS:      CodexOSWindows,
			wantSurface: CodexSurfaceDesktop,
			wantArch:    CodexArchX8664,
			wantLevel:   CodexProfileConfidenceHigh,
		},
		{
			name:        "mac cli",
			headers:     http.Header{"User-Agent": {"codex-tui/0.146.0 (Mac OS X 14.0; arm64) iTerm"}},
			wantOS:      CodexOSMacOS,
			wantSurface: CodexSurfaceCLI,
			wantArch:    CodexArchARM64,
			wantLevel:   CodexProfileConfidenceHigh,
		},
		{
			name:        "mac desktop",
			headers:     http.Header{"User-Agent": {"Codex Desktop/0.146.0 (Mac OS X 14.0; arm64) CodexDesktop"}},
			wantOS:      CodexOSMacOS,
			wantSurface: CodexSurfaceDesktop,
			wantArch:    CodexArchARM64,
			wantLevel:   CodexProfileConfidenceHigh,
		},
		{
			name:        "linux cli from body metadata",
			headers:     http.Header{},
			body:        `{"client_metadata":{"platform":"linux","arch":"x86_64","client":"terminal"}}`,
			wantOS:      CodexOSLinux,
			wantSurface: CodexSurfaceCLI,
			wantArch:    CodexArchX8664,
			wantLevel:   CodexProfileConfidenceHigh,
		},
		{
			name:        "explicit sdk surface remains generic on windows host",
			headers:     http.Header{},
			body:        `{"client_metadata":{"os":"windows","arch":"x86_64","surface":"sdk"}}`,
			wantOS:      CodexOSGeneric,
			wantSurface: CodexSurfaceSDK,
			wantLevel:   CodexProfileConfidenceHigh,
		},
		{
			name:        "linux desktop",
			headers:     http.Header{"User-Agent": {"Codex Desktop/0.146.0 (Ubuntu 22.04; arm64) CodexDesktop"}},
			wantOS:      CodexOSLinux,
			wantSurface: CodexSurfaceDesktop,
			wantArch:    CodexArchARM64,
			wantLevel:   CodexProfileConfidenceHigh,
		},
		{
			name: "header turn metadata supplies OS",
			headers: http.Header{
				"User-Agent":            {"Codex Desktop/0.146.0"},
				"X-Codex-Turn-Metadata": {`{"os":"linux","arch":"arm64","workspace":"/home/alice/project"}`},
			},
			wantOS: CodexOSLinux, wantSurface: CodexSurfaceDesktop,
			wantArch: CodexArchARM64, wantLevel: CodexProfileConfidenceHigh,
		},
		{
			name:        "python remains generic even on linux",
			headers:     http.Header{"User-Agent": {"openai-python/2.1.0 (linux; x86_64)"}},
			wantOS:      CodexOSGeneric,
			wantSurface: CodexSurfaceSDK,
			wantLevel:   CodexProfileConfidenceHigh,
		},
		{
			name:        "vscode is generic third party",
			headers:     http.Header{"User-Agent": {"codex_vscode/0.146.0 (Windows 11; x86_64) vscode"}},
			wantOS:      CodexOSGeneric,
			wantSurface: CodexSurfaceThirdParty,
			wantLevel:   CodexProfileConfidenceHigh,
		},
		{
			name:        "unknown does not guess an OS",
			headers:     http.Header{"User-Agent": {"unknown-client/1"}},
			wantOS:      CodexOSGeneric,
			wantSurface: CodexSurfaceThirdParty,
			wantLevel:   CodexProfileConfidenceLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyCodexClientProfile(CodexClientProfileSignals{Headers: tt.headers, Body: []byte(tt.body)})
			require.Equal(t, tt.wantOS, got.OSClass)
			require.Equal(t, tt.wantSurface, got.Surface)
			require.Equal(t, tt.wantArch, got.Architecture)
			require.Equal(t, tt.wantLevel, got.Confidence)
			for _, evidence := range got.Evidence {
				require.NotContains(t, strings.ToLower(evidence), "unknown-client")
			}
		})
	}
}

func TestClassifyCodexClientProfilePreservesAmbiguousStrongSignals(t *testing.T) {
	headers := http.Header{"User-Agent": {"codex_cli_rs/0.146.0 (Windows 11; x86_64) WindowsTerminal"}}
	body := []byte(`{"client_metadata":{"os":"linux","arch":"x86_64","surface":"cli"}}`)
	profile := ClassifyCodexClientProfile(CodexClientProfileSignals{Headers: headers, Body: body})

	require.True(t, profile.Ambiguous)
	require.Empty(t, profile.OSClass)
	ctx := withCodexProfileRequest(WithHTTPUpstreamIsolationScope(t.Context(), 7, 101), codexProfileRequest{
		Profile: profile, APIKeyScope: "user:7|key:101", ConversationHash: "ambiguous",
	})
	request, ok := codexProfileRequestFromContext(ctx)
	require.True(t, ok, "ambiguous is a classified failure, not an absent Profile context")
	require.True(t, request.Profile.Ambiguous)
}

func TestClassifyCodexClientProfileRejectsConflictingStrongSignals(t *testing.T) {
	profile := ClassifyCodexClientProfile(CodexClientProfileSignals{
		Headers: http.Header{"User-Agent": {"codex_cli_rs/0.146.0 (Windows 11; x86_64) WindowsTerminal"}},
		Body:    []byte(`{"client_metadata":{"os":"linux","arch":"x86_64"}}`),
	})
	require.True(t, profile.Ambiguous)
	require.Empty(t, profile.OSClass)

	profile = ClassifyCodexClientProfile(CodexClientProfileSignals{
		Headers: http.Header{"User-Agent": {"codex_cli_rs/0.146.0 (Ubuntu 22.04; arm64) xterm"}},
		Body:    []byte(`{"client_metadata":{"os":"linux","arch":"x86_64"}}`),
	})
	require.True(t, profile.Ambiguous)

	profile = ClassifyCodexClientProfile(CodexClientProfileSignals{
		Headers: http.Header{"User-Agent": {"Codex Desktop/0.146.0"}},
		Body:    []byte(`{"client_metadata":{"os":"linux","arch":"x86_64","workspace":"C:\\\\work\\project"}}`),
	})
	require.False(t, profile.Ambiguous)
	require.Equal(t, CodexOSLinux, profile.OSClass, "explicit OS must outrank weak workspace path evidence")
}

func TestResolveCodexRuntimeProfileCatalog(t *testing.T) {
	tests := []CodexOSProfilePolicy{
		{OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop, Architecture: CodexArchX8664, SlotCount: 1, Epoch: 1},
		{OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchARM64, SlotCount: 1, Epoch: 1},
		{OSClass: CodexOSMacOS, CanonicalSurface: CodexSurfaceDesktop, Architecture: CodexArchARM64, SlotCount: 1, Epoch: 1},
		{OSClass: CodexOSMacOS, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchARM64, SlotCount: 1, Epoch: 1},
		{OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceDesktop, Architecture: CodexArchARM64, SlotCount: 1, Epoch: 1},
		{OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchX8664, SlotCount: 1, Epoch: 1},
		{OSClass: CodexOSGeneric, CanonicalSurface: CodexSurfaceSDK, SlotCount: 1, Epoch: 1},
		{OSClass: CodexOSGeneric, CanonicalSurface: CodexSurfaceThirdParty, SlotCount: 1, Epoch: 1},
	}
	for _, policy := range tests {
		profile, err := ResolveCodexRuntimeProfile(policy)
		require.NoError(t, err)
		require.NotEmpty(t, profile.UserAgent)
		require.NotEmpty(t, profile.Originator)
		require.Equal(t, codexRuntimeCatalogVersion, profile.CatalogVersion)
		require.Contains(t, profile.UserAgent, profile.Version)
		require.GreaterOrEqual(t, CompareVersions(profile.Version, codexUpstreamMinVersion), 0)
		if profile.Surface == CodexSurfaceDesktop {
			require.NotEmpty(t, profile.AppBuild)
			require.NotEqual(t, profile.Version, profile.AppBuild)
			require.Contains(t, profile.UserAgent, "(Codex Desktop; "+profile.AppBuild+")")
		}
	}

	epochTwo, err := ResolveCodexRuntimeProfile(CodexOSProfilePolicy{
		OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceDesktop,
		Architecture: CodexArchARM64, SlotCount: 1, Epoch: 2,
	})
	require.NoError(t, err)
	require.Equal(t, codexRuntimeCatalogVersion, epochTwo.CatalogVersion)

	linux, err := ResolveCodexRuntimeProfile(CodexOSProfilePolicy{
		OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
		Architecture: CodexArchX8664, SlotCount: 1, Epoch: 1,
	})
	require.NoError(t, err)
	require.True(t, linux.Supports(CodexClientProfile{OSClass: CodexOSLinux, Surface: CodexSurfaceDesktop, Architecture: CodexArchX8664}))
	require.False(t, linux.Supports(CodexClientProfile{OSClass: CodexOSWindows, Surface: CodexSurfaceCLI, Architecture: CodexArchX8664}))
	require.True(t, linux.Supports(CodexClientProfile{OSClass: CodexOSLinux, Surface: CodexSurfaceCLI, Architecture: CodexArchARM64}), "Linux arm64 must converge to the account's canonical Linux x86_64 profile")

	windows, err := ResolveCodexRuntimeProfile(CodexOSProfilePolicy{
		OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
		Architecture: CodexArchX8664, SlotCount: 1, Epoch: 1,
	})
	require.NoError(t, err)
	require.True(t, windows.Supports(CodexClientProfile{OSClass: CodexOSWindows, Surface: CodexSurfaceCLI, Architecture: CodexArchARM64}), "Windows arm64 must converge to the account's canonical Windows x86_64 profile")
}

func TestBuildCodexIdentityAttemptPlanDefaultOffIsHardGate(t *testing.T) {
	plan, err := BuildCodexIdentityAttemptPlan(CodexIdentityAttemptInput{
		Mode:        CodexIdentityPolicyOff,
		AccountID:   -1,
		AccountSeed: "caller-controlled-invalid-seed",
	})
	require.NoError(t, err)
	require.Nil(t, plan)

	plan, err = BuildCodexIdentityAttemptPlan(CodexIdentityAttemptInput{})
	require.NoError(t, err)
	require.Nil(t, plan)
}

func TestBuildCodexIdentityAttemptPlanStableSlotsAndTenantSessions(t *testing.T) {
	base := codexRuntimeAttemptInput(t)
	first, err := BuildCodexIdentityAttemptPlan(base)
	require.NoError(t, err)
	second, err := BuildCodexIdentityAttemptPlan(base)
	require.NoError(t, err)
	require.Equal(t, first.RequestMappings, second.RequestMappings)

	otherKey := base
	otherKey.APIKeyScope = "user:7|key:202"
	otherKey.Source.SessionID = "client-session-b"
	otherKey.ConversationKey = "conversation-b"
	third, err := BuildCodexIdentityAttemptPlan(otherKey)
	require.NoError(t, err)
	require.Equal(t, first.UpstreamValue(CodexIdentityInstallation), third.UpstreamValue(CodexIdentityInstallation), "API keys assigned to one slot share only the device identity")
	require.NotEqual(t, first.UpstreamValue(CodexIdentitySession), third.UpstreamValue(CodexIdentitySession))
	require.NotEqual(t, first.UpstreamValue(CodexIdentityThread), third.UpstreamValue(CodexIdentityThread))

	otherSlot := base
	otherSlot.Slot.Index = 1
	fourth, err := BuildCodexIdentityAttemptPlan(otherSlot)
	require.NoError(t, err)
	require.NotEqual(t, first.UpstreamValue(CodexIdentityInstallation), fourth.UpstreamValue(CodexIdentityInstallation))

	otherEpoch := base
	otherEpoch.Slot.Epoch = 2
	fifth, err := BuildCodexIdentityAttemptPlan(otherEpoch)
	require.NoError(t, err)
	require.NotEqual(t, first.UpstreamValue(CodexIdentityInstallation), fifth.UpstreamValue(CodexIdentityInstallation))

	for _, mapping := range first.RequestMappings {
		require.NotContains(t, mapping.UpstreamValue, base.AccountSeed)
		require.NotContains(t, mapping.UpstreamValue, base.APIKeyScope)
	}
}

func TestBuildCodexIdentityAttemptPlanSessionPolicies(t *testing.T) {
	base := codexRuntimeAttemptInput(t)

	isolatedA, err := BuildCodexIdentityAttemptPlan(base)
	require.NoError(t, err)
	isolatedBInput := base
	isolatedBInput.ConversationKey = "another-conversation"
	isolatedB, err := BuildCodexIdentityAttemptPlan(isolatedBInput)
	require.NoError(t, err)
	require.NotEqual(t, isolatedA.UpstreamValue(CodexIdentitySession), isolatedB.UpstreamValue(CodexIdentitySession))

	sharedAInput := base
	sharedAInput.SessionPolicy = CodexSessionPolicySpec{Mode: CodexSessionAPIKeyShared}
	sharedA, err := BuildCodexIdentityAttemptPlan(sharedAInput)
	require.NoError(t, err)
	sharedBInput := sharedAInput
	sharedBInput.ConversationKey = "another-conversation"
	sharedB, err := BuildCodexIdentityAttemptPlan(sharedBInput)
	require.NoError(t, err)
	require.Equal(t, sharedA.UpstreamValue(CodexIdentitySession), sharedB.UpstreamValue(CodexIdentitySession))
	require.NotEqual(t, sharedA.UpstreamValue(CodexIdentityThread), sharedB.UpstreamValue(CodexIdentityThread))

	poolSessions := make(map[string]struct{})
	for i := 0; i < 50; i++ {
		poolInput := base
		poolInput.SessionPolicy = CodexSessionPolicySpec{Mode: CodexSessionPool, SessionsPerDevice: 3}
		poolInput.ConversationKey = "pool-conversation-" + string(rune('a'+i%26)) + strings.Repeat("x", i/26)
		poolPlan, poolErr := BuildCodexIdentityAttemptPlan(poolInput)
		require.NoError(t, poolErr)
		require.GreaterOrEqual(t, poolPlan.SessionSlotIndex, 0)
		require.Less(t, poolPlan.SessionSlotIndex, 3)
		poolSessions[poolPlan.UpstreamValue(CodexIdentitySession)] = struct{}{}
	}
	require.LessOrEqual(t, len(poolSessions), 3)
	require.Greater(t, len(poolSessions), 1)

	deviceInput := base
	deviceInput.SessionPolicy = CodexSessionPolicySpec{Mode: CodexSessionDeviceShared}
	_, err = BuildCodexIdentityAttemptPlan(deviceInput)
	require.ErrorContains(t, err, "device_shared requires")
	deviceInput.SessionPolicy = CodexSessionPolicySpec{
		Mode:                          CodexSessionDeviceShared,
		MaxActiveConversationsPerSlot: 1,
		DisableCrossKeyContinuation:   true,
	}
	deviceInput.SessionRuntime = CodexSessionRuntimeConstraints{MaxActiveConversationsPerSlot: 1, DisableCrossKeyContinuation: true}
	deviceA, err := BuildCodexIdentityAttemptPlan(deviceInput)
	require.NoError(t, err)
	deviceOtherKey := deviceInput
	deviceOtherKey.APIKeyScope = "user:8|key:303"
	deviceOtherKey.ConversationKey = "other-device-conversation"
	deviceB, err := BuildCodexIdentityAttemptPlan(deviceOtherKey)
	require.NoError(t, err)
	require.Equal(t, deviceA.UpstreamValue(CodexIdentitySession), deviceB.UpstreamValue(CodexIdentitySession))
	require.NotEqual(t, deviceA.UpstreamValue(CodexIdentityThread), deviceB.UpstreamValue(CodexIdentityThread))
}

func TestBuildCodexIdentityAttemptPlanFailoverChangesAccountIdentity(t *testing.T) {
	firstInput := codexRuntimeAttemptInput(t)
	first, err := BuildCodexIdentityAttemptPlan(firstInput)
	require.NoError(t, err)

	secondInput := firstInput
	secondInput.AccountID = 92
	secondInput.AccountSeed = "22222222-2222-4222-8222-222222222222"
	second, err := BuildCodexIdentityAttemptPlan(secondInput)
	require.NoError(t, err)
	require.NotEqual(t, first.UpstreamValue(CodexIdentityInstallation), second.UpstreamValue(CodexIdentityInstallation))
	require.NotEqual(t, first.UpstreamValue(CodexIdentitySession), second.UpstreamValue(CodexIdentitySession))
}

func TestExtractCodexIdentitySourceKnownFieldsOnly(t *testing.T) {
	headers := http.Header{
		"X-Codex-Installation-Id": {"header-install"},
		"Session-Id":              {"header-session"},
		"Thread-Id":               {"header-thread"},
		"X-Codex-Turn-Metadata":   {`{"turn_id":"header-turn","window_id":"header-window"}`},
	}
	body := []byte(`{
		"prompt_cache_key":"cache-a",
		"client_metadata":{
			"installation_id":"body-install",
			"session_id":"body-session",
			"workspace_path":"/home/alice/project",
			"git_remote_url":"git@example.com:private/project.git",
			"git_sha":"0123456789012345678901234567890123456789"
		},
		"input":[{"role":"user","content":"session_id must not be scanned"}]
	}`)
	source := ExtractCodexIdentitySource(headers, body)
	require.Equal(t, "header-install", source.InstallationID)
	require.Equal(t, "header-session", source.SessionID)
	require.Equal(t, "header-thread", source.ThreadID)
	require.Equal(t, "header-turn", source.TurnID)
	require.Equal(t, "header-window", source.WindowID)
	require.Equal(t, "cache-a", source.PromptCacheKey)
	require.Equal(t, "/home/alice/project", source.Workspace)
	require.Equal(t, "git@example.com:private/project.git", source.GitRemote)
	require.Equal(t, "0123456789012345678901234567890123456789", source.GitCommit)
}

func codexRuntimeAttemptInput(t *testing.T) CodexIdentityAttemptInput {
	t.Helper()
	profile, err := ResolveCodexRuntimeProfile(CodexOSProfilePolicy{
		OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
		Architecture: CodexArchX8664, SlotCount: 2, Epoch: 1,
	})
	require.NoError(t, err)
	return CodexIdentityAttemptInput{
		Mode:            CodexIdentityPolicyOSProfileDevicePool,
		AccountID:       91,
		APIKeyScope:     "user:7|key:101",
		AccountSeed:     codexRuntimeTestSeed,
		Profile:         profile,
		Slot:            CodexResolvedSlot{Index: 0, Epoch: 1},
		SessionPolicy:   CodexSessionPolicySpec{Mode: CodexSessionConversationIsolated},
		ConversationKey: "conversation-a",
		RequestNonce:    "request-a",
		Source: CodexIdentitySource{
			InstallationID: "client-install",
			SessionID:      "client-session",
			ConversationID: "client-conversation",
			ThreadID:       "client-thread",
			TurnID:         "client-turn",
			WindowID:       "client-window:0",
			PromptCacheKey: "client-cache",
			Workspace:      "/home/alice/private-project",
			GitRemote:      "git@example.com:private/project.git",
			GitCommit:      "0123456789012345678901234567890123456789",
		},
	}
}
