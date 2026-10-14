package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dimAgentCapabilitiesHTTPUpstream struct{ service.HTTPUpstream }

func (u *dimAgentCapabilitiesHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if req.URL.Query().Get("type") != "dim" {
		return &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"missing type"}`))}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"deepseek-v4.1-flash","dim":{"name":"Flash","limit":{"context":1000000,"output":384000},"reasoning":{"supported":true,"effort":"high","effortOptions":["low","high","max"]},"modalities":["text","image"]}}]}`))}, nil
}

func newDimAgentCapabilitiesHandler(t *testing.T) (*OpenAIGatewayHandler, *service.Group) {
	t.Helper()
	account := service.Account{
		ID: 91, Name: "dimagent", Platform: service.PlatformDimAgent, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "access", "refresh_token": "refresh",
			"expires_at":     time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"relay_base_url": "https://dimagent.example/v1",
		},
	}
	group := &service.Group{ID: 19, Platform: service.PlatformDimAgent}
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}}
	gatewayService := service.NewOpenAIGatewayService(
		codexModelsFailoverAccountRepo{accounts: []service.Account{account}},
		nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil,
		&dimAgentCapabilitiesHTTPUpstream{},
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	gatewayService.SetDimAgentTokenProvider(service.NewDimAgentTokenProvider(nil, nil, nil))
	return &OpenAIGatewayHandler{gatewayService: gatewayService}, group
}

func performDimAgentCapabilitiesRequest(t *testing.T, h *OpenAIGatewayHandler, group *service.Group, etag string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/model-capabilities", nil)
	if etag != "" {
		c.Request.Header.Set("If-None-Match", etag)
	}
	if group != nil {
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{GroupID: &group.ID, Group: group})
	}
	h.DimAgentModelCapabilities(c)
	return recorder
}

func TestDimAgentModelCapabilitiesHandlerReturnsCatalogAnd304(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, group := newDimAgentCapabilitiesHandler(t)
	response := performDimAgentCapabilitiesRequest(t, h, group, "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.NotEmpty(t, response.Header().Get("ETag"))
	require.Equal(t, "private, max-age=60", response.Header().Get("Cache-Control"))
	var body service.DimAgentModelCapabilitiesEnvelope
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, service.DimAgentCapabilitiesObject, body.Object)
	require.Equal(t, int64(19), body.CatalogScope.GroupID)
	require.Equal(t, "deepseek-v4.1-flash", body.Data[0].ID)

	notModified := performDimAgentCapabilitiesRequest(t, h, group, response.Header().Get("ETag"))
	require.Equal(t, http.StatusNotModified, notModified.Code)
	require.Empty(t, notModified.Body.String())
}

func TestDimAgentModelCapabilitiesHandlerRejectsMissingOrWrongGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := newDimAgentCapabilitiesHandler(t)
	require.Equal(t, http.StatusUnauthorized, performDimAgentCapabilitiesRequest(t, h, nil, "").Code)
	require.Equal(t, http.StatusNotFound, performDimAgentCapabilitiesRequest(t, h, &service.Group{ID: 20, Platform: service.PlatformOpenAI}, "").Code)
}
