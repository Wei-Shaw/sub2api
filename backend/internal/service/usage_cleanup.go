package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/port/usagecleanup"
)

// Usage-cleanup BC types/consts live in domain; re-exported here for existing call sites.
type UsageCleanupFilters = domain.UsageCleanupFilters
type UsageCleanupTask = domain.UsageCleanupTask

// UsageCleanupRepository interface lives in port/usagecleanup.
type UsageCleanupRepository = usagecleanup.UsageCleanupRepository

const (
	UsageCleanupStatusPending   = domain.UsageCleanupStatusPending
	UsageCleanupStatusRunning   = domain.UsageCleanupStatusRunning
	UsageCleanupStatusSucceeded = domain.UsageCleanupStatusSucceeded
	UsageCleanupStatusFailed    = domain.UsageCleanupStatusFailed
	UsageCleanupStatusCanceled  = domain.UsageCleanupStatusCanceled
)
