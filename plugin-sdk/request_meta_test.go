package pluginsdk

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestMetadata_FullHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set(HeaderPluginUserID, "42")
	r.Header.Set(HeaderPluginUserRole, "admin")
	r.Header.Set(HeaderPluginAPIKeyID, "1234")
	r.Header.Set(HeaderPluginClientIP, "203.0.113.5")
	r.Header.Set(HeaderPluginName, "channel-management")
	r.Header.Set(HeaderPluginRequestID, "req-uuid")
	r.Header.Set(HeaderTraceparent, "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01")

	m := RequestMetadata(r)

	if m.UserID != 42 || m.UserRole != "admin" {
		t.Fatalf("user fields: %+v", m)
	}
	if m.APIKeyID != 1234 {
		t.Fatalf("api key id: %+v", m)
	}
	if m.PluginName != "channel-management" {
		t.Fatalf("plugin name: %q", m.PluginName)
	}
	if m.RequestID != "req-uuid" {
		t.Fatalf("request id: %q", m.RequestID)
	}
	if m.ClientIP != "203.0.113.5" {
		t.Fatalf("client ip: %q", m.ClientIP)
	}
	if m.TraceID != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || m.SpanID != "bbbbbbbbbbbbbbbb" {
		t.Fatalf("trace fields: %+v", m)
	}
}

func TestRequestMetadata_MissingHeadersReturnsZero(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	m := RequestMetadata(r)
	if m.UserID != 0 || m.APIKeyID != 0 {
		t.Fatalf("ids should be zero, got %+v", m)
	}
	if m.TraceID != "" || m.SpanID != "" || m.RequestID != "" {
		t.Fatalf("string fields should be empty, got %+v", m)
	}
}

func TestRequestMetadata_NilRequest(t *testing.T) {
	m := RequestMetadata(nil)
	if (m != RequestMeta{}) {
		t.Fatalf("nil req should return zero meta")
	}
}

func TestRequestMetadata_MalformedNumericHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set(HeaderPluginUserID, "not-a-number")
	r.Header.Set(HeaderPluginAPIKeyID, "abc")

	m := RequestMetadata(r)
	if m.UserID != 0 || m.APIKeyID != 0 {
		t.Fatalf("malformed ids should fall back to 0, got %+v", m)
	}
}

func TestRequestMetadata_MalformedTraceparent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set(HeaderTraceparent, "garbage")
	m := RequestMetadata(r)
	if m.TraceID != "" || m.SpanID != "" {
		t.Fatalf("malformed traceparent should yield empty trace ids, got %+v", m)
	}
}

func TestLoggerWithRequest_TagsRecord(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))
	meta := RequestMeta{
		TraceID:   "tr",
		RequestID: "rid",
		UserID:    7,
		APIKeyID:  9,
	}
	logger := LoggerWithRequest(base, meta)
	logger.Info("hello")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	if record["trace_id"] != "tr" || record["request_id"] != "rid" {
		t.Fatalf("missing trace/request id: %v", record)
	}
	if v, ok := record["user_id"].(float64); !ok || int64(v) != 7 {
		t.Fatalf("user id wrong: %v", record["user_id"])
	}
}

func TestLoggerWithRequest_AnonymousReturnsBase(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))
	logger := LoggerWithRequest(base, RequestMeta{})
	if logger != base {
		t.Fatalf("anonymous request should return base logger unchanged")
	}
}

func TestLoggerWithRequest_NilBaseFallsBack(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil base should not panic: %v", r)
		}
	}()
	logger := LoggerWithRequest(nil, RequestMeta{TraceID: "tr"})
	if logger == nil {
		t.Fatalf("logger should not be nil")
	}
}
