package domain

import "time"

// TempUnschedState 临时不可调度状态（属 scheduler BC；纯标量，无跨 BC 指针）。
type TempUnschedState struct {
	UntilUnix       int64  `json:"until_unix"`
	TriggeredAtUnix int64  `json:"triggered_at_unix"`
	StatusCode      int    `json:"status_code"`
	MatchedKeyword  string `json:"matched_keyword"`
	RuleIndex       int    `json:"rule_index"`
	ErrorMessage    string `json:"error_message"`
}

// SchedulerOutboxEvent 是调度 outbox 的一行事件（属 scheduler BC；纯标量 + map + time，无跨 BC 指针）。
type SchedulerOutboxEvent struct {
	ID        int64
	EventType string
	AccountID *int64
	GroupID   *int64
	Payload   map[string]any
	CreatedAt time.Time
}
