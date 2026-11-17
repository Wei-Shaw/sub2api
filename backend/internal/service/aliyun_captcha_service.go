package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrAliyunCaptchaVerificationFailed = infraerrors.BadRequest("ALIYUN_CAPTCHA_VERIFICATION_FAILED", "aliyun captcha verification failed")
	ErrAliyunCaptchaNotConfigured      = infraerrors.ServiceUnavailable("ALIYUN_CAPTCHA_NOT_CONFIGURED", "aliyun captcha not configured")
	// ErrCaptchaInvalidCredentials 阿里云验证码凭证无效（仅后台保存校验时返回，公开接口错误码不变）
	ErrCaptchaInvalidCredentials = infraerrors.BadRequest("CAPTCHA_INVALID_CREDENTIALS", "invalid aliyun captcha credentials")
)

// AliyunCaptchaCredentials 阿里云验证码 2.0 服务端校验所需的完整凭证
type AliyunCaptchaCredentials struct {
	AccessKeyID     string
	AccessKeySecret string
	SceneID         string
	Endpoint        string
}

// AliyunCaptchaVerifyResult VerifyIntelligentCaptcha 的归一化结果
type AliyunCaptchaVerifyResult struct {
	VerifyResult bool
	VerifyCode   string // 阿里云细分结果码，仅用于日志
}

// AliyunCaptchaAPIError 阿里云 OpenAPI 业务错误。
// repository 层负责把 SDK 错误归一化为该类型，service 层不依赖 SDK 包。
type AliyunCaptchaAPIError struct {
	Code    string
	Message string
}

func (e *AliyunCaptchaAPIError) Error() string {
	return fmt.Sprintf("aliyun captcha api error: %s: %s", e.Code, e.Message)
}

// AliyunCaptchaVerifier 调用阿里云验证码 2.0 服务端校验的端口
type AliyunCaptchaVerifier interface {
	VerifyCaptcha(ctx context.Context, cred AliyunCaptchaCredentials, captchaVerifyParam string) (*AliyunCaptchaVerifyResult, error)
}

const (
	// AliyunCaptchaRegionCN 中国内地；AliyunCaptchaRegionSGP 新加坡。
	// 该值同时下发给前端 AliyunCaptchaConfig.region，两端必须一致。
	AliyunCaptchaRegionCN  = "cn"
	AliyunCaptchaRegionSGP = "sgp"

	aliyunCaptchaEndpointCN  = "captcha.cn-shanghai.aliyuncs.com"
	aliyunCaptchaEndpointSGP = "captcha.ap-southeast-1.aliyuncs.com"
)

// aliyunCaptchaEndpoint 按后台配置的地域返回服务端接入点，未知值回退中国内地
func aliyunCaptchaEndpoint(region string) string {
	if region == AliyunCaptchaRegionSGP {
		return aliyunCaptchaEndpointSGP
	}
	return aliyunCaptchaEndpointCN
}

// normalizeAliyunCaptchaRegion 非法值一律视为中国内地
func normalizeAliyunCaptchaRegion(value string) string {
	if value == AliyunCaptchaRegionSGP {
		return AliyunCaptchaRegionSGP
	}
	return AliyunCaptchaRegionCN
}

// aliyunCredentialValidationParam 用于后台保存时探测凭证有效性的假验证参数
const aliyunCredentialValidationParam = "sub2api-credential-validation"

// aliyunInvalidCredentialCodes 表示 AK/SK 本身无效的阿里云错误码；
// 其余错误码（如 param 无效）说明签名已通过、凭证可用。
var aliyunInvalidCredentialCodes = map[string]struct{}{
	"InvalidAccessKeyId.NotFound":  {},
	"InvalidAccessKeyId.Inactive":  {},
	"SignatureDoesNotMatch":        {},
	"Forbidden.AccessKeyDisabled":  {},
	"IncompleteSignature":          {},
	"InvalidSecurityToken.Expired": {},
}

// AliyunCaptchaService 阿里云验证码 2.0 服务端校验
type AliyunCaptchaService struct {
	settingService *SettingService
	verifier       AliyunCaptchaVerifier
}

func NewAliyunCaptchaService(settingService *SettingService, verifier AliyunCaptchaVerifier) *AliyunCaptchaService {
	return &AliyunCaptchaService{settingService: settingService, verifier: verifier}
}

func aliyunCaptchaCredentials(config AliyunCaptchaConfig) (AliyunCaptchaCredentials, bool) {
	cred := AliyunCaptchaCredentials{
		AccessKeyID:     strings.TrimSpace(config.AccessKeyID),
		AccessKeySecret: strings.TrimSpace(config.AccessKeySecret),
		SceneID:         strings.TrimSpace(config.SceneID),
		Endpoint:        aliyunCaptchaEndpoint(config.Region),
	}
	if cred.AccessKeyID == "" || cred.AccessKeySecret == "" || cred.SceneID == "" {
		return AliyunCaptchaCredentials{}, false
	}
	return cred, true
}

// VerifyParamWithConfig 校验阿里云验证码 2.0 的 captchaVerifyParam。
// 调用异常时返回错误（fail-closed），与 Turnstile 网络错误行为对称。
func (s *AliyunCaptchaService) VerifyParamWithConfig(ctx context.Context, config AliyunCaptchaConfig, captchaVerifyParam string) error {
	if s == nil || s.verifier == nil {
		return ErrAliyunCaptchaNotConfigured
	}
	cred, ok := aliyunCaptchaCredentials(config)
	if !ok {
		logger.LegacyPrintf("service.aliyun_captcha", "%s", "[AliyunCaptcha] credentials not configured")
		return ErrAliyunCaptchaNotConfigured
	}

	if strings.TrimSpace(captchaVerifyParam) == "" {
		logger.LegacyPrintf("service.aliyun_captcha", "%s", "[AliyunCaptcha] captchaVerifyParam is empty")
		return ErrAliyunCaptchaVerificationFailed
	}

	result, err := s.verifier.VerifyCaptcha(ctx, cred, captchaVerifyParam)
	if err != nil {
		logger.LegacyPrintf("service.aliyun_captcha", "[AliyunCaptcha] verify request failed: %v", err)
		return fmt.Errorf("%w: verifier request failed", ErrAliyunCaptchaVerificationFailed)
	}

	if result == nil || !result.VerifyResult {
		if result != nil {
			logger.LegacyPrintf("service.aliyun_captcha", "[AliyunCaptcha] rejected, verify code: %s", result.VerifyCode)
		}
		return ErrAliyunCaptchaVerificationFailed
	}
	return nil
}

// ValidateCredentials 用假验证参数探测阿里云 AK/SK 是否可用（后台保存设置时调用）。
// 凭证类错误码返回 ErrCaptchaInvalidCredentials；正常响应（包括 param 无效导致的
// VerifyResult=false）说明签名通过、凭证有效；其余错误原样返回给管理员排查。
func (s *AliyunCaptchaService) ValidateCredentials(ctx context.Context, accessKeyID, accessKeySecret, sceneID, region string) error {
	if s.verifier == nil {
		return ErrAliyunCaptchaNotConfigured
	}
	cred := AliyunCaptchaCredentials{
		AccessKeyID:     accessKeyID,
		AccessKeySecret: accessKeySecret,
		SceneID:         sceneID,
		Endpoint:        aliyunCaptchaEndpoint(region),
	}

	_, err := s.verifier.VerifyCaptcha(ctx, cred, aliyunCredentialValidationParam)
	if err != nil {
		var apiErr *AliyunCaptchaAPIError
		if errors.As(err, &apiErr) {
			if _, invalid := aliyunInvalidCredentialCodes[apiErr.Code]; invalid {
				return ErrCaptchaInvalidCredentials
			}
		}
		return fmt.Errorf("validate aliyun captcha credentials: %w", err)
	}

	return nil
}
