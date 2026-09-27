//go:build unit

package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testStepUpHeader = "X-Test-Step-Up"

// fakePaymentStepUp stands in for the real step-up gate (covered in middleware/step_up_test.go):
// it answers like enforceStepUp does for a session without a grant, unless the test marks the request as verified.
func fakePaymentStepUp(c *gin.Context) {
	if c.GetHeader(testStepUpHeader) != "granted" {
		servermiddleware.AbortWithError(c, http.StatusForbidden, "STEP_UP_REQUIRED", "This operation requires recent two-factor verification")
		return
	}
	c.Next()
}

func TestAdminPaymentConfigWritesRequireStepUp(t *testing.T) {
	router, _ := newPaymentRoutesTestRouter(t, `{"enabled":false}`)

	serve := func(method, path string, granted bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		if granted {
			req.Header.Set(testStepUpHeader, "granted")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	writes := []struct{ method, path string }{
		{http.MethodPut, "/api/v1/admin/payment/config"},
		{http.MethodPost, "/api/v1/admin/payment/providers"},
		{http.MethodPut, "/api/v1/admin/payment/providers/1"},
		{http.MethodDelete, "/api/v1/admin/payment/providers/1"},
	}
	for _, rt := range writes {
		rec := serve(rt.method, rt.path, false)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s without step-up", rt.method, rt.path)
		require.Contains(t, rec.Body.String(), "STEP_UP_REQUIRED", "%s %s without step-up", rt.method, rt.path)

		// With a grant the request reaches the (zero-value) handler: it fails binding (400) or panics (500), but is not gated.
		rec = serve(rt.method, rt.path, true)
		require.Contains(t, []int{http.StatusBadRequest, http.StatusInternalServerError}, rec.Code, "%s %s with step-up", rt.method, rt.path)
		require.NotContains(t, rec.Body.String(), "STEP_UP", "%s %s with step-up", rt.method, rt.path)
	}

	// Reads stay ungated so the settings page loads without a TOTP prompt.
	for _, path := range []string{"/api/v1/admin/payment/config", "/api/v1/admin/payment/providers"} {
		rec := serve(http.MethodGet, path, false)
		require.NotEqual(t, http.StatusForbidden, rec.Code, "GET %s", path)
		require.NotContains(t, rec.Body.String(), "STEP_UP", "GET %s", path)
	}
}
