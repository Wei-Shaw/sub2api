// Package enginetrace records upstream engine telemetry without interpreting model identity.
package enginetrace

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"gopkg.in/natefinch/lumberjack.v2"
)

var outputOnce sync.Once
var output io.Writer

type Record struct {
	Time           string          `json:"time"`
	RequestID      string          `json:"request_id"`
	AccountID      int64           `json:"account_id"`
	RequestedModel string          `json:"requested_model"`
	UpstreamModel  string          `json:"upstream_model"`
	Transport      string          `json:"transport"`
	ResponseID     string          `json:"response_id"`
	ResponseModel  string          `json:"response_model"`
	EventType      string          `json:"event_type"`
	Status         string          `json:"status"`
	EngineIDs      json.RawMessage `json:"engine_ids"`
	FieldPath      string          `json:"field_path,omitempty"`
}

// A Trace belongs to one upstream attempt, or to a serial WebSocket relay.
type Trace struct {
	writer   io.Writer
	record   Record
	terminal bool
}

func New(requestID string, accountID int64, requestedModel, upstreamModel, transport string) *Trace {
	path := os.Getenv("SUB2API_ENGINE_LOG")
	if path == "" {
		return nil
	}
	outputOnce.Do(func() {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			log.Printf("engine trace: cannot create log directory: %v", err)
			return
		}
		output = &lumberjack.Logger{Filename: path, MaxSize: 16, MaxBackups: 7, MaxAge: 14, Compress: true}
	})
	if output == nil {
		return nil
	}
	return &Trace{writer: output, record: Record{
		RequestID: requestID, AccountID: accountID, RequestedModel: requestedModel,
		UpstreamModel: upstreamModel, Transport: transport, Status: "not_observed",
	}}
}

// Models updates the request metadata for the current turn of a persistent relay.
func (t *Trace) Models(requested, upstream string) {
	if t != nil {
		t.record.RequestedModel, t.record.UpstreamModel = requested, upstream
	}
}

func (t *Trace) Observe(payload []byte, eventType string) {
	if t == nil {
		return
	}
	if eventType == "" {
		eventType = gjson.GetBytes(payload, "type").String()
	}
	// Never inspect output items, deltas, tool arguments or arbitrary nested JSON.
	if strings.HasSuffix(eventType, ".delta") || strings.HasPrefix(eventType, "response.output_item.") || strings.HasPrefix(eventType, "response.function_call_arguments.") {
		return
	}
	if !gjson.ValidBytes(payload) {
		return
	}
	if eventType == "response.created" && t.terminal {
		t.record.ResponseID, t.record.ResponseModel, t.record.FieldPath = "", "", ""
		t.record.EngineIDs, t.record.Status, t.terminal = nil, "not_observed", false
	}
	for _, path := range []string{"response.id", "response_id", "id"} {
		if value := gjson.GetBytes(payload, path); value.Type == gjson.String && value.Str != "" {
			t.record.ResponseID = value.Str
			break
		}
	}
	for _, path := range []string{"response.model", "model"} {
		if value := gjson.GetBytes(payload, path); value.Type == gjson.String && value.Str != "" {
			t.record.ResponseModel = value.Str
			break
		}
	}
	found := false
	for _, path := range []string{
		"timing_metrics.engine_ids", "response.timing_metrics.engine_ids",
		"metadata.timing_metrics.engine_ids", "response.metadata.timing_metrics.engine_ids",
	} {
		value := gjson.GetBytes(payload, path)
		if !value.Exists() {
			continue
		}
		found = true
		t.record.EngineIDs = append(json.RawMessage(nil), value.Raw...)
		t.record.FieldPath = path
		t.record.Status = "invalid"
		if value.Type == gjson.Null {
			t.record.Status = "empty"
		} else if value.Type == gjson.String {
			t.record.Status = "observed"
			if strings.TrimSpace(value.Str) == "" {
				t.record.Status = "empty"
			}
		} else if value.IsArray() {
			t.record.Status = "empty"
			for _, id := range value.Array() {
				if id.Type != gjson.String {
					t.record.Status = "invalid"
					break
				}
				if strings.TrimSpace(id.Str) != "" {
					t.record.Status = "observed"
				}
			}
		}
		t.write(eventType)
	}
	switch eventType {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
		t.terminal = true
		if !found {
			t.write(eventType)
		}
	}
}

// Close records incomplete streams too; absence is never treated as a healthy model.
func (t *Trace) Close() {
	if t != nil && !t.terminal {
		t.write("stream_closed")
		t.terminal = true
	}
}

func (t *Trace) write(eventType string) {
	t.record.Time = time.Now().UTC().Format(time.RFC3339Nano)
	t.record.EventType = eventType
	data, err := json.Marshal(t.record)
	if err == nil {
		_, err = t.writer.Write(append(data, '\n'))
	}
	if err != nil {
		log.Printf("engine trace: log write failed: %v", err)
	}
}
