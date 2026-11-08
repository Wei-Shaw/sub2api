package middleware

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

func newGroupModelAllowlistTestRouter(apiKey *service.APIKey, pathPrefix string) (*gin.Engine, *[]string) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var calls []string
	router.Use(func(c *gin.Context) {
		if apiKey != nil {
			c.Set(string(ContextKeyAPIKey), apiKey)
		}
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	register := func(method, path string) {
		router.Handle(method, path, func(c *gin.Context) {
			calls = append(calls, "handler:"+path)
			c.Status(http.StatusOK)
		})
	}
	register(http.MethodPost, pathPrefix+"/messages")
	register(http.MethodPost, pathPrefix+"/responses")
	register(http.MethodPost, pathPrefix+"/chat/completions")
	register(http.MethodPost, pathPrefix+"/embeddings")
	register(http.MethodGet, pathPrefix+"/models/:model")
	register(http.MethodPost, pathPrefix+"/models/*modelAction")
	register(http.MethodGet, pathPrefix+"/realtime")
	register(http.MethodPost, pathPrefix+"/images/edits")
	register(http.MethodPost, pathPrefix+"/live")
	return router, &calls
}

func allowlistAPIKey(enabled bool, models ...string) *service.APIKey {
	return &service.APIKey{
		Group: &service.Group{
			Platform: service.PlatformAnthropic,
			ModelAllowlist: service.GroupModelAllowlist{
				Enabled: enabled,
				Models:  models,
			},
		},
	}
}

func doJSON(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// readTrackingBody 记录请求体是否被读取，用于断言快速路径零读取。
type readTrackingBody struct {
	io.Reader
	read bool
}

func (b *readTrackingBody) Read(p []byte) (int, error) {
	b.read = true
	return b.Reader.Read(p)
}

func (b *readTrackingBody) Close() error { return nil }

func TestGroupModelAllowlistDisabledDoesNotReadBody(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(false, "claude-sonnet-4.5"), "/v1")

	body := &readTrackingBody{Reader: strings.NewReader(`{"model":"claude-opus-4.6"}`)}
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(*calls) != 1 {
		t.Fatalf("expected handler to run once, got %v", *calls)
	}
	if body.read {
		t.Fatal("allowlist disabled: middleware must not read the request body")
	}
}

// 白名单开启但请求不携带模型时同样不应读体之外产生副作用：读体是必要开销，
// 这里验证读取后请求体被完整回填（handler 可零拷贝重读）。
func TestGroupModelAllowlistEnabledModelFreeRequestRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), allowlistAPIKey(true, "claude-sonnet-4.5"))
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	router.POST("/v1/messages", func(c *gin.Context) {
		body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			t.Errorf("handler reread body: %v", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		if string(body) != `{"input":"no model field"}` {
			t.Errorf("handler saw wrong body: %s", body)
		}
		c.Status(http.StatusOK)
	})

	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"input":"no model field"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// 413 传递：请求体超过 MaxBytesReader 上限时按现有合成中间件方式返回 413。
func TestGroupModelAllowlistRequestBodyTooLargePassesThrough413(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	bigBody := `{"model":"claude-opus-4.6","padding":"` + strings.Repeat("x", 512) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.MaxBytesReader(httptest.NewRecorder(), req.Body, 64)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Request body is too large") {
		t.Fatalf("expected too-large message, got %s", w.Body.String())
	}
	if len(*calls) != 0 {
		t.Fatalf("expected handler not to run, got %v", *calls)
	}
}

func TestGroupModelAllowlistJSONBodyAllowed(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"model":"claude-sonnet-4.5"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(*calls) != 1 {
		t.Fatalf("expected handler to run once, got %v", *calls)
	}
}

func TestGroupModelAllowlistJSONBodyDeniedOpenAIFormat(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/responses", `{"model":"gpt-5.4"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"model_not_found"`) {
		t.Fatalf("expected OpenAI error format, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `gpt-5.4\" is not available for this group`) {
		t.Fatalf("expected model in message, got %s", w.Body.String())
	}
	if len(*calls) != 0 {
		t.Fatalf("expected handler not to run, got %v", *calls)
	}
}

func TestGroupModelAllowlistMessagesDeniedAnthropicFormat(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"model":"claude-opus-4.6"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"not_found_error"`) {
		t.Fatalf("expected Anthropic not_found_error format, got %s", w.Body.String())
	}
}

func TestGroupModelAllowlistGeminiDeniedGoogleFormat(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gemini-2.5-pro"), "/v1beta")

	w := doJSON(t, router, http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", `{}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"NOT_FOUND"`) {
		t.Fatalf("expected Google NOT_FOUND format, got %s", w.Body.String())
	}
}

func TestGroupModelAllowlistGeminiWildcardAndPrefixForms(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gemini-2.5-*"), "/v1beta")

	w := doJSON(t, router, http.MethodPost, "/v1beta/models/models/gemini-2.5-flash:generateContent", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("models/ prefixed model should match wildcard entry, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistGeminiGetModelRouteParam(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gemini-2.5-pro"), "/v1beta")

	req := httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-2.5-flash", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unlisted GET model, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-2.5-pro", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for listed GET model, got %d", w.Code)
	}
}

func TestGroupModelAllowlistRealtimeQueryParameter(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "grok-4.6"), "/v1")

	req := httptest.NewRequest(http.MethodGet, "/v1/realtime?model=grok-4.6", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for listed realtime model, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=grok-4.20", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unlisted realtime model, got %d", w.Code)
	}
}

func TestGroupModelAllowlistMultipartModelField(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gpt-image-1"), "/v1")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.WriteField("prompt", "cat")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistMultipartDenied(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gpt-image-1"), "/v1")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "dall-e-3")
	_ = writer.WriteField("prompt", "cat")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistNoModelPassesThrough(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"input":"no model field"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when no model extractable, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistBodyRestoredForHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), allowlistAPIKey(true, "claude-sonnet-4.5"))
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	router.POST("/v1/messages", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Errorf("handler reread body: %v", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		if !strings.Contains(string(body), `"claude-sonnet-4.5"`) {
			t.Errorf("handler saw wrong body: %s", body)
		}
		c.Status(http.StatusOK)
	})

	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"model":"claude-sonnet-4.5","input":"hi"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistWebSocketUpgradeSkipsBodyExtraction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), allowlistAPIKey(true, "claude-sonnet-4.5"))
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	router.GET("/v1/responses", func(c *gin.Context) {
		c.Status(http.StatusSwitchingProtocols)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSwitchingProtocols {
		t.Fatalf("expected WS upgrade to reach handler, got %d: %s", w.Code, w.Body.String())
	}
}

// 伪造 WebSocket Upgrade 头的普通请求不得绕过白名单校验。
func TestGroupModelAllowlistForgedUpgradeHeaderDoesNotBypass(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-opus-4.6"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 despite forged upgrade headers, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "claude-opus-4.6") {
		t.Fatalf("expected blocked model in message, got %s", w.Body.String())
	}
	if len(*calls) != 0 {
		t.Fatalf("expected handler not to run, got %v", *calls)
	}
}

// Grok Realtime 升级请求的模型在查询参数里，携带 Upgrade 头也必须校验。
func TestGroupModelAllowlistRealtimeUpgradeStillChecksQueryModel(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "grok-4.6"), "/v1")

	req := httptest.NewRequest(http.MethodGet, "/v1/realtime?model=grok-4.20", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unlisted realtime model with upgrade headers, got %d: %s", w.Code, w.Body.String())
	}
}

// Live 入口的 handler 只读 session.model：准入必须以 session.model 为准，
// 与顶层 model 不一致时按实际执行模型校验。
func TestGroupModelAllowlistLiveUsesSessionModel(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gpt-realtime-model"), "/v1")

	// session.model 不在白名单：即使顶层 model 命中也必须拒绝。
	w := doJSON(t, router, http.MethodPost, "/v1/live", `{"model":"gpt-realtime-model","session":{"model":"blocked-model"},"sdp":"v=0"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when session.model is blocked, got %d: %s", w.Code, w.Body.String())
	}

	// session.model 命中：顶层 model 不在白名单也不影响（handler 不读它）。
	w = doJSON(t, router, http.MethodPost, "/v1/live", `{"model":"blocked-model","session":{"model":"gpt-realtime-model"},"sdp":"v=0"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when session.model is allowlisted, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupModelAllowlistMarksRejectionReasons(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), allowlistAPIKey(true, "claude-sonnet-4.5"))
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	router.POST("/v1/responses", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := doJSON(t, router, http.MethodPost, "/v1/responses", `{"model":"gpt-5.4"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	// IngressRejectModelNotAllowed 与业务限流原因由 ops 中间件在真实链路落日志，
	// 此处至少验证拒绝原因可从 context 读取（MarkIngressRejected 生效）。
}

func TestGroupModelAllowlistNilGroupPasses(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(&service.APIKey{}, "/v1")
	w := doJSON(t, router, http.MethodPost, "/v1/messages", `{"model":"whatever"}`)
	if w.Code != http.StatusOK || len(*calls) != 1 {
		t.Fatalf("expected passthrough for key without group, got %d", w.Code)
	}
}

// encoding/json 绑定系 handler（如批量生图 ShouldBindJSON）对键名大小写不敏感：
// {"Model":"blocked"} 会绑定为 blocked，准入必须拒绝。
func TestGroupModelAllowlistCaseVariantModelKeyRejected(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/responses", `{"Model":"claude-opus-4.6"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("case-variant model key must be validated, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "claude-opus-4.6") {
		t.Fatalf("expected blocked model in message, got %s", w.Body.String())
	}
	if len(*calls) != 0 {
		t.Fatalf("expected handler not to run, got %v", *calls)
	}
}

// encoding/json 绑定对重复键取末值、gjson 取首个：两个值都必须通过校验。
func TestGroupModelAllowlistDuplicateModelKeysRejected(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "claude-sonnet-4.5"), "/v1")

	w := doJSON(t, router, http.MethodPost, "/v1/responses", `{"model":"claude-sonnet-4.5","model":"claude-opus-4.6"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("duplicate model keys must all be validated, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "claude-opus-4.6") {
		t.Fatalf("expected the disallowed duplicate to be reported, got %s", w.Body.String())
	}
}

// 图片编辑的 multipart 解析器对重复 model 字段取末值：全部字段值都必须校验。
func TestGroupModelAllowlistMultipartDuplicateModelFieldsRejected(t *testing.T) {
	router, _ := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gpt-image-1"), "/v1")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.WriteField("model", "gpt-image-1.5")
	_ = writer.WriteField("prompt", "cat")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("duplicate multipart model fields must all be validated, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "gpt-image-1.5") {
		t.Fatalf("expected the disallowed field value to be reported, got %s", w.Body.String())
	}
}

// 全部候选都在白名单内时放行（重复但同值的字段不误伤）。
func TestGroupModelAllowlistDuplicateIdenticalModelsAllowed(t *testing.T) {
	router, calls := newGroupModelAllowlistTestRouter(allowlistAPIKey(true, "gpt-image-1"), "/v1")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("identical duplicate fields should pass, got %d: %s", w.Code, w.Body.String())
	}
	if len(*calls) != 1 {
		t.Fatalf("expected handler to run once, got %v", *calls)
	}
}
