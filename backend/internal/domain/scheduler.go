package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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

// Scheduler bucket-mode constants. All are referenced by the repository layer,
// so they live in domain; the service package re-exports them as const aliases.
const (
	SchedulerModeSingle = "single"
	SchedulerModeMixed  = "mixed"
	SchedulerModeForced = "forced"
)

// Scheduler cache errors. All are referenced by the repository layer, so they
// live in domain; the service package re-exports them as var aliases.
var (
	ErrSchedulerBucketRetired              = errors.New("scheduler bucket retired")
	ErrSchedulerBucketWriteFenced          = errors.New("scheduler bucket write fenced")
	ErrSchedulerGroupLifecycleLeaseInvalid = errors.New("scheduler group lifecycle lease invalid")
	ErrSchedulerGroupLifecycleLeaseLost    = errors.New("scheduler group lifecycle lease lost")
)

// SchedulerBucketWriteToken fences a snapshot writer to one bucket epoch.
// Tokens must be captured before any database load or queued rebuild work.
type SchedulerBucketWriteToken struct {
	Bucket SchedulerBucket
	Epoch  int64
}

func (t SchedulerBucketWriteToken) ValidFor(bucket SchedulerBucket) bool {
	return t.Epoch > 0 && t.Bucket == bucket
}

// SchedulerGroupLifecycleLease identifies one owner of a group's short-lived
// retirement/reopen critical section.
type SchedulerGroupLifecycleLease struct {
	GroupID    int64
	OwnerToken string
}

func (l SchedulerGroupLifecycleLease) ValidFor(groupID int64) bool {
	return groupID > 0 && l.GroupID == groupID && l.OwnerToken != ""
}

// SchedulerBucket identifies one schedulable group/platform/mode tuple.
type SchedulerBucket struct {
	GroupID  int64
	Platform string
	Mode     string
}

func (b SchedulerBucket) String() string {
	return fmt.Sprintf("%d:%s:%s", b.GroupID, b.Platform, b.Mode)
}

func ParseSchedulerBucket(raw string) (SchedulerBucket, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return SchedulerBucket{}, false
	}
	groupID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return SchedulerBucket{}, false
	}
	if parts[1] == "" || parts[2] == "" {
		return SchedulerBucket{}, false
	}
	return SchedulerBucket{
		GroupID:  groupID,
		Platform: parts[1],
		Mode:     parts[2],
	}, true
}
