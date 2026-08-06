package handler

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEnsureWeb3BrowserSessionCookiePolicy(t *testing.T) {
	tests := []struct {
		name           string
		sameSite       http.SameSite
		forwardedProto string
		wantSecure     bool
	}{
		{name: "lax http", sameSite: http.SameSiteLaxMode, forwardedProto: "http", wantSecure: false},
		{name: "lax https", sameSite: http.SameSiteLaxMode, forwardedProto: "https", wantSecure: true},
		{name: "none forces secure", sameSite: http.SameSiteNoneMode, forwardedProto: "http", wantSecure: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/web3/login/challenge", nil)
			ctx.Request.Header.Set("X-Forwarded-Proto", tt.forwardedProto)

			_, err := ensureWeb3BrowserSession(ctx, tt.sameSite)
			require.NoError(t, err)
			cookies := recorder.Result().Cookies()
			require.Len(t, cookies, 1)
			require.Equal(t, tt.sameSite, cookies[0].SameSite)
			require.Equal(t, tt.wantSecure, cookies[0].Secure)
			require.True(t, cookies[0].HttpOnly)
			require.Equal(t, web3BrowserCookiePath, cookies[0].Path)
		})
	}
}

func TestResolveWeb3BrowserCookieSameSite(t *testing.T) {
	require.Equal(t, http.SameSiteLaxMode, resolveWeb3BrowserCookieSameSite(nil))
	require.Equal(t, http.SameSiteLaxMode, resolveWeb3BrowserCookieSameSite(&config.Config{}))
	require.Equal(t, http.SameSiteNoneMode, resolveWeb3BrowserCookieSameSite(&config.Config{
		Web3Auth: config.Web3AuthConfig{BrowserCookieSameSite: " NoNe "},
	}))
}

func TestWeb3BrowserSessionCookieRoundTrip(t *testing.T) {
	for _, sameSite := range []string{
		config.Web3BrowserCookieSameSiteLax,
		config.Web3BrowserCookieSameSiteNone,
	} {
		t.Run(sameSite, func(t *testing.T) {
			handler := NewWeb3AuthHandler(&config.Config{
				Web3Auth: config.Web3AuthConfig{BrowserCookieSameSite: sameSite},
			}, nil, nil, nil, nil)
			var issuedSession string
			router := gin.New()
			router.POST("/api/v1/auth/web3/login/challenge", func(ctx *gin.Context) {
				var err error
				issuedSession, err = ensureWeb3BrowserSession(ctx, handler.browserCookieSameSite)
				require.NoError(t, err)
				ctx.Status(http.StatusNoContent)
			})
			router.POST("/api/v1/auth/web3/login/verify", func(ctx *gin.Context) {
				ctx.String(http.StatusOK, readWeb3BrowserSession(ctx))
			})

			server := httptest.NewTLSServer(router)
			defer server.Close()
			jar, err := cookiejar.New(nil)
			require.NoError(t, err)
			client := server.Client()
			client.Jar = jar

			challengeResponse, err := client.Post(
				server.URL+"/api/v1/auth/web3/login/challenge",
				"application/json",
				strings.NewReader("{}"),
			)
			require.NoError(t, err)
			require.NoError(t, challengeResponse.Body.Close())
			require.Equal(t, http.StatusNoContent, challengeResponse.StatusCode)
			require.NotEmpty(t, issuedSession)

			verifyResponse, err := client.Post(
				server.URL+"/api/v1/auth/web3/login/verify",
				"application/json",
				strings.NewReader("{}"),
			)
			require.NoError(t, err)
			defer verifyResponse.Body.Close()
			body, err := io.ReadAll(verifyResponse.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, verifyResponse.StatusCode)
			require.Equal(t, issuedSession, string(body))
		})
	}
}
