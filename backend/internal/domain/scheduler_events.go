package domain

// Scheduler outbox event type sentinels. The Account*-shaped events are also
// referenced by the repository layer (account / usage billing), so they live
// in domain; the service package re-exports them as const aliases. The
// group / full-rebuild events remain private to the service package.
const (
	SchedulerOutboxEventAccountChanged       = "account_changed"
	SchedulerOutboxEventAccountGroupsChanged = "account_groups_changed"
	SchedulerOutboxEventAccountBulkChanged   = "account_bulk_changed"
	SchedulerOutboxEventAccountLastUsed      = "account_last_used"
)
