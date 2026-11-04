//go:build unit

package service_test

import (
	"context"
	"testing"

	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestLoginOrRegisterVerifiedEmailOAuth_BlacklistedExistingUserCanStillSignIn(t *testing.T) {
	svc, repo, _ := newAuthServiceForEmailBindWithRefreshCache(t, map[string]string{
		service.SettingKeyRegistrationEnabled:        "true",
		service.SettingKeyRegistrationEmailBlacklist: `["kept.user@gmail.com"]`,
	}, nil, nil, newEmailBindRefreshTokenCacheStub())
	ctx := context.Background()

	existing := &service.User{
		Email:        "kept.user@gmail.com",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Concurrency:  1,
	}
	require.NoError(t, repo.Create(ctx, existing))

	tokens, user, err := svc.LoginOrRegisterVerifiedEmailOAuth(ctx, service.EmailOAuthIdentityInput{
		ProviderType:    "github",
		ProviderSubject: "gh-kept-user",
		Email:           "kept.user@gmail.com",
		EmailVerified:   true,
		Username:        "kept",
	})
	require.NoError(t, err)
	require.NotNil(t, tokens)
	require.Equal(t, existing.ID, user.ID)
}

func TestLoginOrRegisterVerifiedEmailOAuth_BlacklistedNewEmailRejected(t *testing.T) {
	svc, _, client := newAuthServiceForEmailBindWithRefreshCache(t, map[string]string{
		service.SettingKeyRegistrationEnabled:        "true",
		service.SettingKeyRegistrationEmailBlacklist: `["blocked.user@gmail.com"]`,
	}, nil, nil, newEmailBindRefreshTokenCacheStub())
	ctx := context.Background()

	_, _, err := svc.LoginOrRegisterVerifiedEmailOAuth(ctx, service.EmailOAuthIdentityInput{
		ProviderType:    "google",
		ProviderSubject: "google-blocked-user",
		Email:           "blockeduser+x@googlemail.com",
		EmailVerified:   true,
	})
	require.ErrorIs(t, err, service.ErrEmailBlocked)

	count, err := client.User.Query().Where(dbuser.EmailEQ("blockeduser+x@googlemail.com")).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}
