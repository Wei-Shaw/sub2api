//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
)

type totpSecurityUserRepo struct {
	UserRepository
	user *User
}

func (r *totpSecurityUserRepo) GetByID(context.Context, int64) (*User, error) {
	if r.user == nil {
		return nil, errors.New("fake user missing")
	}
	return r.user, nil
}

func (r *totpSecurityUserRepo) UpdateTotpSecret(_ context.Context, _ int64, encryptedSecret *string) error {
	r.user.TotpSecretEncrypted = encryptedSecret
	return nil
}

func (r *totpSecurityUserRepo) EnableTotp(context.Context, int64) error {
	r.user.TotpEnabled = true
	now := time.Now()
	r.user.TotpEnabledAt = &now
	return nil
}

func (r *totpSecurityUserRepo) DisableTotp(context.Context, int64) error {
	r.user.TotpEnabled = false
	r.user.TotpEnabledAt = nil
	r.user.TotpSecretEncrypted = nil
	return nil
}

type totpSecurityCache struct {
	setupSession     *TotpSetupSession
	loginSession     *TotpLoginSession
	getAttemptsErr   error
	incrementErr     error
	clearAttemptsErr error
	attempts         int
	grantGeneration  string
	grantSessionKey  string
}

func (c *totpSecurityCache) GetSetupSession(context.Context, int64) (*TotpSetupSession, error) {
	return c.setupSession, nil
}

func (c *totpSecurityCache) SetSetupSession(_ context.Context, _ int64, session *TotpSetupSession, _ time.Duration) error {
	c.setupSession = session
	return nil
}

func (c *totpSecurityCache) DeleteSetupSession(context.Context, int64) error {
	c.setupSession = nil
	return nil
}

func (c *totpSecurityCache) GetLoginSession(context.Context, string) (*TotpLoginSession, error) {
	return c.loginSession, nil
}

func (c *totpSecurityCache) SetLoginSession(_ context.Context, _ string, session *TotpLoginSession, _ time.Duration) error {
	c.loginSession = session
	return nil
}

func (c *totpSecurityCache) ConsumeLoginSession(context.Context, string) (*TotpLoginSession, error) {
	session := c.loginSession
	c.loginSession = nil
	return session, nil
}

func (c *totpSecurityCache) DeleteLoginSession(context.Context, string) error {
	c.loginSession = nil
	return nil
}

func (c *totpSecurityCache) IncrementVerifyAttempts(context.Context, int64) (int, error) {
	if c.incrementErr != nil {
		return 0, c.incrementErr
	}
	c.attempts++
	return c.attempts, nil
}

func (c *totpSecurityCache) GetVerifyAttempts(context.Context, int64) (int, error) {
	if c.getAttemptsErr != nil {
		return 0, c.getAttemptsErr
	}
	return c.attempts, nil
}

func (c *totpSecurityCache) ClearVerifyAttempts(context.Context, int64) error {
	if c.clearAttemptsErr != nil {
		return c.clearAttemptsErr
	}
	c.attempts = 0
	return nil
}

func (c *totpSecurityCache) SetStepUpGrant(_ context.Context, _ int64, sessionKey, credentialGeneration string, _ time.Duration) error {
	c.grantSessionKey = sessionKey
	c.grantGeneration = credentialGeneration
	return nil
}

func (c *totpSecurityCache) HasStepUpGrant(_ context.Context, _ int64, sessionKey, credentialGeneration string) (bool, error) {
	return c.grantSessionKey == sessionKey && c.grantGeneration == credentialGeneration, nil
}

type totpSecurityEncryptor struct {
	plainToCipher map[string]string
	cipherToPlain map[string]string
}

func (e totpSecurityEncryptor) Encrypt(plaintext string) (string, error) {
	if ciphertext, ok := e.plainToCipher[plaintext]; ok {
		return ciphertext, nil
	}
	return "", errors.New("fake plaintext not configured")
}

func (e totpSecurityEncryptor) Decrypt(ciphertext string) (string, error) {
	if plaintext, ok := e.cipherToPlain[ciphertext]; ok {
		return plaintext, nil
	}
	return "", errors.New("fake ciphertext not configured")
}

type totpSecuritySettingRepo struct {
	SettingRepository
}

func (totpSecuritySettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if key == SettingKeyTotpEnabled {
		return "true", nil
	}
	return "", errors.New("fake setting missing")
}

func newTotpSecurityService(user *User, cache *totpSecurityCache, encryptor SecretEncryptor) *TotpService {
	return NewTotpService(
		&totpSecurityUserRepo{user: user},
		encryptor,
		cache,
		NewSettingService(totpSecuritySettingRepo{}, nil),
		nil,
		nil,
	)
}

func differentTotpCode(valid string) string {
	first := byte('0')
	if valid[0] == '0' {
		first = '1'
	}
	return string(first) + valid[1:]
}

func TestTotpSecretAndPrefixNeverAppearInDebugLogs(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"
	const ciphertext = "fake-encrypted-totp-secret"
	cache := &totpSecurityCache{setupSession: &TotpSetupSession{
		Secret:     secret,
		SetupToken: "fake-setup-token",
		CreatedAt:  time.Now(),
	}}
	user := &User{ID: 42, Email: "local@example.test", Role: RoleAdmin}
	svc := newTotpSecurityService(user, cache, totpSecurityEncryptor{
		plainToCipher: map[string]string{secret: ciphertext},
		cipherToPlain: map[string]string{ciphertext: secret},
	})

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	require.NoError(t, svc.CompleteSetup(context.Background(), user.ID, code, "fake-setup-token"))
	require.NoError(t, svc.VerifyCode(context.Background(), user.ID, code))

	output := logs.String()
	require.Contains(t, output, "totp_complete_setup_before_encrypt")
	require.Contains(t, output, "totp_verify_result")
	require.NotContains(t, output, secret)
	require.NotContains(t, output, secret[:4])
	require.NotContains(t, output, "secret_prefix")
	require.NotContains(t, output, "decrypted_prefix")
}

func TestTotpVerifyCodeRateLimitStorageErrorsFailClosed(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"
	const ciphertext = "fake-ciphertext"
	validCode, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	user := &User{ID: 42, TotpEnabled: true, TotpSecretEncrypted: stringPointer(ciphertext)}
	encryptor := totpSecurityEncryptor{cipherToPlain: map[string]string{ciphertext: secret}}

	tests := []struct {
		name  string
		cache *totpSecurityCache
		code  string
	}{
		{name: "attempt counter read", cache: &totpSecurityCache{getAttemptsErr: errors.New("fake redis read failure")}, code: validCode},
		{name: "attempt counter increment", cache: &totpSecurityCache{incrementErr: errors.New("fake redis increment failure")}, code: differentTotpCode(validCode)},
		{name: "attempt counter clear", cache: &totpSecurityCache{clearAttemptsErr: errors.New("fake redis clear failure")}, code: validCode},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc := newTotpSecurityService(user, test.cache, encryptor)
			err := svc.VerifyCode(context.Background(), user.ID, test.code)
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrTotpInvalidCode)
		})
	}
}

func TestStepUpGrantCannotSurviveTotpRotationOrDisable(t *testing.T) {
	const secretA = "JBSWY3DPEHPK3PXP"
	const secretB = "KRSXG5DSNFXGOIDB"
	const cipherA = "fake-ciphertext-a"
	const cipherB = "fake-ciphertext-b"
	user := &User{ID: 42, TotpEnabled: true, TotpSecretEncrypted: stringPointer(cipherA)}
	cache := &totpSecurityCache{}
	svc := newTotpSecurityService(user, cache, totpSecurityEncryptor{cipherToPlain: map[string]string{
		cipherA: secretA,
		cipherB: secretB,
	}})

	code, err := totp.GenerateCode(secretA, time.Now())
	require.NoError(t, err)
	_, err = svc.VerifyStepUp(context.Background(), user.ID, "session-a", code)
	require.NoError(t, err)

	granted, err := svc.HasStepUpGrant(context.Background(), user.ID, "session-a")
	require.NoError(t, err)
	require.True(t, granted)

	user.TotpSecretEncrypted = stringPointer(cipherB)
	granted, err = svc.HasStepUpGrant(context.Background(), user.ID, "session-a")
	require.NoError(t, err)
	require.False(t, granted)

	user.TotpEnabled = false
	user.TotpSecretEncrypted = nil
	granted, err = svc.HasStepUpGrant(context.Background(), user.ID, "session-a")
	require.ErrorIs(t, err, ErrTotpNotSetup)
	require.False(t, granted)
}

func stringPointer(value string) *string {
	return &value
}
