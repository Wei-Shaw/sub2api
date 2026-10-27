//go:build unit

package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Run the real Images handler, scheduler, parser and forwarding service over
// HTTP with fixture authentication, an account repository and a local upstream.
type nativeImageRouteUpstream struct {
	service.HTTPUpstream
	target *url.URL
	client *http.Client
}

func (u *nativeImageRouteUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	forwarded := req.Clone(req.Context())
	forwarded.URL.Scheme = u.target.Scheme
	forwarded.URL.Host = u.target.Host
	return u.client.Do(forwarded)
}

func newNativeImageRoute(t *testing.T, delay time.Duration, status int) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, http.MethodPost, r.Method)
		switch r.URL.Path {
		case "/v1/images/generations":
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.Equal(t, "gpt-image-1", gjson.GetBytes(body, "model").String())
		case "/v1/images/edits":
			require.NoError(t, r.ParseMultipartForm(1<<20))
			defer func() {
				if r.MultipartForm != nil {
					_ = r.MultipartForm.RemoveAll()
				}
			}()
			require.Equal(t, "gpt-image-1", r.FormValue("model"))
			part, _, err := r.FormFile("image")
			require.NoError(t, err)
			_ = part.Close()
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
		}
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = io.WriteString(w, `{"created":1710000000,"data":[{"b64_json":"aW1hZ2U="}]}`)
		} else {
			_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","code":"native_route_rejected","message":"delayed image rejection"}}`)
		}
	}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.ImageNonstreamKeepaliveInterval = 1
	repo := openAIImagesFailoverAccountRepo{accounts: []service.Account{{ID: 1, Name: "fixture", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://fixture.example/v1"}}}}
	transport := &nativeImageRouteUpstream{target: target, client: upstream.Client()}
	deferred := service.NewDeferredService(repo, nil, time.Minute)
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil, service.NewBillingService(cfg, nil), nil, nil, transport, deferred, nil, nil, nil, nil, nil, nil, nil)
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	handler := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, GroupID: &groupID, Group: &service.Group{ID: 1, AllowImageGeneration: true}, User: &service.User{ID: 1}})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
	})
	router.POST("/v1/images/generations", handler.Images)
	router.POST("/v1/images/edits", handler.Images)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, &calls
}

func nativeImageRequest(t *testing.T, endpoint string, native bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	contentType := "application/json"
	if strings.HasSuffix(endpoint, "/edits") {
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("model", "gpt-image-1"))
		require.NoError(t, writer.WriteField("prompt", "edit fixture"))
		part, err := writer.CreateFormFile("image", "input.png")
		require.NoError(t, err)
		_, err = part.Write([]byte("fixture-image"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		contentType = writer.FormDataContentType()
	} else {
		_, _ = body.WriteString(`{"model":"gpt-image-1","prompt":"fixture"}`)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	if native {
		req.Header.Set("X-Codex-Image-Turn-Id", "fixture-turn")
	}
	return req
}

func TestNativeImagesRoutesDelayedResponse(t *testing.T) {
	for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
		for _, status := range []int{http.StatusBadRequest, http.StatusOK} {
			t.Run(fmt.Sprintf("%s/%d", path, status), func(t *testing.T) {
				gateway, calls := newNativeImageRoute(t, 1200*time.Millisecond, status)
				target, err := url.Parse(gateway.URL)
				require.NoError(t, err)
				proxy := httputil.NewSingleHostReverseProxy(target)
				baseTransport, ok := http.DefaultTransport.(*http.Transport)
				require.True(t, ok)
				transport := baseTransport.Clone()
				transport.ResponseHeaderTimeout = 3 * time.Second
				defer transport.CloseIdleConnections()
				proxy.Transport = transport
				downstream := httptest.NewServer(proxy)
				defer downstream.Close()
				started := time.Now()
				resp, err := downstream.Client().Do(nativeImageRequest(t, downstream.URL+path, true))
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()
				require.GreaterOrEqual(t, time.Since(started), 1200*time.Millisecond, "no heartbeat may commit early headers")
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, status, resp.StatusCode, string(body))
				require.EqualValues(t, 1, calls.Load())
				require.True(t, gjson.ValidBytes(body), string(body))
				require.NotContains(t, string(body), "event:")
				if status == http.StatusOK {
					require.EqualValues(t, 1710000000, gjson.GetBytes(body, "created").Int())
					require.Equal(t, "aW1hZ2U=", gjson.GetBytes(body, "data.0.b64_json").String())
				} else {
					require.Equal(t, "native_route_rejected", gjson.GetBytes(body, "error.code").String())
					require.Equal(t, "delayed image rejection", gjson.GetBytes(body, "error.message").String())
				}
			})
		}
	}
}

// This is a real HTTP proxy header deadline. Native clients intentionally get
// no heartbeat: a deadline below image latency fails, while a sufficiently long
// deadline is covered above. Ordinary clients retain their heartbeat behavior.
func TestNativeImagesRoutesProxyTimeout(t *testing.T) {
	for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
		for _, native := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/native=%t", path, native), func(t *testing.T) {
				gateway, calls := newNativeImageRoute(t, 1600*time.Millisecond, http.StatusOK)
				target, err := url.Parse(gateway.URL)
				require.NoError(t, err)
				proxy := httputil.NewSingleHostReverseProxy(target)
				baseTransport, ok := http.DefaultTransport.(*http.Transport)
				require.True(t, ok)
				transport := baseTransport.Clone()
				transport.ResponseHeaderTimeout = 1300 * time.Millisecond
				defer transport.CloseIdleConnections()
				proxy.Transport = transport
				proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) { w.WriteHeader(http.StatusGatewayTimeout) }
				downstream := httptest.NewServer(proxy)
				defer downstream.Close()
				resp, err := downstream.Client().Do(nativeImageRequest(t, downstream.URL+path, native))
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				if native {
					require.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)
				} else {
					require.Equal(t, http.StatusOK, resp.StatusCode)
					require.Equal(t, "aW1hZ2U=", gjson.GetBytes(body, "data.0.b64_json").String())
				}
				require.EqualValues(t, 1, calls.Load())
			})
		}
	}
}
