//go:build unit

package routes

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type deepseekUploadSlots struct {
	service.ConcurrencyCache
	acquired, released int
	deny               bool
}

func (s *deepseekUploadSlots) AcquireUserSlot(context.Context, int64, int, string) (bool, error) {
	if s.deny {
		return false, io.ErrClosedPipe
	}
	s.acquired++
	return true, nil
}

func (s *deepseekUploadSlots) ReleaseUserSlot(context.Context, int64, string) error {
	s.released++
	return nil
}

func newDeepseekUploadTestRouter(t *testing.T, cfg *config.Config, group *service.Group, slots *deepseekUploadSlots) *gin.Engine {
	t.Helper()
	cfg.RunMode = config.RunModeSimple
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	router := gin.New()
	h := &handler.Handlers{
		Gateway:       &handler.GatewayHandler{},
		OpenAIGateway: handler.NewOpenAIGatewayHandler(&service.OpenAIGatewayService{}, service.NewConcurrencyService(slots), billing, &service.APIKeyService{}, nil, nil, nil, nil, cfg),
		AsyncImage:    handler.NewAsyncImageHandler(nil, nil),
	}
	RegisterGatewayRoutes(router, h, func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &group.ID, Group: group, UserID: 7, User: &service.User{ID: 7}})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7, Concurrency: 1})
		c.Next()
	}, nil, nil, nil, nil, nil, cfg)
	return router
}

func TestDeepseekToolsLimitsBeforeCompositePreread(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxBodySize: 2048, TextMaxBodySize: 32}}
	router := newDeepseekUploadTestRouter(t, cfg, &service.Group{ID: 1, Platform: service.PlatformComposite}, &deepseekUploadSlots{})
	for _, path := range []string{"/v1/tools/search", "/v1/tools/fetch", "/v1/tools/tts", "/v1/files"} {
		bodySize := 33
		if path == "/v1/files" {
			bodySize = 2049
		}
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.Repeat("x", bodySize)))
		req.ContentLength = -1
		req.Header.Set("anthropic-beta", "files-api-2025-04-14")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, 413, w.Code, path)
		require.Equal(t, path == "/v1/files", gjson.GetBytes(w.Body.Bytes(), "type").String() == "error")
		if path == "/v1/files" {
			require.Equal(t, "request_too_large", gjson.GetBytes(w.Body.Bytes(), "error.type").String())
		}
	}
}

type admittedUploadReader struct {
	t     *testing.T
	slots *deepseekUploadSlots
	io.Reader
}

func (r admittedUploadReader) Read(p []byte) (int, error) {
	r.t.Helper()
	require.Equal(r.t, 1, r.slots.acquired-r.slots.released, "every body read must hold the user slot")
	return r.Reader.Read(p)
}

func TestDeepseekUploadAdmissionBeforeBodyRead(t *testing.T) {
	var form bytes.Buffer
	w := multipart.NewWriter(&form)
	require.NoError(t, w.WriteField("purpose", "user_data"))
	require.NoError(t, w.WriteField("purpose", "text_recognition"))
	require.NoError(t, w.Close())
	for _, tc := range []struct {
		path, platform  string
		allowlist, beta bool
	}{
		{"/v1/files", service.PlatformDeepseek, false, true},
		{"/v1/files", service.PlatformComposite, true, false},
		{"/v1/tools/ocr", service.PlatformDeepseek, true, false},
		{"/v1/tools/asr", service.PlatformComposite, false, false},
	} {
		t.Run(tc.path+"/"+tc.platform, func(t *testing.T) {
			cfg := &config.Config{Gateway: config.GatewayConfig{MaxBodySize: 1 << 20, TextMaxBodySize: 1024}}
			slots := &deepseekUploadSlots{}
			router := newDeepseekUploadTestRouter(t, cfg, allowlistGroup(tc.platform, tc.allowlist, "model"), slots)
			send := func(body []byte) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodPost, tc.path, admittedUploadReader{t, slots, bytes.NewReader(body)})
				req.Header.Set("Content-Type", w.FormDataContentType())
				if tc.beta {
					req.Header.Set("anthropic-beta", "files-api-2025-04-14")
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return rec
			}
			slots.deny = true
			require.Equal(t, http.StatusServiceUnavailable, send(form.Bytes()).Code)
			slots.deny = false
			rec := send(form.Bytes())
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "purpose")
			require.Equal(t, tc.beta, gjson.GetBytes(rec.Body.Bytes(), "type").String() == "error")
			require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
			require.Equal(t, 1, slots.acquired)
			require.Equal(t, 1, slots.released)

			// An invalid part header passes duplicate validation and reaches the upload parser.
			rec = send([]byte("--" + w.Boundary() + "\r\ninvalid header\r\n\r\n"))
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "Invalid multipart file upload")
			require.Equal(t, 2, slots.acquired)
			require.Equal(t, 2, slots.released)
		})
	}
}
