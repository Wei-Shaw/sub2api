//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// keyedEmailCacheStub stores verification codes per cache key so tests can
// assert which namespace a code was written to.
type keyedEmailCacheStub struct {
	emailCacheStub
	codes map[string]*VerificationCodeData
}

func (s *keyedEmailCacheStub) GetVerificationCode(_ context.Context, key string) (*VerificationCodeData, error) {
	return s.codes[key], nil
}

func (s *keyedEmailCacheStub) SetVerificationCode(_ context.Context, key string, data *VerificationCodeData, _ time.Duration) error {
	if s.codes == nil {
		s.codes = map[string]*VerificationCodeData{}
	}
	s.codes[key] = data
	return nil
}

func (s *keyedEmailCacheStub) DeleteVerificationCode(_ context.Context, key string) error {
	delete(s.codes, key)
	return nil
}

func (s *keyedEmailCacheStub) IncrVerificationCodeAttempts(_ context.Context, key string) (int, error) {
	data := s.codes[key]
	if data == nil {
		return 0, ErrInvalidVerifyCode
	}
	data.Attempts++
	return data.Attempts, nil
}

func TestRequiresAdminLoginEmailCode(t *testing.T) {
	ctx := context.Background()
	admin := &User{ID: 1, Email: "admin@example.com", Role: RoleAdmin}
	user := &User{ID: 2, Email: "user@example.com", Role: RoleUser}
	smtp := map[string]string{SettingKeySMTPHost: "smtp.example.com"}

	withSettings := func(extra map[string]string) map[string]string {
		settings := map[string]string{}
		for k, v := range smtp {
			settings[k] = v
		}
		for k, v := range extra {
			settings[k] = v
		}
		return settings
	}

	t.Run("setting off keeps the default flow", func(t *testing.T) {
		svc := newAuthService(&userRepoStub{}, withSettings(nil), &keyedEmailCacheStub{}, nil)
		require.False(t, svc.RequiresAdminLoginEmailCode(ctx, admin))
	})

	t.Run("setting on requires a code for admins only", func(t *testing.T) {
		svc := newAuthService(&userRepoStub{}, withSettings(map[string]string{
			SettingKeyAdminLoginEmailCodeEnabled: "true",
		}), &keyedEmailCacheStub{}, nil)
		require.True(t, svc.RequiresAdminLoginEmailCode(ctx, admin))
		require.False(t, svc.RequiresAdminLoginEmailCode(ctx, user))
		require.False(t, svc.RequiresAdminLoginEmailCode(ctx, nil))
	})

	t.Run("setting on without SMTP falls back to the default flow", func(t *testing.T) {
		svc := newAuthService(&userRepoStub{}, map[string]string{
			SettingKeyAdminLoginEmailCodeEnabled: "true",
		}, &keyedEmailCacheStub{}, nil)
		require.False(t, svc.RequiresAdminLoginEmailCode(ctx, admin))
	})

	t.Run("no email service falls back to the default flow", func(t *testing.T) {
		svc := newAuthService(&userRepoStub{}, withSettings(map[string]string{
			SettingKeyAdminLoginEmailCodeEnabled: "true",
		}), nil, nil)
		require.False(t, svc.RequiresAdminLoginEmailCode(ctx, admin))
	})
}

func TestAdminLoginEmailCodeUsesSeparateNamespace(t *testing.T) {
	ctx := context.Background()
	cache := &keyedEmailCacheStub{}
	svc := NewEmailService(&settingRepoStub{values: map[string]string{}}, cache)
	email := "admin@example.com"
	now := time.Now()

	// A registration code (unprefixed key) must not complete an admin login.
	require.NoError(t, cache.SetVerificationCode(ctx, email, &VerificationCodeData{
		Code: "111111", CreatedAt: now, ExpiresAt: now.Add(verifyCodeTTL),
	}, verifyCodeTTL))
	require.ErrorIs(t, svc.VerifyAdminLoginCode(ctx, email, "111111"), ErrInvalidVerifyCode)

	// An admin login code must not satisfy registration verification.
	require.NoError(t, cache.SetVerificationCode(ctx, adminLoginVerifyCodeKeyPrefix+email, &VerificationCodeData{
		Code: "222222", CreatedAt: now, ExpiresAt: now.Add(verifyCodeTTL),
	}, verifyCodeTTL))
	require.ErrorIs(t, svc.VerifyCode(ctx, email, "222222"), ErrInvalidVerifyCode)

	// Wrong code is rejected and the stored code is kept; the right one is consumed.
	require.ErrorIs(t, svc.VerifyAdminLoginCode(ctx, email, "000000"), ErrInvalidVerifyCode)
	require.NoError(t, svc.VerifyAdminLoginCode(ctx, email, "222222"))
	require.Nil(t, cache.codes[adminLoginVerifyCodeKeyPrefix+email])
	require.ErrorIs(t, svc.VerifyAdminLoginCode(ctx, email, "222222"), ErrInvalidVerifyCode)

	// The registration code is untouched.
	require.NotNil(t, cache.codes[email])
}

func TestAdminLoginEmailCodeMaxAttempts(t *testing.T) {
	ctx := context.Background()
	cache := &keyedEmailCacheStub{}
	svc := NewEmailService(&settingRepoStub{values: map[string]string{}}, cache)
	email := "admin@example.com"
	now := time.Now()
	require.NoError(t, cache.SetVerificationCode(ctx, adminLoginVerifyCodeKeyPrefix+email, &VerificationCodeData{
		Code: "222222", CreatedAt: now, ExpiresAt: now.Add(verifyCodeTTL),
	}, verifyCodeTTL))

	for i := 0; i < maxVerifyCodeAttempts-1; i++ {
		require.ErrorIs(t, svc.VerifyAdminLoginCode(ctx, email, "000000"), ErrInvalidVerifyCode)
	}
	require.ErrorIs(t, svc.VerifyAdminLoginCode(ctx, email, "000000"), ErrVerifyCodeMaxAttempts)
	require.ErrorIs(t, svc.VerifyAdminLoginCode(ctx, email, "222222"), ErrVerifyCodeMaxAttempts)
}

func TestSendAdminLoginEmailCodeHonoursCooldown(t *testing.T) {
	ctx := context.Background()
	cache := &keyedEmailCacheStub{}
	svc := NewEmailService(&settingRepoStub{values: map[string]string{}}, cache)
	email := "admin@example.com"

	require.NoError(t, cache.SetVerificationCode(ctx, adminLoginVerifyCodeKeyPrefix+email, &VerificationCodeData{
		Code: "222222", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(verifyCodeTTL),
	}, verifyCodeTTL))
	require.ErrorIs(t, svc.SendAdminLoginVerifyCode(ctx, email, "Sub2API"), ErrVerifyCodeTooFrequent)
	// The pending code is not replaced during the cooldown.
	require.Equal(t, "222222", cache.codes[adminLoginVerifyCodeKeyPrefix+email].Code)

	// A recent registration code does not put the admin login code on cooldown.
	cache.codes = map[string]*VerificationCodeData{
		email: {Code: "111111", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(verifyCodeTTL)},
	}
	err := svc.SendAdminLoginVerifyCode(ctx, email, "Sub2API")
	require.NotErrorIs(t, err, ErrVerifyCodeTooFrequent)
	require.NotNil(t, cache.codes[adminLoginVerifyCodeKeyPrefix+email])
	require.Equal(t, "111111", cache.codes[email].Code)
}

func TestCreateEmailCodeLoginSession(t *testing.T) {
	ctx := context.Background()
	cache := &adminEmailCodeTotpCacheStub{sessions: map[string]*TotpLoginSession{}}
	svc := &TotpService{cache: cache}

	token, err := svc.CreateEmailCodeLoginSession(ctx, 7, "admin@example.com", false)
	require.NoError(t, err)
	session := cache.sessions[token]
	require.NotNil(t, session)
	require.True(t, session.EmailCode)
	require.False(t, session.AllowTotp)
	require.Equal(t, int64(7), session.UserID)

	token, err = svc.CreateEmailCodeLoginSession(ctx, 7, "admin@example.com", true)
	require.NoError(t, err)
	require.True(t, cache.sessions[token].AllowTotp)
}

type adminEmailCodeTotpCacheStub struct {
	TotpCache
	sessions map[string]*TotpLoginSession
}

func (s *adminEmailCodeTotpCacheStub) SetLoginSession(_ context.Context, token string, session *TotpLoginSession, _ time.Duration) error {
	s.sessions[token] = session
	return nil
}
