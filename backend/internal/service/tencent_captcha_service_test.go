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
	response    *TencentCaptchaVerifyResponse
	err         error
	calls       int
	proof       TencentCaptchaProof
	remoteIP    string
	credentials TencentCaptchaCredentials
}

func (s *tencentCaptchaVerifierStub) VerifyTicket(_ context.Context, credentials TencentCaptchaCredentials, proof TencentCaptchaProof, remoteIP string) (*TencentCaptchaVerifyResponse, error) {
	s.calls++
	s.credentials = credentials
	s.proof = proof
	s.remoteIP = remoteIP
	return s.response, s.err
}

func newTencentCaptchaTestService(verifier TencentCaptchaVerifier) *TencentCaptchaService {
	return newTencentCaptchaTestServiceWithRegion(verifier, "")
}

func newTencentCaptchaTestServiceWithRegion(verifier TencentCaptchaVerifier, region string) *TencentCaptchaService {
	values := map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          "123456789",
		SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
		SettingKeyTencentCaptchaCloudSecretID:  "cloud-secret-id",
		SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret-key",
	}
	if region != "" {
		values[SettingKeyTencentCaptchaRegion] = region
	}
	settings := NewSettingService(&settingPublicRepoStub{values: values}, &config.Config{})
	return NewTencentCaptchaService(settings, verifier)
}

// 站点决定服务端票据校验接入点：国际站账号的密钥在国内站接入点上无法通过鉴权，
// 因此这条映射一旦错位，国际站验证码会整体失效。
func TestTencentCaptchaServiceRoutesVerifyEndpointByRegion(t *testing.T) {
	cases := []struct {
		name         string
		region       string
		wantEndpoint string
	}{
		{"未配置回落中国站", "", "captcha.tencentcloudapi.com"},
		{"中国站", TencentCaptchaRegionCN, "captcha.tencentcloudapi.com"},
		{"国际站", TencentCaptchaRegionINTL, "captcha.intl.tencentcloudapi.com"},
		{"非法值回落中国站", "sgp", "captcha.tencentcloudapi.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
			svc := newTencentCaptchaTestServiceWithRegion(verifier, tc.region)

			require.NoError(t, svc.VerifyTicket(context.Background(), "ticket", "@rand", "203.0.113.10"))
			require.Equal(t, tc.wantEndpoint, verifier.credentials.Endpoint)
		})
	}
}

func TestTencentCaptchaServiceAcceptsCaptchaCodeOne(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	svc := newTencentCaptchaTestService(verifier)

	err := svc.VerifyTicket(context.Background(), "ticket", "@rand", "203.0.113.10")

	require.NoError(t, err)
	require.Equal(t, 1, verifier.calls)
	require.Equal(t, TencentCaptchaProof{Ticket: "ticket", Randstr: "@rand"}, verifier.proof)
	require.Equal(t, "203.0.113.10", verifier.remoteIP)
}

func TestTencentCaptchaServiceRejectsDisasterRecoveryTicketWithoutCallingVerifier(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	svc := newTencentCaptchaTestService(verifier)

	err := svc.VerifyTicket(context.Background(), "trerror_1001_123456789_1", "@rand", "203.0.113.10")

	require.ErrorIs(t, err, ErrTencentCaptchaVerificationFailed)
	require.Zero(t, verifier.calls)
}

func TestTencentCaptchaServiceRejectsEveryNonOneCode(t *testing.T) {
	for _, code := range []int64{0, 7, 8, 9, 15, 16, 21, 100} {
		t.Run(string(rune(code)), func(t *testing.T) {
			verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: code}}
			svc := newTencentCaptchaTestService(verifier)

			err := svc.VerifyTicket(context.Background(), "ticket", "@rand", "203.0.113.10")

			require.ErrorIs(t, err, ErrTencentCaptchaVerificationFailed)
		})
	}
}

func TestTencentCaptchaServiceFailsClosedOnVerifierError(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{err: errors.New("sdk unavailable")}
	svc := newTencentCaptchaTestService(verifier)

	err := svc.VerifyTicket(context.Background(), "ticket", "@rand", "203.0.113.10")

	require.Error(t, err)
	require.ErrorIs(t, err, ErrTencentCaptchaVerificationFailed)
}

func TestTencentCaptchaServiceRejectsIncompleteConfiguration(t *testing.T) {
	settings := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyTencentCaptchaEnabled: "true",
		SettingKeyTencentCaptchaAppID:   "123456789",
	}}, &config.Config{})
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	svc := NewTencentCaptchaService(settings, verifier)

	err := svc.VerifyTicket(context.Background(), "ticket", "@rand", "203.0.113.10")

	require.ErrorIs(t, err, ErrTencentCaptchaNotConfigured)
	require.Zero(t, verifier.calls)
}

func TestTencentCaptchaServiceFailsClosedOnSettingsReadError(t *testing.T) {
	settings := NewSettingService(&settingPublicRepoStub{err: errors.New("settings unavailable")}, &config.Config{})
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	svc := NewTencentCaptchaService(settings, verifier)

	err := svc.VerifyTicket(context.Background(), "ticket", "@rand", "203.0.113.10")

	require.ErrorIs(t, err, ErrServiceUnavailable)
	require.Zero(t, verifier.calls)
}
