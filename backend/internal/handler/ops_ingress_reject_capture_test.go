package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpsCaptureWriterDoesNotCopyIngressRejectBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	writer := acquireOpsCaptureWriter(context.Writer)
	defer releaseOpsCaptureWriter(writer)
	writer.ctx = context
	context.Writer = writer
	middleware2.MarkIngressRejected(context, middleware2.IngressRejectInvalidAPIKey)
	context.Status(http.StatusUnauthorized)
	_, err := context.Writer.WriteString(`{"code":"INVALID_API_KEY","message":"Invalid API key"}`)
	require.NoError(t, err)
	require.Zero(t, writer.buf.Len())
}
