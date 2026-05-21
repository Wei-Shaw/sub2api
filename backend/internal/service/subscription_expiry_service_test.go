package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionExpiryServiceDeduplicatesSameBucketAcrossTicks(t *testing.T) {
	repo := newExpiryReminderSubscriptionRepo(expiryReminderTestSubscription(1, time.Now().Add(7*24*time.Hour+2*time.Hour)))
	notificationSvc, smtpServer := newSubscriptionExpiryTestNotificationService(t)
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetNotificationEmailService(notificationSvc)

	for i := 0; i < 5; i++ {
		svc.runOnce()
	}

	require.Equal(t, int64(1), smtpServer.messageCount())
	require.Len(t, repo.marks, 1)
	require.Equal(t, "7d", repo.marks[0].reminderKey)
}

func TestSubscriptionExpiryServiceSendsEachReminderBucketOnce(t *testing.T) {
	firstExpiry := time.Now().Add(7*24*time.Hour + 2*time.Hour)
	repo := newExpiryReminderSubscriptionRepo(expiryReminderTestSubscription(1, firstExpiry))
	notificationSvc, smtpServer := newSubscriptionExpiryTestNotificationService(t)
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetNotificationEmailService(notificationSvc)

	svc.runOnce()
	repo.setExpiresAt(t, 1, time.Now().Add(3*24*time.Hour+2*time.Hour))
	svc.runOnce()
	repo.setExpiresAt(t, 1, time.Now().Add(24*time.Hour+2*time.Hour))
	svc.runOnce()

	require.Equal(t, int64(3), smtpServer.messageCount())
	require.Len(t, repo.marks, 3)
	require.Equal(t, []string{"7d", "3d", "1d"}, []string{
		repo.marks[0].reminderKey,
		repo.marks[1].reminderKey,
		repo.marks[2].reminderKey,
	})
}

func TestSubscriptionExpiryServiceAllowsSameBucketAfterRenewal(t *testing.T) {
	firstExpiry := time.Now().Add(7*24*time.Hour + 2*time.Hour)
	secondExpiry := time.Now().Add(7*24*time.Hour + 4*time.Hour)
	repo := newExpiryReminderSubscriptionRepo(expiryReminderTestSubscription(1, firstExpiry))
	notificationSvc, smtpServer := newSubscriptionExpiryTestNotificationService(t)
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetNotificationEmailService(notificationSvc)

	svc.runOnce()
	repo.setExpiresAt(t, 1, secondExpiry)
	svc.runOnce()

	require.Equal(t, int64(2), smtpServer.messageCount())
	require.Len(t, repo.marks, 2)
	require.Equal(t, "7d", repo.marks[0].reminderKey)
	require.Equal(t, "7d", repo.marks[1].reminderKey)
	require.True(t, repo.marks[0].expiresAt.Equal(firstExpiry))
	require.True(t, repo.marks[1].expiresAt.Equal(secondExpiry))
}

func newSubscriptionExpiryTestNotificationService(t *testing.T) (*NotificationEmailService, *notificationEmailTestSMTPServer) {
	t.Helper()
	repo := newNotificationEmailMemorySettingRepo()
	smtpServer := startNotificationEmailTestSMTPServer(t)
	require.NoError(t, repo.SetMultiple(context.Background(), smtpServer.settings()))
	emailSvc := NewEmailService(repo, nil)
	return NewNotificationEmailService(repo, emailSvc), smtpServer
}

func expiryReminderTestSubscription(id int64, expiresAt time.Time) UserSubscription {
	return UserSubscription{
		ID:        id,
		UserID:    id * 10,
		GroupID:   id * 100,
		StartsAt:  time.Now().Add(-24 * time.Hour),
		ExpiresAt: expiresAt,
		Status:    SubscriptionStatusActive,
		User: &User{
			ID:       id * 10,
			Email:    "user@example.com",
			Username: "user",
		},
		Group: &Group{
			ID:   id * 100,
			Name: "Pro",
		},
	}
}

type expiryReminderSubscriptionRepo struct {
	UserSubscriptionRepository
	subs  []UserSubscription
	marks []expiryReminderMark
}

type expiryReminderMark struct {
	subscriptionID int64
	reminderKey    string
	expiresAt      time.Time
	sentAt         time.Time
}

func newExpiryReminderSubscriptionRepo(sub UserSubscription) *expiryReminderSubscriptionRepo {
	return &expiryReminderSubscriptionRepo{subs: []UserSubscription{sub}}
}

func (r *expiryReminderSubscriptionRepo) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	return 0, nil
}

func (r *expiryReminderSubscriptionRepo) List(_ context.Context, params pagination.PaginationParams, _, _ *int64, _, _, _, _ string) ([]UserSubscription, *pagination.PaginationResult, error) {
	subs := make([]UserSubscription, len(r.subs))
	copy(subs, r.subs)
	return subs, &pagination.PaginationResult{
		Total:    int64(len(subs)),
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    1,
	}, nil
}

func (r *expiryReminderSubscriptionRepo) MarkExpiryReminderSent(_ context.Context, subscriptionID int64, reminderKey string, expiresAt, sentAt time.Time) error {
	for i := range r.subs {
		if r.subs[i].ID != subscriptionID {
			continue
		}
		r.subs[i].ExpiryReminderKey = reminderKey
		r.subs[i].ExpiryReminderExpiresAt = &expiresAt
		r.subs[i].ExpiryReminderSentAt = &sentAt
		r.marks = append(r.marks, expiryReminderMark{
			subscriptionID: subscriptionID,
			reminderKey:    reminderKey,
			expiresAt:      expiresAt,
			sentAt:         sentAt,
		})
		return nil
	}
	return ErrSubscriptionNotFound
}

func (r *expiryReminderSubscriptionRepo) setExpiresAt(t *testing.T, subscriptionID int64, expiresAt time.Time) {
	t.Helper()
	for i := range r.subs {
		if r.subs[i].ID == subscriptionID {
			r.subs[i].ExpiresAt = expiresAt
			return
		}
	}
	t.Fatalf("subscription %d not found", subscriptionID)
}
