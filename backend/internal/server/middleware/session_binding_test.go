//go:build unit

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requestSessionBinding(
	t *testing.T,
	includeIP bool,
	trustedProxies []string,
	remoteAddr string,
	userAgent string,
	forwardedFor string,
) *service.SessionBinding {
	t.Helper()

	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(trustedProxies))
	engine.Use(SessionBindingContext(includeIP))

	var binding *service.SessionBinding
	engine.GET("/", func(c *gin.Context) {
		binding = service.SessionBindingFromContext(c.Request.Context())
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = remoteAddr
	request.Header.Set("User-Agent", userAgent)
	if forwardedFor != "" {
		request.Header.Set("X-Forwarded-For", forwardedFor)
	}

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.NotNil(t, binding)
	return binding
}

func TestSessionBindingContextIncludesIPWhenEnabled(t *testing.T) {
	first := requestSessionBinding(t, true, []string{"10.0.0.0/8"}, "10.0.0.2:1234", "test-agent", "9.9.9.9")
	second := requestSessionBinding(t, true, []string{"10.0.0.0/8"}, "10.0.0.2:1234", "test-agent", "8.8.8.8")

	assert.Equal(t, "9.9.9.9", first.IP)
	assert.Equal(t, "8.8.8.8", second.IP)
	assert.Equal(t, "test-agent", first.UserAgent)
	assert.Equal(t, "test-agent", second.UserAgent)
	assert.Equal(t, (&service.SessionBinding{IP: "9.9.9.9", UserAgent: "test-agent"}).Hash(), first.Hash())
	assert.Equal(t, (&service.SessionBinding{IP: "8.8.8.8", UserAgent: "test-agent"}).Hash(), second.Hash())
	assert.NotEqual(t, first.Hash(), second.Hash())
}

func TestSessionBindingContextIgnoresTrustedProxyPeerChanges(t *testing.T) {
	first := requestSessionBinding(t, true, []string{"10.0.0.0/8"}, "10.0.0.2:1234", "test-agent", "9.9.9.9")
	second := requestSessionBinding(t, true, []string{"10.0.0.0/8"}, "10.0.0.3:1234", "test-agent", "9.9.9.9")

	assert.Equal(t, "9.9.9.9", first.IP)
	assert.Equal(t, first.IP, second.IP)
	assert.Equal(t, first.Hash(), second.Hash())
}

func TestSessionBindingContextOmitsIPWhenDisabled(t *testing.T) {
	first := requestSessionBinding(t, false, nil, "10.0.0.2:1234", "test-agent", "9.9.9.9")
	second := requestSessionBinding(t, false, nil, "10.0.0.3:1234", "test-agent", "8.8.8.8")

	assert.Empty(t, first.IP)
	assert.Empty(t, second.IP)
	assert.Equal(t, "test-agent", first.UserAgent)
	assert.Equal(t, "test-agent", second.UserAgent)
	assert.Equal(t, first.Hash(), second.Hash())
}

func TestSessionBindingContextStillBindsUserAgentWhenIPDisabled(t *testing.T) {
	first := requestSessionBinding(t, false, nil, "10.0.0.2:1234", "first-agent", "9.9.9.9")
	second := requestSessionBinding(t, false, nil, "10.0.0.2:1234", "second-agent", "9.9.9.9")

	assert.NotEqual(t, first.Hash(), second.Hash())
}

func TestCurrentSessionBindingHashUsesRequestContext(t *testing.T) {
	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(nil))
	engine.Use(SessionBindingContext(false))

	var got string
	engine.GET("/", func(c *gin.Context) {
		got = currentSessionBindingHash(c)
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "9.9.9.9:1234"
	request.Header.Set("User-Agent", "Mozilla/5.0")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	want := (&service.SessionBinding{UserAgent: "Mozilla/5.0"}).Hash()
	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Equal(t, want, got)
	assert.NotEqual(t, (&service.SessionBinding{IP: "9.9.9.9", UserAgent: "Mozilla/5.0"}).Hash(), got)
}

func TestCurrentSessionBindingHashAllowsMissingContextBinding(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	assert.Empty(t, currentSessionBindingHash(context))
}
