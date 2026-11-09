package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type openAIAlertClaimRepo struct {
	IdempotencyRepository
	mu     sync.Mutex
	nextID int64
	items  map[string]*IdempotencyRecord
}

type openAIAlertSettingRepo struct {
	SettingRepository
	values map[string]string
}

func (r *openAIAlertSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	return r.values[key], nil
}

func (r *openAIAlertSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = r.values[key]
	}
	return result, nil
}

func (r *openAIAlertClaimRepo) CreateProcessing(_ context.Context, rec *IdempotencyRecord) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = make(map[string]*IdempotencyRecord)
	}
	key := rec.Scope + ":" + rec.IdempotencyKeyHash
	if _, exists := r.items[key]; exists {
		return false, nil
	}
	r.nextID++
	rec.ID = r.nextID
	copy := *rec
	r.items[key] = &copy
	return true, nil
}
func (r *openAIAlertClaimRepo) GetByScopeAndKeyHash(_ context.Context, scope, hash string) (*IdempotencyRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec := r.items[scope+":"+hash]; rec != nil {
		copy := *rec
		return &copy, nil
	}
	return nil, nil
}
func (r *openAIAlertClaimRepo) TryReclaim(_ context.Context, id int64, fromStatus string, now, lockedUntil, expiresAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.items {
		if rec.ID == id && rec.Status == fromStatus && (rec.LockedUntil == nil || !rec.LockedUntil.After(now)) {
			rec.Status, rec.LockedUntil, rec.ExpiresAt = IdempotencyStatusProcessing, &lockedUntil, expiresAt
			return true, nil
		}
	}
	return false, nil
}
func (r *openAIAlertClaimRepo) MarkSucceeded(_ context.Context, id int64, code int, body string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.items {
		if rec.ID == id {
			rec.Status, rec.ResponseStatus, rec.ResponseBody, rec.ExpiresAt = IdempotencyStatusSucceeded, &code, &body, expiresAt
			return nil
		}
	}
	return errors.New("claim not found")
}
func (r *openAIAlertClaimRepo) MarkFailedRetryable(_ context.Context, id int64, reason string, retryAt, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.items {
		if rec.ID == id {
			rec.Status, rec.ErrorReason, rec.LockedUntil, rec.ExpiresAt = IdempotencyStatusFailedRetryable, &reason, &retryAt, expiresAt
			return nil
		}
	}
	return errors.New("claim not found")
}

func TestOpenAIProviderBalanceAlertDeliveryAndDedup(t *testing.T) {
	var mu sync.Mutex
	var messages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("form: %v", err)
		}
		mu.Lock()
		messages = append(messages, r.Form.Get("text"))
		if r.URL.Path != "/botadmin-token/sendMessage" || r.Form.Get("chat_id") != "admin-chat" {
			t.Errorf("wrong admin destination: %s / %s", r.URL.Path, r.Form.Get("chat_id"))
		}
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	settings := &openAIAlertSettingRepo{values: map[string]string{
		SettingTelegramAdminBotToken: "admin-token", SettingTelegramAdminChatID: "admin-chat",
		SettingTelegramCustomerBotToken: "customer-token", SettingTelegramNotifyProviderBalance: "true",
	}}
	telegram := NewTelegramNotifyService(settings)
	telegram.apiBaseURL = srv.URL + "/bot"
	repo := &openAIAlertClaimRepo{}
	claims := NewIdempotencyCoordinator(repo, IdempotencyConfig{DefaultTTL: time.Hour, ProcessingTimeout: time.Minute, FailedRetryBackoff: time.Millisecond, MaxStoredResponseLen: 1024})
	alerts := NewOpenAIProviderBalanceAlerts(telegram, claims)
	account := &Account{ID: 577, Name: "provider-577", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	ctx := context.Background()
	depleted := []byte(`{"error":{"message":"Insufficient account balance"}}`)
	alerts.Observe(ctx, account, http.StatusForbidden, depleted)
	alerts.Observe(ctx, account, http.StatusForbidden, depleted)
	mu.Lock()
	count := len(messages)
	var text string
	if count > 0 {
		text = messages[0]
	}
	mu.Unlock()
	if count != 1 || !strings.Contains(text, "577") || !strings.Contains(text, "nạp") {
		t.Fatalf("deliveries=%d text=%q; want one actionable admin refill alert", count, text)
	}
	for _, tc := range []struct {
		account *Account
		code    int
		body    string
	}{
		{account, http.StatusForbidden, `<html>insufficient account balance</html>`},
		{account, http.StatusForbidden, `{"error":{"message":"Forbidden"}}`},
		{account, http.StatusBadGateway, string(depleted)},
		{account, http.StatusForbidden, `{"error":{"message":"Customer has insufficient balance"}}`},
		{&Account{ID: 582, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, http.StatusForbidden, `{"error":{"message":"Customer insufficient account balance"}}`},
		{&Account{ID: 578, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, http.StatusForbidden, string(depleted)},
		{&Account{ID: 579, Platform: PlatformDeepseek, Type: AccountTypeAPIKey}, http.StatusForbidden, string(depleted)},
	} {
		alerts.Observe(ctx, tc.account, tc.code, []byte(tc.body))
	}
	mu.Lock()
	count = len(messages)
	mu.Unlock()
	if count != 1 {
		t.Fatalf("unrelated errors caused %d admin sends", count)
	}
	settings.values[SettingTelegramNotifyProviderBalance] = "false"
	alerts.Observe(ctx, &Account{ID: 580, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, http.StatusForbidden, depleted)
	mu.Lock()
	count = len(messages)
	mu.Unlock()
	if count != 1 {
		t.Fatalf("disabled feature caused %d admin sends", count)
	}
}

func TestOpenAIProviderBalanceAlertFailedSendRetryable(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	settings := &openAIAlertSettingRepo{values: map[string]string{SettingTelegramAdminBotToken: "admin-token", SettingTelegramAdminChatID: "admin-chat", SettingTelegramNotifyProviderBalance: "true"}}
	telegram := NewTelegramNotifyService(settings)
	telegram.apiBaseURL = srv.URL + "/bot"
	repo := &openAIAlertClaimRepo{}
	alerts := NewOpenAIProviderBalanceAlerts(telegram, NewIdempotencyCoordinator(repo, IdempotencyConfig{DefaultTTL: time.Hour, ProcessingTimeout: time.Minute, FailedRetryBackoff: time.Millisecond, MaxStoredResponseLen: 1024}))
	account := &Account{ID: 577, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(`{"error":{"code":"INSUFFICIENT_BALANCE","message":"payment required"}}`)
	alerts.Observe(context.Background(), account, http.StatusForbidden, body)
	repo.mu.Lock()
	var status string
	for _, item := range repo.items {
		status = item.Status
	}
	repo.mu.Unlock()
	if status != IdempotencyStatusFailedRetryable {
		t.Fatalf("first failure persisted %q, want retryable", status)
	}
	time.Sleep(2 * time.Millisecond)
	alerts.Observe(context.Background(), account, http.StatusForbidden, body)
	repo.mu.Lock()
	for _, item := range repo.items {
		status = item.Status
	}
	repo.mu.Unlock()
	mu.Lock()
	n := attempts
	mu.Unlock()
	if status != IdempotencyStatusSucceeded || n != 2 {
		t.Fatalf("status=%q attempts=%d; want succeeded after retry", status, n)
	}
}

func TestOpenAIProviderBalanceAlertFromUpstreamError(t *testing.T) {
	var mu sync.Mutex
	sends := 0
	sent := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		sends++
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
		sent <- struct{}{}
	}))
	defer srv.Close()
	settings := &openAIAlertSettingRepo{values: map[string]string{
		SettingTelegramAdminBotToken: "admin-token", SettingTelegramAdminChatID: "admin-chat", SettingTelegramNotifyProviderBalance: "true",
	}}
	telegram := NewTelegramNotifyService(settings)
	telegram.apiBaseURL = srv.URL + "/bot"
	alerts := NewOpenAIProviderBalanceAlerts(telegram, NewIdempotencyCoordinator(&openAIAlertClaimRepo{}, IdempotencyConfig{
		DefaultTTL: time.Hour, ProcessingTimeout: time.Minute, FailedRetryBackoff: time.Millisecond, MaxStoredResponseLen: 1024,
	}))
	rate := &RateLimitService{}
	rate.SetOpenAIProviderBalanceAlerts(alerts)
	account := &Account{ID: 577, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // A canceled client must not cancel the admin delivery.
	if rate.HandleUpstreamError(ctx, account, http.StatusForbidden, nil, []byte(`{"error":{"message":"Insufficient account balance"}}`)) {
		t.Fatal("alert must not change existing pool-mode error policy")
	}
	select {
	case <-sent:
	case <-time.After(3 * time.Second):
		t.Fatal("no admin send after gateway error")
	}
	mu.Lock()
	count := sends
	mu.Unlock()
	if count != 1 {
		t.Fatalf("upstream 403 caused %d admin sends, want 1", count)
	}
}

func TestOpenAIProviderBalanceAlertTransportErrorDoesNotPersistToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := srv.URL
	srv.Close() // Force a transport error whose URL would ordinarily contain the bot token.
	settings := &openAIAlertSettingRepo{values: map[string]string{
		SettingTelegramAdminBotToken: "sensitive-bot-token", SettingTelegramAdminChatID: "admin-chat",
		SettingTelegramNotifyProviderBalance: "true",
	}}
	telegram := NewTelegramNotifyService(settings)
	telegram.apiBaseURL = baseURL + "/bot"
	repo := &openAIAlertClaimRepo{}
	alerts := NewOpenAIProviderBalanceAlerts(telegram, NewIdempotencyCoordinator(repo, IdempotencyConfig{
		DefaultTTL: time.Hour, ProcessingTimeout: time.Minute, FailedRetryBackoff: time.Millisecond, MaxStoredResponseLen: 1024,
	}))
	alerts.Observe(context.Background(), &Account{ID: 577, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		http.StatusForbidden, []byte(`{"error":{"code":"INSUFFICIENT_BALANCE"}}`))
	repo.mu.Lock()
	defer repo.mu.Unlock()
	for _, rec := range repo.items {
		if rec.Status != IdempotencyStatusFailedRetryable || rec.ErrorReason == nil ||
			strings.Contains(*rec.ErrorReason, "sensitive-bot-token") {
			t.Fatalf("claim failure status=%q leaked token=%t", rec.Status,
				rec.ErrorReason != nil && strings.Contains(*rec.ErrorReason, "sensitive-bot-token"))
		}
		return
	}
	t.Fatal("transport failure was not persisted as retryable")
}
