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
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal pkix: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
}

func TestMapWxState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "SUCCESS maps to paid",
			input: wxpayTradeStateSuccess,
			want:  payment.ProviderStatusPaid,
		},
		{
			name:  "REFUND maps to refunded",
			input: wxpayTradeStateRefund,
			want:  payment.ProviderStatusRefunded,
		},
		{
			name:  "CLOSED maps to failed",
			input: wxpayTradeStateClosed,
			want:  payment.ProviderStatusFailed,
		},
		{
			name:  "PAYERROR maps to failed",
			input: wxpayTradeStatePayError,
			want:  payment.ProviderStatusFailed,
		},
		{
			name:  "unknown state maps to pending",
			input: "NOTPAY",
			want:  payment.ProviderStatusPending,
		},
		{
			name:  "empty string maps to pending",
			input: "",
			want:  payment.ProviderStatusPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mapWxState(tt.input)
			if got != tt.want {
				t.Errorf("mapWxState(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWxSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input *string
		want  string
	}{
		{
			name:  "nil pointer returns empty string",
			input: nil,
			want:  "",
		},
		{
			name:  "non-nil pointer returns value",
			input: strPtr("hello"),
			want:  "hello",
		},
		{
			name:  "pointer to empty string returns empty string",
			input: strPtr(""),
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := wxSV(tt.input)
			if got != tt.want {
				t.Errorf("wxSV() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildWxpayTransactionMetadata(t *testing.T) {
	t.Parallel()

	tx := &payments.Transaction{
		Appid:      strPtr("wx-app-id"),
		Mchid:      strPtr("mch-id"),
		TradeState: strPtr(wxpayTradeStateSuccess),
		Amount: &payments.TransactionAmount{
			Currency: strPtr(wxpayCurrency),
		},
	}

	metadata := buildWxpayTransactionMetadata(tx)
	if metadata[wxpayMetadataAppID] != "wx-app-id" {
		t.Fatalf("appid = %q", metadata[wxpayMetadataAppID])
	}
	if metadata[wxpayMetadataMerchantID] != "mch-id" {
		t.Fatalf("mchid = %q", metadata[wxpayMetadataMerchantID])
	}
	if metadata[wxpayMetadataCurrency] != wxpayCurrency {
		t.Fatalf("currency = %q", metadata[wxpayMetadataCurrency])
	}
	if metadata[wxpayMetadataTradeState] != wxpayTradeStateSuccess {
		t.Fatalf("trade_state = %q", metadata[wxpayMetadataTradeState])
	}
}

func strPtr(s string) *string {
	return &s
}

func TestFormatPEM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		keyType string
		want    string
	}{
		{
			name:    "raw key gets wrapped with headers",
			key:     "MIIBIjANBgkqhki...",
			keyType: "PUBLIC KEY",
			want:    "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhki...\n-----END PUBLIC KEY-----",
		},
		{
			name:    "already formatted key is returned as-is",
			key:     "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBg...\n-----END PRIVATE KEY-----",
			keyType: "PRIVATE KEY",
			want:    "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBg...\n-----END PRIVATE KEY-----",
		},
		{
			name:    "key with leading/trailing whitespace is trimmed before check",
			key:     "  \n MIIBIjANBgkqhki...  \n ",
			keyType: "PUBLIC KEY",
			want:    "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhki...\n-----END PUBLIC KEY-----",
		},
		{
			name:    "already formatted key with whitespace is trimmed and returned",
			key:     "  -----BEGIN RSA PRIVATE KEY-----\ndata\n-----END RSA PRIVATE KEY-----  ",
			keyType: "RSA PRIVATE KEY",
			want:    "-----BEGIN RSA PRIVATE KEY-----\ndata\n-----END RSA PRIVATE KEY-----",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatPEM(tt.key, tt.keyType)
			if got != tt.want {
				t.Errorf("formatPEM(%q, %q) =\n%s\nwant:\n%s", tt.key, tt.keyType, got, tt.want)
			}
		})
	}
}

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
	}

	// helper to clone and override config fields
	withOverride := func(overrides map[string]string) map[string]string {
		cfg := make(map[string]string, len(validConfig))
		for k, v := range validConfig {
			cfg[k] = v
		}
		for k, v := range overrides {
			cfg[k] = v
		}
		return cfg
	}

	tests := []struct {
		name      string
		config    map[string]string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid config succeeds",
			config:  validConfig,
			wantErr: false,
		},
		{
			name:      "missing appId",
			config:    withOverride(map[string]string{"appId": ""}),
			wantErr:   true,
			errSubstr: "appId",
		},
		{
			name:      "missing mchId",
			config:    withOverride(map[string]string{"mchId": ""}),
			wantErr:   true,
			errSubstr: "mchId",
		},
		{
			name:      "missing privateKey",
			config:    withOverride(map[string]string{"privateKey": ""}),
			wantErr:   true,
			errSubstr: "privateKey",
		},
		{
			name:      "missing apiV3Key",
			config:    withOverride(map[string]string{"apiV3Key": ""}),
			wantErr:   true,
			errSubstr: "apiV3Key",
		},
		{
			name:      "missing certSerial",
			config:    withOverride(map[string]string{"certSerial": ""}),
			wantErr:   true,
			errSubstr: "certSerial",
		},
		{
			name:      "missing publicKey",
			config:    withOverride(map[string]string{"publicKey": ""}),
			wantErr:   true,
			errSubstr: "publicKey",
		},
		{
			name:      "missing publicKeyId",
			config:    withOverride(map[string]string{"publicKeyId": ""}),
			wantErr:   true,
			errSubstr: "publicKeyId",
		},
		{
			name:      "malformed privateKey PEM",
			config:    withOverride(map[string]string{"privateKey": "not-a-valid-pem"}),
			wantErr:   true,
			errSubstr: "WXPAY_CONFIG_INVALID_KEY",
		},
		{
			name:      "malformed publicKey PEM",
			config:    withOverride(map[string]string{"publicKey": "not-a-valid-pem"}),
			wantErr:   true,
			errSubstr: "WXPAY_CONFIG_INVALID_KEY",
		},
		{
			name:      "apiV3Key too short",
			config:    withOverride(map[string]string{"apiV3Key": "short"}),
			wantErr:   true,
			errSubstr: "WXPAY_CONFIG_INVALID_KEY_LENGTH",
		},
		{
			name:      "apiV3Key too long",
			config:    withOverride(map[string]string{"apiV3Key": "123456789012345678901234567890123"}), // 33 bytes
			wantErr:   true,
			errSubstr: "WXPAY_CONFIG_INVALID_KEY_LENGTH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewWxpay("test-instance", tt.config)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("expected non-nil Wxpay instance")
			}
			if got.instanceID != "test-instance" {
				t.Errorf("instanceID = %q, want %q", got.instanceID, "test-instance")
			}
		})
	}
}

func TestBuildWxpayResultURLPreservesResumeToken(t *testing.T) {
	t.Parallel()

	resultURL, err := buildWxpayResultURL("https://app.example.com/payment/result?order_id=42&resume_token=resume-42&status=success", payment.CreatePaymentRequest{
		OrderID:     "sub2_42",
		PaymentType: payment.TypeWxpay,
	})
	if err != nil {
		t.Fatalf("buildWxpayResultURL returned error: %v", err)
	}

	parsed, err := url.Parse(resultURL)
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
	}
	query := parsed.Query()
	if parsed.Path != wxpayResultPath {
		t.Fatalf("path = %q, want %q", parsed.Path, wxpayResultPath)
	}
	if query.Get("resume_token") != "resume-42" {
		t.Fatalf("resume_token = %q, want %q", query.Get("resume_token"), "resume-42")
	}
	if query.Get("order_id") != "42" {
		t.Fatalf("order_id = %q, want %q", query.Get("order_id"), "42")
	}
	if query.Get("out_trade_no") != "sub2_42" {
		t.Fatalf("out_trade_no = %q, want %q", query.Get("out_trade_no"), "sub2_42")
	}
}

func TestResolveWxpayJSAPIAppID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config map[string]string
		want   string
	}{
		{
			name: "prefers dedicated mp app id",
			config: map[string]string{
				"mpAppId": "wx-mp-app",
				"appId":   "wx-merchant-app",
			},
			want: "wx-mp-app",
		},
		{
			name: "falls back to merchant app id",
			config: map[string]string{
				"appId": "wx-merchant-app",
			},
			want: "wx-merchant-app",
		},
		{
			name:   "missing app ids returns empty",
			config: map[string]string{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveWxpayJSAPIAppID(tt.config); got != tt.want {
				t.Fatalf("ResolveWxpayJSAPIAppID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveWxpayCreateMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		req      payment.CreatePaymentRequest
		wantMode string
		wantErr  string
	}{
		{
			name:     "desktop uses native",
			req:      payment.CreatePaymentRequest{},
			wantMode: wxpayModeNative,
		},
		{
			name: "mobile uses h5 when client ip is present",
			req: payment.CreatePaymentRequest{
				IsMobile: true,
				ClientIP: "203.0.113.10",
			},
			wantMode: wxpayModeH5,
		},
		{
			name: "mobile without client ip returns clear error",
			req: payment.CreatePaymentRequest{
				IsMobile: true,
			},
			wantErr: "requires client IP",
		},
		{
			name: "openid uses jsapi mode",
			req: payment.CreatePaymentRequest{
				OpenID: "openid-123",
			},
			wantMode: wxpayModeJSAPI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveWxpayCreateMode(tt.req)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q should contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantMode {
				t.Fatalf("resolveWxpayCreateMode() = %q, want %q", got, tt.wantMode)
			}
		})
	}
}

func TestCreatePaymentWithOpenIDReturnsJSAPIResult(t *testing.T) {
	origJSAPIPrepay := wxpayJSAPIPrepayWithRequestPayment
	origNativePrepay := wxpayNativePrepay
	origH5Prepay := wxpayH5Prepay
	t.Cleanup(func() {
		wxpayJSAPIPrepayWithRequestPayment = origJSAPIPrepay
		wxpayNativePrepay = origNativePrepay
		wxpayH5Prepay = origH5Prepay
	})

	jsapiCalls := 0
	nativeCalls := 0
	h5Calls := 0
	wxpayJSAPIPrepayWithRequestPayment = func(ctx context.Context, svc jsapi.JsapiApiService, req jsapi.PrepayRequest) (*jsapi.PrepayWithRequestPaymentResponse, *core.APIResult, error) {
		jsapiCalls++
		if got := wxSV(req.Payer.Openid); got != "openid-123" {
			t.Fatalf("openid = %q, want %q", got, "openid-123")
		}
		if req.SceneInfo == nil || wxSV(req.SceneInfo.PayerClientIp) != "203.0.113.10" {
			t.Fatalf("scene_info payer_client_ip = %q, want %q", wxSV(req.SceneInfo.PayerClientIp), "203.0.113.10")
		}
		return &jsapi.PrepayWithRequestPaymentResponse{
			Appid:     core.String("wx123"),
			TimeStamp: core.String("1712345678"),
			NonceStr:  core.String("nonce-123"),
			Package:   core.String("prepay_id=wx_prepay_123"),
			SignType:  core.String("RSA"),
			PaySign:   core.String("signed-payload"),
		}, nil, nil
	}
	wxpayNativePrepay = func(ctx context.Context, svc native.NativeApiService, req native.PrepayRequest) (*native.PrepayResponse, *core.APIResult, error) {
		nativeCalls++
		return &native.PrepayResponse{}, nil, nil
	}
	wxpayH5Prepay = func(ctx context.Context, svc h5.H5ApiService, req h5.PrepayRequest) (*h5.PrepayResponse, *core.APIResult, error) {
		h5Calls++
		return &h5.PrepayResponse{}, nil, nil
	}

	provider := &Wxpay{
		config: map[string]string{
			"appId": "wx123",
			"mchId": "mch123",
		},
		coreClient: &core.Client{},
	}

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_88",
		Amount:      "66.88",
		PaymentType: payment.TypeWxpay,
		NotifyURL:   "https://merchant.example/payment/notify",
		OpenID:      "openid-123",
		ClientIP:    "203.0.113.10",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsapiCalls != 1 {
		t.Fatalf("jsapi prepay calls = %d, want 1", jsapiCalls)
	}
	if nativeCalls != 0 {
		t.Fatalf("native prepay calls = %d, want 0", nativeCalls)
	}
	if h5Calls != 0 {
		t.Fatalf("h5 prepay calls = %d, want 0", h5Calls)
	}
	if resp.ResultType != payment.CreatePaymentResultJSAPIReady {
		t.Fatalf("result type = %q, want %q", resp.ResultType, payment.CreatePaymentResultJSAPIReady)
	}
	if resp.JSAPI == nil {
		t.Fatal("expected jsapi payload, got nil")
	}
	if resp.JSAPI.AppID != "wx123" {
		t.Fatalf("jsapi appId = %q, want %q", resp.JSAPI.AppID, "wx123")
	}
	if resp.JSAPI.TimeStamp != "1712345678" {
		t.Fatalf("jsapi timeStamp = %q, want %q", resp.JSAPI.TimeStamp, "1712345678")
	}
	if resp.JSAPI.NonceStr != "nonce-123" {
		t.Fatalf("jsapi nonceStr = %q, want %q", resp.JSAPI.NonceStr, "nonce-123")
	}
	if resp.JSAPI.Package != "prepay_id=wx_prepay_123" {
		t.Fatalf("jsapi package = %q, want %q", resp.JSAPI.Package, "prepay_id=wx_prepay_123")
	}
	if resp.JSAPI.SignType != "RSA" {
		t.Fatalf("jsapi signType = %q, want %q", resp.JSAPI.SignType, "RSA")
	}
	if resp.JSAPI.PaySign != "signed-payload" {
		t.Fatalf("jsapi paySign = %q, want %q", resp.JSAPI.PaySign, "signed-payload")
	}
}

func TestCreatePaymentMobileH5IncludesConfiguredSceneInfo(t *testing.T) {
	origJSAPIPrepay := wxpayJSAPIPrepayWithRequestPayment
	origNativePrepay := wxpayNativePrepay
	origH5Prepay := wxpayH5Prepay
	t.Cleanup(func() {
		wxpayJSAPIPrepayWithRequestPayment = origJSAPIPrepay
		wxpayNativePrepay = origNativePrepay
		wxpayH5Prepay = origH5Prepay
	})

	jsapiCalls := 0
	nativeCalls := 0
	h5Calls := 0
	wxpayJSAPIPrepayWithRequestPayment = func(ctx context.Context, svc jsapi.JsapiApiService, req jsapi.PrepayRequest) (*jsapi.PrepayWithRequestPaymentResponse, *core.APIResult, error) {
		jsapiCalls++
		return &jsapi.PrepayWithRequestPaymentResponse{}, nil, nil
	}
	wxpayNativePrepay = func(ctx context.Context, svc native.NativeApiService, req native.PrepayRequest) (*native.PrepayResponse, *core.APIResult, error) {
		nativeCalls++
		return &native.PrepayResponse{}, nil, nil
	}
	wxpayH5Prepay = func(ctx context.Context, svc h5.H5ApiService, req h5.PrepayRequest) (*h5.PrepayResponse, *core.APIResult, error) {
		h5Calls++
		if req.SceneInfo == nil {
			t.Fatal("expected scene_info, got nil")
		}
		if got := wxSV(req.SceneInfo.PayerClientIp); got != "203.0.113.10" {
			t.Fatalf("scene_info payer_client_ip = %q, want %q", got, "203.0.113.10")
		}
		if req.SceneInfo.H5Info == nil {
			t.Fatal("expected scene_info.h5_info, got nil")
		}
		if got := wxSV(req.SceneInfo.H5Info.Type); got != wxpayH5Type {
			t.Fatalf("scene_info.h5_info.type = %q, want %q", got, wxpayH5Type)
		}
		if got := wxSV(req.SceneInfo.H5Info.AppName); got != "Sub2API" {
			t.Fatalf("scene_info.h5_info.app_name = %q, want %q", got, "Sub2API")
		}
		if got := wxSV(req.SceneInfo.H5Info.AppUrl); got != "https://app.example.com" {
			t.Fatalf("scene_info.h5_info.app_url = %q, want %q", got, "https://app.example.com")
		}
		return &h5.PrepayResponse{
			H5Url: core.String("https://wx.tenpay.example/h5pay?prepay_id=1"),
		}, nil, nil
	}

	provider := &Wxpay{
		config: map[string]string{
			"appId":     "wx123",
			"mchId":     "mch123",
			"h5AppName": "Sub2API",
			"h5AppUrl":  "https://app.example.com",
		},
		coreClient: &core.Client{},
	}

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_99",
		Amount:      "66.88",
		PaymentType: payment.TypeWxpay,
		Subject:     "Balance Recharge",
		NotifyURL:   "https://merchant.example/payment/notify",
		ReturnURL:   "https://merchant.example/payment/result?resume_token=resume-99",
		ClientIP:    "203.0.113.10",
		IsMobile:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsapiCalls != 0 {
		t.Fatalf("jsapi prepay calls = %d, want 0", jsapiCalls)
	}
	if nativeCalls != 0 {
		t.Fatalf("native prepay calls = %d, want 0", nativeCalls)
	}
	if h5Calls != 1 {
		t.Fatalf("h5 prepay calls = %d, want 1", h5Calls)
	}
	if !strings.Contains(resp.PayURL, "redirect_url=") {
		t.Fatalf("pay_url = %q, want redirect_url query appended", resp.PayURL)
	}
}

func TestCreatePaymentMobileH5ReturnsNoAuthErrorWithoutNativeFallback(t *testing.T) {
	origJSAPIPrepay := wxpayJSAPIPrepayWithRequestPayment
	origNativePrepay := wxpayNativePrepay
	origH5Prepay := wxpayH5Prepay
	t.Cleanup(func() {
		wxpayJSAPIPrepayWithRequestPayment = origJSAPIPrepay
		wxpayNativePrepay = origNativePrepay
		wxpayH5Prepay = origH5Prepay
	})

	jsapiCalls := 0
	nativeCalls := 0
	h5Calls := 0
	wxpayJSAPIPrepayWithRequestPayment = func(ctx context.Context, svc jsapi.JsapiApiService, req jsapi.PrepayRequest) (*jsapi.PrepayWithRequestPaymentResponse, *core.APIResult, error) {
		jsapiCalls++
		return &jsapi.PrepayWithRequestPaymentResponse{}, nil, nil
	}
	wxpayH5Prepay = func(ctx context.Context, svc h5.H5ApiService, req h5.PrepayRequest) (*h5.PrepayResponse, *core.APIResult, error) {
		h5Calls++
		return nil, nil, errors.New("NO_AUTH")
	}
	wxpayNativePrepay = func(ctx context.Context, svc native.NativeApiService, req native.PrepayRequest) (*native.PrepayResponse, *core.APIResult, error) {
		nativeCalls++
		return &native.PrepayResponse{
			CodeUrl: core.String("weixin://wxpay/bizpayurl?pr=fallback-native"),
		}, nil, nil
	}

	provider := &Wxpay{
		config: map[string]string{
			"appId": "wx123",
			"mchId": "mch123",
		},
		coreClient: &core.Client{},
	}

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2_100",
		Amount:      "66.88",
		PaymentType: payment.TypeWxpay,
		Subject:     "Balance Recharge",
		NotifyURL:   "https://merchant.example/payment/notify",
		ClientIP:    "203.0.113.10",
		IsMobile:    true,
	})
	if err == nil {
		t.Fatal("expected no-auth error, got nil")
	}
	if jsapiCalls != 0 {
		t.Fatalf("jsapi prepay calls = %d, want 0", jsapiCalls)
	}
	if h5Calls != 1 {
		t.Fatalf("h5 prepay calls = %d, want 1", h5Calls)
	}
	if nativeCalls != 0 {
		t.Fatalf("native prepay calls = %d, want 0", nativeCalls)
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
	if !strings.Contains(err.Error(), "NO_AUTH") {
		t.Fatalf("error = %v, want NO_AUTH", err)
	}
}
