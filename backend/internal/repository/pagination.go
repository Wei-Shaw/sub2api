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

func paginateSlice[T any](items []T, params pagination.PaginationParams) []T {
	if len(items) == 0 {
		return []T{REDACTED
REDACTED

	offset := params.Offset()
	if offset >= len(items) {
		return []T{REDACTED
REDACTED

	limit := params.Limit()
	end := offset + limit
	if end > len(items) {
		end = len(items)
REDACTED

	return items[offset:end]
REDACTED
