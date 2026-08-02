//go:build unit

package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type iamRecoveryEmailCacheStub struct {
	service.EmailCache
	resetToken *service.PasswordResetTokenData
	consumed   bool
}

func (s *iamRecoveryEmailCacheStub) GetPasswordResetToken(context.Context, string) (*service.PasswordResetTokenData, error) {
	return s.resetToken, nil
}

func (s *iamRecoveryEmailCacheStub) DeletePasswordResetToken(context.Context, string) error {
	s.consumed = true
	s.resetToken = nil
	return nil
}

func TestIAMVerifiedRecoveryEmailCanResetPassword(t *testing.T) {
	ctx := context.Background()
	cache := &iamRecoveryEmailCacheStub{resetToken: &service.PasswordResetTokenData{
		Token:     "reset-token",
		CreatedAt: time.Now(),
	}}
	auth, _, client := newAuthServiceForEmailBind(t, map[string]string{
		service.SettingKeyEmailVerifyEnabled:   "true",
		service.SettingKeyPasswordResetEnabled: "true",
	}, cache, nil)

	user, err := client.User.Create().
		SetAccountID("1719905235756637").
		SetExternalUserID("201705485041478971").
		SetIdentityType(service.IdentityTypeIAM).
		SetLoginName("finance.reader").
		SetPasswordHash("old-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetRecoveryEmail("recovery@example.com").
		SetRecoveryEmailVerifiedAt(time.Now()).
		SetMustChangePassword(true).
		Save(ctx)
	require.NoError(t, err)

	require.NoError(t, auth.ResetPassword(ctx, "recovery@example.com", "reset-token", "new-password"))
	require.True(t, cache.consumed)

	updated, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.True(t, auth.CheckPassword("new-password", updated.PasswordHash))
	require.False(t, updated.MustChangePassword)
	require.Equal(t, user.AuthzGeneration+1, updated.AuthzGeneration)
}

func TestIAMUnverifiedRecoveryEmailCannotResetPassword(t *testing.T) {
	ctx := context.Background()
	cache := &iamRecoveryEmailCacheStub{resetToken: &service.PasswordResetTokenData{
		Token:     "reset-token",
		CreatedAt: time.Now(),
	}}
	auth, _, client := newAuthServiceForEmailBind(t, map[string]string{
		service.SettingKeyEmailVerifyEnabled:   "true",
		service.SettingKeyPasswordResetEnabled: "true",
	}, cache, nil)

	_, err := client.User.Create().
		SetAccountID("1719905235756637").
		SetExternalUserID("201705485041478971").
		SetIdentityType(service.IdentityTypeIAM).
		SetLoginName("finance.reader").
		SetPasswordHash("old-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetRecoveryEmail("recovery@example.com").
		Save(ctx)
	require.NoError(t, err)

	err = auth.ResetPassword(ctx, "recovery@example.com", "reset-token", "new-password")
	require.ErrorIs(t, err, service.ErrInvalidResetToken)
}
