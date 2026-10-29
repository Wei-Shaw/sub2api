package conversationlog

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func fixture(enabled bool, limit int) *Recorder {
	r := &Recorder{queue: make(chan Record, 32), slots: make(chan struct{}, 32)}
	r.config.Store(&Config{Enabled: enabled, RetentionDays: 7, MaxBodyBytes: limit, RedactSensitive: true})
	return r
}

func authenticated(*gin.Context) (Identity, bool) {
	return Identity{UserID: 7, KeyID: 9, UserLabel: "test@example.test", KeyName: "test"}, true
}

func TestCaptureDoesNotChangeJSONResponseAndRedactsCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := fixture(true, 1<<20)
	engine := gin.New()
	engine.Use(r.Middleware(authenticated))
	wire := `{"choices":[{"message":{"role":"assistant","content":"你好","tool_calls":[{"type":"function","function":{"name":"lookup","arguments":"{}"}}]}}]}`
	engine.POST("/v1/chat/completions", func(c *gin.Context) { _, _ = io.ReadAll(c.Request.Body); c.Data(200, "application/json", []byte(wire)) })
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"test","messages":[{"role":"user","content":"previous"},{"role":"user","content":"本次问题"}],"api_key":"payload-secret"}`))
	req.Header.Set("Authorization", "Bearer header-secret")
	out := httptest.NewRecorder()
	engine.ServeHTTP(out, req)
	if out.Body.String() != wire {
		t.Fatal("wire response changed")
	}
	rec := <-r.queue
	if rec.Prompt != "本次问题" || rec.Answer != "你好" || rec.Outcome != "completed" || rec.UserID != 7 {
		t.Fatalf("bad capture: %+v", rec)
	}
	if strings.Contains(rec.RequestBody, "payload-secret") || strings.Contains(rec.RequestBody, "header-secret") || !strings.Contains(rec.RequestBody, "[redacted]") {
		t.Fatal("credentials persisted")
	}
	if !strings.Contains(string(rec.ToolCalls), "lookup") {
		t.Fatal("tool call missing")
	}
}

func TestSSEFragmentationAndFlushArePreserved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := fixture(true, 1<<20)
	engine := gin.New()
	engine.Use(r.Middleware(authenticated))
	wire := "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\ndata: [DONE]\n\n"
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.Header("Content-Type", "text/event-stream")
		for _, b := range []byte(wire) {
			_, _ = c.Writer.Write([]byte{b})
			c.Writer.Flush()
		}
	})
	out := httptest.NewRecorder()
	engine.ServeHTTP(out, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"test","stream":true}`)))
	if out.Body.String() != wire || !out.Flushed {
		t.Fatal("SSE/flush changed")
	}
	rec := <-r.queue
	if rec.Answer != "你好" || rec.Outcome != "completed" {
		t.Fatalf("bad stream capture: %+v", rec)
	}
}

func TestSSEIsDeliveredBeforeHandlerFinishes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := fixture(true, 1<<20)
	engine := gin.New()
	engine.Use(r.Middleware(authenticated))
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	engine.POST("/v1/responses", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.WriteString("data: {\"type\":\"response.output_text.delta\",\"delta\":\"first\"}\n\n")
		c.Writer.Flush()
		<-release
	})
	server := httptest.NewServer(engine)
	defer func() { unblock(); server.Close() }()
	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Post(server.URL+"/v1/responses", "application/json", strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := make([]byte, 5)
	if _, err = io.ReadFull(res.Body, buf); err != nil || string(buf) != "data:" {
		t.Fatalf("first event delayed: %s %v", buf, err)
	}
	// Unblock handler before server.Close (which otherwise waits for this request).
	unblock()
}

func TestDisabledUnauthenticatedAndUnsupportedRequestsAreNotSaved(t *testing.T) {
	for _, tt := range []struct {
		name    string
		enabled bool
		path    string
		auth    bool
	}{{"off", false, "/v1/chat/completions", true}, {"unauth", true, "/v1/chat/completions", false}, {"admin", true, "/api/v1/auth/login", true}, {"websocket", true, "/v1/responses-ws", true}} {
		t.Run(tt.name, func(t *testing.T) {
			r := fixture(tt.enabled, 1024)
			e := gin.New()
			e.Use(r.Middleware(func(c *gin.Context) (Identity, bool) {
				if !tt.auth {
					return Identity{}, false
				}
				return authenticated(c)
			}))
			e.POST(tt.path, func(c *gin.Context) { _, _ = io.ReadAll(c.Request.Body); c.String(200, "ok") })
			e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", tt.path, strings.NewReader(`{}`)))
			if len(r.queue) != 0 {
				t.Fatal("unexpected saved record")
			}
		})
	}
}

func TestTruncationAndCancellation(t *testing.T) {
	r := fixture(true, 64)
	e := gin.New()
	e.Use(r.Middleware(authenticated))
	e.POST("/v1/responses", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.Data(200, "application/json", []byte(`{"output":[{"content":[{"type":"output_text","text":"`+strings.Repeat("x", 300)+`"}]}]}`))
	})
	out := httptest.NewRecorder()
	e.ServeHTTP(out, httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"input":"`+strings.Repeat("x", 300)+`"}`)))
	v := <-r.queue
	if !v.RequestTruncated || !v.ResponseTruncated || len(out.Body.Bytes()) <= 64 {
		t.Fatal("capture limit affected wire or was not marked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	r2 := fixture(true, 1024)
	e2 := gin.New()
	e2.Use(r2.Middleware(authenticated))
	e2.POST("/v1/responses", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.WriteString("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
		cancel()
	})
	e2.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{}`)).WithContext(ctx))
	v = <-r2.queue
	if v.Outcome != "interrupted" || v.Answer != "partial" {
		t.Fatalf("cancellation lost: %+v", v)
	}
}

func TestProtocolParsers(t *testing.T) {
	cases := []struct {
		name, body, want string
		stream           bool
		terminal, failed bool
	}{
		{"responses-json", `{"output":[{"type":"message","content":[{"type":"output_text","text":"reply"}]}]}`, "reply", false, true, false},
		{"responses-json-null-error", `{"status":"completed","error":null,"output":[{"type":"message","content":[{"type":"output_text","text":"reply"}]}]}`, "reply", false, true, false},
		{"responses-json-real-error", `{"error":{"message":"failed"}}`, "", false, true, true},
		{"chat-json-null-error", `{"error":null,"choices":[{"message":{"content":"reply"}}]}`, "reply", false, true, false},
		{"messages-json", `{"content":[{"type":"text","text":"reply"}]}`, "reply", false, true, false},
		{"responses-sse", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"reply\"}\r\n\r\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"content\":[{\"type\":\"output_text\",\"text\":\"reply\"}]}]}}\n\n", "reply", true, true, false},
		{"messages-sse", "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"reply\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n", "reply", true, true, false},
		{"stream-error", "data: {\"type\":\"error\",\"error\":{\"message\":\"failed\"}}\n\n", "", true, true, true},
		{"incomplete", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n", "partial", true, false, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, _, term, failed := parseResponse([]byte(tt.body), tt.stream, true)
			if got != tt.want || term != tt.terminal || failed != tt.failed {
				t.Fatalf("%q %v %v", got, term, failed)
			}
		})
	}
	_, calls, _, _ := parseResponse([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call1\",\"function\":{\"name\":\"read\",\"arguments\":\"{\"}}]}}]}\n\ndata: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"}\"}}]}}]}\n\ndata: [DONE]\n\n"), true, true)
	var result []struct {
		Function struct{ Name, Arguments string }
	}
	if json.Unmarshal(calls, &result) != nil || len(result) != 1 || result[0].Function.Arguments != "{}" {
		t.Fatalf("tool fragments not merged: %s", calls)
	}
}

func TestResponsesNullErrorIsRecordedAsCompleted(t *testing.T) {
	r := fixture(true, 1024)
	e := gin.New()
	e.Use(r.Middleware(authenticated))
	wire := `{"object":"response","status":"completed","error":null,"incomplete_details":null,"output":[{"type":"message","content":[{"type":"output_text","text":"OK"}]}]}`
	e.POST("/v1/responses", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.Data(200, "application/json", []byte(wire))
	})
	out := httptest.NewRecorder()
	e.ServeHTTP(out, httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"test","input":"Reply only OK.","stream":false}`)))
	rec := <-r.queue
	if rec.Outcome != "completed" || rec.Answer != "OK" || rec.StatusCode != 200 || out.Body.String() != wire {
		t.Fatalf("successful Responses reply misclassified: %+v", rec)
	}
}

func TestCapacitySkipsWithoutBlocking(t *testing.T) {
	r := fixture(true, 1024)
	for i := 0; i < 32; i++ {
		r.queue <- Record{}
	}
	r.enqueue(Record{})
	if r.dropped.Load() != 1 {
		t.Fatal("queue saturation was not counted")
	}
}

func TestDatabaseWriterAndShutdownDrain(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT enabled,retention_days,max_body_bytes,redact_sensitive").WillReturnRows(sqlmock.NewRows([]string{"enabled", "retention_days", "max_body_bytes", "redact_sensitive"}).AddRow(true, 7, 1048576, true))
	mock.ExpectExec("INSERT INTO conversation_logs").WillReturnResult(sqlmock.NewResult(1, 1))
	r := NewRecorder(db)
	r.enqueue(Record{ToolCalls: json.RawMessage(`[]`)})
	r.Close()
	if r.saved.Load() != 1 {
		t.Fatal("shutdown did not drain pending writes")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigValidationAndFilters(t *testing.T) {
	if (Config{RetentionDays: 0, MaxBodyBytes: 1048576}).validate() == nil {
		t.Fatal("invalid retention accepted")
	}
	if (Config{RetentionDays: 7, MaxBodyBytes: 1 << 30}).validate() == nil {
		t.Fatal("unbounded capture accepted")
	}
	w, args := where(Filters{UserID: 7, Model: "' OR TRUE --", Search: "secret", Outcome: "completed"})
	if len(args) != 4 || strings.Contains(w, "' OR TRUE --") {
		t.Fatal("SQL filters are not parameterized")
	}
}

func TestEscapedNULIsSafeForPostgresWithoutChangingWire(t *testing.T) {
	r := fixture(true, 1024)
	e := gin.New()
	e.Use(r.Middleware(authenticated))
	wire := `{"choices":[{"message":{"content":"before\u0000after"}}]}`
	e.POST("/v1/chat/completions", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.Data(200, "application/json", []byte(wire))
	})
	out := httptest.NewRecorder()
	e.ServeHTTP(out, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"a\u0000b"}]}`)))
	rec := <-r.queue
	if out.Body.String() != wire {
		t.Fatal("normalization changed the client response")
	}
	if rec.Answer != "before\uFFFDafter" || rec.Prompt != "a\uFFFDb" || strings.Contains(rec.ResponseBody, `\u0000`) {
		t.Fatalf("NUL was not normalized: %+v", rec)
	}
	var value any
	if json.Unmarshal([]byte(sanitizeJSON([]byte(`{"a\u0000b":"x"}`), true)), &value) != nil {
		t.Fatal("normalized JSON is invalid")
	}
}
