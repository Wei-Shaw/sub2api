//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestA206OpenAIImagesJSONKeepalive_DisabledByContextIsNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx := WithOpenAIImagesJSONKeepaliveDisabled(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil).WithContext(ctx)
	originalWriter := c.Writer

	stop := StartOpenAIImagesJSONKeepalive(c, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	require.False(t, c.Writer.Written(), "no heartbeat may commit the status")
	require.False(t, OpenAIImagesJSONKeepalivePresent(c))
	c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": "upstream failed"}})
	stop()

	require.Same(t, originalWriter, c.Writer)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Equal(t, "upstream failed", gjson.Get(rec.Body.String(), "error.message").String())
}
