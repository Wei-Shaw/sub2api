//go:build unit

package provider

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/jsapi"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
)

// generateTestKeyPair returns a fresh RSA 2048 key pair as PEM strings.
// The wechatpay-go SDK expects PKCS8 private keys and PKIX public keys.
func generateTestKeyPair(t *testing.T) (privPEM, pubPEM string) {
REDACTED
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
REDACTED
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
REDACTED
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal pkix: %v", err)
REDACTED
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDERREDACTED)),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDERREDACTED))
REDACTED

func TestMapWxState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
REDACTED{
		{
			name:  "SUCCESS maps to paid",
			input: wxpayTradeStateSuccess,
			want:  payment.ProviderStatusPaid,
	REDACTED,
		{
			name:  "REFUND maps to refunded",
			input: wxpayTradeStateRefund,
			want:  payment.ProviderStatusRefunded,
	REDACTED,
		{
			name:  "CLOSED maps to failed",
			input: wxpayTradeStateClosed,
			want:  payment.ProviderStatusFailed,
	REDACTED,
		{
			name:  "PAYERROR maps to failed",
			input: wxpayTradeStatePayError,
			want:  payment.ProviderStatusFailed,
	REDACTED,
		{
			name:  "unknown state maps to pending",
			input: "NOTPAY",
			want:  payment.ProviderStatusPending,
	REDACTED,
		{
			name:  "empty string maps to pending",
			input: "",
			want:  payment.ProviderStatusPending,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mapWxState(tt.input)
			if got != tt.want {
				t.Errorf("mapWxState(%q) = %q, want %q", tt.input, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestWxSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input *string
		want  string
REDACTED{
		{
			name:  "nil pointer returns empty string",
			input: nil,
			want:  "",
	REDACTED,
		{
			name:  "non-nil pointer returns value",
			input: strPtr("hello"),
			want:  "hello",
	REDACTED,
		{
			name:  "pointer to empty string returns empty string",
			input: strPtr(""),
			want:  "",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := wxSV(tt.input)
			if got != tt.want {
				t.Errorf("wxSV() = %q, want %q", got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestBuildWxpayTransactionMetadata(t *testing.T) {
	t.Parallel()

	tx := &payments.Transaction{
		Appid:      strPtr("wx-app-id"),
		Mchid:      strPtr("mch-id"),
		TradeState: strPtr(wxpayTradeStateSuccess),
		Amount: &payments.TransactionAmount{
			Currency: strPtr(wxpayCurrency),
	REDACTED,
REDACTED

	metadata := buildWxpayTransactionMetadata(tx)
	if metadata[wxpayMetadataAppID] != "wx-app-id" {
		t.Fatalf("appid = %q", metadata[wxpayMetadataAppID])
REDACTED
	if metadata[wxpayMetadataMerchantID] != "mch-id" {
		t.Fatalf("mchid = %q", metadata[wxpayMetadataMerchantID])
REDACTED
	if metadata[wxpayMetadataCurrency] != wxpayCurrency {
		t.Fatalf("currency = %q", metadata[wxpayMetadataCurrency])
REDACTED
	if metadata[wxpayMetadataTradeState] != wxpayTradeStateSuccess {
		t.Fatalf("trade_state = %q", metadata[wxpayMetadataTradeState])
REDACTED
REDACTED

func strPtr(s string) *string {
	return &s
REDACTED

func TestFormatPEM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		keyType string
		want    string
REDACTED{
		{
			name:    "raw key gets wrapped with headers",
			key:     "MIIBIjANBgkqhki...",
			keyType: "PUBLIC KEY",
			want:    "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhki...\n-----END PUBLIC KEY-----",
	REDACTED,
		{
			name:    "already formatted key is returned as-is",
			key:     "REDACTED
REDACTED
REDACTED\nMIIEvQIBADANBg...\n-----END PRIVATE KEY-----",
	REDACTED,
		{
			name:    "key with leading/trailing whitespace is trimmed before check",
			key:     "  \n MIIBIjANBgkqhki...  \n ",
			keyType: "PUBLIC KEY",
			want:    "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhki...\n-----END PUBLIC KEY-----",
	REDACTED,
		{
			name:    "already formatted key with whitespace is trimmed and returned",
			key:     "  REDACTED
REDACTED
REDACTED\ndata\n-----END RSA PRIVATE KEY-----",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatPEM(tt.key, tt.keyType)
			if got != tt.want {
				t.Errorf("formatPEM(%q, %q) =\n%s\nwant:\n%s", tt.key, tt.keyType, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestNewWxpay(t *testing.T) {
	t.Parallel()

	privPEM, pubPEM := generateTestKeyPair(t)
	validConfig := map[string]string{
		"appId":       "wx1234567890",
		"mchId":       "1234567890",
		"privateKey":  privPEM,
		"apiV3Key":    "12345678901234567890123456789012", // exactly 32 bytes
		"publicKey":   pubPEM,
		"publicKeyId": "PUB_KEY_ID_TEST",
		"certSerial":  "SERIAL001",
REDACTED

	// helper to clone and override config fields
	withOverride := func(overrides map[string]string) map[string]string {
		cfg := make(map[string]string, len(validConfig))
		for k, v := range validConfig {
			cfg[k] = v
	REDACTED
		for k, v := range overrides {
			cfg[k] = v
	REDACTED
		return cfg
REDACTED

	tests := []struct {
		name      string
		config    map[string]string
		wantErr   bool
		errSubstr string
REDACTED{
		{
			name:    "valid config succeeds",
			config:  validConfig,
			wantErr: false,
	REDACTED,
		{
			name:      "missing appId",
			config:    withOverride(map[string]string{"appId": ""REDACTED),
			wantErr:   true,
			errSubstr: "appId",
	REDACTED,
		{
			name:      "missing mchId",
			config:    withOverride(map[string]string{"mchId": ""REDACTED),
			wantErr:   true,
			errSubstr: "mchId",
	REDACTED,
		{
			name:      "missing privateKey",
			config:    withOverride(map[string]string{"privateKey": ""REDACTED),
			wantErr:   true,
			errSubstr: "privateKey",
	REDACTED,
		{
			name:      "missing apiV3Key",
			config:    withOverride(map[string]string{"apiV3Key": ""REDACTED),
			wantErr:   true,
			errSubstr: "apiV3Key",
	REDACTED,
		{
			name:      "missing certSerial",
			config:    withOverride(map[string]string{"certSerial": ""REDACTED),
			wantErr:   true,
			errSubstr: "certSerial",
	REDACTED,
		{
			name:      "missing publicKey",
			config:    withOverride(map[string]string{"publicKey": ""REDACTED),
			wantErr:   true,
			errSubstr: "publicKey",
	REDACTED,
		{
			name:      "missing publicKeyId",
			config:    withOverride(map[string]string{"publicKeyId": ""REDACTED),
			wantErr:   true,
			errSubstr: "publicKeyId",
	REDACTED,
		{
			name:      "malformed privateKey PEM",
			config:    withOverride(map[string]string{"privateKey": "not-a-valid-pem"REDACTED),
			wantErr:   true,
			errSubstr: "WXPAY_CONFIG_INVALID_KEY",
	REDACTED,
		{
			name:      "malformed publicKey PEM",
			config:    withOverride(map[string]string{"publicKey": "not-a-valid-pem"REDACTED),
			wantErr:   true,
			errSubstr: "WXPAY_CONFIG_INVALID_KEY",
	REDACTED,
		{
			name:      "apiV3Key too short",
			config:    withOverride(map[string]string{"apiV3Key": "short"REDACTED),
			wantErr:   true,
			errSubstr: "WXPAY_CONFIG_INVALID_KEY_LENGTH",
	REDACTED,
		{
			name:      "apiV3Key too long",
			config:    withOverride(map[string]string{"apiV3Key": "123456789012345678901234567890123"REDACTED), // 33 bytes
			wantErr:   true,
			errSubstr: "WXPAY_CONFIG_INVALID_KEY_LENGTH",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewWxpay("test-instance", tt.config)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
			REDACTED
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errSubstr)
			REDACTED
				return
		REDACTED
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
		REDACTED
			if got == nil {
				t.Fatal("expected non-nil Wxpay instance")
		REDACTED
			if got.instanceID != "test-instance" {
				t.Errorf("instanceID = %q, want %q", got.instanceID, "test-instance")
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestBuildWxpayResultURLPreservesResumeToken(t *testing.T) {
	t.Parallel()

	resultURL, err := buildWxpayResultURL("https://app.example.com/payment/result?order_id=42&resume_token=resume-42&status=success", payment.CreatePaymentRequest{
		OrderID:     "sub2_42",
		PaymentType: payment.TypeWxpay,
REDACTED)
	if err != nil {
		t.Fatalf("buildWxpayResultURL returned error: %v", err)
REDACTED

	parsed, err := url.Parse(resultURL)
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
REDACTED
	query := parsed.Query()
	if parsed.Path != wxpayResultPath {
		t.Fatalf("path = %q, want %q", parsed.Path, wxpayResultPath)
REDACTED
	if query.Get("resume_token") != "resume-42" {
		t.Fatalf("resume_token = %q, want %q", query.Get("resume_token"), "resume-42")
REDACTED
	if query.Get("order_id") != "42" {
		t.Fatalf("order_id = %q, want %q", query.Get("order_id"), "42")
REDACTED
	if query.Get("out_trade_no") != "sub2_42" {
		t.Fatalf("out_trade_no = %q, want %q", query.Get("out_trade_no"), "sub2_42")
REDACTED
REDACTED

func TestResolveWxpayJSAPIAppID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config map[string]string
		want   string
REDACTED{
		{
			name: "prefers dedicated mp app id",
			config: map[string]string{
				"mpAppId": "wx-mp-app",
				"appId":   "wx-merchant-app",
		REDACTED,
			want: "wx-mp-app",
	REDACTED,
		{
			name: "falls back to merchant app id",
			config: map[string]string{
				"appId": "wx-merchant-app",
		REDACTED,
			want: "wx-merchant-app",
	REDACTED,
		{
			name:   "missing app ids returns empty",
			config: map[string]string{REDACTED,
			want:   "",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveWxpayJSAPIAppID(tt.config); got != tt.want {
				t.Fatalf("ResolveWxpayJSAPIAppID() = %q, want %q", got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestResolveWxpayCreateMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		req      payment.CreatePaymentRequest
		wantMode string
		wantErr  string
REDACTED{
		{
			name:     "desktop uses native",
			req:      payment.CreatePaymentRequest{REDACTED,
			wantMode: wxpayModeNative,
	REDACTED,
		{
			name: "mobile uses h5 when client ip is present",
			req: payment.CreatePaymentRequest{
				IsMobile: true,
				ClientIP: "203.0.113.10",
		REDACTED,
			wantMode: wxpayModeH5,
	REDACTED,
		{
			name: "mobile without client ip returns clear error",
			req: payment.CreatePaymentRequest{
				IsMobile: true,
		REDACTED,
			wantErr: "requires client IP",
	REDACTED,
		{
			name: "openid uses jsapi mode",
			req: payment.CreatePaymentRequest{
				OpenID: "openid-123",
		REDACTED,
			wantMode: wxpayModeJSAPI,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveWxpayCreateMode(tt.req)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
			REDACTED
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q should contain %q", err.Error(), tt.wantErr)
			REDACTED
				return
		REDACTED
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
		REDACTED
			if got != tt.wantMode {
				t.Fatalf("resolveWxpayCreateMode() = %q, want %q", got, tt.wantMode)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestCreatePaymentWithOpenIDReturnsJSAPIResult(t *testing.T) {
	origJSAPIPrepay := wxpayJSAPIPrepayWithRequestPayment
	origNativePrepay := wxpayNativePrepay
	origH5Prepay := wxpayH5Prepay
	t.Cleanup(func() {
		wxpayJSAPIPrepayWithRequestPayment = origJSAPIPrepay
		wxpayNativePrepay = origNativePrepay
		wxpayH5Prepay = origH5Prepay
REDACTED)

	jsapiCalls := 0
	nativeCalls := 0
	h5Calls := 0
	wxpayJSAPIPrepayWithRequestPayment = func(ctx context.Context, svc jsapi.JsapiApiService, req jsapi.PrepayRequest) (*jsapi.PrepayWithRequestPaymentResponse, *core.APIResult, error) {
		jsapiCalls++
		if got := wxSV(req.Payer.Openid); got != "openid-123" {
			t.Fatalf("openid = %q, want %q", got, "openid-123")
	REDACTED
		if req.SceneInfo == nil || wxSV(req.SceneInfo.PayerClientIp) != "203.0.113.10" {
			t.Fatalf("scene_info payer_client_ip = %q, want %q", wxSV(req.SceneInfo.PayerClientIp), "203.0.113.10")
	REDACTED
		return &jsapi.PrepayWithRequestPaymentResponse{
			Appid:     core.String("wx123"),
			TimeStamp: core.String("1712345678"),
			NonceStr:  core.String("nonce-123"),
			Package:   core.String("REDACTED"),
			SignType:  core.String("RSA"),
			PaySign:   core.String("signed-payload"),
	REDACTED, nil, nil
REDACTED
	wxpayNativePrepay = func(ctx context.Context, svc native.NativeApiService, req native.PrepayRequest) (*native.PrepayResponse, *core.APIResult, error) {
		nativeCalls++
		return &native.PrepayResponse{REDACTED, nil, nil
REDACTED
	wxpayH5Prepay = func(ctx context.Context, svc h5.H5ApiService, req h5.PrepayRequest) (*h5.PrepayResponse, *core.APIResult, error) {
		h5Calls++
		return &h5.PrepayResponse{REDACTED, nil, nil
REDACTED

	provider := &Wxpay{
		config: map[string]string{
			"appId": "wx123",
			"mchId": "mch123",
	REDACTED,
		coreClient: &core.Client{REDACTED,
REDACTED

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_88",
		Amount:      "66.88",
		PaymentType: payment.TypeWxpay,
		NotifyURL:   "https://merchant.example/payment/notify",
		OpenID:      "openid-123",
		ClientIP:    "203.0.113.10",
REDACTED)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
REDACTED
	if jsapiCalls != 1 {
		t.Fatalf("jsapi prepay calls = %d, want 1", jsapiCalls)
REDACTED
	if nativeCalls != 0 {
		t.Fatalf("native prepay calls = %d, want 0", nativeCalls)
REDACTED
	if h5Calls != 0 {
		t.Fatalf("h5 prepay calls = %d, want 0", h5Calls)
REDACTED
	if resp.ResultType != payment.CreatePaymentResultJSAPIReady {
		t.Fatalf("result type = %q, want %q", resp.ResultType, payment.CreatePaymentResultJSAPIReady)
REDACTED
	if resp.JSAPI == nil {
		t.Fatal("expected jsapi payload, got nil")
REDACTED
	if resp.JSAPI.AppID != "wx123" {
		t.Fatalf("jsapi appId = %q, want %q", resp.JSAPI.AppID, "wx123")
REDACTED
	if resp.JSAPI.TimeStamp != "1712345678" {
		t.Fatalf("jsapi timeStamp = %q, want %q", resp.JSAPI.TimeStamp, "1712345678")
REDACTED
	if resp.JSAPI.NonceStr != "nonce-123" {
		t.Fatalf("jsapi nonceStr = %q, want %q", resp.JSAPI.NonceStr, "nonce-123")
REDACTED
	if resp.JSAPI.Package != "REDACTED" {
		t.Fatalf("jsapi package = %q, want %q", resp.JSAPI.Package, "REDACTED")
REDACTED
	if resp.JSAPI.SignType != "RSA" {
		t.Fatalf("jsapi signType = %q, want %q", resp.JSAPI.SignType, "RSA")
REDACTED
	if resp.JSAPI.PaySign != "signed-payload" {
		t.Fatalf("jsapi paySign = %q, want %q", resp.JSAPI.PaySign, "signed-payload")
REDACTED
REDACTED

func TestCreatePaymentMobileH5IncludesConfiguredSceneInfo(t *testing.T) {
	origJSAPIPrepay := wxpayJSAPIPrepayWithRequestPayment
	origNativePrepay := wxpayNativePrepay
	origH5Prepay := wxpayH5Prepay
	t.Cleanup(func() {
		wxpayJSAPIPrepayWithRequestPayment = origJSAPIPrepay
		wxpayNativePrepay = origNativePrepay
		wxpayH5Prepay = origH5Prepay
REDACTED)

	jsapiCalls := 0
	nativeCalls := 0
	h5Calls := 0
	wxpayJSAPIPrepayWithRequestPayment = func(ctx context.Context, svc jsapi.JsapiApiService, req jsapi.PrepayRequest) (*jsapi.PrepayWithRequestPaymentResponse, *core.APIResult, error) {
		jsapiCalls++
		return &jsapi.PrepayWithRequestPaymentResponse{REDACTED, nil, nil
REDACTED
	wxpayNativePrepay = func(ctx context.Context, svc native.NativeApiService, req native.PrepayRequest) (*native.PrepayResponse, *core.APIResult, error) {
		nativeCalls++
		return &native.PrepayResponse{REDACTED, nil, nil
REDACTED
	wxpayH5Prepay = func(ctx context.Context, svc h5.H5ApiService, req h5.PrepayRequest) (*h5.PrepayResponse, *core.APIResult, error) {
		h5Calls++
		if req.SceneInfo == nil {
			t.Fatal("expected scene_info, got nil")
	REDACTED
		if got := wxSV(req.SceneInfo.PayerClientIp); got != "203.0.113.10" {
			t.Fatalf("scene_info payer_client_ip = %q, want %q", got, "203.0.113.10")
	REDACTED
		if req.SceneInfo.H5Info == nil {
			t.Fatal("expected scene_info.h5_info, got nil")
	REDACTED
		if got := wxSV(req.SceneInfo.H5Info.Type); got != wxpayH5Type {
			t.Fatalf("scene_info.h5_info.type = %q, want %q", got, wxpayH5Type)
	REDACTED
		if got := wxSV(req.SceneInfo.H5Info.AppName); got != "Sub2API" {
			t.Fatalf("scene_info.h5_info.app_name = %q, want %q", got, "Sub2API")
	REDACTED
		if got := wxSV(req.SceneInfo.H5Info.AppUrl); got != "https://app.example.com" {
			t.Fatalf("scene_info.h5_info.app_url = %q, want %q", got, "https://app.example.com")
	REDACTED
		return &h5.PrepayResponse{
			H5Url: core.String("https://wx.tenpay.example/h5pay?prepay_id=1"),
	REDACTED, nil, nil
REDACTED

	provider := &Wxpay{
		config: map[string]string{
			"appId":     "wx123",
			"mchId":     "mch123",
			"h5AppName": "Sub2API",
			"h5AppUrl":  "https://app.example.com",
	REDACTED,
		coreClient: &core.Client{REDACTED,
REDACTED

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_99",
		Amount:      "66.88",
		PaymentType: payment.TypeWxpay,
		Subject:     "Balance Recharge",
		NotifyURL:   "https://merchant.example/payment/notify",
		ReturnURL:   "https://merchant.example/payment/result?resume_token=resume-99",
		ClientIP:    "203.0.113.10",
		IsMobile:    true,
REDACTED)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
REDACTED
	if jsapiCalls != 0 {
		t.Fatalf("jsapi prepay calls = %d, want 0", jsapiCalls)
REDACTED
	if nativeCalls != 0 {
		t.Fatalf("native prepay calls = %d, want 0", nativeCalls)
REDACTED
	if h5Calls != 1 {
		t.Fatalf("h5 prepay calls = %d, want 1", h5Calls)
REDACTED
	if !strings.Contains(resp.PayURL, "redirect_url=") {
		t.Fatalf("pay_url = %q, want redirect_url query appended", resp.PayURL)
REDACTED
REDACTED

func TestCreatePaymentMobileH5FallsBackToNativeOnNoAuth(t *testing.T) {
	origJSAPIPrepay := wxpayJSAPIPrepayWithRequestPayment
	origNativePrepay := wxpayNativePrepay
	origH5Prepay := wxpayH5Prepay
	t.Cleanup(func() {
		wxpayJSAPIPrepayWithRequestPayment = origJSAPIPrepay
		wxpayNativePrepay = origNativePrepay
		wxpayH5Prepay = origH5Prepay
REDACTED)

	jsapiCalls := 0
	nativeCalls := 0
	h5Calls := 0
	wxpayJSAPIPrepayWithRequestPayment = func(ctx context.Context, svc jsapi.JsapiApiService, req jsapi.PrepayRequest) (*jsapi.PrepayWithRequestPaymentResponse, *core.APIResult, error) {
		jsapiCalls++
		return &jsapi.PrepayWithRequestPaymentResponse{REDACTED, nil, nil
REDACTED
	wxpayH5Prepay = func(ctx context.Context, svc h5.H5ApiService, req h5.PrepayRequest) (*h5.PrepayResponse, *core.APIResult, error) {
		h5Calls++
		return nil, nil, errors.New("NO_AUTH")
REDACTED
	wxpayNativePrepay = func(ctx context.Context, svc native.NativeApiService, req native.PrepayRequest) (*native.PrepayResponse, *core.APIResult, error) {
		nativeCalls++
		return &native.PrepayResponse{
			CodeUrl: core.String("weixin://wxpay/bizpayurl?pr=fallback-native"),
	REDACTED, nil, nil
REDACTED

	provider := &Wxpay{
		config: map[string]string{
			"appId": "wx123",
			"mchId": "mch123",
	REDACTED,
		coreClient: &core.Client{REDACTED,
REDACTED

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_100",
		Amount:      "66.88",
		PaymentType: payment.TypeWxpay,
		Subject:     "Balance Recharge",
		NotifyURL:   "https://merchant.example/payment/notify",
		ClientIP:    "203.0.113.10",
		IsMobile:    true,
REDACTED)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
REDACTED
	if jsapiCalls != 0 {
		t.Fatalf("jsapi prepay calls = %d, want 0", jsapiCalls)
REDACTED
	if h5Calls != 1 {
		t.Fatalf("h5 prepay calls = %d, want 1", h5Calls)
REDACTED
	if nativeCalls != 1 {
		t.Fatalf("native prepay calls = %d, want 1", nativeCalls)
REDACTED
	if resp.PayURL != "weixin://wxpay/bizpayurl?pr=fallback-native" {
		t.Fatalf("pay_url = %q, want native fallback url", resp.PayURL)
REDACTED
	if resp.QRCode != "weixin://wxpay/bizpayurl?pr=fallback-native" {
		t.Fatalf("qr_code = %q, want native fallback url", resp.QRCode)
REDACTED
REDACTED
