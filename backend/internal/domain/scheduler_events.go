package domain

// Scheduler outbox event type sentinels. All are referenced by the repository
// layer, so they live in domain; the service package re-exports them as const
// aliases.
const (
	SchedulerOutboxEventAccountChanged       = "account_changed"
	SchedulerOutboxEventAccountGroupsChanged = "account_groups_changed"
	SchedulerOutboxEventAccountBulkChanged   = "account_bulk_changed"
	SchedulerOutboxEventAccountLastUsed      = "account_last_used"
	SchedulerOutboxEventGroupChanged         = "group_changed"
	SchedulerOutboxEventFullRebuild          = "full_rebuild"
)
