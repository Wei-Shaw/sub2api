package conversationlog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestCaptureRedactionModesJSONAndSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mask := range []bool{true, false} {
		for _, stream := range []bool{true, false} {
			t.Run(fmt.Sprintf("redact=%v/stream=%v", mask, stream), func(t *testing.T) {
				r := fixture(true, 1<<20)
				cfg := *r.config.Load()
				cfg.RedactSensitive = mask
				r.config.Store(&cfg)
				engine := gin.New()
				engine.Use(r.Middleware(authenticated))
				wire := `{"content":[{"type":"text","text":"OK"},{"type":"tool_use","id":"fixture","name":"fixture","input":{"password":"synthetic-response-secret"}}]}`
				if stream {
					wire = "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"fixture\",\"name\":\"fixture\",\"input\":{\"password\":\"synthetic-response-secret\"}}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
				}
				engine.POST("/v1/messages", func(c *gin.Context) {
					_, _ = io.ReadAll(c.Request.Body)
					kind := "application/json"
					if stream {
						kind = "text/event-stream"
					}
					c.Data(200, kind, []byte(wire))
				})
				body := fmt.Sprintf(`{"messages":[{"role":"user","content":"plain prompt"}],"stream":%v,"metadata":{"api_key":"synthetic-request-secret","nested":{"password":"synthetic-request-secret"}},"image":{"data":"synthetic-inline-binary"}}`, stream)
				req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer synthetic-header-secret")
				out := httptest.NewRecorder()
				engine.ServeHTTP(out, req)
				rec := <-r.queue
				if rec.Redacted != mask || rec.Outcome != "completed" || out.Body.String() != wire {
					t.Fatal("capture mode, outcome or wire response changed")
				}
				for name, value := range map[string]string{"request": rec.RequestBody, "response": rec.ResponseBody, "tools": string(rec.ToolCalls)} {
					secret := "synthetic-response-secret"
					if name == "request" {
						secret = "synthetic-request-secret"
					}
					if strings.Contains(value, secret) == mask {
						t.Fatalf("%s did not follow redaction mode %v", name, mask)
					}
				}
				serialized, _ := json.Marshal(rec)
				if strings.Contains(string(serialized), "synthetic-header-secret") || strings.Contains(string(serialized), "synthetic-inline-binary") {
					t.Fatal("excluded header or inline binary was captured")
				}
			})
		}
	}
}

func TestCaptureKeepsTheRedactionModeAtRequestStart(t *testing.T) {
	r := fixture(true, 1<<20)
	initial := *r.config.Load()
	initial.RedactSensitive = false
	r.config.Store(&initial)
	engine := gin.New()
	engine.Use(r.Middleware(authenticated))
	engine.POST("/v1/responses", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		next := *r.config.Load()
		next.RedactSensitive = true
		r.config.Store(&next)
		c.Data(200, "application/json", []byte(`{"output":[],"error":null}`))
	})
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"input":"plain","password":"synthetic-inflight-secret"}`)))
	rec := <-r.queue
	if rec.Redacted || !strings.Contains(rec.RequestBody, "synthetic-inflight-secret") {
		t.Fatal("in-flight capture changed mode after a config update")
	}
}

func TestConfigUpdatePreservesOmittedRedactionAndAcceptsExplicitToggle(t *testing.T) {
	for _, tt := range []struct {
		name            string
		initial, wanted bool
		body            string
	}{
		{"legacy-on", true, true, `{"enabled":true,"retention_days":7,"max_body_bytes":1048576}`},
		{"legacy-off", false, false, `{"enabled":true,"retention_days":7,"max_body_bytes":1048576}`},
		{"explicit-off", true, false, `{"enabled":true,"retention_days":7,"max_body_bytes":1048576,"redact_sensitive":false}`},
		{"explicit-on", false, true, `{"enabled":true,"retention_days":7,"max_body_bytes":1048576,"redact_sensitive":true}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			r := fixture(true, 1<<20)
			cfg := *r.config.Load()
			cfg.RedactSensitive = tt.initial
			r.config.Store(&cfg)
			r.db = db
			mock.ExpectExec("UPDATE conversation_log_config SET").WithArgs(true, 7, 1048576, tt.wanted).WillReturnResult(sqlmock.NewResult(0, 1))
			engine := gin.New()
			r.RegisterAdmin(engine.Group("/api/v1/admin"))
			out := httptest.NewRecorder()
			engine.ServeHTTP(out, httptest.NewRequest("PUT", "/api/v1/admin/conversation-logs/config", strings.NewReader(tt.body)))
			if out.Code != 200 || r.config.Load().RedactSensitive != tt.wanted {
				t.Fatal("configuration did not persist the requested mode")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
