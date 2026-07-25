package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	portapikey "github.com/Wei-Shaw/sub2api/internal/port/apikey"
)

// API Key status constants re-exported from domain.
const (
	StatusAPIKeyActive         = domain.StatusAPIKeyActive
	StatusAPIKeyDisabled       = domain.StatusAPIKeyDisabled
	StatusAPIKeyQuotaExhausted = domain.StatusAPIKeyQuotaExhausted
	StatusAPIKeyExpired        = domain.StatusAPIKeyExpired
)

// Rate limit window durations re-exported from domain.
const (
	RateLimitWindow5h = domain.RateLimitWindow5h
	RateLimitWindow1d = domain.RateLimitWindow1d
	RateLimitWindow7d = domain.RateLimitWindow7d
)

// Type aliases keep existing service call sites compiling while the apikey BC
// owns its domain types. Mirror of user/group/proxy/redeem/promo/announcement.
type APIKey = domain.APIKey
type APIKeyListFilters = domain.APIKeyListFilters
type APIKeyRateLimitData = domain.APIKeyRateLimitData
type APIKeyQuotaUsageState = domain.APIKeyQuotaUsageState
type APIKeyRepository = portapikey.Repository

// IsWindowExpired re-exports the domain helper.
func IsWindowExpired(windowStart *time.Time, duration time.Duration) bool {
	return domain.IsWindowExpired(windowStart, duration)
}
