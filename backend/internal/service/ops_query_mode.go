package service

import (
	"errors"
	"strings"
)

type OpsQueryMode string

const (
	OpsQueryModeAuto   OpsQueryMode = "auto"
	OpsQueryModeRaw    OpsQueryMode = "raw"
	OpsQueryModePreagg OpsQueryMode = "preagg"
)

// ErrOpsPreaggregatedNotPopulated indicates that raw logs exist for a window, but the
// pre-aggregation tables are not populated yet. This is primarily used to implement
// the forced `preagg` mode UX.
var ErrOpsPreaggregatedNotPopulated = errors.New("ops pre-aggregated tables not populated")

func ParseOpsQueryMode(raw string) OpsQueryMode {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case string(OpsQueryModeRaw):
		return OpsQueryModeRaw
	case string(OpsQueryModePreagg):
		return OpsQueryModePreagg
	default:
		return OpsQueryModeAuto
	}
}

func (m OpsQueryMode) IsValid() bool {
	switch m {
	case OpsQueryModeAuto, OpsQueryModeRaw, OpsQueryModePreagg:
		return true
	default:
		return false
	}
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
