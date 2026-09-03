package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const paymentResultReturnPath = "/payment/result"

const (
	PaymentSourceHostedRedirect = "hosted_redirect"

	paymentResumeNotConfiguredCode    = "PAYMENT_RESUME_NOT_CONFIGURED"
	paymentResumeNotConfiguredMessage = "payment resume tokens require a configured signing key"

	paymentResumeTokenTTL = 24 * time.Hour
)

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

type PaymentResumeService struct {
	signingKey []byte
	verifyKeys [][]byte
}

func NewPaymentResumeService(signingKey []byte, verifyFallbacks ...[]byte) *PaymentResumeService {
	svc := &PaymentResumeService{}
	if len(signingKey) > 0 {
		svc.signingKey = append([]byte(nil), signingKey...)
		svc.verifyKeys = append(svc.verifyKeys, svc.signingKey)
	}
	for _, fallback := range verifyFallbacks {
		if len(fallback) == 0 {
			continue
		}
		cloned := append([]byte(nil), fallback...)
		duplicate := false
		for _, existing := range svc.verifyKeys {
			if bytes.Equal(existing, cloned) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			svc.verifyKeys = append(svc.verifyKeys, cloned)
		}
	}
	return svc
}

func (s *PaymentResumeService) isSigningConfigured() bool {
	return s != nil && len(s.signingKey) > 0
}

func (s *PaymentResumeService) ensureSigningKey() error {
	if s.isSigningConfigured() {
		return nil
	}
	return infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
}

// NormalizeVisibleMethod 归一化用户可见的支付方式标识。
//
// 这里刻意不折叠成网关键：sepay_bank_transfer / sepay_napas / sepay_card 是三个
// 独立的可见方式，折叠成 sepay 会让下单时选不中用户点的那一个。
func NormalizeVisibleMethod(method string) string {
	return strings.TrimSpace(method)
}

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

func NormalizePaymentSource(source string) string {
	switch strings.TrimSpace(strings.ToLower(source)) {
	case "", PaymentSourceHostedRedirect:
		return PaymentSourceHostedRedirect
	default:
		return strings.TrimSpace(strings.ToLower(source))
	}
}

func CanonicalizeReturnURL(raw string, srcHost string, srcURL string) (string, error) {
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

func allowedReturnURLHost(returnURLHost string, requestHost string, refererURL string) bool {
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

func buildPaymentReturnURL(base string, orderID int64, outTradeNo string, resumeToken string) (string, error) {
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

func sameOriginHost(returnURLHost string, requestHost string) bool {
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

func (s *PaymentResumeService) CreateToken(claims ResumeTokenClaims) (string, error) {
	if err := s.ensureSigningKey(); err != nil {
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
	return s.createSignedToken(claims)
}

func (s *PaymentResumeService) ParseToken(token string) (*ResumeTokenClaims, error) {
	if err := s.ensureSigningKey(); err != nil {
		return nil, err
	}
	var claims ResumeTokenClaims
	if err := s.parseSignedToken(token, &claims); err != nil {
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

func (s *PaymentResumeService) createSignedToken(claims any) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal resume claims: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return encodedPayload + "." + s.sign(encodedPayload), nil
}

func (s *PaymentResumeService) parseSignedToken(token string, dest any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token is malformed")
	}
	if !s.verifySignature(parts[0], parts[1]) {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token signature mismatch")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token payload is malformed")
	}
	return json.Unmarshal(payload, dest)
}

func (s *PaymentResumeService) verifySignature(payload string, signature string) bool {
	if s == nil {
		return false
	}
	for _, key := range s.verifyKeys {
		if hmac.Equal([]byte(signature), []byte(signPaymentResumePayload(payload, key))) {
			return true
		}
	}
	return false
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

func (s *PaymentResumeService) sign(payload string) string {
	return signPaymentResumePayload(payload, s.signingKey)
}

func signPaymentResumePayload(payload string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
