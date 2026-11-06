package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type billingTelegramRoundTripFunc func(*http.Request) (*http.Response, error)

func (f billingTelegramRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// Unlike a context-agnostic in-memory repository, this boundary rejects expired
// persistence contexts, so cancelling delivery cannot silently lose retry state.
type notificationPersistenceRepo struct {
	*inMemoryIdempotencyRepo
}

func (r *notificationPersistenceRepo) MarkFailedRetryable(ctx context.Context, id int64, reason string, lockedUntil, expiresAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		return errors.New("notification result persistence must be bounded")
	}
	return r.inMemoryIdempotencyRepo.MarkFailedRetryable(ctx, id, reason, lockedUntil, expiresAt)
}

func newBillingTelegramNotificationHarness(t *testing.T, settings *telegramBalanceLaneSettingsStub, transport http.RoundTripper) (*BalanceNotifyService, *notificationPersistenceRepo) {
	t.Helper()
	repo := &notificationPersistenceRepo{newInMemoryIdempotencyRepo()}
	cfg := DefaultIdempotencyConfig()
	cfg.FailedRetryBackoff = 0
	telegram := NewTelegramNotifyService(settings)
	telegram.httpClient.Transport = transport
	s := NewBalanceNotifyService(nil, settings, nil)
	s.SetTelegramNotifyService(telegram)
	s.SetUserAttributeService(NewUserAttributeService(telegramUserAttributeDefinitionStub{}, telegramUserAttributeValueStub{}))
	s.SetIdempotencyService(NewIdempotencyCoordinator(repo, cfg))
	t.Cleanup(s.Stop)
	return s, repo
}

func billingTelegramSettings() *telegramBalanceLaneSettingsStub {
	return &telegramBalanceLaneSettingsStub{values: map[string]string{
		SettingKeyBalanceLowNotifyEnabled:    "true",
		SettingKeyBalanceLowNotifyThreshold:  "5",
		SettingTelegramCustomerBotToken:      "customer-token",
		SettingTelegramNotifyCustomerBalance: "true",
	}}
}

func seedRetryableCustomerNotification(t *testing.T, s *BalanceNotifyService, user *User, eventID string) {
	t.Helper()
	_, err := s.idempotencySvc.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope: "telegram_customer_balance_notification", ActorScope: fmt.Sprintf("user:%d", user.ID),
		Method: "POST", Route: "telegram/customer/balance-low",
		IdempotencyKey:   customerBalanceNotificationIdempotencyKey(user.ID, eventID, 2, 5),
		Payload:          map[string]any{"chat_id": "customer-chat", "event_id": eventID},
		ExecutionTimeout: time.Second, RequireKey: true,
	}, func(context.Context) (any, error) { return nil, errors.New("previous delivery failed") })
	require.Error(t, err)
}

func submitBillingTelegramCrossing(s *BalanceNotifyService, eventID string, user *User) {
	balance := 2.0
	s.submitUsageNotifications(eventID, &postUsageBillingParams{
		Cost: &CostBreakdown{ActualCost: 4}, User: user,
	}, &billingDeps{balanceNotifyService: s}, &UsageBillingApplyResult{NewBalance: &balance})
}

func TestCustomerTelegramWorkerStopCancelsDeliveryAndPersistsRetry(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry=%v", retry), func(t *testing.T) {
			started := make(chan context.Context, 1)
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			s, repo := newBillingTelegramNotificationHarness(t, billingTelegramSettings(), billingTelegramRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				started <- r.Context()
				select {
				case <-r.Context().Done():
					return nil, r.Context().Err()
				case <-release:
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Request: r}, nil
				}
			}))
			t.Cleanup(unblock) // Unblock even when testing the broken cancellation path.
			user := &User{ID: 42, Balance: 2, BalanceNotifyEnabled: true}
			const eventID = "worker-stop"
			if retry {
				seedRetryableCustomerNotification(t, s, user, eventID)
				s.submitCustomerTelegramRetry(user, eventID)
			} else {
				submitBillingTelegramCrossing(s, eventID, user)
			}
			var deliveryCtx context.Context
			select {
			case deliveryCtx = <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("customer delivery did not start")
			}
			stopped := make(chan struct{})
			go func() { s.Stop(); close(stopped) }()
			select {
			case <-stopped:
				require.ErrorIs(t, deliveryCtx.Err(), context.Canceled)
			case <-time.After(time.Second):
				t.Fatal("Stop did not cancel the active Telegram request")
			}
			record, err := repo.GetByScopeAndKeyHash(context.Background(), "telegram_customer_balance_notification", HashIdempotencyKey(customerBalanceNotificationIdempotencyKey(user.ID, eventID, 2, 5)))
			require.NoError(t, err)
			require.NotNil(t, record)
			require.Equal(t, IdempotencyStatusFailedRetryable, record.Status, "cancelled delivery must remain retryable")
		})
	}
}

func TestCustomerTelegramWorkerCannotStarveAccountQuotaNotifications(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry=%v", retry), func(t *testing.T) {
			smtp := startNotificationEmailTestSMTPServer(t)
			settings := billingTelegramSettings()
			for key, value := range smtp.settings() {
				settings.values[key] = value
			}
			settings.values[SettingKeyAccountQuotaNotifyEnabled] = "true"
			settings.values[SettingKeyAccountQuotaNotifyEmails] = `[{"email":"admin@example.test","verified":true}]`
			started := make(chan struct{}, usageNotificationWorkers)
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			s, _ := newBillingTelegramNotificationHarness(t, settings, billingTelegramRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				started <- struct{}{}
				select {
				case <-r.Context().Done():
					return nil, r.Context().Err()
				case <-release:
					return nil, errors.New("released slow Telegram transport")
				}
			}))
			t.Cleanup(unblock)
			s.emailService = NewEmailService(settings, nil)
			user := &User{ID: 42, Balance: 2, BalanceNotifyEnabled: true}
			for i := range usageNotificationWorkers {
				eventID := fmt.Sprintf("slow-customer-%d", i)
				if retry {
					seedRetryableCustomerNotification(t, s, user, eventID)
					s.submitCustomerTelegramRetry(user, eventID)
				} else {
					submitBillingTelegramCrossing(s, eventID, user)
				}
			}
			for range usageNotificationWorkers {
				select {
				case <-started:
				case <-time.After(2 * time.Second):
					t.Fatal("customer transport workers did not fill")
				}
			}
			account := &Account{ID: 9, Name: "quota-account", Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{
				"quota_daily_limit": 100.0, "quota_weekly_limit": 100.0, "quota_limit": 100.0,
				"quota_notify_daily_enabled": true, "quota_notify_daily_threshold": 20.0,
				"quota_notify_weekly_enabled": true, "quota_notify_weekly_threshold": 20.0,
				"quota_notify_total_enabled": true, "quota_notify_total_threshold": 20.0,
			}}
			// This event crosses BOTH wallet and all three account quota thresholds.
			// It must deliver quota email even with every customer worker blocked.
			balance := 2.0
			s.submitUsageNotifications("quota-with-balance-crossing", &postUsageBillingParams{
				Cost: &CostBreakdown{ActualCost: 4, TotalCost: 4}, User: user, Account: account, AccountRateMultiplier: 1,
			}, &billingDeps{balanceNotifyService: s}, &UsageBillingApplyResult{
				NewBalance: &balance,
				QuotaState: &AccountQuotaState{DailyUsed: 82, DailyLimit: 100, WeeklyUsed: 82, WeeklyLimit: 100, TotalUsed: 82, TotalLimit: 100},
			})
			require.Eventually(t, func() bool { return smtp.messageCount() == 3 }, time.Second, 5*time.Millisecond,
				"slow customer Telegram must not consume the account quota worker pool or the same event's deadline")
			for _, dim := range []string{"Daily", "Weekly", "Total"} {
				found := false
				smtp.messageMu.Lock()
				for _, body := range smtp.messageBodies {
					found = found || strings.Contains(body, dim)
				}
				smtp.messageMu.Unlock()
				require.True(t, found, "missing quota dimension %s", dim)
			}
		})
	}
}

func TestCustomerTelegramWorkerQueueIsBoundedAndDiscardsQueuedRetriesOnStop(t *testing.T) {
	started := make(chan struct{}, usageNotificationWorkers)
	s, repo := newBillingTelegramNotificationHarness(t, billingTelegramSettings(), billingTelegramRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	user := &User{ID: 42, Balance: 2, BalanceNotifyEnabled: true}
	for i := range usageNotificationWorkers {
		s.submitCustomerTelegramBalanceNotification(user, 2, 5, fmt.Sprintf("active-%d", i))
	}
	for range usageNotificationWorkers {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("customer workers did not fill")
		}
	}
	for range usageNotificationQueueSize + 1 {
		s.submitCustomerTelegramRetry(user, "queued-retry")
	}
	w := s.customerTelegramNotifications
	require.EqualValues(t, usageNotificationQueueSize, w.pool.WaitingTasks())
	require.Equal(t, uint64(1), w.dropped.Load())
	require.EqualValues(t, usageNotificationWorkers, w.pool.RunningWorkers())
	// The independent upstream lane must still accept work under customer load.
	quotaRan := make(chan struct{})
	require.True(t, s.usageNotifications.submit(func(context.Context) { close(quotaRan) }))
	select {
	case <-quotaRan:
	case <-time.After(time.Second):
		t.Fatal("full customer queue blocked the upstream notification pool")
	}
	s.Stop()
	require.Len(t, repo.data, usageNotificationWorkers, "queued retries must not create notification records")
	require.False(t, w.submit(func(context.Context) { t.Error("stopped customer worker accepted work") }))
	select {
	case <-started:
		t.Fatal("queued delivery started during shutdown")
	default:
	}
}

func TestDuplicateUsageBillingCustomerNotificationEligibility(t *testing.T) {
	for _, mode := range []string{"simple", "subscription", "zero-cost", "disabled-user", "unrelated-event", "already-delivered", "backoff", "balance"} {
		t.Run(mode, func(t *testing.T) {
			delivered := make(chan time.Time, 2)
			s, repo := newBillingTelegramNotificationHarness(t, billingTelegramSettings(), billingTelegramRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				delivered <- time.Now()
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Request: r}, nil
			}))
			user := &User{ID: 42, Balance: 2, BalanceNotifyEnabled: true}
			const eventID = "duplicate-eligibility"
			seedRetryableCustomerNotification(t, s, user, eventID)
			if mode == "already-delivered" || mode == "backoff" {
				record, err := repo.GetByScopeAndKeyHash(context.Background(), "telegram_customer_balance_notification", HashIdempotencyKey(customerBalanceNotificationIdempotencyKey(user.ID, eventID, 2, 5)))
				require.NoError(t, err)
				require.NotNil(t, record)
				if mode == "already-delivered" {
					require.NoError(t, repo.MarkSucceeded(context.Background(), record.ID, http.StatusOK, `{"sent":true}`, time.Now().Add(time.Hour)))
				} else {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					require.NoError(t, repo.MarkFailedRetryable(ctx, record.ID, "previous failure", time.Now().Add(time.Minute), time.Now().Add(time.Hour)))
				}
			}
			params := &postUsageBillingParams{Cost: &CostBreakdown{ActualCost: 4, TotalCost: 4}, User: user, APIKey: &APIKey{ID: 11}, Account: &Account{ID: 12}}
			requestID := eventID
			switch mode {
			case "simple":
				params.SimpleModeKeyRateLimitOnly = true
			case "subscription":
				params.IsSubscriptionBill = true
			case "zero-cost":
				params.Cost.ActualCost = 0
			case "disabled-user":
				user.BalanceNotifyEnabled = false
			case "unrelated-event":
				requestID = "no-original-notification"
			}
			deps := &billingDeps{balanceNotifyService: s, deferredService: NewDeferredService(nil, nil, time.Second)}
			applied, err := applyUsageBilling(context.Background(), requestID, nil, params, deps, telegramUsageBillingDuplicateStub{})
			require.NoError(t, err)
			require.False(t, applied)
			if mode == "balance" {
				select {
				case <-delivered:
				case <-time.After(time.Second):
					t.Fatal("eligible duplicate did not retry customer delivery")
				}
				require.Eventually(t, func() bool {
					record, _ := repo.GetByScopeAndKeyHash(context.Background(), "telegram_customer_balance_notification", HashIdempotencyKey(customerBalanceNotificationIdempotencyKey(user.ID, eventID, 2, 5)))
					return record != nil && record.Status == IdempotencyStatusSucceeded
				}, time.Second, time.Millisecond)
			} else {
				// Wait until every accepted retry finished instead of sleeping: a
				// delayed settings read must not produce a false negative.
				pool := s.customerTelegramNotifications.pool
				require.Eventually(t, func() bool { return pool.CompletedTasks() == pool.SubmittedTasks() }, time.Second, time.Millisecond)
				select {
				case <-delivered:
					t.Fatal("ineligible duplicate triggered customer Telegram delivery")
				default:
				}
			}
		})
	}
}
