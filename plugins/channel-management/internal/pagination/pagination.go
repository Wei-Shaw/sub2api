// Package pagination re-exports the shared plugin-sdk/pagination helpers. The
// implementation lives in plugin-sdk/pagination (single source of truth); this
// file only forwards the surface so existing import paths and call sites keep
// working. Do not add logic here — extend plugin-sdk/pagination instead.
package pagination

import "github.com/Wei-Shaw/sub2api/plugin-sdk/pagination"

// Types.
type (
	PaginationParams = pagination.PaginationParams
	PaginationResult = pagination.PaginationResult
)

// Constants.
const (
	SortOrderAsc  = pagination.SortOrderAsc
	SortOrderDesc = pagination.SortOrderDesc
)

// Helpers.
var (
	NormalizeSortOrder = pagination.NormalizeSortOrder
	ResultFromTotal    = pagination.ResultFromTotal
)
