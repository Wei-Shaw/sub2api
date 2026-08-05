package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrTencentCaptchaVerificationFailed = infraerrors.BadRequest("TENCENT_CAPTCHA_VERIFICATION_FAILED", "tencent captcha verification failed")
	ErrTencentCaptchaNotConfigured      = infraerrors.ServiceUnavailable("TENCENT_CAPTCHA_NOT_CONFIGURED", "tencent captcha not configured")
)

type TencentCaptchaProof struct {
	Ticket  string
	Randstr string
}

type TencentCaptchaCredentials struct {
	AppID          uint64
	AppSecretKey   string
	CloudSecretID  string
	CloudSecretKey string
}

type TencentCaptchaVerifyResponse struct {
	CaptchaCode int64
	CaptchaMsg  string
	RequestID   string
}

type TencentCaptchaVerifier interface {
	VerifyTicket(context.Context, TencentCaptchaCredentials, TencentCaptchaProof, string) (*TencentCaptchaVerifyResponse, error)
}

type TencentCaptchaService struct {
	settingService *SettingService
	verifier       TencentCaptchaVerifier
}

func NewTencentCaptchaService(settingService *SettingService, verifier TencentCaptchaVerifier) *TencentCaptchaService {
	return &TencentCaptchaService{settingService: settingService, verifier: verifier}
}

func (s *TencentCaptchaService) VerifyTicket(ctx context.Context, ticket, randstr, remoteIP string) error {
	if s == nil || s.settingService == nil {
		return ErrTencentCaptchaNotConfigured
	}
	providerConfig, err := s.settingService.GetCaptchaProviderConfig(ctx)
	if err != nil {
		logger.LegacyPrintf("service.tencent_captcha", "%s", "[TencentCaptcha] failed to read captcha provider settings")
		return ErrServiceUnavailable
	}
	config := providerConfig.Tencent
	if !config.Enabled {
		return nil
	}
	return s.VerifyTicketWithConfig(ctx, config, ticket, randstr, remoteIP)
}

func (s *TencentCaptchaService) VerifyTicketWithConfig(ctx context.Context, config TencentCaptchaConfig, ticket, randstr, remoteIP string) error {
	credentials, ok := parseTencentCaptchaCredentials(config)
	if !ok || s.verifier == nil {
		return ErrTencentCaptchaNotConfigured
	}

	proof := TencentCaptchaProof{
		Ticket:  strings.TrimSpace(ticket),
		Randstr: strings.TrimSpace(randstr),
	}
	if proof.Ticket == "" || proof.Randstr == "" || strings.HasPrefix(proof.Ticket, "trerror_") {
		return ErrTencentCaptchaVerificationFailed
	}

	result, err := s.verifier.VerifyTicket(ctx, credentials, proof, remoteIP)
	if err != nil {
		logger.LegacyPrintf("service.tencent_captcha", "%s", "[TencentCaptcha] verification request failed")
		return fmt.Errorf("%w: verifier request failed", ErrTencentCaptchaVerificationFailed)
	}
	if result == nil || result.CaptchaCode != 1 {
		if result != nil {
			logger.LegacyPrintf("service.tencent_captcha", "[TencentCaptcha] rejected code=%d request_id=%s", result.CaptchaCode, result.RequestID)
		}
		return ErrTencentCaptchaVerificationFailed
	}
	return nil
}

func parseTencentCaptchaCredentials(config TencentCaptchaConfig) (TencentCaptchaCredentials, bool) {
	appID, err := strconv.ParseUint(strings.TrimSpace(config.AppID), 10, 64)
	if err != nil || appID == 0 {
		return TencentCaptchaCredentials{}, false
	}
	credentials := TencentCaptchaCredentials{
		AppID:          appID,
		AppSecretKey:   strings.TrimSpace(config.AppSecretKey),
		CloudSecretID:  strings.TrimSpace(config.CloudSecretID),
		CloudSecretKey: strings.TrimSpace(config.CloudSecretKey),
	}
	if credentials.AppSecretKey == "" || credentials.CloudSecretID == "" || credentials.CloudSecretKey == "" {
		return TencentCaptchaCredentials{}, false
	}
	return credentials, true
}
