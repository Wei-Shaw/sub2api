package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func seedProcessingLease(t *testing.T, repo *inMemoryIdempotencyRepo, opts IdempotencyExecuteOptions, lockedUntil time.Time) {
	t.Helper()
	fingerprint, err := BuildIdempotencyFingerprint(opts.Method, opts.Route, opts.ActorScope, opts.Payload)
	require.NoError(t, err)
	owner, err := repo.CreateProcessing(context.Background(), &IdempotencyRecord{
		Scope: opts.Scope, IdempotencyKeyHash: HashIdempotencyKey(opts.IdempotencyKey), RequestFingerprint: fingerprint,
		Status: IdempotencyStatusProcessing, LockedUntil: &lockedUntil, ExpiresAt: time.Now().Add(8 * 24 * time.Hour),
	})
	require.NoError(t, err)
	require.True(t, owner)
}

func TestIdempotencyCoordinator_ProcessingLeaseReclaimRequiresOptIn(t *testing.T) {
	for _, test := range []struct {
		name                            string
		enabled, expired, wantExecution bool
	}{
		{"default keeps expired processing blocked", false, true, false},
		{"opt in cannot steal a live lease", true, false, false},
		{"opt in reclaims an expired lease", true, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newInMemoryIdempotencyRepo()
			opts := IdempotencyExecuteOptions{Scope: "reclaim-test", Method: "POST", Route: "/reclaim", IdempotencyKey: "fixed-key", RequireKey: true,
				Payload: map[string]string{"credit_hash": "fixed-credit"}, ReclaimExpiredProcessing: test.enabled}
			lease := time.Now().Add(time.Hour)
			if test.expired {
				lease = time.Now().Add(-time.Second)
			}
			seedProcessingLease(t, repo, opts, lease)
			executions := 0
			coordinator := NewIdempotencyCoordinator(repo, DefaultIdempotencyConfig())
			result, err := coordinator.Execute(context.Background(), opts, func(context.Context) (any, error) { executions++; return "ok", nil })
			if test.wantExecution {
				require.NoError(t, err)
				require.Equal(t, 1, executions)
				require.False(t, result.Replayed)
			} else {
				require.ErrorIs(t, err, ErrIdempotencyInProgress)
				require.Zero(t, executions)
			}
		})
	}
}

func TestIdempotencyCoordinator_ProcessingLeaseReclaimHasOneOwner(t *testing.T) {
	repo := newInMemoryIdempotencyRepo()
	opts := IdempotencyExecuteOptions{Scope: "reclaim-race", Method: "POST", Route: "/reclaim", IdempotencyKey: "one-key", RequireKey: true, ReclaimExpiredProcessing: true}
	seedProcessingLease(t, repo, opts, time.Now().Add(-time.Second))
	var calls atomic.Int32
	entered, release, start := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var enterOnce sync.Once
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		coordinator := NewIdempotencyCoordinator(repo, DefaultIdempotencyConfig())
		go func() {
			<-start
			_, err := coordinator.Execute(context.Background(), opts, func(context.Context) (any, error) {
				calls.Add(1)
				enterOnce.Do(func() { close(entered) })
				<-release
				return "ok", nil
			})
			results <- err
		}()
	}
	close(start)
	<-entered
	require.ErrorIs(t, <-results, ErrIdempotencyInProgress)
	require.Equal(t, int32(1), calls.Load())
	close(release)
	require.NoError(t, <-results)
}

func TestIdempotencyCoordinator_ProcessingReclaimDoesNotStealRenewedLease(t *testing.T) {
	repo := &detachedIdempotencyRepo{inMemoryIdempotencyRepo: newInMemoryIdempotencyRepo()}
	cfg := DefaultIdempotencyConfig()
	cfg.ProcessingTimeout = 30 * time.Millisecond
	opts := IdempotencyExecuteOptions{Scope: "reclaim-heartbeat", Method: "POST", Route: "/reclaim", IdempotencyKey: "live-owner", RequireKey: true, ReclaimExpiredProcessing: true}
	coordinator := NewIdempotencyCoordinator(repo, cfg)
	release, done := make(chan struct{}), make(chan error, 1)
	var calls atomic.Int32
	execute := func(context.Context) (any, error) { calls.Add(1); <-release; return "ok", nil }
	go func() { _, err := coordinator.Execute(context.Background(), opts, execute); done <- err }()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	// 已超过最初租期，持续续租应使 opt-in 的另一个实例仍无法取得所有权。
	require.Eventually(t, func() bool { return repo.renewals.Load() >= 5 }, time.Second, 5*time.Millisecond)
	_, err := NewIdempotencyCoordinator(repo, cfg).Execute(context.Background(), opts, execute)
	require.ErrorIs(t, err, ErrIdempotencyInProgress)
	require.Equal(t, int32(1), calls.Load())
	close(release)
	require.NoError(t, <-done)
}

func TestOpenAIQuotaAutoResetService_ExpiredProcessingLeaseReusesOriginalRedeemID(t *testing.T) {
	svc, repo, quota, _ := newExpiryResetFixture(t)
	creditHash := shortOpenAIAutoResetHash("expiry-card")
	attempt := OpenAIAutoResetCreditAttempt{CreditHash: creditHash, Outcome: openAIAutoResetAttemptPending,
		CreditExpiresAt: quota.usage.autoResetCandidates[0].ExpiresAt, TriggerReason: openAIAutoResetTriggerExpiry}
	stableKey := openAIAutoResetAttemptKey(601, attempt)
	redeemID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(stableKey)).String()
	seedProcessingLease(t, svc.idempotency.repo.(*inMemoryIdempotencyRepo), IdempotencyExecuteOptions{
		Scope: "openai_auto_reset_credit", ActorScope: "account:601", Method: "POST", Route: "/system/openai/reset-credit/auto", IdempotencyKey: stableKey,
		Payload: map[string]any{"account_id": int64(601), "credit_hash": creditHash, "generation": ""},
	}, time.Now().Add(-time.Second))
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, &OpenAIAutoResetCreditState{
		Status: OpenAIAutoResetStatusResetting, AttemptCreditHash: creditHash, AttemptKeyVersion: openAIAutoResetCardKeyVersion,
		Attempts: []OpenAIAutoResetCreditAttempt{attempt},
	})
	quota.resetArgs = append(quota.resetArgs, [2]string{"expiry-card", redeemID})
	quota.resetCalls.Store(1) // 模拟上个进程已发出请求、还未来得及落库即退出。
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(2), quota.resetCalls.Load())
	require.Equal(t, quota.resetArgs[0], quota.resetArgs[1])
	require.Equal(t, openAIAutoResetAttemptConsumed, repo.stateForTest().Attempts[0].Outcome)
}
