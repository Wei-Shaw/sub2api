package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type configGuideRoundTripFunc func(*http.Request) (*http.Response, error)

func (f configGuideRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newConfigGuideTestRouter(h *ConfigGuideHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/config-guides")
	omp := g.Group("/omp-openai")
	omp.GET("/manifest.json", h.GetOMPManifest)
	omp.GET("/plugin.txt", h.GetOMPPluginInstructions)
	omp.GET("/models.yml", h.GetOMPModelsYAML)
	omp.GET("/config.yml", h.GetOMPConfigYAML)
	omp.GET("/image-generator.md", h.GetOMPImageGenerator)
	opencode := g.Group("/opencode-openai")
	opencode.GET("/manifest.json", h.GetOpenCodeManifest)
	opencode.GET("/opencode.json", h.GetOpenCodeJSON)
	return r
}

func newConfigGuideTestHandler(t *testing.T, modelsPayload map[string]any, npmVersion string) *ConfigGuideHandler {
	t.Helper()
	originalTransport := http.DefaultTransport
	http.DefaultTransport = configGuideRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://models.dev/api.json":
			return configGuideJSONResponse(http.StatusOK, modelsPayload), nil
		case "https://registry.npmjs.org/omp-openai-provider-tools/latest":
			if strings.TrimSpace(npmVersion) == "" {
				return configGuideTextResponse(http.StatusBadGateway, "boom"), nil
			}
			return configGuideJSONResponse(http.StatusOK, map[string]any{"version": npmVersion}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	return NewConfigGuideHandler(service.NewOpenCodeMetadataService())
}

func configGuideJSONResponse(status int, value any) *http.Response {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(value)
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       ioNopCloser{Reader: bytes.NewReader(buf.Bytes())},
		Request:    &http.Request{},
	}
}

func configGuideTextResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       ioNopCloser{Reader: strings.NewReader(body)},
		Request:    &http.Request{},
	}
}

type ioNopCloser struct {
	Reader interface{ Read([]byte) (int, error) }
}

func (c ioNopCloser) Read(p []byte) (int, error) { return c.Reader.Read(p) }

func (ioNopCloser) Close() error { return nil }

func configGuideModelsDevPayload() map[string]any {
	return map[string]any{
		"openai": map[string]any{
			"models": map[string]any{
				"gpt-5.5": map[string]any{
					"id":                "gpt-5.5",
					"name":              "GPT-5.5",
					"reasoning":         true,
					"attachment":        true,
					"tool_call":         true,
					"structured_output": true,
					"temperature":       false,
					"release_date":      "2026-01-01",
					"modalities":        map[string]any{"input": []any{"text", "image", "pdf"}, "output": []any{"text"}},
					"cost":              map[string]any{"input": 2.5, "output": 15.0, "cache_read": 0.25, "cache_write": 3.75},
					"limit":             map[string]any{"context": 400000, "input": 272000, "output": 128000},
					"experimental": map[string]any{
						"modes": map[string]any{
							"fast": map[string]any{
								"cost": map[string]any{"input": 5.0, "output": 30.0, "cache_read": 0.5},
								"provider": map[string]any{
									"body":    map[string]any{"service_tier": "priority"},
									"headers": map[string]any{"x-test-header": "fast-mode"},
								},
							},
						},
					},
				},
				"gpt-5.4-mini": map[string]any{
					"id":                "gpt-5.4-mini",
					"name":              "GPT-5.4 Mini",
					"reasoning":         true,
					"attachment":        true,
					"tool_call":         true,
					"structured_output": true,
					"temperature":       false,
					"modalities":        map[string]any{"input": []any{"text", "image", "pdf"}, "output": []any{"text"}},
					"cost":              map[string]any{"input": 0.25, "output": 2.0, "cache_read": 0.025},
					"limit":             map[string]any{"context": 400000, "input": 272000, "output": 128000},
				},
			},
		},
	}
}

func TestConfigGuideOMPManifest(t *testing.T) {
	h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
	h.now = func() time.Time { return time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC) }
	r := newConfigGuideTestRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/manifest.json?api_key=sk-test", nil)
	req.Host = "example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", w.Header().Get("Pragma"))

	var manifest configGuideManifest
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &manifest))
	require.Equal(t, 1, manifest.SchemaVersion)
	require.Equal(t, "omp", manifest.Client)
	require.Equal(t, "https://example.com/v1", manifest.BaseURL)
	require.Equal(t, "2026-05-09T00:00:00Z", manifest.GeneratedAt)
	require.Len(t, manifest.Items, 4)
	require.Equal(t, "models", manifest.Items[1].ID)
	require.NotNil(t, manifest.Items[1].TargetPath)
	require.Equal(t, "~/.omp/agent/models.yml", *manifest.Items[1].TargetPath)
	require.Contains(t, manifest.Items[1].URL, "/config-guides/omp-openai/models.yml?api_key=sk-test")
	require.NotContains(t, manifest.Items[1].URL, "base_url=")
	require.Contains(t, manifest.Notes, "Download every item to a local temporary copy before editing existing files; do not transcribe YAML or JSON from chat output.")
	require.Contains(t, manifest.Notes, "If a target file is missing, copy the downloaded file to that path. If it exists, compare both files and merge the smaller side into the larger side when that is safer than replacing.")
	require.Contains(t, manifest.Notes, "After writing files, compare them with the downloaded copies and run the listed plugin doctor/check commands before reporting completion.")
}

func TestConfigGuideOMPManifestWithExplicitBaseURL(t *testing.T) {
	h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
	r := newConfigGuideTestRouter(h)

	w := httptest.NewRecorder()
	target := "/config-guides/omp-openai/manifest.json?api_key=sk-test&base_url=https%3A%2F%2Fapi.example.net%2Fcustom%2F"
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = "example.com"
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var manifest configGuideManifest
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &manifest))
	require.Equal(t, "https://api.example.net/custom", manifest.BaseURL)
	require.Contains(t, manifest.Items[1].URL, "base_url=https%3A%2F%2Fapi.example.net%2Fcustom%2F")

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/models.yml?api_key=sk-test&base_url=https%3A%2F%2Fapi.example.net%2Fcustom%2F", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "baseUrl: https://api.example.net/custom")
}

func TestConfigGuideOMPModelsYAML(t *testing.T) {
	h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "9.9.9")
	r := newConfigGuideTestRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/models.yml?api_key=sk-test", nil)
	req.Host = "example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Contains(t, w.Header().Get("Content-Type"), "application/yaml")
	body := w.Body.String()
	require.Contains(t, body, "omp plugin install npm:omp-openai-provider-tools@9.9.9")
	require.Contains(t, body, "sub2api-openai:")
	require.Contains(t, body, "baseUrl: https://example.com/v1")
	require.Contains(t, body, "apiKey: sk-test")
	require.Contains(t, body, "sub2api-openai-image:")
	require.Contains(t, body, "imageGeneration: true")
	require.Contains(t, body, "sub2api-openai/gpt-5.4-mini-Sys: gpt-5.4-mini-sys")
	require.NotContains(t, body, "pdf")
}

func TestConfigGuideOMPConfigYAML(t *testing.T) {
	h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
	r := newConfigGuideTestRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/config.yml?api_key=sk-test", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "application/yaml")
	body := w.Body.String()
	require.Contains(t, body, "defaultThinkingLevel: xhigh")
	require.Contains(t, body, "serviceTier: priority")
	require.Contains(t, body, "default: sub2api-openai/gpt-5.5-Sys")
	require.NotContains(t, body, "sk-test")
}

func TestConfigGuideOMPPluginUnavailableDoesNotRenderPartialCommand(t *testing.T) {
	h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "")
	r := newConfigGuideTestRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/plugin.txt?api_key=sk-test", nil))

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.NotContains(t, w.Body.String(), "omp plugin install npm:omp-openai-provider-tools@")
	require.NotContains(t, w.Body.String(), "sk-test")
}

func TestConfigGuideOMPImageGeneratorDoesNotContainAPIKey(t *testing.T) {
	h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
	r := newConfigGuideTestRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/image-generator.md?api_key=sk-test", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/markdown")
	require.Contains(t, w.Body.String(), "image_generator")
	require.Contains(t, w.Body.String(), "sub2api-openai-image/gpt-5.5-Sys")
	require.NotContains(t, w.Body.String(), "sk-test")
}

func TestConfigGuideOMPErrorsAreNoStoreAndDoNotEchoAPIKey(t *testing.T) {
	h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
	r := newConfigGuideTestRouter(h)

	cases := []struct {
		target     string
		wantStatus int
	}{
		{"/config-guides/omp-openai/manifest.json", http.StatusBadRequest},
		{"/config-guides/omp-openai/manifest.json?api_key=%20%20", http.StatusBadRequest},
		{"/config-guides/omp-openai/manifest.json?api_key=sk-test&base_url=javascript:alert(1)", http.StatusBadRequest},
		{"/config-guides/omp-openai/manifest.json?api_key=sk-test&base_url=https://", http.StatusBadRequest},
		{"/config-guides/omp-openai/manifest.json?api_key=sk-test&base_url=https://user:pass@example.com/v1", http.StatusBadRequest},
		{"/config-guides/omp-openai/manifest.json?api_key=sk-test&base_url=https://example.com/v1?x=1", http.StatusBadRequest},
		{"/config-guides/omp-openai/manifest.json?api_key=sk-test&base_url=https://example.com/v1%0d%0aapiKey:%20sk-test", http.StatusBadRequest},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.target, nil))
		require.Equal(t, tc.wantStatus, w.Code, tc.target)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"), tc.target)
		require.Equal(t, "no-cache", w.Header().Get("Pragma"), tc.target)
		require.NotContains(t, w.Body.String(), "sk-test", tc.target)
	}
}

func TestConfigGuideOMPManifestIgnoresSpoofedForwardedHost(t *testing.T) {
	h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
	r := newConfigGuideTestRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/manifest.json?api_key=sk-test", nil)
	req.Host = "example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "attacker.example")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"base_url":"https://example.com/v1"`)
	require.NotContains(t, w.Body.String(), "attacker.example")
}

func TestConfigGuideOMPMissingRequiredModelFailsClosed(t *testing.T) {
	payload := configGuideModelsDevPayload()
	delete(payload["openai"].(map[string]any)["models"].(map[string]any), "gpt-5.4-mini")
	h := newConfigGuideTestHandler(t, payload, "0.1.2")
	r := newConfigGuideTestRouter(h)

	for _, target := range []string{
		"/config-guides/omp-openai/manifest.json?api_key=sk-test",
		"/config-guides/omp-openai/models.yml?api_key=sk-test",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))

		require.Equal(t, http.StatusServiceUnavailable, w.Code, target)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"), target)
		require.NotContains(t, w.Body.String(), "apiKey: sk-test", target)
		require.NotContains(t, w.Body.String(), "sk-test", target)
	}
}

func TestConfigGuideOpenCodeManifest(t *testing.T) {
	h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
	h.now = func() time.Time { return time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC) }
	r := newConfigGuideTestRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config-guides/opencode-openai/manifest.json?api_key=sk-test", nil)
	req.Host = "example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var manifest configGuideManifest
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &manifest))
	require.Equal(t, "opencode", manifest.Client)
	require.Equal(t, "https://example.com/v1", manifest.BaseURL)
	require.Len(t, manifest.Items, 1)
	require.Equal(t, "opencode", manifest.Items[0].ID)
	require.NotNil(t, manifest.Items[0].TargetPath)
	require.Equal(t, "~/.config/opencode/opencode.json", *manifest.Items[0].TargetPath)
	require.Contains(t, manifest.Items[0].URL, "/config-guides/opencode-openai/opencode.json?api_key=sk-test")
	require.Contains(t, manifest.Notes, "Download every item to a local temporary copy before editing existing files; do not transcribe YAML or JSON from chat output.")
	require.Contains(t, manifest.Notes, "If a target file is missing, copy the downloaded file to that path. If it exists, compare both files and merge the smaller side into the larger side when that is safer than replacing.")
	require.Contains(t, manifest.Notes, "After writing opencode.json, compare it with the downloaded copy and run an OpenCode configuration parse/check command if available before reporting completion.")
	require.NotContains(t, manifest.Notes, "plugin doctor")
}

func TestConfigGuideOpenCodeJSONPreservesLocalSemantics(t *testing.T) {
	h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
	r := newConfigGuideTestRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-test", nil)
	req.Host = "example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cfg))
	provider := cfg["provider"].(map[string]any)["sub2api-openai"].(map[string]any)
	require.Equal(t, "@ai-sdk/openai", provider["npm"])
	require.Equal(t, "sk-test", provider["options"].(map[string]any)["apiKey"])
	require.Equal(t, "https://example.com/v1", provider["options"].(map[string]any)["baseURL"])
	models := provider["models"].(map[string]any)
	fast := models["gpt-5.5-fast"].(map[string]any)
	require.Equal(t, "gpt-5.5", fast["id"])
	require.Equal(t, "priority", fast["options"].(map[string]any)["serviceTier"])
	require.Equal(t, "fast-mode", fast["headers"].(map[string]any)["x-test-header"])
	fastSys := models["gpt-5.5-fast-Sys"].(map[string]any)
	require.Equal(t, "gpt-5.5-Sys", fastSys["id"])
	require.Equal(t, "priority", fastSys["options"].(map[string]any)["serviceTier"])
	for _, modelID := range []string{"gpt-5.5", "gpt-5.5-fast", "gpt-5.5-Sys", "gpt-5.5-fast-Sys"} {
		model := models[modelID].(map[string]any)
		options := model["options"].(map[string]any)
		metadata := options["metadata"].(map[string]any)
		builtinTools := metadata["builtin_tools"].(map[string]any)
		require.Equal(t, true, builtinTools["web_search"], modelID)
		require.Equal(t, false, options["store"], modelID)
	}
	for _, modelID := range []string{"gpt-5.5", "gpt-5.5-fast", "gpt-5.5-Sys", "gpt-5.5-fast-Sys"} {
		model := models[modelID].(map[string]any)
		imageVariant := model["variants"].(map[string]any)["image"].(map[string]any)
		imageMetadata := imageVariant["metadata"].(map[string]any)
		require.Contains(t, imageMetadata["builtin_tools"], "image_generation", modelID)
	}
	agent := cfg["agent"].(map[string]any)["image"].(map[string]any)
	require.Equal(t, "sub2api-openai/gpt-5.5-fast-Sys", agent["model"])
	require.Equal(t, "image", agent["variant"])
	body := w.Body.String()
	require.NotContains(t, body, `"experimental"`)
	require.NotContains(t, body, `"service_tier"`)
}

func TestConfigGuideOpenCodeMissingRequiredModelFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "missing base model",
			mutate: func(payload map[string]any) {
				delete(payload["openai"].(map[string]any)["models"].(map[string]any), "gpt-5.5")
			},
		},
		{
			name: "missing fast mode",
			mutate: func(payload map[string]any) {
				gpt55 := payload["openai"].(map[string]any)["models"].(map[string]any)["gpt-5.5"].(map[string]any)
				modes := gpt55["experimental"].(map[string]any)["modes"].(map[string]any)
				delete(modes, "fast")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := configGuideModelsDevPayload()
			tc.mutate(payload)
			h := newConfigGuideTestHandler(t, payload, "0.1.2")
			r := newConfigGuideTestRouter(h)

			for _, target := range []string{
				"/config-guides/opencode-openai/manifest.json?api_key=sk-test",
				"/config-guides/opencode-openai/opencode.json?api_key=sk-test",
			} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))

				require.Equal(t, http.StatusServiceUnavailable, w.Code, target)
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"), target)
				require.NotContains(t, w.Body.String(), "apiKey", target)
				require.NotContains(t, w.Body.String(), "sk-test", target)
				require.NotContains(t, w.Body.String(), "gpt-5.5-fast-Sys", target)
			}
		})
	}
}
