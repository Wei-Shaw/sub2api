package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type capabilityOwnerReaderStub struct {
	key *service.APIKey
	err error
}

func (s capabilityOwnerReaderStub) GetByID(context.Context, int64) (*service.APIKey, error) {
	return s.key, s.err
}

type capabilityResolverStub struct {
	caps   *service.APIKeyCapabilities
	err    error
	called bool
	userID int64
	keyID  int64
}

func (s *capabilityResolverStub) ResolveCapabilities(_ context.Context, userID, keyID int64) (*service.APIKeyCapabilities, error) {
	s.called = true
	s.userID = userID
	s.keyID = keyID
	return s.caps, s.err
}

func performCapabilitiesRequest(t *testing.T, h *APIKeyHandler, userID int64, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/user/api-keys/:id/capabilities", func(c *gin.Context) {
		if userID != 0 {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID})
		}
		h.GetCapabilities(c)
	})

	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestAPIKeyHandlerGetCapabilitiesRequiresAuthentication(t *testing.T) {
	resolver := &capabilityResolverStub{}
	h := &APIKeyHandler{
		capabilityOwner:    capabilityOwnerReaderStub{},
		capabilityResolver: resolver,
	}

	w := performCapabilitiesRequest(t, h, 0, "/api/v1/user/api-keys/7/capabilities")

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.False(t, resolver.called)
}

func TestAPIKeyHandlerGetCapabilitiesHidesForeignKey(t *testing.T) {
	resolver := &capabilityResolverStub{}
	h := &APIKeyHandler{
		capabilityOwner: capabilityOwnerReaderStub{key: &service.APIKey{
			ID:     7,
			UserID: 99,
		}},
		capabilityResolver: resolver,
	}

	w := performCapabilitiesRequest(t, h, 42, "/api/v1/user/api-keys/7/capabilities")

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "API key not found")
	require.False(t, resolver.called)
}

func TestAPIKeyHandlerGetCapabilitiesSerializesPublicContractOnly(t *testing.T) {
	resolver := &capabilityResolverStub{caps: &service.APIKeyCapabilities{
		Object: "api_key_capabilities",
		KeyID:  7,
		Group: &service.APIKeyCapabilityGroup{
			ID:       12,
			Platform: "composite",
		},
		GeminiNative: service.APIKeyGeminiNativeCapability{
			ImageGeneration: service.APIKeyGeminiNativeImageCapability{
				Available: true,
				GenerateContent: service.APIKeyCapabilityRoute{
					Available: true,
					Method:    "POST",
					Path:      "/v1beta/models/{model}:generateContent",
				},
				StreamGenerateContent: service.APIKeyCapabilityRoute{
					Available: true,
					Method:    "POST",
					Path:      "/v1beta/models/{model}:streamGenerateContent?alt=sse",
				},
			},
		},
	}}
	h := &APIKeyHandler{
		capabilityOwner: capabilityOwnerReaderStub{key: &service.APIKey{
			ID:     7,
			UserID: 42,
		}},
		capabilityResolver: resolver,
	}

	w := performCapabilitiesRequest(t, h, 42, "/api/v1/user/api-keys/7/capabilities")

	require.Equal(t, http.StatusOK, w.Code)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Equal(t, float64(0), envelope["code"])
	data, ok := envelope["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "api_key_capabilities", data["object"])
	require.Equal(t, float64(7), data["key_id"])
	geminiNative, ok := data["gemini_native"].(map[string]any)
	require.True(t, ok)
	imageGeneration, ok := geminiNative["image_generation"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, imageGeneration["available"])
	generateContent, ok := imageGeneration["generate_content"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/v1beta/models/{model}:generateContent", generateContent["path"])
	require.Equal(t, "POST", generateContent["method"])
	streamGenerateContent, ok := imageGeneration["stream_generate_content"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/v1beta/models/{model}:streamGenerateContent?alt=sse", streamGenerateContent["path"])
	require.Equal(t, "POST", streamGenerateContent["method"])
	require.True(t, resolver.called)
	require.Equal(t, int64(42), resolver.userID)
	require.Equal(t, int64(7), resolver.keyID)

	assertNoCapabilitySecretFields(t, data)
}

func assertNoCapabilitySecretFields(t *testing.T, value any) {
	t.Helper()
	forbidden := map[string]struct{}{
		"account_id":      {},
		"api_key":         {},
		"base_url":        {},
		"credential":      {},
		"key":             {},
		"password":        {},
		"secret":          {},
		"token":           {},
		"upstream_target": {},
	}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if _, found := forbidden[key]; found {
					t.Fatalf("forbidden capability field %q in response", key)
				}
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
}
