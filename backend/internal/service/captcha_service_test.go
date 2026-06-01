//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// captchaVerifierSpy 实现新版 CaptchaVerifier，用于驱动 CaptchaService 的单测。
//
// verifyResult / verifyErr 控制 Verify 返回；
// validateErr 控制 ValidateProviderConfig 返回；
// lastReq 暴露最后一次入参便于断言（避免在每个用例里重复打桩）。
type captchaVerifierSpy struct {
	verifyCalled int
	lastReq      VerifyRequest
	verifyResult *VerifyResult
	verifyErr    error

	validateCalled  int
	lastValidateP   string
	lastValidateCfg map[string]string
	validateErr     error
}

func (s *captchaVerifierSpy) Verify(_ context.Context, req VerifyRequest) (*VerifyResult, error) {
	s.verifyCalled++
	s.lastReq = req
	if s.verifyErr != nil {
		return nil, s.verifyErr
	}
	if s.verifyResult != nil {
		return s.verifyResult, nil
	}
	return &VerifyResult{Success: true}, nil
}

func (s *captchaVerifierSpy) ValidateProviderConfig(_ context.Context, provider string, config map[string]string) error {
	s.validateCalled++
	s.lastValidateP = provider
	s.lastValidateCfg = config
	return s.validateErr
}

func newCaptchaServiceForTest(settings map[string]string, verifier CaptchaVerifier) *CaptchaService {
	return NewCaptchaService(NewSettingService(&settingRepoStub{values: settings}, &config.Config{}), verifier)
}

func TestCaptchaService_VerifyToken_SkipWhenDisabled(t *testing.T) {
	verifier := &captchaVerifierSpy{}
	service := newCaptchaServiceForTest(map[string]string{
		SettingKeyCaptchaProvider: CaptchaProviderHcaptcha,
		SettingKeyCaptchaConfig:   `{"enabled":"false","site_key":"site","secret_key":"secret"}`,
	}, verifier)

	err := service.VerifyToken(context.Background(), "", "127.0.0.1")

	require.NoError(t, err)
	require.Equal(t, 0, verifier.verifyCalled)
}

func TestCaptchaService_VerifyToken_RequiresSecret(t *testing.T) {
	service := newCaptchaServiceForTest(map[string]string{
		SettingKeyCaptchaProvider: CaptchaProviderHcaptcha,
		SettingKeyCaptchaConfig:   `{"enabled":"true","site_key":"site"}`,
	}, &captchaVerifierSpy{})

	err := service.VerifyToken(context.Background(), "token", "127.0.0.1")

	require.ErrorIs(t, err, ErrCaptchaNotConfigured)
}

func TestCaptchaService_VerifyToken_RequiresToken(t *testing.T) {
	service := newCaptchaServiceForTest(map[string]string{
		SettingKeyCaptchaProvider: CaptchaProviderHcaptcha,
		SettingKeyCaptchaConfig:   `{"enabled":"true","site_key":"site","secret_key":"secret"}`,
	}, &captchaVerifierSpy{})

	err := service.VerifyToken(context.Background(), "", "127.0.0.1")

	require.ErrorIs(t, err, ErrCaptchaVerificationFailed)
}

func TestCaptchaService_VerifyToken_ReturnsProviderFailure(t *testing.T) {
	service := newCaptchaServiceForTest(map[string]string{
		SettingKeyCaptchaProvider: CaptchaProviderHcaptcha,
		SettingKeyCaptchaConfig:   `{"enabled":"true","site_key":"site","secret_key":"secret"}`,
	}, &captchaVerifierSpy{verifyResult: &VerifyResult{Success: false, ErrorCode: CaptchaErrInvalid}})

	err := service.VerifyToken(context.Background(), "token", "127.0.0.1")

	require.ErrorIs(t, err, ErrCaptchaVerificationFailed)
}

// TestCaptchaService_VerifyPayload_PropagatesNormalizedErrorCode 验证 §4.5 契约：
// 当 verifier 返回归一化 ErrorCode 时，CaptchaService 必须把该 ErrorCode 通过
// ApplicationError.Metadata["captcha_error_code"] 透传，方便 ResponseEnvelope 把它
// 序列化到响应 metadata，前端据此触发 fallback 重试 / 文案选择（design.md D6）。
//
// 同一 ErrorCode 在 turnstile / hcaptcha 两个 provider 下都要被透传，覆盖
// captchaVerificationFailedWithCode 中 turnstile 走 ErrTurnstileVerificationFailed、
// 其它 provider 走 ErrCaptchaVerificationFailed 的两条兼容窗口分支。
func TestCaptchaService_VerifyPayload_PropagatesNormalizedErrorCode(t *testing.T) {
	codes := []string{
		CaptchaErrInvalid,
		CaptchaErrConfig,
		CaptchaErrTimeout,
		CaptchaErrDuplicate,
		CaptchaErrNetwork,
		CaptchaErrTencentFallbackTicket,
	}
	providers := []string{CaptchaProviderTurnstile, CaptchaProviderHcaptcha}

	for _, provider := range providers {
		for _, code := range codes {
			t.Run(provider+"/"+code, func(t *testing.T) {
				service := newCaptchaServiceForTest(map[string]string{
					SettingKeyCaptchaProvider: provider,
					SettingKeyCaptchaConfig:   `{"enabled":"true","site_key":"site","secret_key":"sk"}`,
				}, &captchaVerifierSpy{verifyResult: &VerifyResult{Success: false, ErrorCode: code}})

				err := service.VerifyPayload(context.Background(), map[string]string{"token": "x"}, "1.2.3.4")
				require.Error(t, err)

				var appErr *infraerrors.ApplicationError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, code, appErr.Metadata["captcha_error_code"])
			})
		}
	}
}

// TestCaptchaService_VerifyPayload_EmptyPayloadCarriesInvalidCode 验证空 payload
// 走快速失败分支时也带 metadata.captcha_error_code = "captcha.invalid"。
func TestCaptchaService_VerifyPayload_EmptyPayloadCarriesInvalidCode(t *testing.T) {
	service := newCaptchaServiceForTest(map[string]string{
		SettingKeyCaptchaProvider: CaptchaProviderTurnstile,
		SettingKeyCaptchaConfig:   `{"enabled":"true","site_key":"site","secret_key":"sk"}`,
	}, &captchaVerifierSpy{})

	err := service.VerifyPayload(context.Background(), map[string]string{}, "1.2.3.4")
	require.Error(t, err)

	var appErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, CaptchaErrInvalid, appErr.Metadata["captcha_error_code"])
}

func TestCaptchaService_VerifyToken_PropagatesRequestError(t *testing.T) {
	service := newCaptchaServiceForTest(map[string]string{
		SettingKeyCaptchaProvider: CaptchaProviderHcaptcha,
		SettingKeyCaptchaConfig:   `{"enabled":"true","site_key":"site","secret_key":"secret"}`,
	}, &captchaVerifierSpy{verifyErr: errors.New("network failed")})

	err := service.VerifyToken(context.Background(), "token", "127.0.0.1")

	require.ErrorContains(t, err, "send request")
}

func TestCaptchaService_VerifyToken_PassesConfigAndPayload(t *testing.T) {
	verifier := &captchaVerifierSpy{}
	service := newCaptchaServiceForTest(map[string]string{
		SettingKeyCaptchaProvider: CaptchaProviderTurnstile,
		SettingKeyCaptchaConfig:   `{"enabled":"true","site_key":"site","secret_key":"sk"}`,
	}, verifier)

	err := service.VerifyToken(context.Background(), "token-x", "1.2.3.4")
	require.NoError(t, err)

	require.Equal(t, 1, verifier.verifyCalled)
	require.Equal(t, CaptchaProviderTurnstile, verifier.lastReq.Provider)
	require.Equal(t, "sk", verifier.lastReq.Config["secret_key"])
	require.Equal(t, "token-x", verifier.lastReq.Payload["token"])
	require.Equal(t, "1.2.3.4", verifier.lastReq.RemoteIP)
}

func TestCaptchaService_ValidateProviderSecretKey_DelegatesToVerifier(t *testing.T) {
	verifier := &captchaVerifierSpy{validateErr: ErrCaptchaInvalidSecretKey}
	service := newCaptchaServiceForTest(nil, verifier)

	err := service.ValidateProviderSecretKey(context.Background(), CaptchaProviderHcaptcha, "bad-secret")

	require.ErrorIs(t, err, ErrCaptchaInvalidSecretKey)
	require.Equal(t, 1, verifier.validateCalled)
	require.Equal(t, CaptchaProviderHcaptcha, verifier.lastValidateP)
	require.Equal(t, "bad-secret", verifier.lastValidateCfg["secret_key"])
}

func TestCaptchaService_ValidateSecretKey_PreservesTurnstileCompatibility(t *testing.T) {
	verifier := &captchaVerifierSpy{validateErr: ErrTurnstileInvalidSecretKey}
	service := newCaptchaServiceForTest(nil, verifier)

	err := service.ValidateSecretKey(context.Background(), "bad-secret")

	require.ErrorIs(t, err, ErrTurnstileInvalidSecretKey)
	require.Equal(t, CaptchaProviderTurnstile, verifier.lastValidateP)
}
