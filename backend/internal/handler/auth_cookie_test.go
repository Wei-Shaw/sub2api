//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetAccessTokenCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	setAccessTokenCookie(c, "access-token-value", 3600)

	var found *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == accessTokenCookieName {
			found = cookie
			break
		}
	}
	require.NotNil(t, found)
	require.Equal(t, "/", found.Path)
	require.Equal(t, 3600, found.MaxAge)
	require.True(t, found.HttpOnly)
	require.NotEmpty(t, found.Value)
}
