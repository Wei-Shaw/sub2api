//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type tencentCaptchaVerifierStub struct {
	response *TencentCaptchaVerifyResponse
	err      error
	calls    int
	proof    TencentCaptchaProof
	remoteIP string
REDACTED

func (s *tencentCaptchaVerifierStub) VerifyTicket(_ context.Context, _ TencentCaptchaCredentials, proof TencentCaptchaProof, remoteIP string) (*TencentCaptchaVerifyResponse, error) {
	s.calls++
	s.proof = proof
	s.remoteIP = remoteIP
	return s.response, s.err
REDACTED

func newTencentCaptchaTestService(verifier TencentCaptchaVerifier) *TencentCaptchaService {
	settings := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          "123456789",
		SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
		SettingKeyTencentCaptchaCloudSecretID:  "cloud-secret-id",
		SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret-key",
REDACTEDREDACTED, &config.Config{REDACTED)
	return NewTencentCaptchaService(settings, verifier)
REDACTED

func TestTencentCaptchaServiceAcceptsCaptchaCodeOne(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTEDREDACTED
	svc := newTencentCaptchaTestService(verifier)

	err := svc.VerifyTicket(context.Background(), "ticket", "@rand", "203.0.113.10")

REDACTED
	require.Equal(t, 1, verifier.calls)
	require.Equal(t, TencentCaptchaProof{Ticket: "ticket", Randstr: "@rand"REDACTED, verifier.proof)
	require.Equal(t, "203.0.113.10", verifier.remoteIP)
REDACTED

func TestTencentCaptchaServiceRejectsDisasterRecoveryTicketWithoutCallingVerifier(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTEDREDACTED
	svc := newTencentCaptchaTestService(verifier)

	err := svc.VerifyTicket(context.Background(), "trerror_1001_123456789_1", "@rand", "203.0.113.10")

	require.ErrorIs(t, err, ErrTencentCaptchaVerificationFailed)
	require.Zero(t, verifier.calls)
REDACTED

func TestTencentCaptchaServiceRejectsEveryNonOneCode(t *testing.T) {
	for _, code := range []int64{0, 7, 8, 9, 15, 16, 21, 100REDACTED {
		t.Run(string(rune(code)), func(t *testing.T) {
			verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: codeREDACTEDREDACTED
			svc := newTencentCaptchaTestService(verifier)

			err := svc.VerifyTicket(context.Background(), "ticket", "@rand", "203.0.113.10")

			require.ErrorIs(t, err, ErrTencentCaptchaVerificationFailed)
	REDACTED)
REDACTED
REDACTED

func TestTencentCaptchaServiceFailsClosedOnVerifierError(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{err: errors.New("sdk unavailable")REDACTED
	svc := newTencentCaptchaTestService(verifier)

	err := svc.VerifyTicket(context.Background(), "ticket", "@rand", "203.0.113.10")

REDACTED
	require.ErrorIs(t, err, ErrTencentCaptchaVerificationFailed)
REDACTED

func TestTencentCaptchaServiceRejectsIncompleteConfiguration(t *testing.T) {
	settings := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyTencentCaptchaEnabled: "true",
		SettingKeyTencentCaptchaAppID:   "123456789",
REDACTEDREDACTED, &config.Config{REDACTED)
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTEDREDACTED
	svc := NewTencentCaptchaService(settings, verifier)

	err := svc.VerifyTicket(context.Background(), "ticket", "@rand", "203.0.113.10")

	require.ErrorIs(t, err, ErrTencentCaptchaNotConfigured)
	require.Zero(t, verifier.calls)
REDACTED

func TestTencentCaptchaServiceFailsClosedOnSettingsReadError(t *testing.T) {
	settings := NewSettingService(&settingPublicRepoStub{err: errors.New("settings unavailable")REDACTED, &config.Config{REDACTED)
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTEDREDACTED
	svc := NewTencentCaptchaService(settings, verifier)

	err := svc.VerifyTicket(context.Background(), "ticket", "@rand", "203.0.113.10")

	require.ErrorIs(t, err, ErrServiceUnavailable)
	require.Zero(t, verifier.calls)
REDACTED
