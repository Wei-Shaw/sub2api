package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/port/billing"
)

// UserPlatformQuota data types, sentinels and the repository contract are owned
// by the billing bounded context (internal/domain + internal/port/billing).
// The aliases below preserve type identity so existing call sites and test
// stubs that reference service.* keep compiling without changes.

var ErrUserPlatformQuotaNotFound = domain.ErrUserPlatformQuotaNotFound

var ErrUserPlatformQuotaFKViolation = domain.ErrUserPlatformQuotaFKViolation

type UserPlatformQuotaSnapshot = domain.UserPlatformQuotaSnapshot

type UserPlatformQuotaRecord = domain.UserPlatformQuotaRecord

type UserPlatformQuotaRepository = billing.UserPlatformQuotaRepository
