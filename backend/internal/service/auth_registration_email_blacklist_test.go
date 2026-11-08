//go:build unit

package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAuthService_Register_BlacklistedEmailAliasRejected(t *testing.T) {
	repo := &userRepoStub{nextID: 9}
	svc := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:        "true",
		SettingKeyRegistrationEmailBlacklist: `["spam.user@gmail.com"]`,
	}, nil, nil)

	_, _, err := svc.Register(context.Background(), "SpamUser+1@googlemail.com", "password")
	require.ErrorIs(t, err, ErrEmailBlocked)
	require.Equal(t, "EMAIL_BLOCKED", infraerrors.FromError(err).Reason)
	require.Empty(t, repo.created)
}

func TestAuthService_Register_BlacklistOverridesSuffixWhitelist(t *testing.T) {
	repo := &userRepoStub{nextID: 9}
	svc := newAuthService(repo, map[string]string{
		SettingKeyRegistrationEnabled:              "true",
		SettingKeyRegistrationEmailSuffixWhitelist: `["@gmail.com"]`,
		SettingKeyRegistrationEmailBlacklist:       `["blocked@gmail.com"]`,
	}, nil, nil)

	_, _, err := svc.Register(context.Background(), "blocked@gmail.com", "password")
	require.ErrorIs(t, err, ErrEmailBlocked)
	require.Empty(t, repo.created)

	_, user, err := svc.Register(context.Background(), "allowed@gmail.com", "password")
	require.NoError(t, err)
	require.Equal(t, "allowed@gmail.com", user.Email)
}

func TestAuthService_SendVerifyCode_BlacklistedEmailRejected(t *testing.T) {
	svc := newAuthService(&userRepoStub{}, map[string]string{
		SettingKeyRegistrationEnabled:        "true",
		SettingKeyEmailVerifyEnabled:         "true",
		SettingKeyRegistrationEmailBlacklist: `["blocked@example.com"]`,
	}, nil, nil)

	require.ErrorIs(t, svc.SendVerifyCode(context.Background(), "blocked+x@example.com"), ErrEmailBlocked)
	_, err := svc.SendVerifyCodeAsync(context.Background(), "blocked@example.com")
	require.ErrorIs(t, err, ErrEmailBlocked)
}

func TestAuthService_ValidateRegistrationEmailPolicy_BlacklistedEmailRejected(t *testing.T) {
	svc := newAuthService(&userRepoStub{}, map[string]string{
		SettingKeyRegistrationEmailBlacklist: `["bind.target@gmail.com"]`,
	}, nil, nil)

	require.ErrorIs(t, svc.validateRegistrationEmailPolicy(context.Background(), "bindtarget@gmail.com"), ErrEmailBlocked)
	require.NoError(t, svc.validateRegistrationEmailPolicy(context.Background(), "someone@gmail.com"))
}
