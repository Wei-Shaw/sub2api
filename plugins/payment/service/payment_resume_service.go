package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
)

const paymentResultReturnPath = "/payment/result"

// Visible-method / payment-source constants. These mirror the values
// stored in plugin settings (see internal/settings/settings_schema.json
// keys visible_method_*_source / visible_method_*_enabled).
const (
	PaymentSourceHostedRedirect    = "hosted_redirect"
	PaymentSourceWechatInAppResume = "wechat_in_app_resume"

	VisibleMethodSourceOfficialAlipay = "official_alipay"
	VisibleMethodSourceEasyPayAlipay  = "easypay_alipay"
	VisibleMethodSourceOfficialWechat = "official_wxpay"
	VisibleMethodSourceEasyPayWechat  = "easypay_wxpay"

	wechatPaymentResumeTokenType = "wechat_payment_resume"

	paymentResumeNotConfiguredCode    = "PAYMENT_RESUME_NOT_CONFIGURED"
	paymentResumeNotConfiguredMessage = "payment resume tokens require a configured signing key"

	paymentResumeTokenTTL       = 24 * time.Hour
	wechatPaymentResumeTokenTTL = 15 * time.Minute
)

// resumeSigningKeySettingKey is the plugin settings key holding the hex-
// encoded HMAC signing key. Operators rotate the key by writing a new
// value via the host's plugin settings page.
const resumeSigningKeySettingKey = "resume_signing_key_hex"

// ResumeTokenClaims is the resume-token payload signed for the standard
// hosted-redirect resume flow.
type ResumeTokenClaims struct {
	OrderID            int64  `json:"oid"`
	UserID             int64  `json:"uid,omitempty"`
	ProviderInstanceID string `json:"pi,omitempty"`
	ProviderKey        string `json:"pk,omitempty"`
	PaymentType        string `json:"pt,omitempty"`
	CanonicalReturnURL string `json:"ru,omitempty"`
	IssuedAt           int64  `json:"iat"`
	ExpiresAt          int64  `json:"exp,omitempty"`
}

// WeChatPaymentResumeClaims is the resume-token payload used when an
// in-WeChat browser switches OAuth scopes mid-flow (jsapi/openid).
type WeChatPaymentResumeClaims struct {
	TokenType   string `json:"tk,omitempty"`
	OpenID      string `json:"openid"`
	PaymentType string `json:"pt,omitempty"`
	Amount      string `json:"amt,omitempty"`
	OrderType   string `json:"ot,omitempty"`
	PlanID      int64  `json:"pid,omitempty"`
	RedirectTo  string `json:"rd,omitempty"`
	Scope       string `json:"scp,omitempty"`
	IssuedAt    int64  `json:"iat"`
	ExpiresAt   int64  `json:"exp,omitempty"`
}

// PaymentResumeService creates / validates HMAC-signed resume tokens.
// The signing key is sourced from plugin settings (resume_signing_key_hex);
// rotating the key invalidates all outstanding resume tokens, which is
// the desired property for a security-sensitive bearer credential.
type PaymentResumeService struct {
	settings pluginsdk.SettingsClient

	mu  sync.RWMutex
	key []byte
}

// NewPaymentResumeService constructs a resume service wired to the SDK
// settings client. The signing key is loaded lazily on first use and
// kept in a mutex-protected field; an admin-side settings rotation is
// picked up automatically via the SDK's settings cache TTL.
func NewPaymentResumeService(settings pluginsdk.SettingsClient) *PaymentResumeService {
	return &PaymentResumeService{settings: settings}
}

// signingKey returns the active HMAC key, fetching it from settings on
// first call. Returns ServiceUnavailable when no key is configured so
// the caller can surface a clear error to the operator.
func (s *PaymentResumeService) signingKey(ctx context.Context) ([]byte, error) {
	s.mu.RLock()
	if len(s.key) > 0 {
		k := s.key
		s.mu.RUnlock()
		return k, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.key) > 0 {
		return s.key, nil
	}
	if s.settings == nil {
		return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
	}
	var raw struct {
		Key string `json:"resume_signing_key_hex"`
	}
	if err := s.settings.GetTyped(ctx, "", &raw); err != nil {
		if errors.Is(err, pluginsdk.ErrSettingNotFound) {
			return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
		}
		return nil, fmt.Errorf("read resume signing key: %w", err)
	}
	if strings.TrimSpace(raw.Key) == "" {
		return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(raw.Key))
	if err != nil {
		return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode,
			"resume signing key is not valid hex")
	}
	if len(decoded) < 16 {
		return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode,
			"resume signing key must be at least 16 bytes")
	}
	s.key = decoded
	return s.key, nil
}

// invalidateCachedKey is exposed for test harnesses that rotate the
// settings value mid-test. Production rotation is picked up via the
// settings watch loop in the SDK.
func (s *PaymentResumeService) invalidateCachedKey() {
	s.mu.Lock()
	s.key = nil
	s.mu.Unlock()
}

// NormalizeVisibleMethod collapses provider-specific aliases (alipay /
// alipay_direct / wxpay / wxpay_direct / easypay) onto the canonical
// alipay or wxpay payment-type token used in storage and settings.
func NormalizeVisibleMethod(method string) string {
	return payment.GetBasePaymentType(strings.TrimSpace(method))
}

// NormalizeVisibleMethods de-duplicates a slice of methods after
// normalisation. The empty string is filtered out.
func NormalizeVisibleMethods(methods []string) []string {
	if len(methods) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(methods))
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		normalized := NormalizeVisibleMethod(method)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

// NormalizePaymentSource maps free-form source strings onto the two
// canonical sources understood by the rest of the plugin.
func NormalizePaymentSource(source string) string {
	switch strings.TrimSpace(strings.ToLower(source)) {
	case "", PaymentSourceHostedRedirect:
		return PaymentSourceHostedRedirect
	case "wechat_in_app", "wxpay_resume", PaymentSourceWechatInAppResume:
		return PaymentSourceWechatInAppResume
	default:
		return strings.TrimSpace(strings.ToLower(source))
	}
}

// NormalizeVisibleMethodSource pairs a method (alipay/wxpay) with the
// "source" the operator enabled for it (official/easypay), returning the
// canonical compound key written to storage.
func NormalizeVisibleMethodSource(method, source string) string {
	switch NormalizeVisibleMethod(method) {
	case payment.TypeAlipay:
		switch strings.TrimSpace(strings.ToLower(source)) {
		case VisibleMethodSourceOfficialAlipay, payment.TypeAlipay, payment.TypeAlipayDirect, "official":
			return VisibleMethodSourceOfficialAlipay
		case VisibleMethodSourceEasyPayAlipay, payment.TypeEasyPay:
			return VisibleMethodSourceEasyPayAlipay
		}
	case payment.TypeWxpay:
		switch strings.TrimSpace(strings.ToLower(source)) {
		case VisibleMethodSourceOfficialWechat, payment.TypeWxpay, payment.TypeWxpayDirect, "wechat", "official":
			return VisibleMethodSourceOfficialWechat
		case VisibleMethodSourceEasyPayWechat, payment.TypeEasyPay:
			return VisibleMethodSourceEasyPayWechat
		}
	}
	return ""
}

// VisibleMethodProviderKeyForSource returns the provider-key the load
// balancer should use when an operator paired (method, source). The
// boolean indicates whether the requested method matches the canonical
// official base; false signals an indirect routing (e.g. easypay).
func VisibleMethodProviderKeyForSource(method, source string) (string, bool) {
	switch NormalizeVisibleMethodSource(method, source) {
	case VisibleMethodSourceOfficialAlipay:
		return payment.TypeAlipay, NormalizeVisibleMethod(method) == payment.TypeAlipay
	case VisibleMethodSourceEasyPayAlipay:
		return payment.TypeEasyPay, NormalizeVisibleMethod(method) == payment.TypeAlipay
	case VisibleMethodSourceOfficialWechat:
		return payment.TypeWxpay, NormalizeVisibleMethod(method) == payment.TypeWxpay
	case VisibleMethodSourceEasyPayWechat:
		return payment.TypeEasyPay, NormalizeVisibleMethod(method) == payment.TypeWxpay
	default:
		return "", false
	}
}

// CanonicalizeReturnURL validates a caller-supplied return_url against
// the same-origin policy and forces it onto the canonical /payment/result
// path. Returns the cleaned URL or a 400 ApplicationError.
func CanonicalizeReturnURL(raw, srcHost, srcURL string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be an absolute http/https URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must use http or https")
	}
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	if parsed.Path != paymentResultReturnPath {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must target the canonical internal payment result page")
	}
	if !allowedReturnURLHost(parsed.Host, srcHost, srcURL) {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must use the same host as the current site or browser origin")
	}
	return parsed.String(), nil
}

func allowedReturnURLHost(returnURLHost, requestHost, refererURL string) bool {
	if sameOriginHost(returnURLHost, requestHost) {
		return true
	}
	refererURL = strings.TrimSpace(refererURL)
	if refererURL == "" {
		return false
	}
	parsedReferer, err := url.Parse(refererURL)
	if err != nil || parsedReferer.Host == "" {
		return false
	}
	return sameOriginHost(returnURLHost, parsedReferer.Host)
}

func buildPaymentReturnURL(base string, orderID int64, outTradeNo, resumeToken string) (string, error) {
	canonical := strings.TrimSpace(base)
	if canonical == "" {
		return "", nil
	}
	parsed, err := url.Parse(canonical)
	if err != nil {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be a valid URL")
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be a valid absolute URL")
	}
	parsed.Fragment = ""
	query := parsed.Query()
	if orderID > 0 {
		query.Set("order_id", strconv.FormatInt(orderID, 10))
	}
	if strings.TrimSpace(outTradeNo) != "" {
		query.Set("out_trade_no", strings.TrimSpace(outTradeNo))
	}
	if strings.TrimSpace(resumeToken) != "" {
		query.Set("resume_token", strings.TrimSpace(resumeToken))
	}
	query.Set("status", "success")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func sameOriginHost(returnURLHost, requestHost string) bool {
	returnHost := strings.TrimSpace(returnURLHost)
	reqHost := strings.TrimSpace(requestHost)
	if returnHost == "" || reqHost == "" {
		return false
	}
	if strings.EqualFold(returnHost, reqHost) {
		return true
	}
	returnName, returnPort := splitHostPortDefault(returnHost)
	reqName, reqPort := splitHostPortDefault(reqHost)
	if returnName == "" || reqName == "" {
		return false
	}
	return strings.EqualFold(returnName, reqName) && returnPort == reqPort
}

func splitHostPortDefault(raw string) (string, string) {
	if host, port, err := net.SplitHostPort(raw); err == nil {
		return host, port
	}
	return raw, ""
}

// CreateToken signs claims and returns the resume token (payload.sig).
// Validates required fields and stamps timestamps when missing.
func (s *PaymentResumeService) CreateToken(ctx context.Context, claims ResumeTokenClaims) (string, error) {
	key, err := s.signingKey(ctx)
	if err != nil {
		return "", err
	}
	if claims.OrderID <= 0 {
		return "", fmt.Errorf("resume token requires order id")
	}
	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
	}
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = time.Now().Add(paymentResumeTokenTTL).Unix()
	}
	return createSignedToken(key, claims)
}

// ParseToken verifies the signature, decodes the payload, and rejects
// expired or malformed tokens with a 400 ApplicationError.
func (s *PaymentResumeService) ParseToken(ctx context.Context, token string) (*ResumeTokenClaims, error) {
	key, err := s.signingKey(ctx)
	if err != nil {
		return nil, err
	}
	var claims ResumeTokenClaims
	if err := parseSignedToken(key, token, &claims); err != nil {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token payload is invalid")
	}
	if claims.OrderID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token missing order id")
	}
	if err := validatePaymentResumeExpiry(claims.ExpiresAt, "INVALID_RESUME_TOKEN", "resume token has expired"); err != nil {
		return nil, err
	}
	return &claims, nil
}

// CreateWeChatPaymentResumeToken signs the in-WeChat resume payload.
func (s *PaymentResumeService) CreateWeChatPaymentResumeToken(ctx context.Context, claims WeChatPaymentResumeClaims) (string, error) {
	key, err := s.signingKey(ctx)
	if err != nil {
		return "", err
	}
	claims.OpenID = strings.TrimSpace(claims.OpenID)
	if claims.OpenID == "" {
		return "", fmt.Errorf("wechat payment resume token requires openid")
	}
	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
	}
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = time.Now().Add(wechatPaymentResumeTokenTTL).Unix()
	}
	if normalized := NormalizeVisibleMethod(claims.PaymentType); normalized != "" {
		claims.PaymentType = normalized
	}
	if claims.PaymentType == "" {
		claims.PaymentType = payment.TypeWxpay
	}
	if claims.OrderType == "" {
		claims.OrderType = payment.OrderTypeBalance
	}
	claims.TokenType = wechatPaymentResumeTokenType
	return createSignedToken(key, claims)
}

// ParseWeChatPaymentResumeToken validates and decodes the in-WeChat
// resume payload, rejecting tokens whose token-type field is wrong.
func (s *PaymentResumeService) ParseWeChatPaymentResumeToken(ctx context.Context, token string) (*WeChatPaymentResumeClaims, error) {
	key, err := s.signingKey(ctx)
	if err != nil {
		return nil, err
	}
	var claims WeChatPaymentResumeClaims
	if err := parseSignedToken(key, token, &claims); err != nil {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token payload is invalid")
	}
	if claims.TokenType != wechatPaymentResumeTokenType {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token type mismatch")
	}
	claims.OpenID = strings.TrimSpace(claims.OpenID)
	if claims.OpenID == "" {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token missing openid")
	}
	if err := validatePaymentResumeExpiry(claims.ExpiresAt, "INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token has expired"); err != nil {
		return nil, err
	}
	if normalized := NormalizeVisibleMethod(claims.PaymentType); normalized != "" {
		claims.PaymentType = normalized
	}
	if claims.PaymentType == "" {
		claims.PaymentType = payment.TypeWxpay
	}
	if claims.OrderType == "" {
		claims.OrderType = payment.OrderTypeBalance
	}
	return &claims, nil
}

// createSignedToken serialises claims to JSON, base64url-encodes it,
// HMAC-signs the encoded payload, and returns "payload.signature".
func createSignedToken(key []byte, claims any) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal resume claims: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return encodedPayload + "." + signPaymentResumePayload(encodedPayload, key), nil
}

func parseSignedToken(key []byte, token string, dest any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token is malformed")
	}
	expected := signPaymentResumePayload(parts[0], key)
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token signature mismatch")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token payload is malformed")
	}
	return json.Unmarshal(payload, dest)
}

func validatePaymentResumeExpiry(expiresAt int64, code, message string) error {
	if expiresAt <= 0 {
		return nil
	}
	if time.Now().Unix() > expiresAt {
		return infraerrors.BadRequest(code, message)
	}
	return nil
}

func signPaymentResumePayload(payload string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
