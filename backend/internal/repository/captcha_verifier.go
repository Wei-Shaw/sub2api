package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	hcaptchaVerifyURL  = "https://hcaptcha.com/siteverify"
)

// captchaHTTPVerifier 实现 service.CaptchaVerifier，覆盖 Turnstile 与 hCaptcha 两个
// "siteverify 协议族"的 provider；天御 provider 的实现见 captcha_tencent.go（§2 引入）。
//
// 字段说明：
//   - httpClient: 复用 httpclient.GetClient 的 SSRF 防护与统一超时；测试可替换。
//   - verifyURLs: provider → siteverify endpoint，便于测试直接重定向到 in-process server。
type captchaHTTPVerifier struct {
	httpClient *http.Client
	verifyURLs map[string]string
}

func NewCaptchaVerifier() service.CaptchaVerifier {
	sharedClient, err := httpclient.GetClient(httpclient.Options{
		Timeout:            10 * time.Second,
		ValidateResolvedIP: true,
	})
	if err != nil {
		sharedClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &captchaHTTPVerifier{
		httpClient: sharedClient,
		verifyURLs: map[string]string{
			service.CaptchaProviderTurnstile: turnstileVerifyURL,
			service.CaptchaProviderHcaptcha:  hcaptchaVerifyURL,
		},
	}
}

// Verify 实现 service.CaptchaVerifier。
//
// 按 Provider 分发：
//   - turnstile / hcaptcha → siteverify HTTP 协议（本文件 verifyTurnstileFamily）
//   - tencent_captcha → 腾讯云 captcha API + TC3 签名（captcha_tencent.go verifyTencentCaptcha）
//   - 其它 → 归一化为 captcha.config 失败（不发 HTTP）
//
// 失败语义：
//   - HTTP 层错误（连接 / 超时 / decode）→ 返回 nil error 但 Result.ErrorCode = captcha.network；
//     这样 service 层不需要区分"网络抖动"与"校验失败"，统一按 verification_failed 走。
//     选择此策略而非 return error 的原因：上层 logger 已经记录 ProviderMsg，
//     再额外传播 error 会让 captcha_service.go 的错误日志重复。
//   - 业务层失败（success=false 等）→ Success=false 配合归一化 ErrorCode。
func (v *captchaHTTPVerifier) Verify(ctx context.Context, req service.VerifyRequest) (*service.VerifyResult, error) {
	switch req.Provider {
	case service.CaptchaProviderTencent:
		return v.verifyTencentCaptcha(ctx, req)
	case service.CaptchaProviderTurnstile, service.CaptchaProviderHcaptcha:
		return v.verifyTurnstileFamily(ctx, req)
	default:
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrConfig,
			ProviderMsg: fmt.Sprintf("unsupported captcha provider: %s", req.Provider),
		}, nil
	}
}

// verifyTurnstileFamily 处理 Turnstile / hCaptcha 这类"siteverify form-encoded"协议。
func (v *captchaHTTPVerifier) verifyTurnstileFamily(ctx context.Context, req service.VerifyRequest) (*service.VerifyResult, error) {
	verifyURL := v.verifyURLs[req.Provider]
	if verifyURL == "" {
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrConfig,
			ProviderMsg: fmt.Sprintf("unsupported captcha provider: %s", req.Provider),
		}, nil
	}

	token := req.Payload["token"]
	if token == "" {
		// payload 缺字段 → 不发 HTTP 请求，归一化为 captcha.invalid。
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrInvalid,
			ProviderMsg: "missing token in payload",
		}, nil
	}

	secretKey := req.Config["secret_key"]
	resp, err := v.callSiteVerify(ctx, verifyURL, secretKey, token, req.RemoteIP)
	if err != nil {
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrNetwork,
			ProviderMsg: err.Error(),
		}, nil
	}

	if resp.Success {
		return &service.VerifyResult{Success: true}, nil
	}

	return &service.VerifyResult{
		Success:     false,
		ErrorCode:   normalizeSiteverifyErrorCodes(resp.ErrorCodes),
		ProviderMsg: strings.Join(resp.ErrorCodes, ","),
	}, nil
}

// ValidateProviderConfig 实现 service.CaptchaVerifier.ValidateProviderConfig。
//
// 行为分支：
//   - turnstile / hcaptcha: 用一个明显无效的 token 探活 siteverify；
//     若返回 invalid-input-secret / missing-input-secret 则判定为"密钥错"；
//     其它失败码（含 invalid-input-response）视为"密钥可用，只是 token 错"。
//   - tencent_captcha: 直接 nil（design D3：天御无可重放探活路径）。
//   - 未知 provider: 显式拒绝，禁止静默放行（spec.md §"Unknown provider rejected"）。
func (v *captchaHTTPVerifier) ValidateProviderConfig(ctx context.Context, provider string, config map[string]string) error {
	switch provider {
	case service.CaptchaProviderTurnstile, service.CaptchaProviderHcaptcha:
		// fall through 到下方探活
	case service.CaptchaProviderTencent:
		return nil
	default:
		return fmt.Errorf("unsupported captcha provider: %s", provider)
	}

	verifyURL := v.verifyURLs[provider]
	if verifyURL == "" {
		// 防御性兜底，理论上 case 已覆盖。
		return fmt.Errorf("unsupported captcha provider: %s", provider)
	}

	resp, err := v.callSiteVerify(ctx, verifyURL, config["secret_key"], "test-validation", "")
	if err != nil {
		return fmt.Errorf("validate secret key: %w", err)
	}

	invalidSecretCodes := map[string]bool{
		"invalid-input-secret": true,
		"missing-input-secret": true,
	}
	for _, code := range resp.ErrorCodes {
		if invalidSecretCodes[code] {
			if provider == service.CaptchaProviderTurnstile {
				return service.ErrTurnstileInvalidSecretKey
			}
			return service.ErrCaptchaInvalidSecretKey
		}
	}
	return nil
}

// callSiteVerify 发送 siteverify 表单请求并解码 JSON 响应。
// 复用给 Verify 与 ValidateProviderConfig 两条路径，避免 form 拼装重复。
func (v *captchaHTTPVerifier) callSiteVerify(ctx context.Context, verifyURL, secret, token, remoteIP string) (*service.CaptchaVerifyResponse, error) {
	formData := url.Values{}
	formData.Set("secret", secret)
	formData.Set("response", token)
	if remoteIP != "" {
		formData.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result service.CaptchaVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// normalizeSiteverifyErrorCodes 把 Turnstile / hCaptcha 返回的 error-codes 数组
// 映射到归一化错误码（design.md D6）。
//
// 映射策略以"出现一个就归类"的优先级：config > timeout > duplicate > 兜底 invalid。
// Turnstile 的 timeout-or-duplicate 同时影射两个语义码，文档侧优先归 timeout。
func normalizeSiteverifyErrorCodes(codes []string) string {
	for _, code := range codes {
		switch code {
		case "invalid-input-secret", "missing-input-secret":
			return service.CaptchaErrConfig
		}
	}
	for _, code := range codes {
		switch code {
		case "timeout-or-duplicate":
			return service.CaptchaErrTimeout
		}
	}
	return service.CaptchaErrInvalid
}
