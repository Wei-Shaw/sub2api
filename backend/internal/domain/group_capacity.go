// Group capacity BC: pure-data projections used by the repository layer to
// aggregate per-group account capacity. Lifted from internal/service so
// account_repo can depend solely on domain. Service re-exports the type as an
// alias (internal/service/group_capacity_service.go).
package domain

import (
	"time"
)

// GroupAccountCapacityRow is the lightweight account projection needed for
// capacity summary aggregation.
type GroupAccountCapacityRow struct {
	GroupID             int64
	AccountID           int64
	Concurrency         int
	Extra               map[string]any
	SessionWindowStart  *time.Time
	SessionWindowEnd    *time.Time
	SessionWindowStatus string
}
