package provider

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type antomTestTransport func(*http.Request) (*http.Response, error)

func (f antomTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func antomTestProvider(t *testing.T) (*Antom, *rsa.PrivateKey) {
	t.Helper()
	merchant, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	upstream, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pub, err := x509.MarshalPKIXPublicKey(&upstream.PublicKey)
	require.NoError(t, err)
	prov, err := NewAntom("1", map[string]string{
		"clientId": "SANDBOX_test", "currency": "USD", "notifyUrl": "https://merchant.example/api/v1/payment/webhook/antom",
		"merchantPrivateKey": string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(merchant)})),
		"antomPublicKey":     string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})),
	})
	require.NoError(t, err)
	return prov, upstream
}

func antomTestSign(t *testing.T, key *rsa.PrivateKey, uri, client, timestamp, body string) string {
	t.Helper()
	digest := sha256.Sum256([]byte("POST " + uri + "\n" + client + "." + timestamp + "." + body))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	require.NoError(t, err)
	return "algorithm=RSA256, keyVersion=1, signature=" + url.QueryEscape(base64.StdEncoding.EncodeToString(signature))
}

func antomTestResponse(t *testing.T, key *rsa.PrivateKey, path, body string) *http.Response {
	t.Helper()
	header := http.Header{}
	header.Set("client-id", "SANDBOX_test")
	header.Set("response-time", "2026-09-16T12:00:00+08:00")
	header.Set("signature", antomTestSign(t, key, path, header.Get("client-id"), header.Get("response-time"), body))
	return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func antomTestHeaders(t *testing.T, key *rsa.PrivateKey, body string) map[string]string {
	t.Helper()
	path := "/api/v1/payment/webhook/antom?source=checkout"
	return map[string]string{":path": path, ":method": "POST", "client-id": "SANDBOX_test", "request-time": "1789531200000",
		"signature": antomTestSign(t, key, path, "SANDBOX_test", "1789531200000", body)}
}

func TestAntomHostedCheckoutSignsExactRequestAndRejectsUntrustedResponses(t *testing.T) {
	t.Parallel()
	prov, key := antomTestProvider(t)
	responseMode := "valid"
	prov.client.Transport = antomTestTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/ams/api/v1/payments/createPaymentSession", r.URL.Path)
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(raw, &payload))
		require.Equal(t, "CHECKOUT_PAYMENT", payload["productScene"])
		require.Equal(t, map[string]any{"currency": "USD", "value": "1234"}, payload["paymentAmount"])
		require.Equal(t, "WAP", payload["env"].(map[string]any)["terminalType"])
		require.Equal(t, "ANDROID", payload["env"].(map[string]any)["osType"])
		require.Equal(t, "42", payload["order"].(map[string]any)["buyer"].(map[string]any)["referenceBuyerId"])
		signed := strings.Split(r.Header.Get("signature"), "signature=")
		require.Len(t, signed, 2)
		encoded, err := url.QueryUnescape(signed[1])
		require.NoError(t, err)
		signature, err := base64.StdEncoding.DecodeString(encoded)
		require.NoError(t, err)
		digest := sha256.Sum256([]byte("POST " + r.URL.RequestURI() + "\nSANDBOX_test." + r.Header.Get("request-time") + "." + string(raw)))
		require.NoError(t, rsa.VerifyPKCS1v15(&prov.privateKey.PublicKey, crypto.SHA256, digest[:], signature))
		body := `{"result":{"resultCode":"SUCCESS","resultStatus":"S"},"normalUrl":"https://checkout.antom.com/session"}`
		response := antomTestResponse(t, key, r.URL.Path, body)
		switch responseMode {
		case "missing signature":
			response.Header.Del("signature")
		case "wrong client":
			response.Header.Set("client-id", "SANDBOX_other")
		case "tampered body":
			response.Body = io.NopCloser(strings.NewReader(strings.ReplaceAll(body, "/session", "/changed")))
		}
		return response, nil
	})
	req := payment.CreatePaymentRequest{OrderID: "sub2_123", BuyerID: "42", Amount: "12.34", Subject: "Balance", ReturnURL: "https://merchant.example/payment/result", IsMobile: true, UserAgent: "Mozilla/5.0 (Linux; Android 14) Mobile"}
	result, err := prov.CreatePayment(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "sub2_123", result.TradeNo)
	require.Equal(t, "https://checkout.antom.com/session", result.PayURL)
	for _, mode := range []string{"missing signature", "wrong client", "tampered body"} {
		t.Run(mode, func(t *testing.T) {
			responseMode = mode
			_, err := prov.CreatePayment(context.Background(), req)
			require.Error(t, err)
		})
	}
}

func TestAntomInquiryNeverCreditsAuthorizationOrAPISuccessAlone(t *testing.T) {
	t.Parallel()
	prov, key := antomTestProvider(t)
	for _, tc := range []struct {
		name, method, status, extra, want string
	}{
		{name: "api success payment processing", method: "ALIPAY_CN", status: "PROCESSING", want: payment.ProviderStatusPending},
		{name: "apm settled", method: "ALIPAY_CN", status: "SUCCESS", want: payment.ProviderStatusPaid},
		{name: "card authorization only", method: "CARD", status: "SUCCESS", want: payment.ProviderStatusPending},
		{name: "unknown method", status: "SUCCESS", want: payment.ProviderStatusPending},
		{name: "card capture pending", method: "CARD", status: "SUCCESS", extra: `,"transactions":[{"transactionType":"CAPTURE","transactionStatus":"PROCESSING","transactionResult":{"resultStatus":"U"},"transactionAmount":{"currency":"USD","value":"1234"}}]`, want: payment.ProviderStatusPending},
		{name: "card partially captured", method: "CARD", status: "SUCCESS", extra: `,"transactions":[{"transactionType":"CAPTURE","transactionStatus":"SUCCESS","transactionResult":{"resultStatus":"S"},"transactionAmount":{"currency":"USD","value":"500"}}]`, want: payment.ProviderStatusPending},
		{name: "card fully captured", method: "CARD", status: "SUCCESS", extra: `,"transactions":[{"transactionType":"CAPTURE","transactionStatus":"SUCCESS","transactionResult":{"resultStatus":"S"},"transactionAmount":{"currency":"USD","value":"1234"}}]`, want: payment.ProviderStatusPaid},
		{name: "apple pay authorization", method: "APPLEPAY", status: "SUCCESS", want: payment.ProviderStatusPending},
		{name: "failed", method: "ALIPAY_CN", status: "FAIL", want: payment.ProviderStatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prov.client.Transport = antomTestTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"result":{"resultCode":"SUCCESS","resultStatus":"S"},"paymentRequestId":"sub2_123","paymentId":"pay_123","paymentAmount":{"currency":"USD","value":"1234"},"paymentStatus":"` + tc.status + `","paymentMethodType":"` + tc.method + `"` + tc.extra + `}`
				return antomTestResponse(t, key, r.URL.Path, body), nil
			})
			result, err := prov.QueryOrder(context.Background(), "sub2_123")
			require.NoError(t, err)
			require.Equal(t, tc.want, result.Status)
			require.Equal(t, 12.34, result.Amount)
		})
	}
}

func TestAntomNotificationAuthenticityAndAuthorizationBoundary(t *testing.T) {
	t.Parallel()
	prov, key := antomTestProvider(t)
	raw := `{"notifyType":"PAYMENT_RESULT","paymentRequestId":"sub2_123","paymentId":"pay_123","paymentMethodType":"ALIPAY_CN","paymentAmount":{"currency":"USD","value":"1234"},"result":{"resultCode":"SUCCESS","resultStatus":"S"}}`
	headers := antomTestHeaders(t, key, raw)
	result, err := prov.VerifyNotification(context.Background(), raw, headers)
	require.NoError(t, err)
	require.Equal(t, "sub2_123", result.OrderID)
	require.Equal(t, 12.34, result.Amount)
	for _, kind := range []string{"body", "client", "path", "algorithm"} {
		t.Run(kind, func(t *testing.T) {
			changed := antomTestHeaders(t, key, raw)
			body := raw
			switch kind {
			case "body":
				body = strings.ReplaceAll(raw, "1234", "9234")
			case "client":
				changed["client-id"] = "SANDBOX_other"
			case "path":
				changed[":path"] = "/another/notify"
			case "algorithm":
				changed["signature"] = strings.ReplaceAll(changed["signature"], "RSA256", "RSA1")
			}
			_, err := prov.VerifyNotification(context.Background(), body, changed)
			require.Error(t, err)
		})
	}
	for _, body := range []string{strings.ReplaceAll(raw, "ALIPAY_CN", "CARD"), strings.ReplaceAll(raw, "PAYMENT_RESULT", "PAYMENT_PENDING")} {
		result, err := prov.VerifyNotification(context.Background(), body, antomTestHeaders(t, key, body))
		require.NoError(t, err)
		require.Nil(t, result)
	}
	for _, body := range []string{strings.ReplaceAll(raw, "USD", "CNY"), strings.ReplaceAll(raw, "1234", "12.34")} {
		_, err := prov.VerifyNotification(context.Background(), body, antomTestHeaders(t, key, body))
		require.Error(t, err)
	}
}

func TestAntomCaptureResolvesMerchantOrderAndRequiresFullAmount(t *testing.T) {
	t.Parallel()
	prov, key := antomTestProvider(t)
	prov.client.Transport = antomTestTransport(func(r *http.Request) (*http.Response, error) {
		var request map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "pay_123", request["paymentId"])
		return antomTestResponse(t, key, r.URL.Path, `{"result":{"resultCode":"SUCCESS","resultStatus":"S"},"paymentRequestId":"sub2_123","paymentId":"pay_123","paymentAmount":{"currency":"USD","value":"1234"},"paymentStatus":"SUCCESS","paymentMethodType":"CARD"}`), nil
	})
	raw := `{"notifyType":"CAPTURE_RESULT","paymentId":"pay_123","captureAmount":{"currency":"USD","value":"1234"},"result":{"resultCode":"SUCCESS","resultStatus":"S"}}`
	result, err := prov.VerifyNotification(context.Background(), raw, antomTestHeaders(t, key, raw))
	require.NoError(t, err)
	require.Equal(t, "sub2_123", result.OrderID)
	require.Equal(t, payment.NotificationStatusSuccess, result.Status)
	partial := strings.ReplaceAll(raw, "1234", "500")
	_, err = prov.VerifyNotification(context.Background(), partial, antomTestHeaders(t, key, partial))
	require.Error(t, err)
}

func TestAntomRefundUnknownRetainsIdempotencyAndInquiryUsesRefundStatus(t *testing.T) {
	t.Parallel()
	prov, key := antomTestProvider(t)
	ids := []string{}
	refundStatus := "PROCESSING"
	prov.client.Transport = antomTestTransport(func(r *http.Request) (*http.Response, error) {
		var request map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		switch {
		case strings.HasSuffix(r.URL.Path, "/inquiryPayment"):
			return antomTestResponse(t, key, r.URL.Path, `{"result":{"resultCode":"SUCCESS","resultStatus":"S"},"paymentRequestId":"sub2_123","paymentId":"pay_123","paymentAmount":{"currency":"USD","value":"1234"}}`), nil
		case strings.HasSuffix(r.URL.Path, "/refund"):
			require.Equal(t, "pay_123", request["paymentId"])
			ids = append(ids, request["refundRequestId"].(string))
			return antomTestResponse(t, key, r.URL.Path, `{"result":{"resultCode":"REFUND_IN_PROCESS","resultStatus":"U"}}`), nil
		default:
			require.Equal(t, ids[0], request["refundRequestId"])
			return antomTestResponse(t, key, r.URL.Path, `{"result":{"resultCode":"SUCCESS","resultStatus":"S"},"refundStatus":"`+refundStatus+`","refundAmount":{"currency":"USD","value":"1234"}}`), nil
		}
	})
	req := payment.RefundRequest{TradeNo: "sub2_123", OrderID: "sub2_123", Amount: "12.34"}
	first, err := prov.Refund(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPending, first.Status)
	second, err := prov.Refund(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, first.RefundID, second.RefundID)
	query := payment.RefundQueryRequest{RefundID: first.RefundID, Amount: "12.34"}
	pending, err := prov.QueryRefund(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPending, pending.Status)
	refundStatus = "FAIL"
	failed, err := prov.QueryRefund(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusFailed, failed.Status)
	refundStatus = "SUCCESS"
	final, err := prov.QueryRefund(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, final.Status)
}

func TestAntomRefundCanRetryAfterConfirmedFailure(t *testing.T) {
	t.Parallel()
	prov, key := antomTestProvider(t)
	funded := false
	failedIDs := map[string]bool{}
	prov.client.Transport = antomTestTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/inquiryPayment") {
			return antomTestResponse(t, key, r.URL.Path, `{"result":{"resultCode":"SUCCESS","resultStatus":"S"},"paymentRequestId":"sub2_123","paymentId":"pay_123","paymentAmount":{"currency":"USD","value":"1234"}}`), nil
		}
		var request map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		id := request["refundRequestId"].(string)
		if !funded || failedIDs[id] {
			failedIDs[id] = true
			return antomTestResponse(t, key, r.URL.Path, `{"result":{"resultCode":"MERCHANT_BALANCE_NOT_ENOUGH","resultStatus":"F"}}`), nil
		}
		return antomTestResponse(t, key, r.URL.Path, `{"result":{"resultCode":"SUCCESS","resultStatus":"S"}}`), nil
	})
	req := payment.RefundRequest{TradeNo: "sub2_123", Amount: "12.34"}
	_, err := prov.Refund(context.Background(), req)
	require.Error(t, err)
	funded = true
	req.AttemptID = "confirmed-failure-7"
	retried, err := prov.Refund(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, retried.Status)
}

func TestAntomCancelRequiresConfirmedSuccess(t *testing.T) {
	t.Parallel()
	prov, key := antomTestProvider(t)
	status := "S"
	prov.client.Transport = antomTestTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/ams/api/v1/payments/cancel", r.URL.Path)
		var payload map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "sub2_123", payload["paymentRequestId"])
		return antomTestResponse(t, key, r.URL.Path, `{"result":{"resultCode":"SUCCESS","resultStatus":"`+status+`"}}`), nil
	})
	require.NoError(t, prov.CancelPayment(context.Background(), "sub2_123"))
	status = "U"
	require.Error(t, prov.CancelPayment(context.Background(), "sub2_123"))
}

func TestAntomMobileBrowserEnvironment(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, ua, terminal, os string }{
		{"ios", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) Mobile", "WAP", "IOS"},
		{"unknown mobile browser", "CustomBrowser Mobile", "WEB", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prov, key := antomTestProvider(t)
			prov.client.Transport = antomTestTransport(func(r *http.Request) (*http.Response, error) {
				var payload struct{ Env map[string]string }
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				require.Equal(t, tc.terminal, payload.Env["terminalType"])
				require.Equal(t, tc.os, payload.Env["osType"])
				return antomTestResponse(t, key, r.URL.Path, `{"result":{"resultCode":"SUCCESS","resultStatus":"S"},"normalUrl":"https://checkout.antom.com/session"}`), nil
			})
			_, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "mobile-order", BuyerID: "42", Amount: "12.34", ReturnURL: "https://merchant.example/result", IsMobile: true, UserAgent: tc.ua})
			require.NoError(t, err)
		})
	}
}
