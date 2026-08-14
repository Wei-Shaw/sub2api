package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
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

// Resume tokens ride in the return URL we hand to a hosted checkout, and a
// provider will not necessarily store a long one: NOWPayments refuses an
// invoice whose success_url passes ~255 characters with an opaque HTTP 500.
// The token is therefore packed rather than written as signed JSON — the
// compact form is around 40 characters where the JSON one ran past 260.
const (
	paymentResumeTokenVersion = 1
	// A 96-bit tag is beyond forgery for a value that lives 24 hours and only
	// ever resolves one order.
	paymentResumeTokenMACLen = 12
	// The provider binding is stored as a digest because the token has no room
	// for three strings; 32 bits is enough to catch an accidental mismatch,
	// which is all the binding was ever a defence against.
	paymentResumeBindHashLen = 4
)

type ResumeTokenClaims struct {
	OrderID int64 `json:"oid"`
	UserID  int64 `json:"uid,omitempty"`
	// ProviderInstanceID, ProviderKey and PaymentType are only populated when a
	// legacy JSON token is parsed. Tokens minted now carry BindHash instead.
	ProviderInstanceID string `json:"pi,omitempty"`
	ProviderKey        string `json:"pk,omitempty"`
	PaymentType        string `json:"pt,omitempty"`
	IssuedAt           int64  `json:"iat"`
	ExpiresAt          int64  `json:"exp,omitempty"`
	BindHash           []byte `json:"-"`
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

// NormalizePaymentSource collapses historical payment-source spellings onto the
// single value still in use.
func NormalizePaymentSource(source string) string {
	source = strings.TrimSpace(strings.ToLower(source))
	if source == "" {
		return PaymentSourceHostedRedirect
	}
	return source
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
	return s.createCompactToken(claims)
}

func (s *PaymentResumeService) ParseToken(token string) (*ResumeTokenClaims, error) {
	if err := s.ensureSigningKey(); err != nil {
		return nil, err
	}
	claims, err := s.parseAnyToken(token)
	if err != nil {
		return nil, err
	}
	if claims.OrderID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token missing order id")
	}
	if err := validatePaymentResumeExpiry(claims.ExpiresAt, "INVALID_RESUME_TOKEN", "resume token has expired"); err != nil {
		return nil, err
	}
	return claims, nil
}

// parseAnyToken accepts both token formats. The signed-JSON form always carries
// a "." between payload and signature and the packed form never does, so the
// separator alone tells them apart — old tokens keep resolving until the last
// one issued before the change ages out.
func (s *PaymentResumeService) parseAnyToken(token string) (*ResumeTokenClaims, error) {
	if strings.Contains(token, ".") {
		var claims ResumeTokenClaims
		if err := s.parseSignedToken(token, &claims); err != nil {
			return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token payload is invalid")
		}
		return &claims, nil
	}
	return s.parseCompactToken(token)
}

// createCompactToken packs the claims into a version byte, three varints and a
// provider-binding digest, then appends a truncated HMAC over all of it.
func (s *PaymentResumeService) createCompactToken(claims ResumeTokenClaims) (string, error) {
	payload := make([]byte, 0, 32)
	payload = append(payload, paymentResumeTokenVersion)
	payload = binary.AppendUvarint(payload, uint64(claims.OrderID))
	payload = binary.AppendUvarint(payload, uint64(claims.UserID))
	payload = binary.AppendUvarint(payload, uint64(claims.ExpiresAt))
	payload = append(payload, paymentResumeBindHash(claims.ProviderKey, claims.ProviderInstanceID, claims.PaymentType)...)

	mac := hmac.New(sha256.New, s.signingKey)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(append(payload, mac.Sum(nil)[:paymentResumeTokenMACLen]...)), nil
}

func (s *PaymentResumeService) parseCompactToken(token string) (*ResumeTokenClaims, error) {
	malformed := infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token is malformed")
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	// One version byte, three varints of at least one byte each, the binding
	// digest and the tag.
	if err != nil || len(raw) < 4+paymentResumeBindHashLen+paymentResumeTokenMACLen {
		return nil, malformed
	}
	payload := raw[:len(raw)-paymentResumeTokenMACLen]
	if !s.verifyCompactSignature(payload, raw[len(raw)-paymentResumeTokenMACLen:]) {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token signature mismatch")
	}
	if payload[0] != paymentResumeTokenVersion {
		return nil, malformed
	}
	reader := bytes.NewReader(payload[1 : len(payload)-paymentResumeBindHashLen])
	orderID, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, malformed
	}
	userID, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, malformed
	}
	expiresAt, err := binary.ReadUvarint(reader)
	if err != nil || reader.Len() != 0 {
		return nil, malformed
	}
	return &ResumeTokenClaims{
		OrderID:   int64(orderID),
		UserID:    int64(userID),
		ExpiresAt: int64(expiresAt),
		BindHash:  payload[len(payload)-paymentResumeBindHashLen:],
	}, nil
}

func (s *PaymentResumeService) verifyCompactSignature(payload, presented []byte) bool {
	if s == nil {
		return false
	}
	for _, key := range s.verifyKeys {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write(payload)
		if hmac.Equal(presented, mac.Sum(nil)[:paymentResumeTokenMACLen]) {
			return true
		}
	}
	return false
}

// paymentResumeBindHash digests the provider identity a token was issued
// against, matching the case-insensitive comparison the string claims used.
func paymentResumeBindHash(providerKey, providerInstanceID, paymentType string) []byte {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.ToLower(strings.TrimSpace(providerKey)),
		strings.TrimSpace(providerInstanceID),
		strings.ToLower(strings.TrimSpace(paymentType)),
	}, "\x00")))
	return sum[:paymentResumeBindHashLen]
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
