package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type schedulingWSSettings struct{ SettingRepository }

func (schedulingWSSettings) GetValue(context.Context, string) (string, error) {
	return `{"enabled":true,"max_active_accounts":3}`, nil
}

type schedulingWSRepository struct {
	AccountSchedulingPoolRepository
	mu       sync.Mutex
	deny     bool
	requests []AccountSchedulingAdmission
	released []string
}

func (r *schedulingWSRepository) Reconcile(_ context.Context, request AccountSchedulingAdmission) (*AccountSchedulingDecision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
	return &AccountSchedulingDecision{Enabled: true, Allowed: map[int64]bool{request.AccountID: !r.deny}, Acquired: !r.deny}, nil
}

func (r *schedulingWSRepository) ReleaseLease(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.released = append(r.released, id)
	return nil
}

func (r *schedulingWSRepository) RefreshLease(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func schedulingWSFixture() (*OpenAIGatewayService, *Account, *schedulingWSRepository) {
	r := &schedulingWSRepository{}
	s := &OpenAIGatewayService{accountSchedulingPools: NewAccountSchedulingPoolService(r, nil, nil, schedulingWSSettings{})}
	a := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	return s, a, r
}

func TestAccountSchedulingWSLeasesEachTurnAndReusesBridgeLease(t *testing.T) {
	s, a, repo := schedulingWSFixture()
	var before, after []int
	original := &OpenAIWSIngressHooks{
		BeforeTurn: func(turn int) error { before = append(before, turn); return nil },
		AfterTurn:  func(turn int, _ *OpenAIForwardResult, _ error) { after = append(after, turn) },
	}
	ctx, hooks, cleanup, err := s.withAccountSchedulingWS(context.Background(), a, original)
	require.NoError(t, err)
	defer cleanup()
	require.Len(t, repo.requests, 1, "passthrough first frame must acquire even without BeforeTurn")
	first, reused, err := AccountSchedulingProvidedLease(ctx, a.ID, "")
	require.NoError(t, err)
	require.True(t, reused)
	require.NoError(t, hooks.BeforeTurn(1))
	require.Len(t, repo.requests, 1, "ctx_pool first turn must not acquire twice")
	hooks.AfterTurn(1, nil, nil)
	require.Equal(t, []string{first.id}, repo.released)
	require.NoError(t, ctx.Err(), "normal release must leave the client connection alive")
	lease, reused, err := AccountSchedulingProvidedLease(ctx, a.ID, "")
	require.NoError(t, err)
	require.Nil(t, lease)
	require.False(t, reused)
	require.NoError(t, hooks.BeforeTurn(2))
	second, reused, err := AccountSchedulingProvidedLease(ctx, a.ID, "")
	require.NoError(t, err)
	require.True(t, reused)
	require.NotEqual(t, first.id, second.id)
	_, _, err = AccountSchedulingProvidedLease(ctx, a.ID, "http://different-exit:8080")
	require.ErrorIs(t, err, ErrAccountSchedulingUnavailable)
	hooks.AfterTurn(2, nil, nil)
	require.Equal(t, []int{1, 2}, before)
	require.Equal(t, []int{1, 2}, after)
	require.Len(t, repo.released, 2)
	cleanup()
	require.Len(t, repo.released, 2, "cleanup must be idempotent")
}

func TestAccountSchedulingWSRechecksEligibilityAfterIdle(t *testing.T) {
	s, a, repo := schedulingWSFixture()
	ctx, hooks, cleanup, err := s.withAccountSchedulingWS(context.Background(), a, nil)
	require.NoError(t, err)
	defer cleanup()
	hooks.AfterTurn(1, nil, nil)
	repo.mu.Lock()
	repo.deny = true
	repo.mu.Unlock()
	require.Error(t, hooks.BeforeTurn(2), "a quota-exhausted or rotated account must reconnect")
	require.NoError(t, ctx.Err())
	require.Len(t, repo.released, 1)
}

func TestAccountSchedulingWSOverlappingCallbacksReleaseMatchingTurn(t *testing.T) {
	s, a, repo := schedulingWSFixture()
	ctx, hooks, cleanup, err := s.withAccountSchedulingWS(context.Background(), a, nil)
	require.NoError(t, err)
	defer cleanup()
	first, _, err := AccountSchedulingProvidedLease(ctx, a.ID, "")
	require.NoError(t, err)
	require.NoError(t, hooks.BeforeTurn(2))
	second, _, err := AccountSchedulingProvidedLease(ctx, a.ID, "")
	require.NoError(t, err)
	hooks.AfterTurn(1, nil, nil)
	require.Equal(t, []string{first.id}, repo.released)
	require.NoError(t, second.Context().Err())
	provided, _, err := AccountSchedulingProvidedLease(ctx, a.ID, "")
	require.NoError(t, err)
	require.Same(t, second, provided)
	hooks.AfterTurn(2, nil, nil)
	require.Equal(t, []string{first.id, second.id}, repo.released)
}

func TestAccountSchedulingWSLeaseLossCancelsUpstreamSession(t *testing.T) {
	s, a, _ := schedulingWSFixture()
	ctx, _, cleanup, err := s.withAccountSchedulingWS(context.Background(), a, nil)
	require.NoError(t, err)
	defer cleanup()
	lease, _, err := AccountSchedulingProvidedLease(ctx, a.ID, "")
	require.NoError(t, err)
	lease.cancel(ErrAccountSchedulingLeaseLost)
	select {
	case <-ctx.Done():
		require.ErrorIs(t, context.Cause(ctx), ErrAccountSchedulingLeaseLost)
	case <-time.After(time.Second):
		t.Fatal("lease loss left the upstream session running")
	}
}
