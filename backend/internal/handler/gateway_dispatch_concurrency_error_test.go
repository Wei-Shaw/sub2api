//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type dispatchFallbackUnavailableStore struct {
	disableExtra func()
}

type drainingAdmissionStore struct{}

func (drainingAdmissionStore) TryAcquireUserLease(context.Context, service.UserLeaseRequest) (service.UserLeaseResult, error) {
	return service.UserLeaseResult{Draining: true}, nil
}

func (drainingAdmissionStore) RenewUserLease(context.Context, int64, string, service.AdmissionClass) (bool, error) {
	return false, nil
}

func (drainingAdmissionStore) ReleaseUserLease(context.Context, int64, string) error {
	return nil
}

func (drainingAdmissionStore) TryAcquireTargetLease(context.Context, service.TargetLeaseRequest) (service.TargetLeaseResult, error) {
	return service.TargetLeaseResult{}, nil
}

func (drainingAdmissionStore) BeginTargetDispatch(context.Context, service.TargetDispatchRequest) (service.TargetDispatchResult, error) {
	return service.TargetDispatchResult{Started: true}, nil
}

func (drainingAdmissionStore) RenewTargetLease(context.Context, string, int64, string) (bool, error) {
	return false, nil
}

func (drainingAdmissionStore) ReleaseTargetLease(context.Context, string, int64, string) error {
	return nil
}

func (s *dispatchFallbackUnavailableStore) TryAcquireUserLease(_ context.Context, request service.UserLeaseRequest) (service.UserLeaseResult, error) {
	if request.ExtraLimit > 0 {
		return service.UserLeaseResult{Acquired: true, Class: service.AdmissionClassExtra}, nil
	}
	return service.UserLeaseResult{}, nil
}

func (s *dispatchFallbackUnavailableStore) RenewUserLease(context.Context, int64, string, service.AdmissionClass) (bool, error) {
	return true, nil
}

func (s *dispatchFallbackUnavailableStore) ReleaseUserLease(context.Context, int64, string) error {
	return nil
}

func (s *dispatchFallbackUnavailableStore) TryAcquireTargetLease(context.Context, service.TargetLeaseRequest) (service.TargetLeaseResult, error) {
	if s.disableExtra != nil {
		s.disableExtra()
	}
	return service.TargetLeaseResult{Acquired: true}, nil
}

func (s *dispatchFallbackUnavailableStore) BeginTargetDispatch(context.Context, service.TargetDispatchRequest) (service.TargetDispatchResult, error) {
	return service.TargetDispatchResult{Started: true}, nil
}

func (s *dispatchFallbackUnavailableStore) RenewTargetLease(context.Context, string, int64, string) (bool, error) {
	return true, nil
}

func (s *dispatchFallbackUnavailableStore) ReleaseTargetLease(context.Context, string, int64, string) error {
	return nil
}

func TestGatewayDispatchFallbackUnavailableReturnsConcurrencyProtocolError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		path     string
		body     string
		invoke   func(*GatewayHandler, *gin.Context)
		codePath string
	}{
		{
			name:     "anthropic messages",
			path:     "/v1/messages",
			body:     `{"model":"claude-sonnet-4-5","max_tokens":256,"messages":[{"role":"user","content":"hello"}]}`,
			invoke:   (*GatewayHandler).Messages,
			codePath: "error.type",
		},
		{
			name:     "chat completions",
			path:     "/v1/chat/completions",
			body:     `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}]}`,
			invoke:   (*GatewayHandler).ChatCompletions,
			codePath: "error.type",
		},
		{
			name:     "responses",
			path:     "/v1/responses",
			body:     `{"model":"claude-sonnet-4-5","stream":false,"input":"hello"}`,
			invoke:   (*GatewayHandler).Responses,
			codePath: "error.code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groupID := int64(2401)
			accountID := int64(1401)
			userID := int64(4401)
			group := &service.Group{
				ID:       groupID,
				Hydrated: true,
				Platform: service.PlatformAnthropic,
				Status:   service.StatusActive,
			}
			account := &service.Account{
				ID:          accountID,
				Name:        "dispatch-fallback-unavailable",
				Platform:    service.PlatformAnthropic,
				Type:        service.AccountTypeAPIKey,
				Concurrency: 1,
				Priority:    1,
				Status:      service.StatusActive,
				Schedulable: true,
				Credentials: map[string]any{
					"api_key":  "upstream-key",
					"base_url": "https://api.anthropic.com",
				},
				Extra:         map[string]any{"anthropic_passthrough": true},
				GroupIDs:      []int64{groupID},
				AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
			}
			upstream := &successfulAnthropicUpstream{}
			h, _, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
			defer cleanup()

			enabled := &atomic.Bool{}
			enabled.Store(true)
			settingService := service.NewSettingService(extraConcurrencySettingRepository{
				waitTimeoutSeconds: "1",
				enabledFlag:        enabled,
			}, &config.Config{})
			store := &dispatchFallbackUnavailableStore{}
			store.disableExtra = func() {
				enabled.Store(false)
				settingService.InvalidateExtraConcurrencyRuntimeSettings()
			}
			gatewayAdmission := service.NewGatewayAdmission(
				store,
				h.gatewayService,
				fixedAdmissionCapacity{accountID: accountID},
			)
			gatewayAdmission.SetExtraConcurrencyRuntimeSettingsSource(settingService)
			h.settingService = settingService
			h.gatewayAdmission = gatewayAdmission

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
			c.Request = req
			apiKey := &service.APIKey{
				ID:      3401,
				UserID:  userID,
				GroupID: &groupID,
				Status:  service.StatusActive,
				User: &service.User{
					ID:               userID,
					Status:           service.StatusActive,
					Concurrency:      1,
					ExtraConcurrency: 1,
					Balance:          100,
				},
				Group: group,
			}
			c.Set(string(middleware.ContextKeyAPIKey), apiKey)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
				UserID:           userID,
				Concurrency:      1,
				ExtraConcurrency: 1,
			})

			tt.invoke(h, c)

			require.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
			require.Equal(t, "rate_limit_error", gjson.GetBytes(recorder.Body.Bytes(), tt.codePath).String())
			require.Zero(t, upstream.calls.Load())
		})
	}
}

func TestGatewayAdmissionDrainingFallsBackToLegacyAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(2402)
	accountID := int64(1402)
	userID := int64(4402)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "draining-legacy-fallback",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Concurrency: 1,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://api.anthropic.com",
		},
		Extra:         map[string]any{"anthropic_passthrough": true},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	upstream := &successfulAnthropicUpstream{}
	h, legacyCache, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	legacyCache.userSeq = []bool{true}
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, h.cfg)
	h.gatewayAdmission = service.NewGatewayAdmission(
		drainingAdmissionStore{},
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/messages",
		bytes.NewBufferString(`{"model":"claude-sonnet-4-5","max_tokens":256,"messages":[{"role":"user","content":"hello"}]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	apiKey := &service.APIKey{
		ID:      3402,
		UserID:  userID,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:               userID,
			Status:           service.StatusActive,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: group,
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           userID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.Messages(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, int32(1), upstream.calls.Load())
	require.Equal(t, 1, legacyCache.userAcquireCalls)
	require.Equal(t, 1, legacyCache.userReleaseCalls)
}

func TestOpenAIResponsesWebSocketDispatchFallbackUnavailableClosesTryAgainLater(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var upstreamMessages atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		for {
			readCtx, cancelRead := context.WithTimeout(r.Context(), 5*time.Second)
			_, _, err = conn.Read(readCtx)
			cancelRead()
			if err != nil {
				return
			}
			upstreamMessages.Add(1)
		}
	}))
	defer upstream.Close()

	groupID := int64(2501)
	accountID := int64(1501)
	userID := int64(4501)
	account := service.Account{
		ID:          accountID,
		Name:        "openai-ws-dispatch-fallback-unavailable",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-ws", "base_url": upstream.URL},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"openai_apikey_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
		},
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: account}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	defer billingCacheService.Stop()
	gatewayService := service.NewOpenAIGatewayService(
		accountRepo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCacheService, nil,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	enabled := &atomic.Bool{}
	enabled.Store(true)
	settingService := service.NewSettingService(extraConcurrencySettingRepository{
		waitTimeoutSeconds: "1",
		enabledFlag:        enabled,
	}, cfg)
	store := &dispatchFallbackUnavailableStore{}
	store.disableExtra = func() {
		enabled.Store(false)
		settingService.InvalidateExtraConcurrencyRuntimeSettings()
	}
	gatewayAdmission := service.NewGatewayAdmission(
		store,
		nil,
		fixedAdmissionCapacity{accountID: accountID},
	)
	gatewayAdmission.SetExtraConcurrencyRuntimeSettingsSource(settingService)
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingCacheService,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper: NewConcurrencyHelper(
			service.NewConcurrencyService(&concurrencyCacheMock{}),
			SSEPingFormatNone,
			time.Second,
		),
		gatewayAdmission:   gatewayAdmission,
		maxAccountSwitches: 1,
		cfg:                cfg,
		settingService:     settingService,
	}
	apiKey := &service.APIKey{
		ID:      3501,
		UserID:  userID,
		GroupID: &groupID,
		User: &service.User{
			ID:               userID,
			Status:           service.StatusActive,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:           userID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	handlerServer := httptest.NewServer(router)
	defer handlerServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(handlerServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false}`))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 5*time.Second)
	_, _, readErr := clientConn.Read(readCtx)
	cancelRead()
	require.Error(t, readErr)
	require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(readErr))
	require.Zero(t, upstreamMessages.Load())
}
