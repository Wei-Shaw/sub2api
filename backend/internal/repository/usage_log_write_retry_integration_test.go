//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// usageLogWriteRetryFixture creates the foreign-key rows a storable usage_logs row needs.
func usageLogWriteRetryFixture(t *testing.T, name string) *service.UsageLog {
	t.Helper()
	client := testEntClient(t)
	suffix := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	user := mustCreateUser(t, client, &service.User{Email: "usage-retry-" + suffix + "@example.com"})
	group := mustCreateGroup(t, client, &service.Group{Name: "g-usage-retry-" + suffix})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-retry-" + suffix, Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-retry-" + suffix})
	return &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		GroupID:      &group.ID,
		RequestID:    "client:" + uuid.NewString(),
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 20,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    time.Now().UTC(),
	}
}

// holdUsageLogsInsertLock stalls every INSERT into usage_logs, however the stall came about in
// production: SHARE ROW EXCLUSIVE conflicts with the ROW EXCLUSIVE lock an INSERT takes, while
// plain SELECTs stay possible.
func holdUsageLogsInsertLock(t *testing.T) (release func()) {
	t.Helper()
	tx, err := integrationDB.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	_, err = tx.Exec("LOCK TABLE usage_logs IN SHARE ROW EXCLUSIVE MODE")
	require.NoError(t, err)
	released := false
	release = func() {
		if !released {
			released = true
			require.NoError(t, tx.Rollback())
		}
	}
	t.Cleanup(release)
	return release
}

func usageLogRowCount(t *testing.T, log *service.UsageLog) int {
	t.Helper()
	var count int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM usage_logs WHERE request_id = $1 AND api_key_id = $2",
		log.RequestID, log.APIKeyID).Scan(&count))
	return count
}

// A stall that outlasts several attempts and the caller's deadline but not the retry budget only
// delays the rows; none is lost, and the writer holds a single connection while rows pile up.
func TestUsageLogBestEffort_StallLongerThanCallerDeadlineOnlyDelaysRow(t *testing.T) {
	repo := newUsageLogRepositoryWithSQL(testEntClient(t), integrationDB)
	repo.bestEffortRetry = usageLogWriteRetry{
		attemptTimeout: 300 * time.Millisecond,
		baseDelay:      10 * time.Millisecond,
		maxDelay:       100 * time.Millisecond,
		budget:         time.Minute,
	}
	first := usageLogWriteRetryFixture(t, "stall-delay")
	logs := make([]*service.UsageLog, 20)
	for i := range logs {
		row := *first
		row.RequestID = "client:" + uuid.NewString()
		logs[i] = &row
	}

	release := holdUsageLogsInsertLock(t)
	callerErrs := make(chan error, len(logs))
	for _, log := range logs {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			callerErrs <- repo.CreateBestEffort(ctx, log)
		}()
	}

	// Sample the waiting sessions for longer than the caller deadline.
	sawWaiter := false
	for i := 0; i < 30; i++ {
		var lockWaiters int
		require.NoError(t, integrationDB.QueryRow(
			"SELECT COUNT(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'",
		).Scan(&lockWaiters))
		require.LessOrEqual(t, lockWaiters, 1)
		sawWaiter = sawWaiter || lockWaiters == 1
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, sawWaiter)
	for _, log := range logs {
		require.Zero(t, usageLogRowCount(t, log))
	}
	release()

	require.Eventually(t, func() bool {
		for _, log := range logs {
			if usageLogRowCount(t, log) != 1 {
				return false
			}
		}
		return true
	}, 10*time.Second, 50*time.Millisecond)
	require.Zero(t, repo.bestEffortLost.Load())
	for range logs {
		require.NoError(t, <-callerErrs)
	}
}

// A stall longer than the retry budget makes the writer give the row up and count exactly one
// loss; the row is not written once the lock is gone.
func TestUsageLogBestEffort_RetryBudgetExhaustedCountsLossOnce(t *testing.T) {
	repo := newUsageLogRepositoryWithSQL(testEntClient(t), integrationDB)
	repo.bestEffortRetry = usageLogWriteRetry{
		attemptTimeout: 300 * time.Millisecond,
		baseDelay:      10 * time.Millisecond,
		maxDelay:       50 * time.Millisecond,
		budget:         time.Second,
	}
	log := usageLogWriteRetryFixture(t, "budget")

	release := holdUsageLogsInsertLock(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := repo.CreateBestEffort(ctx, log)
	require.True(t, service.IsUsageLogCreateDropped(err), "err=%v", err)
	require.Equal(t, uint64(1), repo.bestEffortLost.Load())

	release()
	time.Sleep(time.Second)
	require.Zero(t, usageLogRowCount(t, log))
	require.Equal(t, uint64(1), repo.bestEffortLost.Load())
}

// A permanent error (foreign key violation) is isolated per row: the bad row is counted as lost
// at once without being retried, and the good row of the same batch is stored.
func TestUsageLogBestEffort_PermanentErrorIsolatedWithoutRetry(t *testing.T) {
	repo := newUsageLogRepositoryWithSQL(testEntClient(t), integrationDB)
	good := usageLogWriteRetryFixture(t, "permanent")
	bad := *good
	bad.RequestID = "client:" + uuid.NewString()
	bad.UserID = good.UserID + 1_000_000_000

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	goodErr := make(chan error, 1)
	badErr := make(chan error, 1)
	go func() { goodErr <- repo.CreateBestEffort(ctx, good) }()
	go func() { badErr <- repo.CreateBestEffort(ctx, &bad) }()

	start := time.Now()
	err := <-badErr
	require.True(t, service.IsUsageLogCreateDropped(err), "err=%v", err)
	require.Less(t, time.Since(start), 2*time.Second)
	require.NoError(t, <-goodErr)
	require.Equal(t, 1, usageLogRowCount(t, good))
	require.Zero(t, usageLogRowCount(t, &bad))
	require.Equal(t, uint64(1), repo.bestEffortLost.Load())
}
