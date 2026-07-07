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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func TestNormalizeVisibleMethods(t *testing.T) {
	t.Parallel()

	got := NormalizeVisibleMethods([]string{
		"alipay_direct",
		"alipay",
		" wxpay_direct ",
		"wxpay",
		"stripe",
		"ldc",
REDACTED)

	want := []string{"alipay", "wxpay", "stripe", "ldc"REDACTED
	if len(got) != len(want) {
		t.Fatalf("NormalizeVisibleMethods len = %d, want %d (%v)", len(got), len(want), got)
REDACTED
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("NormalizeVisibleMethods[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
	REDACTED
REDACTED
REDACTED

func TestEnabledVisibleMethodsForEasyPayIncludesCustomSupportedTypes(t *testing.T) {
	t.Parallel()

	got := enabledVisibleMethodsForProvider(payment.TypeEasyPay, "alipay,ldc,usdt_trc20")
	want := []string{"alipay", "ldc", "usdt_trc20"REDACTED
	if len(got) != len(want) {
		t.Fatalf("enabledVisibleMethodsForProvider len = %d, want %d (%v)", len(got), len(want), got)
REDACTED
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("enabledVisibleMethodsForProvider[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
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

	got, err := CanonicalizeReturnURL("https://example.com/payment/result?b=2#a", "example.com", "")
	if err != nil {
		t.Fatalf("CanonicalizeReturnURL returned error: %v", err)
REDACTED
	if got != "https://example.com/payment/result?b=2" {
		t.Fatalf("CanonicalizeReturnURL = %q, want %q", got, "https://example.com/payment/result?b=2")
REDACTED
REDACTED

func TestCanonicalizeReturnURLRejectsRelativeURL(t *testing.T) {
	t.Parallel()

	if _, err := CanonicalizeReturnURL("/payment/result", "example.com", ""); err == nil {
		t.Fatal("CanonicalizeReturnURL should reject relative URLs")
REDACTED
REDACTED

func TestCanonicalizeReturnURLRejectsExternalHost(t *testing.T) {
	t.Parallel()

	if _, err := CanonicalizeReturnURL("https://evil.example/payment/result", "app.example.com", ""); err == nil {
		t.Fatal("CanonicalizeReturnURL should reject external hosts")
REDACTED
REDACTED

func TestCanonicalizeReturnURLAllowsConfiguredFrontendHost(t *testing.T) {
	t.Parallel()

	got, err := CanonicalizeReturnURL(
		"https://app.example.com/payment/result?from=checkout",
		"api.example.com",
		"https://app.example.com/purchase",
	)
	if err != nil {
		t.Fatalf("CanonicalizeReturnURL returned error: %v", err)
REDACTED
	if got != "https://app.example.com/payment/result?from=checkout" {
		t.Fatalf("CanonicalizeReturnURL = %q, want %q", got, "https://app.example.com/payment/result?from=checkout")
REDACTED
REDACTED

func TestCanonicalizeReturnURLRejectsNonCanonicalPath(t *testing.T) {
	t.Parallel()

	if _, err := CanonicalizeReturnURL("https://app.example.com/orders/42", "app.example.com", ""); err == nil {
		t.Fatal("CanonicalizeReturnURL should reject non-canonical result paths")
REDACTED
REDACTED

func TestBuildPaymentReturnURL(t *testing.T) {
	t.Parallel()

	got, err := buildPaymentReturnURL("https://example.com/payment/result?from=checkout#fragment", 42, "sub2_42", "resume-token")
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
	if query.Get("out_trade_no") != "sub2_42" {
		t.Fatalf("out_trade_no = %q", query.Get("out_trade_no"))
REDACTED
	if query.Get("resume_token") != "resume-token" {
		t.Fatalf("resume_token = %q", query.Get("resume_token"))
REDACTED
	if query.Get("status") != "success" {
		t.Fatalf("status = %q", query.Get("status"))
REDACTED
REDACTED

func TestBuildPaymentReturnURLWithoutResumeTokenStillIncludesOutTradeNo(t *testing.T) {
	t.Parallel()

	got, err := buildPaymentReturnURL("https://example.com/payment/result", 42, "sub2_42", "")
	if err != nil {
		t.Fatalf("buildPaymentReturnURL returned error: %v", err)
REDACTED

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
REDACTED
	query := parsed.Query()
	if query.Get("order_id") != "42" {
		t.Fatalf("order_id = %q", query.Get("order_id"))
REDACTED
	if query.Get("out_trade_no") != "sub2_42" {
		t.Fatalf("out_trade_no = %q", query.Get("out_trade_no"))
REDACTED
	if query.Get("resume_token") != "" {
		t.Fatalf("resume_token = %q, want empty", query.Get("resume_token"))
REDACTED
REDACTED

func TestBuildPaymentReturnURLEmptyBase(t *testing.T) {
	t.Parallel()

	got, err := buildPaymentReturnURL("", 42, "sub2_42", "resume-token")
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

func TestParseTokenRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	svc := NewPaymentResumeService([]byte("REDACTED"))
	token, err := svc.CreateToken(ResumeTokenClaims{
		OrderID:   42,
		UserID:    7,
		IssuedAt:  time.Now().Add(-25 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
REDACTED)
	if err != nil {
		t.Fatalf("CreateToken returned error: %v", err)
REDACTED

	_, err = svc.ParseToken(token)
	if err == nil {
		t.Fatal("ParseToken should reject expired tokens")
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

func TestParseWeChatPaymentResumeTokenRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	svc := NewPaymentResumeService([]byte("REDACTED"))
	token, err := svc.CreateWeChatPaymentResumeToken(WeChatPaymentResumeClaims{
		OpenID:      "openid-123",
		PaymentType: payment.TypeWxpay,
		IssuedAt:    time.Now().Add(-30 * time.Minute).Unix(),
		ExpiresAt:   time.Now().Add(-1 * time.Minute).Unix(),
REDACTED)
	if err != nil {
		t.Fatalf("CreateWeChatPaymentResumeToken returned error: %v", err)
REDACTED

	_, err = svc.ParseWeChatPaymentResumeToken(token)
	if err == nil {
		t.Fatal("ParseWeChatPaymentResumeToken should reject expired tokens")
REDACTED
REDACTED

func TestPaymentServiceParseWeChatPaymentResumeTokenUsesExplicitSigningKey(t *testing.T) {
	t.Setenv("PAYMENT_RESUME_SIGNING_KEY", "explicit-payment-resume-signing-key")

	token, err := NewPaymentResumeService([]byte("explicit-payment-resume-signing-key")).CreateWeChatPaymentResumeToken(WeChatPaymentResumeClaims{
		OpenID:      "openid-explicit-key",
		PaymentType: payment.TypeWxpay,
REDACTED)
	if err != nil {
		t.Fatalf("CreateWeChatPaymentResumeToken returned error: %v", err)
REDACTED

	svc := &PaymentService{
		configService: &PaymentConfigService{
			encryptionKey: []byte("REDACTED"),
	REDACTED,
REDACTED

	claims, err := svc.ParseWeChatPaymentResumeToken(token)
	if err != nil {
		t.Fatalf("ParseWeChatPaymentResumeToken returned error: %v", err)
REDACTED
	if claims.OpenID != "openid-explicit-key" {
		t.Fatalf("openid = %q, want %q", claims.OpenID, "openid-explicit-key")
REDACTED
REDACTED

func TestPaymentServiceParseWeChatPaymentResumeTokenAcceptsLegacyEncryptionKeyDuringMigration(t *testing.T) {
	t.Setenv("PAYMENT_RESUME_SIGNING_KEY", "explicit-payment-resume-signing-key")

	legacyKey := []byte("REDACTED")
	token, err := NewPaymentResumeService(legacyKey).CreateWeChatPaymentResumeToken(WeChatPaymentResumeClaims{
		OpenID:      "openid-legacy-key",
		PaymentType: payment.TypeWxpay,
REDACTED)
	if err != nil {
		t.Fatalf("CreateWeChatPaymentResumeToken returned error: %v", err)
REDACTED

	svc := &PaymentService{
		configService: &PaymentConfigService{
			encryptionKey: legacyKey,
	REDACTED,
REDACTED

	claims, err := svc.ParseWeChatPaymentResumeToken(token)
	if err != nil {
		t.Fatalf("ParseWeChatPaymentResumeToken returned error: %v", err)
REDACTED
	if claims.OpenID != "openid-legacy-key" {
		t.Fatalf("openid = %q, want %q", claims.OpenID, "openid-legacy-key")
REDACTED
REDACTED

func TestNewConfiguredPaymentResumeServicePrefersExplicitSigningKeyAndKeepsLegacyVerificationFallback(t *testing.T) {
	t.Setenv("PAYMENT_RESUME_SIGNING_KEY", "explicit-payment-resume-signing-key")

	legacyKey := []byte("REDACTED")
	svc := newLegacyAwarePaymentResumeService(legacyKey)

	explicitToken, err := svc.CreateWeChatPaymentResumeToken(WeChatPaymentResumeClaims{
		OpenID:      "openid-explicit-key",
		PaymentType: payment.TypeWxpay,
REDACTED)
	if err != nil {
		t.Fatalf("CreateWeChatPaymentResumeToken returned error: %v", err)
REDACTED

	explicitClaims, err := NewPaymentResumeService([]byte("explicit-payment-resume-signing-key")).ParseWeChatPaymentResumeToken(explicitToken)
	if err != nil {
		t.Fatalf("ParseWeChatPaymentResumeToken returned error: %v", err)
REDACTED
	if explicitClaims.OpenID != "openid-explicit-key" {
		t.Fatalf("openid = %q, want %q", explicitClaims.OpenID, "openid-explicit-key")
REDACTED

	legacyToken, err := NewPaymentResumeService(legacyKey).CreateWeChatPaymentResumeToken(WeChatPaymentResumeClaims{
		OpenID:      "openid-legacy-key",
		PaymentType: payment.TypeWxpay,
REDACTED)
	if err != nil {
		t.Fatalf("CreateWeChatPaymentResumeToken returned error: %v", err)
REDACTED

	legacyClaims, err := svc.ParseWeChatPaymentResumeToken(legacyToken)
	if err != nil {
		t.Fatalf("ParseWeChatPaymentResumeToken returned error: %v", err)
REDACTED
	if legacyClaims.OpenID != "openid-legacy-key" {
		t.Fatalf("openid = %q, want %q", legacyClaims.OpenID, "openid-legacy-key")
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

func TestVisibleMethodLoadBalancerUsesEnabledProviderInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("Official Alipay").
		SetConfig("{REDACTED").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetSortOrder(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("create alipay provider: %v", err)
REDACTED

	inner := &captureLoadBalancer{REDACTED
	configService := &PaymentConfigService{
		entClient: client,
REDACTED
	lb := newVisibleMethodLoadBalancer(inner, configService)

	_, err = lb.SelectInstance(ctx, "", payment.TypeAlipay, payment.StrategyRoundRobin, 12.5)
	if err != nil {
		t.Fatalf("SelectInstance returned error: %v", err)
REDACTED
	if inner.lastProviderKey != payment.TypeAlipay {
		t.Fatalf("lastProviderKey = %q, want %q", inner.lastProviderKey, payment.TypeAlipay)
REDACTED
REDACTED

func TestVisibleMethodLoadBalancerUsesConfiguredSourceWhenMultipleProvidersEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		method        payment.PaymentType
		officialName  string
		officialTypes string
		easyPayName   string
		easyPayTypes  string
		sourceSetting string
		wantProvider  string
REDACTED{
		{
			name:          "alipay uses official source",
			method:        payment.TypeAlipay,
			officialName:  "Official Alipay",
			officialTypes: "alipay",
			easyPayName:   "EasyPay Alipay",
			easyPayTypes:  "alipay",
			sourceSetting: VisibleMethodSourceOfficialAlipay,
			wantProvider:  payment.TypeAlipay,
	REDACTED,
		{
			name:          "alipay uses easypay source",
			method:        payment.TypeAlipay,
			officialName:  "Official Alipay",
			officialTypes: "alipay",
			easyPayName:   "EasyPay Alipay",
			easyPayTypes:  "alipay",
			sourceSetting: VisibleMethodSourceEasyPayAlipay,
			wantProvider:  payment.TypeEasyPay,
	REDACTED,
		{
			name:          "wxpay uses official source",
			method:        payment.TypeWxpay,
			officialName:  "Official WeChat",
			officialTypes: "wxpay",
			easyPayName:   "EasyPay WeChat",
			easyPayTypes:  "wxpay",
			sourceSetting: VisibleMethodSourceOfficialWechat,
			wantProvider:  payment.TypeWxpay,
	REDACTED,
		{
			name:          "wxpay uses easypay source",
			method:        payment.TypeWxpay,
			officialName:  "Official WeChat",
			officialTypes: "wxpay",
			easyPayName:   "EasyPay WeChat",
			easyPayTypes:  "wxpay",
			sourceSetting: VisibleMethodSourceEasyPayWechat,
			wantProvider:  payment.TypeEasyPay,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)

			officialProviderKey := payment.TypeAlipay
			if tt.method == payment.TypeWxpay {
				officialProviderKey = payment.TypeWxpay
		REDACTED

			_, err := client.PaymentProviderInstance.Create().
				SetProviderKey(officialProviderKey).
				SetName(tt.officialName).
				SetConfig("{REDACTED").
				SetSupportedTypes(tt.officialTypes).
				SetEnabled(true).
				SetSortOrder(1).
				Save(ctx)
			if err != nil {
				t.Fatalf("create official provider: %v", err)
		REDACTED

			_, err = client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeEasyPay).
				SetName(tt.easyPayName).
				SetConfig("{REDACTED").
				SetSupportedTypes(tt.easyPayTypes).
				SetEnabled(true).
				SetSortOrder(2).
				Save(ctx)
			if err != nil {
				t.Fatalf("create easypay provider: %v", err)
		REDACTED

			inner := &captureLoadBalancer{REDACTED
			configService := &PaymentConfigService{
				entClient: client,
				settingRepo: &paymentConfigSettingRepoStub{
					values: map[string]string{
						visibleMethodSourceSettingKey(tt.method): tt.sourceSetting,
				REDACTED,
			REDACTED,
		REDACTED
			lb := newVisibleMethodLoadBalancer(inner, configService)

			_, err = lb.SelectInstance(ctx, "", tt.method, payment.StrategyRoundRobin, 12.5)
			if err != nil {
				t.Fatalf("SelectInstance returned error: %v", err)
		REDACTED
			if inner.lastProviderKey != tt.wantProvider {
				t.Fatalf("lastProviderKey = %q, want %q", inner.lastProviderKey, tt.wantProvider)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestVisibleMethodLoadBalancerPreservesLegacyCrossProviderRoutingWhenSourceMissing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("Official Alipay").
		SetConfig("{REDACTED").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetSortOrder(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("create official provider: %v", err)
REDACTED

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeEasyPay).
		SetName("EasyPay Alipay").
		SetConfig("{REDACTED").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetSortOrder(2).
		Save(ctx)
	if err != nil {
		t.Fatalf("create easypay provider: %v", err)
REDACTED

	inner := &captureLoadBalancer{REDACTED
	configService := &PaymentConfigService{
		entClient: client,
		settingRepo: &paymentConfigSettingRepoStub{
			values: map[string]string{
				visibleMethodSourceSettingKey(payment.TypeAlipay): "",
		REDACTED,
	REDACTED,
REDACTED
	lb := newVisibleMethodLoadBalancer(inner, configService)

	_, err = lb.SelectInstance(ctx, "", payment.TypeAlipay, payment.StrategyRoundRobin, 9.9)
	if err != nil {
		t.Fatalf("SelectInstance returned error: %v", err)
REDACTED
	if inner.lastProviderKey != "" {
		t.Fatalf("lastProviderKey = %q, want legacy cross-provider empty key", inner.lastProviderKey)
REDACTED
	if inner.lastPaymentType != payment.TypeAlipay {
		t.Fatalf("lastPaymentType = %q, want %q", inner.lastPaymentType, payment.TypeAlipay)
REDACTED
REDACTED

func TestVisibleMethodLoadBalancerRejectsInvalidSourceWhenMultipleProvidersEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		method      payment.PaymentType
		sourceValue string
		wantMessage string
REDACTED{
		{
			name:        "invalid wxpay source",
			method:      payment.TypeWxpay,
			sourceValue: "stripe",
			wantMessage: "wxpay source must be one of the supported payment providers",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)

			officialProviderKey := payment.TypeAlipay
			officialSupportedTypes := "alipay"
			officialName := "Official Alipay"
			easyPaySupportedTypes := "alipay"
			easyPayName := "EasyPay Alipay"
			if tt.method == payment.TypeWxpay {
				officialProviderKey = payment.TypeWxpay
				officialSupportedTypes = "wxpay"
				officialName = "Official WeChat"
				easyPaySupportedTypes = "wxpay"
				easyPayName = "EasyPay WeChat"
		REDACTED

			_, err := client.PaymentProviderInstance.Create().
				SetProviderKey(officialProviderKey).
				SetName(officialName).
				SetConfig("{REDACTED").
				SetSupportedTypes(officialSupportedTypes).
				SetEnabled(true).
				SetSortOrder(1).
				Save(ctx)
			if err != nil {
				t.Fatalf("create official provider: %v", err)
		REDACTED

			_, err = client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeEasyPay).
				SetName(easyPayName).
				SetConfig("{REDACTED").
				SetSupportedTypes(easyPaySupportedTypes).
				SetEnabled(true).
				SetSortOrder(2).
				Save(ctx)
			if err != nil {
				t.Fatalf("create easypay provider: %v", err)
		REDACTED

			inner := &captureLoadBalancer{REDACTED
			configService := &PaymentConfigService{
				entClient: client,
				settingRepo: &paymentConfigSettingRepoStub{
					values: map[string]string{
						visibleMethodSourceSettingKey(tt.method): tt.sourceValue,
				REDACTED,
			REDACTED,
		REDACTED
			lb := newVisibleMethodLoadBalancer(inner, configService)

			_, err = lb.SelectInstance(ctx, "", tt.method, payment.StrategyRoundRobin, 9.9)
			if err == nil {
				t.Fatal("SelectInstance should reject invalid visible method source configuration")
		REDACTED
			if infraerrors.Reason(err) != "INVALID_PAYMENT_VISIBLE_METHOD_SOURCE" {
				t.Fatalf("Reason(err) = %q, want %q", infraerrors.Reason(err), "INVALID_PAYMENT_VISIBLE_METHOD_SOURCE")
		REDACTED
			if infraerrors.Message(err) != tt.wantMessage {
				t.Fatalf("Message(err) = %q, want %q", infraerrors.Message(err), tt.wantMessage)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestVisibleMethodLoadBalancerRejectsMissingEnabledVisibleMethodProvider(t *testing.T) {
	t.Parallel()

	inner := &captureLoadBalancer{REDACTED
	configService := &PaymentConfigService{
		entClient: newPaymentConfigServiceTestClient(t),
REDACTED
	lb := newVisibleMethodLoadBalancer(inner, configService)

	if _, err := lb.SelectInstance(context.Background(), "", payment.TypeWxpay, payment.StrategyRoundRobin, 9.9); err == nil {
		t.Fatal("SelectInstance should reject when no enabled provider instance exists")
REDACTED
REDACTED

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
	mac := hmac.New(sha256.New, []byte("sub2api-payment-resume"))
	_, _ = mac.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encodedPayload + "." + signature
REDACTED
