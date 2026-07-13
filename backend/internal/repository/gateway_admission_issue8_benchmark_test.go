package repository

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

type issue8PollOnceStore struct {
	attempts atomic.Int64
}

func (s *issue8PollOnceStore) TryAcquireUserLease(
	_ context.Context,
	_ service.UserLeaseRequest,
) (service.UserLeaseResult, error) {
	if s.attempts.Add(1)%2 == 1 {
		return service.UserLeaseResult{}, nil
	}
	return service.UserLeaseResult{Acquired: true, Class: service.AdmissionClassStandard}, nil
}

func (*issue8PollOnceStore) RenewUserLease(
	context.Context,
	int64,
	string,
	service.AdmissionClass,
) (bool, error) {
	return true, nil
}

func (*issue8PollOnceStore) ReleaseUserLease(context.Context, int64, string) error {
	return nil
}

func (*issue8PollOnceStore) TryAcquireTargetLease(
	context.Context,
	service.TargetLeaseRequest,
) (service.TargetLeaseResult, error) {
	return service.TargetLeaseResult{Acquired: true}, nil
}

func (*issue8PollOnceStore) BeginTargetDispatch(
	context.Context,
	service.TargetDispatchRequest,
) (service.TargetDispatchResult, error) {
	return service.TargetDispatchResult{Started: true}, nil
}

func (*issue8PollOnceStore) RenewTargetLease(
	context.Context,
	string,
	int64,
	string,
) (bool, error) {
	return true, nil
}

func (*issue8PollOnceStore) ReleaseTargetLease(context.Context, string, int64, string) error {
	return nil
}

func BenchmarkIssue8GatewayAdmissionLifecycle(b *testing.B) {
	rdb := newBenchmarkRedisClient(b)
	b.Cleanup(func() { _ = rdb.Close() })

	baseID := time.Now().UnixNano()
	platform := "issue8-benchmark-" + strconv.FormatInt(baseID, 10)
	ctx := context.Background()

	b.Run("flag_off_legacy_short", func(b *testing.B) {
		const requestID = "issue8-legacy-short"
		userID := baseID + 1
		accountID := baseID + 2
		cache := NewConcurrencyCache(rdb, benchSlotTTLMinutes, int(benchSlotTTL.Seconds()))

		b.ReportAllocs()
		b.ReportMetric(4, "lease-calls/op")
		b.ResetTimer()
		for range b.N {
			userAcquired, err := cache.AcquireUserSlot(ctx, userID, 1, requestID)
			if err != nil || !userAcquired {
				b.Fatalf("acquire legacy user slot: acquired=%v err=%v", userAcquired, err)
			}
			accountAcquired, err := cache.AcquireAccountSlot(ctx, accountID, 1, requestID)
			if err != nil || !accountAcquired {
				b.Fatalf("acquire legacy account slot: acquired=%v err=%v", accountAcquired, err)
			}
			if err := cache.ReleaseAccountSlot(ctx, accountID, requestID); err != nil {
				b.Fatalf("release legacy account slot: %v", err)
			}
			if err := cache.ReleaseUserSlot(ctx, userID, requestID); err != nil {
				b.Fatalf("release legacy user slot: %v", err)
			}
		}
		b.StopTimer()
		cleanupIssue8BenchmarkKeys(b, rdb, "", []int64{userID}, []int64{accountID})
	})

	b.Run("flag_on_standard_short", func(b *testing.B) {
		userID := baseID + 3
		accountID := baseID + 4
		store := NewGatewayAdmissionStore(rdb, benchSlotTTL)
		capacitySource := issue8BenchmarkCapacitySource(b, rdb, platform, accountID, 4)
		admission := service.NewGatewayAdmission(store, nil, capacitySource)
		request := service.GatewayAdmissionRequest{
			UserID:        userID,
			StandardLimit: 1,
			Settings: service.ExtraConcurrencyRuntimeSettings{
				Enabled:            true,
				WaitTimeoutSeconds: 1,
				ReservePercent:     25,
				MinReservedSlots:   1,
			},
		}
		account := &service.Account{ID: accountID, Platform: platform, Concurrency: 4}

		b.ReportAllocs()
		b.ReportMetric(4, "lease-calls/op")
		b.ResetTimer()
		for range b.N {
			runIssue8GatewayLifecycle(b, ctx, admission, request, account, service.AdmissionClassStandard)
		}
		b.StopTimer()
		cleanupIssue8BenchmarkKeys(b, rdb, platform, []int64{userID}, []int64{accountID})
	})

	b.Run("flag_on_extra_short", func(b *testing.B) {
		userID := baseID + 5
		accountID := baseID + 6
		store := NewGatewayAdmissionStore(rdb, benchSlotTTL)
		seedRequestID := "issue8-standard-seed"
		seed, err := store.TryAcquireUserLease(ctx, service.UserLeaseRequest{
			RequestID:     seedRequestID,
			UserID:        userID,
			StandardLimit: 1,
			ExtraLimit:    1,
		})
		if err != nil || !seed.Acquired || seed.Class != service.AdmissionClassStandard {
			b.Fatalf("seed standard user lease: result=%+v err=%v", seed, err)
		}
		capacitySource := issue8BenchmarkCapacitySource(b, rdb, platform, accountID, 4)
		admission := service.NewGatewayAdmission(store, nil, capacitySource)
		request := service.GatewayAdmissionRequest{
			UserID:        userID,
			StandardLimit: 1,
			ExtraLimit:    1,
			Settings: service.ExtraConcurrencyRuntimeSettings{
				Enabled:            true,
				WaitTimeoutSeconds: 1,
				ReservePercent:     25,
				MinReservedSlots:   1,
			},
		}
		account := &service.Account{ID: accountID, Platform: platform, Concurrency: 4}

		b.ReportAllocs()
		b.ReportMetric(4, "lease-calls/op")
		b.ResetTimer()
		for range b.N {
			runIssue8GatewayLifecycle(b, ctx, admission, request, account, service.AdmissionClassExtra)
		}
		b.StopTimer()
		if err := store.ReleaseUserLease(ctx, userID, seedRequestID); err != nil {
			b.Fatalf("release standard seed: %v", err)
		}
		cleanupIssue8BenchmarkKeys(b, rdb, platform, []int64{userID}, []int64{accountID})
	})

	b.Run("flag_on_poll_once", func(b *testing.B) {
		store := &issue8PollOnceStore{}
		admission := service.NewGatewayAdmission(store, nil, nil)
		request := service.GatewayAdmissionRequest{
			UserID:        baseID + 7,
			StandardLimit: 1,
			Settings: service.ExtraConcurrencyRuntimeSettings{
				WaitTimeoutSeconds: 1,
			},
		}

		b.ReportAllocs()
		b.ReportMetric(1, "polls/op")
		b.ResetTimer()
		for range b.N {
			session, err := admission.Begin(ctx, request)
			if err != nil {
				b.Fatalf("begin one-poll admission: %v", err)
			}
			session.Close()
		}
	})
}

func issue8BenchmarkCapacitySource(
	b *testing.B,
	rdb *redis.Client,
	platform string,
	accountID int64,
	capacity int,
) service.AdmissionCapacitySource {
	b.Helper()

	schedulerCache := NewSchedulerCache(rdb)
	capacityCache, ok := schedulerCache.(service.AdmissionCapacityCache)
	if !ok {
		b.Fatal("scheduler cache does not expose admission capacity projection")
	}
	err := capacityCache.SetAdmissionCapacity(context.Background(), platform, service.AdmissionCapacitySnapshot{
		TotalConcurrency: capacity,
		AccountConcurrency: map[int64]int{
			accountID: capacity,
		},
		BuiltAt: time.Now().UTC(),
	})
	if err != nil {
		b.Fatalf("seed scheduler admission capacity projection: %v", err)
	}
	return service.NewSchedulerSnapshotService(schedulerCache, nil, nil, nil, nil)
}

func runIssue8GatewayLifecycle(
	b *testing.B,
	ctx context.Context,
	admission *service.GatewayAdmission,
	request service.GatewayAdmissionRequest,
	account *service.Account,
	expectedClass service.AdmissionClass,
) {
	b.Helper()

	session, err := admission.Begin(ctx, request)
	if err != nil {
		b.Fatalf("begin gateway admission: %v", err)
	}
	if class := session.Class(); class != expectedClass {
		session.Close()
		b.Fatalf("gateway admission class: got %q want %q", class, expectedClass)
	}
	target, err := session.NextTarget(ctx, service.GatewayTargetRequest{
		Selector: service.GatewayTargetSelectorFunc(func(
			ctx context.Context,
			claimer service.TargetClaimer,
		) (*service.AccountSelectionResult, error) {
			release, acquired, claimErr := claimer.TryClaim(ctx, service.TargetClaimRequest{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountConcurrency: account.Concurrency,
			})
			return &service.AccountSelectionResult{
				Account:     account,
				Acquired:    acquired,
				ReleaseFunc: release,
			}, claimErr
		}),
	})
	if err != nil {
		session.Close()
		b.Fatalf("select gateway target: %v", err)
	}
	err = target.Dispatch(ctx, nil, func(context.Context, *service.Account) error { return nil })
	session.Close()
	if err != nil {
		b.Fatalf("dispatch gateway target: %v", err)
	}
}

func cleanupIssue8BenchmarkKeys(
	b *testing.B,
	rdb *redis.Client,
	platform string,
	userIDs []int64,
	accountIDs []int64,
) {
	b.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	keys := make([]string, 0, len(userIDs)*9+len(accountIDs)*8)
	for _, userID := range userIDs {
		keys = append(keys, gatewayAdmissionUserLeaseKeys(userID)...)
		keys = append(keys, userSlotKey(userID), waitQueueKey(userID))
		if err := rdb.ZRem(ctx, userActiveIndexKey, strconv.FormatInt(userID, 10)).Err(); err != nil {
			b.Fatalf("cleanup user active index: %v", err)
		}
	}
	for _, accountID := range accountIDs {
		if platform != "" {
			keys = append(keys, gatewayAdmissionTargetLeaseKeys(platform, accountID)...)
		}
		keys = append(keys, accountSlotKey(accountID), accountWaitKey(accountID))
		if err := rdb.ZRem(ctx, accountActiveIndexKey, strconv.FormatInt(accountID, 10)).Err(); err != nil {
			b.Fatalf("cleanup account active index: %v", err)
		}
	}
	if platform != "" {
		keys = append(keys, schedulerCapacityPrefix+platform)
	}
	if len(keys) > 0 {
		if err := rdb.Unlink(ctx, keys...).Err(); err != nil {
			b.Fatalf("cleanup benchmark keys: %v", err)
		}
	}
}
