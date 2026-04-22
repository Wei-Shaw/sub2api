//go:build unit

package provider

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/smartwalle/alipay/v3"
)

func TestIsTradeNotExist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error returns false",
			err:  nil,
			want: false,
		},
		{
			name: "error containing ACQ.TRADE_NOT_EXIST returns true",
			err:  errors.New("alipay: sub_code=ACQ.TRADE_NOT_EXIST, sub_msg=交易不存在"),
			want: true,
		},
		{
			name: "error not containing the code returns false",
			err:  errors.New("alipay: sub_code=ACQ.SYSTEM_ERROR, sub_msg=系统错误"),
			want: false,
		},
		{
			name: "error with only partial match returns false",
			err:  errors.New("ACQ.TRADE_NOT"),
			want: false,
		},
		{
			name: "error with exact constant value returns true",
			err:  errors.New(alipayErrTradeNotExist),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isTradeNotExist(tt.err)
			if got != tt.want {
				t.Errorf("isTradeNotExist(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestNewAlipay(t *testing.T) {
	t.Parallel()

	validConfig := map[string]string{
		"appId":      "2021001234567890",
		"privateKey": "MIIEvQIBADANBgkqhkiG9w0BAQEFAASC...",
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
			name:      "missing privateKey",
			config:    withOverride(map[string]string{"privateKey": ""}),
			wantErr:   true,
			errSubstr: "privateKey",
		},
		{
			name:      "nil config map returns error for appId",
			config:    map[string]string{},
			wantErr:   true,
			errSubstr: "appId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewAlipay("test-instance", tt.config)
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
				t.Fatal("expected non-nil Alipay instance")
			}
			if got.instanceID != "test-instance" {
				t.Errorf("instanceID = %q, want %q", got.instanceID, "test-instance")
			}
		})
	}
}

func TestCreateTradeUsesPagePayForDesktop(t *testing.T) {
	origPagePay := alipayTradePagePay
	origWapPay := alipayTradeWapPay
	t.Cleanup(func() {
		alipayTradePagePay = origPagePay
		alipayTradeWapPay = origWapPay
	})

	pagePayCalls := 0
	wapPayCalls := 0
	alipayTradePagePay = func(client *alipay.Client, param alipay.TradePagePay) (*url.URL, error) {
		pagePayCalls++
		if param.OutTradeNo != "sub2_100" {
			t.Fatalf("out_trade_no = %q, want %q", param.OutTradeNo, "sub2_100")
		}
		if param.NotifyURL != "https://merchant.example.com/api/v1/payment/webhook/alipay" {
			t.Fatalf("notify_url = %q", param.NotifyURL)
		}
		return url.Parse("https://openapi.alipay.com/gateway.do?page-pay")
	}
	alipayTradeWapPay = func(client *alipay.Client, param alipay.TradeWapPay) (*url.URL, error) {
		wapPayCalls++
		return url.Parse("https://openapi.alipay.com/gateway.do?wap-pay")
	}

	provider := &Alipay{}
	resp, err := provider.createPagePayTrade(&alipay.Client{}, payment.CreatePaymentRequest{
		OrderID: "sub2_100",
		Amount:  "88.00",
		Subject: "Balance recharge",
	}, "https://merchant.example.com/api/v1/payment/webhook/alipay", "https://merchant.example.com/payment/result")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pagePayCalls != 1 {
		t.Fatalf("page pay calls = %d, want 1", pagePayCalls)
	}
	if wapPayCalls != 0 {
		t.Fatalf("wap pay calls = %d, want 0", wapPayCalls)
	}
	if resp.PayURL == "" {
		t.Fatal("expected pay_url for desktop page pay")
	}
}

func TestCreateTradeUsesWapPayForMobile(t *testing.T) {
	origWapPay := alipayTradeWapPay
	t.Cleanup(func() {
		alipayTradeWapPay = origWapPay
	})

	wapPayCalls := 0
	alipayTradeWapPay = func(client *alipay.Client, param alipay.TradeWapPay) (*url.URL, error) {
		wapPayCalls++
		if param.ReturnURL != "https://merchant.example.com/payment/result" {
			t.Fatalf("return_url = %q", param.ReturnURL)
		}
		return url.Parse("https://openapi.alipay.com/gateway.do?wap-pay")
	}

	provider := &Alipay{}
	resp, err := provider.createWapTrade(&alipay.Client{}, payment.CreatePaymentRequest{
		OrderID:  "sub2_101",
		Amount:   "18.00",
		Subject:  "Balance recharge",
		IsMobile: true,
	}, "https://merchant.example.com/api/v1/payment/webhook/alipay", "https://merchant.example.com/payment/result")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wapPayCalls != 1 {
		t.Fatalf("wap pay calls = %d, want 1", wapPayCalls)
	}
	if resp.PayURL == "" {
		t.Fatal("expected pay_url for mobile wap pay")
	}
}

func TestAlipayMerchantIdentityMetadata(t *testing.T) {
	t.Parallel()

	provider := &Alipay{
		config: map[string]string{
			"appId": "2021001234567890",
		},
	}

	metadata := provider.MerchantIdentityMetadata()
	if metadata["app_id"] != "2021001234567890" {
		t.Fatalf("app_id = %q, want %q", metadata["app_id"], "2021001234567890")
	}
}
