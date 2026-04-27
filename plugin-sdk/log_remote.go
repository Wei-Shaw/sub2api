package pluginsdk

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// LogChannelCapacity bounds the in-process channel between plugin slog calls
// and the LogProxy stream consumer goroutine. Drop-on-full keeps the slog
// path non-blocking; per V4 user feedback there is intentionally no ring
// buffer, no batch flushing, and no stderr fallback — dropped records are
// only reported via dropped_since_last_send the next time a Send succeeds.
const LogChannelCapacity = 256

// remoteSlogReconnectMin / remoteSlogReconnectMax are the lower/upper bounds
// of the exponential-backoff reconnect loop. Picked to recover from short
// host blips fast (100ms) without hammering a struggling host (cap 30s).
const (
	remoteSlogReconnectMin = 100 * time.Millisecond
	remoteSlogReconnectMax = 30 * time.Second
)

// remoteSlogShutdownDrain is the best-effort window the SDK waits for the
// consumer goroutine to drain remaining records during Shutdown.
const remoteSlogShutdownDrain = 2 * time.Second

// remoteSlogHandler implements slog.Handler by enqueueing each record onto a
// bounded channel; a separate goroutine drains the channel into a LogProxy
// client-streaming RPC. Records that arrive while the channel is full or the
// stream is down are dropped and counted via droppedCount.
//
// The handler is safe for concurrent Handle calls and intentionally never
// blocks the caller — slog must stay cheap inside business code.
type remoteSlogHandler struct {
	level        slog.Leveler
	ch           chan *pb.LogRecord
	droppedCount atomic.Uint64

	// attrs / groupPrefix accumulate slog.Logger.With / WithGroup metadata.
	// They are flattened onto every Handle call so the wire record is
	// self-contained.
	attrs       []slog.Attr
	groupPrefix string
}

// newRemoteSlogHandler returns a handler configured to emit at >= level. The
// caller is expected to start a sender goroutine via runRemoteSlogSender.
func newRemoteSlogHandler(level slog.Leveler) *remoteSlogHandler {
	return &remoteSlogHandler{
		level: level,
		ch:    make(chan *pb.LogRecord, LogChannelCapacity),
	}
}

// Enabled gates by the configured level — same as a JSONHandler would.
func (h *remoteSlogHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	if h.level == nil {
		return lvl >= slog.LevelInfo
	}
	return lvl >= h.level.Level()
}

// Handle converts the slog.Record into a *pb.LogRecord and tries a
// non-blocking send. On overflow it bumps the dropped counter; the next
// successful Send picks the counter up and reports it via
// dropped_since_last_send.
func (h *remoteSlogHandler) Handle(_ context.Context, r slog.Record) error {
	rec := &pb.LogRecord{
		TimeUnixNano: uint64(r.Time.UnixNano()),
		Level:        int32(r.Level),
		Msg:          r.Message,
	}
	rec.Attrs = make([]*pb.LogAttr, 0, len(h.attrs)+r.NumAttrs())
	for _, a := range h.attrs {
		rec.Attrs = appendAttr(rec.Attrs, h.groupPrefix, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		rec.Attrs = appendAttr(rec.Attrs, h.groupPrefix, a)
		return true
	})

	select {
	case h.ch <- rec:
	default:
		h.droppedCount.Add(1)
	}
	return nil
}

// WithAttrs / WithGroup mirror the slog handler contract by returning a copy
// with the new metadata appended; the channel is shared so the new logger
// stays wired to the same stream.
func (h *remoteSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *remoteSlogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	if h.groupPrefix == "" {
		clone.groupPrefix = name
	} else {
		clone.groupPrefix = h.groupPrefix + "." + name
	}
	return &clone
}

// Close shuts down the channel; the sender goroutine drains it (with the
// shutdown drain timeout) and then closes the stream.
func (h *remoteSlogHandler) Close() {
	defer func() {
		// Guard against double-close from racing shutdown paths.
		_ = recover()
	}()
	close(h.ch)
}

// appendAttr flattens a slog.Attr (recursively for groups) into a list of
// pb.LogAttr. Group attrs use "." as the key separator to align with the
// host-side slog→zap convention.
func appendAttr(out []*pb.LogAttr, prefix string, a slog.Attr) []*pb.LogAttr {
	if a.Equal(slog.Attr{}) {
		return out
	}
	key := a.Key
	if prefix != "" && key != "" {
		key = prefix + "." + key
	}
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindGroup:
		nested := v.Group()
		if len(nested) == 0 {
			return out
		}
		for _, sub := range nested {
			out = appendAttr(out, key, sub)
		}
		return out
	case slog.KindString:
		return append(out, &pb.LogAttr{Key: key, Value: &pb.LogAttr_StringValue{StringValue: v.String()}})
	case slog.KindInt64:
		return append(out, &pb.LogAttr{Key: key, Value: &pb.LogAttr_IntValue{IntValue: v.Int64()}})
	case slog.KindUint64:
		// proto only has int64; uint64 max overflows but we accept the wrap
		// rather than introduce a separate field — uint counters this large
		// are unusual in plugin logs.
		return append(out, &pb.LogAttr{Key: key, Value: &pb.LogAttr_IntValue{IntValue: int64(v.Uint64())}}) //nolint:gosec
	case slog.KindFloat64:
		return append(out, &pb.LogAttr{Key: key, Value: &pb.LogAttr_FloatValue{FloatValue: v.Float64()}})
	case slog.KindBool:
		return append(out, &pb.LogAttr{Key: key, Value: &pb.LogAttr_BoolValue{BoolValue: v.Bool()}})
	case slog.KindTime:
		return append(out, &pb.LogAttr{Key: key, Value: &pb.LogAttr_TimeUnixNano{TimeUnixNano: uint64(v.Time().UnixNano())}})
	case slog.KindDuration:
		return append(out, &pb.LogAttr{Key: key, Value: &pb.LogAttr_DurationNanos{DurationNanos: int64(v.Duration())}})
	default:
		// Any / LogValuer fallthrough: JSON-encode and stash in bytes_value
		// with a ".json" key suffix so the host can render it as a structured
		// blob without losing the data.
		raw, err := json.Marshal(v.Any())
		if err != nil {
			raw = []byte(strings.ReplaceAll(v.String(), "\x00", ""))
		}
		return append(out, &pb.LogAttr{Key: key + ".json", Value: &pb.LogAttr_BytesValue{BytesValue: raw}})
	}
}

// runRemoteSlogSender is the per-handler consumer goroutine. It owns the
// LogProxy client-streaming RPC: open stream → send records → on send error
// close + reconnect with exponential backoff. Records arriving while the
// stream is down are dropped (the channel send in Handle goes to default).
//
// Stops when the handler's channel is closed AND the channel is drained.
func runRemoteSlogSender(ctx context.Context, client pb.LogProxyClient, h *remoteSlogHandler, logger *slog.Logger) {
	if client == nil || h == nil {
		return
	}
	backoff := remoteSlogReconnectMin
	for {
		stream, err := client.PushLogs(ctx)
		if err != nil {
			if waitOrExit(ctx, h, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = remoteSlogReconnectMin

		// Inner consume loop: drain channel until send fails or channel closes.
		streamErr := consumeLogs(ctx, stream, h)
		if streamErr == nil {
			// Channel closed cleanly during shutdown — try to flush and exit.
			_, _ = stream.CloseAndRecv()
			return
		}
		// Send failed: close stream and back off before reconnecting. Any
		// records arriving in the meantime will hit the channel-default path
		// and be counted as dropped.
		_, _ = stream.CloseAndRecv()
		if logger != nil {
			logger.Debug("plugin log stream broken, reconnecting",
				"backoff", backoff.String(),
				"error", streamErr,
			)
		}
		if waitOrExit(ctx, h, backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

// consumeLogs forwards records from h.ch to stream until an error or ch
// close. Returns nil when ch closed (clean shutdown), or the underlying
// Send error (triggers reconnect).
func consumeLogs(ctx context.Context, stream pb.LogProxy_PushLogsClient, h *remoteSlogHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case rec, ok := <-h.ch:
			if !ok {
				return nil
			}
			if dropped := h.droppedCount.Swap(0); dropped > 0 {
				rec.DroppedSinceLastSend = dropped
			}
			if err := stream.Send(rec); err != nil {
				// Re-add the dropped count we just consumed so the next
				// successful send still reports it; the failed record
				// itself is lost (acceptable for fire-and-forget).
				if rec.DroppedSinceLastSend > 0 {
					h.droppedCount.Add(rec.DroppedSinceLastSend)
				}
				return err
			}
		}
	}
}

// waitOrExit sleeps for d but returns early if ctx is cancelled or the
// handler channel was closed. Returns true when the caller should exit.
func waitOrExit(ctx context.Context, h *remoteSlogHandler, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(d):
		return false
	case _, ok := <-h.ch:
		// During backoff a record arrived; we count it as dropped to keep
		// behaviour consistent with the channel-full default path.
		if ok {
			h.droppedCount.Add(1)
			return false
		}
		return true
	}
}

func nextBackoff(d time.Duration) time.Duration {
	next := d * 2
	if next > remoteSlogReconnectMax {
		return remoteSlogReconnectMax
	}
	return next
}

// shutdownRemoteSlogSender closes the handler's channel and waits up to
// remoteSlogShutdownDrain for the sender goroutine to exit. Used by
// runner.gracefulShutdown so the final batch of records reaches the host
// before the process exits.
func shutdownRemoteSlogSender(h *remoteSlogHandler, done <-chan struct{}) {
	if h == nil {
		return
	}
	h.Close()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(remoteSlogShutdownDrain):
	}
}

