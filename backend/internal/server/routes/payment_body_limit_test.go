//go:build unit

package routes

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaymentPublicRoutesLimitRequestBodySize(t *testing.T) {
	router, _ := newPaymentRoutesTestRouter(t, `{"enabled":false}`)

	for _, path := range []string{"/api/v1/payment/public/orders/verify", "/api/v1/payment/public/orders/resolve"} {
		// 8MB of whitespace: without a cap the JSON decoder reads all of it; with the cap it stops at 1MB+1.
		body := &countingReader{r: bytes.NewReader(bytes.Repeat([]byte(" "), 8<<20))}
		req := httptest.NewRequest(http.MethodPost, path, body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.30:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code, path)
		require.Contains(t, w.Body.String(), "request body too large", path)
		require.LessOrEqual(t, body.read, int64(1<<20)+1, path)
	}
}

func TestPaymentPublicRoutesAcceptNormalBody(t *testing.T) {
	router, _ := newPaymentRoutesTestRouter(t, `{"enabled":false}`)

	for path, payload := range map[string]string{
		"/api/v1/payment/public/orders/verify":  `{"out_trade_no":"sub2_20260101000000abcdef"}`,
		"/api/v1/payment/public/orders/resolve": `{"resume_token":"token"}`,
	} {
		body := &countingReader{r: bytes.NewReader([]byte(payload))}
		req := httptest.NewRequest(http.MethodPost, path, body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.31:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// The body binds and the handler moves on to its (nil) service, which the test recovery turns into 500.
		require.Equal(t, http.StatusInternalServerError, w.Code, path)
		require.Equal(t, int64(len(payload)), body.read, path)
	}
}
