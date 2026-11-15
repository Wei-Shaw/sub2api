//go:build unit

package admin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type webImageAccountTestRepo struct {
	service.AccountRepository
	account                                             *service.Account
	clearError, clearQuota, clearModels, clearTemporary int
}

func (r *webImageAccountTestRepo) GetByID(context.Context, int64) (*service.Account, error) {
	return r.account, nil
}
func (r *webImageAccountTestRepo) ClearError(context.Context, int64) error {
	r.clearError++
	return nil
}
func (r *webImageAccountTestRepo) ClearRateLimit(context.Context, int64) error {
	r.clearQuota++
	return nil
}
func (r *webImageAccountTestRepo) ClearModelRateLimits(context.Context, int64) error {
	r.clearModels++
	return nil
}
func (r *webImageAccountTestRepo) ClearTempUnschedulable(context.Context, int64) error {
	r.clearTemporary++
	return nil
}
func (r *webImageAccountTestRepo) ClearAntigravityQuotaScopes(context.Context, int64) error {
	r.clearQuota++
	return nil
}

func TestAccountHandlerWebImageSuccessPreservesPausedCodexAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	t.Setenv("SUB2API_WEB_IMAGE_STATE_DIR", dir)
	reset := time.Now().Add(time.Hour)
	a := &service.Account{ID: 78, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusError, ErrorMessage: "Codex 429", Schedulable: false, RateLimitResetAt: &reset,
		Credentials: map[string]any{"plan_type": "free", "access_token": "unused", "model_mapping": map[string]any{"paint": "gpt-image-2"}},
		Extra:       map[string]any{service.AccountExtraChatGPTWebImageEnabled: true, service.AccountExtraChatGPTWebImageRegularChat: true}}
	repo := &webImageAccountTestRepo{account: a}
	// Seed a completed job: exercise the real handler/service path without making
	// an upstream request or introducing a mutable production test hook.
	scope, err := json.Marshal([]any{"admin-web-image-test-v1", "admin:0", "click-a", int64(78), "paint", "cat"})
	require.NoError(t, err)
	digest := sha256.Sum256(scope)
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	body, err := json.Marshal(map[string]any{"state": "complete", "billing_id": "web-image:cached-admin-test", "result": chatgptwebimage.Result{Images: []chatgptwebimage.Image{{Bytes: pngBytes.Bytes(), Format: "png"}}}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, hex.EncodeToString(digest[:])+".json"), body, 0600))
	svc := service.NewAccountTestService(repo, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetOpenAIGatewayService(&service.OpenAIGatewayService{})
	h := &AccountHandler{accountTestService: svc, rateLimitService: service.NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
	r := gin.New()
	r.POST("/api/v1/admin/accounts/:id/test", h.Test)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/78/test", strings.NewReader(`{"model_id":"paint","prompt":"cat","operation_id":"click-a","capability_only":false}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Account-Test-Capability-Only", "false")
		r.ServeHTTP(rec, req)
		require.Contains(t, rec.Body.String(), `"success":true`)
		require.Contains(t, rec.Body.String(), `"provider":"chatgpt_web"`)
	}
	require.Zero(t, repo.clearError)
	require.Zero(t, repo.clearQuota)
	require.Zero(t, repo.clearModels)
	require.Zero(t, repo.clearTemporary)
	require.Equal(t, service.StatusError, a.Status)
	require.False(t, a.Schedulable)
	require.Equal(t, &reset, a.RateLimitResetAt)
}
