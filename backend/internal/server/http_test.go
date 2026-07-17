//go:build unit

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func clientIPForTrustedProxyTest(t *testing.T, r *gin.Engine) string {
	t.Helper()

	var clientIP string
	r.GET("/", func(c *gin.Context) {
		clientIP = c.ClientIP()
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "10.0.0.2:1234"
	request.Header.Set("X-Forwarded-For", "9.9.9.9")
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	return clientIP
}

func TestConfigureTrustedProxiesDisablesForwardedHeadersWhenEmpty(t *testing.T) {
	r := gin.New()

	assert.False(t, configureTrustedProxies(r, nil))
	assert.Equal(t, "10.0.0.2", clientIPForTrustedProxyTest(t, r))
}

func TestConfigureTrustedProxiesTrustsForwardedHeadersForConfiguredPeer(t *testing.T) {
	r := gin.New()

	assert.True(t, configureTrustedProxies(r, []string{"10.0.0.0/8"}))
	assert.Equal(t, "9.9.9.9", clientIPForTrustedProxyTest(t, r))
}

func TestConfigureTrustedProxiesFallsBackWhenConfigurationIsInvalid(t *testing.T) {
	r := gin.New()

	assert.False(t, configureTrustedProxies(r, []string{"not-a-cidr"}))
	assert.Equal(t, "10.0.0.2", clientIPForTrustedProxyTest(t, r))
}
