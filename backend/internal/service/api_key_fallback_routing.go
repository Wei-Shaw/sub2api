package service

import (
	"context"
	"errors"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// APIKeyRoutingCandidate is one ordered, request-scoped group candidate.
type APIKeyRoutingCandidate struct {
	Group        *Group
	Subscription *UserSubscription
	Unavailable  error
}

// APIKeyRoutingState keeps fallback routing mutable while the request context
// itself remains immutable. One request owns one state instance.
type APIKeyRoutingState struct {
	mu              sync.RWMutex
	apiKey          *APIKey
	candidates      []APIKeyRoutingCandidate
	committed       bool
	effectiveIndex  int
	subscriptionRef UserSubscription
	eligibility     func(context.Context, *APIKey, *Group, *UserSubscription) error
	eligible        map[int]bool
}

func NewAPIKeyRoutingState(apiKey *APIKey, candidates []APIKeyRoutingCandidate) *APIKeyRoutingState {
	state := &APIKeyRoutingState{apiKey: apiKey, candidates: append([]APIKeyRoutingCandidate(nil), candidates...), effectiveIndex: -1, eligible: make(map[int]bool)}
	if len(state.candidates) > 0 {
		state.applyCandidateLocked(0)
	}
	return state
}

func WithAPIKeyRoutingState(ctx context.Context, state *APIKeyRoutingState) context.Context {
	if state == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.APIKeyRoutingState, state)
}

func APIKeyRoutingStateFromContext(ctx context.Context) *APIKeyRoutingState {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(ctxkey.APIKeyRoutingState).(*APIKeyRoutingState)
	return state
}

func (s *APIKeyRoutingState) SetEligibilityChecker(check func(context.Context, *APIKey, *Group, *UserSubscription) error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.eligibility = check
	s.mu.Unlock()
}

func (s *APIKeyRoutingState) Candidates(groupID *int64) []APIKeyRoutingCandidate {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.committed || len(s.candidates) == 0 || groupID == nil || s.apiKey == nil || s.apiKey.GroupID == nil || *groupID != *s.apiKey.GroupID {
		return nil
	}
	start := s.effectiveIndex
	if start < 0 {
		start = 0
	}
	return append([]APIKeyRoutingCandidate(nil), s.candidates[start:]...)
}

func (s *APIKeyRoutingState) CheckEligibility(ctx context.Context, candidate APIKeyRoutingCandidate) error {
	if candidate.Unavailable != nil {
		return candidate.Unavailable
	}
	if candidate.Group == nil || !candidate.Group.IsActive() {
		return ErrGroupNotFound
	}
	s.mu.RLock()
	check := s.eligibility
	apiKey := s.apiKey
	s.mu.RUnlock()
	if check == nil {
		return nil
	}
	return check(ctx, apiKey, candidate.Group, candidate.Subscription)
}

func (s *APIKeyRoutingState) EnsureEligible(ctx context.Context) error {
	_, err := s.EnsureEligibleFrom(ctx, -1)
	return err
}

func (s *APIKeyRoutingState) EnsureEligibleFrom(ctx context.Context, start int) (int, error) {
	if s == nil {
		return -1, nil
	}
	s.mu.RLock()
	if start < 0 {
		start = s.effectiveIndex
	}
	candidates := append([]APIKeyRoutingCandidate(nil), s.candidates...)
	s.mu.RUnlock()
	if start < 0 {
		start = 0
	}
	var lastErr error
	for index := start; index < len(candidates); index++ {
		s.mu.RLock()
		alreadyEligible := s.eligible[index]
		s.mu.RUnlock()
		if alreadyEligible {
			s.Activate(index)
			return index, nil
		}
		if err := s.CheckEligibility(ctx, candidates[index]); err != nil {
			if !IsAPIKeyFallbackCandidateUnavailable(err) {
				return -1, err
			}
			lastErr = err
			continue
		}
		s.mu.Lock()
		s.eligible[index] = true
		s.applyCandidateLocked(index)
		s.mu.Unlock()
		return index, nil
	}
	if lastErr != nil {
		return -1, lastErr
	}
	return -1, ErrNoAvailableAccounts
}

func (s *APIKeyRoutingState) Activate(index int) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.committed || index < 0 || index >= len(s.candidates) {
		return false
	}
	s.applyCandidateLocked(index)
	return true
}

func (s *APIKeyRoutingState) Commit(index int) bool {
	if !s.Activate(index) {
		return false
	}
	s.mu.Lock()
	s.committed = true
	s.mu.Unlock()
	return true
}

func (s *APIKeyRoutingState) MarkEligible(index int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.eligible[index] = true
	s.mu.Unlock()
}

func (s *APIKeyRoutingState) EffectiveIndex() int {
	if s == nil {
		return -1
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.effectiveIndex
}

func (s *APIKeyRoutingState) SubscriptionRef() *UserSubscription {
	if s == nil {
		return nil
	}
	return &s.subscriptionRef
}

func (s *APIKeyRoutingState) EffectiveSubscription() *UserSubscription {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.effectiveIndex < 0 || s.effectiveIndex >= len(s.candidates) || s.candidates[s.effectiveIndex].Subscription == nil {
		return nil
	}
	return &s.subscriptionRef
}

func (s *APIKeyRoutingState) FirstAvailable() (int, bool) {
	if s == nil {
		return 0, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for index, candidate := range s.candidates {
		if candidate.Unavailable == nil && candidate.Group != nil && candidate.Group.IsActive() {
			return index, true
		}
	}
	return 0, false
}

func (s *APIKeyRoutingState) applyCandidateLocked(index int) {
	candidate := s.candidates[index]
	s.effectiveIndex = index
	if s.apiKey != nil && candidate.Group != nil {
		groupID := candidate.Group.ID
		s.apiKey.GroupID = &groupID
		s.apiKey.Group = candidate.Group
	}
	if candidate.Subscription != nil {
		s.subscriptionRef = *candidate.Subscription
	} else {
		s.subscriptionRef = UserSubscription{}
	}
}

func IsAPIKeyFallbackSelectionError(err error) bool {
	return errors.Is(err, ErrNoAvailableAccounts) || errors.Is(err, ErrNoAvailableCompactAccounts)
}

// IsAPIKeyFallbackCandidateUnavailable reports errors tied to the current
// candidate group. Request-wide policy, rate-limit, cancellation, and
// infrastructure errors must stop routing instead of silently changing groups.
func IsAPIKeyFallbackCandidateUnavailable(err error) bool {
	return errors.Is(err, ErrGroupNotFound) ||
		errors.Is(err, ErrGroupNotAllowed) ||
		errors.Is(err, ErrInsufficientBalance) ||
		errors.Is(err, ErrSubscriptionNotFound) ||
		errors.Is(err, ErrSubscriptionInvalid) ||
		errors.Is(err, ErrDailyLimitExceeded) ||
		errors.Is(err, ErrWeeklyLimitExceeded) ||
		errors.Is(err, ErrMonthlyLimitExceeded) ||
		errors.Is(err, ErrGroupRPMExceeded) ||
		errors.Is(err, ErrUserPlatformDailyQuotaExhausted) ||
		errors.Is(err, ErrUserPlatformWeeklyQuotaExhausted) ||
		errors.Is(err, ErrUserPlatformMonthlyQuotaExhausted)
}
