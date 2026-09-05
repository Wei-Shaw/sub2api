package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type testCodexClientProfileProvider func(CodexClientProfileRequest) (CodexClientProfileRecord, error)

func (f testCodexClientProfileProvider) LookupCodexClientProfile(r CodexClientProfileRequest) (CodexClientProfileRecord, error) {
	return f(r)
}

func TestCodexClientProfileProvider(t *testing.T) {
	policy := CodexOSProfilePolicy{OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchX8664, SlotCount: 1}
	profile, err := ResolveCodexRuntimeProfileWithProvider(policy, "0.146.0", nil)
	require.NoError(t, err)
	require.Equal(t, CodexClientProfileUnverified, profile.ClientProfileVerification)
	require.Equal(t, "builtin", profile.ClientProfileSource)
	require.Contains(t, profile.UserAgent, "0.146.0")

	fixture := testCodexClientProfileProvider(func(r CodexClientProfileRequest) (CodexClientProfileRecord, error) {
		record, err := (BuiltinCodexClientProfileProvider{}).LookupCodexClientProfile(r)
		record.Source = "unit-test-fixture"
		record.Evidence = "test-only metadata fixture, not an official release verification"
		record.Verification = CodexClientProfileVerified
		return record, err
	})
	profile, err = ResolveCodexRuntimeProfileWithProvider(policy, "0.146.0", fixture)
	require.NoError(t, err)
	require.Equal(t, CodexClientProfileVerified, profile.ClientProfileVerification)
	require.Equal(t, "unit-test-fixture", profile.ClientProfileSource)

	for name, mutate := range map[string]func(*CodexClientProfileRecord){
		"different version": func(r *CodexClientProfileRecord) { r.Request.ClientVersion = "0.145.0" },
		"different surface": func(r *CodexClientProfileRecord) { r.Request.Surface = CodexSurfaceDesktop },
		"missing evidence":  func(r *CodexClientProfileRecord) { r.Evidence = "" },
		"missing source":    func(r *CodexClientProfileRecord) { r.Source = "" },
		"unknown status":    func(r *CodexClientProfileRecord) { r.Verification = "assumed" },
		"invalid identity":  func(r *CodexClientProfileRecord) { r.ClientName = "unsupported-client" },
	} {
		t.Run(name, func(t *testing.T) {
			provider := testCodexClientProfileProvider(func(request CodexClientProfileRequest) (CodexClientProfileRecord, error) {
				record, err := fixture(request)
				mutate(&record)
				return record, err
			})
			_, err := ResolveCodexRuntimeProfileWithProvider(policy, "0.146.0", provider)
			require.Error(t, err)
		})
	}
	unavailable := errors.New("fixture unavailable")
	_, err = ResolveCodexRuntimeProfileWithProvider(policy, "0.146.0", testCodexClientProfileProvider(func(CodexClientProfileRequest) (CodexClientProfileRecord, error) {
		return CodexClientProfileRecord{}, unavailable
	}))
	require.ErrorIs(t, err, unavailable)
}
