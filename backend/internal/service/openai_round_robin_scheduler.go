package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	openAIRoundRobinSchedulerSettingCacheTTL  = 5 * time.Second
	openAIRoundRobinSchedulerSettingDBTimeout = 2 * time.Second
)

type cachedOpenAIRoundRobinSchedulerSetting struct {
	enabled   bool
	expiresAt int64
}

var openAIRoundRobinSchedulerSettingCache atomic.Value // *cachedOpenAIRoundRobinSchedulerSetting
var openAIRoundRobinSchedulerSettingSF singleflight.Group

type openAIRoundRobinScheduler struct {
	service *OpenAIGatewayService
	mu      sync.Mutex
	cursors map[string]uint64
}

func newOpenAIRoundRobinScheduler(service *OpenAIGatewayService) *openAIRoundRobinScheduler {
	return &openAIRoundRobinScheduler{
		service: service,
		cursors: make(map[string]uint64),
	}
}

func (s *OpenAIGatewayService) isOpenAIRoundRobinSchedulerEnabled(ctx context.Context) bool {
	if cached, ok := openAIRoundRobinSchedulerSettingCache.Load().(*cachedOpenAIRoundRobinSchedulerSetting); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.enabled
		}
	}

	result, _, _ := openAIRoundRobinSchedulerSettingSF.Do(openAIRoundRobinSchedulerSettingKey, func() (any, error) {
		if cached, ok := openAIRoundRobinSchedulerSettingCache.Load().(*cachedOpenAIRoundRobinSchedulerSetting); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.enabled, nil
			}
		}

		enabled := false
		if repo := s.openAIAdvancedSchedulerSettingRepo(); repo != nil {
			dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIRoundRobinSchedulerSettingDBTimeout)
			defer cancel()

			value, err := repo.GetValue(dbCtx, openAIRoundRobinSchedulerSettingKey)
			if err == nil {
				enabled = strings.EqualFold(strings.TrimSpace(value), "true")
			}
		}

		openAIRoundRobinSchedulerSettingCache.Store(&cachedOpenAIRoundRobinSchedulerSetting{
			enabled:   enabled,
			expiresAt: time.Now().Add(openAIRoundRobinSchedulerSettingCacheTTL).UnixNano(),
		})
		return enabled, nil
	})

	enabled, _ := result.(bool)
	return enabled
}

func (s *OpenAIGatewayService) getOpenAIRoundRobinScheduler(ctx context.Context) *openAIRoundRobinScheduler {
	if s == nil || !s.isOpenAIRoundRobinSchedulerEnabled(ctx) {
		return nil
	}
	s.openaiRoundRobinSchedulerOnce.Do(func() {
		if s.openaiRoundRobinScheduler == nil {
			s.openaiRoundRobinScheduler = newOpenAIRoundRobinScheduler(s)
		}
	})
	return s.openaiRoundRobinScheduler
}

func resetOpenAIRoundRobinSchedulerSettingCacheForTest() {
	openAIRoundRobinSchedulerSettingCache = atomic.Value{}
	openAIRoundRobinSchedulerSettingSF = singleflight.Group{}
}

func (r *openAIRoundRobinScheduler) Select(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{
		Layer: openAIAccountScheduleLayerRoundRobin,
	}
	start := time.Now()
	defer func() {
		decision.LatencyMs = time.Since(start).Milliseconds()
	}()

	if r == nil || r.service == nil {
		return nil, decision, ErrNoAvailableAccounts
	}
	selection, candidateCount, err := r.selectByRoundRobin(ctx, req)
	decision.CandidateCount = candidateCount
	if err != nil {
		return nil, decision, err
	}
	if selection != nil && selection.Account != nil {
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
	}
	return selection, decision, nil
}

func (r *openAIRoundRobinScheduler) selectByRoundRobin(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, int, error) {
	if r.service.checkChannelPricingRestriction(ctx, req.GroupID, req.RequestedModel) {
		return nil, 0, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, req.RequestedModel)
	}

	accounts, err := r.service.listSchedulableAccounts(ctx, req.GroupID)
	if err != nil {
		return nil, 0, err
	}

	candidates, compactBlocked := r.filterCandidates(ctx, accounts, req)
	if len(candidates) == 0 {
		return nil, 0, noAvailableOpenAISelectionError(req.RequestedModel, compactBlocked)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority < candidates[j].Priority
		}
		return candidates[i].ID < candidates[j].ID
	})

	cfg := r.service.schedulingConfig()
	key := buildOpenAIRoundRobinCursorKey(req)
	for start := 0; start < len(candidates); {
		end := start + 1
		for end < len(candidates) && candidates[end].Priority == candidates[start].Priority {
			end++
		}

		ordered := r.orderBucket(key, candidates[start:end])
		var waitAccount *Account
		usableBucket := false
		for _, account := range ordered {
			fresh := r.service.recheckSelectedOpenAIAccountFromDB(ctx, account, req.RequestedModel, false)
			if fresh == nil {
				continue
			}
			if req.RequireCompact && openAICompactSupportTier(fresh) == 0 {
				compactBlocked = true
				continue
			}
			if !isOpenAIAccountEligibleForRequest(fresh, req.RequestedModel, req.RequireCompact) ||
				!r.service.isOpenAIAccountTransportCompatible(fresh, req.RequiredTransport) ||
				!fresh.SupportsOpenAIImageCapability(req.RequiredImageCapability) {
				continue
			}
			usableBucket = true
			result, acquireErr := r.service.tryAcquireAccountSlot(ctx, fresh.ID, fresh.Concurrency)
			if acquireErr != nil {
				return nil, len(candidates), acquireErr
			}
			if result != nil && result.Acquired {
				selection, err := r.service.newSelectionResult(ctx, fresh, true, result.ReleaseFunc, nil)
				return selection, len(candidates), err
			}
			if waitAccount == nil {
				waitAccount = fresh
			}
		}
		if waitAccount != nil {
			selection, err := r.service.newSelectionResult(ctx, waitAccount, false, nil, &AccountWaitPlan{
				AccountID:      waitAccount.ID,
				MaxConcurrency: waitAccount.Concurrency,
				Timeout:        cfg.FallbackWaitTimeout,
				MaxWaiting:     cfg.FallbackMaxWaiting,
			})
			return selection, len(candidates), err
		}
		if usableBucket {
			return nil, len(candidates), noAvailableOpenAISelectionError(req.RequestedModel, compactBlocked)
		}
		start = end
	}
	return nil, len(candidates), noAvailableOpenAISelectionError(req.RequestedModel, compactBlocked)
}

func (r *openAIRoundRobinScheduler) filterCandidates(
	ctx context.Context,
	accounts []Account,
	req OpenAIAccountScheduleRequest,
) ([]*Account, bool) {
	needsUpstreamCheck := r.service.needsUpstreamChannelRestrictionCheck(ctx, req.GroupID)
	candidates := make([]*Account, 0, len(accounts))
	compactBlocked := false
	for i := range accounts {
		account := &accounts[i]
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[account.ID]; excluded {
				continue
			}
		}
		account = r.service.recheckSelectedOpenAIAccountFromDB(ctx, account, req.RequestedModel, false)
		if account == nil {
			continue
		}
		if !isOpenAIAccountEligibleForRequest(account, req.RequestedModel, req.RequireCompact) {
			if req.RequireCompact && account != nil && account.IsSchedulable() && account.IsOpenAI() &&
				(req.RequestedModel == "" || account.IsModelSupported(req.RequestedModel)) &&
				openAICompactSupportTier(account) == 0 {
				compactBlocked = true
			}
			continue
		}
		if needsUpstreamCheck && req.GroupID != nil && r.service.isUpstreamModelRestrictedByChannel(ctx, *req.GroupID, account, req.RequestedModel, req.RequireCompact) {
			continue
		}
		if !r.service.isOpenAIAccountTransportCompatible(account, req.RequiredTransport) {
			continue
		}
		if !account.SupportsOpenAIImageCapability(req.RequiredImageCapability) {
			continue
		}
		candidates = append(candidates, account)
	}
	return candidates, compactBlocked
}

func (r *openAIRoundRobinScheduler) orderBucket(key string, bucket []*Account) []*Account {
	if len(bucket) <= 1 {
		return append([]*Account(nil), bucket...)
	}
	r.mu.Lock()
	if r.cursors == nil {
		r.cursors = make(map[string]uint64)
	}
	cursor := r.cursors[key]
	start := int(cursor % uint64(len(bucket)))
	r.cursors[key] = cursor + 1
	r.mu.Unlock()

	ordered := make([]*Account, 0, len(bucket))
	for offset := 0; offset < len(bucket); offset++ {
		ordered = append(ordered, bucket[(start+offset)%len(bucket)])
	}
	return ordered
}

func buildOpenAIRoundRobinCursorKey(req OpenAIAccountScheduleRequest) string {
	groupID := int64(0)
	if req.GroupID != nil {
		groupID = *req.GroupID
	}
	return strings.Join([]string{
		strconv.FormatInt(groupID, 10),
		strings.TrimSpace(req.RequestedModel),
		string(req.RequiredTransport),
		string(req.RequiredImageCapability),
		strconv.FormatBool(req.RequireCompact),
	}, ":")
}
