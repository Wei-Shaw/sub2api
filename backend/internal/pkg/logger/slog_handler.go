package logger

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type slogZapHandler struct {
	logger *zap.Logger
	attrs  []slog.Attr
	groups []string
}

func newSlogZapHandler(logger *zap.Logger) slog.Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &slogZapHandler{
		logger: logger,
		attrs:  make([]slog.Attr, 0, 8),
		groups: make([]string, 0, 4),
	}
}

func (h *slogZapHandler) Enabled(_ context.Context, level slog.Level) bool {
	switch {
	case level >= slog.LevelError:
		return h.logger.Core().Enabled(LevelError)
	case level >= slog.LevelWarn:
		return h.logger.Core().Enabled(LevelWarn)
	case level <= slog.LevelDebug:
		return h.logger.Core().Enabled(LevelDebug)
	default:
		return h.logger.Core().Enabled(LevelInfo)
	}
}

func (h *slogZapHandler) Handle(_ context.Context, record slog.Record) error {
	fields := make([]zap.Field, 0, len(h.attrs)+record.NumAttrs()+3)
	fields = append(fields, slogAttrsToZapFields(h.groups, h.attrs)...)
	record.Attrs(func(attr slog.Attr) bool {
		fields = append(fields, slogAttrToZapField(h.groups, attr))
		return true
	})

	switch {
	case record.Level >= slog.LevelError:
		h.logger.Error(record.Message, fields...)
	case record.Level >= slog.LevelWarn:
		h.logger.Warn(record.Message, fields...)
	case record.Level <= slog.LevelDebug:
		h.logger.Debug(record.Message, fields...)
	default:
		h.logger.Info(record.Message, fields...)
	}
	return nil
}

func (h *slogZapHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &next
}

func (h *slogZapHandler) WithGroup(name string) slog.Handler {
	name = strings.TrimSpace(name)
	if name == "" {
		return h
	}
	next := *h
	next.groups = append(append([]string{}, h.groups...), name)
	return &next
}

func slogAttrsToZapFields(groups []string, attrs []slog.Attr) []zap.Field {
	fields := make([]zap.Field, 0, len(attrs))
	for _, attr := range attrs {
		fields = append(fields, slogAttrToZapField(groups, attr))
	}
	return fields
}

func slogAttrToZapField(groups []string, attr slog.Attr) zap.Field {
	if len(groups) > 0 {
		attr.Key = strings.Join(append(append([]string{}, groups...), attr.Key), ".")
	}
	value := attr.Value.Resolve()
	switch value.Kind() {
	case slog.KindBool:
		return zap.Bool(attr.Key, value.Bool())
	case slog.KindInt64:
		return zap.Int64(attr.Key, value.Int64())
	case slog.KindUint64:
		return zap.Uint64(attr.Key, value.Uint64())
	case slog.KindFloat64:
		return zap.Float64(attr.Key, value.Float64())
	case slog.KindDuration:
		return zap.Duration(attr.Key, value.Duration())
	case slog.KindTime:
		return zap.Time(attr.Key, value.Time())
	case slog.KindString:
		return zap.String(attr.Key, value.String())
	case slog.KindGroup:
		groupFields := make([]zap.Field, 0, len(value.Group()))
		for _, nested := range value.Group() {
			groupFields = append(groupFields, slogAttrToZapField(nil, nested))
		}
		return zap.Object(attr.Key, zapObjectFields(groupFields))
	case slog.KindAny:
		if t, ok := value.Any().(time.Time); ok {
			return zap.Time(attr.Key, t)
		}
		return zap.Any(attr.Key, value.Any())
	default:
		return zap.String(attr.Key, value.String())
	}
}

type zapObjectFields []zap.Field

func (z zapObjectFields) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	for _, field := range z {
		field.AddTo(enc)
	}
	return nil
}
