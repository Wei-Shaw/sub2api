//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type captchaRegisterVerifierSpy struct {
	called    int
	lastToken string
	result    *VerifyResult
	err       error
}

func (s *captchaRegisterVerifierSpy) Verify(_ context.Context, req VerifyRequest) (*VerifyResult, error) {
	s.called++
	s.lastToken = req.Payload["token"]
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &VerifyResult{Success: true}, nil
}

func (s *captchaRegisterVerifierSpy) ValidateProviderConfig(_ context.Context, _ string, _ map[string]string) error {
	return nil
}

func newAuthServiceForRegisterCaptchaTest(settings map[string]string, verifier CaptchaVerifier) *AuthService {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Mode: "release",
		},
		Turnstile: config.TurnstileConfig{
			Required: true,
		},
	}

	settingService := NewSettingService(&settingRepoStub{values: settings}, cfg)
	captchaService := NewCaptchaService(settingService, verifier)

	return NewAuthService(
		nil, // entClient
		&userRepoStub{},
		nil, // redeemRepo
		nil, // refreshTokenCache
		cfg,
		settingService,
		nil, // emailService
		captchaService,
		nil, // emailQueueService
		nil, // promoService
		nil, // defaultSubAssigner
		nil, // affiliateService
		nil, // userPlatformQuotaRepo
	)
}

func TestAuthService_VerifyCaptchaForRegister_SkipWhenEmailVerifyCodeProvided(t *testing.T) {
	verifier := &captchaRegisterVerifierSpy{}
	service := newAuthServiceForRegisterCaptchaTest(map[string]string{
		SettingKeyEmailVerifyEnabled:  "true",
		SettingKeyTurnstileEnabled:    "true",
		SettingKeyTurnstileSecretKey:  "secret",
		SettingKeyRegistrationEnabled: "true",
	}, verifier)

	err := service.VerifyCaptchaForRegister(context.Background(), "", "127.0.0.1", "123456")
	require.NoError(t, err)
	require.Equal(t, 0, verifier.called)
}

func TestAuthService_VerifyCaptchaForRegister_RequireWhenVerifyCodeMissing(t *testing.T) {
	verifier := &captchaRegisterVerifierSpy{}
	service := newAuthServiceForRegisterCaptchaTest(map[string]string{
		SettingKeyEmailVerifyEnabled: "true",
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "secret",
	}, verifier)

	err := service.VerifyCaptchaForRegister(context.Background(), "", "127.0.0.1", "")
	require.ErrorIs(t, err, ErrTurnstileVerificationFailed)
}

func TestAuthService_VerifyCaptchaForRegister_NoSkipWhenEmailVerifyDisabled(t *testing.T) {
	verifier := &captchaRegisterVerifierSpy{}
	service := newAuthServiceForRegisterCaptchaTest(map[string]string{
		SettingKeyEmailVerifyEnabled: "false",
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "secret",
	}, verifier)

	err := service.VerifyCaptchaForRegister(context.Background(), "turnstile-token", "127.0.0.1", "123456")
	require.NoError(t, err)
	require.Equal(t, 1, verifier.called)
	require.Equal(t, "turnstile-token", verifier.lastToken)
}
