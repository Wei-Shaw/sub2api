package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
)

func TestNormalizePasskeyName(t *testing.T) {
	require.Equal(t, defaultPasskeyName, normalizePasskeyName("   "))
	require.Equal(t, "Laptop", normalizePasskeyName("  Laptop  "))

	longName := strings.Repeat("密", maxPasskeyNameLength+10)
	require.Len(t, []rune(normalizePasskeyName(longName)), maxPasskeyNameLength)
REDACTED

func TestPasskeySummaryReportsCurrentBackupState(t *testing.T) {
	record := &PasskeyCredentialRecord{
		Credential: webauthn.Credential{
			Flags: webauthn.CredentialFlags{BackupEligible: trueREDACTED,
	REDACTED,
REDACTED
	require.False(t, passkeySummary(record).Backup)

	record.Credential.Flags.BackupState = true
	require.True(t, passkeySummary(record).Backup)
REDACTED

// 桩仅实现测试所需方法；未桩方法调用即 panic（嵌入 nil 接口）。
type passkeyPwUserRepoStub struct {
	UserRepository
	user *User
REDACTED

func (s *passkeyPwUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, nil
REDACTED

type passkeyPwRepoStub struct {
	PasskeyRepository
	handleCalled bool
	deleteCalled bool
REDACTED

func (s *passkeyPwRepoStub) EnsureUserHandle(_ context.Context, _ int64, candidate []byte) ([]byte, error) {
	s.handleCalled = true
	return candidate, nil
REDACTED

func (s *passkeyPwRepoStub) ListByUserID(context.Context, int64) ([]PasskeyCredentialRecord, error) {
	return nil, nil
REDACTED

func (s *passkeyPwRepoStub) Delete(context.Context, int64, int64) error {
	s.deleteCalled = true
	return nil
REDACTED

type passkeyPwSessionStoreStub struct {
	PasskeySessionStore
REDACTED

func (s *passkeyPwSessionStoreStub) Store(context.Context, *PasskeySession, time.Duration) (string, error) {
	return "session-token", nil
REDACTED

func newPasskeyPwService(t *testing.T, user *User) (*PasskeyService, *passkeyPwRepoStub) {
REDACTED
	repo := &passkeyPwRepoStub{REDACTED
	svc, err := NewPasskeyService(&config.Config{WebAuthn: config.WebAuthnConfig{
		Enabled:       true,
		RPDisplayName: "Sub2API",
		RPID:          "sub2api.example.com",
		RPOrigins:     []string{"https://sub2api.example.com"REDACTED,
REDACTEDREDACTED, repo, &passkeyPwSessionStoreStub{REDACTED, &passkeyPwUserRepoStub{user: userREDACTED)
REDACTED
	return svc, repo
REDACTED

// 注册与吊销必须验证账号密码：被窃会话不得静默添加/移除凭据。
// 用密码而非 TOTP step-up，保证未配置 TOTP 加密密钥的部署同样受保护。
func TestPasskeyEnrollmentAndRevocationRequireAccountPassword(t *testing.T) {
	user := &User{ID: 7, Email: "user@example.com", Status: StatusActiveREDACTED
	require.NoError(t, user.SetPassword("correct-password"))
	svc, repo := newPasskeyPwService(t, user)

	_, _, err := svc.BeginRegistration(context.Background(), user.ID, "")
	require.ErrorIs(t, err, ErrPasswordRequired)
	_, _, err = svc.BeginRegistration(context.Background(), user.ID, "wrong-password")
	require.ErrorIs(t, err, ErrPasswordIncorrect)
	require.False(t, repo.handleCalled)

	creation, token, err := svc.BeginRegistration(context.Background(), user.ID, "correct-password")
REDACTED
	require.NotNil(t, creation)
	require.Equal(t, "session-token", token)
	require.True(t, repo.handleCalled)

	err = svc.Delete(context.Background(), user.ID, 1, "")
	require.ErrorIs(t, err, ErrPasswordRequired)
	err = svc.Delete(context.Background(), user.ID, 1, "wrong-password")
	require.ErrorIs(t, err, ErrPasswordIncorrect)
	require.False(t, repo.deleteCalled)

	require.NoError(t, svc.Delete(context.Background(), user.ID, 1, "correct-password"))
	require.True(t, repo.deleteCalled)
REDACTED
