package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// Scheduler outbox event type sentinels. The Account*-shaped events are
// declared in domain (also referenced by the repository layer) and re-aliased
// here; group / full-rebuild events remain private to the service package.
const (
	SchedulerOutboxEventAccountChanged       = domain.SchedulerOutboxEventAccountChanged
	SchedulerOutboxEventAccountGroupsChanged = domain.SchedulerOutboxEventAccountGroupsChanged
	SchedulerOutboxEventAccountBulkChanged   = domain.SchedulerOutboxEventAccountBulkChanged
	SchedulerOutboxEventAccountLastUsed      = domain.SchedulerOutboxEventAccountLastUsed
	SchedulerOutboxEventGroupChanged         = "group_changed"
	SchedulerOutboxEventFullRebuild          = "full_rebuild"
)
