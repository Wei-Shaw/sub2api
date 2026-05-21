//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type turnstileVerifierSpy struct {
	called    int
	lastToken string
	result    *TurnstileVerifyResponse
	err       error
}

func (s *turnstileVerifierSpy) VerifyToken(_ context.Context, _ string, token, _ string) (*TurnstileVerifyResponse, error) {
	s.called++
	s.lastToken = token
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &TurnstileVerifyResponse{Success: true}, nil
}

func newAuthServiceForRegisterTurnstileTest(settings map[string]string, verifier TurnstileVerifier) *AuthService {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Mode: "release",
		},
		Turnstile: config.TurnstileConfig{
			Required: true,
		},
	}

	settingService := NewSettingService(&settingRepoStub{values: settings}, cfg)
	turnstileService := NewTurnstileService(settingService, verifier)

	return NewAuthService(
		nil, // entClient
		&userRepoStub{},
		nil, // externalAuth
		nil, // redeemRepo
		nil, // refreshTokenCache
		cfg,
		settingService,
		nil, // emailService
		turnstileService,
		nil, // emailQueueService
		nil, // promoService
		nil, // defaultSubAssigner
		nil, // affiliateService
	)
}

func TestAuthService_VerifyTurnstileForRegister_SkipWhenEmailVerifyCodeProvided(t *testing.T) {
	verifier := &turnstileVerifierSpy{}
	service := newAuthServiceForRegisterTurnstileTest(map[string]string{
		SettingKeyEmailVerifyEnabled:  "true",
		SettingKeyTurnstileEnabled:    "true",
		SettingKeyTurnstileSecretKey:  "secret",
		SettingKeyRegistrationEnabled: "true",
	}, verifier)

	err := service.VerifyTurnstileForRegister(context.Background(), "", "127.0.0.1", "123456")
	require.NoError(t, err)
	require.Equal(t, 0, verifier.called)
}

func TestAuthService_VerifyTurnstileForRegister_RequireWhenVerifyCodeMissing(t *testing.T) {
	verifier := &turnstileVerifierSpy{}
	service := newAuthServiceForRegisterTurnstileTest(map[string]string{
		SettingKeyEmailVerifyEnabled: "true",
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "secret",
	}, verifier)

	err := service.VerifyTurnstileForRegister(context.Background(), "", "127.0.0.1", "")
	require.ErrorIs(t, err, ErrTurnstileVerificationFailed)
}

func TestAuthService_VerifyTurnstileForRegister_NoSkipWhenEmailVerifyDisabled(t *testing.T) {
	verifier := &turnstileVerifierSpy{}
	service := newAuthServiceForRegisterTurnstileTest(map[string]string{
		SettingKeyEmailVerifyEnabled: "false",
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "secret",
	}, verifier)

	err := service.VerifyTurnstileForRegister(context.Background(), "turnstile-token", "127.0.0.1", "123456")
	require.NoError(t, err)
	require.Equal(t, 1, verifier.called)
	require.Equal(t, "turnstile-token", verifier.lastToken)
}
