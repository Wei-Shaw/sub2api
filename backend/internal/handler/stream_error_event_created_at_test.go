package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWriteResponsesFailedSSE_IncludesPositiveCreatedAt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	require.True(t, writeResponsesFailedSSE(ctx, "upstream_error", "failed"))
	require.Positive(t, gjson.Get(recorder.Body.String(), "response.created_at").Int())
}
