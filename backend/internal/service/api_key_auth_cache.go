package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// Auth cache snapshot cluster lives in domain (no service-only deps); aliases
// preserve the existing service.APIKeyAuth* references during incremental migration.
type (
	APIKeyAuthSnapshot      = domain.APIKeyAuthSnapshot
	APIKeyAuthUserSnapshot  = domain.APIKeyAuthUserSnapshot
	APIKeyAuthGroupSnapshot = domain.APIKeyAuthGroupSnapshot
	APIKeyAuthCacheEntry    = domain.APIKeyAuthCacheEntry
)
