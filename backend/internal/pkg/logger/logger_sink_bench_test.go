package logger

import (
	"io"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// opsLikeSink mirrors the production ops sink policy: it keeps warn+ entries
// and audit entries, and drops everything else after receiving them.
type opsLikeSink struct {
	mu       sync.Mutex
	received int
	kept     []*LogEvent
}

func (s *opsLikeSink) WriteLogEvent(event *LogEvent) {
	level, _ := zapcore.ParseLevel(event.Level)
	component := event.Component
	if fc, _ := event.Fields["component"].(string); strings.TrimSpace(fc) != "" {
		component = fc
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.received++
	if s.AcceptsLogEntry(level, component) {
		s.kept = append(s.kept, event)
	}
}

func (s *opsLikeSink) AcceptsLogEntry(level Level, component string) bool {
	return level >= zapcore.WarnLevel || component == "audit"
}

// plainSink has no entry filter, so it must keep receiving every entry.
type plainSink struct{ received int }

func (s *plainSink) WriteLogEvent(*LogEvent) { s.received++ }

func newSinkBenchLogger() *zap.Logger {
	inner := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(io.Discard), zapcore.InfoLevel)
	return zap.New(newSinkCore().Wrap(inner)).With(zap.String("service", "sub2api"), zap.String("env", "bench"))
}

func TestSinkCoreWrite_FilteringSinkSkipsRejectedEntries(t *testing.T) {
	sink := &opsLikeSink{}
	SetSink(sink)
	defer SetSink(nil)
	l := newSinkBenchLogger()

	l.With(zap.String("component", "handler.gateway")).Info("dropped before encoding")
	l.Info("non-string component takes the full path", zap.Int("component", 1))
	if sink.received != 1 {
		t.Fatalf("received=%d, want only the non-string component entry", sink.received)
	}

	l.Warn("warn is kept")
	l.With(zap.String("component", "handler.gateway")).Info("last component wins", zap.String("component", "audit"))
	l.Named("audit").Info("logger name when component is blank", zap.String("component", " "))
	if sink.received != 4 || len(sink.kept) != 3 {
		t.Fatalf("received=%d kept=%d, want 4/3", sink.received, len(sink.kept))
	}
}

func TestSinkCoreWrite_SinkWithoutFilterReceivesEverything(t *testing.T) {
	sink := &plainSink{}
	SetSink(sink)
	defer SetSink(nil)

	newSinkBenchLogger().With(zap.String("component", "handler.gateway")).Info("info entry")
	if sink.received != 1 {
		t.Fatalf("received=%d, want 1", sink.received)
	}
}

func BenchmarkSinkCoreWrite(b *testing.B) {
	SetSink(&opsLikeSink{})
	defer SetSink(nil)
	l := newSinkBenchLogger().With(zap.String("component", "handler.gateway.messages"))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("sticky.account_selected",
			zap.Int64("selected_account_id", 42),
			zap.String("account_name", "acc"),
			zap.Bool("slot_acquired", true),
			zap.Int64("sticky_bound_account_id", 42),
		)
	}
}
