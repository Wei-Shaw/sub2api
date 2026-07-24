package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ResolveProxyFallbackTarget re-exports the domain pure function.
func ResolveProxyFallbackTarget(start Proxy, byID map[int64]Proxy, now time.Time) (*int64, bool) {
	return domain.ResolveProxyFallbackTarget(start, byID, now)
}
