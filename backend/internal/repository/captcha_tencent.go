package repository

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 腾讯天御 (TencentCaptcha) 校验实现 —— design.md D4 / D5 / D6 / D8。
//
// 不引入 tencentcloud-sdk-go-captcha：手写 TC3-HMAC-SHA256 签名；
// HTTP 调用复用 captchaHTTPVerifier.httpClient（带 SSRF 防护与统一超时）。
//
// 协议要点（参考腾讯云 captcha API DescribeCaptchaResult）：
//   - Host: captcha.tencentcloudapi.com
//   - Service: captcha
//   - Action: DescribeCaptchaResult
//   - Version: 2019-07-22
//   - Region: 该接口为全局接口，留空
//   - Body: JSON
//   - 必填业务字段: CaptchaType=9, Ticket, Randstr, UserIp, CaptchaAppId, AppSecretKey
//   - 校验结果字段: Response.CaptchaCode (1 = 成功)

const (
	tencentCaptchaEndpoint    = "https://captcha.tencentcloudapi.com/"
	tencentCaptchaHost        = "captcha.tencentcloudapi.com"
	tencentCaptchaService     = "captcha"
	tencentCaptchaAction      = "DescribeCaptchaResult"
	tencentCaptchaVersion     = "2019-07-22"
	tencentCaptchaSignAlgo    = "TC3-HMAC-SHA256"
	tencentCaptchaContentType = "application/json; charset=utf-8"

	// trerror_ 是天御官方定义的"容灾票据"前缀；前后端协议规定本系统**不放行**该票据。
	tencentFallbackTicketPrefix = "trerror_"
)

// tencentCaptchaResponse 是 DescribeCaptchaResult 的最小字段集；
// 只关心 CaptchaCode / CaptchaMsg / EvilLevel，以及 Error 错误对象。
//
// 腾讯云 API 错误返回形态：
//
//	{ "Response": { "Error": { "Code": "...", "Message": "..." }, "RequestId": "..." } }
//
// 业务返回形态：
//
//	{ "Response": { "CaptchaCode": 1, "CaptchaMsg": "OK", "EvilLevel": 0, "RequestId": "..." } }
type tencentCaptchaResponse struct {
	Response struct {
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error,omitempty"`
		CaptchaCode int    `json:"CaptchaCode"`
		CaptchaMsg  string `json:"CaptchaMsg"`
		EvilLevel   int    `json:"EvilLevel"`
		RequestId   string `json:"RequestId"`
	} `json:"Response"`
}

// verifyTencentCaptcha 是 captchaHTTPVerifier 上的 tencent 分支。
//
// 流程：
//  1. 先短路 trerror_ 容灾票据（design.md D5：服务端永远拒绝，不发 HTTP）。
//  2. 校验 Payload 必填字段（ticket、randstr）+ Config 必填字段（4 个 key）。
//  3. 组装 JSON 请求体 + TC3-HMAC-SHA256 签名头。
//  4. POST endpoint，解析响应 → 归一化为 VerifyResult。
//
// UserIp 直接使用入参 req.RemoteIP（即上游 c.ClientIP()），不做 trusted_proxies 兜底（design.md D8）。
func (v *captchaHTTPVerifier) verifyTencentCaptcha(ctx context.Context, req service.VerifyRequest) (*service.VerifyResult, error) {
	ticket := req.Payload["ticket"]
	randstr := req.Payload["randstr"]

	// D5 严格模式：trerror_ 前缀 → 不发 HTTP，写 WARN 审计日志。
	if strings.HasPrefix(ticket, tencentFallbackTicketPrefix) {
		logger.LegacyPrintf(
			"service.captcha",
			"[Captcha] tencent fallback_ticket rejected ip=%s prefix=%s",
			anonymizeIP(req.RemoteIP),
			tencentFallbackTicketPrefix,
		)
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrTencentFallbackTicket,
			ProviderMsg: "fallback ticket rejected",
		}, nil
	}

	if ticket == "" || randstr == "" {
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrInvalid,
			ProviderMsg: "missing ticket or randstr in payload",
		}, nil
	}

	captchaAppIDStr := req.Config["captcha_app_id"]
	appSecretKey := req.Config["app_secret_key"]
	secretID := req.Config["secret_id"]
	secretKey := req.Config["secret_key"]
	if captchaAppIDStr == "" || appSecretKey == "" || secretID == "" || secretKey == "" {
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrConfig,
			ProviderMsg: "tencent captcha config incomplete",
		}, nil
	}

	captchaAppID, err := strconv.ParseUint(captchaAppIDStr, 10, 64)
	if err != nil {
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrConfig,
			ProviderMsg: "captcha_app_id is not a valid uint64: " + err.Error(),
		}, nil
	}

	body, err := buildTencentCaptchaBody(captchaAppID, appSecretKey, ticket, randstr, req.RemoteIP)
	if err != nil {
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrConfig,
			ProviderMsg: "build request body: " + err.Error(),
		}, nil
	}

	httpReq, err := newTencentCaptchaSignedRequest(ctx, body, secretID, secretKey, time.Now().UTC())
	if err != nil {
		// 签名构造期间的错误属于本地逻辑错误，归类为 network 而非 config，
		// 因为它不是用户配置导致（密钥格式 verifier 不预校验）。
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrNetwork,
			ProviderMsg: "build signed request: " + err.Error(),
		}, nil
	}

	resp, err := v.httpClient.Do(httpReq)
	if err != nil {
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrNetwork,
			ProviderMsg: err.Error(),
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrNetwork,
			ProviderMsg: "read response: " + err.Error(),
		}, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrNetwork,
			ProviderMsg: fmt.Sprintf("non-2xx status %d: %s", resp.StatusCode, truncate(string(respBody), 200)),
		}, nil
	}

	var decoded tencentCaptchaResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrNetwork,
			ProviderMsg: "decode response: " + err.Error(),
		}, nil
	}

	// 平台错误（鉴权失败 / 流控等）走 Error 字段；映射到 captcha.config 或 captcha.network。
	if decoded.Response.Error != nil {
		errCode := decoded.Response.Error.Code
		errMsg := decoded.Response.Error.Message
		// 鉴权失败、签名错误、AppId 无效 → 配置错误。
		if isTencentAuthErrorCode(errCode) {
			return &service.VerifyResult{
				Success:     false,
				ErrorCode:   service.CaptchaErrConfig,
				ProviderMsg: errCode + ": " + errMsg,
			}, nil
		}
		return &service.VerifyResult{
			Success:     false,
			ErrorCode:   service.CaptchaErrNetwork,
			ProviderMsg: errCode + ": " + errMsg,
		}, nil
	}

	if decoded.Response.CaptchaCode == 1 {
		return &service.VerifyResult{
			Success:   true,
			EvilLevel: decoded.Response.EvilLevel,
		}, nil
	}

	return &service.VerifyResult{
		Success:     false,
		ErrorCode:   normalizeTencentCaptchaCode(decoded.Response.CaptchaCode),
		ProviderMsg: fmt.Sprintf("CaptchaCode=%d msg=%s", decoded.Response.CaptchaCode, decoded.Response.CaptchaMsg),
	}, nil
}

// normalizeTencentCaptchaCode 把 CaptchaCode（!= 1）映射到 D6 归一化错误码。
//
// 映射表来自 design.md D6：
//   - {6, 7, 15} → captcha.config（解密失败 / 签名失败 / app_secret 不匹配）
//   - 9          → captcha.timeout（票据过期）
//   - 10         → captcha.duplicate（票据重复使用）
//   - 其余       → captcha.invalid（兜底）
func normalizeTencentCaptchaCode(code int) string {
	switch code {
	case 6, 7, 15:
		return service.CaptchaErrConfig
	case 9:
		return service.CaptchaErrTimeout
	case 10:
		return service.CaptchaErrDuplicate
	default:
		return service.CaptchaErrInvalid
	}
}

// isTencentAuthErrorCode 判断腾讯云返回的 Error.Code 是否属于"鉴权/配置"类。
//
// 文档列出的鉴权类错误前缀有：AuthFailure.* / InvalidParameter.SecretId* /
// UnauthorizedOperation.* 等；这里采用"前缀+特定全名"的最小集，避免误判。
func isTencentAuthErrorCode(code string) bool {
	if code == "" {
		return false
	}
	if strings.HasPrefix(code, "AuthFailure") ||
		strings.HasPrefix(code, "UnauthorizedOperation") {
		return true
	}
	switch code {
	case "InvalidParameterValue.AppidNotFound",
		"InvalidParameterValue.AppSecretKeyError",
		"InvalidParameterValue.AppIdSecretIdMismatch":
		return true
	}
	return false
}

// truncate 截断长字符串用于错误日志；避免一行日志爆出 10KB body。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

// anonymizeIP 对 IP 做粗粒度匿名化用于审计日志：
//   - IPv4: 末段置 0（1.2.3.4 → 1.2.3.0）
//   - IPv6: 仅保留前 48 位，其余置 0
//   - 解析失败：返回固定占位符 "unknown"，避免把脏数据写入日志
//
// 仅在日志路径使用；UserIp 上送给天御 API 的值仍是原始 RemoteIP。
func anonymizeIP(raw string) string {
	if raw == "" {
		return "unknown"
	}
	parsed := net.ParseIP(strings.TrimSpace(raw))
	if parsed == nil {
		return "unknown"
	}
	if v4 := parsed.To4(); v4 != nil {
		v4[3] = 0
		return v4.String()
	}
	// IPv6: 保留前 48 位（前 6 字节），其余置 0。
	v6 := parsed.To16()
	if v6 == nil {
		return "unknown"
	}
	for i := 6; i < 16; i++ {
		v6[i] = 0
	}
	return v6.String()
}

// buildTencentCaptchaBody 组装 DescribeCaptchaResult 请求体（JSON）。
//
// 字段含义（见腾讯云 captcha 文档）：
//   - CaptchaType: 9 = Web 端通用模板（design 文档 Q2 已锁定本期固定 9）
//   - CaptchaAppId: uint64
//   - AppSecretKey: 业务端 App SecretKey（区别于 SecretId/SecretKey 这对云 API 鉴权对）
//   - Ticket / Randstr / UserIp: 客户端凭证
func buildTencentCaptchaBody(captchaAppID uint64, appSecretKey, ticket, randstr, userIP string) ([]byte, error) {
	body := map[string]any{
		"CaptchaType": 9,
		// UserIp 直接使用 gin 的 ClientIP（已按 trusted_proxies 规则解析）。
		// 注意：未配置 trusted_proxies 时该值可能是内网/网关 IP，会降低
		// 天御风控的评分准确性，但不影响功能（评分不是通过性的硬门槛）。
		// 见 design.md D8 / OpenSpec Requirement: UserIp Source Stability。
		"UserIp":       userIP,
		"Ticket":       ticket,
		"Randstr":      randstr,
		"CaptchaAppId": captchaAppID,
		"AppSecretKey": appSecretKey,
	}
	return json.Marshal(body)
}

// newTencentCaptchaSignedRequest 构造 TC3-HMAC-SHA256 签名后的 *http.Request。
//
// 实现严格按照腾讯云 API 文档：
//
//	https://cloud.tencent.com/document/api/1110/36926
//
// 签名时间戳传入而非用 time.Now()，便于单测固定签名向量。
func newTencentCaptchaSignedRequest(ctx context.Context, body []byte, secretID, secretKey string, signedAt time.Time) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tencentCaptchaEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", tencentCaptchaContentType)
	httpReq.Header.Set("Host", tencentCaptchaHost)
	httpReq.Header.Set("X-TC-Action", tencentCaptchaAction)
	httpReq.Header.Set("X-TC-Version", tencentCaptchaVersion)
	httpReq.Header.Set("X-TC-Timestamp", strconv.FormatInt(signedAt.Unix(), 10))

	authHeader := buildTencentTC3Authorization(
		secretID, secretKey,
		body,
		signedAt,
	)
	httpReq.Header.Set("Authorization", authHeader)
	return httpReq, nil
}

// buildTencentTC3Authorization 拼装 Authorization 头。
//
// 完整 5 段：
//  1. CanonicalRequest = HTTPRequestMethod + '\n' + CanonicalURI + '\n' +
//     CanonicalQueryString + '\n' + CanonicalHeaders + '\n' + SignedHeaders + '\n' +
//     HashedRequestPayload
//  2. StringToSign = Algorithm + '\n' + RequestTimestamp + '\n' + CredentialScope + '\n' +
//     HashedCanonicalRequest
//  3. SecretSigning = HMAC-SHA256(HMAC-SHA256(HMAC-SHA256("TC3"+SecretKey, Date), Service), "tc3_request")
//  4. Signature = HEX(HMAC-SHA256(SecretSigning, StringToSign))
//  5. Authorization = "TC3-HMAC-SHA256 Credential=<SecretId>/<CredentialScope>, SignedHeaders=<SignedHeaders>, Signature=<Signature>"
func buildTencentTC3Authorization(secretID, secretKey string, body []byte, signedAt time.Time) string {
	// 1. CanonicalRequest
	httpRequestMethod := "POST"
	canonicalURI := "/"
	canonicalQueryString := ""
	canonicalHeaders := buildCanonicalHeaders(map[string]string{
		"content-type": tencentCaptchaContentType,
		"host":         tencentCaptchaHost,
		// X-TC-Action 必须参与签名（小写）：腾讯云 2023 年起新增的强制要求。
		"x-tc-action": strings.ToLower(tencentCaptchaAction),
	})
	signedHeaders := "content-type;host;x-tc-action"
	hashedPayload := sha256Hex(body)
	canonicalRequest := strings.Join([]string{
		httpRequestMethod,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")

	// 2. StringToSign
	date := signedAt.Format("2006-01-02")
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, tencentCaptchaService)
	hashedCanonicalRequest := sha256Hex([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		tencentCaptchaSignAlgo,
		strconv.FormatInt(signedAt.Unix(), 10),
		credentialScope,
		hashedCanonicalRequest,
	}, "\n")

	// 3. SecretSigning
	secretDate := hmacSHA256([]byte("TC3"+secretKey), []byte(date))
	secretService := hmacSHA256(secretDate, []byte(tencentCaptchaService))
	secretSigning := hmacSHA256(secretService, []byte("tc3_request"))

	// 4. Signature
	signature := hex.EncodeToString(hmacSHA256(secretSigning, []byte(stringToSign)))

	// 5. Authorization
	return fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		tencentCaptchaSignAlgo,
		secretID, credentialScope,
		signedHeaders,
		signature,
	)
}

// buildCanonicalHeaders 按腾讯云规范拼装规范化 headers。
//
// 规则：
//   - header 名小写
//   - header 值 trim 空白
//   - 按 header 名字典序升序
//   - 每行 "name:value\n"
func buildCanonicalHeaders(headers map[string]string) string {
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		_, _ = sb.WriteString(k)
		_, _ = sb.WriteString(":")
		_, _ = sb.WriteString(strings.TrimSpace(headers[k]))
		_, _ = sb.WriteString("\n")
	}
	return sb.String()
}

// sha256Hex 返回 input 的 SHA-256 小写 hex。
func sha256Hex(input []byte) string {
	sum := sha256.Sum256(input)
	return hex.EncodeToString(sum[:])
}

// hmacSHA256 返回 HMAC-SHA256(key, data)，二进制形式（用于继续派生）。
func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(data)
	return h.Sum(nil)
}
