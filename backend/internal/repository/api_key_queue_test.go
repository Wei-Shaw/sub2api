package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newAPIKeyQueueCache(t *testing.T) (*concurrencyCache, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	// The virtual clock starts at a fixed point so deadlines are stable.
	server.SetTime(time.Unix(1_800_000_000, 0))
	return &concurrencyCache{rdb: client, slotTTLSeconds: 60, waitQueueTTLSeconds: 60}, server
}

func TestAPIKeyQueueFastPathWaitingAndTransfer(t *testing.T) {
	cache, server := newAPIKeyQueueCache(t)
	ctx := context.Background()

	// Q01: an uncontended key takes the single fast path without wait state.
	acquired, nowMs, err := cache.AcquireAPIKeySlotWithTime(ctx, 1, 1, "holder")
	require.NoError(t, err)
	require.True(t, acquired)
	require.NotZero(t, nowMs)
	require.False(t, server.Exists(apiKeyWaitKey(1)), "fast path must not create wait state")

	// A second request gets BUSY and the Redis timestamp from the same script.
	acquired, busyNowMs, err := cache.AcquireAPIKeySlotWithTime(ctx, 1, 1, "waiter")
	require.NoError(t, err)
	require.False(t, acquired)
	require.GreaterOrEqual(t, busyNowMs, nowMs)
	deadlineMs := busyNowMs + 30_000

	// ENTER registers one ticket with the fixed deadline.
	outcome, err := cache.AdvanceAPIKeyQueue(ctx, 1, "waiter", service.APIKeyQueueModeEnter, deadlineMs, 1, 20)
	require.NoError(t, err)
	require.Equal(t, service.APIKeyQueueOutcomeWaiting, outcome)
	active, waiting, err := cache.GetAPIKeyQueueStats(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, active)
	require.Equal(t, 1, waiting)

	// Q04: repeated POLL keeps the same member, deadline and counts.
	for i := 0; i < 3; i++ {
		outcome, err = cache.AdvanceAPIKeyQueue(ctx, 1, "waiter", service.APIKeyQueueModePoll, deadlineMs, 1, 20)
		require.NoError(t, err)
		require.Equal(t, service.APIKeyQueueOutcomeWaiting, outcome)
	}
	_, waiting, err = cache.GetAPIKeyQueueStats(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, waiting)

	// Queue full rejects an extra attempt without touching existing tickets.
	outcome, err = cache.AdvanceAPIKeyQueue(ctx, 1, "third", service.APIKeyQueueModeEnter, deadlineMs, 1, 1)
	require.NoError(t, err)
	require.Equal(t, service.APIKeyQueueOutcomeQueueFull, outcome)

	// Q03: a freed slot is transferred atomically on the next POLL.
	require.NoError(t, cache.ReleaseAPIKeySlot(ctx, 1, "holder"))
	outcome, err = cache.AdvanceAPIKeyQueue(ctx, 1, "waiter", service.APIKeyQueueModePoll, deadlineMs, 1, 20)
	require.NoError(t, err)
	require.Equal(t, service.APIKeyQueueOutcomeAcquired, outcome)
	active, waiting, err = cache.GetAPIKeyQueueStats(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, active)
	require.Zero(t, waiting, "no window where the same attempt is both active and waiting")

	// Q08b: closing the waiting stage after a confirmed ACQUIRED must keep the
	// regular member (the new reservation) while still fencing late ENTER/POLL.
	require.NoError(t, cache.AbortAPIKeyQueueAttempt(ctx, 1, "waiter", deadlineMs, false))
	require.NoError(t, cache.rdb.ZScore(ctx, apiKeySlotKey(1), "waiter").Err(),
		"confirmed grant must survive the waiting-stage fence")
	outcome, err = cache.AdvanceAPIKeyQueue(ctx, 1, "waiter", service.APIKeyQueueModeEnter, deadlineMs, 1, 20)
	require.NoError(t, err)
	require.Equal(t, service.APIKeyQueueOutcomeAcquired, outcome,
		"a late ENTER idempotently confirms the existing regular member")

	// Cancel-style cleanup removes the member explicitly; afterwards the fence
	// prevents re-admission.
	require.NoError(t, cache.AbortAPIKeyQueueAttempt(ctx, 1, "waiter", deadlineMs, true))
	require.ErrorIs(t, cache.rdb.ZScore(ctx, apiKeySlotKey(1), "waiter").Err(), redis.Nil)
	outcome, err = cache.AdvanceAPIKeyQueue(ctx, 1, "waiter", service.APIKeyQueueModeEnter, deadlineMs, 1, 20)
	require.NoError(t, err)
	require.Equal(t, service.APIKeyQueueOutcomeAborted, outcome)
	require.ErrorIs(t, cache.rdb.ZScore(ctx, apiKeySlotKey(1), "waiter").Err(), redis.Nil,
		"fenced attempt must not recreate the regular member")
}

func TestAPIKeyQueueTimeoutTicketLostAndPolicyChanged(t *testing.T) {
	cache, server := newAPIKeyQueueCache(t)
	ctx := context.Background()
	_, nowMs, err := cache.AcquireAPIKeySlotWithTime(ctx, 2, 1, "holder")
	require.NoError(t, err)
	require.True(t, server.Exists(apiKeySlotKey(2)))

	t.Run("timeout prunes waiting member", func(t *testing.T) {
		deadlineMs := nowMs + 1_000
		outcome, err := cache.AdvanceAPIKeyQueue(ctx, 2, "timed", service.APIKeyQueueModeEnter, deadlineMs, 1, 20)
		require.NoError(t, err)
		require.Equal(t, service.APIKeyQueueOutcomeWaiting, outcome)
		server.SetTime(time.Unix(1_800_000_002, 0))
		outcome, err = cache.AdvanceAPIKeyQueue(ctx, 2, "timed", service.APIKeyQueueModePoll, deadlineMs, 1, 20)
		require.NoError(t, err)
		require.Equal(t, service.APIKeyQueueOutcomeTimeout, outcome)
		_, waiting, err := cache.GetAPIKeyQueueStats(ctx, 2)
		require.NoError(t, err)
		require.Zero(t, waiting)
	})

	t.Run("ticket lost does not re-queue", func(t *testing.T) {
		deadlineMs := nowMs + 30_000
		outcome, err := cache.AdvanceAPIKeyQueue(ctx, 2, "lost", service.APIKeyQueueModeEnter, deadlineMs, 1, 20)
		require.NoError(t, err)
		require.Equal(t, service.APIKeyQueueOutcomeWaiting, outcome)
		require.NoError(t, cache.rdb.ZRem(ctx, apiKeyWaitKey(2), "lost").Err())
		outcome, err = cache.AdvanceAPIKeyQueue(ctx, 2, "lost", service.APIKeyQueueModePoll, deadlineMs, 1, 20)
		require.NoError(t, err)
		require.Equal(t, service.APIKeyQueueOutcomeTicketLost, outcome)
		require.ErrorIs(t, cache.rdb.ZScore(ctx, apiKeyWaitKey(2), "lost").Err(), redis.Nil,
			"ticket must not be recreated")
	})

	t.Run("limit changed to zero", func(t *testing.T) {
		outcome, err := cache.AdvanceAPIKeyQueue(ctx, 2, "policy", service.APIKeyQueueModeEnter, nowMs+30_000, 0, 20)
		require.NoError(t, err)
		require.Equal(t, service.APIKeyQueueOutcomePolicyChanged, outcome)
	})

	t.Run("queue disabled reports limit reached", func(t *testing.T) {
		outcome, err := cache.AdvanceAPIKeyQueue(ctx, 2, "noqueue", service.APIKeyQueueModeEnter, nowMs+30_000, 1, 0)
		require.NoError(t, err)
		require.Equal(t, service.APIKeyQueueOutcomeLimitReached, outcome)
	})
}

func TestAPIKeyQueueCancelBeforeLateEnter(t *testing.T) {
	cache, server := newAPIKeyQueueCache(t)
	ctx := context.Background()
	_, nowMs, err := cache.AcquireAPIKeySlotWithTime(ctx, 3, 1, "holder")
	require.NoError(t, err)
	deadlineMs := nowMs + 30_000

	// Cancellation cleanup runs before the queued ENTER reaches Redis.
	require.NoError(t, cache.AbortAPIKeyQueueAttempt(ctx, 3, "cancelled", deadlineMs, true))
	outcome, err := cache.AdvanceAPIKeyQueue(ctx, 3, "cancelled", service.APIKeyQueueModeEnter, deadlineMs, 1, 20)
	require.NoError(t, err)
	require.Equal(t, service.APIKeyQueueOutcomeAborted, outcome)
	require.False(t, server.Exists(apiKeyWaitKey(3)), "cancelled ticket must not be recreated")
}

func TestAPIKeyLiveLeaseTransferMovesExactDimensions(t *testing.T) {
	cache, _ := newAPIKeyQueueCache(t)
	ctx := context.Background()
	const keyID = int64(9)

	// Two ordinary reservations per dimension. The transfer must move each
	// exact member instead of adding another allowance for the same dimension.
	for _, ids := range []struct {
		account int64
		user    int64
		key     string
	}{{101, 201, "key-1"}, {102, 202, "key-2"}} {
		acquired, err := cache.AcquireAccountSlot(ctx, ids.account, 2, "acct-"+ids.key)
		require.NoError(t, err)
		require.True(t, acquired)
		acquired, err = cache.AcquireUserSlot(ctx, ids.user, 2, "user-"+ids.key)
		require.NoError(t, err)
		require.True(t, acquired)
		acquired, _, err = cache.AcquireAPIKeySlotWithTime(ctx, keyID, 2, ids.key)
		require.NoError(t, err)
		require.True(t, acquired)
	}

	transfer := func(account, user int64, accountReq, userReq, keyReq, lease string) (bool, error) {
		return cache.AcquireLiveLeaseTransferring(ctx, service.LiveLeaseTransferRequest{
			AccountID:        account,
			AccountMax:       2,
			AccountRequestID: accountReq,
			UserID:           user,
			UserMax:          2,
			UserRequestID:    userReq,
			APIKeyID:         keyID,
			APIKeyMax:        2,
			KeyRequestID:     keyReq,
			LeaseID:          lease,
		})
	}

	acquired, err := transfer(101, 201, "acct-key-1", "user-key-1", "key-1", "live-1")
	require.NoError(t, err)
	require.True(t, acquired)
	accountCount, err := cache.GetAccountConcurrency(ctx, 101)
	require.NoError(t, err)
	require.Equal(t, 1, accountCount, "Live member counts exactly once for the account")
	userCount, err := cache.GetUserConcurrency(ctx, 201)
	require.NoError(t, err)
	require.Equal(t, 1, userCount, "Live member counts exactly once for the user")
	active, waiting, err := cache.GetAPIKeyQueueStats(ctx, keyID)
	require.NoError(t, err)
	require.Equal(t, 2, active, "one Live member plus the still-held second reservation")
	require.Zero(t, waiting)
	require.False(t, serverExistsKey(t, cache, keyID, "key-1"), "transferred regular key member is gone")

	// The account dimension has exactly one free slot left: the Live member is
	// not double counted alongside a second allowance.
	acquired, err = cache.AcquireAccountSlot(ctx, 101, 2, "ordinary-after-live")
	require.NoError(t, err)
	require.True(t, acquired)
	blocked, err := cache.AcquireAccountSlot(ctx, 101, 2, "ordinary-blocked")
	require.NoError(t, err)
	require.False(t, blocked, "Live member must occupy account capacity exactly once")
	require.NoError(t, cache.ReleaseAccountSlot(ctx, 101, "ordinary-after-live"))

	// A second Live transfer still fits; a third key reservation does not.
	acquired, err = transfer(102, 202, "acct-key-2", "user-key-2", "key-2", "live-2")
	require.NoError(t, err)
	require.True(t, acquired)
	active, _, err = cache.GetAPIKeyQueueStats(ctx, keyID)
	require.NoError(t, err)
	require.Equal(t, 2, active)
	acquired, _, err = cache.AcquireAPIKeySlotWithTime(ctx, keyID, 2, "key-3")
	require.NoError(t, err)
	require.False(t, acquired, "two Live sessions each own one Key member")

	// Idempotent replay of a completed transfer confirms without adding state.
	acquired, err = transfer(102, 202, "acct-key-2", "user-key-2", "key-2", "live-2")
	require.NoError(t, err)
	require.True(t, acquired)
	active, _, err = cache.GetAPIKeyQueueStats(ctx, keyID)
	require.NoError(t, err)
	require.Equal(t, 2, active)
}

func TestAPIKeyLiveLeaseTransferFailsClosedOnMissingSource(t *testing.T) {
	cache, _ := newAPIKeyQueueCache(t)
	ctx := context.Background()
	acquired, _, err := cache.AcquireAPIKeySlotWithTime(ctx, 9, 1, "key-reservation")
	require.NoError(t, err)
	require.True(t, acquired)

	// Account max > 0 but the exact account member is missing: fail closed and
	// never transfer the key source.
	_, err = cache.AcquireLiveLeaseTransferring(ctx, service.LiveLeaseTransferRequest{
		AccountID:        10,
		AccountMax:       1,
		AccountRequestID: "missing-account",
		UserID:           20,
		UserMax:          1,
		UserRequestID:    "missing-user",
		APIKeyID:         9,
		APIKeyMax:        1,
		KeyRequestID:     "key-reservation",
		LeaseID:          "live-missing",
	})
	require.ErrorIs(t, err, service.ErrLiveLeaseSourceLost)
	require.NoError(t, cache.rdb.ZScore(ctx, apiKeySlotKey(9), "key-reservation").Err(),
		"key source must survive a failed transfer")

	// A missing key reservation fails closed instead of inventing capacity.
	_, err = cache.AcquireLiveLeaseTransferring(ctx, service.LiveLeaseTransferRequest{
		AccountID:    10,
		UserID:       20,
		APIKeyID:     9,
		APIKeyMax:    1,
		KeyRequestID: "missing-key",
		LeaseID:      "live-2",
	})
	require.ErrorIs(t, err, service.ErrAPIKeyReservationLost)
	require.NoError(t, cache.rdb.ZScore(ctx, apiKeySlotKey(9), "key-reservation").Err())
}

func TestAPIKeyLiveLeaseTransferAbortFencesExactMembers(t *testing.T) {
	cache, _ := newAPIKeyQueueCache(t)
	ctx := context.Background()
	acquired, err := cache.AcquireAccountSlot(ctx, 10, 1, "acct")
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = cache.AcquireUserSlot(ctx, 20, 1, "user")
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, _, err = cache.AcquireAPIKeySlotWithTime(ctx, 9, 1, "key")
	require.NoError(t, err)
	require.True(t, acquired)

	request := service.LiveLeaseTransferRequest{
		AccountID: 10, AccountMax: 1, AccountRequestID: "acct",
		UserID: 20, UserMax: 1, UserRequestID: "user",
		APIKeyID: 9, APIKeyMax: 1, KeyRequestID: "key",
		LeaseID: "live-aborted",
	}
	require.NoError(t, cache.AbortLiveLeaseTransfer(ctx, request))

	// Exact members are removed and the fence blocks a late transfer with the
	// same leaseID.
	for _, key := range []string{accountSlotKey(10), userSlotKey(20), apiKeySlotKey(9)} {
		require.EqualValues(t, 0, cache.rdb.ZCard(ctx, key).Val(), "key %s must be empty", key)
	}
	_, err = cache.AcquireLiveLeaseTransferring(ctx, request)
	require.ErrorIs(t, err, service.ErrLiveLeaseTransferFenced)
	require.EqualValues(t, 0, cache.rdb.ZCard(ctx, liveAccountSlotKey(10)).Val())
	require.EqualValues(t, 0, cache.rdb.ZCard(ctx, liveUserSlotKey(20)).Val())
	require.EqualValues(t, 0, cache.rdb.ZCard(ctx, liveAPIKeySlotKey(9)).Val())
}

func serverExistsKey(t *testing.T, cache *concurrencyCache, apiKeyID int64, member string) bool {
	t.Helper()
	return cache.rdb.ZScore(context.Background(), apiKeySlotKey(apiKeyID), member).Err() == nil
}

func TestAPIKeyLiveLeaseTransferMovesStatsOnlyMember(t *testing.T) {
	cache, _ := newAPIKeyQueueCache(t)
	ctx := context.Background()

	require.NoError(t, cache.TrackAPIKeySlot(ctx, 9, "stats-member"))
	require.True(t, serverExistsKey(t, cache, 9, "stats-member"))

	acquired, err := cache.AcquireLiveLeaseTransferring(ctx, service.LiveLeaseTransferRequest{
		AccountID: 10, AccountMax: 0,
		UserID: 20, UserMax: 0,
		APIKeyID: 9, APIKeyMax: 0,
		KeyRequestID: "stats-member",
		LeaseID:      "live-stats",
	})
	require.NoError(t, err)
	require.True(t, acquired)
	require.False(t, serverExistsKey(t, cache, 9, "stats-member"),
		"stats-only member becomes the Live member in the same atomic step")
	active, waiting, err := cache.GetAPIKeyQueueStats(ctx, 9)
	require.NoError(t, err)
	require.Equal(t, 1, active)
	require.Zero(t, waiting)
}

func TestAPIKeyLiveLeaseMigrateAccountKeepsJointLease(t *testing.T) {
	cache, _ := newAPIKeyQueueCache(t)
	ctx := context.Background()

	acquired, err := cache.AcquireAccountSlot(ctx, 10, 1, "acct-a")
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = cache.AcquireUserSlot(ctx, 20, 1, "user-20")
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, _, err = cache.AcquireAPIKeySlotWithTime(ctx, 9, 1, "key-9")
	require.NoError(t, err)
	require.True(t, acquired)

	acquired, err = cache.AcquireLiveLeaseTransferring(ctx, service.LiveLeaseTransferRequest{
		AccountID: 10, AccountMax: 1, AccountRequestID: "acct-a",
		UserID: 20, UserMax: 1, UserRequestID: "user-20",
		APIKeyID: 9, APIKeyMax: 1, KeyRequestID: "key-9",
		LeaseID: "joint-live",
	})
	require.NoError(t, err)
	require.True(t, acquired)

	// The joint user member blocks a competing request at limit 1.
	blocked, err := cache.AcquireUserSlot(ctx, 20, 1, "competing-1")
	require.NoError(t, err)
	require.False(t, blocked)

	// Simulate a retryable SDP failure: only the account member is released.
	require.NoError(t, cache.ReleaseLiveLeaseAccount(ctx, 10, "joint-live"))
	blocked, err = cache.AcquireUserSlot(ctx, 20, 1, "competing-2")
	require.NoError(t, err)
	require.False(t, blocked, "joint user ownership survives the SDP retry")
	keyLive, err := cache.rdb.ZScore(ctx, liveAPIKeySlotKey(9), "joint-live").Result()
	require.NoError(t, err)
	require.NotZero(t, keyLive, "joint Key ownership survives the SDP retry")

	// New selection: migrate the joint lease to account 11.
	acquired, err = cache.AcquireAccountSlot(ctx, 11, 1, "acct-b")
	require.NoError(t, err)
	require.True(t, acquired)
	migrated, err := cache.MigrateLiveLeaseAccount(ctx, service.LiveLeaseTransferRequest{
		AccountID: 11, AccountMax: 1, AccountRequestID: "acct-b",
		UserID: 20, APIKeyID: 9,
		LeaseID: "joint-live", ReplacedAccountID: 10,
	})
	require.NoError(t, err)
	require.True(t, migrated)

	accountCount, err := cache.GetAccountConcurrency(ctx, 11)
	require.NoError(t, err)
	require.Equal(t, 1, accountCount, "migrated account counts exactly once")
	userCount, err := cache.GetUserConcurrency(ctx, 20)
	require.NoError(t, err)
	require.Equal(t, 1, userCount, "joint user counts exactly once, never twice")
	active, waiting, err := cache.GetAPIKeyQueueStats(ctx, 9)
	require.NoError(t, err)
	require.Equal(t, 1, active)
	require.Zero(t, waiting)
	blocked, err = cache.AcquireUserSlot(ctx, 20, 1, "competing-3")
	require.NoError(t, err)
	require.False(t, blocked, "competing user still cannot exceed the user limit")
	otherUser, err := cache.AcquireUserSlot(ctx, 20, 1, "competing-4")
	require.NoError(t, err)
	require.False(t, otherUser)
	require.EqualValues(t, 0, cache.rdb.ZCard(ctx, userSlotKey(20)).Val(),
		"ordinary user reservation was consumed by the transfer, not retained alongside Live")
}

func TestAPIKeyLiveLeaseMigrateAccountFailsClosed(t *testing.T) {
	cache, _ := newAPIKeyQueueCache(t)
	ctx := context.Background()

	acquired, _, err := cache.AcquireAPIKeySlotWithTime(ctx, 9, 0, "key-9")
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = cache.AcquireLiveLeaseTransferring(ctx, service.LiveLeaseTransferRequest{
		AccountID: 10, AccountMax: 0,
		UserID: 20, UserMax: 0,
		APIKeyID: 9, APIKeyMax: 0, KeyRequestID: "key-9",
		LeaseID: "joint-live",
	})
	require.NoError(t, err)
	require.True(t, acquired)

	// The joint user member disappears: migration must fail closed.
	require.NoError(t, cache.rdb.ZRem(ctx, liveUserSlotKey(20), "joint-live").Err())
	_, err = cache.MigrateLiveLeaseAccount(ctx, service.LiveLeaseTransferRequest{
		AccountID: 11, AccountMax: 0,
		UserID: 20, APIKeyID: 9,
		LeaseID: "joint-live", ReplacedAccountID: 10,
	})
	require.ErrorIs(t, err, service.ErrLiveLeaseSourceLost)

	// The account dimension is full for the candidate: fail with capacity, keep
	// the joint members for another candidate.
	require.NoError(t, cache.rdb.ZAdd(ctx, liveUserSlotKey(20), redis.Z{Score: float64(time.Unix(1_800_000_000, 0).Unix()), Member: "joint-live"}).Err())
	for _, member := range []string{"other", "acct-c"} {
		require.NoError(t, cache.rdb.ZAdd(ctx, accountSlotKey(12), redis.Z{Score: float64(time.Unix(1_800_000_000, 0).Unix()), Member: member}).Err())
	}
	_, err = cache.MigrateLiveLeaseAccount(ctx, service.LiveLeaseTransferRequest{
		AccountID: 12, AccountMax: 1, AccountRequestID: "acct-c",
		UserID: 20, APIKeyID: 9,
		LeaseID: "joint-live", ReplacedAccountID: 10,
	})
	require.ErrorIs(t, err, service.ErrLiveConcurrencyFull)
}

func TestAPIKeyQueueClosedFenceTTLCoversFurthestDeadline(t *testing.T) {
	cache, server := newAPIKeyQueueCache(t)
	ctx := context.Background()
	_, nowMs, err := cache.AcquireAPIKeySlotWithTime(ctx, 1, 1, "holder")
	require.NoError(t, err)

	// Attempt A fences a far deadline, then attempt B a short one.
	require.NoError(t, cache.AbortAPIKeyQueueAttempt(ctx, 1, "A", nowMs+30_000, true))
	require.NoError(t, cache.AbortAPIKeyQueueAttempt(ctx, 1, "B", nowMs+1_000, true))

	// Seven seconds later B's marker is expired but A's must survive.
	server.SetTime(time.Unix(1_800_000_007, 0))
	_, _, err = cache.GetAPIKeyQueueStats(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, cache.rdb.ZCard(ctx, apiKeyWaitClosedKey(1)).Val(),
		"short-deadline marker must not shorten the whole fence key TTL")
	require.Greater(t, server.TTL(apiKeyWaitClosedKey(1)), 20*time.Second,
		"far fence must keep its full lifetime")

	// A delayed ENTER from A must still be refused, not re-queued.
	outcome, err := cache.AdvanceAPIKeyQueue(ctx, 1, "A", service.APIKeyQueueModeEnter, nowMs+30_000, 1, 20)
	require.NoError(t, err)
	require.Equal(t, service.APIKeyQueueOutcomeAborted, outcome)
	require.ErrorIs(t, cache.rdb.ZScore(ctx, apiKeyWaitKey(1), "A").Err(), redis.Nil,
		"fenced attempt must not recreate a waiting ticket")
}

func TestAPIKeyQueueClosedFenceTTLMaxOnSuccessfulAdmit(t *testing.T) {
	cache, server := newAPIKeyQueueCache(t)
	ctx := context.Background()
	_, nowMs, err := cache.AcquireAPIKeySlotWithTime(ctx, 1, 1, "holder")
	require.NoError(t, err)
	deadlineA := nowMs + 30_000
	outcome, err := cache.AdvanceAPIKeyQueue(ctx, 1, "A", service.APIKeyQueueModeEnter, deadlineA, 1, 20)
	require.NoError(t, err)
	require.Equal(t, service.APIKeyQueueOutcomeWaiting, outcome)
	outcome, err = cache.AdvanceAPIKeyQueue(ctx, 1, "B", service.APIKeyQueueModeEnter, nowMs+1_000, 1, 20)
	require.NoError(t, err)
	require.Equal(t, service.APIKeyQueueOutcomeWaiting, outcome)

	// Seven seconds later B timed out; A acquires and writes its far fence.
	server.SetTime(time.Unix(1_800_000_007, 0))
	require.NoError(t, cache.ReleaseAPIKeySlot(ctx, 1, "holder"))
	outcome, err = cache.AdvanceAPIKeyQueue(ctx, 1, "A", service.APIKeyQueueModePoll, deadlineA, 1, 20)
	require.NoError(t, err)
	require.Equal(t, service.APIKeyQueueOutcomeAcquired, outcome)
	require.EqualValues(t, 1, cache.rdb.ZCard(ctx, apiKeyWaitClosedKey(1)).Val())
	require.Greater(t, server.TTL(apiKeyWaitClosedKey(1)), 20*time.Second,
		"successful admit must keep the full fence lifetime for its far deadline")
}

func TestAPIKeyLiveLeaseMigrateAccountConsumesNewReservation(t *testing.T) {
	cache, _ := newAPIKeyQueueCache(t)
	ctx := context.Background()
	acquired, _, err := cache.AcquireAPIKeySlotWithTime(ctx, 9, 1, "key")
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = cache.AcquireUserSlot(ctx, 20, 1, "user")
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = cache.AcquireLiveLeaseTransferring(ctx, service.LiveLeaseTransferRequest{
		AccountID: 10, AccountMax: 0,
		UserID: 20, UserMax: 1, UserRequestID: "user",
		APIKeyID: 9, APIKeyMax: 1, KeyRequestID: "key",
		LeaseID: "joint-live",
	})
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, cache.ReleaseLiveLeaseAccount(ctx, 10, "joint-live"))

	acquired, err = cache.AcquireAccountSlot(ctx, 11, 1, "acct-new")
	require.NoError(t, err)
	require.True(t, acquired)
	migrated, err := cache.MigrateLiveLeaseAccount(ctx, service.LiveLeaseTransferRequest{
		AccountID: 11, AccountMax: 1, AccountRequestID: "acct-new",
		UserID: 20, APIKeyID: 9,
		LeaseID: "joint-live", ReplacedAccountID: 10,
	})
	require.NoError(t, err)
	require.True(t, migrated)
	require.ErrorIs(t, cache.rdb.ZScore(ctx, accountSlotKey(11), "acct-new").Err(), redis.Nil,
		"new account reservation is consumed by the migration")
	accountCount, err := cache.GetAccountConcurrency(ctx, 11)
	require.NoError(t, err)
	require.Equal(t, 1, accountCount)
}

func TestAPIKeyQueueTwoClientsShareLimit(t *testing.T) {
	cache, _ := newAPIKeyQueueCache(t)
	first := service.NewConcurrencyService(cache)
	first.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 2, Timeout: 500 * time.Millisecond})
	second := service.NewConcurrencyService(cache)
	second.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 2, Timeout: 500 * time.Millisecond})

	ctx, cancelOwner := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelOwner()

	holder, err := first.ReserveAPIKeySlotWithWait(ctx, 77, 1)
	require.NoError(t, err)
	require.NotNil(t, holder)

	type result struct {
		reservation *service.APIKeySlotReservation
		err         error
	}
	done := make(chan result, 1)
	go func() {
		reservation, waitErr := second.ReserveAPIKeySlotWithWait(ctx, 77, 1)
		done <- result{reservation: reservation, err: waitErr}
	}()

	// The second instance must be waiting (not admitted) while the first holds
	// the only slot.
	require.Eventually(t, func() bool {
		active, waiting, statsErr := cache.GetAPIKeyQueueStats(context.Background(), 77)
		return statsErr == nil && active == 1 && waiting == 1
	}, 2*time.Second, 10*time.Millisecond)

	holder.Release()
	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.NotNil(t, got.reservation)
		got.reservation.Release()
	case <-time.After(2 * time.Second):
		t.Fatal("waiter did not acquire after the holder released")
	}
	active, waiting, err := cache.GetAPIKeyQueueStats(context.Background(), 77)
	require.NoError(t, err)
	require.Zero(t, active)
	require.Zero(t, waiting)
}
