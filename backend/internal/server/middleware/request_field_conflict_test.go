package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// newRequestFieldConflictTestRouter 按网关链的顺序挂载：白名单准入在前，
// 字段歧义拒绝在后。handlerBodies 记录 handler 实际读到的请求体。
func newRequestFieldConflictTestRouter(apiKey *service.APIKey) (*gin.Engine, *[]string) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var handlerBodies []string
	router.Use(func(c *gin.Context) {
		if apiKey != nil {
			c.Set(string(ContextKeyAPIKey), apiKey)
		}
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	router.Use(RejectConflictingRequestFields())
	handler := func(c *gin.Context) {
		body, _ := httputil.ReadRequestBodyWithPrealloc(c.Request)
		handlerBodies = append(handlerBodies, string(body))
		c.Status(http.StatusOK)
	}
	for _, path := range []string{
		"/v1/messages", "/v1/responses", "/v1/chat/completions", "/v1/images/edits", "/v1/live",
		"/v1beta/models/*modelAction",
	} {
		router.POST(path, handler)
	}
	router.GET("/v1/models", handler)
	return router, &handlerBodies
}

// 复现报告的请求体：第一个 model 是便宜模型，最后一个是贵模型。分组没有开启
// 白名单时旧行为是放行并按第一个计费，现在必须在进入 handler 之前拒绝。
func TestRejectConflictingRequestFieldsBlocksDisagreeingModelWithoutAllowlist(t *testing.T) {
	for _, apiKey := range []*service.APIKey{nil, {}, allowlistAPIKey(false)} {
		router, handlerBodies := newRequestFieldConflictTestRouter(apiKey)

		w := doJSON(t, router, http.MethodPost, "/v1/responses",
			`{"model":"gpt-5.4-nano","stream":false,"input":"hi","model":"gpt-5.4"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("conflicting model keys must be rejected, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `\"model\"`) || !strings.Contains(w.Body.String(), "invalid_request_error") {
			t.Fatalf("expected the field name and invalid_request_error, got %s", w.Body.String())
		}
		if len(*handlerBodies) != 0 {
			t.Fatalf("expected handler not to run, got %v", *handlerBodies)
		}
	}
}

// 白名单开启且两个模型都在名单内时，白名单会放行；字段歧义检查仍要拒绝，
// 否则计费模型和转发模型照样分叉。
func TestRejectConflictingRequestFieldsBlocksEvenWhenAllCandidatesAllowlisted(t *testing.T) {
	router, handlerBodies := newRequestFieldConflictTestRouter(allowlistAPIKey(true, "claude-3-5-haiku-20241022", "claude-opus-4-8"))

	w := doJSON(t, router, http.MethodPost, "/v1/messages",
		`{"model":"claude-3-5-haiku-20241022","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"model":"claude-opus-4-8"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("conflicting model keys must be rejected, got %d: %s", w.Code, w.Body.String())
	}
	if len(*handlerBodies) != 0 {
		t.Fatalf("expected handler not to run, got %v", *handlerBodies)
	}
}

func TestRejectConflictingRequestFieldsCoversOtherFields(t *testing.T) {
	router, handlerBodies := newRequestFieldConflictTestRouter(allowlistAPIKey(false))

	for _, body := range []string{
		`{"model":"gpt-5.4","service_tier":"default","input":"hi","service_tier":"priority"}`,
		`{"model":"gpt-5.4","Model":"gpt-5.4-nano"}`,
		`{"model":"gpt-5.4","messages":[],"messages":[{"role":"user","content":"hi"}]}`,
	} {
		w := doJSON(t, router, http.MethodPost, "/v1/chat/completions", body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s must be rejected, got %d: %s", body, w.Code, w.Body.String())
		}
	}
	if len(*handlerBodies) != 0 {
		t.Fatalf("expected handler not to run, got %v", *handlerBodies)
	}
}

func TestRejectConflictingRequestFieldsErrorFormats(t *testing.T) {
	router, _ := newRequestFieldConflictTestRouter(allowlistAPIKey(false))
	body := `{"model":"a","contents":[],"model":"b"}`

	openai := doJSON(t, router, http.MethodPost, "/v1/responses", body)
	if !strings.Contains(openai.Body.String(), `"error":{"message"`) || strings.Contains(openai.Body.String(), "model_not_found") {
		t.Fatalf("unexpected OpenAI error format: %s", openai.Body.String())
	}
	anthropic := doJSON(t, router, http.MethodPost, "/v1/messages", body)
	if !strings.Contains(anthropic.Body.String(), `"type":"error"`) || !strings.Contains(anthropic.Body.String(), `"type":"invalid_request_error"`) {
		t.Fatalf("unexpected Anthropic error format: %s", anthropic.Body.String())
	}
	google := doJSON(t, router, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", body)
	if google.Code != http.StatusBadRequest || !strings.Contains(google.Body.String(), `"status":"INVALID_ARGUMENT"`) {
		t.Fatalf("unexpected Google error format: %d %s", google.Code, google.Body.String())
	}
}

// 取值相同的重复字段、普通请求体和非对象请求体都放行，handler 读到的请求体
// 与客户端发送的完全一致。
func TestRejectConflictingRequestFieldsPassesUnambiguousBodiesUnchanged(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-5.4","input":"hi"}`,
		`{"model":"gpt-5.4","input":"hi","model":"gpt-5.4"}`,
		`{"model":"gpt-5.4","metadata":{"k":"1","k":"1"}}`,
		`not json`,
		``,
	} {
		router, handlerBodies := newRequestFieldConflictTestRouter(allowlistAPIKey(false))
		w := doJSON(t, router, http.MethodPost, "/v1/responses", body)
		if w.Code != http.StatusOK {
			t.Fatalf("body %q should pass, got %d: %s", body, w.Code, w.Body.String())
		}
		if len(*handlerBodies) != 1 || (*handlerBodies)[0] != body {
			t.Fatalf("handler must read the original body %q, got %v", body, *handlerBodies)
		}
	}
}

func TestRejectConflictingRequestFieldsMultipart(t *testing.T) {
	send := func(models ...string) *httptest.ResponseRecorder {
		router, _ := newRequestFieldConflictTestRouter(allowlistAPIKey(false))
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for _, model := range models {
			_ = writer.WriteField("model", model)
		}
		_ = writer.WriteField("prompt", "cat")
		_ = writer.Close()
		req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	if w := send("gpt-image-1", "gpt-image-1.5"); w.Code != http.StatusBadRequest {
		t.Fatalf("conflicting multipart model fields must be rejected, got %d: %s", w.Code, w.Body.String())
	}
	if w := send("gpt-image-1", "gpt-image-1"); w.Code != http.StatusOK {
		t.Fatalf("identical multipart model fields should pass, got %d: %s", w.Code, w.Body.String())
	}
	if w := send("gpt-image-1"); w.Code != http.StatusOK {
		t.Fatalf("single multipart model field should pass, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRejectConflictingRequestFieldsLiveSessionModel(t *testing.T) {
	router, _ := newRequestFieldConflictTestRouter(allowlistAPIKey(false))

	w := doJSON(t, router, http.MethodPost, "/v1/live", `{"sdp":"v=0","session":{"model":"gpt-live-a","model":"gpt-live-b"}}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "session.model") {
		t.Fatalf("conflicting session.model must be rejected, got %d: %s", w.Code, w.Body.String())
	}
}

// GET/DELETE 不带需要检查的请求体：中间件不读取，直接放行。
func TestRejectConflictingRequestFieldsDoesNotReadBodylessMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RejectConflictingRequestFields())
	called := false
	router.GET("/v1/models", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	body := &readTrackingBody{Reader: strings.NewReader(`{"model":"a","model":"b"}`)}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", body)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !called {
		t.Fatalf("GET must reach the handler, got %d", w.Code)
	}
	if body.read {
		t.Fatal("middleware must not read the body of a GET request")
	}
}

// 413 传递：请求体超过上限时保持现有的 413 响应，不被改写成 400。
func TestRejectConflictingRequestFieldsRequestBodyTooLargePassesThrough413(t *testing.T) {
	router, handlerBodies := newRequestFieldConflictTestRouter(allowlistAPIKey(false))

	bigBody := `{"model":"a","padding":"` + strings.Repeat("x", 512) + `","model":"b"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.MaxBytesReader(httptest.NewRecorder(), req.Body, 64)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
	if len(*handlerBodies) != 0 {
		t.Fatalf("expected handler not to run, got %v", *handlerBodies)
	}
}
