package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAIModelAccessHandlerForTest(t *testing.T, selectFn func(context.Context, service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error)) *OpenAIGatewayHandler {
	t.Helper()
	gatewayService := &service.OpenAIGatewayService{}
	setUnexportedFieldForTest(t, gatewayService, "openaiScheduler", &openAIAccountSchedulerStub{selectFn: selectFn})
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimple})
	t.Cleanup(billingService.Stop)
	return &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingService,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&concurrencyCacheMock{
			acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) {
				return true, nil
			},
			acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
				return true, nil
			},
		}), SSEPingFormatNone, time.Second),
	}
}

func newOpenAIModelAccessContext(method, path, body, role string) (*httptest.ResponseRecorder, *gin.Context) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	groupID := int64(21)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 201, GroupID: &groupID, User: &service.User{ID: 11, Role: role}})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 11, Concurrency: 1})
	c.Set(string(middleware.ContextKeyUserRole), role)
	return w, c
}

func requireOpenAIModelAccessError(t *testing.T, status int, body string) {
	t.Helper()
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "invalid_request_error")
	require.Contains(t, body, "gpt-5.5-Sys")
}

func TestOpenAIResponses_NonAdminNonSysModelRequiresSysModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheduled := false
	h := newOpenAIModelAccessHandlerForTest(t, func(context.Context, service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
		scheduled = true
		return nil, service.OpenAIAccountScheduleDecision{Layer: "load_balance"}, service.ErrNoAvailableAccounts
	})
	w, c := newOpenAIModelAccessContext(http.MethodPost, "/openai/v1/responses", `{"model":"gpt-5.5","stream":false,"input":[{"type":"input_text","text":"hello"}]}`, service.RoleUser)

	h.Responses(c)

	requireOpenAIModelAccessError(t, w.Code, w.Body.String())
	require.False(t, scheduled, "non-admin non-Sys request must be rejected before scheduling")
}

func TestOpenAIChatCompletions_NonAdminNonSysModelRequiresSysModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheduled := false
	h := newOpenAIModelAccessHandlerForTest(t, func(context.Context, service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
		scheduled = true
		return nil, service.OpenAIAccountScheduleDecision{Layer: "load_balance"}, service.ErrNoAvailableAccounts
	})
	w, c := newOpenAIModelAccessContext(http.MethodPost, "/v1/chat/completions", `{"model":"gpt-5.5","stream":false,"messages":[{"role":"user","content":"hello"}]}`, service.RoleUser)

	h.ChatCompletions(c)

	requireOpenAIModelAccessError(t, w.Code, w.Body.String())
	require.False(t, scheduled, "non-admin non-Sys request must be rejected before scheduling")
}

func TestOpenAIMessages_NonAdminNonSysModelRequiresSysModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheduled := false
	h := newOpenAIModelAccessHandlerForTest(t, func(context.Context, service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
		scheduled = true
		return nil, service.OpenAIAccountScheduleDecision{Layer: "load_balance"}, service.ErrNoAvailableAccounts
	})
	w, c := newOpenAIModelAccessContext(http.MethodPost, "/v1/messages", `{"model":"gpt-5.5","stream":false,"messages":[{"role":"user","content":"hello"}]}`, service.RoleUser)

	h.Messages(c)

	requireOpenAIModelAccessError(t, w.Code, w.Body.String())
	require.False(t, scheduled, "non-admin non-Sys request must be rejected before scheduling")
}

func TestOpenAIChatCompletions_NonAdminSysModelAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var capturedReq service.OpenAIAccountScheduleRequest
	h := newOpenAIModelAccessHandlerForTest(t, func(_ context.Context, req service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
		capturedReq = req
		return nil, service.OpenAIAccountScheduleDecision{Layer: "load_balance"}, service.ErrNoAvailableAccounts
	})
	w, c := newOpenAIModelAccessContext(http.MethodPost, "/v1/chat/completions", `{"model":"gpt-5.5-Sys","stream":false,"messages":[{"role":"user","content":"hello"}]}`, service.RoleUser)

	h.ChatCompletions(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Equal(t, service.TargetGroupExhausted, capturedReq.TargetGroup)
	require.Equal(t, "gpt-5.5", capturedReq.RequestedModel)
}

func TestOpenAIResponsesWebSocket_NonAdminNonSysModelRequiresSysModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheduled := make(chan struct{}, 1)
	h := newOpenAIModelAccessHandlerForTest(t, func(context.Context, service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
		select {
		case scheduled <- struct{}{}:
		default:
		}
		return nil, service.OpenAIAccountScheduleDecision{Layer: "load_balance"}, service.ErrNoAvailableAccounts
	})
	wsServer := newOpenAIModelAccessWSTestServer(h, service.RoleUser)
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.5","stream":true}`))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	require.Error(t, err)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	require.Contains(t, closeErr.Reason, "gpt-5.5-Sys")
	select {
	case <-scheduled:
		t.Fatal("non-admin non-Sys websocket request must be rejected before scheduling")
	default:
	}
}

func newOpenAIModelAccessWSTestServer(h *OpenAIGatewayHandler, role string) *httptest.Server {
	groupID := int64(22)
	apiKey := &service.APIKey{ID: 202, GroupID: &groupID, User: &service.User{ID: 12, Role: role}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 12, Concurrency: 1})
		c.Set(string(middleware.ContextKeyUserRole), role)
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	return httptest.NewServer(router)
}
