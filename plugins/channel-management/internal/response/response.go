// Package response re-exports the shared plugin-sdk/httpx gin response
// envelope. The implementation lives in plugin-sdk/httpx (single source of
// truth); this file only forwards the surface so existing import paths and
// call sites keep working. Do not add logic here — extend plugin-sdk/httpx.
package response

import "github.com/Wei-Shaw/sub2api/plugin-sdk/httpx"

// Types.
type (
	Response      = httpx.Response
	PaginatedData = httpx.PaginatedData
)

// Helpers.
var (
	Success          = httpx.Success
	Created          = httpx.Created
	Accepted         = httpx.Accepted
	Error            = httpx.Error
	ErrorWithDetails = httpx.ErrorWithDetails
	ErrorFrom        = httpx.ErrorFrom
	BadRequest       = httpx.BadRequest
	Unauthorized     = httpx.Unauthorized
	Forbidden        = httpx.Forbidden
	NotFound         = httpx.NotFound
	InternalError    = httpx.InternalError
	Paginated        = httpx.Paginated
	ParsePagination  = httpx.ParsePagination
)
