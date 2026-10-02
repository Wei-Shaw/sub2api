//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type telegramIdentityRepoStub struct {
	userRepoStub
	identities map[int64][]UserAuthIdentityRecord
}

func (s *telegramIdentityRepoStub) ListUserAuthIdentities(_ context.Context, userID int64) ([]UserAuthIdentityRecord, error) {
	return s.identities[userID], nil
}

type telegramSent struct {
	path   string
	chatID string
	text   string
}

func newTelegramTestService(t *testing.T, enabled bool) (*NotificationEmailService, chan telegramSent) {
	t.Helper()
	sent := make(chan telegramSent, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChatID string `json:"chat_id"`
			Text   string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sent <- telegramSent{path: r.URL.Path, chatID: body.ChatID, text: body.Text}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	settings := newNotificationEmailMemorySettingRepo()
	if enabled {
		settings.values[SettingKeyTelegramOAuthEnabled] = "true"
	}
	settings.values[SettingKeyTelegramOAuthBotToken] = "123:ABC"
	svc := NewNotificationEmailService(settings, nil)
	svc.EnableTelegram(&telegramIdentityRepoStub{identities: map[int64][]UserAuthIdentityRecord{
		7: {{ProviderType: "email", ProviderSubject: "a@example.com"}, {ProviderType: "telegram", ProviderSubject: "777"}},
	}})
	svc.telegram.apiBase = server.URL
	return svc, sent
}

func rechargeInput(userID int64) NotificationEmailSendInput {
	return NotificationEmailSendInput{
		Event:      NotificationEmailEventBalanceRechargeSuccess,
		UserID:     userID,
		SourceType: "payment_order",
		SourceID:   "42",
		Variables:  map[string]string{"recharge_amount": "12.50", "current_balance": "20.00", "order_id": "42"},
	}
}

func TestTelegramNotify_SendsToLinkedUserOnce(t *testing.T) {
	svc, sent := newTelegramTestService(t, true)
	ctx := context.Background()
	tmpl := telegramMessageTemplates[NotificationEmailEventBalanceRechargeSuccess]

	require.NoError(t, svc.sendTelegram(ctx, rechargeInput(7), tmpl))
	msg := <-sent
	require.Equal(t, "/bot123:ABC/sendMessage", msg.path)
	require.Equal(t, "777", msg.chatID)
	require.Contains(t, msg.text, "Nạp tiền thành công")
	require.Contains(t, msg.text, "$12.50")
	require.NotContains(t, msg.text, "{{")

	// 同一订单重复触发（多个邮件收件人 / 任务重跑）不再发送。
	require.NoError(t, svc.sendTelegram(ctx, rechargeInput(7), tmpl))
	require.Empty(t, sent)
}

func TestTelegramNotify_SkipsUnlinkedUserAndDisabledLogin(t *testing.T) {
	tmpl := telegramMessageTemplates[NotificationEmailEventBalanceRechargeSuccess]

	svc, sent := newTelegramTestService(t, true)
	require.NoError(t, svc.sendTelegram(context.Background(), rechargeInput(8), tmpl))
	require.Empty(t, sent)

	disabled, sentDisabled := newTelegramTestService(t, false)
	require.NoError(t, disabled.sendTelegram(context.Background(), rechargeInput(7), tmpl))
	require.Empty(t, sentDisabled)
}

func TestTelegramNotify_LoginIncludesClientAndIsNotDeduplicated(t *testing.T) {
	svc, sent := newTelegramTestService(t, true)
	auth := &AuthService{emailService: &EmailService{notificationEmailService: svc}}
	ctx := WithSessionBinding(context.Background(), &SessionBinding{IP: "203.0.113.9", UserAgent: "Mozilla/5.0"})

	for i := 0; i < 2; i++ {
		auth.notifyLoginTelegram(ctx, &User{ID: 7})
		select {
		case msg := <-sent:
			require.Equal(t, "777", msg.chatID)
			require.Contains(t, msg.text, "203.0.113.9")
			require.Contains(t, msg.text, "Mozilla/5.0")
		case <-time.After(5 * time.Second):
			t.Fatalf("login notification %d not sent", i+1)
		}
	}
}
