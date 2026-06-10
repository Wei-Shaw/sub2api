package observability

import (
	"sync/atomic"
	"time"
)

const ChatSessionCaptureSlowThreshold = time.Second

type ChatSessionCaptureStats struct {
	QueueLength        int       `json:"queue_length"`
	QueueCapacity      int       `json:"queue_capacity"`
	EnqueuedTotal      uint64    `json:"enqueued_total"`
	DroppedTotal       uint64    `json:"dropped_total"`
	SuccessTotal       uint64    `json:"success_total"`
	FailureTotal       uint64    `json:"failure_total"`
	SlowTotal          uint64    `json:"slow_total"`
	LastDurationMS     int64     `json:"last_duration_ms"`
	MaxDurationMS      int64     `json:"max_duration_ms"`
	AverageDurationMS  float64   `json:"average_duration_ms"`
	LastSuccessAt      time.Time `json:"last_success_at,omitempty"`
	LastFailureAt      time.Time `json:"last_failure_at,omitempty"`
	LastSlowAt         time.Time `json:"last_slow_at,omitempty"`
	LastFailureMessage string    `json:"last_failure_message,omitempty"`
	LastSlowDurationMS int64     `json:"last_slow_duration_ms,omitempty"`
	SlowThresholdMS    int64     `json:"slow_threshold_ms"`
	UpdatedAt          time.Time `json:"updated_at"`
}

var chatSessionCaptureMetrics = &chatSessionCaptureMetricsState{}

type chatSessionCaptureMetricsState struct {
	enqueuedTotal uint64
	droppedTotal  uint64
	successTotal  uint64
	failureTotal  uint64
	slowTotal     uint64

	totalDurationMS uint64
	lastDurationMS  int64
	maxDurationMS   int64
	queueLength     int64
	queueCapacity   int64

	lastSuccessUnixNano int64
	lastFailureUnixNano int64
	lastSlowUnixNano    int64
	lastSlowDurationMS  int64
	lastFailureMessage  atomic.Value
}

func IncChatSessionCaptureEnqueued() {
	atomic.AddUint64(&chatSessionCaptureMetrics.enqueuedTotal, 1)
}

func IncChatSessionCaptureDropped() {
	atomic.AddUint64(&chatSessionCaptureMetrics.droppedTotal, 1)
}

func SetChatSessionCaptureQueueStats(length, capacity int) {
	if length < 0 {
		length = 0
	}
	if capacity < 0 {
		capacity = 0
	}
	atomic.StoreInt64(&chatSessionCaptureMetrics.queueLength, int64(length))
	atomic.StoreInt64(&chatSessionCaptureMetrics.queueCapacity, int64(capacity))
}

func ObserveChatSessionCaptureSuccess(duration time.Duration) {
	durationMS := duration.Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}
	atomic.AddUint64(&chatSessionCaptureMetrics.successTotal, 1)
	atomic.AddUint64(&chatSessionCaptureMetrics.totalDurationMS, uint64(durationMS))
	atomic.StoreInt64(&chatSessionCaptureMetrics.lastDurationMS, durationMS)
	updateMaxInt64(&chatSessionCaptureMetrics.maxDurationMS, durationMS)
	atomic.StoreInt64(&chatSessionCaptureMetrics.lastSuccessUnixNano, time.Now().UnixNano())
}

func ObserveChatSessionCaptureFailure(duration time.Duration, err error) {
	durationMS := duration.Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}
	atomic.AddUint64(&chatSessionCaptureMetrics.failureTotal, 1)
	atomic.StoreInt64(&chatSessionCaptureMetrics.lastDurationMS, durationMS)
	updateMaxInt64(&chatSessionCaptureMetrics.maxDurationMS, durationMS)
	atomic.StoreInt64(&chatSessionCaptureMetrics.lastFailureUnixNano, time.Now().UnixNano())
	if err != nil {
		chatSessionCaptureMetrics.lastFailureMessage.Store(err.Error())
	}
}

func ObserveChatSessionCaptureSlow(duration time.Duration) {
	durationMS := duration.Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}
	atomic.AddUint64(&chatSessionCaptureMetrics.slowTotal, 1)
	atomic.StoreInt64(&chatSessionCaptureMetrics.lastSlowDurationMS, durationMS)
	atomic.StoreInt64(&chatSessionCaptureMetrics.lastSlowUnixNano, time.Now().UnixNano())
}

func ChatSessionCaptureSnapshot(slowThreshold time.Duration) ChatSessionCaptureStats {
	successTotal := atomic.LoadUint64(&chatSessionCaptureMetrics.successTotal)
	totalDurationMS := atomic.LoadUint64(&chatSessionCaptureMetrics.totalDurationMS)
	avg := 0.0
	if successTotal > 0 {
		avg = float64(totalDurationMS) / float64(successTotal)
	}

	stats := ChatSessionCaptureStats{
		QueueLength:        int(atomic.LoadInt64(&chatSessionCaptureMetrics.queueLength)),
		QueueCapacity:      int(atomic.LoadInt64(&chatSessionCaptureMetrics.queueCapacity)),
		EnqueuedTotal:      atomic.LoadUint64(&chatSessionCaptureMetrics.enqueuedTotal),
		DroppedTotal:       atomic.LoadUint64(&chatSessionCaptureMetrics.droppedTotal),
		SuccessTotal:       successTotal,
		FailureTotal:       atomic.LoadUint64(&chatSessionCaptureMetrics.failureTotal),
		SlowTotal:          atomic.LoadUint64(&chatSessionCaptureMetrics.slowTotal),
		LastDurationMS:     atomic.LoadInt64(&chatSessionCaptureMetrics.lastDurationMS),
		MaxDurationMS:      atomic.LoadInt64(&chatSessionCaptureMetrics.maxDurationMS),
		AverageDurationMS:  avg,
		LastSlowDurationMS: atomic.LoadInt64(&chatSessionCaptureMetrics.lastSlowDurationMS),
		SlowThresholdMS:    slowThreshold.Milliseconds(),
		UpdatedAt:          time.Now().UTC(),
	}
	if v := atomic.LoadInt64(&chatSessionCaptureMetrics.lastSuccessUnixNano); v > 0 {
		stats.LastSuccessAt = time.Unix(0, v).UTC()
	}
	if v := atomic.LoadInt64(&chatSessionCaptureMetrics.lastFailureUnixNano); v > 0 {
		stats.LastFailureAt = time.Unix(0, v).UTC()
	}
	if v := atomic.LoadInt64(&chatSessionCaptureMetrics.lastSlowUnixNano); v > 0 {
		stats.LastSlowAt = time.Unix(0, v).UTC()
	}
	if v, ok := chatSessionCaptureMetrics.lastFailureMessage.Load().(string); ok {
		stats.LastFailureMessage = v
	}
	return stats
}

func updateMaxInt64(target *int64, value int64) {
	for {
		current := atomic.LoadInt64(target)
		if value <= current {
			return
		}
		if atomic.CompareAndSwapInt64(target, current, value) {
			return
		}
	}
}
