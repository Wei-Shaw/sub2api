package service

import (
	"context"
	"errors"
	"sync"

	coderws "github.com/coder/websocket"
)

// withAccountSchedulingWS keeps a request lease for each active turn, including
// the first passthrough frame (which does not call BeforeTurn). Idle connections
// hold no request lease. The persistent active-account slot remains independent.
func (s *OpenAIGatewayService) withAccountSchedulingWS(ctx context.Context, account *Account, hooks *OpenAIWSIngressHooks) (context.Context, *OpenAIWSIngressHooks, func(), error) {
	if s.accountSchedulingPools == nil || account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return ctx, hooks, func() {}, nil
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	sessionCtx, cancelSession := context.WithCancelCause(ctx)
	type turnLease struct {
		lease *AccountSchedulingLease
		stop  func() bool
	}
	var mu sync.Mutex
	leases := make(map[int]turnLease)
	currentTurn := 1
	acquire := func(turn int) error {
		mu.Lock()
		defer mu.Unlock()
		if existing, ok := leases[turn]; ok {
			if existing.lease != nil && existing.lease.Context().Err() != nil {
				return ErrAccountSchedulingUnavailable
			}
			return nil
		}
		lease, err := s.AcquireSchedulingPoolLease(sessionCtx, account, proxyURL)
		if err != nil {
			return NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "account is no longer eligible; please reconnect", err)
		}
		entry := turnLease{lease: lease}
		if lease != nil {
			entry.stop = context.AfterFunc(lease.Context(), func() {
				if errors.Is(context.Cause(lease.Context()), ErrAccountSchedulingLeaseLost) {
					cancelSession(ErrAccountSchedulingLeaseLost)
				}
			})
		}
		leases[turn] = entry
		currentTurn = turn
		return nil
	}
	release := func(turn int) {
		mu.Lock()
		entry, ok := leases[turn]
		delete(leases, turn)
		mu.Unlock()
		if ok {
			if entry.stop != nil {
				entry.stop()
			}
			entry.lease.Release()
		}
	}
	cleanup := func() {
		mu.Lock()
		turns := make([]int, 0, len(leases))
		for turn := range leases {
			turns = append(turns, turn)
		}
		mu.Unlock()
		for _, turn := range turns {
			release(turn)
		}
		cancelSession(nil)
	}
	if err := acquire(1); err != nil {
		cleanup()
		return ctx, hooks, func() {}, err
	}
	// HTTP bridge turns reuse the matching lease, while a different account or
	// actual proxy still has to pass the final transport admission check.
	sessionCtx = WithAccountSchedulingLeaseProvider(sessionCtx, account.ID, proxyURL, func() *AccountSchedulingLease {
		mu.Lock()
		defer mu.Unlock()
		return leases[currentTurn].lease
	})
	wrapped := &OpenAIWSIngressHooks{}
	if hooks != nil {
		*wrapped = *hooks
	}
	before, after := wrapped.BeforeTurn, wrapped.AfterTurn
	wrapped.BeforeTurn = func(turn int) error {
		if before != nil {
			if err := before(turn); err != nil {
				return err
			}
		}
		return acquire(turn)
	}
	wrapped.AfterTurn = func(turn int, result *OpenAIForwardResult, err error) {
		defer release(turn)
		if after != nil {
			after(turn, result, err)
		}
	}
	return sessionCtx, wrapped, cleanup, nil
}
