package repository

import "github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

func paginationResultFromTotal(total int64, params pagination.PaginationParams) *pagination.PaginationResult {
	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED
	return &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED
REDACTED
