//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type aliyunVerifierSpy struct {
	called    int
	lastCred  AliyunCaptchaCredentials
	lastParam string
	result    *AliyunCaptchaVerifyResult
	err       error
REDACTED

func (s *aliyunVerifierSpy) VerifyCaptcha(_ context.Context, cred AliyunCaptchaCredentials, param string) (*AliyunCaptchaVerifyResult, error) {
	s.called++
	s.lastCred = cred
	s.lastParam = param
	if s.err != nil {
		return nil, s.err
REDACTED
	if s.result != nil {
		return s.result, nil
REDACTED
	return &AliyunCaptchaVerifyResult{VerifyResult: trueREDACTED, nil
REDACTED

func aliyunEnabledSettings() map[string]string {
	return map[string]string{
		SettingKeyAliyunCaptchaEnabled:         "true",
		SettingKeyAliyunCaptchaAccessKeyID:     "ak-id",
		SettingKeyAliyunCaptchaAccessKeySecret: "ak-secret",
		SettingKeyAliyunCaptchaSceneID:         "scene-1",
		SettingKeyAliyunCaptchaPrefix:          "prefix-1",
REDACTED
REDACTED

func aliyunTestConfig() AliyunCaptchaConfig {
	return AliyunCaptchaConfig{
		Enabled:         true,
		AccessKeyID:     "ak-id",
		AccessKeySecret: "ak-secret",
		SceneID:         "scene-1",
		Region:          AliyunCaptchaRegionCN,
REDACTED
REDACTED

func newAliyunAuthServiceForTest(cfg *config.Config, settings map[string]string, aliyunSpy *aliyunVerifierSpy) *AuthService {
	settingService := NewSettingService(&settingPublicRepoStub{values: settingsREDACTED, cfg)
	authService := NewAuthService(
		nil, // entClient
		nil, // userRepo
		nil, // redeemRepo
		nil, // refreshTokenCache
		cfg,
		settingService,
		nil, // emailService
		NewTurnstileService(settingService, &turnstileVerifierSpy{REDACTED),
		nil, // emailQueueService
		nil, // promoService
		nil, // defaultSubAssigner
		nil, // affiliateService
		nil, // userPlatformQuotaRepo
	)
	authService.SetAliyunCaptchaService(NewAliyunCaptchaService(settingService, aliyunSpy))
	return authService
REDACTED

func TestAliyunCaptchaServiceVerifyParamDispatch(t *testing.T) {
	spy := &aliyunVerifierSpy{REDACTED
	svc := NewAliyunCaptchaService(nil, spy)

	err := svc.VerifyParamWithConfig(context.Background(), aliyunTestConfig(), "captcha-verify-param")

REDACTED
	require.Equal(t, 1, spy.called)
	require.Equal(t, "captcha-verify-param", spy.lastParam)
	require.Equal(t, "ak-id", spy.lastCred.AccessKeyID)
	require.Equal(t, "scene-1", spy.lastCred.SceneID)
	require.Equal(t, "captcha.cn-shanghai.aliyuncs.com", spy.lastCred.Endpoint)
REDACTED

func TestAliyunCaptchaServiceSgpEndpoint(t *testing.T) {
	spy := &aliyunVerifierSpy{REDACTED
	svc := NewAliyunCaptchaService(nil, spy)
	cfg := aliyunTestConfig()
	cfg.Region = AliyunCaptchaRegionSGP

	err := svc.VerifyParamWithConfig(context.Background(), cfg, "captcha-verify-param")

REDACTED
	require.Equal(t, "captcha.ap-southeast-1.aliyuncs.com", spy.lastCred.Endpoint)
REDACTED

func TestAliyunCaptchaServiceFailsClosedOnVerifierError(t *testing.T) {
	spy := &aliyunVerifierSpy{err: errors.New("network down")REDACTED
	svc := NewAliyunCaptchaService(nil, spy)

	err := svc.VerifyParamWithConfig(context.Background(), aliyunTestConfig(), "captcha-verify-param")

	require.ErrorIs(t, err, ErrAliyunCaptchaVerificationFailed)
REDACTED

func TestAliyunCaptchaServiceRejectsVerifyResultFalse(t *testing.T) {
	spy := &aliyunVerifierSpy{result: &AliyunCaptchaVerifyResult{VerifyResult: false, VerifyCode: "F001"REDACTEDREDACTED
	svc := NewAliyunCaptchaService(nil, spy)

	err := svc.VerifyParamWithConfig(context.Background(), aliyunTestConfig(), "captcha-verify-param")

	require.ErrorIs(t, err, ErrAliyunCaptchaVerificationFailed)
REDACTED

func TestAliyunCaptchaServiceRejectsIncompleteCredentials(t *testing.T) {
	spy := &aliyunVerifierSpy{REDACTED
	svc := NewAliyunCaptchaService(nil, spy)
	cfg := aliyunTestConfig()
	cfg.AccessKeySecret = ""

	err := svc.VerifyParamWithConfig(context.Background(), cfg, "captcha-verify-param")

	require.ErrorIs(t, err, ErrAliyunCaptchaNotConfigured)
	require.Zero(t, spy.called)
REDACTED

func TestAliyunCaptchaServiceRejectsEmptyParam(t *testing.T) {
	spy := &aliyunVerifierSpy{REDACTED
	svc := NewAliyunCaptchaService(nil, spy)

	err := svc.VerifyParamWithConfig(context.Background(), aliyunTestConfig(), "")

	require.ErrorIs(t, err, ErrAliyunCaptchaVerificationFailed)
	require.Zero(t, spy.called)
REDACTED

func TestAliyunCaptchaServiceValidateCredentials(t *testing.T) {
	t.Run("invalid credential code", func(t *testing.T) {
		spy := &aliyunVerifierSpy{err: &AliyunCaptchaAPIError{Code: "SignatureDoesNotMatch", Message: "bad sk"REDACTEDREDACTED
		svc := NewAliyunCaptchaService(nil, spy)

		err := svc.ValidateCredentials(context.Background(), "id", "sk", "scene", "cn")
		require.ErrorIs(t, err, ErrCaptchaInvalidCredentials)
REDACTED)

	t.Run("network error surfaces", func(t *testing.T) {
		spy := &aliyunVerifierSpy{err: errors.New("timeout")REDACTED
		svc := NewAliyunCaptchaService(nil, spy)

		err := svc.ValidateCredentials(context.Background(), "id", "sk", "scene", "cn")
	REDACTED
		require.NotErrorIs(t, err, ErrCaptchaInvalidCredentials)
REDACTED)

	t.Run("verify result false means credentials valid", func(t *testing.T) {
		spy := &aliyunVerifierSpy{result: &AliyunCaptchaVerifyResult{VerifyResult: falseREDACTEDREDACTED
		svc := NewAliyunCaptchaService(nil, spy)

		err := svc.ValidateCredentials(context.Background(), "id", "sk", "scene", "sgp")
	REDACTED
		require.Equal(t, "captcha.ap-southeast-1.aliyuncs.com", spy.lastCred.Endpoint)
REDACTED)
REDACTED

func TestAuthServiceVerifyCaptchaDispatchesAliyun(t *testing.T) {
	spy := &aliyunVerifierSpy{REDACTED
	authService := newAliyunAuthServiceForTest(&config.Config{REDACTED, aliyunEnabledSettings(), spy)

	// 阿里云 captchaVerifyParam 复用 turnstile_token 请求字段
	err := authService.VerifyCaptcha(context.Background(), CaptchaProof{TurnstileToken: "captcha-verify-param"REDACTED, "127.0.0.1")

REDACTED
	require.Equal(t, 1, spy.called)
	require.Equal(t, "captcha-verify-param", spy.lastParam)
REDACTED

func TestAuthServiceVerifyCaptchaRejectsProviderConflict(t *testing.T) {
	settings := aliyunEnabledSettings()
	settings[SettingKeyTurnstileEnabled] = "true"
	settings[SettingKeyTurnstileSecretKey] = "secret"
	spy := &aliyunVerifierSpy{REDACTED
	authService := newAliyunAuthServiceForTest(&config.Config{REDACTED, settings, spy)

	err := authService.VerifyCaptcha(context.Background(), CaptchaProof{TurnstileToken: "param"REDACTED, "127.0.0.1")

	require.ErrorIs(t, err, ErrCaptchaProviderConflict)
	require.Zero(t, spy.called)
REDACTED

func TestAuthServiceVerifyCaptchaRequiredModeWithAliyun(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{Mode: "release"REDACTED,
		Turnstile: config.TurnstileConfig{Required: trueREDACTED,
REDACTED
	spy := &aliyunVerifierSpy{REDACTED
	authService := newAliyunAuthServiceForTest(cfg, aliyunEnabledSettings(), spy)

	// required 模式 + 阿里云启用且凭证齐全：不误报 NOT_CONFIGURED，正常走阿里云校验
	err := authService.VerifyCaptcha(context.Background(), CaptchaProof{TurnstileToken: "captcha-verify-param"REDACTED, "127.0.0.1")

REDACTED
	require.Equal(t, 1, spy.called)
REDACTED

func TestAuthServiceVerifyActionCaptchaIfEnabledDispatchesAliyun(t *testing.T) {
	spy := &aliyunVerifierSpy{REDACTED
	authService := newAliyunAuthServiceForTest(&config.Config{REDACTED, aliyunEnabledSettings(), spy)

	err := authService.VerifyActionCaptchaIfEnabled(context.Background(), CaptchaProof{TurnstileToken: "captcha-verify-param"REDACTED, "127.0.0.1")

REDACTED
	require.Equal(t, 1, spy.called)
	require.Equal(t, "captcha-verify-param", spy.lastParam)
REDACTED

func TestAuthServiceVerifyActionCaptchaIfEnabledSkipsWhenOnlyTurnstile(t *testing.T) {
	spy := &aliyunVerifierSpy{REDACTED
	authService := newAliyunAuthServiceForTest(&config.Config{REDACTED, map[string]string{
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "secret",
REDACTED, spy)

	// Turnstile 不扩大既有覆盖：扩展入口不拦截
	err := authService.VerifyActionCaptchaIfEnabled(context.Background(), CaptchaProof{REDACTED, "127.0.0.1")

REDACTED
	require.Zero(t, spy.called)
REDACTED
