package enginetrace

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestTelemetryLifecycle(t *testing.T) {
	var out bytes.Buffer
	trace := &Trace{writer: &out, record: Record{RequestID: "req1", AccountID: 27, RequestedModel: "sol", Status: "not_observed"}}
	trace.Observe([]byte(`{"type":"response.created","response":{"id":"resp1","model":"sol"}}`), "")
	trace.Observe([]byte(`{"type":"response.timing","timing_metrics":{"engine_ids":["gpt56lun-codex-a","gpt56sol-codex-b"]}}`), "")
	trace.Observe([]byte(`{"type":"response.completed","response":{"id":"resp1","model":"sol"}}`), "")
	trace.Close()
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d records: %s", len(lines), out.String())
	}
	var record Record
	if err := json.Unmarshal([]byte(lines[1]), &record); err != nil {
		t.Fatal(err)
	}
	if record.Status != "observed" || string(record.EngineIDs) != `["gpt56lun-codex-a","gpt56sol-codex-b"]` || record.ResponseID != "resp1" || record.ResponseModel != "sol" || record.AccountID != 27 {
		t.Fatalf("lost telemetry or association: %+v", record)
	}
	trace.Observe([]byte(`{"type":"response.created","response":{"id":"resp2","model":"sol"}}`), "")
	trace.Observe([]byte(`{"type":"response.completed","response":{"id":"resp2"}}`), "")
	lines = strings.Split(strings.TrimSpace(out.String()), "\n")
	if err := json.Unmarshal([]byte(lines[2]), &record); err != nil {
		t.Fatal(err)
	}
	if record.Status != "not_observed" || string(record.EngineIDs) != "null" {
		t.Fatalf("previous turn leaked: %+v", record)
	}
}

func TestFieldShapesAndPaths(t *testing.T) {
	for _, tc := range []struct{ value, status string }{
		{`"gpt56lun-codex-test"`, "observed"}, {`["a","b"]`, "observed"},
		{`null`, "empty"}, {`[]`, "empty"}, {`""`, "empty"}, {`[""]`, "empty"},
		{`123`, "invalid"}, {`["a",42]`, "invalid"}, {`{"x":1}`, "invalid"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			var out bytes.Buffer
			trace := &Trace{writer: &out}
			payload := []byte(`{"type":"response.completed","response":{"metadata":{"timing_metrics":{"engine_ids":` + tc.value + `}}}}`)
			before := string(payload)
			trace.Observe(payload, "")
			if trace.record.Status != tc.status || string(trace.record.EngineIDs) != tc.value || trace.record.FieldPath != "response.metadata.timing_metrics.engine_ids" {
				t.Fatalf("bad record: %+v", trace.record)
			}
			if string(payload) != before {
				t.Fatal("mutated response")
			}
		})
	}
}

func TestOutputCannotImpersonateTelemetry(t *testing.T) {
	var out bytes.Buffer
	trace := &Trace{writer: &out, record: Record{Status: "not_observed"}}
	trace.Observe([]byte(`{"type":"response.output_item.done","item":{"timing_metrics":{"engine_ids":"fake"},"text":"secret prompt"}}`), "")
	trace.Observe([]byte(`{"type":"response.completed","response":{"output":[{"timing_metrics":{"engine_ids":"fake"},"text":"secret answer"}]}}`), "")
	if trace.record.Status != "not_observed" || strings.Contains(out.String(), "secret") || strings.Contains(out.String(), "fake") {
		t.Fatalf("captured content: %s", out.String())
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errors.New("disk unavailable") }

func TestDisabledMalformedAndFailedLogging(t *testing.T) {
	t.Setenv("SUB2API_ENGINE_LOG", "")
	trace := New("", 0, "", "", "")
	if trace != nil {
		t.Fatal("tracing should be disabled")
	}
	trace.Observe(nil, "")
	trace.Close()
	trace = &Trace{writer: failedWriter{}, record: Record{Status: "not_observed"}}
	trace.Observe([]byte(`{"timing_metrics":`), "")
	trace.Observe([]byte(`{"type":"response.completed"}`), "")
	trace.Close()
}
