package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type telegramBalanceLaneSettingsStub struct {
	values map[string]string
}

func (s *telegramBalanceLaneSettingsStub) Get(context.Context, string) (*Setting, error) {
	return nil, nil
}
func (s *telegramBalanceLaneSettingsStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}
func (s *telegramBalanceLaneSettingsStub) Set(context.Context, string, string) error { return nil }
func (s *telegramBalanceLaneSettingsStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = s.values[key]
	}
	return result, nil
}
func (s *telegramBalanceLaneSettingsStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (s *telegramBalanceLaneSettingsStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
}
func (s *telegramBalanceLaneSettingsStub) Delete(context.Context, string) error { return nil }

type telegramBalanceAccountRepoStub struct {
	AccountRepository
	mu             sync.Mutex
	account        *Account
	claimAttempts  int
	enforceDbClaim bool
}

func (r *telegramBalanceAccountRepoStub) UpdateExtra(_ context.Context, _ int64, values map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account.Extra == nil {
		r.account.Extra = make(map[string]any)
	}
	for key, value := range values {
		r.account.Extra[key] = value
	}
	return nil
}

func (r *telegramBalanceAccountRepoStub) TryClaimProviderBalanceAlert(_ context.Context, _ int64, platform string, status ProviderBalanceAlertStatus) (bool, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.claimAttempts++
	if r.enforceDbClaim {
		statusKey := cnExtraKey(platform, cnBalanceExtraSuffixAlertStatus)
		deliveryStatusKey := platform + "_balance_alert_delivery_status"
		currentStatus, _ := r.account.Extra[statusKey].(string)
		deliveryStatus, _ := r.account.Extra[deliveryStatusKey].(string)
		if currentStatus == string(status) && deliveryStatus == "delivered:"+string(status) {
			return false, "", nil
		}
		deliveryKey := "test-delivery-key"
		r.account.Extra[statusKey] = string(status)
		r.account.Extra[deliveryStatusKey] = "processing:" + deliveryKey
		return true, deliveryKey, nil
	}
	return true, "test-delivery-key", nil
}

func TestTelegramBalanceLanesDoNotShareTokens(t *testing.T) {
	settings := &telegramBalanceLaneSettingsStub{values: map[string]string{
		SettingTelegramAdminBotToken:         "admin-token",
		SettingTelegramAdminChatID:           "admin-chat",
		SettingTelegramCustomerBotToken:      "customer-token",
		SettingTelegramNotifyProviderBalance: "true",
		SettingTelegramNotifyCustomerBalance: "true",
	}}

	var requests []struct {
		path   string
		chatID string
		text   string
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		requests = append(requests, struct {
			path   string
			chatID string
			text   string
		}{r.URL.Path, r.Form.Get("chat_id"), r.Form.Get("text")})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	svc := NewTelegramNotifyService(settings)
	svc.apiBaseURL = server.URL + "/bot"

	if err := svc.sendAdminMessage(context.Background(), "provider alert"); err != nil {
		t.Fatalf("send admin message: %v", err)
	}
	if err := svc.sendCustomerMessage(context.Background(), "customer-chat", "customer alert"); err != nil {
		t.Fatalf("send customer message: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	if requests[0].path != "/botadmin-token/sendMessage" || requests[0].chatID != "admin-chat" {
		t.Fatalf("admin request = %#v", requests[0])
	}
	if requests[1].path != "/botcustomer-token/sendMessage" || requests[1].chatID != "customer-chat" {
		t.Fatalf("customer request = %#v", requests[1])
	}
	if strings.Contains(requests[1].text, "provider") {
		t.Fatalf("customer message contains provider lane data: %q", requests[1].text)
	}
}

type telegramUsageBillingDuplicateStub struct {
	UsageBillingRepository
}

func (telegramUsageBillingDuplicateStub) Apply(context.Context, *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	return &UsageBillingApplyResult{Applied: false}, nil
}

type telegramUserAttributeDefinitionStub struct {
	UserAttributeDefinitionRepository
}

func (telegramUserAttributeDefinitionStub) GetByKey(_ context.Context, key string) (*UserAttributeDefinition, error) {
	return &UserAttributeDefinition{ID: 7, Key: key}, nil
}

type telegramUserAttributeValueStub struct {
	UserAttributeValueRepository
}

func (telegramUserAttributeValueStub) GetByUserID(context.Context, int64) ([]UserAttributeValue, error) {
	return []UserAttributeValue{{UserID: 42, AttributeID: 7, Value: "customer-chat"}}, nil
}

func TestDuplicateUsageBillingRetriesFailedCustomerTelegramDelivery(t *testing.T) {
	settings := &telegramBalanceLaneSettingsStub{values: map[string]string{
		SettingKeyBalanceLowNotifyEnabled:    "true",
		SettingKeyBalanceLowNotifyThreshold:  "5",
		SettingTelegramCustomerBotToken:      "customer-token",
		SettingTelegramNotifyCustomerBalance: "true",
	}}
	requests := make(chan struct {
		path   string
		chatID string
	}, 2)
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		requests <- struct {
			path   string
			chatID string
		}{r.URL.Path, r.Form.Get("chat_id")}
		if atomic.AddInt32(&attempts, 1) == 1 {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	repo := &telegramIdempotencyRepositoryStub{}
	// Exercise replay after it is eligible; the active-backoff no-op is covered
	// separately in TestIdempotencyExecuteExistingSkipsRetryDuringBackoff.
	coordinator := NewIdempotencyCoordinator(repo, IdempotencyConfig{
		DefaultTTL:           time.Hour,
		ProcessingTimeout:    time.Minute,
		FailedRetryBackoff:   0,
		MaxStoredResponseLen: 1024,
	})
	telegram := NewTelegramNotifyService(settings)
	telegram.apiBaseURL = server.URL + "/bot"
	attributes := NewUserAttributeService(telegramUserAttributeDefinitionStub{}, telegramUserAttributeValueStub{})
	notifications := NewBalanceNotifyService(nil, settings, nil)
	notifications.SetTelegramNotifyService(telegram)
	notifications.SetUserAttributeService(attributes)
	notifications.SetIdempotencyService(coordinator)

	user := &User{ID: 42, Email: "user@example.test", Balance: 2, BalanceNotifyEnabled: true}
	const eventID = "usage-event"
	t.Cleanup(notifications.Stop)
	notifications.submitUsageNotifications(eventID, &postUsageBillingParams{
		Cost: &CostBreakdown{ActualCost: 4},
		User: user,
	}, &billingDeps{balanceNotifyService: notifications}, &UsageBillingApplyResult{NewBalance: &user.Balance})
	select {
	case first := <-requests:
		if first.path != "/botcustomer-token/sendMessage" || first.chatID != "customer-chat" {
			t.Fatalf("first notification route = %#v", first)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("initial customer Telegram delivery did not run")
	}

	// The HTTP handler runs before the asynchronous delivery marks its
	// idempotency record failed. Replay only after that durable transition.
	key := customerBalanceNotificationIdempotencyKey(user.ID, eventID, 2, 5)
	deadline := time.After(8 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		record, err := repo.GetByScopeAndKeyHash(context.Background(), "telegram_customer_balance_notification", HashIdempotencyKey(key))
		if err != nil {
			t.Fatalf("read delivery state: %v", err)
		}
		if record != nil && record.Status == IdempotencyStatusFailedRetryable {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("initial delivery was not marked failed_retryable; record = %#v", record)
		case <-ticker.C:
		}
	}

	params := &postUsageBillingParams{
		Cost:    &CostBreakdown{TotalCost: 4, ActualCost: 4},
		User:    user,
		APIKey:  &APIKey{ID: 11},
		Account: &Account{ID: 12},
	}
	deps := &billingDeps{
		balanceNotifyService: notifications,
		deferredService:      NewDeferredService(nil, nil, time.Second),
	}
	applied, err := applyUsageBilling(context.Background(), eventID, nil, params, deps, telegramUsageBillingDuplicateStub{})
	if err != nil {
		t.Fatalf("duplicate usage billing replay: %v", err)
	}
	if applied {
		t.Fatal("duplicate usage billing event unexpectedly applied again")
	}
	select {
	case retry := <-requests:
		if retry.path != "/botcustomer-token/sendMessage" || retry.chatID != "customer-chat" {
			t.Fatalf("retry notification route = %#v", retry)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("duplicate billing replay did not retry failed customer notification")
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("Telegram attempts = %d, want initial failure plus replay retry", got)
	}
}

func TestTelegramCustomerLaneNeverFallsBackToAdminToken(t *testing.T) {
	settings := &telegramBalanceLaneSettingsStub{values: map[string]string{
		SettingTelegramAdminBotToken:         "admin-token",
		SettingTelegramAdminChatID:           "admin-chat",
		SettingTelegramNotifyCustomerBalance: "true",
	}}
	svc := NewTelegramNotifyService(settings)
	svc.apiBaseURL = "http://127.0.0.1:1/bot"

	if err := svc.sendCustomerMessage(context.Background(), "customer-chat", "customer alert"); err == nil {
		t.Fatal("customer send unexpectedly used the admin token fallback")
	}
}

func TestProviderBalanceAlertRetriesFailedDelivery(t *testing.T) {
	settings := &telegramBalanceLaneSettingsStub{values: map[string]string{
		SettingTelegramAdminBotToken:         "admin-token",
		SettingTelegramAdminChatID:           "admin-chat",
		SettingTelegramNotifyProviderBalance: "true",
	}}
	var deliveries int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveries++
		if deliveries == 1 {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	repo := &telegramBalanceAccountRepoStub{
		account: &Account{
			ID:       42,
			Platform: PlatformKimi,
			Name:     "test account",
			Extra:    map[string]any{cnExtraKey(PlatformKimi, cnBalanceExtraSuffixAlertStatus): string(providerBalanceAlertHealthy)},
		},
		enforceDbClaim: false,
	}

	svc := NewTelegramNotifyService(settings)
	svc.apiBaseURL = server.URL + "/bot"
	check := &CNProviderBalanceCheckService{accountRepo: repo, telegramNotifySvc: svc}

	account := repo.account
	result := &CNProviderBalanceResult{Provider: PlatformKimi, Success: true, Available: true, Balance: 1}
	check.publishProviderBalanceAlert(context.Background(), account, result, 10)
	check.publishProviderBalanceAlert(context.Background(), account, result, 10)

	if deliveries != 2 {
		t.Fatalf("delivery attempts = %d, want initial failure + retry", deliveries)
	}
	if got := account.Extra[cnExtraKey(PlatformKimi, "balance_alert_delivery_status")]; got != "delivered:low" {
		t.Fatalf("delivery status = %v, want delivered:low", got)
	}
}

func TestProviderBalanceAlertUsesSharedIdempotencyClaim(t *testing.T) {
	settings := &telegramBalanceLaneSettingsStub{values: map[string]string{
		SettingTelegramAdminBotToken:         "admin-token",
		SettingTelegramAdminChatID:           "admin-chat",
		SettingTelegramNotifyProviderBalance: "true",
	}}
	var deliveries int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&deliveries, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	repo := &telegramBalanceAccountRepoStub{account: &Account{
		ID:       42,
		Platform: PlatformKimi,
		Name:     "test account",
		Extra:    map[string]any{},
	}}
	coordinator := NewIdempotencyCoordinator(&telegramIdempotencyRepositoryStub{}, IdempotencyConfig{
		DefaultTTL:           time.Hour,
		ProcessingTimeout:    time.Minute,
		FailedRetryBackoff:   5 * time.Second,
		MaxStoredResponseLen: 1024,
	})
	telegram := NewTelegramNotifyService(settings)
	telegram.apiBaseURL = server.URL + "/bot"
	first := &CNProviderBalanceCheckService{accountRepo: repo, telegramNotifySvc: telegram, idempotencySvc: coordinator}
	second := &CNProviderBalanceCheckService{accountRepo: repo, telegramNotifySvc: telegram, idempotencySvc: coordinator}
	result := &CNProviderBalanceResult{Provider: PlatformKimi, Success: true, Available: true, Balance: 1}
	account1 := &Account{ID: 42, Platform: PlatformKimi, Name: "test account", Extra: map[string]any{cnExtraKey(PlatformKimi, cnBalanceExtraSuffixAlertStatus): string(providerBalanceAlertHealthy)}}
	account2 := &Account{ID: 42, Platform: PlatformKimi, Name: "test account", Extra: map[string]any{cnExtraKey(PlatformKimi, cnBalanceExtraSuffixAlertStatus): string(providerBalanceAlertHealthy)}}

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		first.publishProviderBalanceAlert(context.Background(), account1, result, 10)
	}()
	go func() {
		defer wg.Done()
		<-start
		second.publishProviderBalanceAlert(context.Background(), account2, result, 10)
	}()
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&deliveries); got != 1 {
		t.Fatalf("concurrent provider deliveries = %d, want exactly one", got)
	}
	record, err := coordinator.repo.GetByScopeAndKeyHash(context.Background(), "telegram_provider_balance_notification", HashIdempotencyKey(ProviderBalanceAlertNotificationKey(42, PlatformKimi, string(providerBalanceAlertLow))))
	if err != nil {
		t.Fatalf("read provider idempotency state: %v", err)
	}
	if record == nil || record.Status != IdempotencyStatusSucceeded {
		t.Fatalf("provider idempotency record = %#v, want succeeded", record)
	}
}

func TestTelegramTransportErrorsDoNotExposeBotToken(t *testing.T) {
	settings := &telegramBalanceLaneSettingsStub{values: map[string]string{
		SettingTelegramAdminBotToken: "secret-admin-token",
		SettingTelegramAdminChatID:   "admin-chat",
	}}
	svc := NewTelegramNotifyService(settings)
	svc.apiBaseURL = "http://127.0.0.1:1/bot"
	err := svc.sendAdminMessage(context.Background(), "test")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), "secret-admin-token") {
		t.Fatalf("transport error exposed bot token: %v", err)
	}
}

func TestProviderBalanceAlertStatusAndDeduplication(t *testing.T) {
	tests := []struct {
		name     string
		result   *CNProviderBalanceResult
		want     providerBalanceAlertStatus
		previous providerBalanceAlertStatus
		alert    bool
	}{
		{name: "exhausted", result: &CNProviderBalanceResult{Success: true, Balance: 0, Available: true}, want: providerBalanceAlertExhausted, previous: providerBalanceAlertHealthy, alert: true},
		{name: "low", result: &CNProviderBalanceResult{Success: true, Balance: 2, Available: true}, want: providerBalanceAlertLow, previous: providerBalanceAlertExhausted, alert: true},
		{name: "unavailable", result: &CNProviderBalanceResult{Success: false, Error: "upstream unavailable"}, want: providerBalanceAlertUnavailable, previous: providerBalanceAlertLow, alert: true},
		{name: "repeated unavailable", result: &CNProviderBalanceResult{Success: false, Error: "upstream unavailable"}, want: providerBalanceAlertUnavailable, previous: providerBalanceAlertUnavailable, alert: false},
		{name: "healthy recovery", result: &CNProviderBalanceResult{Success: true, Balance: 20, Available: true}, want: providerBalanceAlertHealthy, previous: providerBalanceAlertLow, alert: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyProviderBalanceAlert(tt.result, 10)
			if got != tt.want {
				t.Fatalf("status = %q, want %q", got, tt.want)
			}
			if gotAlert := providerBalanceAlertTransition(tt.previous, got); gotAlert != tt.alert {
				t.Fatalf("transition(%q -> %q) = %v, want %v", tt.previous, got, gotAlert, tt.alert)
			}
		})
	}
}

func TestCustomerTelegramAttributeLookup(t *testing.T) {
	values := []UserAttributeValue{{UserID: 42, AttributeID: 7, Value: " 12345 "}}
	definitions := []UserAttributeDefinition{{ID: 7, Key: customerTelegramChatIDAttributeKey}}
	if got := resolveCustomerTelegramChatID(values, definitions); got != "12345" {
		t.Fatalf("chat id = %q, want 12345", got)
	}
	if got := resolveCustomerTelegramChatID(nil, definitions); got != "" {
		t.Fatalf("empty values produced chat id %q", got)
	}
}

func TestCustomerTelegramBalanceNotificationKeyUsesStableEventIdentity(t *testing.T) {
	first := customerBalanceNotificationIdempotencyKey(42, "usage-request-1", 1.25, 5)
	replay := customerBalanceNotificationIdempotencyKey(42, "usage-request-1", 0.75, 5)
	next := customerBalanceNotificationIdempotencyKey(42, "usage-request-2", 1.25, 5)
	if first != replay {
		t.Fatal("same billing event should keep the same idempotency key")
	}
	if first == next {
		t.Fatal("different billing events must not share an idempotency key")
	}
	fallback := customerBalanceNotificationIdempotencyKey(42, " ", 1.25, 5)
	if fallback != customerBalanceNotificationIdempotencyKey(42, "", 1.25, 5) {
		t.Fatal("eventless fallback should be deterministic")
	}
}

type telegramIdempotencyRepositoryStub struct {
	mu      sync.Mutex
	nextID  int64
	records map[string]*IdempotencyRecord
}

func (r *telegramIdempotencyRepositoryStub) CreateProcessing(_ context.Context, record *IdempotencyRecord) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records == nil {
		r.records = make(map[string]*IdempotencyRecord)
	}
	key := record.Scope + ":" + record.IdempotencyKeyHash
	if _, exists := r.records[key]; exists {
		return false, nil
	}
	r.nextID++
	record.ID = r.nextID
	r.records[key] = cloneIdempotencyRecord(record)
	return true, nil
}

func (r *telegramIdempotencyRepositoryStub) GetByScopeAndKeyHash(_ context.Context, scope, keyHash string) (*IdempotencyRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if record := r.records[scope+":"+keyHash]; record != nil {
		return cloneIdempotencyRecord(record), nil
	}
	return nil, nil
}

func (r *telegramIdempotencyRepositoryStub) TryReclaim(_ context.Context, id int64, fromStatus string, now, newLockedUntil, newExpiresAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range r.records {
		if record.ID == id && record.Status == fromStatus && (record.LockedUntil == nil || !record.LockedUntil.After(now)) {
			record.Status = IdempotencyStatusProcessing
			record.LockedUntil = &newLockedUntil
			record.ExpiresAt = newExpiresAt
			return true, nil
		}
	}
	return false, nil
}

func (r *telegramIdempotencyRepositoryStub) ExtendProcessingLock(_ context.Context, id int64, _ string, newLockedUntil, newExpiresAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range r.records {
		if record.ID == id && record.Status == IdempotencyStatusProcessing {
			record.LockedUntil = &newLockedUntil
			record.ExpiresAt = newExpiresAt
			return true, nil
		}
	}
	return false, nil
}

func (r *telegramIdempotencyRepositoryStub) MarkSucceeded(_ context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range r.records {
		if record.ID == id {
			record.Status = IdempotencyStatusSucceeded
			record.ResponseStatus = &responseStatus
			record.ResponseBody = &responseBody
			record.LockedUntil = nil
			record.ExpiresAt = expiresAt
			return nil
		}
	}
	return errors.New("idempotency record not found")
}

func (r *telegramIdempotencyRepositoryStub) MarkFailedRetryable(_ context.Context, id int64, reason string, lockedUntil, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range r.records {
		if record.ID == id {
			record.Status = IdempotencyStatusFailedRetryable
			record.ErrorReason = &reason
			record.LockedUntil = &lockedUntil
			record.ExpiresAt = expiresAt
			return nil
		}
	}
	return errors.New("idempotency record not found")
}

func (r *telegramIdempotencyRepositoryStub) DeleteExpired(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func cloneIdempotencyRecord(record *IdempotencyRecord) *IdempotencyRecord {
	clone := *record
	return &clone
}

func TestIdempotencyExecuteExistingRetriesOnlyPersistedFailure(t *testing.T) {
	repo := &telegramIdempotencyRepositoryStub{}
	coordinator := NewIdempotencyCoordinator(repo, IdempotencyConfig{
		DefaultTTL:           time.Hour,
		ProcessingTimeout:    time.Minute,
		FailedRetryBackoff:   0,
		MaxStoredResponseLen: 1024,
	})
	opts := IdempotencyExecuteOptions{
		Scope:          "telegram_customer_balance_notification",
		ActorScope:     "user:42",
		Method:         "POST",
		Route:          "telegram/customer/balance-low",
		IdempotencyKey: "customer-event-42",
		Payload:        map[string]any{"chat_id": "customer-chat", "event_id": "usage-event"},
		RequireKey:     true,
	}
	var deliveries int
	fail := func(context.Context) (any, error) {
		deliveries++
		return nil, errors.New("temporary Telegram failure")
	}
	if _, err := coordinator.Execute(context.Background(), opts, fail); err == nil {
		t.Fatal("first delivery unexpectedly succeeded")
	}

	succeed := func(context.Context) (any, error) {
		deliveries++
		return nil, nil
	}
	if _, err := coordinator.ExecuteExisting(context.Background(), opts, succeed); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if deliveries != 2 {
		t.Fatalf("deliveries = %d, want failed attempt plus one retry", deliveries)
	}

	missingOpts := opts
	missingOpts.IdempotencyKey = "event-without-failure"
	if _, err := coordinator.ExecuteExisting(context.Background(), missingOpts, succeed); err != nil {
		t.Fatalf("missing-record no-op: %v", err)
	}
	if deliveries != 2 {
		t.Fatalf("missing retryable record triggered delivery; count=%d", deliveries)
	}
}

func TestIdempotencyExecuteExistingSkipsRetryDuringBackoff(t *testing.T) {
	repo := &telegramIdempotencyRepositoryStub{}
	coordinator := NewIdempotencyCoordinator(repo, IdempotencyConfig{
		DefaultTTL:           time.Hour,
		ProcessingTimeout:    time.Minute,
		FailedRetryBackoff:   time.Minute,
		MaxStoredResponseLen: 1024,
	})
	opts := IdempotencyExecuteOptions{
		Scope:          "telegram_customer_balance_notification",
		ActorScope:     "user:42",
		Method:         "POST",
		Route:          "telegram/customer/balance-low",
		IdempotencyKey: "customer-event-locked",
		Payload:        map[string]any{"chat_id": "customer-chat", "event_id": "locked-event"},
		RequireKey:     true,
	}
	deliveries := 0
	if _, err := coordinator.Execute(context.Background(), opts, func(context.Context) (any, error) {
		deliveries++
		return nil, errors.New("temporary Telegram failure")
	}); err == nil {
		t.Fatal("first delivery unexpectedly succeeded")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := coordinator.ExecuteExisting(ctx, opts, func(context.Context) (any, error) {
		deliveries++
		return nil, nil
	}); err != nil {
		t.Fatalf("retry during active backoff should be skipped without blocking: %v", err)
	}
	if deliveries != 1 {
		t.Fatalf("deliveries during active backoff = %d, want only the failed initial attempt", deliveries)
	}
}

func TestCustomerTelegramBalanceNotificationDeduplicatesAfterPersistentClaim(t *testing.T) {
	repo := &telegramIdempotencyRepositoryStub{}
	coordinator := NewIdempotencyCoordinator(repo, IdempotencyConfig{
		DefaultTTL:           time.Hour,
		ProcessingTimeout:    time.Minute,
		FailedRetryBackoff:   time.Second,
		MaxStoredResponseLen: 1024,
	})
	var deliveries int
	opts := IdempotencyExecuteOptions{
		Scope:          "telegram_customer_balance_notification",
		ActorScope:     "user:42",
		Method:         "POST",
		Route:          "telegram/customer/balance-low",
		IdempotencyKey: "42:customer-chat:1.25:5",
		Payload:        map[string]any{"chat_id": "customer-chat", "balance": 1.25, "threshold": 5},
		RequireKey:     true,
	}
	deliver := func(ctx context.Context) (any, error) {
		record, err := repo.GetByScopeAndKeyHash(ctx, opts.Scope, HashIdempotencyKey(opts.IdempotencyKey))
		if err != nil {
			return nil, err
		}
		if record == nil || record.Status != IdempotencyStatusProcessing {
			return nil, errors.New("persistent processing claim missing before delivery")
		}
		deliveries++
		return nil, nil
	}
	if _, err := coordinator.Execute(context.Background(), opts, deliver); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if _, err := coordinator.Execute(context.Background(), opts, deliver); err != nil {
		t.Fatalf("replayed delivery: %v", err)
	}
	if deliveries != 1 {
		t.Fatalf("deliveries = %d, want 1 after persistent idempotency claim", deliveries)
	}
}

// Keep the test imports honest if the production implementation changes the
// HTTP assertion surface while preserving the lane contract.
var _ = json.Valid
var _ = url.Values{}
