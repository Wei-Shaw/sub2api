package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// issue #6292: mapi.php 上游若回相对形式的 payurl/payurl2/qrcode，网关原样
// 落库，前端把 qr_code 直接喂给 QRCode.toCanvas，扫出来是一段裸路径；pay_url 则在
// 网关自己的域名下 404。这里锁定归一化规则与全部「不得改写」的边界。

func TestResolveEasyPayReturnedRef(t *testing.T) {
	t.Parallel()

	const apiBase = "https://pay.example.com/xpay/epay"

	tests := []struct {
		name    string
		apiBase string
		ref     string
		want    string
	}{
		{
			// issue 复现主线：站点根相对路径按 apiBase 的 origin 补全，
			// apiBase 自身的 /xpay/epay 前缀不参与（根相对语义）。
			name:    "root relative path resolves against origin",
			apiBase: apiBase,
			ref:     "/api/pay/toapp/ORDER_ID",
			want:    "https://pay.example.com/api/pay/toapp/ORDER_ID",
		},
		{
			name:    "root relative path keeps query string",
			apiBase: apiBase,
			ref:     "/api/pay/toapp?oid=ORDER_ID&t=1",
			want:    "https://pay.example.com/api/pay/toapp?oid=ORDER_ID&t=1",
		},
		{
			name:    "protocol relative reference inherits base scheme",
			apiBase: apiBase,
			ref:     "//cashier.example.com/pay/ORDER_ID",
			want:    "https://cashier.example.com/pay/ORDER_ID",
		},
		{
			name:    "surrounding whitespace does not defeat resolution",
			apiBase: apiBase,
			ref:     "  /api/pay/toapp/ORDER_ID  ",
			want:    "https://pay.example.com/api/pay/toapp/ORDER_ID",
		},
		{
			name:    "http base keeps its scheme",
			apiBase: "http://pay.example.com",
			ref:     "/api/pay/toapp/ORDER_ID",
			want:    "http://pay.example.com/api/pay/toapp/ORDER_ID",
		},
		// —— 以下全部必须原样返回 ——
		{
			name:    "empty stays empty",
			apiBase: apiBase,
			ref:     "",
			want:    "",
		},
		{
			name:    "absolute https url is untouched",
			apiBase: apiBase,
			ref:     "https://cashier.other.com/pay/ORDER_ID",
			want:    "https://cashier.other.com/pay/ORDER_ID",
		},
		{
			name:    "wechat deep link is untouched",
			apiBase: apiBase,
			ref:     "weixin://wxpay/bizpayurl?pr=ABCdef",
			want:    "weixin://wxpay/bizpayurl?pr=ABCdef",
		},
		{
			name:    "alipay deep link is untouched",
			apiBase: apiBase,
			ref:     "alipays://platformapi/startapp?saId=10000007",
			want:    "alipays://platformapi/startapp?saId=10000007",
		},
		{
			name:    "face-to-face wxp payload is untouched",
			apiBase: apiBase,
			ref:     "wxp://f2f0Abc_dEfGhIjKlMnOpQrStU",
			want:    "wxp://f2f0Abc_dEfGhIjKlMnOpQrStU",
		},
		{
			// 无前导斜杠的裸 token：把它改写成 <apiBase>/token 会毁掉一个本来
			// 可用的二维码载荷，比 bug 本身更糟，所以不碰。
			name:    "opaque token without leading slash is untouched",
			apiBase: apiBase,
			ref:     "OrderToken123",
			want:    "OrderToken123",
		},
		{
			name:    "path-like reference without leading slash is untouched",
			apiBase: apiBase,
			ref:     "api/pay/toapp/ORDER_ID",
			want:    "api/pay/toapp/ORDER_ID",
		},
		{
			name:    "empty api base leaves the reference alone",
			apiBase: "",
			ref:     "/api/pay/toapp/ORDER_ID",
			want:    "/api/pay/toapp/ORDER_ID",
		},
		{
			name:    "api base without scheme leaves the reference alone",
			apiBase: "pay.example.com",
			ref:     "/api/pay/toapp/ORDER_ID",
			want:    "/api/pay/toapp/ORDER_ID",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveEasyPayReturnedRef(tt.apiBase, tt.ref); got != tt.want {
				t.Fatalf("resolveEasyPayReturnedRef(%q, %q) = %q, want %q", tt.apiBase, tt.ref, got, tt.want)
			}
		})
	}
}

// newEasyPayMapiStub serves mapi.php with a canned body and reports the api base
// the provider should be configured with. basePath lets a test prove that a
// root-relative reference drops the api base's own path prefix.
func newEasyPayMapiStub(t *testing.T, basePath, body string) (*EasyPay, string) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	apiBase := server.URL + basePath
	provider, err := NewEasyPay("test-instance", map[string]string{
		"pid":       "pid-1",
		"pkey":      "pkey-1",
		"apiBase":   apiBase,
		"notifyUrl": "https://example.com/notify",
		"returnUrl": "https://example.com/return",
	})
	if err != nil {
		t.Fatalf("NewEasyPay: %v", err)
	}
	return provider, server.URL
}

func TestEasyPayCreatePaymentResolvesRelativeQRCode(t *testing.T) {
	t.Parallel()

	provider, origin := newEasyPayMapiStub(t, "/xpay/epay",
		`{"code":1,"trade_no":"TRADE_NO","payurl":"","qrcode":"/api/pay/toapp/ORDER_ID"}`)

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2-relative-qr",
		Amount:      "1.00",
		PaymentType: payment.TypeWxpay,
		Subject:     "Relative QR",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if want := origin + "/api/pay/toapp/ORDER_ID"; resp.QRCode != want {
		t.Fatalf("QRCode = %q, want %q (a bare path would render as text in WeChat)", resp.QRCode, want)
	}
	if resp.PayURL != "" {
		t.Fatalf("PayURL = %q, want empty (upstream sent none)", resp.PayURL)
	}
	if resp.TradeNo != "TRADE_NO" {
		t.Fatalf("TradeNo = %q, want TRADE_NO", resp.TradeNo)
	}
}

func TestEasyPayCreatePaymentResolvesRelativeMobilePayURL2(t *testing.T) {
	t.Parallel()

	provider, origin := newEasyPayMapiStub(t, "",
		`{"code":1,"trade_no":"TRADE_NO","payurl":"/pc/pay/ORDER_ID","payurl2":"/h5/pay/ORDER_ID"}`)

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2-relative-h5",
		Amount:      "1.00",
		PaymentType: payment.TypeWxpay,
		Subject:     "Relative H5",
		IsMobile:    true,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	// payurl2 优先于 payurl 的选择语义不变，只是补全了地址。
	if want := origin + "/h5/pay/ORDER_ID"; resp.PayURL != want {
		t.Fatalf("PayURL = %q, want %q", resp.PayURL, want)
	}
}

func TestEasyPayCreatePaymentKeepsUpstreamAbsoluteAndOpaqueValues(t *testing.T) {
	t.Parallel()

	const (
		absoluteURL = "https://cashier.example.com/pay/ORDER_ID"
		deepLink    = "weixin://wxpay/bizpayurl?pr=ABCdef"
	)
	provider, _ := newEasyPayMapiStub(t, "/xpay/epay",
		`{"code":1,"trade_no":"TRADE_NO","payurl":"`+absoluteURL+`","qrcode":"`+deepLink+`"}`)

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2-absolute",
		Amount:      "1.00",
		PaymentType: payment.TypeWxpay,
		Subject:     "Absolute",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if resp.PayURL != absoluteURL {
		t.Fatalf("PayURL = %q, want %q unchanged", resp.PayURL, absoluteURL)
	}
	if resp.QRCode != deepLink {
		t.Fatalf("QRCode = %q, want %q unchanged", resp.QRCode, deepLink)
	}
}
