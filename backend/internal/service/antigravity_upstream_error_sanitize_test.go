//go:build unit

package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildAntigravityClientErrorBody_ScrubsPoolIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := []byte(`{"error":{"code":403,"message":"Permission denied on resource project projects/123456789 for consumer: projects/123456789; caller pool-sa@my-gcp-proj.iam.gserviceaccount.com","status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","metadata":{"consumer":"projects/123456789","service":"cloudcode-pa.googleapis.com"}}]}}`)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Data(http.StatusForbidden, "application/json", buildAntigravityClientErrorBody(http.StatusForbidden, upstream))

	require.Equal(t, http.StatusForbidden, rec.Code)
	out := rec.Body.String()
	require.NotContains(t, out, "123456789")
	require.NotContains(t, out, "pool-sa@")
	require.NotContains(t, out, "gserviceaccount.com")
	require.NotContains(t, out, "details")

	var parsed struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Equal(t, 403, parsed.Error.Code)
	require.Equal(t, "PERMISSION_DENIED", parsed.Error.Status)
	require.True(t, strings.Contains(parsed.Error.Message, "Permission denied"))
}

func TestBuildAntigravityClientErrorBody_NonJSONBody(t *testing.T) {
	out := string(buildAntigravityClientErrorBody(http.StatusTooManyRequests, []byte("quota exceeded for consumer 987654321 sa@x.iam.gserviceaccount.com")))
	require.NotContains(t, out, "987654321")
	require.NotContains(t, out, "gserviceaccount")
	require.Contains(t, out, `"status":"RESOURCE_EXHAUSTED"`)
	require.Contains(t, out, `"code":429`)
}
