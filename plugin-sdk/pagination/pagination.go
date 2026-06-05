// Package pagination is the single-source paging helper shared by every
// plugin. Before this package, each plugin carried its own copy under
// internal/pagination and the copies had already split their APIs (one grew a
// NormalizedSortOrder method, another grew ResultFromTotal). Parallel
// implementations of the same paging contract violate the CLAUDE.md
// "复用 (Reuse)" principle.
//
// This package owns the superset surface and depends only on the stdlib so it
// stays gin-free. The shape mirrors backend/internal/pkg/pagination.
package pagination

import "strings"

const (
	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"

	// defaultPageSize / maxPageSize bound PageSize so a hostile or malformed
	// limit cannot request an unbounded page.
	defaultPageSize = 20
	maxPageSize     = 1000
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
		return defaultPageSize
	}
	if p.PageSize > maxPageSize {
		return maxPageSize
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

// ResultFromTotal builds a PaginationResult from a total count and params.
func ResultFromTotal(total int64, params PaginationParams) *PaginationResult {
	limit := params.Limit()
	pages := int(total) / limit
	if int(total)%limit > 0 {
		pages++
	}
	return &PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: limit,
		Pages:    pages,
	}
}
