// Package service provides business logic and domain services for the application.
//
// UsageLog BC alias shim (Phase 3): the UsageLog entity, its RequestType /
// BillingType values, domain errors, and pure methods now live in
// internal/domain. The aliases below keep every existing service/handler/repo/
// test call site compiling unchanged. Create-result helpers remain in
// usage_log_create_result.go (app-layer error disposition).
package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	portusage "github.com/Wei-Shaw/sub2api/internal/port/usage"
)

// --- Type aliases (entity + port contract) ---

type UsageLog = domain.UsageLog
type UsageLogRepository = portusage.Repository

// BillingType* constants re-exported from domain.
const (
	BillingTypeBalance      = domain.BillingTypeBalance
	BillingTypeSubscription = domain.BillingTypeSubscription
)

// RequestType type + constants re-exported from domain.
type RequestType = domain.RequestType

const (
	RequestTypeUnknown      = domain.RequestTypeUnknown
	RequestTypeSync         = domain.RequestTypeSync
	RequestTypeStream       = domain.RequestTypeStream
	RequestTypeWSV2         = domain.RequestTypeWSV2
	RequestTypeCyberBlocked = domain.RequestTypeCyberBlocked
)

// RequestType helpers re-exported from domain as thin wrappers (free functions
// cannot be aliased in Go).
func RequestTypeFromInt16(v int16) RequestType {
	return domain.RequestTypeFromInt16(v)
}

func ParseUsageRequestType(value string) (RequestType, error) {
	return domain.ParseUsageRequestType(value)
}

func RequestTypeFromLegacy(stream bool, openAIWSMode bool) RequestType {
	return domain.RequestTypeFromLegacy(stream, openAIWSMode)
}

func ApplyLegacyRequestFields(requestType RequestType, fallbackStream bool, fallbackOpenAIWSMode bool) (stream bool, openAIWSMode bool) {
	return domain.ApplyLegacyRequestFields(requestType, fallbackStream, fallbackOpenAIWSMode)
}
