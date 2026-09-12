package handler

import (
	"errors"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var errOpenAIWSQuotaTurnActive = errors.New("previous websocket quota turn is still active")

// openAIWSTurnSubscription owns one logical turn's final-admission snapshot.
// Upstream failover restarts transport turn numbering at one, but must not move
// an already admitted request into a newer subscription quota bucket.
type openAIWSTurnSubscription struct {
	mu           sync.Mutex
	turn         int
	admitted     bool
	subscription *service.UserSubscription
}

func (s *openAIWSTurnSubscription) startAttempt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turn = 1
}

// admit invokes check exactly once for each logical turn, on a request-owned
// copy. A failed check publishes nothing; no shared auth/cache object is edited.
func (s *openAIWSTurnSubscription) admit(turn int, base *service.UserSubscription, check func(*service.UserSubscription) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.admitted {
		if s.turn != turn {
			return errOpenAIWSQuotaTurnActive
		}
		return nil
	}
	subscription := service.CloneSubscriptionForRequest(base)
	if err := check(subscription); err != nil {
		return err
	}
	s.turn, s.admitted, s.subscription = turn, true, subscription
	return nil
}

// finish returns an independent copy for synchronous cyber billing and the
// asynchronous usage task. Failure retains the frozen admission for failover.
// A stale or duplicate callback cannot retire a newer turn's snapshot.
func (s *openAIWSTurnSubscription) finish(turn int, turnErr error) *service.UserSubscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.admitted || s.turn != turn {
		return nil
	}
	subscription := service.CloneSubscriptionForRequest(s.subscription)
	if turnErr == nil {
		s.admitted, s.subscription = false, nil
	}
	return subscription
}
