package service

import (
	"context"
	"fmt"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrCaptchaVerificationFailed = infraerrors.BadRequest("CAPTCHA_VERIFICATION_FAILED", "captcha verification failed")
	ErrCaptchaNotConfigured      = infraerrors.ServiceUnavailable("CAPTCHA_NOT_CONFIGURED", "captcha not configured")
	ErrCaptchaInvalidSecretKey   = infraerrors.BadRequest("CAPTCHA_INVALID_SECRET_KEY", "invalid captcha secret key")
)

const (
	CaptchaProviderTurnstile = "turnstile"
	CaptchaProviderHcaptcha  = "hcaptcha"
	// CaptchaProviderTencent 腾讯天御 (TencentCaptcha) provider 标识。
	// 协议见 design.md D2 / D3 / D4 / D5。
	CaptchaProviderTencent = "tencent_captcha"
)

// 归一化错误码（design.md D6）。
// handler/前端凭此区分错误类别，禁止直接读 ProviderMsg 做逻辑判断。
const (
	CaptchaErrInvalid               = "captcha.invalid"
	CaptchaErrConfig                = "captcha.config"
	CaptchaErrTimeout               = "captcha.timeout"
	CaptchaErrDuplicate             = "captcha.duplicate"
	CaptchaErrNetwork               = "captcha.network"
	CaptchaErrTencentFallbackTicket = "captcha.tencent.fallback_ticket"
)

// VerifyRequest 是验证码服务端校验的统一请求对象（design.md D3）。
//
// Config 直接复用 captcha_config 的 KV 形态，verifier 内部按 Provider 自取所需字段：
//   - turnstile / hcaptcha: secret_key
//   - tencent_captcha: captcha_app_id / app_secret_key / secret_id / secret_key
//
// Payload 是结构化客户端凭证（design.md D2）：
//   - turnstile / hcaptcha: {"token": "..."}
//   - tencent_captcha: {"ticket": "...", "randstr": "..."}
type VerifyRequest struct {
	Provider string
	Config   map[string]string
	Payload  map[string]string
	RemoteIP string
}

// VerifyResult 是 verifier 的归一化返回。
//
// Success=true 时其它字段视为补充信息；Success=false 时 ErrorCode 必填。
// EvilLevel 仅 tencent_captcha 提供，其它 provider 恒为 0。
// RawResponse 仅在调试日志路径使用，业务代码不应依赖其内部结构。
type VerifyResult struct {
	Success     bool
	ErrorCode   string
	ProviderMsg string
	EvilLevel   int
	RawResponse map[string]any
}

// CaptchaVerifier 是 service 层依赖的 captcha 校验抽象（design.md D3）。
//
// 实现位于 repository 包，按 Provider 分发到具体协议（Turnstile/hCaptcha/天御）。
// 老的 VerifyToken 单密钥签名已废弃，存量调用通过 CaptchaService 的兼容包装方法过渡。
type CaptchaVerifier interface {
	Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error)
	// ValidateProviderConfig 用于 admin 保存配置时的预校验。
	// Turnstile/hCaptcha: 用 deliberately-invalid token 探活，区分"密钥错"与"token 错"。
	// tencent_captcha: 直接返回 nil（票据 5 分钟内一次性，无可重放探活路径）；
	//   admin 端据此提示"无法预校验，请实测一次"。
	// 任何不识别的 provider 必须返回非 nil 错误，禁止静默放行。
	ValidateProviderConfig(ctx context.Context, provider string, config map[string]string) error
}

// CaptchaService verifies tokens for the currently configured captcha provider.
type CaptchaService struct {
	settingService *SettingService
	verifier       CaptchaVerifier
}

// CaptchaVerifyResponse 是 Turnstile/hCaptcha siteverify 端点的原始响应形态。
//
// 历史上同时承担"verifier 返回值"与"siteverify 响应解码目标"两个职责；
// 自接口重构（D3）起，verifier 返回值改为 VerifyResult；本结构仅作为 repository 包内部
// 解码 siteverify JSON 的结构化目标继续存在，**业务代码不应再直接依赖它**。
type CaptchaVerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes"`
	Action      string   `json:"action"`
	CData       string   `json:"cdata"`
}

func NewCaptchaService(settingService *SettingService, verifier CaptchaVerifier) *CaptchaService {
	return &CaptchaService{
		settingService: settingService,
		verifier:       verifier,
	}
}

// VerifyToken 是兼容老调用面的薄封装（task 1.4）。
//
// 内部把 (token, remoteIP) 翻译为 VerifyRequest 调新接口；
// 所有 Turnstile/hCaptcha/单 token 类 provider 都走这条路径。
// 待全部 handler 接入 captcha_payload 协议后（§4），该方法可被 VerifyPayload 取代。
func (s *CaptchaService) VerifyToken(ctx context.Context, token string, remoteIP string) error {
	return s.VerifyPayload(ctx, map[string]string{"token": token}, remoteIP)
}

// captchaErrMetadataKey 是把归一化错误码（CaptchaErrInvalid / Config / Timeout / ...）
// 透出到 ApplicationError.Metadata 的键名（design.md D6）。
// handler 层不直接读 metadata，由 ResponseEnvelope 自动序列化到 metadata.captcha_error_code，
// 前端据此实现 fallback 重试 / 文案选择 / 触发新一次 widget execute() 等逻辑。
const captchaErrMetadataKey = "captcha_error_code"

// captchaVerificationFailedWithCode 在保留既有 BadRequest 错误码（TURNSTILE_/CAPTCHA_VERIFICATION_FAILED）
// 的前提下，把归一化错误码写入 metadata。errorCode 为空时返回原始错误，避免 metadata 出现空 key。
func captchaVerificationFailedWithCode(provider, errorCode string) error {
	base := ErrCaptchaVerificationFailed
	if provider == CaptchaProviderTurnstile {
		base = ErrTurnstileVerificationFailed
	}
	if errorCode == "" {
		return base
	}
	return base.WithMetadata(map[string]string{captchaErrMetadataKey: errorCode})
}

// VerifyPayload 是 captcha_payload 协议下的统一入口（design.md D2）。
//
// payload 内部允许是空 map（被 VerifyToken("") 转过来时为 {"token": ""}），
// 这种"空凭证"情形保留与历史相同的语义：依然走 not-configured / verification-failed 分支，
// 由调用方区分错误类型。
//
// 失败时若 verifier 返回了归一化 ErrorCode（D6），会通过 metadata.captcha_error_code 透传给前端。
func (s *CaptchaService) VerifyPayload(ctx context.Context, payload map[string]string, remoteIP string) error {
	runtime := s.settingService.GetCaptchaRuntime(ctx)
	if !runtime.Enabled {
		logger.LegacyPrintf("service.captcha", "%s", "[Captcha] Disabled, skipping verification")
		return nil
	}

	if !captchaConfigCredentialed(runtime) {
		logger.LegacyPrintf("service.captcha", "[Captcha] Provider credentials not configured (provider=%s)", runtime.Provider)
		if runtime.Provider == CaptchaProviderTurnstile {
			return ErrTurnstileNotConfigured
		}
		return ErrCaptchaNotConfigured
	}

	if !captchaPayloadHasCredential(runtime.Provider, payload) {
		logger.LegacyPrintf("service.captcha", "[Captcha] Empty payload for provider=%s", runtime.Provider)
		return captchaVerificationFailedWithCode(runtime.Provider, CaptchaErrInvalid)
	}

	logger.LegacyPrintf("service.captcha", "[Captcha] Verifying payload for provider=%s IP=%s", runtime.Provider, remoteIP)
	result, err := s.verifier.Verify(ctx, VerifyRequest{
		Provider: runtime.Provider,
		Config:   runtime.Config,
		Payload:  payload,
		RemoteIP: remoteIP,
	})
	if err != nil {
		logger.LegacyPrintf("service.captcha", "[Captcha] Request failed: %v", err)
		return fmt.Errorf("send request: %w", err)
	}

	if !result.Success {
		logger.LegacyPrintf("service.captcha", "[Captcha] Verification failed code=%s msg=%s", result.ErrorCode, result.ProviderMsg)
		return captchaVerificationFailedWithCode(runtime.Provider, result.ErrorCode)
	}

	logger.LegacyPrintf("service.captcha", "%s", "[Captcha] Verification successful")
	return nil
}

// IsEnabled checks whether any captcha provider is enabled.
func (s *CaptchaService) IsEnabled(ctx context.Context) bool {
	return s.settingService.GetCaptchaRuntime(ctx).Enabled
}

// ValidateSecretKey 验证 Turnstile Secret Key 是否有效（兼容老调用面）。
//
// 等价于以最小可用 turnstile config（仅含 secret_key）调用 ValidateProviderConfig。
func (s *CaptchaService) ValidateSecretKey(ctx context.Context, secretKey string) error {
	return s.ValidateProviderSecretKey(ctx, CaptchaProviderTurnstile, secretKey)
}

// ValidateProviderSecretKey 是兼容老调用面的薄封装（task 1.4）。
//
// admin 设置页的"测试密钥"按钮目前仅传单个 secret_key，因此包成只含一个字段的 config。
// 天御场景下 secret_key 不再是充要凭证，admin 端会在 §3 / §8 中改造为传完整 config，
// 届时该薄封装的 turnstile/hcaptcha 行为不变，天御走 ValidateProviderConfig 的 nil 返回。
func (s *CaptchaService) ValidateProviderSecretKey(ctx context.Context, provider, secretKey string) error {
	cfg := map[string]string{"secret_key": secretKey}
	return s.ValidateProviderConfig(ctx, provider, cfg)
}

// ValidateProviderConfig 直接代理底层 verifier 的预校验能力。
//
// 错误归一化：底层探活若识别出"密钥错"，按 provider 返回对应的 BadRequest 错误码；
// 其它异常透传给调用方，由 handler 层决定渲染策略。
func (s *CaptchaService) ValidateProviderConfig(ctx context.Context, provider string, config map[string]string) error {
	if err := s.verifier.ValidateProviderConfig(ctx, provider, config); err != nil {
		return err
	}
	return nil
}

// captchaConfigCredentialed 判定当前 provider 在 runtime 下是否具备最小校验凭证。
//
// 这里只做"非空"判断，不替代 verifier 内部的格式校验——避免双重校验导致提示信息分裂。
func captchaConfigCredentialed(runtime CaptchaRuntime) bool {
	switch runtime.Provider {
	case CaptchaProviderTurnstile, CaptchaProviderHcaptcha:
		return runtime.SecretKey != ""
	case CaptchaProviderTencent:
		// 天御需要四个字段中的两组：CaptchaAppId+AppSecretKey 用于业务校验，
		// SecretId+SecretKey 用于 TC3 签名鉴权。任一缺失都无法发出有效请求。
		return runtime.Config["captcha_app_id"] != "" &&
			runtime.Config["app_secret_key"] != "" &&
			runtime.Config["secret_id"] != "" &&
			runtime.Config["secret_key"] != ""
	default:
		return runtime.SecretKey != ""
	}
}

// captchaPayloadHasCredential 判定 payload 是否包含当前 provider 所需的最少凭证字段。
//
// 仅做"字段存在且非空"检查；具体长度/格式由 verifier 自身负责。
func captchaPayloadHasCredential(provider string, payload map[string]string) bool {
	if len(payload) == 0 {
		return false
	}
	switch provider {
	case CaptchaProviderTencent:
		return payload["ticket"] != "" && payload["randstr"] != ""
	default:
		return payload["token"] != ""
	}
}
