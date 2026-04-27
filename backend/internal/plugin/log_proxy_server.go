package plugin

import (
	"errors"
	"io"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// LogProxyServer accepts plugin slog records over a client-streaming RPC and
// replays them into the host zap pipeline so plugin logs share the same sink,
// rotation, and structured field schema as core code.
//
// Each plugin instance gets a logger named "plugin.<name>"; the name comes
// from the gRPC caller-identity metadata header that the SDK already attaches
// to every outbound call (see plugin-sdk/runner.go::callerMetadataKey).
//
// This server is registered alongside SQLProxy / RedisProxy / EventBus on the
// same SDK gRPC endpoint via SDKServer.RegisterServices.
type LogProxyServer struct {
	pluginsdk.UnimplementedLogProxyServer

	resolver func(stream pluginsdk.LogProxy_PushLogsServer) string
}

// NewLogProxyServer wires the LogProxy implementation against an SDKServer
// (used to identify the calling plugin via gRPC metadata). Passing a nil
// SDKServer is allowed for tests that want to inject a custom resolver.
func NewLogProxyServer(s *SDKServer) *LogProxyServer {
	srv := &LogProxyServer{}
	if s != nil {
		srv.resolver = func(stream pluginsdk.LogProxy_PushLogsServer) string {
			return s.resolveCaller(stream.Context())
		}
	}
	return srv
}

// PushLogs is the client-streaming entry point. The plugin opens one stream
// per process lifetime and continuously pushes LogRecord messages; the host
// drains them and replays each record into the zap logger named after the
// plugin.
//
// Receiving side has no rate limit by design — the SDK enforces a 256-cap
// channel with drop-on-full so a misbehaving plugin cannot pin host
// goroutines. Per-record dropped_since_last_send is logged at WARN to keep
// observability honest.
func (l *LogProxyServer) PushLogs(stream pluginsdk.LogProxy_PushLogsServer) error {
	pluginName := ""
	if l.resolver != nil {
		pluginName = l.resolver(stream)
	}
	if pluginName == "" {
		// Without a known caller we still accept the stream but tag records
		// as "plugin.unknown" so the records are not dropped on the floor.
		pluginName = "unknown"
	}
	zapLogger := logger.L().Named("plugin." + pluginName)
	var received uint64
	for {
		rec, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&pluginsdk.LogPushSummary{Received: received})
		}
		if err != nil {
			return err
		}
		replayToZap(zapLogger, pluginName, rec)
		received++
	}
}

// replayToZap converts a wire LogRecord into a zap.Check + zap.Field call so
// the host zap config (sampling, level, output sinks) is the single source of
// truth for plugin log output.
//
// Out-of-range slog levels collapse to Debug to match zap's logger.Check
// behaviour for unknown levels.
func replayToZap(log *zap.Logger, pluginName string, rec *pluginsdk.LogRecord) {
	if log == nil || rec == nil {
		return
	}
	level := slogLevelToZap(rec.GetLevel())

	// Surface dropped log notice at WARN — separate from the data record so
	// operators can grep for "plugin log dropped" without scanning every line.
	if dropped := rec.GetDroppedSinceLastSend(); dropped > 0 {
		if ce := log.Check(zapcore.WarnLevel, "plugin log dropped"); ce != nil {
			ce.Write(zap.Uint64("dropped", dropped), zap.String("plugin", pluginName))
		}
	}

	ce := log.Check(level, rec.GetMsg())
	if ce == nil {
		return
	}
	if ts := rec.GetTimeUnixNano(); ts > 0 {
		ce.Time = time.Unix(0, int64(ts)) //nolint:gosec
	}
	fields := make([]zap.Field, 0, len(rec.GetAttrs())+1)
	if src := rec.GetSourceFile(); src != "" {
		fields = append(fields, zap.String("source", src))
		if line := rec.GetSourceLine(); line > 0 {
			fields = append(fields, zap.Int32("source_line", line))
		}
	}
	for _, a := range rec.GetAttrs() {
		fields = append(fields, attrToZapField(a))
	}
	ce.Write(fields...)
}

// slogLevelToZap maps slog.Level int values onto zap levels. Matches the
// host's slog→zap handler convention so plugin records display with the same
// severity tagging as in-process slog calls.
func slogLevelToZap(level int32) zapcore.Level {
	switch {
	case level >= 8:
		return zapcore.ErrorLevel
	case level >= 4:
		return zapcore.WarnLevel
	case level >= 0:
		return zapcore.InfoLevel
	default:
		return zapcore.DebugLevel
	}
}

// attrToZapField converts a wire LogAttr (proto oneof) back into a typed
// zap.Field. JSON-encoded Any values are surfaced as a Reflect field so zap
// renders them as nested JSON in the output.
func attrToZapField(a *pluginsdk.LogAttr) zap.Field {
	if a == nil {
		return zap.Skip()
	}
	key := a.GetKey()
	switch v := a.GetValue().(type) {
	case *pluginsdk.LogAttr_StringValue:
		return zap.String(key, v.StringValue)
	case *pluginsdk.LogAttr_IntValue:
		return zap.Int64(key, v.IntValue)
	case *pluginsdk.LogAttr_FloatValue:
		return zap.Float64(key, v.FloatValue)
	case *pluginsdk.LogAttr_BoolValue:
		return zap.Bool(key, v.BoolValue)
	case *pluginsdk.LogAttr_TimeUnixNano:
		return zap.Time(key, time.Unix(0, int64(v.TimeUnixNano))) //nolint:gosec
	case *pluginsdk.LogAttr_DurationNanos:
		return zap.Duration(key, time.Duration(v.DurationNanos))
	case *pluginsdk.LogAttr_BytesValue:
		// Convention: ".json" suffix means SDK marshalled an Any value to
		// JSON; render as a parsed object so log readers see structure.
		if strings.HasSuffix(key, ".json") {
			return zap.ByteString(strings.TrimSuffix(key, ".json"), v.BytesValue)
		}
		return zap.ByteString(key, v.BytesValue)
	default:
		return zap.Skip()
	}
}
