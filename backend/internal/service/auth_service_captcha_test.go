//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newAuthServiceForCaptchaRepoTest(repo *settingRepoStub, required bool, turnstileVerifier TurnstileVerifier, tencentVerifier TencentCaptchaVerifier) *AuthService {
	cfg := &config.Config{
		Server:    config.ServerConfig{Mode: "release"REDACTED,
		Turnstile: config.TurnstileConfig{Required: requiredREDACTED,
REDACTED
	settingService := NewSettingService(repo, cfg)
	turnstileService := NewTurnstileService(settingService, turnstileVerifier)
	tencentService := NewTencentCaptchaService(settingService, tencentVerifier)
	svc := NewAuthService(nil, &userRepoStub{REDACTED, nil, nil, cfg, settingService, nil, turnstileService, nil, nil, nil, nil, nil)
	svc.SetTencentCaptchaService(tencentService)
	return svc
REDACTED

func newAuthServiceForCaptchaTest(settings map[string]string, required bool, turnstileVerifier TurnstileVerifier, tencentVerifier TencentCaptchaVerifier) *AuthService {
	cfg := &config.Config{
		Server:    config.ServerConfig{Mode: "release"REDACTED,
		Turnstile: config.TurnstileConfig{Required: requiredREDACTED,
REDACTED
	settingService := NewSettingService(&settingRepoStub{values: settingsREDACTED, cfg)
	var turnstileService *TurnstileService
	if turnstileVerifier != nil {
		turnstileService = NewTurnstileService(settingService, turnstileVerifier)
REDACTED
	svc := NewAuthService(nil, &userRepoStub{REDACTED, nil, nil, cfg, settingService, nil, turnstileService, nil, nil, nil, nil, nil)
	if tencentVerifier != nil {
		svc.SetTencentCaptchaService(NewTencentCaptchaService(settingService, tencentVerifier))
REDACTED
	return svc
REDACTED

func tencentCaptchaSettings() map[string]string {
	return map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          "123456789",
		SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
		SettingKeyTencentCaptchaCloudSecretID:  "cloud-secret-id",
		SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret-key",
REDACTED
REDACTED

func TestVerifyCaptchaUsesTencentWhenEnabled(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTEDREDACTED
	svc := newAuthServiceForCaptchaTest(tencentCaptchaSettings(), false, nil, verifier)

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
REDACTED, "203.0.113.10")

REDACTED
	require.Equal(t, 1, verifier.calls)
REDACTED

func TestVerifyCaptchaRejectsDirtyDoubleEnabledSettings(t *testing.T) {
	settings := tencentCaptchaSettings()
	settings[SettingKeyTurnstileEnabled] = "true"
	settings[SettingKeyTurnstileSecretKey] = "turnstile-secret"
	turnstileVerifier := &turnstileVerifierSpy{REDACTED
	tencentVerifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTEDREDACTED
	svc := newAuthServiceForCaptchaTest(settings, false, turnstileVerifier, tencentVerifier)

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{
		TurnstileToken: "turnstile-token",
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
REDACTED, "203.0.113.10")

	require.ErrorIs(t, err, ErrCaptchaProviderConflict)
	require.Zero(t, turnstileVerifier.called)
	require.Zero(t, tencentVerifier.calls)
REDACTED

func TestVerifyCaptchaRequiredModeAcceptsCompleteTencentProvider(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTEDREDACTED
	svc := newAuthServiceForCaptchaTest(tencentCaptchaSettings(), true, nil, verifier)

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
REDACTED, "203.0.113.10")

REDACTED
REDACTED

func TestVerifyCaptchaForRegisterSkipsDuplicateTencentTicketAfterEmailCode(t *testing.T) {
	settings := tencentCaptchaSettings()
	settings[SettingKeyEmailVerifyEnabled] = "true"
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTEDREDACTED
	svc := newAuthServiceForCaptchaTest(settings, true, nil, verifier)

	err := svc.VerifyCaptchaForRegister(context.Background(), CaptchaProof{REDACTED, "203.0.113.10", "123456")

REDACTED
	require.Zero(t, verifier.calls)
REDACTED

func TestVerifyCaptchaFailsClosedWhenProviderSettingsCannotBeRead(t *testing.T) {
	repo := &settingRepoStub{err: errors.New("settings unavailable")REDACTED
	svc := newAuthServiceForCaptchaRepoTest(repo, false, &turnstileVerifierSpy{REDACTED, &tencentCaptchaVerifierStub{REDACTED)

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{REDACTED, "203.0.113.10")

	require.ErrorIs(t, err, ErrServiceUnavailable)
REDACTED

func TestVerifyCaptchaReadsProviderConfigurationOnce(t *testing.T) {
	repo := &settingRepoStub{values: tencentCaptchaSettings()REDACTED
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTEDREDACTED
	svc := newAuthServiceForCaptchaRepoTest(repo, false, &turnstileVerifierSpy{REDACTED, verifier)

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
REDACTED, "203.0.113.10")

REDACTED
	require.Equal(t, 1, repo.getMultipleCalls)
	require.Zero(t, repo.getValueCalls)
	require.Equal(t, 1, verifier.calls)
REDACTED

func TestVerifyCaptchaRejectsEnabledTencentProviderWithIncompleteCredentials(t *testing.T) {
	repo := &settingRepoStub{values: map[string]string{
		SettingKeyTencentCaptchaEnabled: "true",
		SettingKeyTencentCaptchaAppID:   "123456789",
REDACTEDREDACTED
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTEDREDACTED
	svc := newAuthServiceForCaptchaRepoTest(repo, false, &turnstileVerifierSpy{REDACTED, verifier)

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
REDACTED, "203.0.113.10")

	require.ErrorIs(t, err, ErrTencentCaptchaNotConfigured)
	require.Equal(t, 1, repo.getMultipleCalls)
	require.Zero(t, verifier.calls)
REDACTED

func TestVerifyTencentCaptchaIfEnabledVerifiesTencentProof(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTEDREDACTED
	svc := newAuthServiceForCaptchaTest(tencentCaptchaSettings(), false, nil, verifier)

	err := svc.VerifyTencentCaptchaIfEnabled(context.Background(), CaptchaProof{
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
REDACTED, "203.0.113.10")

REDACTED
	require.Equal(t, 1, verifier.calls)
	require.Equal(t, TencentCaptchaProof{Ticket: "ticket", Randstr: "@rand"REDACTED, verifier.proof)
REDACTED

func TestVerifyTencentCaptchaIfEnabledDoesNotExpandTurnstileCoverage(t *testing.T) {
	settings := map[string]string{
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "turnstile-secret",
REDACTED
	turnstileVerifier := &turnstileVerifierSpy{REDACTED
	svc := newAuthServiceForCaptchaTest(settings, false, turnstileVerifier, nil)

	err := svc.VerifyTencentCaptchaIfEnabled(context.Background(), CaptchaProof{REDACTED, "203.0.113.10")

REDACTED
	require.Zero(t, turnstileVerifier.called)
REDACTED

func TestVerifyTencentCaptchaIfEnabledFailsClosedOnSettingReadError(t *testing.T) {
	repo := &settingRepoStub{err: errors.New("settings unavailable")REDACTED
	svc := newAuthServiceForCaptchaRepoTest(repo, false, &turnstileVerifierSpy{REDACTED, &tencentCaptchaVerifierStub{REDACTED)

	err := svc.VerifyTencentCaptchaIfEnabled(context.Background(), CaptchaProof{REDACTED, "203.0.113.10")

	require.ErrorIs(t, err, ErrServiceUnavailable)
REDACTED
