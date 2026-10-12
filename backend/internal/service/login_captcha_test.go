//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoginCaptchaPolicy(t *testing.T) {
	for _, tc := range []struct {
		name        string
		value       string
		required    bool
		wantEnabled bool
	}{
		{"missing fails closed", "", false, true},
		{"enabled", "true", false, true},
		{"disabled for login only", "false", false, false},
		{"invalid fails closed", "invalid", false, true},
		{"required overrides disabled", "false", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := map[string]string{
				SettingKeyLoginCaptchaEnabled: tc.value,
				SettingKeyTurnstileEnabled:    "true",
				SettingKeyTurnstileSiteKey:    "site",
				SettingKeyTurnstileSecretKey:  "secret",
			}
			svc := newAuthServiceForCaptchaTest(settings, tc.required, &turnstileVerifierSpy{}, nil)
			public, err := svc.settingService.GetPublicSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.wantEnabled, public.LoginCaptchaEnabled)
			err = svc.VerifyCaptchaForLogin(context.Background(), CaptchaProof{}, "203.0.113.1")
			if tc.wantEnabled {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Error(t, svc.VerifyCaptchaForRegister(context.Background(), CaptchaProof{}, "203.0.113.1", ""))
			require.Error(t, svc.VerifyCaptcha(context.Background(), CaptchaProof{}, "203.0.113.1"))
		})
	}
}

func TestLoginCaptchaReadFailure(t *testing.T) {
	svc := newAuthServiceForCaptchaRepoTest(&settingRepoStub{err: errors.New("unavailable")}, false, &turnstileVerifierSpy{}, nil)
	require.ErrorIs(t, svc.VerifyCaptchaForLogin(context.Background(), CaptchaProof{}, ""), ErrServiceUnavailable)
}
