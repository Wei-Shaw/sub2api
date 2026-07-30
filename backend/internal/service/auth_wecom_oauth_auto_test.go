//go:build unit

package service_test

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newAuthServiceWithRefreshTokenCache(
	base *service.AuthService,
	repo service.UserRepository,
	settings map[string]string,
) *service.AuthService {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-auth-identity-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
		},
		Default: config.DefaultConfig{
			UserBalance:     3.5,
			UserConcurrency: 2,
		},
	}
	return service.NewAuthService(
		base.EntClient(),
		repo,
		nil,
		newEmailBindRefreshTokenCacheStub(),
		cfg,
		service.NewSettingService(&authIdentitySettingRepoStub{values: settings}, cfg),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

func TestAuthServiceLoginOrRegisterWeComCreatesUserWhenRegistrationDisabled(t *testing.T) {
	svc, repo, client := newAuthServiceWithEnt(t, map[string]string{
		service.SettingKeyRegistrationEnabled: "false",
	}, nil)
	svcWithTokenCache := newAuthServiceWithRefreshTokenCache(svc, repo, map[string]string{
		service.SettingKeyRegistrationEnabled: "false",
	})
	ctx := context.Background()

	tokenPair, user, err := svcWithTokenCache.LoginOrRegisterVerifiedEmailOAuth(ctx, service.EmailOAuthIdentityInput{
		ProviderType:     "wecom",
		ProviderKey:      "wecom-main",
		ProviderSubject:  "corp/user1",
		Email:            "user1" + service.WeComConnectSyntheticEmailDomain,
		EmailVerified:    true,
		Username:         "user1",
		UpstreamMetadata: map[string]any{"userid": "user1"},
	})

	require.NoError(t, err)
	require.NotNil(t, tokenPair)
	require.NotNil(t, user)
	require.Equal(t, "wecom", user.SignupSource)

	storedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "wecom", storedUser.SignupSource)
	require.Equal(t, "user1"+service.WeComConnectSyntheticEmailDomain, storedUser.Email)
}

func TestAuthServiceLoginOrRegisterWeComCreatesUserWithoutInvitationCode(t *testing.T) {
	settings := map[string]string{
		service.SettingKeyRegistrationEnabled:   "false",
		service.SettingKeyInvitationCodeEnabled: "true",
	}
	svc, repo, _ := newAuthServiceWithEnt(t, settings, nil)
	svcWithTokenCache := newAuthServiceWithRefreshTokenCache(svc, repo, settings)

	tokenPair, user, err := svcWithTokenCache.LoginOrRegisterVerifiedEmailOAuth(
		context.Background(),
		service.EmailOAuthIdentityInput{
			ProviderType:    "wecom",
			ProviderKey:     "wecom-main",
			ProviderSubject: "corp/no-invitation",
			Email:           "no-invitation" + service.WeComConnectSyntheticEmailDomain,
			EmailVerified:   true,
			Username:        "no-invitation",
		},
	)

	require.NoError(t, err)
	require.NotNil(t, tokenPair)
	require.NotNil(t, user)
}

func TestAuthServiceLoginOrRegisterWeComCreatesUserWithRealEmailWhenRegistrationDisabled(t *testing.T) {
	svc, repo, client := newAuthServiceWithEnt(t, map[string]string{
		service.SettingKeyRegistrationEnabled: "false",
	}, nil)
	svcWithTokenCache := newAuthServiceWithRefreshTokenCache(svc, repo, map[string]string{
		service.SettingKeyRegistrationEnabled: "false",
	})
	ctx := context.Background()

	tokenPair, user, err := svcWithTokenCache.LoginOrRegisterVerifiedEmailOAuth(ctx, service.EmailOAuthIdentityInput{
		ProviderType:     "wecom",
		ProviderKey:      "wecom-main",
		ProviderSubject:  "corp/user-real",
		Email:            "User.Real@Example.COM",
		EmailVerified:    true,
		Username:         "user-real",
		UpstreamMetadata: map[string]any{"userid": "user-real"},
	})

	require.NoError(t, err)
	require.NotNil(t, tokenPair)
	require.NotNil(t, user)
	require.Equal(t, "user.real@example.com", user.Email)

	storedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "wecom", storedUser.SignupSource)
	require.Equal(t, "user.real@example.com", storedUser.Email)
}

func TestAuthServiceLoginOrRegisterWeComAllowsIdentityEmailChange(t *testing.T) {
	svc, repo, client := newAuthServiceWithEnt(t, map[string]string{
		service.SettingKeyRegistrationEnabled: "false",
	}, nil)
	svcWithTokenCache := newAuthServiceWithRefreshTokenCache(svc, repo, map[string]string{
		service.SettingKeyRegistrationEnabled: "false",
	})
	ctx := context.Background()

	existing := &service.User{
		Email:       "legacy" + service.WeComConnectSyntheticEmailDomain,
		Username:    "legacy-wecom",
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Balance:     1,
		Concurrency: 1,
	}
	require.NoError(t, existing.SetPassword("password"))
	require.NoError(t, repo.Create(ctx, existing))

	_, err := client.AuthIdentity.Create().
		SetUserID(existing.ID).
		SetProviderType("wecom").
		SetProviderKey("wecom-main").
		SetProviderSubject("corp/legacy").
		SetMetadata(map[string]any{"email": existing.Email}).
		Save(ctx)
	require.NoError(t, err)

	tokenPair, user, err := svcWithTokenCache.LoginOrRegisterVerifiedEmailOAuth(ctx, service.EmailOAuthIdentityInput{
		ProviderType:     "wecom",
		ProviderKey:      "wecom-main",
		ProviderSubject:  "corp/legacy",
		Email:            "legacy.real@example.com",
		EmailVerified:    true,
		Username:         "legacy-real",
		UpstreamMetadata: map[string]any{"userid": "legacy"},
	})

	require.NoError(t, err)
	require.NotNil(t, tokenPair)
	require.NotNil(t, user)
	require.Equal(t, existing.ID, user.ID)
	require.Equal(t, existing.Email, user.Email)
}

func TestAuthServiceLoginOrRegisterGitHubStillRequiresRegistrationEnabled(t *testing.T) {
	svc, _, _ := newAuthServiceWithEnt(t, map[string]string{
		service.SettingKeyRegistrationEnabled: "false",
	}, nil)

	tokenPair, user, err := svc.LoginOrRegisterVerifiedEmailOAuth(context.Background(), service.EmailOAuthIdentityInput{
		ProviderType:    "github",
		ProviderKey:     "github",
		ProviderSubject: "12345",
		Email:           "fresh-github@example.com",
		EmailVerified:   true,
		Username:        "fresh-github",
	})

	require.Nil(t, tokenPair)
	require.Nil(t, user)
	require.ErrorIs(t, err, service.ErrRegDisabled)
}
