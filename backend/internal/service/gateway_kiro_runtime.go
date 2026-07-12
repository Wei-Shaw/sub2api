package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kirocooldown"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const defaultKiroStreamKeepalive = 25 * time.Second

type kiroCooldownRecoveryAttemptedKeyType struct{}

var kiroCooldownRecoveryAttemptedKey = kiroCooldownRecoveryAttemptedKeyType{}

func isKiroGroup(group *Group) bool {
	return group != nil && group.Platform == PlatformKiro
}

func (s *GatewayService) BindStickySessionForGroup(ctx context.Context, groupID *int64, sessionHash string, accountID int64, group *Group) error {
	return s.BindStickySessionWithTTL(ctx, groupID, sessionHash, accountID, stickySessionTTLForGroup(group))
}

func (s *GatewayService) BindStickySessionWithTTL(ctx context.Context, groupID *int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if sessionHash == "" || accountID <= 0 || s.cache == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = stickySessionTTL
	}
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, accountID, ttl)
}

func stickySessionTTLForGroup(group *Group) time.Duration {
	if isKiroGroup(group) {
		return group.EffectiveKiroStickySessionTTL()
	}
	return stickySessionTTL
}

func (s *GatewayService) stickySessionTTLForGroupID(ctx context.Context, groupID *int64) time.Duration {
	resolvedGroupID := derefGroupID(groupID)
	if group := s.groupFromContext(ctx, resolvedGroupID); group != nil {
		return stickySessionTTLForGroup(group)
	}
	if groupID != nil && s.groupRepo != nil {
		if group, err := s.groupRepo.GetByIDLite(ctx, *groupID); err == nil {
			return stickySessionTTLForGroup(group)
		}
	}
	return stickySessionTTL
}

func (s *GatewayService) streamKeepaliveIntervalForAccount(account *Account) time.Duration {
	if account != nil && account.Platform == PlatformKiro {
		if s != nil && s.cfg != nil && s.cfg.Gateway.KiroStreamKeepaliveInterval > 0 {
			return time.Duration(s.cfg.Gateway.KiroStreamKeepaliveInterval) * time.Second
		}
		return defaultKiroStreamKeepalive
	}
	if s != nil && s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		return time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}
	return 0
}

func (s *GatewayService) tryRecoverKiroCooldownPool(ctx context.Context, accounts []Account, requestedModel string, excludedIDs map[int64]struct{}, allowMixedScheduling bool) bool {
	if s == nil || s.kiroCooldownStore == nil || ctx.Value(kiroCooldownRecoveryAttemptedKey) == true {
		return false
	}
	tokenKeys := s.kiroTransientCooldownRecoveryKeys(ctx, accounts, requestedModel, excludedIDs, allowMixedScheduling)
	if len(tokenKeys) == 0 {
		return false
	}
	cleared, err := s.kiroCooldownStore.ClearEarliestTransientCooldown(ctx, tokenKeys)
	if err != nil {
		logger.LegacyPrintf("service.gateway", "Kiro cooldown pool recovery failed: %v", err)
		return false
	}
	if cleared {
		logger.LegacyPrintf("service.gateway", "Kiro cooldown pool recovery cleared one transient cooldown")
	}
	return cleared
}

func (s *GatewayService) kiroTransientCooldownRecoveryKeys(ctx context.Context, accounts []Account, requestedModel string, excludedIDs map[int64]struct{}, allowMixedScheduling bool) []string {
	tokenKeys := make([]string, 0, len(accounts))
	eligible := 0
	for i := range accounts {
		acc := &accounts[i]
		if !isKiroDirectModeAccount(acc) {
			if allowMixedScheduling {
				continue
			}
			return nil
		}
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
		}
		if !acc.IsSchedulable() {
			continue
		}
		if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel, "") {
			continue
		}
		if !s.isAccountSchedulableForQuota(acc) ||
			!s.isAccountSchedulableForWindowCost(ctx, acc, false) ||
			!s.isAccountSchedulableForRPM(ctx, acc, false) {
			continue
		}
		eligible++
		state, err := s.getKiroCooldownState(ctx, buildKiroAccountKey(acc))
		if err != nil || state == nil || !state.Active || state.Reason != kirocooldown.CooldownReason429 {
			return nil
		}
		tokenKeys = append(tokenKeys, buildKiroAccountKey(acc))
	}
	if eligible == 0 || len(tokenKeys) != eligible {
		return nil
	}
	return tokenKeys
}
