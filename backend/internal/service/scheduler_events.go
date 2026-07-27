package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// Scheduler outbox event type sentinels — all declared in domain (referenced by
// the repository layer) and re-aliased here.
const (
	SchedulerOutboxEventAccountChanged       = domain.SchedulerOutboxEventAccountChanged
	SchedulerOutboxEventAccountGroupsChanged = domain.SchedulerOutboxEventAccountGroupsChanged
	SchedulerOutboxEventAccountBulkChanged   = domain.SchedulerOutboxEventAccountBulkChanged
	SchedulerOutboxEventAccountLastUsed      = domain.SchedulerOutboxEventAccountLastUsed
	SchedulerOutboxEventGroupChanged         = domain.SchedulerOutboxEventGroupChanged
	SchedulerOutboxEventFullRebuild          = domain.SchedulerOutboxEventFullRebuild
)
