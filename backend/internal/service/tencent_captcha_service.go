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
	// Endpoint 服务端票据校验接入点，由地域推导，repository 层直接使用
	Endpoint string
}

const (
	// TencentCaptchaRegionCN 中国站（cloud.tencent.com）；TencentCaptchaRegionINTL 国际站（tencentcloud.com）。
	// 该值同时决定前端加载的 SDK 脚本与服务端校验接入点，两端必须一致：
	// 国际站 CaptchaAppId 配国内站 SDK 会被腾讯直接判为「appid 所属地域与实际使用地域不符」。
	TencentCaptchaRegionCN   = "cn"
	TencentCaptchaRegionINTL = "intl"

	tencentCaptchaEndpointCN   = "captcha.tencentcloudapi.com"
	tencentCaptchaEndpointINTL = "captcha.intl.tencentcloudapi.com"
)

// tencentCaptchaEndpoint 按后台配置的地域返回服务端接入点，未知值回退中国站
func tencentCaptchaEndpoint(region string) string {
	if region == TencentCaptchaRegionINTL {
		return tencentCaptchaEndpointINTL
	}
	return tencentCaptchaEndpointCN
}

// normalizeTencentCaptchaRegion 非法值一律视为中国站
func normalizeTencentCaptchaRegion(value string) string {
	if value == TencentCaptchaRegionINTL {
		return TencentCaptchaRegionINTL
	}
	return TencentCaptchaRegionCN
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
		logger.LegacyPrintf(
			"service.tencent_captcha",
			"[TencentCaptcha] verification request failed region=%s endpoint=%s error=%v",
			normalizeTencentCaptchaRegion(config.Region),
			credentials.Endpoint,
			err,
		)
		return fmt.Errorf("%w: verifier request failed", ErrTencentCaptchaVerificationFailed)
	}
	if result == nil || result.CaptchaCode != 1 {
		if result != nil {
			logger.LegacyPrintf(
				"service.tencent_captcha",
				"[TencentCaptcha] rejected region=%s code=%d message=%q request_id=%q",
				normalizeTencentCaptchaRegion(config.Region),
				result.CaptchaCode,
				result.CaptchaMsg,
				result.RequestID,
			)
		} else {
			logger.LegacyPrintf(
				"service.tencent_captcha",
				"[TencentCaptcha] rejected region=%s empty_response=true",
				normalizeTencentCaptchaRegion(config.Region),
			)
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
		Endpoint:       tencentCaptchaEndpoint(config.Region),
	}
	if credentials.AppSecretKey == "" || credentials.CloudSecretID == "" || credentials.CloudSecretKey == "" {
		return TencentCaptchaCredentials{}, false
	}
	return credentials, true
}
