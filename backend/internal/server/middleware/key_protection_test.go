package middleware_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/keyprotection"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type protectionSettingsStub struct {
	config keyprotection.Config
	err    error
}

func (s *protectionSettingsStub) GetKeyProtectionConfig(context.Context) (keyprotection.Config, error) {
	return s.config, s.err
}
func protectedTestStore(t *testing.T) (*keyprotection.RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	return keyprotection.NewRedisStore(client, strings.Repeat("42", 32)), mr
}
func protectionRouter(settings *protectionSettingsStub, store *keyprotection.RedisStore, h gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.OpsErrorLoggerMiddleware(nil))
	r.Use(func(c *gin.Context) {
		group := int64(3)
		user := int64(1)
		if c.GetHeader("X-Test-Principal") == "two" {
			user = 2
		}
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 7, UserID: user, GroupID: &group})
		c.Next()
	})
	r.Use(middleware.KeyProtection(settings, store, 1<<20))
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/embeddings", "/v1/images/generations", "/v1beta/models/*action"} {
		r.POST(path, h)
	}
	r.GET("/v1/responses", h)
	return r
}

func TestKeyProtectionHTTPRoundTripAuditRetryAndLogs(t *testing.T) {
	secret := "ghp_" + strings.Repeat("A", 36)
	for _, protocol := range []string{"chat", "responses", "messages"} {
		t.Run(protocol, func(t *testing.T) {
			store, _ := protectedTestStore(t)
			cfg := keyprotection.DefaultConfig()
			cfg.Enabled = true
			settings := &protectionSettingsStub{config: cfg}
			path := "/v1/chat/completions"
			auditProtocol := "openai_chat_completions"
			input := map[string]any{"model": "fake-model", "messages": []any{map[string]any{"role": "user", "content": "请用 " + secret + " 查询仓库，再使用 " + secret}}, "stream": false}
			if protocol == "responses" {
				path = "/v1/responses"
				auditProtocol = "responses"
				delete(input, "messages")
				input["input"] = "请用 " + secret + " 查询仓库，再使用 " + secret
			}
			if protocol == "messages" {
				path = "/v1/messages"
				auditProtocol = "messages"
				input["max_tokens"] = 100
			}
			var upstreamBodies [][]byte
			var attempts atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				upstreamBodies = append(upstreamBodies, body)
				require.NotContains(t, string(body), secret)
				require.Contains(t, string(body), "keyx_")
				if attempts.Add(1) == 1 {
					w.WriteHeader(503)
					_, _ = w.Write([]byte(`{"error":{"message":"retry fake"}}`))
					return
				}
				var root map[string]any
				require.NoError(t, json.Unmarshal(body, &root))
				text := ""
				if protocol == "responses" {
					text = root["input"].(string)
				} else {
					text = root["messages"].([]any)[0].(map[string]any)["content"].(string)
				}
				args, _ := json.Marshal(map[string]string{"command": text, "escaped": "quotes \" and slash \\\n" + text})
				var reply any
				switch protocol {
				case "chat":
					reply = map[string]any{"id": "chat_fake", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": text, "tool_calls": []any{map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": "query", "arguments": string(args)}}}}}}, "usage": map[string]int{"prompt_tokens": 33, "completion_tokens": 9}}
				case "responses":
					reply = map[string]any{"id": "resp_fake", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": text}}}, map[string]any{"type": "function_call", "name": "query", "call_id": "call_1", "arguments": string(args)}}, "usage": map[string]int{"input_tokens": 33, "output_tokens": 9}}
				case "messages":
					var toolInput any
					require.NoError(t, json.Unmarshal(args, &toolInput))
					reply = map[string]any{"id": "msg_fake", "type": "message", "content": []any{map[string]any{"type": "text", "text": text}, map[string]any{"type": "tool_use", "id": "call_1", "name": "query", "input": toolInput}}, "usage": map[string]int{"input_tokens": 33, "output_tokens": 9}}
				}
				w.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(w).Encode(reply))
			}))
			defer upstream.Close()
			core, observed := observer.New(zap.InfoLevel)
			router := protectionRouter(settings, store, func(c *gin.Context) {
				body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
				require.NoError(t, err)
				snapshot, err := securityaudit.ExtractPromptSnapshot(securityaudit.Request{Protocol: auditProtocol, Body: body})
				require.NoError(t, err)
				require.NotContains(t, snapshot.FullPrompt, secret)
				require.NotContains(t, snapshot.ScanText, secret)
				require.Contains(t, snapshot.ScanText, "keyx_")
				cached, ok := c.Get(gin.BodyBytesKey)
				require.True(t, ok)
				require.Equal(t, body, cached)
				// Both simulated failover attempts use the HTTP reread hook.
				for i := 0; i < 2; i++ {
					retryBody, err := c.Request.GetBody()
					require.NoError(t, err)
					req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstream.URL, retryBody)
					require.NoError(t, err)
					resp, err := http.DefaultClient.Do(req)
					require.NoError(t, err)
					data, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					_ = resp.Body.Close()
					if resp.StatusCode == 503 {
						continue
					}
					c.Data(resp.StatusCode, "application/json", data)
				}
			})
			body, err := json.Marshal(input)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(logger.IntoContext(req.Context(), zap.New(core)))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), secret)
			require.NotContains(t, w.Body.String(), "keyx_")
			require.Contains(t, w.Body.String(), `33`)
			require.Contains(t, w.Body.String(), `9`)
			require.True(t, json.Valid(w.Body.Bytes()))
			require.Len(t, upstreamBodies, 2)
			require.Equal(t, upstreamBodies[0], upstreamBodies[1])
			require.ElementsMatch(t, []string{"no-store", "private"}, strings.Split(w.Header().Get("Cache-Control"), ", "))
			for _, entry := range observed.All() {
				serialized, _ := json.Marshal(entry.ContextMap())
				require.NotContains(t, string(serialized), secret)
				require.NotContains(t, entry.Message, secret)
			}
		})
	}
}

func TestKeyProtectionContinuationAndMultiRoundEcho(t *testing.T) {
	store, mr := protectedTestStore(t)
	cfg := keyprotection.DefaultConfig()
	cfg.Enabled = true
	var calls int
	var firstToken string
	router := protectionRouter(&protectionSettingsStub{config: cfg}, store, func(c *gin.Context) {
		calls++
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		var root map[string]any
		require.NoError(t, json.Unmarshal(body, &root))
		text := root["input"].(string)
		if calls == 1 {
			firstToken = text
		}
		if calls == 2 {
			require.Equal(t, firstToken, text)
		}
		c.JSON(200, gin.H{"id": fmt.Sprintf("resp_%d", calls), "output": []any{gin.H{"type": "message", "content": []any{gin.H{"type": "output_text", "text": firstToken}}}}})
	})
	secret := "ghp_" + strings.Repeat("B", 36)
	request := func(input, previous, user string) *httptest.ResponseRecorder {
		payload := map[string]any{"input": input}
		if previous != "" {
			payload["previous_response_id"] = previous
		}
		data, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(data))
		req.Header.Set("X-Test-Principal", user)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	w := request(secret, "", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), secret)
	w = request(secret, "", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), secret)
	w = request("continue", "resp_1", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), secret)
	w = request("continue", "resp_1", "two")
	require.Equal(t, 400, w.Code)
	require.Equal(t, 3, calls)
	mr.FastForward(time.Duration(cfg.TTLSeconds+1) * time.Second)
	w = request("continue", "resp_1", "")
	require.Equal(t, 400, w.Code)
	require.Equal(t, 3, calls)
	require.NotContains(t, w.Body.String(), secret)
}

func TestKeyProtectionDisabledSelectionFailuresAndCompression(t *testing.T) {
	store, _ := protectedTestStore(t)
	cfg := keyprotection.DefaultConfig()
	settings := &protectionSettingsStub{config: cfg}
	calls := 0
	last := ""
	router := protectionRouter(settings, store, func(c *gin.Context) {
		calls++
		b, _ := io.ReadAll(c.Request.Body)
		last = string(b)
		c.Data(200, "application/json", []byte(`{"choices":[]}`))
	})
	secret := "ghp_" + strings.Repeat("C", 36)
	raw := `{"messages":[{"role":"user","content":"` + secret + `"}]}`
	run := func(path string, body []byte, encoding string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, bytes.NewReader(body))
		r.Header.Set("Content-Encoding", encoding)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 200, run("/v1/chat/completions", []byte(raw), "").Code)
	require.Equal(t, raw, last)
	settings.config.Enabled = true
	settings.config.UserIDs = []int64{99}
	require.Equal(t, 200, run("/v1/chat/completions", []byte(raw), "").Code)
	require.Equal(t, raw, last)
	settings.config.UserIDs = nil
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	_, _ = gz.Write([]byte(raw))
	require.NoError(t, gz.Close())
	require.Equal(t, 200, run("/v1/chat/completions", compressed.Bytes(), "gzip").Code)
	require.NotContains(t, last, secret)
	before := calls
	for _, path := range []string{"/v1/embeddings", "/v1/images/generations", "/v1beta/models/fake:generateContent"} {
		require.Equal(t, 400, run(path, []byte(raw), "").Code)
	}
	for _, body := range []string{`{"messages":`, `{"messages":[],"messages":[]}`, `{"input":"a","background":true}`, `{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"secret","signature":"signed"}]}]}`} {
		require.NotEqual(t, 200, run("/v1/chat/completions", []byte(body), "").Code)
	}
	require.NotEqual(t, 200, run("/v1/chat/completions", []byte(raw), "unsupported").Code)
	req := httptest.NewRequest("GET", "/v1/responses", nil)
	req.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, 400, w.Code)
	require.Equal(t, before, calls)
	settings.err = errors.New("fake backend outage")
	require.Equal(t, 503, run("/v1/chat/completions", []byte(raw), "").Code)
	require.Equal(t, before, calls)
}
