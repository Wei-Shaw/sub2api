package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyJSHookHeadersToWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	h := rec.Header()
	h.Set("Content-Type", "application/json")
	applyJSHookHeadersToWriter(h, http.Header{"X-Custom": []string{"a"}}, []string{"X-Remove"})
	h.Set("X-Remove", "gone")
	applyJSHookHeadersToWriter(h, nil, []string{"X-Remove"})
	require.Equal(t, "a", rec.Header().Get("X-Custom"))
	require.Empty(t, rec.Header().Get("X-Remove"))
}

func TestApplyJSHookHeadersToGinRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Anthropic-Version", "2023-06-01")
	ApplyJSHookHeadersToGinRequest(c, http.Header{"X-JS": []string{"1"}}, nil)
	require.Equal(t, "1", c.Request.Header.Get("X-JS"))
}