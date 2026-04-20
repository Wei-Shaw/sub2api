package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	PaymentSourceHostedRedirect    = "hosted_redirect"
	PaymentSourceWechatInAppResume = "wechat_in_app_resume"

	paymentResumeFallbackSigningKey = "sub2api-payment-resume"

	SettingPaymentVisibleMethodAlipaySource  = "payment_visible_method_alipay_source"
	SettingPaymentVisibleMethodWxpaySource   = "payment_visible_method_wxpay_source"
	SettingPaymentVisibleMethodAlipayEnabled = "payment_visible_method_alipay_enabled"
	SettingPaymentVisibleMethodWxpayEnabled  = "payment_visible_method_wxpay_enabled"

	VisibleMethodSourceOfficialAlipay = "official_alipay"
	VisibleMethodSourceEasyPayAlipay  = "easypay_alipay"
	VisibleMethodSourceOfficialWechat = "official_wxpay"
	VisibleMethodSourceEasyPayWechat  = "easypay_wxpay"
)

type ResumeTokenClaims struct {
	OrderID            int64  `json:"oid"`
	UserID             int64  `json:"uid,omitempty"`
	ProviderInstanceID string `json:"pi,omitempty"`
	ProviderKey        string `json:"pk,omitempty"`
	PaymentType        string `json:"pt,omitempty"`
	CanonicalReturnURL string `json:"ru,omitempty"`
	IssuedAt           int64  `json:"iat"`
REDACTED

type PaymentResumeService struct {
	signingKey []byte
REDACTED

type visibleMethodLoadBalancer struct {
	inner         payment.LoadBalancer
	configService *PaymentConfigService
REDACTED

func NewPaymentResumeService(signingKey []byte) *PaymentResumeService {
	return &PaymentResumeService{signingKey: signingKeyREDACTED
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
	if inner == nil || configService == nil || configService.settingRepo == nil {
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

	enabledKey := visibleMethodEnabledSettingKey(visibleMethod)
	sourceKey := visibleMethodSourceSettingKey(visibleMethod)
	vals, err := lb.configService.settingRepo.GetMultiple(ctx, []string{enabledKey, sourceKeyREDACTED)
	if err != nil {
		return nil, fmt.Errorf("load visible method routing for %s: %w", visibleMethod, err)
REDACTED
	if vals[enabledKey] != "true" {
		return nil, fmt.Errorf("visible payment method %s is disabled", visibleMethod)
REDACTED

	targetProviderKey, ok := VisibleMethodProviderKeyForSource(visibleMethod, vals[sourceKey])
	if !ok {
		return nil, fmt.Errorf("visible payment method %s has no valid source", visibleMethod)
REDACTED
	return lb.inner.SelectInstance(ctx, targetProviderKey, paymentType, strategy, orderAmount)
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

func CanonicalizeReturnURL(raw string) (string, error) {
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
	return parsed.String(), nil
REDACTED

func buildPaymentReturnURL(base string, orderID int64, resumeToken string) (string, error) {
	canonical, err := CanonicalizeReturnURL(base)
	if err != nil || canonical == "" {
		return canonical, err
REDACTED

	parsed, err := url.Parse(canonical)
	if err != nil {
		return "", infraerrors.BadRequest("INVALID_RETURN_URL", "return_url must be a valid URL")
REDACTED

	query := parsed.Query()
	if orderID > 0 {
		query.Set("order_id", strconv.FormatInt(orderID, 10))
REDACTED
	if strings.TrimSpace(resumeToken) != "" {
		query.Set("resume_token", strings.TrimSpace(resumeToken))
REDACTED
	query.Set("status", "success")
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
REDACTED

func (s *PaymentResumeService) CreateToken(claims ResumeTokenClaims) (string, error) {
	if claims.OrderID <= 0 {
		return "", fmt.Errorf("resume token requires order id")
REDACTED
	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
REDACTED
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal resume claims: %w", err)
REDACTED
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return encodedPayload + "." + s.sign(encodedPayload), nil
REDACTED

func (s *PaymentResumeService) ParseToken(token string) (*ResumeTokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token is malformed")
REDACTED
	if !hmac.Equal([]byte(parts[1]), []byte(s.sign(parts[0]))) {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token signature mismatch")
REDACTED
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token payload is malformed")
REDACTED
	var claims ResumeTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token payload is invalid")
REDACTED
	if claims.OrderID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token missing order id")
REDACTED
	return &claims, nil
REDACTED

func (s *PaymentResumeService) sign(payload string) string {
	key := s.signingKey
	if len(key) == 0 {
		key = []byte(paymentResumeFallbackSigningKey)
REDACTED
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
REDACTED
