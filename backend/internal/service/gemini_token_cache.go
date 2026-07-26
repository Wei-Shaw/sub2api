package service

import "github.com/Wei-Shaw/sub2api/internal/port/cache"

// GeminiTokenCache stores short-lived access tokens and coordinates refresh to avoid stampedes.
type GeminiTokenCache = cache.GeminiTokenCache
