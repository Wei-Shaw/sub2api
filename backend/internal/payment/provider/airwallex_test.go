//go:build unit

package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
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

func TestNewAirwallexValidatesConfig(t *testing.T) {
	t.Parallel()

	_, err := NewAirwallex("1", map[string]string{
		"clientId":      "cid",
		"apiKey":        "key",
		"webhookSecret": "secret",
		"apiBase":       "https://evil.example.com/api/v1",
	})
	require.ErrorContains(t, err, "apiBase host")

	_, err = NewAirwallex("1", map[string]string{
		"clientId":      "cid",
		"apiKey":        "key",
		"webhookSecret": "secret",
		"apiBase":       airwallexDemoAPIBase,
		"countryCode":   "C1",
	})
	require.ErrorContains(t, err, "countryCode")

	prov, err := NewAirwallex("1", map[string]string{
		"clientId":      "cid",
		"apiKey":        "key",
		"webhookSecret": "secret",
		"apiBase":       airwallexDemoAPIBase,
	})
	require.NoError(t, err)
	require.Equal(t, payment.TypeAirwallex, prov.ProviderKey())
	require.Equal(t, []payment.PaymentType{payment.TypeAirwallex}, prov.SupportedTypes())
	require.Equal(t, payment.DefaultPaymentCurrency, prov.config["currency"])
	require.Equal(t, airwallexDefaultCountry, prov.config["countryCode"])
}

func TestAirwallexCreatePaymentUsesServerAmountAndStableRequestID(t *testing.T) {
	t.Parallel()

	var createRequests []airwallexCreatePaymentIntentRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/authentication/login":
			require.Equal(t, "cid", r.Header.Get("x-client-id"))
			require.Equal(t, "key", r.Header.Get("x-api-key"))
			_, _ = w.Write([]byte(`{"token":"token-1","expires_at":"2099-01-01T00:00:00Z"}`))
		case "/api/v1/pa/payment_intents/create":
			require.Equal(t, "Bearer token-1", r.Header.Get("Authorization"))
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.Contains(t, string(body), `"amount":12.34`)
			var payload airwallexCreatePaymentIntentRequest
			require.NoError(t, json.Unmarshal(body, &payload))
			createRequests = append(createRequests, payload)
			_, _ = w.Write([]byte(`{"id":"int_123","client_secret":"secret_123","amount":12.34,"currency":"CNY","merchant_order_id":"sub2_order","status":"REQUIRES_PAYMENT_METHOD"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov := mustTestAirwallexProvider(t, server)
	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "sub2_order",
		Amount:    "12.34",
		ReturnURL: "https://merchant.example.com/payment/result",
	})
	require.NoError(t, err)
	require.Equal(t, "int_123", resp.TradeNo)
	require.Equal(t, "secret_123", resp.ClientSecret)
	require.Equal(t, "int_123", resp.IntentID)
	require.Equal(t, "CNY", resp.Currency)
	require.Equal(t, "CN", resp.CountryCode)
	require.Equal(t, "demo", resp.PaymentEnv)
	require.Len(t, createRequests, 1)
	require.Equal(t, "12.34", createRequests[0].Amount.StringFixed(2))
	require.Equal(t, "CNY", createRequests[0].Currency)
	require.Equal(t, "sub2_order", createRequests[0].MerchantOrderID)
	require.Equal(t, airwallexDeterministicRequestID("payment-intent", "sub2_order", "12.34", "CNY"), createRequests[0].RequestID)
}

func TestAirwallexCreatePaymentUsesConfiguredCurrency(t *testing.T) {
	t.Parallel()

	var createRequest airwallexCreatePaymentIntentRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/authentication/login":
			_, _ = w.Write([]byte(`{"token":"token-1","expires_at":"2099-01-01T00:00:00Z"}`))
		case "/api/v1/pa/payment_intents/create":
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &createRequest))
			_, _ = w.Write([]byte(`{"id":"int_123","client_secret":"secret_123","amount":12.34,"currency":"HKD","merchant_order_id":"sub2_order","status":"REQUIRES_PAYMENT_METHOD"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov, err := NewAirwallex("1", map[string]string{
		"clientId":      "cid",
		"apiKey":        "key",
		"webhookSecret": "whsec",
		"apiBase":       airwallexDemoAPIBase,
		"currency":      "hkd",
		"countryCode":   "HK",
	})
	require.NoError(t, err)
	prov.config["apiBase"] = server.URL + "/api/v1"
	prov.httpClient = server.Client()

	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "sub2_order",
		Amount:    "12.34",
		ReturnURL: "https://merchant.example.com/payment/result",
	})
	require.NoError(t, err)
	require.Equal(t, "HKD", createRequest.Currency)
	require.Equal(t, "HKD", resp.Currency)
	require.Equal(t, "HK", resp.CountryCode)
	require.Equal(t, "HKD", prov.MerchantIdentityMetadata()["currency"])
}

func TestAirwallexRequestsUseConfiguredAccountID(t *testing.T) {
	t.Parallel()

	paRequestCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/authentication/login":
			require.Equal(t, "acct_123", r.Header.Get("x-login-as"))
			_, _ = w.Write([]byte(`{"token":"token-1","expires_at":"2099-01-01T00:00:00Z"}`))
		case "/api/v1/pa/payment_intents/create":
			paRequestCount++
			require.Equal(t, "acct_123", r.Header.Get("x-on-behalf-of"))
			_, _ = w.Write([]byte(`{"id":"int_123","client_secret":"secret_123","amount":12.34,"currency":"CNY","merchant_order_id":"sub2_order","status":"REQUIRES_PAYMENT_METHOD"}`))
		case "/api/v1/pa/payment_intents/int_123":
			paRequestCount++
			require.Equal(t, "acct_123", r.Header.Get("x-on-behalf-of"))
			_, _ = w.Write([]byte(`{"id":"int_123","amount":12.34,"currency":"CNY","merchant_order_id":"sub2_order","status":"SUCCEEDED"}`))
		case "/api/v1/pa/refunds/create":
			paRequestCount++
			require.Equal(t, "acct_123", r.Header.Get("x-on-behalf-of"))
			_, _ = w.Write([]byte(`{"id":"ref_123","payment_intent_id":"int_123","amount":12.34,"currency":"CNY","status":"SETTLED"}`))
		case "/api/v1/pa/payment_intents/int_123/cancel":
			paRequestCount++
			require.Equal(t, "acct_123", r.Header.Get("x-on-behalf-of"))
			_, _ = w.Write([]byte(`{"id":"int_123","amount":12.34,"currency":"CNY","merchant_order_id":"sub2_order","status":"CANCELLED"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov, err := NewAirwallex("1", map[string]string{
		"clientId":      "cid",
		"apiKey":        "key",
		"webhookSecret": "whsec",
		"apiBase":       airwallexDemoAPIBase,
		"accountId":     "acct_123",
	})
	require.NoError(t, err)
	prov.config["apiBase"] = server.URL + "/api/v1"
	prov.httpClient = server.Client()

	_, err = prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_order",
		Amount:  "12.34",
	})
	require.NoError(t, err)
	_, err = prov.QueryOrder(context.Background(), "int_123")
	require.NoError(t, err)
	_, err = prov.Refund(context.Background(), payment.RefundRequest{
		TradeNo: "int_123",
		Amount:  "12.34",
		Reason:  "test refund",
	})
	require.NoError(t, err)
	require.NoError(t, prov.CancelPayment(context.Background(), "int_123"))
	require.Contains(t, prov.tokenCacheKey(), "acct_123")
	require.Equal(t, 4, paRequestCount)
}

func TestAirwallexRefundRejectsUnsettledStatus(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"RECEIVED", "ACCEPTED", "FAILED"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/authentication/login":
					_, _ = w.Write([]byte(`{"token":"token-1","expires_at":"2099-01-01T00:00:00Z"}`))
				case "/api/v1/pa/refunds/create":
					_, _ = w.Write([]byte(`{"id":"ref_123","payment_intent_id":"int_123","amount":12.34,"currency":"CNY","status":"` + status + `"}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			prov := mustTestAirwallexProvider(t, server)
			resp, err := prov.Refund(context.Background(), payment.RefundRequest{
				TradeNo: "int_123",
				Amount:  "12.34",
				Reason:  "test refund",
			})

			require.ErrorContains(t, err, "airwallex refund not settled")
			require.NotNil(t, resp)
			require.Equal(t, "ref_123", resp.RefundID)
			if status == airwallexRefundStatusFailed {
				require.Equal(t, payment.ProviderStatusFailed, resp.Status)
			} else {
				require.Equal(t, payment.ProviderStatusPending, resp.Status)
			}
		})
	}
}

func TestAirwallexAuthErrorIncludesCredentialGuidance(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/authentication/login", r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"credentials_invalid","details":["Access Denied"],"message":"UNAUTHORIZED","source":""}`))
	}))
	defer server.Close()

	prov := mustTestAirwallexProvider(t, server)
	_, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_order",
		Amount:  "12.34",
	})

	require.ErrorContains(t, err, "credentials_invalid")
	require.ErrorContains(t, err, "API Base environment")
	require.ErrorContains(t, err, "https://api-demo.airwallex.com/api/v1")
	require.ErrorContains(t, err, "https://api.airwallex.com/api/v1")
	require.ErrorContains(t, err, "Account ID")
}

func TestAirwallexVerifyNotificationRequiresValidSignatureAndCurrency(t *testing.T) {
	t.Parallel()

	prov, err := NewAirwallex("1", map[string]string{
		"clientId":      "cid",
		"apiKey":        "key",
		"webhookSecret": "whsec",
		"apiBase":       airwallexDemoAPIBase,
		"accountId":     "acct_123",
	})
	require.NoError(t, err)

	raw := `{"id":"evt_1","name":"payment_intent.succeeded","accountId":"acct_123","data":{"object":{"id":"int_123","merchant_order_id":"sub2_abc","amount":88.66,"currency":"CNY","status":"SUCCEEDED"}}}`
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	headers := signedAirwallexHeaders(raw, timestamp, "whsec")

	n, err := prov.VerifyNotification(context.Background(), raw, headers)
	require.NoError(t, err)
	require.NotNil(t, n)
	require.Equal(t, "int_123", n.TradeNo)
	require.Equal(t, "sub2_abc", n.OrderID)
	require.Equal(t, payment.NotificationStatusSuccess, n.Status)
	require.InDelta(t, 88.66, n.Amount, 0.0001)
	require.Equal(t, "CNY", n.Metadata["currency"])
	require.Equal(t, "acct_123", n.Metadata["account_id"])

	headers["x-signature"] = strings.Repeat("0", 64)
	_, err = prov.VerifyNotification(context.Background(), raw, headers)
	require.ErrorContains(t, err, "invalid signature")
}

func TestVerifyAirwallexWebhookSignatureRejectsReplay(t *testing.T) {
	t.Parallel()

	raw := `{"id":"evt_1"}`
	timestamp := "1778241600000"
	headers := signedAirwallexHeaders(raw, timestamp, "whsec")
	err := verifyAirwallexWebhookSignature(raw, headers, "whsec", time.UnixMilli(1778241600000).Add(airwallexWebhookTolerance+time.Millisecond))
	require.ErrorContains(t, err, "outside tolerance")
}

func TestAirwallexQueryOrderMapsSucceeded(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/authentication/login":
			_, _ = w.Write([]byte(`{"token":"token-1","expires_at":"2099-01-01T00:00:00Z"}`))
		case "/api/v1/pa/payment_intents/int_123":
			_, _ = w.Write([]byte(`{"id":"int_123","amount":99.01,"currency":"CNY","merchant_order_id":"sub2_order","status":"SUCCEEDED"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov := mustTestAirwallexProvider(t, server)
	resp, err := prov.QueryOrder(context.Background(), "int_123")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, resp.Status)
	require.InDelta(t, 99.01, resp.Amount, 0.0001)
	require.Equal(t, "CNY", resp.Metadata["currency"])
	require.Equal(t, "SUCCEEDED", resp.Metadata["status"])
}

func mustTestAirwallexProvider(t *testing.T, server *httptest.Server) *Airwallex {
	t.Helper()
	prov, err := NewAirwallex("1", map[string]string{
		"clientId":      "cid",
		"apiKey":        "key",
		"webhookSecret": "whsec",
		"apiBase":       airwallexDemoAPIBase,
	})
	require.NoError(t, err)
	prov.config["apiBase"] = server.URL + "/api/v1"
	prov.httpClient = server.Client()
	return prov
}

func signedAirwallexHeaders(rawBody, timestamp, secret string) map[string]string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte(rawBody))
	return map[string]string{
		"x-timestamp": timestamp,
		"x-signature": hex.EncodeToString(mac.Sum(nil)),
	}
}
