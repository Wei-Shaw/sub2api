package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexIdentityPolicyNormalizeDefaultsOff(t *testing.T) {
	got, err := (CodexIdentityPolicySpec{}).NormalizeAndValidate(PlatformOpenAI, AccountTypeOAuth)
	require.NoError(t, err)
	require.Equal(t, CodexIdentityPolicyOff, got.Mode)
	require.Equal(t, CodexIdentityBindingAPIKeyOS, got.BindingScope)
	require.Equal(t, CodexSessionConversationIsolated, got.SessionPolicy.Mode)
	require.Equal(t, defaultCodexAffinityTTLSeconds, got.AffinityTTLSeconds)
	require.Equal(t, CodexUnsupportedProfileReject, got.UnsupportedPolicy)
	require.EqualValues(t, 1, got.Version)
}

func TestCodexIdentityPolicyAcceptsLinuxDesktopAndProxyOverrides(t *testing.T) {
	profileProxy := int64(17)
	slotProxy := int64(23)
	got, err := (CodexIdentityPolicySpec{
		Mode: CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []CodexOSProfilePolicy{
			{
				OSClass:          CodexOSLinux,
				CanonicalSurface: CodexSurfaceDesktop,
				Architecture:     CodexArchARM64,
				SlotCount:        2,
				ProxyID:          &profileProxy,
				Slots: []CodexDeviceSlotPolicy{
					{Index: 1, ProxyID: &slotProxy},
				},
			},
		},
	}).NormalizeAndValidate(PlatformOpenAI, AccountTypeOAuth)
	require.NoError(t, err)
	require.EqualValues(t, 1, got.Profiles[0].Epoch)
	require.Equal(t, []int64{17, 23}, got.ReferencedProxyIDs())
}

func TestCodexIdentityPolicyRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		typeName string
		policy   CodexIdentityPolicySpec
	}{
		{
			name:     "non openai oauth",
			platform: PlatformAnthropic,
			typeName: AccountTypeOAuth,
			policy: CodexIdentityPolicySpec{Mode: CodexIdentityPolicyOSProfileDevicePool, Profiles: []CodexOSProfilePolicy{{
				OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchX8664, SlotCount: 1,
			}}},
		},
		{
			name:     "cross shape",
			platform: PlatformOpenAI,
			typeName: AccountTypeOAuth,
			policy: CodexIdentityPolicySpec{Mode: CodexIdentityPolicyOSProfileDevicePool, Profiles: []CodexOSProfilePolicy{{
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceSDK, Architecture: CodexArchX8664, SlotCount: 1,
			}}},
		},
		{
			name:     "duplicate os",
			platform: PlatformOpenAI,
			typeName: AccountTypeOAuth,
			policy: CodexIdentityPolicySpec{Mode: CodexIdentityPolicyOSProfileDevicePool, Profiles: []CodexOSProfilePolicy{
				{OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchX8664, SlotCount: 1},
				{OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceDesktop, Architecture: CodexArchARM64, SlotCount: 1},
			}},
		},
		{
			name:     "slot outside count",
			platform: PlatformOpenAI,
			typeName: AccountTypeOAuth,
			policy: CodexIdentityPolicySpec{Mode: CodexIdentityPolicyOSProfileDevicePool, Profiles: []CodexOSProfilePolicy{{
				OSClass: CodexOSMacOS, CanonicalSurface: CodexSurfaceDesktop, Architecture: CodexArchARM64, SlotCount: 1,
				Slots: []CodexDeviceSlotPolicy{{Index: 1}},
			}}},
		},
		{
			name:     "invalid session pool",
			platform: PlatformOpenAI,
			typeName: AccountTypeOAuth,
			policy: CodexIdentityPolicySpec{
				Mode:          CodexIdentityPolicyOSProfileDevicePool,
				SessionPolicy: CodexSessionPolicySpec{Mode: CodexSessionPool, SessionsPerDevice: 4},
				Profiles: []CodexOSProfilePolicy{{
					OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchX8664, SlotCount: 1,
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.policy.NormalizeAndValidate(tt.platform, tt.typeName)
			require.Error(t, err)
		})
	}
}

func TestPendingAccountIsNotSchedulable(t *testing.T) {
	account := &Account{
		Status:            StatusActive,
		Schedulable:       true,
		ProvisioningState: AccountProvisioningPending,
	}
	require.False(t, account.IsSchedulable())

	legacy := &Account{Status: StatusActive, Schedulable: true}
	require.True(t, legacy.IsSchedulable(), "legacy cache payloads without provisioning_state remain compatible")
}

func TestCodexDeviceSharedRequiresHardSafetyLimits(t *testing.T) {
	base := CodexIdentityPolicySpec{
		Mode:          CodexIdentityPolicyOSProfileDevicePool,
		SessionPolicy: CodexSessionPolicySpec{Mode: CodexSessionDeviceShared},
		Profiles: []CodexOSProfilePolicy{{
			OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop, Architecture: CodexArchX8664, SlotCount: 1,
		}},
	}
	_, err := base.NormalizeAndValidate(PlatformOpenAI, AccountTypeOAuth)
	require.Error(t, err)

	base.SessionPolicy.MaxActiveConversationsPerSlot = 1
	base.SessionPolicy.DisableCrossKeyContinuation = true
	_, err = base.NormalizeAndValidate(PlatformOpenAI, AccountTypeOAuth)
	require.NoError(t, err)
}

func TestAccountProvisioningRejectsLegacyAndNewFingerprintModesTogether(t *testing.T) {
	policy := CodexIdentityPolicySpec{
		Mode: CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []CodexOSProfilePolicy{{
			OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchX8664, SlotCount: 1,
		}},
	}
	_, err := (AccountProvisioningSpec{
		Account: &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{codexFingerprintModeExtraKey: string(codexFingerprintSession)},
		},
		Identity: &policy,
	}).NormalizeAndValidate()
	require.Error(t, err)
}

func TestCodexIdentityPolicyServerOwnsVersionAndProfileEpoch(t *testing.T) {
	requested := CodexIdentityPolicySpec{
		Mode:    CodexIdentityPolicyOSProfileDevicePool,
		Version: 99,
		Profiles: []CodexOSProfilePolicy{{
			OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchX8664, SlotCount: 1, Epoch: 88,
		}},
	}
	created, err := PrepareCodexIdentityPolicyForCreate(requested, PlatformOpenAI, AccountTypeOAuth)
	require.NoError(t, err)
	require.EqualValues(t, 1, created.Version)
	require.EqualValues(t, 1, created.Profiles[0].Epoch)

	sessionOnly := created
	sessionOnly.SessionPolicy = CodexSessionPolicySpec{Mode: CodexSessionAPIKeyShared}
	updated, changed, err := PrepareCodexIdentityPolicyForUpdate(created, sessionOnly, PlatformOpenAI, AccountTypeOAuth)
	require.NoError(t, err)
	require.True(t, changed)
	require.EqualValues(t, 2, updated.Version)
	require.EqualValues(t, 1, updated.Profiles[0].Epoch, "session-only change must not rotate device identity")

	profileChange := updated
	profileChange.Profiles = append([]CodexOSProfilePolicy(nil), updated.Profiles...)
	profileChange.Profiles[0].Slots = append([]CodexDeviceSlotPolicy(nil), updated.Profiles[0].Slots...)
	profileChange.Profiles[0].CanonicalSurface = CodexSurfaceDesktop
	profileChange.Version = 500
	profileChange.Profiles[0].Epoch = 500
	updatedAgain, changed, err := PrepareCodexIdentityPolicyForUpdate(updated, profileChange, PlatformOpenAI, AccountTypeOAuth)
	require.NoError(t, err)
	require.True(t, changed)
	require.EqualValues(t, 3, updatedAgain.Version)
	require.EqualValues(t, 2, updatedAgain.Profiles[0].Epoch)
}

func TestCodexIdentityPolicyCanBeDisabledWhileAccountLeavesOAuth(t *testing.T) {
	existing, err := PrepareCodexIdentityPolicyForCreate(CodexIdentityPolicySpec{
		Mode: CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []CodexOSProfilePolicy{{
			OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
			Architecture: CodexArchX8664, SlotCount: 1,
		}},
	}, PlatformOpenAI, AccountTypeOAuth)
	require.NoError(t, err)

	next, changed, err := PrepareCodexIdentityPolicyForAccountTransition(
		existing, PlatformOpenAI, AccountTypeOAuth,
		DefaultCodexIdentityPolicySpec(), PlatformOpenAI, AccountTypeAPIKey,
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, CodexIdentityPolicyOff, next.Mode)
	require.Empty(t, next.Profiles)
	require.EqualValues(t, 2, next.Version)
}

func TestAccountProvisioningSpecRejectsCallerControlledActiveWithoutAccount(t *testing.T) {
	_, err := (AccountProvisioningSpec{ProvisioningState: AccountProvisioningActive}).NormalizeAndValidate()
	require.Error(t, err)
}

func TestAccountProvisioningCannotActivateDevicePoolWithoutOAuthCredential(t *testing.T) {
	policy := CodexIdentityPolicySpec{
		Mode: CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []CodexOSProfilePolicy{{
			OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
			Architecture: CodexArchX8664, SlotCount: 1,
		}},
	}
	base := AccountProvisioningSpec{
		Account:  &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{}},
		Identity: &policy, FinalStatus: StatusActive, Schedulable: true,
		ProvisioningState: AccountProvisioningActive,
	}
	_, err := base.NormalizeAndValidate()
	require.Error(t, err)

	base.ProvisioningState = AccountProvisioningPending
	_, err = base.NormalizeAndValidate()
	require.NoError(t, err, "pending rows may hold incomplete credentials but remain unschedulable")

	base.ProvisioningState = AccountProvisioningActive
	base.Account.Credentials["refresh_token"] = "rt-test"
	_, err = base.NormalizeAndValidate()
	require.NoError(t, err)
}

func TestAccountProvisioningAcceptsValidAgentIdentityCredential(t *testing.T) {
	key, encodedPrivateKey := newTestAgentIdentityKey(t)
	policy := CodexIdentityPolicySpec{
		Mode: CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []CodexOSProfilePolicy{{
			OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
			Architecture: CodexArchX8664, SlotCount: 1,
		}},
	}
	_, err := (AccountProvisioningSpec{
		Account: &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"auth_mode":         OpenAIAuthModeAgentIdentity,
				"agent_runtime_id":  key.runtimeID,
				"agent_private_key": encodedPrivateKey,
			},
		},
		Identity: &policy, FinalStatus: StatusActive, Schedulable: true,
		ProvisioningState: AccountProvisioningActive,
	}).NormalizeAndValidate()
	require.NoError(t, err)
}

func TestAccountProvisioningAgentIdentityCannotUseBearerTokenToBypassKeyValidation(t *testing.T) {
	policy := CodexIdentityPolicySpec{
		Mode: CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []CodexOSProfilePolicy{{
			OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
			Architecture: CodexArchX8664, SlotCount: 1,
		}},
	}
	_, err := (AccountProvisioningSpec{
		Account: &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"auth_mode":         OpenAIAuthModeAgentIdentity,
				"access_token":      "stale-bearer-token",
				"agent_runtime_id":  "runtime-with-invalid-key",
				"agent_private_key": "not-a-pkcs8-ed25519-key",
			},
		},
		Identity: &policy, FinalStatus: StatusActive, Schedulable: true,
		ProvisioningState: AccountProvisioningActive,
	}).NormalizeAndValidate()
	require.Error(t, err)
}

func TestCodexProxyModeDistinguishesDirectFromInherited(t *testing.T) {
	proxyID := int64(17)
	policy, err := (CodexIdentityPolicySpec{
		Mode: CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []CodexOSProfilePolicy{
			{
				OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
				Architecture: CodexArchX8664, SlotCount: 1, ProxyMode: CodexProxyDirect,
			},
			{
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceCLI,
				Architecture: CodexArchX8664, SlotCount: 1, ProxyID: &proxyID,
			},
		},
	}).NormalizeAndValidate(PlatformOpenAI, AccountTypeOAuth)
	require.NoError(t, err)
	require.Equal(t, CodexProxyDirect, policy.Profiles[0].ProxyMode)
	require.Equal(t, CodexProxyExplicit, policy.Profiles[1].ProxyMode)

	policy.Profiles[0].ProxyID = &proxyID
	_, err = policy.NormalizeAndValidate(PlatformOpenAI, AccountTypeOAuth)
	require.Error(t, err)
}
