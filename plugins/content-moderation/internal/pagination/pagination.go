// Package pagination provides a minimal copy of the core's pagination
// helpers so the plugin's service and repository can paginate query results
// without importing the core's internal package. The shape mirrors
// backend/internal/pkg/pagination.
package pagination

import "strings"

const (
	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"
)

// PaginationParams holds page/size/sort parameters for a list query.
type PaginationParams struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// PaginationResult is the metadata returned alongside a page of results.
type PaginationResult struct {
	Total    int64
	Page     int
	PageSize int
	Pages    int
}

// Offset returns the SQL OFFSET for the current page.
func (p PaginationParams) Offset() int {
	if p.Page < 1 {
		p.Page = 1
	}
	return (p.Page - 1) * p.PageSize
}

// Limit returns the SQL LIMIT, clamped to [1, 1000] with a default of 20.
func (p PaginationParams) Limit() int {
	if p.PageSize < 1 {
		return 20
	}
	if p.PageSize > 1000 {
		return 1000
	}
	return p.PageSize
}

// NormalizeSortOrder normalizes order to asc/desc, falling back to defaultOrder.
func NormalizeSortOrder(order string, defaultOrder string) string {
	switch strings.ToLower(strings.TrimSpace(defaultOrder)) {
	case SortOrderAsc:
		defaultOrder = SortOrderAsc
	default:
		defaultOrder = SortOrderDesc
	}
	switch strings.ToLower(strings.TrimSpace(order)) {
	case SortOrderAsc:
		return SortOrderAsc
	case SortOrderDesc:
		return SortOrderDesc
	default:
		return defaultOrder
	}
}

// ResultFromTotal builds a PaginationResult from a total count and params.
func ResultFromTotal(total int64, params PaginationParams) *PaginationResult {
	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
	}
	return &PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
	}
}
