//go:build unit

package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// barrierRefreshTokenCacheStub 让所有并发请求都先读到旧 token，再统一放行，
// 从而确定性地复现 "Get 之后、消费之前" 的 double-spend 窗口。
type barrierRefreshTokenCacheStub struct {
	*emailBindRefreshTokenCacheStub

	target  int
	barrier chan struct{}
	mu      sync.Mutex
	reads   int
}

func newBarrierRefreshTokenCacheStub(target int) *barrierRefreshTokenCacheStub {
	return &barrierRefreshTokenCacheStub{
		emailBindRefreshTokenCacheStub: newEmailBindRefreshTokenCacheStub(),
		target:                         target,
		barrier:                        make(chan struct{}),
	}
}

func (s *barrierRefreshTokenCacheStub) GetRefreshToken(ctx context.Context, tokenHash string) (*service.RefreshTokenData, error) {
	data, err := s.emailBindRefreshTokenCacheStub.GetRefreshToken(ctx, tokenHash)

	s.mu.Lock()
	s.reads++
	if s.reads == s.target {
		close(s.barrier)
	}
	s.mu.Unlock()
	<-s.barrier
	return data, err
}

// 同一个 refresh token 并发刷新时，只能有一个请求成功兑换，其余必须失败。
func TestAuthServiceRefreshTokenPairConsumesTokenOnce(t *testing.T) {
	ctx := context.Background()
	const workers = 8
	cache := newBarrierRefreshTokenCacheStub(workers)

	user := &service.User{
		ID:           41,
		Email:        "refresh-race@example.com",
		Username:     "refresh-race",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		TokenVersion: 1,
	}
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-refresh-race-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
		},
	}
	svc := service.NewAuthService(nil, newEmailBindUserRepoStub(user), nil, cache, cfg, nil, nil, nil, nil, nil, nil, nil, nil)

	pair, err := svc.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)

	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, refreshErr := svc.RefreshTokenPair(ctx, pair.RefreshToken)
			errs <- refreshErr
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	successes, invalid := 0, 0
	for refreshErr := range errs {
		switch {
		case refreshErr == nil:
			successes++
		case errors.Is(refreshErr, service.ErrRefreshTokenInvalid):
			invalid++
		default:
			t.Fatalf("unexpected refresh error: %v", refreshErr)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, workers-1, invalid)
}
