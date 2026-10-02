package routes

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// authBrandingRepoStub only answers the reads used by public settings, the
// panel rate-limit config, and a branding-asset cache miss.
type authBrandingRepoStub struct {
	values map[string]string
}

func (s *authBrandingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *authBrandingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *authBrandingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *authBrandingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *authBrandingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *authBrandingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *authBrandingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func TestAuthRoutesPublicBrandingAssetSharesSettingsLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x01, 0x02, 0x03}
	raw := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	svc := service.NewSettingService(&authBrandingRepoStub{values: map[string]string{
		service.SettingKeySiteLogo:               raw,
		service.SettingKeyPanelRateLimitSettings: `{"enabled":true,"user_rpm":0,"heavy_rpm":0,"exempt_admin":true,"public_ip_rpm":1}`,
	}}, &config.Config{})
	public, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Contains(t, public.SiteLogo, "/api/v1/settings/public/logo-")

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	router := gin.New()
	v1 := router.Group("/api/v1")
	RegisterAuthRoutes(
		v1,
		&handler.Handlers{
			Auth:    &handler.AuthHandler{},
			Setting: handler.NewSettingHandler(svc, "test"),
		},
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }),
		servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }),
		rdb,
		svc,
		servermiddleware.NewPanelRateLimiter(rdb, svc),
	)

	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	require.True(t, registered["GET /api/v1/settings/public"])
	require.True(t, registered["GET /api/v1/settings/public/:asset"])
	require.True(t, registered["HEAD /api/v1/settings/public/:asset"])

	// The JSON settings route still wins over the :asset pattern.
	jsonResp := doAuthBrandingRequest(router, http.MethodGet, "/api/v1/settings/public", "203.0.113.9:1000")
	require.Equal(t, http.StatusOK, jsonResp.Code)
	require.Contains(t, jsonResp.Body.String(), public.SiteLogo)

	// Same public IP, same settings-group bucket: the logo request is the second hit.
	limited := doAuthBrandingRequest(router, http.MethodGet, public.SiteLogo, "203.0.113.9:1000")
	require.Equal(t, http.StatusTooManyRequests, limited.Code)

	// A different public IP is counted separately and still receives the bytes.
	assetResp := doAuthBrandingRequest(router, http.MethodGet, public.SiteLogo, "198.51.100.7:1000")
	require.Equal(t, http.StatusOK, assetResp.Code)
	require.Equal(t, "image/png", assetResp.Header().Get("Content-Type"))
	require.Equal(t, "public, max-age=31536000, immutable", assetResp.Header().Get("Cache-Control"))
	require.Equal(t, png, assetResp.Body.Bytes())

	headResp := doAuthBrandingRequest(router, http.MethodHead, public.SiteLogo, "198.51.100.8:1000")
	require.Equal(t, http.StatusOK, headResp.Code)
	require.Empty(t, headResp.Body.Bytes())
	require.Equal(t, "image/png", headResp.Header().Get("Content-Type"))
}

func doAuthBrandingRequest(router http.Handler, method, path, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
