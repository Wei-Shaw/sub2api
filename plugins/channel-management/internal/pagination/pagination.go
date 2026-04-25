// Package pagination is a minimal copy of backend/internal/pkg/pagination so
// the plugin can express paginated queries without importing the core.
package pagination

import "strings"

const (
	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"
)

// PaginationParams captures user-supplied paging parameters. The zero value
// means "first page, default size, default sort".
type PaginationParams struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// PaginationResult is the metadata returned alongside paginated data.
type PaginationResult struct {
	Total    int64
	Page     int
	PageSize int
	Pages    int
}

// Offset computes the SQL OFFSET that matches Page/PageSize, clamping Page to
// at least 1 so a zero-valued struct still produces a valid offset.
func (p PaginationParams) Offset() int {
	if p.Page < 1 {
		p.Page = 1
	}
	return (p.Page - 1) * p.PageSize
}

// Limit clamps PageSize into the supported range.
func (p PaginationParams) Limit() int {
	if p.PageSize < 1 {
		return 20
	}
	if p.PageSize > 1000 {
		return 1000
	}
	return p.PageSize
}

// NormalizeSortOrder normalises "asc"/"desc" (case-insensitive) and falls
// back to defaultOrder for everything else.
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

// NormalizedSortOrder returns the normalised sort order using defaultOrder
// as the fallback.
func (p PaginationParams) NormalizedSortOrder(defaultOrder string) string {
	return NormalizeSortOrder(p.SortOrder, defaultOrder)
}
