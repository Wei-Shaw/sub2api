package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/port/scheduler"
)

// Scheduler bucket-mode constants — declared in domain (referenced by the
// repository layer) and re-aliased here.
const (
	SchedulerModeSingle = domain.SchedulerModeSingle
	SchedulerModeMixed  = domain.SchedulerModeMixed
	SchedulerModeForced = domain.SchedulerModeForced
)

// Scheduler cache errors — declared in domain (referenced by the repository
// layer) and re-aliased here so sentinel identity is preserved.
var (
	ErrSchedulerBucketRetired              = domain.ErrSchedulerBucketRetired
	ErrSchedulerBucketWriteFenced          = domain.ErrSchedulerBucketWriteFenced
	ErrSchedulerGroupLifecycleLeaseInvalid = domain.ErrSchedulerGroupLifecycleLeaseInvalid
	ErrSchedulerGroupLifecycleLeaseLost    = domain.ErrSchedulerGroupLifecycleLeaseLost
)

// SchedulerBucketWriteToken fences a snapshot writer to one bucket epoch.
// Tokens must be captured before any database load or queued rebuild work.
type SchedulerBucketWriteToken = domain.SchedulerBucketWriteToken

// SchedulerGroupLifecycleLease identifies one owner of a group's short-lived
// retirement/reopen critical section.
type SchedulerGroupLifecycleLease = domain.SchedulerGroupLifecycleLease

// SchedulerBucket identifies one schedulable group/platform/mode tuple.
type SchedulerBucket = domain.SchedulerBucket

// ParseSchedulerBucket parses a "groupID:platform:mode" string. Forwarded to
// domain — Go has no function aliases, so this thin wrapper preserves the
// historical call sites.
func ParseSchedulerBucket(raw string) (SchedulerBucket, bool) {
	return domain.ParseSchedulerBucket(raw)
}

// SchedulerCache 负责调度快照与账号快照的缓存读写。
type SchedulerCache = scheduler.SchedulerCache
