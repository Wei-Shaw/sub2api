package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

const defaultOTLPLogScope = "sub2api"

// OTLPLogSink exports application LogEvents through the OpenTelemetry logs SDK.
// The SDK batch processor keeps network I/O off the application logging path.
type OTLPLogSink struct {
	provider *sdklog.LoggerProvider
	loggers  sync.Map // map[string]otellog.Logger
}

// NewOTLPLogSinkFromEnv creates an OTLP sink only when a logs-specific or
// generic OTLP endpoint is configured. Exporter options are read from the
// standard OpenTelemetry environment variables.
func NewOTLPLogSinkFromEnv(ctx context.Context) (*OTLPLogSink, error) {
	if !otlpLogEndpointConfigured() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := otlpLogResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP log resource: %w", err)
	}

	exporter, err := newOTLPLogExporter(ctx, otlpLogProtocol())
	if err != nil {
		return nil, err
	}
	processor := sdklog.NewBatchProcessor(exporter)
	return newOTLPLogSink(sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(processor),
	)), nil
}

func newOTLPLogSink(provider *sdklog.LoggerProvider) *OTLPLogSink {
	return &OTLPLogSink{provider: provider}
}

func otlpLogEndpointConfigured() bool {
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != ""
}

func otlpLogProtocol() string {
	if protocol := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_LOGS_PROTOCOL")); protocol != "" {
		return strings.ToLower(protocol)
	}
	if protocol := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")); protocol != "" {
		return strings.ToLower(protocol)
	}
	return "http/protobuf"
}

func otlpLogResource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(
		ctx,
		resource.WithAttributes(attribute.String("service.name", defaultOTLPLogScope)),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
}

func newOTLPLogExporter(ctx context.Context, protocol string) (sdklog.Exporter, error) {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "grpc":
		exporter, err := otlploggrpc.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("create OTLP/gRPC log exporter: %w", err)
		}
		return exporter, nil
	case "", "http/protobuf":
		exporter, err := otlploghttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("create OTLP/HTTP log exporter: %w", err)
		}
		return exporter, nil
	default:
		return nil, fmt.Errorf("unsupported OTLP logs protocol %q (supported: grpc, http/protobuf)", protocol)
	}
}

func (s *OTLPLogSink) WriteLogEvent(event *logger.LogEvent) {
	if s == nil || s.provider == nil || event == nil {
		return
	}

	levelText := strings.ToLower(strings.TrimSpace(event.Level))
	if levelText == "" {
		levelText = "info"
	}
	record := otellog.Record{}
	timestamp := event.Time
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	record.SetTimestamp(timestamp)
	record.SetObservedTimestamp(time.Now())
	record.SetSeverityText(levelText)
	record.SetSeverity(otelSeverity(levelText))
	record.SetBody(otellog.StringValue(logredact.RedactText(event.Message)))
	record.AddAttributes(otelLogAttributes(event)...)

	s.loggerFor(event).Emit(context.Background(), record)
}

func (s *OTLPLogSink) loggerFor(event *logger.LogEvent) otellog.Logger {
	component := otlpEventComponent(event)
	loggerName := strings.TrimSpace(event.LoggerName)
	scopeName := loggerName
	if scopeName == "" {
		scopeName = component
	}
	if scopeName == "" {
		scopeName = defaultOTLPLogScope
	}

	cacheKey := scopeName + "\x00" + component + "\x00" + loggerName
	if cached, ok := s.loggers.Load(cacheKey); ok {
		if cachedLogger, valid := cached.(otellog.Logger); valid {
			return cachedLogger
		}
		s.loggers.Delete(cacheKey)
	}

	scopeAttributes := make([]attribute.KeyValue, 0, 2)
	if component != "" {
		scopeAttributes = append(scopeAttributes, attribute.String("component", component))
	}
	if loggerName != "" {
		scopeAttributes = append(scopeAttributes, attribute.String("logger.name", loggerName))
	}
	created := s.provider.Logger(scopeName, otellog.WithInstrumentationAttributes(scopeAttributes...))
	actual, _ := s.loggers.LoadOrStore(cacheKey, created)
	if actualLogger, ok := actual.(otellog.Logger); ok {
		return actualLogger
	}
	return created
}

func (s *OTLPLogSink) Shutdown(ctx context.Context) error {
	if s == nil || s.provider == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.provider.Shutdown(ctx)
}

func otelSeverity(level string) otellog.Severity {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace":
		return otellog.SeverityTrace
	case "debug":
		return otellog.SeverityDebug
	case "warn", "warning":
		return otellog.SeverityWarn
	case "error":
		return otellog.SeverityError
	case "dpanic", "panic", "fatal":
		return otellog.SeverityFatal
	case "info":
		return otellog.SeverityInfo
	default:
		return otellog.SeverityUndefined
	}
}

func otelLogAttributes(event *logger.LogEvent) []otellog.KeyValue {
	fields := logredact.RedactMap(event.Fields)
	if component := otlpEventComponent(event); component != "" {
		if _, exists := fields["component"]; !exists {
			fields["component"] = component
		}
	}
	if loggerName := strings.TrimSpace(event.LoggerName); loggerName != "" {
		if _, exists := fields["logger.name"]; !exists {
			fields["logger.name"] = loggerName
		}
	}

	canonicalFields := make(map[string]any, len(fields))
	keys := make([]string, 0, len(fields))
	for key, value := range fields {
		if key = strings.TrimSpace(key); key != "" {
			if _, exists := canonicalFields[key]; !exists {
				keys = append(keys, key)
			}
			canonicalFields[key] = value
		}
	}
	sort.Strings(keys)

	attributes := make([]otellog.KeyValue, 0, len(keys))
	for _, key := range keys {
		attributes = append(attributes, otellog.KeyValue{Key: key, Value: otelLogValue(canonicalFields[key])})
	}
	return attributes
}

func otlpEventComponent(event *logger.LogEvent) string {
	if event == nil {
		return ""
	}
	component := strings.TrimSpace(event.Component)
	if event.Fields != nil {
		if fieldComponent, ok := event.Fields["component"].(string); ok && strings.TrimSpace(fieldComponent) != "" {
			component = strings.TrimSpace(fieldComponent)
		}
	}
	return component
}

func otelLogValue(value any) otellog.Value {
	switch v := value.(type) {
	case nil:
		return otellog.Value{}
	case bool:
		return otellog.BoolValue(v)
	case string:
		return otellog.StringValue(logredact.RedactText(v))
	case []byte:
		return otellog.StringValue(logredact.RedactText(string(v)))
	case int:
		return otellog.IntValue(v)
	case int8:
		return otellog.Int64Value(int64(v))
	case int16:
		return otellog.Int64Value(int64(v))
	case int32:
		return otellog.Int64Value(int64(v))
	case int64:
		return otellog.Int64Value(v)
	case uint:
		return unsignedOTelLogValue(uint64(v))
	case uint8:
		return otellog.Int64Value(int64(v))
	case uint16:
		return otellog.Int64Value(int64(v))
	case uint32:
		return otellog.Int64Value(int64(v))
	case uint64:
		return unsignedOTelLogValue(v)
	case float32:
		return otellog.Float64Value(float64(v))
	case float64:
		return otellog.Float64Value(v)
	case json.Number:
		if integer, err := v.Int64(); err == nil {
			return otellog.Int64Value(integer)
		}
		if decimal, err := v.Float64(); err == nil {
			return otellog.Float64Value(decimal)
		}
		return otellog.StringValue(v.String())
	case time.Time:
		return otellog.StringValue(v.UTC().Format(time.RFC3339Nano))
	case error:
		return otellog.StringValue(logredact.RedactText(v.Error()))
	case []any:
		items := make([]otellog.Value, 0, len(v))
		for _, item := range v {
			items = append(items, otelLogValue(item))
		}
		return otellog.SliceValue(items...)
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		items := make([]otellog.KeyValue, 0, len(keys))
		for _, key := range keys {
			items = append(items, otellog.KeyValue{Key: key, Value: otelLogValue(v[key])})
		}
		return otellog.MapValue(items...)
	default:
		if encoded, err := json.Marshal(v); err == nil {
			return otellog.StringValue(logredact.RedactJSON(encoded))
		}
		return otellog.StringValue(logredact.RedactText(fmt.Sprint(v)))
	}
}

func unsignedOTelLogValue(value uint64) otellog.Value {
	if value <= math.MaxInt64 {
		return otellog.Int64Value(int64(value))
	}
	return otellog.StringValue(fmt.Sprintf("%d", value))
}
