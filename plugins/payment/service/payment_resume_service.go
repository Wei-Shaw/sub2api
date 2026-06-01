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
	"strings"
	"sync"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
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
// rotating the key invalidates all outstanding resume tokens unless the
// operator keeps the previous key on the comma-separated rotation list
// (see signingKeys for the parsing rules).
type PaymentResumeService struct {
	settings pluginsdk.SettingsClient

	mu sync.RWMutex
	// keys[0] is the primary key used for minting new tokens; the rest
	// are verify-only fallbacks kept around so tokens minted under a
	// previous key remain valid for their TTL after the operator
	// rotates. Empty until the first successful settings read.
	keys [][]byte
}

// NewPaymentResumeService constructs a resume service wired to the SDK
// settings client. The signing key is loaded lazily on first use and
// kept in a mutex-protected field; an admin-side settings rotation is
// picked up automatically via the SDK's settings cache TTL.
func NewPaymentResumeService(settings pluginsdk.SettingsClient) *PaymentResumeService {
	return &PaymentResumeService{settings: settings}
}

// resumeSigningMinKeyBytes is the minimum HMAC key length we accept.
// Matches the previous single-key implementation so a comma-separated
// rotation list cannot smuggle a shorter key past validation.
const resumeSigningMinKeyBytes = 16

// signingKeys returns every active HMAC key in priority order. keys[0]
// is the primary mint key; subsequent entries are verify-only fallbacks
// supplied by the operator for rotation. The first call fetches the
// setting from the SDK; subsequent calls return the cached slice until
// invalidateCachedKey is called.
//
// The setting value is parsed as a comma-separated list of hex strings
// so rotation works without a plugin restart: the operator writes
// "<new_hex>,<old_hex>" and tokens minted under either key keep
// verifying. Whitespace inside the list is trimmed; empty segments are
// dropped silently. A list that decodes to zero usable keys returns
// ServiceUnavailable just like an empty single-key value would.
func (s *PaymentResumeService) signingKeys(ctx context.Context) ([][]byte, error) {
	s.mu.RLock()
	if len(s.keys) > 0 {
		k := s.keys
		s.mu.RUnlock()
		return k, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.keys) > 0 {
		return s.keys, nil
	}
	if s.settings == nil {
		return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
	}
	// SETTINGS-V2 stores one row per key; the host SettingsExtension.Get
	// rejects empty keys with InvalidArgument. Always request the
	// specific setting (which is a JSON-encoded string).
	var rawKey string
	if err := s.settings.GetTyped(ctx, resumeSigningKeySettingKey, &rawKey); err != nil {
		if errors.Is(err, pluginsdk.ErrSettingNotFound) {
			return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
		}
		return nil, fmt.Errorf("read resume signing key: %w", err)
	}
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
	}

	parts := strings.Split(rawKey, ",")
	parsed := make([][]byte, 0, len(parts))
	for _, part := range parts {
		segment := strings.TrimSpace(part)
		if segment == "" {
			continue
		}
		decoded, err := hex.DecodeString(segment)
		if err != nil {
			return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode,
				"resume signing key is not valid hex")
		}
		if len(decoded) < resumeSigningMinKeyBytes {
			return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode,
				"resume signing key must be at least 16 bytes")
		}
		parsed = append(parsed, decoded)
	}
	if len(parsed) == 0 {
		return nil, infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
	}
	s.keys = parsed
	return s.keys, nil
}

// primarySigningKey returns the key used for minting new tokens. Always
// keys[0] from signingKeys.
func (s *PaymentResumeService) primarySigningKey(ctx context.Context) ([]byte, error) {
	keys, err := s.signingKeys(ctx)
	if err != nil {
		return nil, err
	}
	return keys[0], nil
}

// invalidateCachedKey is exposed for test harnesses that rotate the
// settings value mid-test. Production rotation is picked up via the
// settings watch loop in the SDK.
func (s *PaymentResumeService) invalidateCachedKey() {
	s.mu.Lock()
	s.keys = nil
	s.mu.Unlock()
}

// CreateToken signs claims and returns the resume token (payload.sig).
// Validates required fields and stamps timestamps when missing.
func (s *PaymentResumeService) CreateToken(ctx context.Context, claims ResumeTokenClaims) (string, error) {
	key, err := s.primarySigningKey(ctx)
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
// expired or malformed tokens with a 400 ApplicationError. Verification
// tries every configured key (primary + rotation fallbacks) so tokens
// minted under a previous key remain valid until they expire.
func (s *PaymentResumeService) ParseToken(ctx context.Context, token string) (*ResumeTokenClaims, error) {
	keys, err := s.signingKeys(ctx)
	if err != nil {
		return nil, err
	}
	var claims ResumeTokenClaims
	if err := parseSignedTokenAnyKey(keys, token, &claims); err != nil {
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
	key, err := s.primarySigningKey(ctx)
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
// Verification tries every configured key (primary + rotation fallbacks).
func (s *PaymentResumeService) ParseWeChatPaymentResumeToken(ctx context.Context, token string) (*WeChatPaymentResumeClaims, error) {
	keys, err := s.signingKeys(ctx)
	if err != nil {
		return nil, err
	}
	var claims WeChatPaymentResumeClaims
	if err := parseSignedTokenAnyKey(keys, token, &claims); err != nil {
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

// parseSignedTokenAnyKey verifies token against every key in keys
// (primary + rotation fallbacks) and decodes the payload into dest on
// the first signature match. Returns the same malformed/mismatch errors
// as parseSignedToken when no key validates so callers do not have to
// reason about per-key failure modes.
func parseSignedTokenAnyKey(keys [][]byte, token string, dest any) error {
	if len(keys) == 0 {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token signature mismatch")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token is malformed")
	}
	signature := []byte(parts[1])
	matched := false
	for _, key := range keys {
		expected := signPaymentResumePayload(parts[0], key)
		if hmac.Equal(signature, []byte(expected)) {
			matched = true
			break
		}
	}
	if !matched {
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
