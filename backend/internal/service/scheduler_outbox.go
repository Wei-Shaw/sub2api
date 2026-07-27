package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/port/scheduler"
)

type SchedulerOutboxEvent = domain.SchedulerOutboxEvent

// SchedulerOutboxRepository 提供调度 outbox 的读取接口。
type SchedulerOutboxRepository = scheduler.SchedulerOutboxRepository

// SchedulerOutboxCleanupLease holds the PostgreSQL advisory lock used by
// scheduler outbox cleanup.
type SchedulerOutboxCleanupLease = scheduler.SchedulerOutboxCleanupLease
