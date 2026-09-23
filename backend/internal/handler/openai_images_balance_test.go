//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIImagesBalanceExhaustionPreservesMachineReadableCause(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, streamStarted := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
		(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
			StatusCode:       http.StatusBadGateway,
			Reason:           service.OpenAIImagesInsufficientBalanceReason,
			ClientStatusCode: http.StatusPaymentRequired,
			ClientMessage:    service.OpenAIImagesInsufficientBalanceMessage,
			ResponseHeaders:  http.Header{"Retry-After": []string{"23"}, "X-Request-Id": []string{"req-images-balance"}},
		}, streamStarted)
		require.Equal(t, "23", recorder.Header().Get("Retry-After"))

		if streamStarted {
			require.Contains(t, recorder.Body.String(), `"code":"insufficient_balance"`)
		} else {
			require.Equal(t, http.StatusPaymentRequired, recorder.Code)
			require.Equal(t, service.OpenAIImagesInsufficientBalanceCode, gjson.GetBytes(recorder.Body.Bytes(), "error.code").String())
		}
	}
}
