package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/port/usergrouprate"
)

// User-group-rate BC types live in domain; re-exported here for existing call sites.
type UserGroupRateEntry = domain.UserGroupRateEntry
type GroupRateMultiplierInput = domain.GroupRateMultiplierInput
type GroupRPMOverrideInput = domain.GroupRPMOverrideInput

// UserGroupRateRepository interface lives in port/usergrouprate.
type UserGroupRateRepository = usergrouprate.UserGroupRateRepository
