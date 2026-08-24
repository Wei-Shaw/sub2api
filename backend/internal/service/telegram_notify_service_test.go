package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type telegramSettingRepositoryStub struct {
	values map[string]string
}

func (s telegramSettingRepositoryStub) Get(context.Context, string) (*Setting, error) {
	return nil, nil
}

func (s telegramSettingRepositoryStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s telegramSettingRepositoryStub) Set(context.Context, string, string) error { return nil }

func (s telegramSettingRepositoryStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		values[key] = s.values[key]
	}
	return values, nil
}

func (s telegramSettingRepositoryStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (s telegramSettingRepositoryStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
}
func (s telegramSettingRepositoryStub) Delete(context.Context, string) error { return nil }

func TestTelegramNotifyServiceSendsNewUserMessage(t *testing.T) {
	t.Parallel()

	var received struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/sendMessage" {
			t.Errorf("request path = %q, want %q", r.URL.Path, "/bottest-token/sendMessage")
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse request form: %v", err)
		}
		received.ChatID = r.Form.Get("chat_id")
		received.Text = r.Form.Get("text")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()

	svc := NewTelegramNotifyService(telegramSettingRepositoryStub{values: map[string]string{
		SettingTelegramBotToken:      "test-token",
		SettingTelegramChatID:        "12345",
		SettingTelegramNotifyNewUser: "true",
	}})
	svc.apiBaseURL = server.URL + "/bot"

	svc.NotifyNewUser(context.Background(), "alice@example.com", "email")

	if received.ChatID != "12345" {
		t.Fatalf("chat_id = %q, want %q", received.ChatID, "12345")
	}
	for _, want := range []string{"New User Registered", "alice", "alice@example.com", "email"} {
		if !strings.Contains(received.Text, want) {
			t.Errorf("message %q does not contain %q", received.Text, want)
		}
	}
	if _, err := url.Parse(server.URL); err != nil {
		t.Fatalf("test server URL is invalid: %v", err)
	}
}
