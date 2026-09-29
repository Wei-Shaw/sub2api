//go:build unit

package service

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
)

const b05TotpSecret = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"

type b05UserRepoStub struct {
	UserRepository
	user *User
}

func (s *b05UserRepoStub) GetByID(ctx context.Context, id int64) (*User, error) {
	return s.user, nil
}

func (s *b05UserRepoStub) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	return nil
}

func (s *b05UserRepoStub) EnableTotp(ctx context.Context, userID int64) error {
	return nil
}

// b05EncryptorStub 加密仅加前缀，解密固定返回测试密钥。
type b05EncryptorStub struct{}

func (b05EncryptorStub) Encrypt(plaintext string) (string, error)  { return "enc:" + plaintext, nil }
func (b05EncryptorStub) Decrypt(ciphertext string) (string, error) { return b05TotpSecret, nil }

type b05TotpCacheStub struct {
	TotpCache
}

func (b05TotpCacheStub) GetVerifyAttempts(ctx context.Context, userID int64) (int, error) {
	return 0, nil
}

func (b05TotpCacheStub) IncrementVerifyAttempts(ctx context.Context, userID int64) (int, error) {
	return 1, nil
}

func (b05TotpCacheStub) ClearVerifyAttempts(ctx context.Context, userID int64) error { return nil }

func (b05TotpCacheStub) GetSetupSession(ctx context.Context, userID int64) (*TotpSetupSession, error) {
	return &TotpSetupSession{Secret: b05TotpSecret, SetupToken: "b05-setup-token", CreatedAt: time.Now()}, nil
}

func (b05TotpCacheStub) DeleteSetupSession(ctx context.Context, userID int64) error { return nil }

func b05CaptureDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func b05NewTotpService(user *User) *TotpService {
	settingSvc := NewSettingService(&totpVMSettingRepoStub{values: map[string]string{SettingKeyTotpEnabled: "true"}}, nil)
	return NewTotpService(&b05UserRepoStub{user: user}, b05EncryptorStub{}, b05TotpCacheStub{}, settingSvc, nil, nil)
}

func TestTotpVerifyCodeDebugLogDoesNotLeakSecretB05(t *testing.T) {
	buf := b05CaptureDebugLogs(t)
	enc := "enc"
	svc := b05NewTotpService(&User{ID: 7, TotpEnabled: true, TotpSecretEncrypted: &enc})

	code, err := totp.GenerateCode(b05TotpSecret, time.Now())
	require.NoError(t, err)
	require.NoError(t, svc.VerifyCode(context.Background(), 7, code))

	out := buf.String()
	require.Contains(t, out, "totp_verify_result")
	require.NotContains(t, out, b05TotpSecret[:4])
	require.NotContains(t, out, "secret_prefix")
}

func TestTotpCompleteSetupDebugLogDoesNotLeakSecretB05(t *testing.T) {
	buf := b05CaptureDebugLogs(t)
	svc := b05NewTotpService(&User{ID: 8})

	code, err := totp.GenerateCode(b05TotpSecret, time.Now())
	require.NoError(t, err)
	require.NoError(t, svc.CompleteSetup(context.Background(), 8, code, "b05-setup-token"))

	out := buf.String()
	require.Contains(t, out, "totp_complete_setup_verified")
	require.NotContains(t, out, b05TotpSecret[:4])
	require.NotContains(t, out, "secret_prefix")
	require.NotContains(t, out, "decrypted_prefix")
}
