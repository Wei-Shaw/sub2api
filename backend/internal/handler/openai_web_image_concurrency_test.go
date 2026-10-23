//go:build unit

package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type webImageConcurrencyPlanner struct {
	service.HTTPUpstream
	calls atomic.Int32
}

func (p *webImageConcurrencyPlanner) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	p.calls.Add(1)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(`data: {"type":"response.completed","response":{"id":"resp_test","object":"response","status":"completed","model":"gpt-5.4","output":[{"id":"fc_web","call_id":"call_web","type":"function_call","name":"sub2api_web_image","arguments":"{\"prompt\":\"draw a cat\"}"}],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n")),
	}, nil
}

func webImageConcurrencyHandler(t *testing.T, web bool) (*OpenAIGatewayHandler, *webImageConcurrencyPlanner) {
	t.Helper()
	account := service.Account{
		ID: 1, Name: "image-concurrency-test", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"access_token": "test-token", "plan_type": "pro"},
	}
	if web {
		account.Credentials["plan_type"] = "free"
		account.Extra = map[string]any{service.AccountExtraChatGPTWebImageEnabled: true, service.AccountExtraChatGPTWebImageRegularChat: true}
		account.Credentials["model_mapping"] = map[string]any{"gpt-5.4": "gpt-5.4", "gpt-image-2": "gpt-image-2"}
	}
	cfg := &config.Config{RunMode: config.RunModeSimple, Gateway: config.GatewayConfig{ImageConcurrency: config.ImageConcurrencyConfig{
		Enabled: true, MaxConcurrentRequests: 1, OverflowMode: config.ImageConcurrencyOverflowModeReject,
	}}}
	planner := &webImageConcurrencyPlanner{}
	gateway := service.NewOpenAIGatewayService(
		openAIImagesFailoverAccountRepo{accounts: []service.Account{account}}, nil, nil, nil, nil, nil, nil,
		cfg, nil, nil, nil, nil, nil, planner, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
	return h, planner
}

func webImageConcurrencyRequest(path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	groupID := int64(1)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID: 10, GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		User: &service.User{ID: 20},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 20, Concurrency: 0})
	return c, rec
}

func cacheWebImageHandlerResult(t *testing.T, dir, path, body string) {
	t.Helper()
	var request any
	require.NoError(t, json.Unmarshal([]byte(body), &request))
	canonical, err := json.Marshal(request)
	require.NoError(t, err)
	identity := fmt.Sprintf("v1\x00%d\x00%s\x00%s\x00", 10, "", path)
	key := fmt.Sprintf("%x", sha256.Sum256(append([]byte(identity), canonical...)))
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	record, err := json.Marshal(map[string]any{
		"state": "complete", "billing_id": "web-image:00000000-0000-4000-8000-000000000001", "billed": true,
		"result": chatgptwebimage.Result{Images: []chatgptwebimage.Image{{Bytes: encoded.Bytes(), Format: "png", Width: 2, Height: 2}}},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, key+".json"), record, 0600))
}

func TestOpenAIGatewayHandlerSelectedImageAccountUsesConcurrencyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/v1/responses", "/v1/images/generations"} {
		t.Run(path, func(t *testing.T) {
			h, planner := webImageConcurrencyHandler(t, false)
			body := `{"model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation"}]}`
			if path == "/v1/images/generations" {
				body = `{"model":"gpt-image-2","prompt":"draw a cat"}`
			}
			c, rec := webImageConcurrencyRequest(path, body)
			release, acquired := h.tryAcquireImageGenerationSlot(context.Background())
			require.True(t, acquired)
			defer release()
			if path == "/v1/responses" {
				h.Responses(c)
			} else {
				h.Images(c)
			}
			require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
			require.Equal(t, "gateway_concurrency_limit", gjson.GetBytes(rec.Body.Bytes(), "error.code").String())
			require.Zero(t, planner.calls.Load(), "a full image limiter must prevent upstream generation")
		})
	}
}

func TestOpenAIGatewayHandlerWebImageReplayWorksWhenImageConcurrencyFull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/v1/responses", "/v1/images/generations"} {
		t.Run(path, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("SUB2API_WEB_IMAGE_STATE_DIR", dir)
			h, planner := webImageConcurrencyHandler(t, true)
			body := `{"model":"gpt-5.4","stream":false,"input":"draw a cat","tools":[{"type":"image_generation","model":"gpt-image-2"}],"tool_choice":{"type":"image_generation"}}`
			if path == "/v1/images/generations" {
				body = `{"model":"gpt-image-2","prompt":"draw a cat"}`
			}
			cacheWebImageHandlerResult(t, dir, path, body)
			c, rec := webImageConcurrencyRequest(path, body)
			release, acquired := h.tryAcquireImageGenerationSlot(context.Background())
			require.True(t, acquired)
			defer release()
			if path == "/v1/responses" {
				h.Responses(c)
				require.Equal(t, int32(1), planner.calls.Load())
				require.Contains(t, rec.Body.String(), "image_generation_call")
			} else {
				h.Images(c)
				require.Zero(t, planner.calls.Load())
				require.True(t, gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").Exists(), rec.Body.String())
			}
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.NotContains(t, rec.Body.String(), "gateway_concurrency_limit")
			h.imageLimiter.mu.Lock()
			active := h.imageLimiter.active
			h.imageLimiter.mu.Unlock()
			require.Equal(t, 1, active, "replay must not touch the unrelated active generation slot")
		})
	}
}

func TestOpenAIGatewayHandlerNaturalLanguageWebImageUsesSharedConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_WEB_IMAGE_STATE_DIR", t.TempDir())
	h, planner := webImageConcurrencyHandler(t, true)
	c, rec := webImageConcurrencyRequest("/v1/responses", `{"model":"gpt-5.4","stream":true,"input":"draw a cat"}`)
	release, acquired := h.tryAcquireImageGenerationSlot(context.Background())
	require.True(t, acquired)
	defer release()
	h.Responses(c)
	require.Equal(t, int32(1), planner.calls.Load(), "ordinary text must reach the planner before acquiring image capacity")
	require.Contains(t, rec.Body.String(), "gateway_concurrency_limit")
	require.Contains(t, rec.Body.String(), "event: response.failed", "a started Responses stream must receive an SSE error")
}

type webImageAccountOrderCache struct {
	helperConcurrencyCacheStub
	orderMu     sync.Mutex
	imageActive func() int
	observed    []int
}

func (c *webImageAccountOrderCache) AcquireAccountSlot(_ context.Context, _ int64, _ int, _ string) (bool, error) {
	c.orderMu.Lock()
	defer c.orderMu.Unlock()
	c.observed = append(c.observed, c.imageActive())
	return true, nil
}

func TestOpenAIGatewayHandlerImageFailoverReleasesImageSlotBeforeNextAccountAcquire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/v1/responses", "/v1/images/generations"} {
		t.Run(path, func(t *testing.T) {
			cache := &webImageAccountOrderCache{}
			concurrency := service.NewConcurrencyService(cache)
			cfg := &config.Config{RunMode: config.RunModeSimple, Gateway: config.GatewayConfig{ImageConcurrency: config.ImageConcurrencyConfig{
				Enabled: true, MaxConcurrentRequests: 1, OverflowMode: config.ImageConcurrencyOverflowModeReject,
			}}}
			accounts := []service.Account{
				{ID: 1, Name: "first", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"access_token": "test-first"}},
				{ID: 2, Name: "second", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Credentials: map[string]any{"access_token": "test-second"}},
			}
			upstream := &openAIResponsesFailoverCancelUpstream{}
			gateway := service.NewOpenAIGatewayService(openAIImagesFailoverAccountRepo{accounts: accounts}, nil, nil, nil, nil, nil, nil,
				cfg, nil, concurrency, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
			billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billing.Stop)
			h := NewOpenAIGatewayHandler(gateway, concurrency, billing,
				service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
			h.maxAccountSwitches = 2
			cache.imageActive = func() int {
				h.imageLimiter.mu.Lock()
				defer h.imageLimiter.mu.Unlock()
				return h.imageLimiter.active
			}
			upstream.onFirstDo = func() { require.Equal(t, 1, cache.imageActive(), "upstream image request must hold its slot") }
			body := `{"model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation"}]}`
			if path == "/v1/images/generations" {
				body = `{"model":"gpt-image-2","prompt":"draw a cat"}`
			}
			c, _ := webImageConcurrencyRequest(path, body)
			if path == "/v1/responses" {
				h.Responses(c)
			} else {
				h.Images(c)
			}
			require.GreaterOrEqual(t, len(upstream.calls()), 2, "test must exercise a real account failover")
			cache.orderMu.Lock()
			observed := append([]int(nil), cache.observed...)
			cache.orderMu.Unlock()
			require.GreaterOrEqual(t, len(observed), 2)
			for _, active := range observed {
				require.Zero(t, active, "holding an image slot while waiting for an account can deadlock another image request")
			}
			require.Zero(t, cache.imageActive())
		})
	}
}
