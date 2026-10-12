//go:build unit

package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelVersionRejectsLegacyPricingSave(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &ChannelHandler{}
	r.PUT("/channels/:id", h.Update)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/channels/3", strings.NewReader(`{"model_pricing":[]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), "CHANNEL_MODIFIED")
}

func TestChannelVersionResponsePreservesPrecisionAndTimezone(t *testing.T) {
	stamp := time.Date(2026, 9, 14, 14, 56, 2, 337798000, time.FixedZone("CST", 8*3600))
	response := channelToResponse(&service.Channel{UpdatedAt: stamp})
	parsed, err := time.Parse(time.RFC3339Nano, response.UpdatedAt)
	require.NoError(t, err)
	require.True(t, stamp.Equal(parsed))
}
