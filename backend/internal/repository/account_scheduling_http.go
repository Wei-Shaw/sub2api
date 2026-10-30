package repository

import (
	"context"
	"net/http"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *httpUpstreamService) SetAccountSchedulingPools(pools *service.AccountSchedulingPoolService) {
	s.accountSchedulingPools = pools
}

func (s *httpUpstreamService) beginAccountSchedulingRequest(req *http.Request, actualProxyURL string, accountID int64) (*http.Request, func(), error) {
	noop := func() {}
	if req == nil || s.accountSchedulingPools == nil || !service.IsAccountSchedulingInference(req.Context()) {
		return req, noop, nil
	}
	lease, reused, err := service.AccountSchedulingProvidedLease(req.Context(), accountID, actualProxyURL)
	if err != nil {
		return nil, noop, err
	}
	if !reused {
		lease, err = s.accountSchedulingPools.Acquire(req.Context(), accountID, actualProxyURL)
		if err != nil {
			return nil, noop, err
		}
	}
	if lease == nil {
		return req, noop, nil
	}
	// Preserve the request's transport-profile and tracing values while making
	// lease loss cancel its upstream operation, including an already-open stream.
	ctx, cancel := context.WithCancelCause(req.Context())
	stop := context.AfterFunc(lease.Context(), func() { cancel(context.Cause(lease.Context())) })
	var once sync.Once
	release := func() {
		once.Do(func() {
			stop()
			cancel(nil)
			if !reused {
				lease.Release()
			}
		})
	}
	if lease.Context().Err() != nil {
		release()
		return nil, noop, service.ErrAccountSchedulingLeaseLost
	}
	return req.WithContext(ctx), release, nil
}
