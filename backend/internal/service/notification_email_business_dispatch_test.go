//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func newBusinessNotificationDispatcher() (*NotificationEmailDispatcher, *fakeNotificationEmailDeliveryRepository) {
	settings := newNotificationEmailMemorySettingRepo()
	repo := newFakeNotificationEmailDeliveryRepository()
	dispatcher := NewNotificationEmailDispatcher(repo, NewNotificationEmailService(settings, nil))
	dispatcher.encryptor = notificationEmailTestEncryptor{}
	return dispatcher, repo
}

func TestPaymentFulfillmentNotificationsUseDurableQueue(t *testing.T) {
	dispatcher, repo := newBusinessNotificationDispatcher()
	svc := &PaymentService{notificationEmailDispatcher: dispatcher}
	order := &dbent.PaymentOrder{ID: 42, UserID: 7, UserEmail: "user@example.com", UserName: "User", Amount: 12.5}

	require.NoError(t, svc.sendBalanceRechargeSuccessNotification(context.Background(), order))
	require.Len(t, repo.items, 1)
	require.Equal(t, NotificationEmailEventBalanceRechargeSuccess, repo.items[0].Event)
	require.Equal(t, "payment_order", repo.items[0].SourceType)
	require.Equal(t, "42", repo.items[0].SourceID)
}

func TestSubscriptionReminderUsesDurableQueue(t *testing.T) {
	dispatcher, repo := newBusinessNotificationDispatcher()
	svc := NewSubscriptionExpiryService(nil, time.Minute)
	svc.SetNotificationEmailDispatcher(dispatcher)
	sub := &UserSubscription{
		ID: 21, UserID: 7, ExpiresAt: time.Now().Add(6*24*time.Hour + 23*time.Hour),
		User:  &User{ID: 7, Email: "user@example.com", Username: "User"},
		Group: &Group{ID: 3, Name: "Pro"},
	}

	svc.sendExpiryReminderIfDue(context.Background(), sub)
	require.Len(t, repo.items, 1)
	require.Equal(t, NotificationEmailEventSubscriptionExpiryReminder, repo.items[0].Event)
	require.Equal(t, "user_subscription", repo.items[0].SourceType)
	require.Equal(t, "7d", repo.items[0].ReminderKey)
}

func TestBalanceAndQuotaNotificationsUseDurableQueue(t *testing.T) {
	dispatcher, repo := newBusinessNotificationDispatcher()
	svc := &BalanceNotifyService{notificationEmailDispatcher: dispatcher}

	svc.sendBalanceLowEmails([]string{"user@example.com"}, 7, "User", "user@example.com", 4.5, 5, "Sub2API", "https://example.com/top-up")
	svc.sendQuotaAlertEmails([]string{"ops@example.com"}, 11, "Account", "openai", quotaDim{name: quotaDimDaily, threshold: 90, thresholdType: thresholdTypePercentage, limit: 100}, 95, "Sub2API")

	require.Len(t, repo.items, 2)
	require.Equal(t, NotificationEmailEventBalanceLow, repo.items[0].Event)
	require.Equal(t, "balance_low", repo.items[0].SourceType)
	require.Equal(t, NotificationEmailEventAccountQuotaAlert, repo.items[1].Event)
	require.Equal(t, "account_quota", repo.items[1].SourceType)
}

func TestContentModerationNotificationsUseDurableQueue(t *testing.T) {
	dispatcher, repo := newBusinessNotificationDispatcher()
	svc := &ContentModerationService{notificationEmailDispatcher: dispatcher}
	userID := int64(7)
	log := &ContentModerationLog{ID: 88, UserID: &userID, UserEmail: "user@example.com", CreatedAt: time.Now().UTC()}

	require.NoError(t, svc.sendViolationEmail(context.Background(), &ContentModerationConfig{}, log))
	require.NoError(t, svc.sendAccountDisabledEmail(context.Background(), &ContentModerationConfig{}, log))
	require.Len(t, repo.items, 2)
	for _, delivery := range repo.items {
		require.Equal(t, "content_moderation", delivery.SourceType)
		require.Equal(t, "88", delivery.SourceID)
	}
	require.Equal(t, NotificationEmailEventContentModerationViolation, repo.items[0].Event)
	require.Equal(t, NotificationEmailEventContentModerationDisabled, repo.items[1].Event)
}
