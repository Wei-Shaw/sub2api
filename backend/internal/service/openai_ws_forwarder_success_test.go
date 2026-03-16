package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_Forward_WSv2_SuccessAndBindSticky(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type receivedPayload struct {
		Type               string
		PreviousResponseID string
		StreamExists       bool
		Stream             bool
	}
	receivedCh := make(chan receivedPayload, 1)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var request map[string]any
		if err := conn.ReadJSON(&request); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		requestJSON := requestToJSONString(request)
		receivedCh <- receivedPayload{
			Type:               strings.TrimSpace(gjson.Get(requestJSON, "type").String()),
			PreviousResponseID: strings.TrimSpace(gjson.Get(requestJSON, "previous_response_id").String()),
			StreamExists:       gjson.Get(requestJSON, "stream").Exists(),
			Stream:             gjson.Get(requestJSON, "stream").Bool(),
		}

		if err := conn.WriteJSON(map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id":    "resp_new_1",
				"model": "gpt-5.1",
			},
		}); err != nil {
			t.Errorf("write response.created failed: %v", err)
			return
		}
		if err := conn.WriteJSON(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":    "resp_new_1",
				"model": "gpt-5.1",
				"usage": map[string]any{
					"input_tokens":  12,
					"output_tokens": 7,
					"input_tokens_details": map[string]any{
						"cached_tokens": 3,
					},
				},
			},
		}); err != nil {
			t.Errorf("write response.completed failed: %v", err)
			return
		}
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")
	groupID := int64(1001)
	c.Set("api_key", &APIKey{GroupID: &groupID})

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 30
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 10
	cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = 3600

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cache := &stubGatewayCache{}
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		cache:            cache,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          9,
		Name:        "openai-ws",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_prev_1","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	require.Equal(t, "resp_new_1", result.RequestID)
	require.True(t, result.OpenAIWSMode)
	require.False(t, gjson.GetBytes(upstream.lastBody, "model").Exists(), "WSv2 成功时不应回落 HTTP 上游")

	received := <-receivedCh
	require.Equal(t, "response.create", received.Type)
	require.Equal(t, "resp_prev_1", received.PreviousResponseID)
	require.True(t, received.StreamExists, "WS 请求应携带 stream 字段")
	require.False(t, received.Stream, "应保持客户端 stream=false 的原始语义")

	store := svc.getOpenAIWSStateStore()
	mappedAccountID, getErr := store.GetResponseAccount(context.Background(), groupID, "resp_new_1")
	require.NoError(t, getErr)
	require.Equal(t, account.ID, mappedAccountID)
	connID, ok := store.GetResponseConn("resp_new_1")
	require.True(t, ok)
	require.NotEmpty(t, connID)

	responseBody := rec.Body.Bytes()
	require.Equal(t, "resp_new_1", gjson.GetBytes(responseBody, "id").String())
}

func requestToJSONString(payload map[string]any) string {
	if len(payload) == 0 {
		return "{}"
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func TestLogOpenAIWSBindResponseAccountWarn(t *testing.T) {
	require.NotPanics(t, func() {
		logOpenAIWSBindResponseAccountWarn(1, 2, "resp_ok", nil)
	})
	require.NotPanics(t, func() {
		logOpenAIWSBindResponseAccountWarn(1, 2, "resp_err", errors.New("bind failed"))
	})
}

func TestOpenAIGatewayService_Forward_WSv2_RewriteModelAndToolCallsOnCompletedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
	groupID := int64(3001)
	c.Set("api_key", &APIKey{GroupID: &groupID})

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	captureConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_model_tool_1","model":"gpt-5.1","tool_calls":[{"function":{"name":"apply_patch","arguments":"{\"file_path\":\"/tmp/a.txt\",\"old_string\":\"a\",\"new_string\":\"b\"}"}}],"usage":{"input_tokens":2,"output_tokens":1}},"tool_calls":[{"function":{"name":"apply_patch","arguments":"{\"file_path\":\"/tmp/a.txt\",\"old_string\":\"a\",\"new_string\":\"b\"}"}}]}`),
		},
	}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}

	account := &Account{
		ID:          1301,
		Name:        "openai-rewrite",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test",
			"model_mapping": map[string]any{
				"custom-original-model": "gpt-5.1",
			},
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"custom-original-model","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_model_tool_1", result.RequestID)
	require.Equal(t, "custom-original-model", gjson.GetBytes(rec.Body.Bytes(), "model").String(), "响应模型应回写为原始请求模型")
	require.Equal(t, "edit", gjson.GetBytes(rec.Body.Bytes(), "tool_calls.0.function.name").String(), "工具名称应被修正为 OpenCode 规范")
}

func TestOpenAIWSPayloadString_OnlyAcceptsStringValues(t *testing.T) {
	payload := map[string]any{
		"type":                 nil,
		"model":                123,
		"prompt_cache_key":     " cache-key ",
		"previous_response_id": []byte(" resp_1 "),
	}

	require.Equal(t, "", openAIWSPayloadString(payload, "type"))
	require.Equal(t, "", openAIWSPayloadString(payload, "model"))
	require.Equal(t, "cache-key", openAIWSPayloadString(payload, "prompt_cache_key"))
	require.Equal(t, "resp_1", openAIWSPayloadString(payload, "previous_response_id"))
}

func TestOpenAIGatewayService_Forward_WSv2_PoolReuseNotOneToOne(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var upgradeCount atomic.Int64
	var sequence atomic.Int64
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgradeCount.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		for {
			var request map[string]any
			if err := conn.ReadJSON(&request); err != nil {
				return
			}
			idx := sequence.Add(1)
			responseID := "resp_reuse_" + strconv.FormatInt(idx, 10)
			if err := conn.WriteJSON(map[string]any{
				"type": "response.created",
				"response": map[string]any{
					"id":    responseID,
					"model": "gpt-5.1",
				},
			}); err != nil {
				return
			}
			if err := conn.WriteJSON(map[string]any{
				"type": "response.completed",
				"response": map[string]any{
					"id":    responseID,
					"model": "gpt-5.1",
					"usage": map[string]any{
						"input_tokens":  2,
						"output_tokens": 1,
					},
				},
			}); err != nil {
				return
			}
		}
	}))
	defer wsServer.Close()

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 30
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 10

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}
	account := &Account{
		ID:          19,
		Name:        "openai-ws",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
		c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
		groupID := int64(2001)
		c.Set("api_key", &APIKey{GroupID: &groupID})

		body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_prev_reuse","input":[{"type":"input_text","text":"hello"}]}`)
		result, err := svc.Forward(context.Background(), c, account, body)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.True(t, strings.HasPrefix(result.RequestID, "resp_reuse_"))
	}

	// 条件式 MarkBroken：正常终端事件退出后连接归还复用，不再无条件销毁。
	require.Equal(t, int64(1), upgradeCount.Load(), "正常完成后连接应归还复用，不应每次新建")
	metrics := svc.SnapshotOpenAIWSPoolMetrics()
	require.GreaterOrEqual(t, metrics.AcquireReuseTotal, int64(1))
	require.GreaterOrEqual(t, metrics.ConnPickTotal, int64(1))
}

func TestOpenAIGatewayService_Forward_WSv2_OAuthStoreFalseByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
	c.Request.Header.Set("session_id", "sess-oauth-1")
	c.Request.Header.Set("conversation_id", "conv-oauth-1")

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.AllowStoreRecovery = false
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	captureConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_oauth_1","model":"gpt-5.1","usage":{"input_tokens":3,"output_tokens":2}}}`),
		},
	}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          29,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "oauth-token-1",
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"store":true,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_oauth_1", result.RequestID)

	require.NotNil(t, captureConn.lastWrite)
	requestJSON := requestToJSONString(captureConn.lastWrite)
	require.True(t, gjson.Get(requestJSON, "store").Exists(), "OAuth WSv2 应显式写入 store 字段")
	require.False(t, gjson.Get(requestJSON, "store").Bool(), "默认策略应将 OAuth store 置为 false")
	require.True(t, gjson.Get(requestJSON, "stream").Exists(), "WSv2 payload 应保留 stream 字段")
	require.True(t, gjson.Get(requestJSON, "stream").Bool(), "OAuth Codex 规范化后应强制 stream=true")
	require.Equal(t, openAIWSBetaV2Value, captureDialer.lastHeaders.Get("OpenAI-Beta"))
	// OAuth 账号的 session_id/conversation_id 应被 isolateOpenAISessionID 隔离，
	// 测试中未设置 api_key 到 context，apiKeyID=0。
	require.Equal(t, isolateOpenAISessionID(0, "sess-oauth-1"), captureDialer.lastHeaders.Get("session_id"))
	require.Equal(t, isolateOpenAISessionID(0, "conv-oauth-1"), captureDialer.lastHeaders.Get("conversation_id"))
}

func TestOpenAIGatewayService_Forward_WSv2_OAuthOriginatorCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userAgent      string
		originator     string
		wantOriginator string
	}{
		{name: "desktop originator preserved", originator: "Codex Desktop", wantOriginator: "Codex Desktop"},
		{name: "vscode originator preserved", originator: "codex_vscode", wantOriginator: "codex_vscode"},
		{name: "official ua fallback to codex_cli_rs", userAgent: "Codex Desktop/1.2.3", wantOriginator: "codex_cli_rs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
			if tt.userAgent != "" {
				c.Request.Header.Set("User-Agent", tt.userAgent)
			}
			if tt.originator != "" {
				c.Request.Header.Set("originator", tt.originator)
			}

			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.AllowStoreRecovery = false
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

			captureConn := &openAIWSCaptureConn{
				events: [][]byte{
					[]byte(`{"type":"response.completed","response":{"id":"resp_oauth_originator","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
				},
			}
			captureDialer := &openAIWSCaptureDialer{conn: captureConn}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(captureDialer)

			svc := &OpenAIGatewayService{
				cfg:              cfg,
				httpUpstream:     &httpUpstreamRecorder{},
				cache:            &stubGatewayCache{},
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
				toolCorrector:    NewCodexToolCorrector(),
				openaiWSPool:     pool,
			}
			account := &Account{
				ID:          129,
				Name:        "openai-oauth",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Credentials: map[string]any{
					"access_token": "oauth-token-1",
				},
				Extra: map[string]any{
					"responses_websockets_v2_enabled": true,
				},
			}

			body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tt.wantOriginator, captureDialer.lastHeaders.Get("originator"))
		})
	}
}

func TestOpenAIGatewayService_Forward_WSv2_HeaderSessionFallbackFromPromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	captureConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_prompt_cache_key","model":"gpt-5.1","usage":{"input_tokens":2,"output_tokens":1}}}`),
		},
	}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          31,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "oauth-token-1",
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":true,"prompt_cache_key":"pcache_123","input":[{"type":"input_text","text":"hi"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_prompt_cache_key", result.RequestID)

	// OAuth 账号的 session_id 应被 isolateOpenAISessionID 隔离（apiKeyID=0，未在 context 设置）。
	require.Equal(t, isolateOpenAISessionID(0, "pcache_123"), captureDialer.lastHeaders.Get("session_id"))
	require.Empty(t, captureDialer.lastHeaders.Get("conversation_id"))
	require.NotNil(t, captureConn.lastWrite)
	require.True(t, gjson.Get(requestToJSONString(captureConn.lastWrite), "stream").Exists())
}

func TestOpenAIGatewayService_Forward_WSv1_Unsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsockets = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = false

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          39,
		Name:        "openai-ws-v1",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1/responses",
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_prev_v1","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "ws v1")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "WSv1")
	require.Nil(t, upstream.lastReq, "WSv1 不支持时不应触发 HTTP 上游请求")
}

func TestOpenAIGatewayService_Forward_WSv2_TurnStateAndMetadataReplayOnReconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var connIndex atomic.Int64
	headersCh := make(chan http.Header, 4)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := connIndex.Add(1)
		headersCh <- cloneHeader(r.Header)

		respHeader := http.Header{}
		if idx == 1 {
			respHeader.Set("x-codex-turn-state", "turn_state_first")
		}
		conn, err := upgrader.Upgrade(w, r, respHeader)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var request map[string]any
		if err := conn.ReadJSON(&request); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		responseID := "resp_turn_" + strconv.FormatInt(idx, 10)
		if err := conn.WriteJSON(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":    responseID,
				"model": "gpt-5.1",
				"usage": map[string]any{
					"input_tokens":  2,
					"output_tokens": 1,
				},
			},
		}); err != nil {
			t.Errorf("write response.completed failed: %v", err)
			return
		}
	}))
	defer wsServer.Close()

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 0

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          49,
		Name:        "openai-turn-state",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	reqBody := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)
	rec1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(rec1)
	c1.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c1.Request.Header.Set("session_id", "session_turn_state")
	c1.Request.Header.Set("x-codex-turn-metadata", "turn_meta_1")
	result1, err := svc.Forward(context.Background(), c1, account, reqBody)
	require.NoError(t, err)
	require.NotNil(t, result1)

	sessionHash := svc.GenerateSessionHash(c1, reqBody)
	store := svc.getOpenAIWSStateStore()
	turnState, ok := store.GetSessionTurnState(0, sessionHash)
	require.True(t, ok)
	require.Equal(t, "turn_state_first", turnState)

	// 主动淘汰连接，模拟下一次请求发生重连。
	connID, hasConn := store.GetResponseConn(result1.RequestID)
	require.True(t, hasConn)
	svc.getOpenAIWSConnPool().evictConn(account.ID, connID)

	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c2.Request.Header.Set("session_id", "session_turn_state")
	c2.Request.Header.Set("x-codex-turn-metadata", "turn_meta_2")
	result2, err := svc.Forward(context.Background(), c2, account, reqBody)
	require.NoError(t, err)
	require.NotNil(t, result2)

	firstHandshakeHeaders := <-headersCh
	secondHandshakeHeaders := <-headersCh
	require.Equal(t, "turn_meta_1", firstHandshakeHeaders.Get("X-Codex-Turn-Metadata"))
	require.Equal(t, "turn_meta_2", secondHandshakeHeaders.Get("X-Codex-Turn-Metadata"))
	require.Equal(t, "turn_state_first", secondHandshakeHeaders.Get("X-Codex-Turn-State"))
}

func TestOpenAIGatewayService_Forward_WSv2_GeneratePrewarm(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("session_id", "session-prewarm")

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.PrewarmGenerateEnabled = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	captureConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_prewarm_1","model":"gpt-5.1","usage":{"input_tokens":0,"output_tokens":0}}}`),
			[]byte(`{"type":"response.completed","response":{"id":"resp_main_1","model":"gpt-5.1","usage":{"input_tokens":4,"output_tokens":2}}}`),
		},
	}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}

	account := &Account{
		ID:          59,
		Name:        "openai-prewarm",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_main_1", result.RequestID)

	require.Len(t, captureConn.writes, 2, "开启 generate=false 预热后应发送两次 WS 请求")
	firstWrite := requestToJSONString(captureConn.writes[0])
	secondWrite := requestToJSONString(captureConn.writes[1])
	require.True(t, gjson.Get(firstWrite, "generate").Exists())
	require.False(t, gjson.Get(firstWrite, "generate").Bool())
	require.False(t, gjson.Get(secondWrite, "generate").Exists())
}

func TestOpenAIGatewayService_PrewarmReadHonorsParentContext(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.PrewarmGenerateEnabled = true
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	svc := &OpenAIGatewayService{
		cfg:           cfg,
		toolCorrector: NewCodexToolCorrector(),
	}
	account := &Account{
		ID:          601,
		Name:        "openai-prewarm-timeout",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
	}
	conn := newOpenAIWSConn("prewarm_ctx_conn", account.ID, &openAIWSBlockingConn{
		readDelay: 200 * time.Millisecond,
	}, nil)
	lease := &openAIWSConnLease{
		accountID: account.ID,
		conn:      conn,
	}
	payload := map[string]any{
		"type":  "response.create",
		"model": "gpt-5.1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := svc.performOpenAIWSGeneratePrewarm(
		ctx,
		lease,
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		payload,
		"",
		map[string]any{"model": "gpt-5.1"},
		account,
		nil,
		0,
	)
	elapsed := time.Since(start)
	require.Error(t, err)
	require.Contains(t, err.Error(), "prewarm_read_event")
	require.Less(t, elapsed, 180*time.Millisecond, "预热读取应受父 context 取消控制，不应阻塞到 read_timeout")
}

func TestOpenAIGatewayService_Forward_WSv2_TurnMetadataInPayloadOnConnReuse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	captureConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_meta_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
			[]byte(`{"type":"response.completed","response":{"id":"resp_meta_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}

	account := &Account{
		ID:          69,
		Name:        "openai-turn-metadata",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)

	rec1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(rec1)
	c1.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c1.Request.Header.Set("session_id", "session-metadata-reuse")
	c1.Request.Header.Set("x-codex-turn-metadata", "turn_meta_payload_1")
	result1, err := svc.Forward(context.Background(), c1, account, body)
	require.NoError(t, err)
	require.NotNil(t, result1)
	require.Equal(t, "resp_meta_1", result1.RequestID)

	require.Len(t, captureConn.writes, 1)
	firstWrite := requestToJSONString(captureConn.writes[0])
	require.Equal(t, "turn_meta_payload_1", gjson.Get(firstWrite, "client_metadata.x-codex-turn-metadata").String())

	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c2.Request.Header.Set("session_id", "session-metadata-reuse")
	c2.Request.Header.Set("x-codex-turn-metadata", "turn_meta_payload_2")
	result2, err := svc.Forward(context.Background(), c2, account, body)
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.Equal(t, "resp_meta_2", result2.RequestID)

	require.Equal(t, 1, captureDialer.DialCount(), "同一账号两轮请求应复用同一 WS 连接")
	require.Len(t, captureConn.writes, 2)

	firstWrite = requestToJSONString(captureConn.writes[0])
	secondWrite := requestToJSONString(captureConn.writes[1])
	require.Equal(t, "turn_meta_payload_1", gjson.Get(firstWrite, "client_metadata.x-codex-turn-metadata").String())
	require.Equal(t, "turn_meta_payload_2", gjson.Get(secondWrite, "client_metadata.x-codex-turn-metadata").String())
}

func TestOpenAIGatewayService_Forward_WSv2StoreFalseSessionConnIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var upgradeCount atomic.Int64
	var sequence atomic.Int64
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgradeCount.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		for {
			var request map[string]any
			if err := conn.ReadJSON(&request); err != nil {
				return
			}
			responseID := "resp_store_false_" + strconv.FormatInt(sequence.Add(1), 10)
			if err := conn.WriteJSON(map[string]any{
				"type": "response.completed",
				"response": map[string]any{
					"id":    responseID,
					"model": "gpt-5.1",
					"usage": map[string]any{
						"input_tokens":  1,
						"output_tokens": 1,
					},
				},
			}); err != nil {
				return
			}
		}
	}))
	defer wsServer.Close()

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 4
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 4
	cfg.Gateway.OpenAIWS.StoreDisabledForceNewConn = true

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          79,
		Name:        "openai-store-false",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"store":false,"input":[{"type":"input_text","text":"hello"}]}`)

	rec1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(rec1)
	c1.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c1.Request.Header.Set("session_id", "session_store_false_a")
	result1, err := svc.Forward(context.Background(), c1, account, body)
	require.NoError(t, err)
	require.NotNil(t, result1)
	require.Equal(t, int64(1), upgradeCount.Load())

	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c2.Request.Header.Set("session_id", "session_store_false_a")
	result2, err := svc.Forward(context.Background(), c2, account, body)
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.Equal(t, int64(1), upgradeCount.Load(), "同一 session(store=false) 应复用同一 WS 连接")

	rec3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(rec3)
	c3.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c3.Request.Header.Set("session_id", "session_store_false_b")
	result3, err := svc.Forward(context.Background(), c3, account, body)
	require.NoError(t, err)
	require.NotNil(t, result3)
	require.Equal(t, int64(2), upgradeCount.Load(), "不同 session(store=false) 应隔离连接，避免续链状态互相覆盖")
}

func TestOpenAIGatewayService_Forward_WSv2StoreFalseDisableForceNewConnAllowsReuse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var upgradeCount atomic.Int64
	var sequence atomic.Int64
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgradeCount.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		for {
			var request map[string]any
			if err := conn.ReadJSON(&request); err != nil {
				return
			}
			responseID := "resp_store_false_reuse_" + strconv.FormatInt(sequence.Add(1), 10)
			if err := conn.WriteJSON(map[string]any{
				"type": "response.completed",
				"response": map[string]any{
					"id":    responseID,
					"model": "gpt-5.1",
					"usage": map[string]any{
						"input_tokens":  1,
						"output_tokens": 1,
					},
				},
			}); err != nil {
				return
			}
		}
	}))
	defer wsServer.Close()

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.StoreDisabledForceNewConn = false

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          80,
		Name:        "openai-store-false-reuse",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"store":false,"input":[{"type":"input_text","text":"hello"}]}`)

	rec1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(rec1)
	c1.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c1.Request.Header.Set("session_id", "session_store_false_reuse_a")
	result1, err := svc.Forward(context.Background(), c1, account, body)
	require.NoError(t, err)
	require.NotNil(t, result1)
	require.Equal(t, int64(1), upgradeCount.Load())

	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c2.Request.Header.Set("session_id", "session_store_false_reuse_b")
	result2, err := svc.Forward(context.Background(), c2, account, body)
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.Equal(t, int64(1), upgradeCount.Load(), "关闭强制新连后，不同 session(store=false) 可复用连接")
}

func TestOpenAIGatewayService_Forward_WSv2ReadTimeoutAppliesPerRead(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	captureConn := &openAIWSCaptureConn{
		readDelays: []time.Duration{
			700 * time.Millisecond,
			700 * time.Millisecond,
		},
		events: [][]byte{
			[]byte(`{"type":"response.created","response":{"id":"resp_timeout_ok","model":"gpt-5.1"}}`),
			[]byte(`{"type":"response.completed","response":{"id":"resp_timeout_ok","model":"gpt-5.1","usage":{"input_tokens":2,"output_tokens":1}}}`),
		},
	}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_fallback","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}

	account := &Account{
		ID:          81,
		Name:        "openai-read-timeout",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_timeout_ok", result.RequestID)
	require.Nil(t, upstream.lastReq, "每次 Read 都应独立应用超时；总时长超过 read_timeout 不应误回退 HTTP")
}

type openAIWSCaptureDialer struct {
	mu          sync.Mutex
	conn        *openAIWSCaptureConn
	lastHeaders http.Header
	handshake   http.Header
	dialCount   int
}

func (d *openAIWSCaptureDialer) Dial(
	ctx context.Context,
	wsURL string,
	headers http.Header,
	proxyURL string,
) (openAIWSClientConn, int, http.Header, error) {
	_ = ctx
	_ = wsURL
	_ = proxyURL
	d.mu.Lock()
	d.lastHeaders = cloneHeader(headers)
	d.dialCount++
	respHeaders := cloneHeader(d.handshake)
	d.mu.Unlock()
	return d.conn, 0, respHeaders, nil
}

func (d *openAIWSCaptureDialer) DialCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dialCount
}

type openAIWSCaptureConn struct {
	mu         sync.Mutex
	readDelays []time.Duration
	events     [][]byte
	lastWrite  map[string]any
	writes     []map[string]any
	closed     bool
}

func (c *openAIWSCaptureConn) WriteJSON(ctx context.Context, value any) error {
	_ = ctx
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errOpenAIWSConnClosed
	}
	switch payload := value.(type) {
	case map[string]any:
		c.lastWrite = cloneMapStringAny(payload)
		c.writes = append(c.writes, cloneMapStringAny(payload))
	case json.RawMessage:
		var parsed map[string]any
		if err := json.Unmarshal(payload, &parsed); err == nil {
			c.lastWrite = cloneMapStringAny(parsed)
			c.writes = append(c.writes, cloneMapStringAny(parsed))
		}
	case []byte:
		var parsed map[string]any
		if err := json.Unmarshal(payload, &parsed); err == nil {
			c.lastWrite = cloneMapStringAny(parsed)
			c.writes = append(c.writes, cloneMapStringAny(parsed))
		}
	}
	return nil
}

func (c *openAIWSCaptureConn) ReadMessage(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errOpenAIWSConnClosed
	}
	if len(c.events) == 0 {
		c.mu.Unlock()
		return nil, io.EOF
	}
	delay := time.Duration(0)
	if len(c.readDelays) > 0 {
		delay = c.readDelays[0]
		c.readDelays = c.readDelays[1:]
	}
	event := c.events[0]
	c.events = c.events[1:]
	c.mu.Unlock()
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return event, nil
}

func (c *openAIWSCaptureConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	payload, err := c.ReadMessage(ctx)
	if err != nil {
		return coderws.MessageText, nil, err
	}
	return coderws.MessageText, payload, nil
}

func (c *openAIWSCaptureConn) WriteFrame(ctx context.Context, _ coderws.MessageType, payload []byte) error {
	return c.WriteJSON(ctx, json.RawMessage(payload))
}

func (c *openAIWSCaptureConn) Ping(ctx context.Context) error {
	_ = ctx
	return nil
}

func (c *openAIWSCaptureConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func cloneMapStringAny(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
