//go:build unit

package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

const testInfiniSecret = "sk_test_secret"
const testInfiniWebhookSecret = "whsec_test"
const testInfiniKeyID = "pk_test_keyid"

func TestNewInfiniValidatesConfig(t *testing.T) {
	t.Parallel()

	_, err := NewInfini("1", map[string]string{
		"keyId":         testInfiniKeyID,
		"secretKey":     testInfiniSecret,
		"webhookSecret": testInfiniWebhookSecret,
	})
	require.ErrorContains(t, err, "apiBase")

	_, err = NewInfini("1", map[string]string{
		"keyId":         testInfiniKeyID,
		"secretKey":     testInfiniSecret,
		"webhookSecret": testInfiniWebhookSecret,
		"apiBase":       "https://evil.example.com",
	})
	require.ErrorContains(t, err, "apiBase host")

	_, err = NewInfini("1", map[string]string{
		"keyId":         testInfiniKeyID,
		"secretKey":     testInfiniSecret,
		"webhookSecret": testInfiniWebhookSecret,
		"apiBase":       infiniProdAPIBase + "/v1",
	})
	require.ErrorContains(t, err, "must not carry a path")

	prov, err := NewInfini("1", map[string]string{
		"keyId":         testInfiniKeyID,
		"secretKey":     testInfiniSecret,
		"webhookSecret": testInfiniWebhookSecret,
		"apiBase":       infiniSandboxAPIBase,
	})
	require.NoError(t, err)
	require.Equal(t, payment.TypeInfini, prov.ProviderKey())
	require.Equal(t, []payment.PaymentType{payment.TypeInfini}, prov.SupportedTypes())
	// Infini prices in foreign currency only, so an unset currency means USD
	// rather than the platform-wide CNY default.
	require.Equal(t, infiniDefaultCurrency, prov.config["currency"])
	require.Equal(t, map[string]string{"currency": "USD", "key_id": testInfiniKeyID}, prov.MerchantIdentityMetadata())
}

func TestInfiniSigningStringKeepsQueryAndExcludesBody(t *testing.T) {
	t.Parallel()

	signing := infiniSigningString("pk_1", "get", "/v1/acquiring/order?order_id=abc", "Tue, 21 Jan 2025 12:00:00 GMT")
	// The trailing newline is part of the signed bytes; without it Infini
	// answers 401 "client request can't be validated".
	require.Equal(t, "pk_1\nGET /v1/acquiring/order?order_id=abc\ndate: Tue, 21 Jan 2025 12:00:00 GMT\n", signing)
	require.True(t, strings.HasSuffix(signing, "\n"))

	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(signing))
	require.Equal(t, base64.StdEncoding.EncodeToString(mac.Sum(nil)), infiniSignature("secret", signing))

	require.Equal(t,
		`Signature keyId="pk_1",algorithm="hmac-sha256",headers="@request-target date",signature="sig"`,
		infiniAuthorizationHeader("pk_1", "sig"))

	sum := sha256.Sum256([]byte(`{"a":1}`))
	require.Equal(t, "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]), infiniDigestHeader([]byte(`{"a":1}`)))
}

func TestInfiniCreatePaymentSignsRequestAndMapsCheckoutURL(t *testing.T) {
	t.Parallel()

	var payload infiniCreateOrderRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, infiniOrderPath, r.URL.RequestURI())
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, infiniDigestHeader(body), r.Header.Get("Digest"))

		date := r.Header.Get("Date")
		require.NotEmpty(t, date)
		_, err = time.Parse(http.TimeFormat, date)
		require.NoError(t, err)
		expected := infiniAuthorizationHeader(testInfiniKeyID,
			infiniSignature(testInfiniSecret, infiniSigningString(testInfiniKeyID, http.MethodPost, infiniOrderPath, date)))
		require.Equal(t, expected, r.Header.Get("Authorization"))

		require.NoError(t, json.Unmarshal(body, &payload))
		// Shaped like a live response: the object sits inside the envelope.
		_, _ = w.Write([]byte(`{"code":0,"message":"","data":{"order_id":"ord_1","request_id":"` + payload.RequestID + `","checkout_url":"https://checkout.infini.money/pay/xyz","client_reference":"sub2_order"}}`))
	}))
	defer server.Close()

	prov := mustTestInfiniProvider(t, server, nil)
	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:    "sub2_order",
		Amount:     "12.34",
		Subject:    "Balance recharge",
		ReturnURL:  "https://merchant.example.com/payment/result",
		PayerEmail: "payer@example.com",
		ExpiresIn:  1800,
	})
	require.NoError(t, err)
	require.Equal(t, "ord_1", resp.TradeNo)
	require.Equal(t, "https://checkout.infini.money/pay/xyz", resp.PayURL)
	require.Equal(t, "USD", resp.Currency)
	require.Equal(t, payment.CreatePaymentResultOrderCreated, resp.ResultType)

	require.Equal(t, "12.34", payload.Amount)
	require.Equal(t, "USD", payload.Currency)
	require.Equal(t, "sub2_order", payload.ClientReference)
	require.Equal(t, "Balance recharge", payload.OrderDesc)
	require.Equal(t, "payer@example.com", payload.Email)
	require.Equal(t, 1800, payload.ExpiresIn)
	require.Equal(t, "https://merchant.example.com/payment/result", payload.SuccessURL)
	require.Equal(t, "https://merchant.example.com/payment/result", payload.FailureURL)
	// Retrying the same order must reuse one upstream idempotency key.
	require.Equal(t, infiniDeterministicRequestID("order", "sub2_order", "12.34", "USD"), payload.RequestID)
}

func TestInfiniDecodesResponseEnvelope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantID  string
		wantURL string
		wantErr string
	}{
		{
			// Live responses wrap the object in {code,message,data}.
			name:    "enveloped",
			body:    `{"code":0,"message":"","data":{"order_id":"ord_1","checkout_url":"https://checkout.infini.money/pay/x"}}`,
			wantID:  "ord_1",
			wantURL: "https://checkout.infini.money/pay/x",
		},
		{
			// The docs show the object at the top level; accept that too.
			name:    "bare object",
			body:    `{"order_id":"ord_1","checkout_url":"https://checkout.infini.money/pay/x"}`,
			wantID:  "ord_1",
			wantURL: "https://checkout.infini.money/pay/x",
		},
		{
			// A non-zero code is an error even under HTTP 200.
			name:    "business error under HTTP 200",
			body:    `{"code":40901,"message":"duplicate request_id","detail":"already used"}`,
			wantErr: "code=40901",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			prov := mustTestInfiniProvider(t, server, nil)
			resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
				OrderID: "sub2_order",
				Amount:  "10",
			})
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantID, resp.TradeNo)
			require.Equal(t, tc.wantURL, resp.PayURL)
		})
	}
}

func TestInfiniQueryOrderDecodesEnvelope(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"","data":{"order_id":"ord_1","status":"paid","amount":"100","amount_confirmed":"100","currency":"USD"}}`))
	}))
	defer server.Close()

	prov := mustTestInfiniProvider(t, server, nil)
	resp, err := prov.QueryOrder(context.Background(), "ord_1")
	require.NoError(t, err)
	require.Equal(t, "ord_1", resp.TradeNo)
	require.Equal(t, payment.ProviderStatusPaid, resp.Status)
	require.InDelta(t, 100, resp.Amount, 0.0001)
	require.Equal(t, "USD", resp.Metadata["currency"])
}

func TestShortenInfiniRedirectURL(t *testing.T) {
	t.Parallel()

	base := "https://sub.266667.xyz/payment/result"
	short := base + "?order_id=9&out_trade_no=sub2_abc&status=success"
	require.Equal(t, short, shortenInfiniRedirectURL(short), "a URL within the limit is untouched")
	require.Empty(t, shortenInfiniRedirectURL("  "))

	// A resume token pushes the real URL past what Binance Pay accepts.
	long := short + "&resume_token=" + strings.Repeat("A", 250)
	require.Greater(t, len(long), infiniMaxRedirectURLLen)
	got := shortenInfiniRedirectURL(long)
	require.LessOrEqual(t, len(got), infiniMaxRedirectURLLen)
	require.NotContains(t, got, "resume_token")
	// out_trade_no must survive: the result page resolves the order with it
	// when no resume token is present.
	require.Contains(t, got, "out_trade_no=sub2_abc")
	require.Contains(t, got, "order_id=9")

	// Even an absurd base is cut down rather than passed through.
	absurd := base + "?order_id=9&out_trade_no=" + strings.Repeat("B", 400)
	require.LessOrEqual(t, len(shortenInfiniRedirectURL(absurd)), infiniMaxRedirectURLLen)
}

func TestInfiniCreatePaymentShortensRedirectURLs(t *testing.T) {
	t.Parallel()

	var payload infiniCreateOrderRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &payload))
		_, _ = w.Write([]byte(`{"code":0,"message":"","data":{"order_id":"ord_1","checkout_url":"https://checkout.infini.money/pay/xyz"}}`))
	}))
	defer server.Close()

	longReturn := "https://sub.266667.xyz/payment/result?order_id=9&out_trade_no=sub2_abc&status=success&resume_token=" + strings.Repeat("A", 250)
	prov := mustTestInfiniProvider(t, server, nil)
	_, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "sub2_abc",
		Amount:    "10",
		ReturnURL: longReturn,
	})
	require.NoError(t, err)
	require.LessOrEqual(t, len(payload.SuccessURL), infiniMaxRedirectURLLen)
	require.LessOrEqual(t, len(payload.FailureURL), infiniMaxRedirectURLLen)
	require.Equal(t, payload.SuccessURL, payload.FailureURL)
	require.Contains(t, payload.SuccessURL, "out_trade_no=sub2_abc")
}

func TestInfiniCreatePaymentOmitsEmailWhenNotForwarded(t *testing.T) {
	t.Parallel()

	var raw map[string]any
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &raw))
		_, _ = w.Write([]byte(`{"code":0,"message":"","data":{"order_id":"ord_1","checkout_url":"https://checkout.infini.money/pay/xyz"}}`))
	}))
	defer server.Close()

	prov := mustTestInfiniProvider(t, server, nil)
	_, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_order",
		Amount:  "10",
	})
	require.NoError(t, err)
	require.NotContains(t, raw, "email")
	require.NotContains(t, raw, "expires_in")
}

func TestInfiniCreatePaymentSurfacesAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":40001,"message":"Invalid request","detail":"amount must be positive"}`))
	}))
	defer server.Close()

	prov := mustTestInfiniProvider(t, server, nil)
	_, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "sub2_order", Amount: "10"})
	require.ErrorContains(t, err, "code=40001")
	require.ErrorContains(t, err, "amount must be positive")
}

func TestInfiniQueryOrderSignsQueryStringAndMapsStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		wantState string
		wantAmt   float64
	}{
		{"paid", `{"order_id":"ord_1","status":"paid","amount":"100","amount_confirmed":"100","currency":"USD"}`, payment.ProviderStatusPaid, 100},
		{"pending", `{"order_id":"ord_1","status":"pending","amount":"100","amount_confirmed":"0","currency":"USD"}`, payment.ProviderStatusPending, 100},
		{"processing", `{"order_id":"ord_1","status":"processing","amount":"100","amount_confirmed":"0","currency":"USD"}`, payment.ProviderStatusPending, 100},
		{"expired unpaid", `{"order_id":"ord_1","status":"expired","amount":"100","amount_confirmed":"0","currency":"USD"}`, payment.ProviderStatusFailed, 100},
		{"underpaid", `{"order_id":"ord_1","status":"partial_paid","amount":"100","amount_confirmed":"40","currency":"USD"}`, payment.ProviderStatusFailed, 100},
		// Funds confirmed after expiry still settle the order.
		{"late full payment", `{"order_id":"ord_1","status":"expired","amount":"100","amount_confirmed":"100","currency":"USD"}`, payment.ProviderStatusPaid, 100},
		// Numeric amounts decode the same as string amounts.
		{"numeric amount", `{"order_id":"ord_1","status":"paid","amount":100,"amount_confirmed":100,"currency":"USD"}`, payment.ProviderStatusPaid, 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wantURI := infiniOrderPath + "?order_id=ord_1"
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, wantURI, r.URL.RequestURI())
				require.Empty(t, r.Header.Get("Digest"))
				date := r.Header.Get("Date")
				expected := infiniAuthorizationHeader(testInfiniKeyID,
					infiniSignature(testInfiniSecret, infiniSigningString(testInfiniKeyID, http.MethodGet, wantURI, date)))
				require.Equal(t, expected, r.Header.Get("Authorization"))
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			prov := mustTestInfiniProvider(t, server, nil)
			resp, err := prov.QueryOrder(context.Background(), "ord_1")
			require.NoError(t, err)
			require.Equal(t, tc.wantState, resp.Status)
			require.InDelta(t, tc.wantAmt, resp.Amount, 0.0001)
			require.Equal(t, "USD", resp.Metadata["currency"])
		})
	}
}

func TestInfiniVerifyNotificationRejectsBadSignatures(t *testing.T) {
	t.Parallel()

	prov := mustTestInfiniProvider(t, nil, nil)
	body := `{"event":"order.completed","order_id":"ord_1","client_reference":"sub2_order","status":"paid","amount":"100","currency":"USD"}`
	now := time.Now()

	_, err := prov.VerifyNotification(context.Background(), body, map[string]string{})
	require.ErrorContains(t, err, "missing webhook signature headers")

	headers := signedInfiniHeaders(body, strconv.FormatInt(now.Unix(), 10), "evt_1", testInfiniWebhookSecret)
	tampered := `{"event":"order.completed","order_id":"ord_1","client_reference":"sub2_order","status":"paid","amount":"999","currency":"USD"}`
	_, err = prov.VerifyNotification(context.Background(), tampered, headers)
	require.ErrorContains(t, err, "invalid signature")

	stale := strconv.FormatInt(now.Add(-2*infiniWebhookTolerance).Unix(), 10)
	_, err = prov.VerifyNotification(context.Background(), body, signedInfiniHeaders(body, stale, "evt_1", testInfiniWebhookSecret))
	require.ErrorContains(t, err, "outside tolerance")

	// The tolerance must span Infini's full retry backoff (~16 minutes).
	require.GreaterOrEqual(t, infiniWebhookTolerance, 20*time.Minute)
}

func TestInfiniVerifyNotificationAcceptsMillisecondTimestamps(t *testing.T) {
	t.Parallel()

	prov := mustTestInfiniProvider(t, nil, nil)
	body := `{"event":"order.completed","order_id":"ord_1","client_reference":"sub2_order","status":"paid","amount":"100","currency":"USD"}`
	headers := signedInfiniHeaders(body, strconv.FormatInt(time.Now().UnixMilli(), 10), "evt_1", testInfiniWebhookSecret)

	notification, err := prov.VerifyNotification(context.Background(), body, headers)
	require.NoError(t, err)
	require.NotNil(t, notification)
}

func TestInfiniVerifyNotificationDispatchesEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		wantNil     bool
		wantStatus  string
		wantAnomaly string
		wantAmount  float64
		wantErr     string
	}{
		{
			name:       "completed credits the order",
			body:       `{"event":"order.completed","order_id":"ord_1","client_reference":"sub2_order","status":"paid","amount":"100","amount_confirmed":"100","currency":"USD"}`,
			wantStatus: payment.NotificationStatusSuccess,
			wantAmount: 100,
		},
		{
			name:    "completed with non-paid status is rejected",
			body:    `{"event":"order.completed","order_id":"ord_1","client_reference":"sub2_order","status":"processing","amount":"100","currency":"USD"}`,
			wantErr: "non-paid status",
		},
		{
			name:       "late full payment credits the order",
			body:       `{"event":"order.late_payment","order_id":"ord_1","client_reference":"sub2_order","status":"expired","amount":"100","amount_confirmed":"100","currency":"USD"}`,
			wantStatus: payment.NotificationStatusSuccess,
			wantAmount: 100,
		},
		{
			// An overpayment must still report the order amount, otherwise the
			// service-layer amount check rejects it.
			name:       "late overpayment reports the order amount",
			body:       `{"event":"order.late_payment","order_id":"ord_1","client_reference":"sub2_order","status":"expired","amount":"100","amount_confirmed":"140","currency":"USD"}`,
			wantStatus: payment.NotificationStatusSuccess,
			wantAmount: 100,
		},
		{
			name:        "late underpayment is an anomaly",
			body:        `{"event":"order.late_payment","order_id":"ord_1","client_reference":"sub2_order","status":"expired","amount":"100","amount_confirmed":"40","currency":"USD"}`,
			wantStatus:  payment.ProviderStatusFailed,
			wantAnomaly: payment.NotificationAnomalyPartialPaid,
			wantAmount:  100,
		},
		{
			name:        "expired with partial funds is an anomaly",
			body:        `{"event":"order.expired","order_id":"ord_1","client_reference":"sub2_order","status":"partial_paid","amount":"100","amount_confirmed":"40","exception_tags":["late"],"currency":"USD"}`,
			wantStatus:  payment.ProviderStatusFailed,
			wantAnomaly: payment.NotificationAnomalyPartialPaid,
			wantAmount:  100,
		},
		{
			name:    "expired without funds is ignored",
			body:    `{"event":"order.expired","order_id":"ord_1","client_reference":"sub2_order","status":"expired","amount":"100","amount_confirmed":"0","currency":"USD"}`,
			wantNil: true,
		},
		{
			name:    "order created is ignored",
			body:    `{"event":"order.create","order_id":"ord_1","client_reference":"sub2_order","status":"pending","amount":"100","currency":"USD"}`,
			wantNil: true,
		},
		{
			name:    "order processing is ignored",
			body:    `{"event":"order.processing","order_id":"ord_1","client_reference":"sub2_order","status":"processing","amount":"100","currency":"USD"}`,
			wantNil: true,
		},
		{
			name:    "card events are ignored",
			body:    `{"id":"evt","event":"card.transaction","version":1,"data":{}}`,
			wantNil: true,
		},
		{
			name:    "missing client reference is rejected",
			body:    `{"event":"order.completed","order_id":"ord_1","status":"paid","amount":"100","currency":"USD"}`,
			wantErr: "missing order_id or client_reference",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			prov := mustTestInfiniProvider(t, nil, nil)
			headers := signedInfiniHeaders(tc.body, strconv.FormatInt(time.Now().Unix(), 10), "evt_1", testInfiniWebhookSecret)
			notification, err := prov.VerifyNotification(context.Background(), tc.body, headers)

			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				require.Nil(t, notification)
				return
			}
			require.NotNil(t, notification)
			require.Equal(t, tc.wantStatus, notification.Status)
			require.Equal(t, tc.wantAnomaly, notification.Anomaly)
			require.InDelta(t, tc.wantAmount, notification.Amount, 0.0001)
			require.Equal(t, "sub2_order", notification.OrderID)
			require.Equal(t, "ord_1", notification.TradeNo)
			require.Equal(t, "USD", notification.Metadata["currency"])
			require.Equal(t, tc.body, notification.RawData)
		})
	}
}

func TestInfiniRefundIsUnsupported(t *testing.T) {
	t.Parallel()

	prov := mustTestInfiniProvider(t, nil, nil)
	_, err := prov.Refund(context.Background(), payment.RefundRequest{TradeNo: "ord_1", Amount: "10"})
	require.ErrorContains(t, err, "INFINI_REFUND_UNSUPPORTED")
}

func mustTestInfiniProvider(t *testing.T, server *httptest.Server, overrides map[string]string) *Infini {
	t.Helper()
	config := map[string]string{
		"keyId":         testInfiniKeyID,
		"secretKey":     testInfiniSecret,
		"webhookSecret": testInfiniWebhookSecret,
		"apiBase":       infiniSandboxAPIBase,
	}
	for k, v := range overrides {
		config[k] = v
	}
	prov, err := NewInfini("1", config)
	require.NoError(t, err)
	if server != nil {
		prov.config["apiBase"] = server.URL
		prov.httpClient = server.Client()
	}
	return prov
}

func signedInfiniHeaders(rawBody, timestamp, eventID, secret string) map[string]string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + eventID + "." + rawBody))
	return map[string]string{
		infiniWebhookTimestampHeader: timestamp,
		infiniWebhookEventIDHeader:   eventID,
		infiniWebhookSignatureHeader: hex.EncodeToString(mac.Sum(nil)),
	}
}
