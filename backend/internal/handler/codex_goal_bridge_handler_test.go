package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeCodexGoalBridgeService struct {
	enabled      bool
	requests     chan service.CodexGoalBridgeRequest
	streamDeltas []string
	streamTools  bool
	result       *service.CodexGoalBridgeResponse
}

func (s *fakeCodexGoalBridgeService) IsEnabled() bool {
	return s.enabled
}

func (s *fakeCodexGoalBridgeService) Handle(ctx context.Context, req service.CodexGoalBridgeRequest) (*service.CodexGoalBridgeResponse, error) {
	if s.requests != nil {
		s.requests <- req
	}
	if s.result != nil {
		return s.result, nil
	}
	return &service.CodexGoalBridgeResponse{
		Protocol:  req.Protocol,
		Model:     "gpt-5.3-codex",
		Text:      "Codex goal result",
		AccountID: 42,
		Account: &service.Account{
			ID:       42,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
		},
		CreatedAt: time.Unix(1710000000, 0),
		Duration:  123 * time.Millisecond,
	}, nil
}

func (s *fakeCodexGoalBridgeService) HandleStream(ctx context.Context, req service.CodexGoalBridgeRequest, sink service.CodexGoalBridgeStreamSink) (*service.CodexGoalBridgeResponse, error) {
	if s.requests != nil {
		s.requests <- req
	}
	created := time.Unix(1710000000, 0)
	if err := sink(service.CodexGoalBridgeStreamEvent{
		Type:                     service.CodexGoalBridgeStreamEventStart,
		Protocol:                 req.Protocol,
		Model:                    "gpt-5.3-codex",
		AccountID:                42,
		DeferResponsesOutputItem: s.result != nil && len(s.result.FunctionCalls) > 0,
		Account: &service.Account{
			ID:       42,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
		},
		CreatedAt: created,
	}); err != nil {
		return nil, err
	}
	if s.result != nil {
		s.result.Stream = true
		if s.result.CreatedAt.IsZero() {
			s.result.CreatedAt = created
		}
		if s.streamTools {
			for i, event := range s.result.ToolEvents {
				if err := sink(service.CodexGoalBridgeStreamEvent{
					Type:           service.CodexGoalBridgeStreamEventToolEvent,
					Protocol:       req.Protocol,
					Model:          s.result.Model,
					AccountID:      s.result.AccountID,
					Account:        s.result.Account,
					CreatedAt:      s.result.CreatedAt,
					ToolEvent:      event,
					ToolEventIndex: i,
				}); err != nil {
					return nil, err
				}
			}
		}
		return s.result, nil
	}
	var text strings.Builder
	for _, delta := range s.streamDeltas {
		text.WriteString(delta)
		if err := sink(service.CodexGoalBridgeStreamEvent{
			Type:      service.CodexGoalBridgeStreamEventDelta,
			Protocol:  req.Protocol,
			Model:     "gpt-5.3-codex",
			AccountID: 42,
			Account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			CreatedAt: created,
			Delta:     delta,
		}); err != nil {
			return nil, err
		}
	}
	return &service.CodexGoalBridgeResponse{
		Protocol:  req.Protocol,
		Model:     "gpt-5.3-codex",
		Text:      text.String(),
		AccountID: 42,
		Account: &service.Account{
			ID:       42,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
		},
		CreatedAt: created,
		Duration:  123 * time.Millisecond,
		Stream:    true,
	}, nil
}

type fakeCodexGoalUsageRecorder struct {
	inputs chan *service.RecordUsageInput
}

func (r *fakeCodexGoalUsageRecorder) RecordUsage(ctx context.Context, input *service.RecordUsageInput) error {
	r.inputs <- input
	return nil
}

func TestCodexGoalBridgeHandlerResponsesIncludesToolEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled:  true,
		requests: make(chan service.CodexGoalBridgeRequest, 1),
		result: &service.CodexGoalBridgeResponse{
			Protocol:  service.CodexGoalProtocolOpenAIResponses,
			Model:     "gpt-5.5",
			Text:      "done",
			AccountID: 42,
			Account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			CreatedAt: time.Unix(1710000000, 0),
			ToolEvents: []service.CodexGoalToolEvent{
				{
					Type:   "web_search_call",
					ID:     "ws_1",
					Action: []byte(`{"type":"search","query":"OpenAI docs"}`),
				},
				{
					Type:        "mcp_call",
					ID:          "mcp_1",
					ServerLabel: "dmcp",
					Name:        "roll",
					Arguments:   `{"diceRollExpression":"2d4+1"}`,
					Output:      "6",
				},
			},
		},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolOpenAIResponses, "/v1/responses"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","input":"hello"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, `"type":"web_search_call"`)
	require.Contains(t, body, `"type":"mcp_call"`)
	require.Contains(t, body, `"server_label":"dmcp"`)
	require.Contains(t, body, `"type":"message"`)
	require.Less(t, strings.Index(body, `"type":"web_search_call"`), strings.Index(body, `"type":"message"`))
}

func TestCodexGoalBridgeHandlerResponsesIncludesFunctionCall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled:     true,
		streamTools: true,
		result: &service.CodexGoalBridgeResponse{
			Protocol:  service.CodexGoalProtocolOpenAIResponses,
			Model:     "gpt-5.5",
			AccountID: 42,
			Account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			CreatedAt: time.Unix(1710000000, 0),
			FunctionCalls: []service.CodexGoalFunctionCall{{
				ID:        "fc_1",
				CallID:    "call_1",
				Name:      "lookup",
				Arguments: `{"q":"beta"}`,
			}},
		},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolOpenAIResponses, "/v1/responses"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","input":"hello"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, `"type":"function_call"`)
	require.Contains(t, body, `"call_id":"call_1"`)
	require.Contains(t, body, `"name":"lookup"`)
	require.Contains(t, body, `"arguments":"{\"q\":\"beta\"}"`)
	require.NotContains(t, body, `"type":"message"`)
}

func TestCodexGoalBridgeHandlerChatIncludesToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled: true,
		result: &service.CodexGoalBridgeResponse{
			Protocol:  service.CodexGoalProtocolOpenAIChat,
			Model:     "gpt-5.5",
			AccountID: 42,
			Account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			CreatedAt: time.Unix(1710000000, 0),
			FunctionCalls: []service.CodexGoalFunctionCall{{
				CallID:    "call_1",
				Name:      "lookup",
				Arguments: `{"q":"chat"}`,
			}},
		},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolOpenAIChat, "/v1/chat/completions"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}]}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, `"finish_reason":"tool_calls"`)
	require.Contains(t, body, `"tool_calls"`)
	require.Contains(t, body, `"id":"call_1"`)
	require.Contains(t, body, `"name":"lookup"`)
	require.Contains(t, body, `"arguments":"{\"q\":\"chat\"}"`)
}

func TestCodexGoalBridgeHandlerAnthropicIncludesToolUse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled: true,
		result: &service.CodexGoalBridgeResponse{
			Protocol:  service.CodexGoalProtocolAnthropic,
			Model:     "gpt-5.5",
			AccountID: 42,
			Account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			CreatedAt: time.Unix(1710000000, 0),
			FunctionCalls: []service.CodexGoalFunctionCall{{
				ID:        "toolu_1",
				Name:      "lookup_marker",
				Arguments: `{"key":"alpha"}`,
			}},
		},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.POST("/v1/messages", func(c *gin.Context) {
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolAnthropic, "/v1/messages"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3-5-sonnet","tools":[{"name":"lookup_marker","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"call lookup"}]}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, `"type":"tool_use"`)
	require.Contains(t, body, `"id":"toolu_1"`)
	require.Contains(t, body, `"name":"lookup_marker"`)
	require.Contains(t, body, `"key":"alpha"`)
	require.Contains(t, body, `"stop_reason":"tool_use"`)
}

func TestCodexGoalBridgeHandlerGeminiIncludesFunctionCall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled: true,
		result: &service.CodexGoalBridgeResponse{
			Protocol:  service.CodexGoalProtocolGemini,
			Model:     "gpt-5.5",
			AccountID: 42,
			Account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			CreatedAt: time.Unix(1710000000, 0),
			FunctionCalls: []service.CodexGoalFunctionCall{{
				ID:        "func_1",
				Name:      "lookup_marker",
				Arguments: `{"key":"gemini"}`,
			}},
		},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.POST("/v1beta/models/gemini-2.5-pro:generateContent", func(c *gin.Context) {
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolGemini, "/gemini-2.5-pro:generateContent"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(`{"tools":[{"functionDeclarations":[{"name":"lookup_marker","parameters":{"type":"object"}}]}],"contents":[{"role":"user","parts":[{"text":"call lookup"}]}]}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, `"functionCall"`)
	require.Contains(t, body, `"id":"func_1"`)
	require.Contains(t, body, `"name":"lookup_marker"`)
	require.Contains(t, body, `"key":"gemini"`)
	require.Contains(t, body, `"finishReason":"STOP"`)
}

func TestCodexGoalBridgeHandlerRecordsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled:      true,
		requests:     make(chan service.CodexGoalBridgeRequest, 1),
		streamDeltas: []string{"Codex ", "goal result"},
	}
	recorder := &fakeCodexGoalUsageRecorder{inputs: make(chan *service.RecordUsageInput, 1)}
	h := &CodexGoalBridgeHandler{service: svc, gatewayService: recorder}
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		groupID := int64(2)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      7,
			UserID:  9,
			GroupID: &groupID,
			User:    &service.User{ID: 9, Status: service.StatusActive},
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI},
		})
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolOpenAIResponses, "/v1/responses"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.3-codex","input":"hello"}`))
	req.Header.Set("User-Agent", "codex-goal-test")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	select {
	case input := <-recorder.inputs:
		require.NotNil(t, input.Result)
		require.Equal(t, int64(7), input.APIKey.ID)
		require.Equal(t, int64(9), input.User.ID)
		require.Equal(t, int64(42), input.Account.ID)
		require.Equal(t, "gpt-5.3-codex", input.Result.Model)
		require.Equal(t, 123*time.Millisecond, input.Result.Duration)
		require.Equal(t, "/v1/responses", input.InboundEndpoint)
		require.Equal(t, "/codex/goal", input.UpstreamEndpoint)
		require.Equal(t, "codex-goal-test", input.UserAgent)
		require.NotEmpty(t, input.RequestPayloadHash)
		require.False(t, input.Result.Stream)
		require.False(t, input.Result.OpenAIWSMode)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for usage record")
	}
}

func TestCodexGoalBridgeHandlerStreamsResponsesDeltas(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled:      true,
		requests:     make(chan service.CodexGoalBridgeRequest, 1),
		streamDeltas: []string{"hel", "lo"},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolOpenAIResponses, "/v1/responses"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.3-codex","stream":true,"input":"hello"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, "event: response.created")
	require.Equal(t, 2, strings.Count(body, "event: response.output_text.delta"))
	require.Contains(t, body, `"delta":"hel"`)
	require.Contains(t, body, `"delta":"lo"`)
	require.NotContains(t, body, `"delta":"hello"`)
	require.Contains(t, body, "event: response.completed")
	require.Less(t, strings.Index(body, `"delta":"hel"`), strings.Index(body, `"delta":"lo"`))
	require.Less(t, strings.Index(body, `"delta":"lo"`), strings.Index(body, "event: response.completed"))

	select {
	case req := <-svc.requests:
		require.Equal(t, service.CodexGoalProtocolOpenAIResponses, req.Protocol)
		require.Contains(t, string(req.Body), `"stream":true`)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for bridge request")
	}
}

func TestCodexGoalBridgeHandlerStreamsResponsesFunctionCall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled:  true,
		requests: make(chan service.CodexGoalBridgeRequest, 1),
		result: &service.CodexGoalBridgeResponse{
			Protocol:  service.CodexGoalProtocolOpenAIResponses,
			Model:     "gpt-5.5",
			AccountID: 42,
			Account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			CreatedAt: time.Unix(1710000000, 0),
			FunctionCalls: []service.CodexGoalFunctionCall{{
				ID:        "fc_1",
				CallID:    "call_1",
				Name:      "lookup",
				Arguments: `{"q":"gamma"}`,
			}},
		},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolOpenAIResponses, "/v1/responses"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","stream":true,"tools":[{"type":"function","name":"lookup"}],"input":"call lookup"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, "event: response.created")
	require.Contains(t, body, "event: response.output_item.added")
	require.Contains(t, body, `"type":"function_call"`)
	require.Contains(t, body, "event: response.function_call_arguments.delta")
	require.Contains(t, body, `"delta":"{\"q\":\"gamma\"}"`)
	require.Contains(t, body, "event: response.function_call_arguments.done")
	require.Contains(t, body, "event: response.output_item.done")
	require.Contains(t, body, "event: response.completed")
	require.NotContains(t, body, "codex_goal_function_call")
	require.NotContains(t, body, "response.output_text.delta")
}

func TestCodexGoalBridgeHandlerStreamsResponsesToolEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled: true,
		result: &service.CodexGoalBridgeResponse{
			Protocol:  service.CodexGoalProtocolOpenAIResponses,
			Model:     "gpt-5.5",
			Text:      "MCP result is 7.",
			AccountID: 42,
			Account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			CreatedAt: time.Unix(1710000000, 0),
			ToolEvents: []service.CodexGoalToolEvent{{
				Type:        "mcp_call",
				ID:          "mcp_1",
				ServerLabel: "dmcp",
				Name:        "roll",
				Arguments:   `{"diceRollExpression":"2d4+1"}`,
				Output:      "7",
			}},
		},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolOpenAIResponses, "/v1/responses"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","stream":true,"tools":[{"type":"mcp","server_url":"https://dmcp-server.example/sse","server_label":"dmcp"}],"input":"roll"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, "event: response.created")
	require.Contains(t, body, "event: response.output_item.added")
	require.Contains(t, body, `"type":"mcp_call"`)
	require.Contains(t, body, `"output":"7"`)
	require.Contains(t, body, `"server_label":"dmcp"`)
	require.Contains(t, body, `"output_index":1`)
	require.Contains(t, body, "event: response.completed")
	require.Equal(t, 2, strings.Count(body, "event: response.output_item.added"))
	require.Equal(t, 2, strings.Count(body, "event: response.output_item.done"))
	require.Less(t, strings.Index(body, `"type":"message"`), strings.Index(body, `"type":"mcp_call"`))
}

func TestCodexGoalBridgeHandlerStreamsAnthropicToolUse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled: true,
		result: &service.CodexGoalBridgeResponse{
			Protocol:  service.CodexGoalProtocolAnthropic,
			Model:     "gpt-5.5",
			AccountID: 42,
			Account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			CreatedAt: time.Unix(1710000000, 0),
			FunctionCalls: []service.CodexGoalFunctionCall{{
				ID:        "toolu_1",
				Name:      "lookup_marker",
				Arguments: `{"key":"stream-alpha"}`,
			}},
		},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.POST("/v1/messages", func(c *gin.Context) {
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolAnthropic, "/v1/messages"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3-5-sonnet","stream":true,"tools":[{"name":"lookup_marker","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"call lookup"}]}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, "event: message_start")
	require.Contains(t, body, "event: content_block_start")
	require.Contains(t, body, `"type":"tool_use"`)
	require.Contains(t, body, `"id":"toolu_1"`)
	require.Contains(t, body, `"key":"stream-alpha"`)
	require.Contains(t, body, `"stop_reason":"tool_use"`)
	require.Contains(t, body, "event: message_stop")
	require.NotContains(t, body, "text_delta")
}

func TestCodexGoalBridgeHandlerStreamsGeminiFunctionCall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled: true,
		result: &service.CodexGoalBridgeResponse{
			Protocol:  service.CodexGoalProtocolGemini,
			Model:     "gpt-5.5",
			AccountID: 42,
			Account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			CreatedAt: time.Unix(1710000000, 0),
			FunctionCalls: []service.CodexGoalFunctionCall{{
				ID:        "func_stream_1",
				Name:      "lookup_marker",
				Arguments: `{"key":"stream-gemini"}`,
			}},
		},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.POST("/v1beta/models/gemini-2.5-pro:streamGenerateContent", func(c *gin.Context) {
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolGemini, "/gemini-2.5-pro:streamGenerateContent"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent", strings.NewReader(`{"tools":[{"functionDeclarations":[{"name":"lookup_marker","parameters":{"type":"object"}}]}],"contents":[{"role":"user","parts":[{"text":"call lookup"}]}]}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, `data: {`)
	require.Contains(t, body, `"functionCall"`)
	require.Contains(t, body, `"id":"func_stream_1"`)
	require.Contains(t, body, `"key":"stream-gemini"`)
	require.Contains(t, body, `"finishReason":"STOP"`)
	require.NotContains(t, body, "codex_goal_function_call")
}

func TestCodexGoalBridgeHandlerStreamsChatToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled: true,
		result: &service.CodexGoalBridgeResponse{
			Protocol:  service.CodexGoalProtocolOpenAIChat,
			Model:     "gpt-5.5",
			AccountID: 42,
			Account: &service.Account{
				ID:       42,
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			CreatedAt: time.Unix(1710000000, 0),
			FunctionCalls: []service.CodexGoalFunctionCall{{
				CallID:    "call_1",
				Name:      "lookup",
				Arguments: `{"q":"chat"}`,
			}},
		},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		require.True(t, h.TryHandle(c, service.CodexGoalProtocolOpenAIChat, "/v1/chat/completions"))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","stream":true,"tools":[{"type":"function","function":{"name":"lookup"}}],"messages":[{"role":"user","content":"call lookup"}]}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	require.Contains(t, body, `"tool_calls"`)
	require.Contains(t, body, `"index":0`)
	require.Contains(t, body, `"id":"call_1"`)
	require.Contains(t, body, `"name":"lookup"`)
	require.Contains(t, body, `"arguments":"{\"q\":\"chat\"}"`)
	require.Contains(t, body, `"finish_reason":"tool_calls"`)
	require.Contains(t, body, "data: [DONE]")
	require.NotContains(t, body, "codex_goal_function_call")
	require.NotContains(t, body, `"content":"`)
}

func TestCodexGoalBridgeHandlerResponsesWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeCodexGoalBridgeService{
		enabled:      true,
		requests:     make(chan service.CodexGoalBridgeRequest, 1),
		streamDeltas: []string{"Codex ", "goal result"},
	}
	h := &CodexGoalBridgeHandler{service: svc}
	router := gin.New()
	router.GET("/responses", func(c *gin.Context) {
		require.True(t, h.TryHandleResponsesWebSocket(c))
	})
	server := httptest.NewServer(router)
	defer server.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	conn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http")+"/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = conn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = conn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[{"type":"input_text","text":"hello from ws"}]}`))
	cancelWrite()
	require.NoError(t, err)

	var eventTypes []string
	var deltas []string
	for i := 0; i < 10; i++ {
		readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
		_, payload, err := conn.Read(readCtx)
		cancelRead()
		require.NoError(t, err)
		var event struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		}
		require.NoError(t, json.Unmarshal(payload, &event))
		eventTypes = append(eventTypes, event.Type)
		if event.Type == "response.output_text.delta" {
			deltas = append(deltas, event.Delta)
		}
		if event.Type == "response.completed" {
			break
		}
	}

	require.Contains(t, eventTypes, "response.created")
	require.Contains(t, eventTypes, "response.output_text.delta")
	require.Contains(t, eventTypes, "response.completed")
	require.Equal(t, "Codex goal result", strings.Join(deltas, ""))

	select {
	case req := <-svc.requests:
		require.Equal(t, service.CodexGoalProtocolOpenAIResponses, req.Protocol)
		require.Equal(t, "/responses", req.Endpoint)
		require.Contains(t, string(req.Body), "hello from ws")
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for bridge request")
	}
}
