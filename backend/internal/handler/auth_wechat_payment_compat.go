package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// String mirrors of the legacy payment.PaymentType / payment.OrderType
// constants used by the WeChat OAuth payment-resume flow. The
// `internal/payment` package and most `internal/service/payment_*.go`
// files moved to `plugins/payment/`; these compat constants stay in core
// because the WeChat OAuth callback runs in the host process and cannot
// import the plugin.
const (
	paymentTypeWxpay             = "wxpay"
	paymentTypeWxpayDirect       = "wxpay_direct"
	paymentOrderTypeBalance      = "balance"
	paymentOrderTypeSubscription = "subscription"
)

// derivePaymentEncryptionKey reproduces the key payment.ProvideEncryptionKey
// derived from cfg.Totp.EncryptionKey. The WeChat OAuth callback still
// needs it to derive the legacy resume-token verification key during the
// migration window. Returns an error when the key isn't configured so the
// caller can degrade gracefully (no resume token minted).
func derivePaymentEncryptionKey(cfg *config.Config) ([]byte, error) {
	if cfg == nil {
		return nil, errors.New("payment encryption key not configured")
	}
	keyHex := strings.TrimSpace(cfg.Totp.EncryptionKey)
	if keyHex == "" {
		return nil, errors.New("payment encryption key not configured")
	}
	if !cfg.Totp.EncryptionKeyConfigured {
		return nil, errors.New("payment encryption key not explicitly configured")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid payment encryption key (hex decode): %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("payment encryption key must be 32 bytes, got %d", len(key))
	}
	return key, nil
}

// --- payment resume token (host side mint + parse) ----------------------
//
// The plugin owns the read-side resume flow for normal orders, but the
// WeChat OAuth callback still mints `wechat_resume_token` URL fragments
// in-host so the browser can pick them up before bouncing into the
// payment plugin. We keep just enough resume-token signing logic in the
// host to issue + verify those short-lived JWS-style tokens. The plugin
// duplicates this logic on its side using the same signing key.

const (
	paymentResumeSigningKeyEnv     = "PAYMENT_RESUME_SIGNING_KEY"
	wechatPaymentResumeTokenType   = "wechat_payment_resume"
	wechatPaymentResumeTokenTTL    = 15 * time.Minute
	invalidWeChatResumeTokenCode   = "INVALID_WECHAT_PAYMENT_RESUME_TOKEN"
	paymentResumeNotConfiguredCode = "PAYMENT_RESUME_NOT_CONFIGURED"
	paymentResumeNotConfiguredMsg  = "payment resume tokens require a configured signing key"
)

// WeChatPaymentResumeClaims mirrors the legacy
// service.WeChatPaymentResumeClaims so the OAuth handler and tests can
// keep using the same struct shape.
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

// paymentResumeService is a slimmed-down replacement for
// service.PaymentResumeService scoped to the WeChat OAuth callback's
// needs (no order-bound ResumeTokenClaims, no provider lookups).
type paymentResumeService struct {
	signingKey []byte
	verifyKeys [][]byte
}

func newPaymentResumeService(signingKey []byte, verifyFallbacks ...[]byte) *paymentResumeService {
	svc := &paymentResumeService{}
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

// newLegacyAwarePaymentResumeService chooses an explicit
// PAYMENT_RESUME_SIGNING_KEY from env over the legacy TOTP-derived key,
// keeping the legacy key as a verify-only fallback when both differ.
func newLegacyAwarePaymentResumeService(legacyKey []byte) *paymentResumeService {
	signingKey, verifyFallbacks := resolvePaymentResumeSigningKeys(legacyKey)
	return newPaymentResumeService(signingKey, verifyFallbacks...)
}

func resolvePaymentResumeSigningKeys(legacyKey []byte) ([]byte, [][]byte) {
	signingKey := parsePaymentResumeSigningKey(os.Getenv(paymentResumeSigningKeyEnv))
	if len(signingKey) == 0 {
		if len(legacyKey) == 0 {
			return nil, nil
		}
		return legacyKey, nil
	}
	if len(legacyKey) == 0 || bytes.Equal(legacyKey, signingKey) {
		return signingKey, nil
	}
	return signingKey, [][]byte{legacyKey}
}

func parsePaymentResumeSigningKey(raw string) []byte {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if len(raw) >= 64 && len(raw)%2 == 0 {
		if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) > 0 {
			return decoded
		}
	}
	return []byte(raw)
}

func (s *paymentResumeService) isSigningConfigured() bool {
	return s != nil && len(s.signingKey) > 0
}

func (s *paymentResumeService) ensureSigningKey() error {
	if s.isSigningConfigured() {
		return nil
	}
	return infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMsg)
}

func (s *paymentResumeService) CreateWeChatPaymentResumeToken(claims WeChatPaymentResumeClaims) (string, error) {
	if err := s.ensureSigningKey(); err != nil {
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
	if claims.PaymentType == "" {
		claims.PaymentType = paymentTypeWxpay
	}
	if claims.OrderType == "" {
		claims.OrderType = paymentOrderTypeBalance
	}
	claims.TokenType = wechatPaymentResumeTokenType
	return s.createSignedToken(claims)
}

func (s *paymentResumeService) ParseWeChatPaymentResumeToken(token string) (*WeChatPaymentResumeClaims, error) {
	if err := s.ensureSigningKey(); err != nil {
		return nil, err
	}
	var claims WeChatPaymentResumeClaims
	if err := s.parseSignedToken(token, &claims); err != nil {
		return nil, infraerrors.BadRequest(invalidWeChatResumeTokenCode, "wechat payment resume token payload is invalid")
	}
	if claims.TokenType != wechatPaymentResumeTokenType {
		return nil, infraerrors.BadRequest(invalidWeChatResumeTokenCode, "wechat payment resume token type mismatch")
	}
	claims.OpenID = strings.TrimSpace(claims.OpenID)
	if claims.OpenID == "" {
		return nil, infraerrors.BadRequest(invalidWeChatResumeTokenCode, "wechat payment resume token missing openid")
	}
	if claims.ExpiresAt > 0 && time.Now().Unix() > claims.ExpiresAt {
		return nil, infraerrors.BadRequest(invalidWeChatResumeTokenCode, "wechat payment resume token has expired")
	}
	if claims.PaymentType == "" {
		claims.PaymentType = paymentTypeWxpay
	}
	if claims.OrderType == "" {
		claims.OrderType = paymentOrderTypeBalance
	}
	return &claims, nil
}

func (s *paymentResumeService) createSignedToken(claims any) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal resume claims: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return encodedPayload + "." + s.sign(encodedPayload), nil
}

func (s *paymentResumeService) parseSignedToken(token string, dest any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return infraerrors.BadRequest(invalidWeChatResumeTokenCode, "resume token is malformed")
	}
	if !s.verifySignature(parts[0], parts[1]) {
		return infraerrors.BadRequest(invalidWeChatResumeTokenCode, "resume token signature mismatch")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return infraerrors.BadRequest(invalidWeChatResumeTokenCode, "resume token payload is malformed")
	}
	return json.Unmarshal(payload, dest)
}

func (s *paymentResumeService) verifySignature(payload string, signature string) bool {
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

func (s *paymentResumeService) sign(payload string) string {
	return signPaymentResumePayload(payload, s.signingKey)
}

func signPaymentResumePayload(payload string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
