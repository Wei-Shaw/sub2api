// Package pagination provides types and helpers for paginated responses.
package pagination

import "strings"

const (
	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"
)

// PaginationParams 分页参数
type PaginationParams struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// PaginationResult 分页结果
type PaginationResult struct {
	Total    int64
	Page     int
	PageSize int
	Pages    int
}

// DefaultPagination 默认分页参数
func DefaultPagination() PaginationParams {
	return PaginationParams{
		Page:      1,
		PageSize:  20,
		SortOrder: SortOrderDesc,
	}
}

// Offset 计算偏移量
func (p PaginationParams) Offset() int {
	if p.Page < 1 {
		p.Page = 1
	}
	return (p.Page - 1) * p.PageSize
}

// Limit 获取限制数
func (p PaginationParams) Limit() int {
	if p.PageSize < 1 {
		return 20
	}
	if p.PageSize > 1000 {
		return 1000
	}
	return p.PageSize
}

// NormalizeSortOrder normalizes sort order to asc/desc and falls back to defaultOrder.
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

// NormalizedSortOrder returns the normalized sort order using defaultOrder as fallback.
func (p PaginationParams) NormalizedSortOrder(defaultOrder string) string {
	return NormalizeSortOrder(p.SortOrder, defaultOrder)
}
