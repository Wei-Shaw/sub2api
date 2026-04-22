package service

import (
	"bytes"
	"context"
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

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const paymentResultReturnPath = "/payment/result"

const (
	PaymentSourceHostedRedirect    = "hosted_redirect"
	PaymentSourceWechatInAppResume = "wechat_in_app_resume"

	SettingPaymentVisibleMethodAlipaySource  = "payment_visible_method_alipay_source"
	SettingPaymentVisibleMethodWxpaySource   = "payment_visible_method_wxpay_source"
	SettingPaymentVisibleMethodAlipayEnabled = "payment_visible_method_alipay_enabled"
	SettingPaymentVisibleMethodWxpayEnabled  = "payment_visible_method_wxpay_enabled"

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

type ResumeTokenClaims struct {
	OrderID            int64  `json:"oid"`
	UserID             int64  `json:"uid,omitempty"`
	ProviderInstanceID string `json:"pi,omitempty"`
	ProviderKey        string `json:"pk,omitempty"`
	PaymentType        string `json:"pt,omitempty"`
	CanonicalReturnURL string `json:"ru,omitempty"`
	IssuedAt           int64  `json:"iat"`
	ExpiresAt          int64  `json:"exp,omitempty"`
REDACTED

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
REDACTED

type PaymentResumeService struct {
	signingKey []byte
	verifyKeys [][]byte
REDACTED

type visibleMethodLoadBalancer struct {
	inner         payment.LoadBalancer
	configService *PaymentConfigService
REDACTED

func NewPaymentResumeService(signingKey []byte, verifyFallbacks ...[]byte) *PaymentResumeService {
	svc := &PaymentResumeService{REDACTED
	if len(signingKey) > 0 {
		svc.signingKey = append([]byte(nil), signingKey...)
		svc.verifyKeys = append(svc.verifyKeys, svc.signingKey)
REDACTED
	for _, fallback := range verifyFallbacks {
		if len(fallback) == 0 {
			continue
	REDACTED
		cloned := append([]byte(nil), fallback...)
		duplicate := false
		for _, existing := range svc.verifyKeys {
			if bytes.Equal(existing, cloned) {
				duplicate = true
				break
		REDACTED
	REDACTED
		if !duplicate {
			svc.verifyKeys = append(svc.verifyKeys, cloned)
	REDACTED
REDACTED
	return svc
REDACTED

func (s *PaymentResumeService) isSigningConfigured() bool {
	return s != nil && len(s.signingKey) > 0
REDACTED

func (s *PaymentResumeService) ensureSigningKey() error {
	if s.isSigningConfigured() {
		return nil
REDACTED
	return infraerrors.ServiceUnavailable(paymentResumeNotConfiguredCode, paymentResumeNotConfiguredMessage)
REDACTED

func NormalizeVisibleMethod(method string) string {
	return payment.GetBasePaymentType(strings.TrimSpace(method))
REDACTED

func NormalizeVisibleMethods(methods []string) []string {
	if len(methods) == 0 {
		return nil
REDACTED
	seen := make(map[string]struct{REDACTED, len(methods))
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		normalized := NormalizeVisibleMethod(method)
		if normalized == "" {
			continue
	REDACTED
		if _, ok := seen[normalized]; ok {
			continue
	REDACTED
		seen[normalized] = struct{REDACTED{REDACTED
		out = append(out, normalized)
REDACTED
	return out
REDACTED

func NormalizePaymentSource(source string) string {
	switch strings.TrimSpace(strings.ToLower(source)) {
	case "", PaymentSourceHostedRedirect:
		return PaymentSourceHostedRedirect
	case "wechat_in_app", "wxpay_resume", PaymentSourceWechatInAppResume:
		return PaymentSourceWechatInAppResume
	default:
		return strings.TrimSpace(strings.ToLower(source))
REDACTED
REDACTED

func NormalizeVisibleMethodSource(method, source string) string {
	switch NormalizeVisibleMethod(method) {
	case payment.TypeAlipay:
		switch strings.TrimSpace(strings.ToLower(source)) {
		case VisibleMethodSourceOfficialAlipay, payment.TypeAlipay, payment.TypeAlipayDirect, "official":
			return VisibleMethodSourceOfficialAlipay
		case VisibleMethodSourceEasyPayAlipay, payment.TypeEasyPay:
			return VisibleMethodSourceEasyPayAlipay
	REDACTED
	case payment.TypeWxpay:
		switch strings.TrimSpace(strings.ToLower(source)) {
		case VisibleMethodSourceOfficialWechat, payment.TypeWxpay, payment.TypeWxpayDirect, "wechat", "official":
			return VisibleMethodSourceOfficialWechat
		case VisibleMethodSourceEasyPayWechat, payment.TypeEasyPay:
			return VisibleMethodSourceEasyPayWechat
	REDACTED
REDACTED
	return ""
REDACTED

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
REDACTED
REDACTED

func newVisibleMethodLoadBalancer(inner payment.LoadBalancer, configService *PaymentConfigService) payment.LoadBalancer {
	if inner == nil || configService == nil || configService.entClient == nil {
		return inner
REDACTED
	return &visibleMethodLoadBalancer{inner: inner, configService: configServiceREDACTED
REDACTED

func (lb *visibleMethodLoadBalancer) GetInstanceConfig(ctx context.Context, instanceID int64) (map[string]string, error) {
	return lb.inner.GetInstanceConfig(ctx, instanceID)
REDACTED

func (lb *visibleMethodLoadBalancer) SelectInstance(ctx context.Context, providerKey string, paymentType payment.PaymentType, strategy payment.Strategy, orderAmount float64) (*payment.InstanceSelection, error) {
	visibleMethod := NormalizeVisibleMethod(paymentType)
	if providerKey != "" || (visibleMethod != payment.TypeAlipay && visibleMethod != payment.TypeWxpay) {
		return lb.inner.SelectInstance(ctx, providerKey, paymentType, strategy, orderAmount)
REDACTED

	inst, err := lb.configService.resolveEnabledVisibleMethodInstance(ctx, visibleMethod)
	if err != nil {
		return nil, err
REDACTED
	if inst == nil {
		return nil, fmt.Errorf("visible payment method %s has no enabled provider instance", visibleMethod)
REDACTED
	return lb.inner.SelectInstance(ctx, inst.ProviderKey, paymentType, strategy, orderAmount)
REDACTED

func visibleMethodEnabledSettingKey(method string) string {
	switch NormalizeVisibleMethod(method) {
	case payment.TypeAlipay:
		return SettingPaymentVisibleMethodAlipayEnabled
	case payment.TypeWxpay:
		return SettingPaymentVisibleMethodWxpayEnabled
	default:
		return ""
REDACTED
REDACTED

func visibleMethodSourceSettingKey(method string) string {
	switch NormalizeVisibleMethod(method) {
	case payment.TypeAlipay:
		return SettingPaymentVisibleMethodAlipaySource
	case payment.TypeWxpay:
		return SettingPaymentVisibleMethodWxpaySource
	default:
		return ""
REDACTED
REDACTED

func CanonicalizeReturnURL(raw string, srcHost string, srcURL string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
REDACTED
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be an absolute http/https URL")
REDACTED
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must use http or https")
REDACTED
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
REDACTED
	if parsed.Path != paymentResultReturnPath {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must target the canonical internal payment result page")
REDACTED
	if !allowedReturnURLHost(parsed.Host, srcHost, srcURL) {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must use the same host as the current site or browser origin")
REDACTED
	return parsed.String(), nil
REDACTED

func allowedReturnURLHost(returnURLHost string, requestHost string, refererURL string) bool {
	if sameOriginHost(returnURLHost, requestHost) {
		return true
REDACTED

	refererURL = strings.TrimSpace(refererURL)
	if refererURL == "" {
		return false
REDACTED
	parsedReferer, err := url.Parse(refererURL)
	if err != nil || parsedReferer.Host == "" {
		return false
REDACTED
	return sameOriginHost(returnURLHost, parsedReferer.Host)
REDACTED

func buildPaymentReturnURL(base string, orderID int64, outTradeNo string, resumeToken string) (string, error) {
	canonical := strings.TrimSpace(base)
	if canonical == "" {
		return "", nil
REDACTED

	parsed, err := url.Parse(canonical)
	if err != nil {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be a valid URL")
REDACTED
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be a valid absolute URL")
REDACTED
	parsed.Fragment = ""

	query := parsed.Query()
	if orderID > 0 {
		query.Set("order_id", strconv.FormatInt(orderID, 10))
REDACTED
	if strings.TrimSpace(outTradeNo) != "" {
		query.Set("out_trade_no", strings.TrimSpace(outTradeNo))
REDACTED
	if strings.TrimSpace(resumeToken) != "" {
		query.Set("resume_token", strings.TrimSpace(resumeToken))
REDACTED
	query.Set("status", "success")
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
REDACTED

func sameOriginHost(returnURLHost string, requestHost string) bool {
	returnHost := strings.TrimSpace(returnURLHost)
	reqHost := strings.TrimSpace(requestHost)
	if returnHost == "" || reqHost == "" {
		return false
REDACTED
	if strings.EqualFold(returnHost, reqHost) {
		return true
REDACTED

	returnName, returnPort := splitHostPortDefault(returnHost)
	reqName, reqPort := splitHostPortDefault(reqHost)
	if returnName == "" || reqName == "" {
		return false
REDACTED
	return strings.EqualFold(returnName, reqName) && returnPort == reqPort
REDACTED

func splitHostPortDefault(raw string) (string, string) {
	if host, port, err := net.SplitHostPort(raw); err == nil {
		return host, port
REDACTED
	return raw, ""
REDACTED

func (s *PaymentResumeService) CreateToken(claims ResumeTokenClaims) (string, error) {
	if err := s.ensureSigningKey(); err != nil {
		return "", err
REDACTED
	if claims.OrderID <= 0 {
		return "", fmt.Errorf("resume token requires order id")
REDACTED
	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
REDACTED
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = time.Now().Add(paymentResumeTokenTTL).Unix()
REDACTED
	return s.createSignedToken(claims)
REDACTED

func (s *PaymentResumeService) ParseToken(token string) (*ResumeTokenClaims, error) {
	if err := s.ensureSigningKey(); err != nil {
		return nil, err
REDACTED
	var claims ResumeTokenClaims
	if err := s.parseSignedToken(token, &claims); err != nil {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token payload is invalid")
REDACTED
	if claims.OrderID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token missing order id")
REDACTED
	if err := validatePaymentResumeExpiry(claims.ExpiresAt, "INVALID_RESUME_TOKEN", "resume token has expired"); err != nil {
		return nil, err
REDACTED
	return &claims, nil
REDACTED

func (s *PaymentResumeService) CreateWeChatPaymentResumeToken(claims WeChatPaymentResumeClaims) (string, error) {
	if err := s.ensureSigningKey(); err != nil {
		return "", err
REDACTED
	claims.OpenID = strings.TrimSpace(claims.OpenID)
	if claims.OpenID == "" {
		return "", fmt.Errorf("wechat payment resume token requires openid")
REDACTED
	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
REDACTED
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = time.Now().Add(wechatPaymentResumeTokenTTL).Unix()
REDACTED
	if normalized := NormalizeVisibleMethod(claims.PaymentType); normalized != "" {
		claims.PaymentType = normalized
REDACTED
	if claims.PaymentType == "" {
		claims.PaymentType = payment.TypeWxpay
REDACTED
	if claims.OrderType == "" {
		claims.OrderType = payment.OrderTypeBalance
REDACTED
	claims.TokenType = wechatPaymentResumeTokenType
	return s.createSignedToken(claims)
REDACTED

func (s *PaymentResumeService) ParseWeChatPaymentResumeToken(token string) (*WeChatPaymentResumeClaims, error) {
	if err := s.ensureSigningKey(); err != nil {
		return nil, err
REDACTED
	var claims WeChatPaymentResumeClaims
	if err := s.parseSignedToken(token, &claims); err != nil {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token payload is invalid")
REDACTED
	if claims.TokenType != wechatPaymentResumeTokenType {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token type mismatch")
REDACTED
	claims.OpenID = strings.TrimSpace(claims.OpenID)
	if claims.OpenID == "" {
		return nil, infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token missing openid")
REDACTED
	if err := validatePaymentResumeExpiry(claims.ExpiresAt, "INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token has expired"); err != nil {
		return nil, err
REDACTED
	if normalized := NormalizeVisibleMethod(claims.PaymentType); normalized != "" {
		claims.PaymentType = normalized
REDACTED
	if claims.PaymentType == "" {
		claims.PaymentType = payment.TypeWxpay
REDACTED
	if claims.OrderType == "" {
		claims.OrderType = payment.OrderTypeBalance
REDACTED
	return &claims, nil
REDACTED

func (s *PaymentResumeService) createSignedToken(claims any) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal resume claims: %w", err)
REDACTED
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return encodedPayload + "." + s.sign(encodedPayload), nil
REDACTED

func (s *PaymentResumeService) parseSignedToken(token string, dest any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token is malformed")
REDACTED
	if !s.verifySignature(parts[0], parts[1]) {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token signature mismatch")
REDACTED
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token payload is malformed")
REDACTED
	return json.Unmarshal(payload, dest)
REDACTED

func (s *PaymentResumeService) verifySignature(payload string, signature string) bool {
	if s == nil {
		return false
REDACTED
	for _, key := range s.verifyKeys {
		if hmac.Equal([]byte(signature), []byte(signPaymentResumePayload(payload, key))) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func validatePaymentResumeExpiry(expiresAt int64, code, message string) error {
	if expiresAt <= 0 {
		return nil
REDACTED
	if time.Now().Unix() > expiresAt {
		return infraerrors.BadRequest(code, message)
REDACTED
	return nil
REDACTED

func (s *PaymentResumeService) sign(payload string) string {
	return signPaymentResumePayload(payload, s.signingKey)
REDACTED

func signPaymentResumePayload(payload string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
REDACTED
