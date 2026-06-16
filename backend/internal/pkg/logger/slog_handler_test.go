package logger

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type captureState struct {
	writes []capturedWrite
}

type capturedWrite struct {
	fields []zapcore.Field
}

type captureCore struct {
	state      *captureState
	withFields []zapcore.Field
}

func newCaptureCore() *captureCore {
	return &captureCore{state: &captureState{}}
}

func (c *captureCore) Enabled(zapcore.Level) bool {
	return true
}

func (c *captureCore) With(fields []zapcore.Field) zapcore.Core {
	nextFields := make([]zapcore.Field, 0, len(c.withFields)+len(fields))
	nextFields = append(nextFields, c.withFields...)
	nextFields = append(nextFields, fields...)
	return &captureCore{
		state:      c.state,
		withFields: nextFields,
	}
}

func (c *captureCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(entry, c)
}

func (c *captureCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	allFields := make([]zapcore.Field, 0, len(c.withFields)+len(fields))
	allFields = append(allFields, c.withFields...)
	allFields = append(allFields, fields...)
	c.state.writes = append(c.state.writes, capturedWrite{
		fields: allFields,
	})
	return nil
}

func (c *captureCore) Sync() error {
	return nil
}

func TestSlogZapHandler_Handle_DoesNotAppendTimeField(t *testing.T) {
	core := newCaptureCore()
	handler := newSlogZapHandler(zap.New(core))

	record := slog.NewRecord(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), slog.LevelInfo, "hello", 0)
	record.AddAttrs(slog.String("component", "http.access"))

	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("handle slog record: %v", err)
	}
	if len(core.state.writes) != 1 {
		t.Fatalf("write calls = %d, want 1", len(core.state.writes))
	}

	var hasComponent bool
	for _, field := range core.state.writes[0].fields {
		if field.Key == "time" {
			t.Fatalf("unexpected duplicate time field in slog adapter output")
		}
		if field.Key == "component" {
			hasComponent = true
		}
	}
	if !hasComponent {
		t.Fatalf("component field should be preserved")
	}
}
