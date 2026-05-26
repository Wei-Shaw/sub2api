package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

type OpsSystemLogSinkHealth struct {
	QueueDepth      int64  `json:"queue_depth"`
	QueueCapacity   int64  `json:"queue_capacity"`
	DroppedCount    uint64 `json:"dropped_count"`
	WriteFailed     uint64 `json:"write_failed_count"`
	WrittenCount    uint64 `json:"written_count"`
	AvgWriteDelayMs uint64 `json:"avg_write_delay_ms"`
	LastError       string `json:"last_error"`
}

type OpsSystemLogSink struct {
	opsRepo OpsRepository

	queue chan *logger.LogEvent

	batchSize     int
	flushInterval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	droppedCount uint64
	writeFailed  uint64
	writtenCount uint64
	totalDelayNs uint64

	lastError atomic.Value
}

func NewOpsSystemLogSink(opsRepo OpsRepository) *OpsSystemLogSink {
	ctx, cancel := context.WithCancel(context.Background())
	s := &OpsSystemLogSink{
		opsRepo:       opsRepo,
		queue:         make(chan *logger.LogEvent, 5000),
		batchSize:     200,
		flushInterval: time.Second,
		ctx:           ctx,
		cancel:        cancel,
	}
	s.lastError.Store("")
	return s
}

func (s *OpsSystemLogSink) Start() {
	if s == nil || s.opsRepo == nil {
		return
	}
	s.wg.Add(1)
	go s.run()
}

func (s *OpsSystemLogSink) Stop() {
	if s == nil {
		return
	}
	s.cancel()
	s.wg.Wait()
}

func (s *OpsSystemLogSink) WriteLogEvent(event *logger.LogEvent) {
	if s == nil || event == nil || !s.shouldIndex(event) {
		return
	}
	if s.ctx != nil {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
	}

	select {
	case s.queue <- event:
	default:
		atomic.AddUint64(&s.droppedCount, 1)
	}
}

func (s *OpsSystemLogSink) shouldIndex(event *logger.LogEvent) bool {
	level := strings.ToLower(strings.TrimSpace(event.Level))
	switch level {
	case "warn", "warning", "error", "fatal", "panic", "dpanic":
		return true
	}

	component := strings.ToLower(strings.TrimSpace(event.Component))
	// zap 的 LoggerName 往往为空或不等于业务组件名；业务组件名通常以字段 component 透传。
	if event.Fields != nil {
		if fc := strings.ToLower(strings.TrimSpace(asString(event.Fields["component"]))); fc != "" {
			component = fc
		}
	}
	if strings.Contains(component, "http.access") {
		return true
	}
	if strings.Contains(component, "audit") {
		return true
	}
	return false
}

func (s *OpsSystemLogSink) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	batch := make([]*logger.LogEvent, 0, s.batchSize)
	flush := func(baseCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		started := time.Now()
		inserted, err := s.flushBatch(baseCtx, batch)
		delay := time.Since(started)
		if err != nil {
			atomic.AddUint64(&s.writeFailed, uint64(len(batch)))
			s.lastError.Store(err.Error())
			_, _ = fmt.Fprintf(os.Stderr, "time=%s level=WARN msg=\"ops system log sink flush failed\" err=%v batch=%d\n",
				time.Now().Format(time.RFC3339Nano), err, len(batch),
			)
		} else {
			atomic.AddUint64(&s.writtenCount, uint64(inserted))
			atomic.AddUint64(&s.totalDelayNs, uint64(delay.Nanoseconds()))
			s.lastError.Store("")
		}
		batch = batch[:0]
	}
	drainAndFlush := func() {
		for {
			select {
			case item := <-s.queue:
				if item == nil {
					continue
				}
				batch = append(batch, item)
				if len(batch) >= s.batchSize {
					flush(context.Background())
				}
			default:
				flush(context.Background())
				return
			}
		}
	}

	for {
		select {
		case <-s.ctx.Done():
			drainAndFlush()
			return
		case item := <-s.queue:
			if item == nil {
				continue
			}
			batch = append(batch, item)
			if len(batch) >= s.batchSize {
				flush(s.ctx)
			}
		case <-ticker.C:
			flush(s.ctx)
		}
	}
}

func (s *OpsSystemLogSink) flushBatch(baseCtx context.Context, batch []*logger.LogEvent) (int, error) {
	inputs := make([]*OpsInsertSystemLogInput, 0, len(batch))
	for _, event := range batch {
		if event == nil {
			continue
		}
		createdAt := event.Time.UTC()
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}

		fields := copyMap(event.Fields)
		requestID := asString(fields["request_id"])
		clientRequestID := asString(fields["client_request_id"])
		platform := asString(fields["platform"])
		model := asString(fields["model"])
		component := strings.TrimSpace(event.Component)
		if fieldComponent := asString(fields["component"]); fieldComponent != "" {
			component = fieldComponent
		}
		if component == "" {
			component = "app"
		}

		userID := asInt64Ptr(fields["user_id"])
		accountID := asInt64Ptr(fields["account_id"])

		// 统一脱敏后写入索引。
		message := logredact.RedactText(strings.TrimSpace(event.Message))
		redactedExtra := logredact.RedactMap(fields)
		extraJSONBytes, _ := json.Marshal(redactedExtra)
		extraJSON := string(extraJSONBytes)
		if strings.TrimSpace(extraJSON) == "" {
			extraJSON = "{}"
		}

		inputs = append(inputs, &OpsInsertSystemLogInput{
			CreatedAt:       createdAt,
			Level:           strings.ToLower(strings.TrimSpace(event.Level)),
			Component:       component,
			Message:         message,
			RequestID:       requestID,
			ClientRequestID: clientRequestID,
			UserID:          userID,
			AccountID:       accountID,
			Platform:        platform,
			Model:           model,
			ExtraJSON:       extraJSON,
		})
	}

	if len(inputs) == 0 {
		return 0, nil
	}
	if baseCtx == nil || baseCtx.Err() != nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()
	inserted, err := s.opsRepo.BatchInsertSystemLogs(ctx, inputs)
	if err != nil {
		return 0, err
	}
	return int(inserted), nil
}

func (s *OpsSystemLogSink) Health() OpsSystemLogSinkHealth {
	if s == nil {
		return OpsSystemLogSinkHealth{}
	}
	written := atomic.LoadUint64(&s.writtenCount)
	totalDelay := atomic.LoadUint64(&s.totalDelayNs)
	var avgDelay uint64
	if written > 0 {
		avgDelay = (totalDelay / written) / uint64(time.Millisecond)
	}

	lastErr, _ := s.lastError.Load().(string)
	return OpsSystemLogSinkHealth{
		QueueDepth:      int64(len(s.queue)),
		QueueCapacity:   int64(cap(s.queue)),
		DroppedCount:    atomic.LoadUint64(&s.droppedCount),
		WriteFailed:     atomic.LoadUint64(&s.writeFailed),
		WrittenCount:    written,
		AvgWriteDelayMs: avgDelay,
		LastError:       strings.TrimSpace(lastErr),
	}
}

func copyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case fmt.Stringer:
		return strings.TrimSpace(t.String())
	default:
		return ""
	}
}

func asInt64Ptr(v any) *int64 {
	switch t := v.(type) {
	case int:
		n := int64(t)
		if n <= 0 {
			return nil
		}
		return &n
	case int64:
		n := t
		if n <= 0 {
			return nil
		}
		return &n
	case float64:
		n := int64(t)
		if n <= 0 {
			return nil
		}
		return &n
	case json.Number:
		if n, err := t.Int64(); err == nil {
			if n <= 0 {
				return nil
			}
			return &n
		}
	case string:
		raw := strings.TrimSpace(t)
		if raw == "" {
			return nil
		}
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			if n <= 0 {
				return nil
			}
			return &n
		}
	}
	return nil
}
