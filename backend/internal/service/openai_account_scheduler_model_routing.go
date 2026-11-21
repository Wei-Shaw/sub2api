package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// selectByModelRouting tries preferred accounts from groups.model_routing before
// session sticky / load-balance. Returns (nil, false, nil) when no rule applies
// or the preferred pool cannot take the request (caller falls through).
func (s *defaultOpenAIAccountScheduler) selectByModelRouting(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, bool, error) {
	if s == nil || s.service == nil {
		return nil, false, nil
	}
	matched, ok := s.resolveOpenAIModelRouting(ctx, req)
	if !ok || len(matched.Rule.Accounts) == 0 {
		return nil, false, nil
	}

	groupID := derefGroupID(req.GroupID)
	routePattern := matched.Pattern
	routeTotalMax := matched.Rule.MaxConcurrency
	if s.service.concurrencyService != nil &&
		s.service.concurrencyService.ModelRouteRuleSaturated(ctx, groupID, routePattern, routeTotalMax) {
		if s.service.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.openai_gateway", "[ModelRoutingDebug] openai scheduler rule total saturated: group_id=%d model=%s pattern=%s max=%d",
				groupID, req.RequestedModel, routePattern, routeTotalMax)
		}
		logger.LegacyPrintf("service.openai_gateway", "[ModelRouting] Rule concurrency saturated for model=%s, falling back to normal OpenAI selection", req.RequestedModel)
		return nil, false, nil
	}

	preferred, err := s.listOpenAIModelRoutingCandidates(ctx, req, matched)
	if err != nil {
		return nil, false, err
	}
	if len(preferred) == 0 {
		if s.service.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.openai_gateway", "[ModelRoutingDebug] openai scheduler no preferred candidates: group_id=%d model=%s pattern=%s",
				groupID, req.RequestedModel, routePattern)
		}
		return nil, false, nil
	}

	// Prefer sticky session account when it is in the preferred pool and eligible.
	ordered := preferred
	if req.StickyAccountID > 0 {
		for i, acc := range preferred {
			if acc.ID == req.StickyAccountID {
				ordered = append([]*Account{acc}, append(preferred[:i], preferred[i+1:]...)...)
				break
			}
		}
	}

	for _, account := range ordered {
		_, routeAccMax := modelRouteLimitsForAccount(matched.Rule, account.ID)
		if s.service.concurrencyService != nil &&
			s.service.concurrencyService.ModelRouteAccountSaturated(ctx, groupID, routePattern, account.ID, routeAccMax) {
			continue
		}
		fresh := s.service.resolveFreshSchedulableOpenAIAccount(ctx, account, req.Platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability)
		if fresh == nil {
			continue
		}
		if !s.isAccountRequestCompatible(ctx, fresh, req) || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) {
			continue
		}
		if req.RequireCompact && openAICompactSupportTier(fresh) == 0 {
			continue
		}

		result, acquireErr := s.service.tryAcquireAccountSlotWithModelRoute(
			ctx, fresh.ID, fresh.Concurrency, groupID, routePattern, routeAccMax, routeTotalMax,
		)
		if acquireErr != nil {
			return nil, false, acquireErr
		}
		if result == nil || !result.Acquired {
			continue
		}
		if req.SessionHash != "" && !req.PreserveStickyBinding {
			_ = s.service.bindOpenAIStickySessionDuringSelection(ctx, req.GroupID, req.SessionHash, fresh.ID)
		}
		if s.service.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.openai_gateway", "[ModelRoutingDebug] openai scheduler routed select: group_id=%d model=%s account=%d pattern=%s",
				groupID, req.RequestedModel, fresh.ID, routePattern)
		}
		return attachSelectionProfitGate(ctx, &AccountSelectionResult{
			Account:     fresh,
			Acquired:    true,
			ReleaseFunc: result.ReleaseFunc,
		}), true, nil
	}

	logger.LegacyPrintf("service.openai_gateway", "[ModelRouting] All routed accounts unavailable for model=%s, falling back to normal OpenAI selection", req.RequestedModel)
	return nil, false, nil
}

func (s *defaultOpenAIAccountScheduler) resolveOpenAIModelRouting(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (domain.MatchedModelRoutingRule, bool) {
	if req.GroupID == nil || strings.TrimSpace(req.RequestedModel) == "" {
		return domain.MatchedModelRoutingRule{}, false
	}
	platform := NormalizeOpenAICompatiblePlatform(req.Platform)
	if !modelRoutingAppliesToTargetPlatform(platform) {
		return domain.MatchedModelRoutingRule{}, false
	}
	group := s.loadOpenAISchedulingGroup(ctx, req.GroupID)
	if group == nil {
		return domain.MatchedModelRoutingRule{}, false
	}
	if !modelRoutingAppliesToPlatform(platform, group.Platform) {
		return domain.MatchedModelRoutingRule{}, false
	}
	return group.GetMatchingRoutingRule(req.RequestedModel)
}

func (s *defaultOpenAIAccountScheduler) loadOpenAISchedulingGroup(ctx context.Context, groupID *int64) *Group {
	if groupID == nil || s.service == nil || s.service.schedulerSnapshot == nil {
		return nil
	}
	group, err := s.service.schedulerSnapshot.GetGroupByIDLite(ctx, *groupID)
	if err != nil {
		return nil
	}
	return group
}

func (s *defaultOpenAIAccountScheduler) listOpenAIModelRoutingCandidates(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	matched domain.MatchedModelRoutingRule,
) ([]*Account, error) {
	accounts, err := s.service.listSchedulableAccounts(ctx, req.GroupID, req.Platform)
	if err != nil {
		return nil, err
	}
	accounts = s.filterGrokFreeQuotaAccounts(ctx, accounts)
	if req.Platform == PlatformGrok {
		now := timeNowUTC()
		accounts = filterGrokTeamModelRateLimitedAccounts(accounts, req.RequestedModel, now)
		accounts = filterGrokModelQuotaBlockedAccounts(accounts, req.RequestedModel, now)
	}

	byID := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		byID[acc.ID] = acc
	}

	type ranked struct {
		account  *Account
		priority int
	}
	rankedList := make([]ranked, 0, len(matched.Rule.Accounts))
	for _, ref := range matched.Rule.Accounts {
		acc, ok := byID[ref.ID]
		if !ok {
			continue
		}
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[acc.ID]; excluded {
				continue
			}
		}
		if !acc.IsSchedulable() {
			continue
		}
		if acc.Platform != NormalizeOpenAICompatiblePlatform(req.Platform) || !acc.IsOpenAICompatible() {
			continue
		}
		if s.service.isOpenAIAccountRequestRuntimeBlocked(acc, req.RequestedModel) {
			continue
		}
		if req.RequirePrivacySet && !acc.IsPrivacySet() {
			continue
		}
		if compatible, _ := s.isAccountRequestCompatibleReason(ctx, acc, req); !compatible {
			continue
		}
		if !s.isAccountTransportCompatible(acc, req.RequiredTransport) {
			continue
		}
		rankedList = append(rankedList, ranked{account: acc, priority: ref.Priority})
	}

	sort.SliceStable(rankedList, func(i, j int) bool {
		if rankedList[i].priority != rankedList[j].priority {
			return rankedList[i].priority < rankedList[j].priority
		}
		return rankedList[i].account.ID < rankedList[j].account.ID
	})

	out := make([]*Account, 0, len(rankedList))
	for _, item := range rankedList {
		out = append(out, item.account)
	}
	return out, nil
}

// timeNowUTC is overridden in tests when needed.
var timeNowUTC = func() time.Time { return time.Now() }
