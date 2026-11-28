//go:build unit

package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTestWebSearchEmulationReturnsFailureCause(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.SetWebSearchManager(nil)
	t.Cleanup(func() { service.SetWebSearchManager(nil) })

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/web-search-emulation/test", bytes.NewBufferString(`{"query":"golang"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	(&SettingHandler{}).TestWebSearchEmulation(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var body struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body.Message, "Web search test failed")
	require.Contains(t, body.Message, "save config first", "the admin must see why the test failed, not a generic internal error")
}
