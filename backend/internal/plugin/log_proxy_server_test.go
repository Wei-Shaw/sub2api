//go:build unit

package plugin

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// TestReplayToZap_RoundTripsAttrs verifies the host-side replay surfaces all
// proto attr kinds as the matching zap.Field type. This guards against drift
// when proto fields are added/renamed.
func TestReplayToZap_RoundTripsAttrs(t *testing.T) {
	core, recorded := observer.New(zapcore.DebugLevel)
	log := zap.New(core)

	now := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	rec := &pluginsdk.LogRecord{
		TimeUnixNano: uint64(now.UnixNano()),
		Level:        int32(0), // slog Info
		Msg:          "hello",
		Attrs: []*pluginsdk.LogAttr{
			{Key: "trace_id", Value: &pluginsdk.LogAttr_StringValue{StringValue: "abc"}},
			{Key: "count", Value: &pluginsdk.LogAttr_IntValue{IntValue: 42}},
			{Key: "ratio", Value: &pluginsdk.LogAttr_FloatValue{FloatValue: 0.75}},
			{Key: "ok", Value: &pluginsdk.LogAttr_BoolValue{BoolValue: true}},
			{Key: "happened_at", Value: &pluginsdk.LogAttr_TimeUnixNano{TimeUnixNano: uint64(now.UnixNano())}},
			{Key: "elapsed", Value: &pluginsdk.LogAttr_DurationNanos{DurationNanos: int64(250 * time.Millisecond)}},
			{Key: "data.json", Value: &pluginsdk.LogAttr_BytesValue{BytesValue: []byte(`{"x":1}`)}},
		},
	}

	replayToZap(log, "channel-management", rec)

	all := recorded.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 record, got %d", len(all))
	}
	entry := all[0]
	if entry.Message != "hello" {
		t.Fatalf("msg = %q", entry.Message)
	}
	fields := entry.ContextMap()
	if fields["trace_id"] != "abc" {
		t.Fatalf("trace_id wrong: %v", fields)
	}
	if fields["count"].(int64) != 42 {
		t.Fatalf("count wrong: %v", fields["count"])
	}
	if fields["ratio"].(float64) != 0.75 {
		t.Fatalf("ratio wrong: %v", fields["ratio"])
	}
	if fields["ok"].(bool) != true {
		t.Fatalf("ok wrong: %v", fields["ok"])
	}
	if fields["happened_at"] == nil {
		t.Fatalf("happened_at missing")
	}
	if fields["elapsed"].(time.Duration) != 250*time.Millisecond {
		t.Fatalf("elapsed wrong: %v", fields["elapsed"])
	}
	// Bytes with .json suffix should be surfaced under the bare key.
	if fields["data"] == nil {
		t.Fatalf("data should be present (json suffix stripped), got: %v", fields)
	}
}

func TestReplayToZap_LevelMapping(t *testing.T) {
	cases := []struct {
		slog int32
		want zapcore.Level
	}{
		{slog: -4, want: zapcore.DebugLevel},
		{slog: 0, want: zapcore.InfoLevel},
		{slog: 4, want: zapcore.WarnLevel},
		{slog: 8, want: zapcore.ErrorLevel},
	}
	for _, tc := range cases {
		if got := slogLevelToZap(tc.slog); got != tc.want {
			t.Fatalf("slog=%d mapped to %v, want %v", tc.slog, got, tc.want)
		}
	}
}

func TestReplayToZap_DroppedRecordEmitsWarning(t *testing.T) {
	core, recorded := observer.New(zapcore.DebugLevel)
	log := zap.New(core)

	rec := &pluginsdk.LogRecord{
		Level:                0,
		Msg:                  "with-drops",
		DroppedSinceLastSend: 17,
	}
	replayToZap(log, "p", rec)

	all := recorded.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries (drop notice + record), got %d", len(all))
	}
	// First entry should be the drop notice at WARN.
	first := all[0]
	if first.Level != zapcore.WarnLevel || first.Message != "plugin log dropped" {
		t.Fatalf("drop notice missing or wrong level: %+v", first)
	}
	if first.ContextMap()["dropped"].(uint64) != 17 {
		t.Fatalf("dropped count wrong: %v", first.ContextMap()["dropped"])
	}
}

func TestReplayToZap_NilRecordIsNoop(t *testing.T) {
	core, recorded := observer.New(zapcore.DebugLevel)
	log := zap.New(core)
	replayToZap(log, "p", nil)
	if got := recorded.Len(); got != 0 {
		t.Fatalf("nil record should not log, got %d entries", got)
	}
}
