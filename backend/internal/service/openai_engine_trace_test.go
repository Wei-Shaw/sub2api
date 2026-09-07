package service

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/enginetrace"
	"github.com/gin-gonic/gin"
)

func TestOpenAIEngineTrace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engine.jsonl")
	t.Setenv("SUB2API_ENGINE_LOG", path)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/responses", nil)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.RequestID, "request-test"))
	o := &upstreamResponseModelObserver{engineTrace: newOpenAIEngineTrace(c, &Account{ID: 27, Platform: PlatformOpenAI}, "sol", "sol", "sse")}
	o.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"id":"resp-test","model":"sol"},"timing_metrics":{"engine_ids":["gpt56lun-codex-test"]}}`), "response.completed")
	o.engineTrace.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record enginetrace.Record
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.RequestID != "request-test" || record.AccountID != 27 || record.Status != "observed" || string(record.EngineIDs) != `["gpt56lun-codex-test"]` {
		t.Fatalf("bad record: %+v", record)
	}
	if o.Model() != "sol" || o.Conflict() {
		t.Fatal("telemetry affected response-model observation")
	}
}
