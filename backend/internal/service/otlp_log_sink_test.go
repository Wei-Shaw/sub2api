package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

type captureLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *captureLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range records {
		e.records = append(e.records, records[i].Clone())
	}
	return nil
}

func (*captureLogExporter) Shutdown(context.Context) error { return nil }

func (*captureLogExporter) ForceFlush(context.Context) error { return nil }

func (e *captureLogExporter) oneRecord(t *testing.T) sdklog.Record {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.records) != 1 {
		t.Fatalf("expected one exported record, got %d", len(e.records))
	}
	return e.records[0].Clone()
}

func TestOTLPLogSinkMapsAndRedactsEvent(t *testing.T) {
	exporter := &captureLogExporter{}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)),
	)
	sink := newOTLPLogSink(provider)
	t.Cleanup(func() {
		if err := sink.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown sink: %v", err)
		}
	})

	timestamp := time.Date(2026, time.August, 28, 8, 0, 0, 0, time.UTC)
	sink.WriteLogEvent(&logger.LogEvent{
		Time:       timestamp,
		Level:      "ERROR",
		LoggerName: "openai",
		Message:    "Using account upstream@example.com Proxy=socks5://user:pass@192.0.2.10:443",
		Fields: map[string]any{
			"component":  "gateway.forward",
			"request_id": "req-123",
			"email":      "upstream@example.com",
			"attempt":    2,
			"nested": map[string]any{
				"message": "retry upstream@example.com",
			},
		},
	})

	record := exporter.oneRecord(t)
	if !record.Timestamp().Equal(timestamp) {
		t.Fatalf("unexpected timestamp: %s", record.Timestamp())
	}
	if record.Severity() != otellog.SeverityError || record.SeverityText() != "error" {
		t.Fatalf("unexpected severity: %s (%d)", record.SeverityText(), record.Severity())
	}
	body := record.Body().AsString()
	for _, secret := range []string{"upstream@example.com", "user", "pass"} {
		if strings.Contains(body, secret) {
			t.Fatalf("body leaked %q: %q", secret, body)
		}
	}
	if !strings.Contains(body, "Proxy=***") {
		t.Fatalf("unexpected redacted body: %q", body)
	}

	attributes := recordAttributes(record)
	if got := attributes["request_id"].AsString(); got != "req-123" {
		t.Fatalf("unexpected request_id: %q", got)
	}
	if got := attributes["email"].AsString(); got != "***" {
		t.Fatalf("expected redacted email attribute, got %q", got)
	}
	if got := attributes["attempt"].AsInt64(); got != 2 {
		t.Fatalf("unexpected attempt: %d", got)
	}
	if got := attributes["nested"].AsMap()[0].Value.AsString(); strings.Contains(got, "upstream@example.com") {
		t.Fatalf("nested attribute leaked email: %q", got)
	}

	scope := record.InstrumentationScope()
	if scope.Name != "openai" {
		t.Fatalf("unexpected instrumentation scope: %q", scope.Name)
	}
	scopeAttrs := make(map[string]string)
	iter := scope.Attributes.Iter()
	for iter.Next() {
		kv := iter.Attribute()
		scopeAttrs[string(kv.Key)] = kv.Value.AsString()
	}
	if scopeAttrs["component"] != "gateway.forward" || scopeAttrs["logger.name"] != "openai" {
		t.Fatalf("unexpected scope attributes: %#v", scopeAttrs)
	}
}

func TestNewOTLPLogSinkFromEnvDisabledWithoutEndpoint(t *testing.T) {
	clearOTLPLogEnv(t)

	sink, err := NewOTLPLogSinkFromEnv(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink != nil {
		t.Fatal("expected OTLP sink to remain disabled")
	}
}

func TestNewOTLPLogSinkFromEnvRejectsUnsupportedProtocol(t *testing.T) {
	clearOTLPLogEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/json")

	sink, err := NewOTLPLogSinkFromEnv(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unsupported OTLP logs protocol") {
		t.Fatalf("expected unsupported protocol error, got sink=%v err=%v", sink, err)
	}
}

func TestOTLPLogProtocolUsesLogsSpecificValue(t *testing.T) {
	clearOTLPLogEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_PROTOCOL", "http/protobuf")

	if got := otlpLogProtocol(); got != "http/protobuf" {
		t.Fatalf("unexpected protocol: %q", got)
	}
}

func TestOTLPLogResourceServiceName(t *testing.T) {
	clearOTLPLogEnv(t)

	res, err := otlpLogResource(context.Background())
	if err != nil {
		t.Fatalf("create default resource: %v", err)
	}
	if got := resourceAttribute(res, "service.name"); got != defaultOTLPLogScope {
		t.Fatalf("unexpected default service name: %q", got)
	}

	t.Setenv("OTEL_SERVICE_NAME", "custom-gateway")
	res, err = otlpLogResource(context.Background())
	if err != nil {
		t.Fatalf("create configured resource: %v", err)
	}
	if got := resourceAttribute(res, "service.name"); got != "custom-gateway" {
		t.Fatalf("unexpected configured service name: %q", got)
	}
}

func recordAttributes(record sdklog.Record) map[string]otellog.Value {
	attributes := make(map[string]otellog.Value)
	record.WalkAttributes(func(kv otellog.KeyValue) bool {
		attributes[kv.Key] = kv.Value
		return true
	})
	return attributes
}

func resourceAttribute(res interface{ Attributes() []attribute.KeyValue }, key string) string {
	for _, kv := range res.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.AsString()
		}
	}
	return ""
}

func clearOTLPLogEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_EXPORTER_OTLP_LOGS_PROTOCOL",
		"OTEL_RESOURCE_ATTRIBUTES",
		"OTEL_SERVICE_NAME",
	} {
		t.Setenv(name, "")
	}
}
