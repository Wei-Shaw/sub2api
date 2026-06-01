// Package errors re-exports the shared plugin-sdk/apperr ApplicationError
// abstraction. The implementation lives in plugin-sdk/apperr (single source of
// truth); this file only forwards the surface so existing import paths and
// call sites keep working. Do not add logic here — extend apperr instead.
package errors

import "github.com/Wei-Shaw/sub2api/plugin-sdk/apperr"

// Types.
type (
	ApplicationError = apperr.ApplicationError
	Status           = apperr.Status
)

// Constants.
const (
	UnknownCode    = apperr.UnknownCode
	UnknownReason  = apperr.UnknownReason
	UnknownMessage = apperr.UnknownMessage
)

// Constructors and helpers.
var (
	New                 = apperr.New
	Clone               = apperr.Clone
	FromError           = apperr.FromError
	BadRequest          = apperr.BadRequest
	Unauthorized        = apperr.Unauthorized
	Forbidden           = apperr.Forbidden
	NotFound            = apperr.NotFound
	Conflict            = apperr.Conflict
	TooManyRequests     = apperr.TooManyRequests
	InternalServer      = apperr.InternalServer
	ServiceUnavailable  = apperr.ServiceUnavailable
	ToHTTP              = apperr.ToHTTP
	SanitizeCauseForLog = apperr.SanitizeCauseForLog
	ClassifyDBError     = apperr.ClassifyDBError
	ClassifyGRPCError   = apperr.ClassifyGRPCError
	IsPQCode            = apperr.IsPQCode
)
