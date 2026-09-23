package service

import (
	"context"
	"errors"
	"fmt"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const MaxSmartRoutingGroups = 10

func SmartRoutingPlatformSupported(platform string) bool {
	switch platform {
	case PlatformOpenAI, PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax:
		return true
	}
	return false
}

func (s *APIKeyService) validateRoutingGroups(ctx context.Context, user *User, ids []int64) error {
	if len(ids) > MaxSmartRoutingGroups {
		return infraerrors.BadRequest("SMART_ROUTING_LIMIT", "Choose at most 10 routing groups")
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return infraerrors.BadRequest("SMART_ROUTING_INVALID", "Routing groups must be positive and unique")
		}
		seen[id] = true
		group, err := s.groupRepo.GetByID(ctx, id)
		if err != nil {
			return fmt.Errorf("get routing group: %w", err)
		}
		if !SmartRoutingPlatformSupported(group.Platform) || group.Status != StatusActive {
			return infraerrors.BadRequest("SMART_ROUTING_UNSUPPORTED_GROUP", "Smart routing requires active OpenAI-compatible groups")
		}
		if !s.canUserBindGroup(ctx, user, group) {
			return ErrGroupNotAllowed
		}
	}
	return nil
}

// SmartRoutingKeys reloads permissions and groups for each request. Only IDs are
// cached with the credential, so deleting or revoking a secondary group takes
// effect without depending on that group's primary-key cache invalidation.
func (s *APIKeyService) SmartRoutingKeys(ctx context.Context, key *APIKey) ([]*APIKey, error) {
	if len(key.RoutingGroupIDs) == 0 || len(key.RoutingGroupIDs) > MaxSmartRoutingGroups {
		return nil, infraerrors.BadRequest("SMART_ROUTING_INVALID", "Invalid routing group configuration")
	}
	user, err := s.userRepo.GetByID(ctx, key.UserID)
	if err != nil {
		return nil, err
	}
	if !user.IsActive() {
		return nil, ErrInsufficientPerms
	}
	result := make([]*APIKey, 0, len(key.RoutingGroupIDs))
	for _, id := range key.RoutingGroupIDs {
		group, err := s.groupRepo.GetByID(ctx, id)
		if errors.Is(err, ErrGroupNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if group.Status != StatusActive || !SmartRoutingPlatformSupported(group.Platform) || !s.canUserBindGroup(ctx, user, group) {
			continue
		}
		candidate := *key
		candidateUser := *user
		candidateUser.UserGroupRPMOverride = nil
		if s.userGroupRateRepo != nil {
			override, err := s.userGroupRateRepo.GetRPMOverrideByUserAndGroup(ctx, user.ID, id)
			if err != nil {
				return nil, err
			}
			candidateUser.UserGroupRPMOverride = override
		}
		candidate.User = &candidateUser
		candidate.GroupID = &group.ID
		candidate.Group = group
		result = append(result, &candidate)
	}
	return result, nil
}
