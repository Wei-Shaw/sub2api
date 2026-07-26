package service

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type OpsQueryMode = domain.OpsQueryMode

const (
	OpsQueryModeAuto   = domain.OpsQueryModeAuto
	OpsQueryModeRaw    = domain.OpsQueryModeRaw
	OpsQueryModePreagg = domain.OpsQueryModePreagg
)

// ErrOpsPreaggregatedNotPopulated moved to internal/domain; service re-exports it
// so existing call sites and tests that reference service.Err... keep compiling.
var ErrOpsPreaggregatedNotPopulated = domain.ErrOpsPreaggregatedNotPopulated

// ParseOpsQueryMode / OpsQueryMode.IsValid moved to internal/domain as pure helpers.
// Service re-exports ParseOpsQueryMode as a thin wrapper so existing call sites
// (free-function references do not follow type aliases) keep compiling.
func ParseOpsQueryMode(raw string) OpsQueryMode {
	return domain.ParseOpsQueryMode(raw)
}

func shouldFallbackOpsPreagg(filter *OpsDashboardFilter, err error) bool {
	return filter != nil &&
		filter.QueryMode == OpsQueryModeAuto &&
		errors.Is(err, ErrOpsPreaggregatedNotPopulated)
}

func cloneOpsFilterWithMode(filter *OpsDashboardFilter, mode OpsQueryMode) *OpsDashboardFilter {
	if filter == nil {
		return nil
	}
	cloned := *filter
	cloned.QueryMode = mode
	return &cloned
}
