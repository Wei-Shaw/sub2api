//go:build unit

package service

import (
	"context"
	"errors"
	"net/textproto"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeNotificationEmailDeliveryRepository struct {
	mu         sync.Mutex
	items      []NotificationEmailDelivery
	byDedup    map[string]int64
	nextID     int64
	enqueueErr error
}

func newFakeNotificationEmailDeliveryRepository() *fakeNotificationEmailDeliveryRepository {
	return &fakeNotificationEmailDeliveryRepository{byDedup: map[string]int64{}, nextID: 1}
}

func (r *fakeNotificationEmailDeliveryRepository) Enqueue(_ context.Context, delivery NotificationEmailDelivery) (int64, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.enqueueErr != nil {
		return 0, false, r.enqueueErr
	}
	if id, ok := r.byDedup[delivery.DedupKey]; ok {
		return id, false, nil
	}
	id := r.nextID
	r.nextID++
	delivery.ID = id
	delivery.CreatedAt = time.Now().UTC()
	delivery.UpdatedAt = delivery.CreatedAt
	delivery.NextAttemptAt = delivery.CreatedAt
	r.items = append(r.items, delivery)
	r.byDedup[delivery.DedupKey] = id
	return id, true, nil
}

func (r *fakeNotificationEmailDeliveryRepository) Claim(context.Context, string, int, time.Duration) ([]NotificationEmailDelivery, error) {
	return nil, nil
}
func (r *fakeNotificationEmailDeliveryRepository) MarkSent(context.Context, int64, string) error {
	return nil
}
func (r *fakeNotificationEmailDeliveryRepository) MarkSuppressed(context.Context, int64, string, string, string) error {
	return nil
}
func (r *fakeNotificationEmailDeliveryRepository) MarkFailed(context.Context, int64, string, string, string, *time.Time) error {
	return nil
}
func (r *fakeNotificationEmailDeliveryRepository) List(_ context.Context, filter NotificationEmailDeliveryListFilter) (NotificationEmailDeliveryListResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := append([]NotificationEmailDelivery(nil), r.items...)
	return NotificationEmailDeliveryListResult{Items: items, Total: int64(len(items))}, nil
}
func (r *fakeNotificationEmailDeliveryRepository) Retry(context.Context, int64) (bool, error) {
	return true, nil
}
func (r *fakeNotificationEmailDeliveryRepository) Stats(context.Context) (NotificationEmailDeliveryStats, error) {
	return NotificationEmailDeliveryStats{}, nil
}
func (r *fakeNotificationEmailDeliveryRepository) DeleteTerminalBefore(context.Context, time.Time, time.Time, int) (int64, error) {
	return 0, nil
}

func TestNotificationEmailDispatcherCreatesStableDeduplicatedEnvelope(t *testing.T) {
	ctx := context.Background()
	settings := newNotificationEmailMemorySettingRepo()
	email := NewNotificationEmailService(settings, nil)
	repo := newFakeNotificationEmailDeliveryRepository()
	dispatcher := NewNotificationEmailDispatcher(repo, email)

	input := NotificationEmailSendInput{
		Event: NotificationEmailEventOpsAlert, RecipientEmail: "Ops@Example.com",
		SourceType: "ops_incident", SourceID: "incident-42", ReminderKey: "firing:1",
		Variables: map[string]string{"rule_name": "upstream errors"},
	}
	first, err := dispatcher.Enqueue(ctx, input)
	require.NoError(t, err)
	require.True(t, first.Created)
	second, err := dispatcher.Enqueue(ctx, input)
	require.NoError(t, err)
	require.False(t, second.Created)
	require.Equal(t, first.ID, second.ID)
	require.Len(t, repo.items, 1)
	require.Equal(t, "ops@example.com", repo.items[0].RecipientEmail)
	require.Equal(t, NotificationEmailChannelOpsAlert, repo.items[0].Channel)
}

func TestNotificationEmailDispatcherRequiresStableBusinessSource(t *testing.T) {
	settings := newNotificationEmailMemorySettingRepo()
	dispatcher := NewNotificationEmailDispatcher(newFakeNotificationEmailDeliveryRepository(), NewNotificationEmailService(settings, nil))
	_, err := dispatcher.Enqueue(context.Background(), NotificationEmailSendInput{
		Event: NotificationEmailEventAuthVerifyCode, RecipientEmail: "user@example.com",
	})
	require.ErrorContains(t, err, "source_type and source_id")
}

func TestNotificationEmailDispatcherRejectsUndeclaredPayload(t *testing.T) {
	settings := newNotificationEmailMemorySettingRepo()
	dispatcher := NewNotificationEmailDispatcher(newFakeNotificationEmailDeliveryRepository(), NewNotificationEmailService(settings, nil))
	_, err := dispatcher.Enqueue(context.Background(), NotificationEmailSendInput{
		Event: NotificationEmailEventOpsAlert, RecipientEmail: "ops@example.com",
		SourceType: "ops_incident", SourceID: "42", Variables: map[string]string{"smtp_password": "must-not-persist"},
	})
	require.ErrorContains(t, err, "unsupported notification email variable")
}

func TestNotificationEmailDeliveryViewMasksRecipientAndPayload(t *testing.T) {
	repo := newFakeNotificationEmailDeliveryRepository()
	repo.items = []NotificationEmailDelivery{{
		ID: 1, Event: NotificationEmailEventOpsAlert, Channel: NotificationEmailChannelOpsAlert,
		RecipientEmail: "alice@example.com", SourceType: "ops_incident", SourceID: "42",
		Status: NotificationEmailDeliveryStatusFailed, Variables: map[string]string{"secret": "not-for-api"},
		LastErrorCategory: "transport", LastError: "smtp timeout", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}}
	dispatcher := NewNotificationEmailDispatcher(repo, nil)
	page, err := dispatcher.List(context.Background(), NotificationEmailDeliveryListFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, "a***e@e***.com", page.Items[0].Recipient)
}

func TestNotificationEmailRetryDelayIsBounded(t *testing.T) {
	require.GreaterOrEqual(t, notificationEmailRetryDelay(1), 12*time.Second)
	require.LessOrEqual(t, notificationEmailRetryDelay(100), 40*time.Minute)
}

func TestNotificationEmailLeaseCoversClaimedBatch(t *testing.T) {
	waves := (notificationEmailDeliveryBatchSize + notificationEmailDeliveryConcurrency - 1) / notificationEmailDeliveryConcurrency
	require.Less(t, time.Duration(waves)*notificationEmailDeliverySendTimeout+notificationEmailDeliveryDBTimeout, notificationEmailDeliveryLease)
}

func TestBoundedNotificationEmailErrorRedactsAddresses(t *testing.T) {
	errorText := boundedNotificationEmailError(assertiveError("delivery to alice@example.com failed\npermanently"))
	require.Equal(t, "delivery to ***@*** failed permanently", errorText)
}

type notificationEmailWorkerTestRepository struct {
	mu                 sync.Mutex
	markedSent         int
	markedSuppressed   int
	markedFailed       int
	lastCategory       string
	lastDetail         string
	lastRetryAt        *time.Time
	cleanupCalls       int
	cleanupDeleteCount int64
}

func (r *notificationEmailWorkerTestRepository) Enqueue(context.Context, NotificationEmailDelivery) (int64, bool, error) {
	return 1, true, nil
}
func (r *notificationEmailWorkerTestRepository) Claim(context.Context, string, int, time.Duration) ([]NotificationEmailDelivery, error) {
	return nil, nil
}
func (r *notificationEmailWorkerTestRepository) MarkSent(context.Context, int64, string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.markedSent++
	return nil
}
func (r *notificationEmailWorkerTestRepository) MarkSuppressed(_ context.Context, _ int64, _, category, detail string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.markedSuppressed++
	r.lastCategory = category
	r.lastDetail = detail
	return nil
}
func (r *notificationEmailWorkerTestRepository) MarkFailed(_ context.Context, _ int64, _, category, detail string, retryAt *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.markedFailed++
	r.lastCategory = category
	r.lastDetail = detail
	r.lastRetryAt = retryAt
	return nil
}
func (r *notificationEmailWorkerTestRepository) List(context.Context, NotificationEmailDeliveryListFilter) (NotificationEmailDeliveryListResult, error) {
	return NotificationEmailDeliveryListResult{}, nil
}
func (r *notificationEmailWorkerTestRepository) Retry(context.Context, int64) (bool, error) {
	return false, nil
}
func (r *notificationEmailWorkerTestRepository) Stats(context.Context) (NotificationEmailDeliveryStats, error) {
	return NotificationEmailDeliveryStats{}, nil
}
func (r *notificationEmailWorkerTestRepository) DeleteTerminalBefore(context.Context, time.Time, time.Time, int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupCalls++
	if r.cleanupCalls == 1 {
		return r.cleanupDeleteCount, nil
	}
	return 0, nil
}

func testNotificationEmailWorker(repo NotificationEmailDeliveryRepository, deliver func(context.Context, NotificationEmailSendInput, bool) (bool, error)) *NotificationEmailDeliveryWorker {
	worker := NewNotificationEmailDeliveryWorker(repo, nil)
	worker.deliver = deliver
	return worker
}

func testWorkerDelivery(attempt, maxAttempts int) NotificationEmailDelivery {
	return NotificationEmailDelivery{
		ID: 7, Event: NotificationEmailEventOpsAlert, RecipientEmail: "ops@example.com",
		SourceType: "ops_incident", SourceID: "incident-7", Status: NotificationEmailDeliveryStatusProcessing,
		AttemptCount: attempt, MaxAttempts: maxAttempts, Variables: map[string]string{"rule_name": "errors"},
	}
}

func TestNotificationEmailWorkerRetriesTransientFailureThenStopsAtMaximum(t *testing.T) {
	repo := &notificationEmailWorkerTestRepository{}
	worker := testNotificationEmailWorker(repo, func(context.Context, NotificationEmailSendInput, bool) (bool, error) {
		return false, notificationEmailDeliveryErr(errors.New("smtp timeout"))
	})

	worker.processDelivery(testWorkerDelivery(1, 2))
	require.Equal(t, 1, repo.markedFailed)
	require.Equal(t, "transport", repo.lastCategory)
	require.NotNil(t, repo.lastRetryAt)

	repo.lastRetryAt = nil
	worker.processDelivery(testWorkerDelivery(2, 2))
	require.Equal(t, 2, repo.markedFailed)
	require.Nil(t, repo.lastRetryAt)
}

func TestNotificationEmailPermanentSMTPFailureIsNotRetried(t *testing.T) {
	repo := &notificationEmailWorkerTestRepository{}
	worker := testNotificationEmailWorker(repo, func(context.Context, NotificationEmailSendInput, bool) (bool, error) {
		return false, notificationEmailDeliveryErr(&textproto.Error{Code: 550, Msg: "mailbox unavailable"})
	})
	worker.processDelivery(testWorkerDelivery(1, 5))
	require.Equal(t, 1, repo.markedFailed)
	require.Equal(t, "transport_permanent", repo.lastCategory)
	require.Nil(t, repo.lastRetryAt)
}

func TestNotificationEmailWorkerSuppressesDisabledChannel(t *testing.T) {
	repo := &notificationEmailWorkerTestRepository{}
	worker := testNotificationEmailWorker(repo, func(context.Context, NotificationEmailSendInput, bool) (bool, error) {
		return false, ErrNotificationEmailChannelDisabled
	})
	worker.processDelivery(testWorkerDelivery(1, 5))
	require.Equal(t, 1, repo.markedSuppressed)
	require.Equal(t, "policy", repo.lastCategory)
	require.Zero(t, repo.markedFailed)
}

func TestNotificationEmailWorkerRecoversDeliveryPanic(t *testing.T) {
	repo := &notificationEmailWorkerTestRepository{}
	worker := testNotificationEmailWorker(repo, func(context.Context, NotificationEmailSendInput, bool) (bool, error) {
		panic("test panic")
	})
	worker.processDelivery(testWorkerDelivery(1, 5))
	require.Equal(t, 1, repo.markedFailed)
	require.Equal(t, "internal", repo.lastCategory)
	require.Contains(t, repo.lastDetail, "test panic")
	require.NotNil(t, repo.lastRetryAt)
}

func TestNotificationEmailWorkerCleansTerminalRowsInBatches(t *testing.T) {
	repo := &notificationEmailWorkerTestRepository{cleanupDeleteCount: notificationEmailDeliveryCleanupBatch}
	worker := testNotificationEmailWorker(repo, func(context.Context, NotificationEmailSendInput, bool) (bool, error) {
		return false, nil
	})
	deleted, err := worker.cleanupTerminalDeliveries(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, int64(notificationEmailDeliveryCleanupBatch), deleted)
	require.Equal(t, 2, repo.cleanupCalls)
}

type notificationEmailMultiWorkerRepo struct {
	NotificationEmailDeliveryRepository
	mu      sync.Mutex
	pending []NotificationEmailDelivery
	sent    map[int64]int
}

func (r *notificationEmailMultiWorkerRepo) Claim(_ context.Context, workerID string, _ int, _ time.Duration) ([]NotificationEmailDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) == 0 {
		return nil, nil
	}
	delivery := r.pending[0]
	r.pending = r.pending[1:]
	delivery.Status = NotificationEmailDeliveryStatusProcessing
	delivery.AttemptCount++
	return []NotificationEmailDelivery{delivery}, nil
}

func (r *notificationEmailMultiWorkerRepo) MarkSent(_ context.Context, id int64, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent[id]++
	return nil
}

func TestNotificationEmailMultipleWorkersDoNotDuplicateClaims(t *testing.T) {
	repo := &notificationEmailMultiWorkerRepo{
		pending: []NotificationEmailDelivery{testWorkerDelivery(0, 5), testWorkerDelivery(0, 5)},
		sent:    map[int64]int{},
	}
	repo.pending[0].ID = 1
	repo.pending[1].ID = 2
	deliver := func(context.Context, NotificationEmailSendInput, bool) (bool, error) { return false, nil }
	first := testNotificationEmailWorker(repo, deliver)
	second := testNotificationEmailWorker(repo, deliver)
	var group sync.WaitGroup
	group.Add(2)
	go func() { defer group.Done(); require.NoError(t, first.processBatch(context.Background())) }()
	go func() { defer group.Done(); require.NoError(t, second.processBatch(context.Background())) }()
	group.Wait()
	require.Equal(t, map[int64]int{1: 1, 2: 1}, repo.sent)
}

type assertiveError string

func (e assertiveError) Error() string { return string(e) }
