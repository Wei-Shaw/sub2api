//go:build unit

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestNormalizeVisibleMethods(t *testing.T) {
	t.Parallel()

	got := NormalizeVisibleMethods([]string{
		"alipay_direct",
		"alipay",
		" wxpay_direct ",
		"wxpay",
		"stripe",
REDACTED)

	want := []string{"alipay", "wxpay", "stripe"REDACTED
	if len(got) != len(want) {
		t.Fatalf("NormalizeVisibleMethods len = %d, want %d (%v)", len(got), len(want), got)
REDACTED
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("NormalizeVisibleMethods[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
	REDACTED
REDACTED
REDACTED

func TestNormalizePaymentSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
REDACTED{
		{name: "empty uses default", input: "", expect: PaymentSourceHostedRedirectREDACTED,
		{name: "wechat alias normalized", input: "wechat_in_app", expect: PaymentSourceWechatInAppResumeREDACTED,
		{name: "canonical value preserved", input: PaymentSourceWechatInAppResume, expect: PaymentSourceWechatInAppResumeREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizePaymentSource(tt.input); got != tt.expect {
				t.Fatalf("NormalizePaymentSource(%q) = %q, want %q", tt.input, got, tt.expect)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestCanonicalizeReturnURL(t *testing.T) {
	t.Parallel()

	got, err := CanonicalizeReturnURL("https://example.com/pay/result?b=2#a")
	if err != nil {
		t.Fatalf("CanonicalizeReturnURL returned error: %v", err)
REDACTED
	if got != "https://example.com/pay/result?b=2" {
		t.Fatalf("CanonicalizeReturnURL = %q, want %q", got, "https://example.com/pay/result?b=2")
REDACTED
REDACTED

func TestCanonicalizeReturnURLRejectsRelativeURL(t *testing.T) {
	t.Parallel()

	if _, err := CanonicalizeReturnURL("/payment/result"); err == nil {
		t.Fatal("CanonicalizeReturnURL should reject relative URLs")
REDACTED
REDACTED

func TestBuildPaymentReturnURL(t *testing.T) {
	t.Parallel()

	got, err := buildPaymentReturnURL("https://example.com/payment/result?from=checkout#fragment", 42, "resume-token")
	if err != nil {
		t.Fatalf("buildPaymentReturnURL returned error: %v", err)
REDACTED

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
REDACTED
	if parsed.Fragment != "" {
		t.Fatalf("buildPaymentReturnURL should strip fragments, got %q", parsed.Fragment)
REDACTED
	query := parsed.Query()
	if query.Get("from") != "checkout" {
		t.Fatalf("expected original query to be preserved, got %q", query.Get("from"))
REDACTED
	if query.Get("order_id") != strconv.FormatInt(42, 10) {
		t.Fatalf("order_id = %q", query.Get("order_id"))
REDACTED
	if query.Get("resume_token") != "resume-token" {
		t.Fatalf("resume_token = %q", query.Get("resume_token"))
REDACTED
	if query.Get("status") != "success" {
		t.Fatalf("status = %q", query.Get("status"))
REDACTED
REDACTED

func TestBuildPaymentReturnURLEmptyBase(t *testing.T) {
	t.Parallel()

	got, err := buildPaymentReturnURL("", 42, "resume-token")
	if err != nil {
		t.Fatalf("buildPaymentReturnURL returned error: %v", err)
REDACTED
	if got != "" {
		t.Fatalf("buildPaymentReturnURL = %q, want empty string", got)
REDACTED
REDACTED

func TestPaymentResumeTokenRoundTrip(t *testing.T) {
	t.Parallel()

	svc := NewPaymentResumeService([]byte("REDACTED"))
	token, err := svc.CreateToken(ResumeTokenClaims{
		OrderID:            42,
		UserID:             7,
		ProviderInstanceID: "19",
		ProviderKey:        "easypay",
		PaymentType:        "wxpay",
		CanonicalReturnURL: "https://example.com/payment/result",
		IssuedAt:           1234567890,
REDACTED)
	if err != nil {
		t.Fatalf("CreateToken returned error: %v", err)
REDACTED

	claims, err := svc.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken returned error: %v", err)
REDACTED
	if claims.OrderID != 42 || claims.UserID != 7 {
		t.Fatalf("claims mismatch: %+v", claims)
REDACTED
	if claims.ProviderInstanceID != "19" || claims.ProviderKey != "easypay" || claims.PaymentType != "wxpay" {
		t.Fatalf("claims provider snapshot mismatch: %+v", claims)
REDACTED
	if claims.CanonicalReturnURL != "https://example.com/payment/result" {
		t.Fatalf("claims return URL = %q", claims.CanonicalReturnURL)
REDACTED
REDACTED

func TestCreateTokenRejectsMissingSigningKey(t *testing.T) {
	t.Parallel()

	svc := NewPaymentResumeService(nil)
	_, err := svc.CreateToken(ResumeTokenClaims{OrderID: 42REDACTED)
	if err == nil {
		t.Fatal("CreateToken should reject missing signing key")
REDACTED
REDACTED

func TestParseTokenRejectsFallbackSignedTokenWhenSigningKeyMissing(t *testing.T) {
	t.Parallel()

	token := mustCreateFallbackSignedToken(t, ResumeTokenClaims{OrderID: 42, UserID: 7REDACTED)
	svc := NewPaymentResumeService(nil)
	_, err := svc.ParseToken(token)
	if err == nil {
		t.Fatal("ParseToken should reject tokens when signing key is missing")
REDACTED
REDACTED

func TestWeChatPaymentResumeTokenRoundTrip(t *testing.T) {
	t.Parallel()

	svc := NewPaymentResumeService([]byte("REDACTED"))
	token, err := svc.CreateWeChatPaymentResumeToken(WeChatPaymentResumeClaims{
		OpenID:      "openid-123",
		PaymentType: payment.TypeWxpay,
		Amount:      "12.50",
		OrderType:   payment.OrderTypeSubscription,
		PlanID:      7,
		RedirectTo:  "/purchase?from=wechat",
		Scope:       "snsapi_base",
		IssuedAt:    1234567890,
REDACTED)
	if err != nil {
		t.Fatalf("CreateWeChatPaymentResumeToken returned error: %v", err)
REDACTED

	claims, err := svc.ParseWeChatPaymentResumeToken(token)
	if err != nil {
		t.Fatalf("ParseWeChatPaymentResumeToken returned error: %v", err)
REDACTED
	if claims.OpenID != "openid-123" || claims.PaymentType != payment.TypeWxpay {
		t.Fatalf("claims mismatch: %+v", claims)
REDACTED
	if claims.Amount != "12.50" || claims.OrderType != payment.OrderTypeSubscription || claims.PlanID != 7 {
		t.Fatalf("claims payment context mismatch: %+v", claims)
REDACTED
	if claims.RedirectTo != "/purchase?from=wechat" || claims.Scope != "snsapi_base" {
		t.Fatalf("claims redirect/scope mismatch: %+v", claims)
REDACTED
REDACTED

func TestCreateWeChatPaymentResumeTokenRejectsMissingSigningKey(t *testing.T) {
	t.Parallel()

	svc := NewPaymentResumeService(nil)
	_, err := svc.CreateWeChatPaymentResumeToken(WeChatPaymentResumeClaims{OpenID: "openid-123"REDACTED)
	if err == nil {
		t.Fatal("CreateWeChatPaymentResumeToken should reject missing signing key")
REDACTED
REDACTED

func TestParseWeChatPaymentResumeTokenRejectsFallbackSignedTokenWhenSigningKeyMissing(t *testing.T) {
	t.Parallel()

	token := mustCreateFallbackSignedToken(t, WeChatPaymentResumeClaims{
		TokenType:   wechatPaymentResumeTokenType,
		OpenID:      "openid-123",
		PaymentType: payment.TypeWxpay,
REDACTED)
	svc := NewPaymentResumeService(nil)
	_, err := svc.ParseWeChatPaymentResumeToken(token)
	if err == nil {
		t.Fatal("ParseWeChatPaymentResumeToken should reject tokens when signing key is missing")
REDACTED
REDACTED

func TestNormalizeVisibleMethodSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		input  string
		want   string
REDACTED{
		{name: "alipay official alias", method: payment.TypeAlipay, input: "alipay", want: VisibleMethodSourceOfficialAlipayREDACTED,
		{name: "alipay easypay alias", method: payment.TypeAlipay, input: "easypay", want: VisibleMethodSourceEasyPayAlipayREDACTED,
		{name: "wxpay official alias", method: payment.TypeWxpay, input: "wxpay", want: VisibleMethodSourceOfficialWechatREDACTED,
		{name: "wxpay easypay alias", method: payment.TypeWxpay, input: "easypay", want: VisibleMethodSourceEasyPayWechatREDACTED,
		{name: "unsupported source", method: payment.TypeWxpay, input: "stripe", want: ""REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeVisibleMethodSource(tt.method, tt.input); got != tt.want {
				t.Fatalf("NormalizeVisibleMethodSource(%q, %q) = %q, want %q", tt.method, tt.input, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestVisibleMethodProviderKeyForSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		source string
		want   string
		ok     bool
REDACTED{
		{name: "official alipay", method: payment.TypeAlipay, source: VisibleMethodSourceOfficialAlipay, want: payment.TypeAlipay, ok: trueREDACTED,
		{name: "easypay alipay", method: payment.TypeAlipay, source: VisibleMethodSourceEasyPayAlipay, want: payment.TypeEasyPay, ok: trueREDACTED,
		{name: "official wechat", method: payment.TypeWxpay, source: VisibleMethodSourceOfficialWechat, want: payment.TypeWxpay, ok: trueREDACTED,
		{name: "easypay wechat", method: payment.TypeWxpay, source: VisibleMethodSourceEasyPayWechat, want: payment.TypeEasyPay, ok: trueREDACTED,
		{name: "mismatched method and source", method: payment.TypeAlipay, source: VisibleMethodSourceOfficialWechat, want: "", ok: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := VisibleMethodProviderKeyForSource(tt.method, tt.source)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("VisibleMethodProviderKeyForSource(%q, %q) = (%q, %v), want (%q, %v)", tt.method, tt.source, got, ok, tt.want, tt.ok)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestVisibleMethodLoadBalancerUsesConfiguredSource(t *testing.T) {
	t.Parallel()

	inner := &captureLoadBalancer{REDACTED
	configService := &PaymentConfigService{
		settingRepo: &paymentSettingRepoStub{
			values: map[string]string{
				SettingPaymentVisibleMethodAlipayEnabled: "true",
				SettingPaymentVisibleMethodAlipaySource:  VisibleMethodSourceOfficialAlipay,
		REDACTED,
	REDACTED,
REDACTED
	lb := newVisibleMethodLoadBalancer(inner, configService)

	_, err := lb.SelectInstance(context.Background(), "", payment.TypeAlipay, payment.StrategyRoundRobin, 12.5)
	if err != nil {
		t.Fatalf("SelectInstance returned error: %v", err)
REDACTED
	if inner.lastProviderKey != payment.TypeAlipay {
		t.Fatalf("lastProviderKey = %q, want %q", inner.lastProviderKey, payment.TypeAlipay)
REDACTED
REDACTED

func TestVisibleMethodLoadBalancerRejectsDisabledVisibleMethod(t *testing.T) {
	t.Parallel()

	inner := &captureLoadBalancer{REDACTED
	configService := &PaymentConfigService{
		settingRepo: &paymentSettingRepoStub{
			values: map[string]string{
				SettingPaymentVisibleMethodWxpayEnabled: "false",
				SettingPaymentVisibleMethodWxpaySource:  VisibleMethodSourceOfficialWechat,
		REDACTED,
	REDACTED,
REDACTED
	lb := newVisibleMethodLoadBalancer(inner, configService)

	if _, err := lb.SelectInstance(context.Background(), "", payment.TypeWxpay, payment.StrategyRoundRobin, 9.9); err == nil {
		t.Fatal("SelectInstance should reject disabled visible method")
REDACTED
REDACTED

type paymentSettingRepoStub struct {
	values map[string]string
REDACTED

func (s *paymentSettingRepoStub) Get(context.Context, string) (*Setting, error) { return nil, nil REDACTED
func (s *paymentSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
REDACTED
func (s *paymentSettingRepoStub) Set(context.Context, string, string) error { return nil REDACTED
func (s *paymentSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = s.values[key]
REDACTED
	return out, nil
REDACTED
func (s *paymentSettingRepoStub) SetMultiple(context.Context, map[string]string) error { return nil REDACTED
func (s *paymentSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
REDACTED
func (s *paymentSettingRepoStub) Delete(context.Context, string) error { return nil REDACTED

type captureLoadBalancer struct {
	lastProviderKey string
	lastPaymentType string
REDACTED

func (c *captureLoadBalancer) GetInstanceConfig(context.Context, int64) (map[string]string, error) {
	return map[string]string{REDACTED, nil
REDACTED

func (c *captureLoadBalancer) SelectInstance(_ context.Context, providerKey string, paymentType payment.PaymentType, _ payment.Strategy, _ float64) (*payment.InstanceSelection, error) {
	c.lastProviderKey = providerKey
	c.lastPaymentType = paymentType
	return &payment.InstanceSelection{ProviderKey: providerKey, SupportedTypes: paymentTypeREDACTED, nil
REDACTED

func mustCreateFallbackSignedToken(t *testing.T, claims any) string {
REDACTED

	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
REDACTED
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(paymentResumeFallbackSigningKey))
	_, _ = mac.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encodedPayload + "." + signature
REDACTED
