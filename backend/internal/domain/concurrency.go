// Concurrency BC: pure-data projections used by the repository layer for
// concurrency load queries. Lifted from internal/service so account_repo and
// (later) concurrency_cache can depend solely on domain. Service re-exports
// all four types as aliases (internal/service/concurrency_service.go).
package domain

// AccountWithConcurrency is the lightweight ID + max-concurrency projection
// the repository returns for account load queries.
type AccountWithConcurrency struct {
	ID             int64
	MaxConcurrency int
}

// UserWithConcurrency is the lightweight ID + max-concurrency projection the
// repository returns for user load queries.
type UserWithConcurrency struct {
	ID             int64
	MaxConcurrency int
}

// AccountLoadInfo reports the current load for a single account.
type AccountLoadInfo struct {
	AccountID          int64
	CurrentConcurrency int
	WaitingCount       int
	LoadRate           int // 0-100+ (percent)
}

// UserLoadInfo reports the current load for a single user.
type UserLoadInfo struct {
	UserID             int64
	CurrentConcurrency int
	WaitingCount       int
	LoadRate           int // 0-100+ (percent)
}
