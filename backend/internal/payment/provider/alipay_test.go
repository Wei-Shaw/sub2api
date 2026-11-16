//go:build unit

package provider

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

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
	origPreCreate := alipayTradePreCreate
	origPagePay := alipayTradePagePay
	origWapPay := alipayTradeWapPay
	t.Cleanup(func() {
		alipayTradePreCreate = origPreCreate
		alipayTradePagePay = origPagePay
		alipayTradeWapPay = origWapPay
	})

	preCreateCalls := 0
	pagePayCalls := 0
	wapPayCalls := 0
	alipayTradePreCreate = func(ctx context.Context, client *alipay.Client, param alipay.TradePreCreate) (*alipay.TradePreCreateRsp, error) {
		preCreateCalls++
		return nil, errors.New("merchant does not have FACE_TO_FACE_PAYMENT")
	}
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
	resp, err := provider.createDesktopTrade(context.Background(), &alipay.Client{}, payment.CreatePaymentRequest{
		OrderID: "sub2_100",
		Amount:  "88.00",
		Subject: "Balance recharge",
	}, "https://merchant.example.com/api/v1/payment/webhook/alipay", "https://merchant.example.com/payment/result")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preCreateCalls != 1 {
		t.Fatalf("precreate calls = %d, want 1", preCreateCalls)
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
	// page.pay returns a checkout page URL, not a scannable QR payload —
	// it must never be exposed via QRCode (the frontend would render an
	// unscannable image from it).
	if resp.QRCode != "" {
		t.Fatalf("qr_code = %q, want empty for page pay", resp.QRCode)
	}
}

// When the provider instance is configured with paymentMode == "redirect",
// the desktop flow must skip precreate and go straight to page.pay.
func TestCreateTradeRedirectModeSkipsPrecreate(t *testing.T) {
	origPreCreate := alipayTradePreCreate
	origPagePay := alipayTradePagePay
	t.Cleanup(func() {
		alipayTradePreCreate = origPreCreate
		alipayTradePagePay = origPagePay
	})

	preCreateCalls := 0
	pagePayCalls := 0
	alipayTradePreCreate = func(ctx context.Context, client *alipay.Client, param alipay.TradePreCreate) (*alipay.TradePreCreateRsp, error) {
		preCreateCalls++
		return &alipay.TradePreCreateRsp{
			Error:  alipay.Error{Code: alipay.CodeSuccess},
			QRCode: "https://qr.alipay.example.com/precreate-token",
		}, nil
	}
	alipayTradePagePay = func(client *alipay.Client, param alipay.TradePagePay) (*url.URL, error) {
		pagePayCalls++
		if param.ProductCode != alipayProductCodePagePay {
			t.Fatalf("product_code = %q, want %q", param.ProductCode, alipayProductCodePagePay)
		}
		return url.Parse("https://openapi.alipay.com/gateway.do?page-pay")
	}

	provider := &Alipay{
		config: map[string]string{"paymentMode": "redirect"},
	}
	resp, err := provider.createDesktopTrade(context.Background(), &alipay.Client{}, payment.CreatePaymentRequest{
		OrderID: "sub2_103",
		Amount:  "12.00",
		Subject: "Balance recharge",
	}, "https://merchant.example.com/api/v1/payment/webhook/alipay", "https://merchant.example.com/payment/result")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preCreateCalls != 0 {
		t.Fatalf("precreate calls = %d, want 0 (redirect mode must skip precreate)", preCreateCalls)
	}
	if pagePayCalls != 1 {
		t.Fatalf("page pay calls = %d, want 1", pagePayCalls)
	}
	if resp.PayURL == "" {
		t.Fatal("expected pay_url for redirect mode")
	}
	if resp.QRCode != "" {
		t.Fatalf("qr_code = %q, want empty for redirect mode", resp.QRCode)
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

func TestCreatePaymentUsesPrecreateForMobileWhenEnabled(t *testing.T) {
	origPreCreate := alipayTradePreCreate
	origWapPay := alipayTradeWapPay
	t.Cleanup(func() {
		alipayTradePreCreate = origPreCreate
		alipayTradeWapPay = origWapPay
	})

	precreateCalls := 0
	wapPayCalls := 0
	alipayTradePreCreate = func(_ context.Context, _ *alipay.Client, param alipay.TradePreCreate) (*alipay.TradePreCreateRsp, error) {
		precreateCalls++
		if param.OutTradeNo != "sub2_mobile_precreate" {
			t.Fatalf("out_trade_no = %q", param.OutTradeNo)
		}
		if param.ProductCode != alipayProductCodePreCreate {
			t.Fatalf("product_code = %q, want %q", param.ProductCode, alipayProductCodePreCreate)
		}
		return &alipay.TradePreCreateRsp{
			Error:  alipay.Error{Code: alipay.CodeSuccess},
			QRCode: "https://qr.alipay.example.com/mobile-dynamic-token",
		}, nil
	}
	alipayTradeWapPay = func(_ *alipay.Client, _ alipay.TradeWapPay) (*url.URL, error) {
		wapPayCalls++
		return url.Parse("https://openapi.alipay.com/gateway.do?wap-pay")
	}

	provider := &Alipay{client: &alipay.Client{}, config: map[string]string{}}
	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:               "sub2_mobile_precreate",
		Amount:                "28.00",
		Subject:               "Balance recharge",
		IsMobile:              true,
		AlipayMobilePrecreate: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if precreateCalls != 1 || wapPayCalls != 0 {
		t.Fatalf("precreate calls = %d, wap calls = %d; want 1, 0", precreateCalls, wapPayCalls)
	}
	if resp.QRCode != "https://qr.alipay.example.com/mobile-dynamic-token" || resp.PayURL != "" {
		t.Fatalf("unexpected response: qr_code=%q pay_url=%q", resp.QRCode, resp.PayURL)
	}
}

func TestCreatePaymentKeepsWapPayForMobileWhenPrecreateDisabled(t *testing.T) {
	origPreCreate := alipayTradePreCreate
	origWapPay := alipayTradeWapPay
	t.Cleanup(func() {
		alipayTradePreCreate = origPreCreate
		alipayTradeWapPay = origWapPay
	})

	precreateCalls := 0
	wapPayCalls := 0
	alipayTradePreCreate = func(_ context.Context, _ *alipay.Client, _ alipay.TradePreCreate) (*alipay.TradePreCreateRsp, error) {
		precreateCalls++
		return nil, errors.New("unexpected precreate call")
	}
	alipayTradeWapPay = func(_ *alipay.Client, _ alipay.TradeWapPay) (*url.URL, error) {
		wapPayCalls++
		return url.Parse("https://openapi.alipay.com/gateway.do?wap-pay")
	}

	provider := &Alipay{client: &alipay.Client{}, config: map[string]string{}}
	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:  "sub2_mobile_wap",
		Amount:   "18.00",
		Subject:  "Balance recharge",
		IsMobile: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if precreateCalls != 0 || wapPayCalls != 1 {
		t.Fatalf("precreate calls = %d, wap calls = %d; want 0, 1", precreateCalls, wapPayCalls)
	}
	if resp.PayURL == "" || resp.QRCode != "" {
		t.Fatalf("unexpected response: qr_code=%q pay_url=%q", resp.QRCode, resp.PayURL)
	}
}

func TestCreateTradeUsesPrecreateForDesktopWhenAvailable(t *testing.T) {
	origPreCreate := alipayTradePreCreate
	origPagePay := alipayTradePagePay
	t.Cleanup(func() {
		alipayTradePreCreate = origPreCreate
		alipayTradePagePay = origPagePay
	})

	preCreateCalls := 0
	pagePayCalls := 0
	alipayTradePreCreate = func(ctx context.Context, client *alipay.Client, param alipay.TradePreCreate) (*alipay.TradePreCreateRsp, error) {
		preCreateCalls++
		if param.ProductCode != alipayProductCodePreCreate {
			t.Fatalf("product_code = %q, want %q", param.ProductCode, alipayProductCodePreCreate)
		}
		return &alipay.TradePreCreateRsp{
			Error:  alipay.Error{Code: alipay.CodeSuccess},
			QRCode: "https://qr.alipay.example.com/precreate-token",
		}, nil
	}
	alipayTradePagePay = func(client *alipay.Client, param alipay.TradePagePay) (*url.URL, error) {
		pagePayCalls++
		return url.Parse("https://openapi.alipay.com/gateway.do?page-pay")
	}

	provider := &Alipay{}
	resp, err := provider.createDesktopTrade(context.Background(), &alipay.Client{}, payment.CreatePaymentRequest{
		OrderID: "sub2_102",
		Amount:  "66.00",
		Subject: "Balance recharge",
	}, "https://merchant.example.com/api/v1/payment/webhook/alipay", "https://merchant.example.com/payment/result")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preCreateCalls != 1 {
		t.Fatalf("precreate calls = %d, want 1", preCreateCalls)
	}
	if pagePayCalls != 0 {
		t.Fatalf("page pay calls = %d, want 0", pagePayCalls)
	}
	if resp.QRCode != "https://qr.alipay.example.com/precreate-token" {
		t.Fatalf("qr_code = %q", resp.QRCode)
	}
	if resp.PayURL != "" {
		t.Fatalf("pay_url = %q, want empty for precreate", resp.PayURL)
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

func TestParseAlipayAmount(t *testing.T) {
	t.Parallel()

	amount, err := parseAlipayAmount("", "88.00", "77.00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amount != 88 {
		t.Fatalf("amount = %v, want 88", amount)
	}

	if _, err := parseAlipayAmount("", "not-a-number"); err == nil {
		t.Fatal("expected error when no valid amount field exists")
	}
}

func TestAlipayRefundRecordsOutRequestNoAsRefundID(t *testing.T) {
	origRefund := alipayTradeRefund
	t.Cleanup(func() { alipayTradeRefund = origRefund })

	for _, tc := range []struct {
		fundChange string
		wantStatus string
	}{
		{fundChange: alipayFundChangeYes, wantStatus: payment.ProviderStatusSuccess},
		{fundChange: "N", wantStatus: payment.ProviderStatusPending},
	} {
		var gotOutRequestNo string
		alipayTradeRefund = func(_ context.Context, _ *alipay.Client, param alipay.TradeRefund) (*alipay.TradeRefundRsp, error) {
			if param.OutTradeNo != "sub2_7" || param.RefundAmount != "12.50" {
				t.Fatalf("unexpected refund param: %+v", param)
			}
			gotOutRequestNo = param.OutRequestNo
			return &alipay.TradeRefundRsp{TradeNo: "2026101522001", FundChange: tc.fundChange}, nil
		}

		provider := &Alipay{client: &alipay.Client{}}
		resp, err := provider.Refund(context.Background(), payment.RefundRequest{OrderID: "sub2_7", Amount: "12.50", Reason: "r"})
		if err != nil {
			t.Fatalf("Refund() error = %v", err)
		}
		if resp.Status != tc.wantStatus {
			t.Fatalf("fund_change=%s status = %q, want %q", tc.fundChange, resp.Status, tc.wantStatus)
		}
		if !strings.HasPrefix(gotOutRequestNo, "sub2_7-refund-") || resp.RefundID != gotOutRequestNo {
			t.Fatalf("RefundID = %q, out_request_no = %q", resp.RefundID, gotOutRequestNo)
		}
	}
}

func TestAlipayQueryRefundMapsStatus(t *testing.T) {
	origQuery := alipayTradeFastPayRefundQuery
	t.Cleanup(func() { alipayTradeFastPayRefundQuery = origQuery })

	const outRequestNo = "sub2_7-refund-1760500000000000000"
	ok := alipay.Error{Code: alipay.CodeSuccess, Msg: "Success"}
	settled := time.Now().Add(-alipayRefundQueryNotFoundGrace - time.Minute)
	for _, tc := range []struct {
		name        string
		rsp         *alipay.TradeFastPayRefundQueryRsp
		requestedAt time.Time
		wantStatus  string
	}{
		{name: "success", rsp: &alipay.TradeFastPayRefundQueryRsp{Error: ok, OutRequestNo: outRequestNo, RefundAmount: "12.50", RefundStatus: alipayRefundStatusSucc}, requestedAt: settled, wantStatus: payment.ProviderStatusSuccess},
		{name: "record without final status", rsp: &alipay.TradeFastPayRefundQueryRsp{Error: ok, OutRequestNo: outRequestNo, RefundAmount: "12.50"}, requestedAt: settled, wantStatus: payment.ProviderStatusPending},
		{name: "no refund record after grace", rsp: &alipay.TradeFastPayRefundQueryRsp{Error: ok}, requestedAt: settled, wantStatus: payment.ProviderStatusFailed},
		{name: "no refund record within grace", rsp: &alipay.TradeFastPayRefundQueryRsp{Error: ok}, requestedAt: time.Now().Add(-time.Minute), wantStatus: payment.ProviderStatusPending},
		{name: "no refund record with unknown request time", rsp: &alipay.TradeFastPayRefundQueryRsp{Error: ok}, wantStatus: payment.ProviderStatusPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alipayTradeFastPayRefundQuery = func(_ context.Context, _ *alipay.Client, param alipay.TradeFastPayRefundQuery) (*alipay.TradeFastPayRefundQueryRsp, error) {
				if param.OutTradeNo != "sub2_7" || param.OutRequestNo != outRequestNo {
					t.Fatalf("unexpected query param: %+v", param)
				}
				return tc.rsp, nil
			}
			provider := &Alipay{client: &alipay.Client{}}
			resp, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{OrderID: "sub2_7", RefundID: outRequestNo, RequestedAt: tc.requestedAt})
			if err != nil {
				t.Fatalf("QueryRefund() error = %v", err)
			}
			if resp.Status != tc.wantStatus || resp.RefundID != outRequestNo {
				t.Fatalf("QueryRefund() = %+v, want status %q", resp, tc.wantStatus)
			}
		})
	}
}

// SDK 对带签名的业务失败响应返回 err == nil，错误码只在响应体里。
func TestAlipayQueryRefundBusinessFailureKeepsRefundPending(t *testing.T) {
	origQuery := alipayTradeFastPayRefundQuery
	t.Cleanup(func() { alipayTradeFastPayRefundQuery = origQuery })

	for _, rspErr := range []alipay.Error{
		{Code: "40004", Msg: "Business Failed", SubCode: "ACQ.SYSTEM_ERROR", SubMsg: "系统错误"},
		{Code: "20000", Msg: "Service Currently Unavailable", SubCode: "isp.unknow-error"},
		{},
	} {
		alipayTradeFastPayRefundQuery = func(context.Context, *alipay.Client, alipay.TradeFastPayRefundQuery) (*alipay.TradeFastPayRefundQueryRsp, error) {
			return &alipay.TradeFastPayRefundQueryRsp{Error: rspErr}, nil
		}
		provider := &Alipay{client: &alipay.Client{}}
		resp, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{
			OrderID:     "sub2_7",
			RefundID:    "sub2_7-refund-1",
			RequestedAt: time.Now().Add(-24 * time.Hour),
		})
		if err == nil {
			t.Fatalf("code %q: QueryRefund() = %+v, want error", rspErr.Code, resp)
		}
		if rspErr.SubCode != "" && !strings.Contains(err.Error(), string(rspErr.Code)) {
			t.Fatalf("code %q: error %q should carry the gateway code", rspErr.Code, err)
		}
	}
}

func TestAlipayQueryRefundErrorsKeepRefundPending(t *testing.T) {
	origQuery := alipayTradeFastPayRefundQuery
	t.Cleanup(func() { alipayTradeFastPayRefundQuery = origQuery })

	calls := 0
	alipayTradeFastPayRefundQuery = func(context.Context, *alipay.Client, alipay.TradeFastPayRefundQuery) (*alipay.TradeFastPayRefundQueryRsp, error) {
		calls++
		return nil, errors.New("ACQ.SYSTEM_ERROR")
	}
	provider := &Alipay{client: &alipay.Client{}}

	// 旧版本记录的是支付宝交易号，无法查询退款请求，需人工核实，且不能调用网关。
	for _, legacyID := range []string{"", "2026101522001", "sub2_7-refund", "sub2_70-refund-1"} {
		if _, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{OrderID: "sub2_7", RefundID: legacyID}); err == nil || !strings.Contains(err.Error(), "verify manually") {
			t.Fatalf("legacy refund id %q: err = %v", legacyID, err)
		}
	}
	if calls != 0 {
		t.Fatalf("legacy refund ids must not reach the gateway, calls = %d", calls)
	}

	if _, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{OrderID: "sub2_7", RefundID: "sub2_7-refund-1"}); err == nil {
		t.Fatal("gateway error must be returned so the refund stays pending")
	}
}
