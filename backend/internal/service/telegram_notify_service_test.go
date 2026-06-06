package service

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type telegramNotifyTestSettingRepo struct {
	values map[string]string
}

func (r *telegramNotifyTestSettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *telegramNotifyTestSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if r == nil || r.values == nil {
		return "", ErrSettingNotFound
	}
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *telegramNotifyTestSettingRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *telegramNotifyTestSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	if r == nil || r.values == nil {
		return out, nil
	}
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *telegramNotifyTestSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *telegramNotifyTestSettingRepo) GetAll(context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *telegramNotifyTestSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTelegramNotifyService_SendTestMessageWithOverrides_UsesUnsavedTokenAndChatID(t *testing.T) {
	repo := &telegramNotifyTestSettingRepo{values: map[string]string{}}
	svc := NewTelegramNotifyService(repo)

	var requestedURL string
	var requestForm url.Values
	svc.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedURL = req.URL.String()
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			requestForm, err = url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse request body: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	err := svc.SendTestMessageWithOverrides(
		context.Background(),
		"telegram-test-token-fixture",
		"-1001234567890",
	)
	if err != nil {
		t.Fatalf("SendTestMessageWithOverrides returned error: %v", err)
	}

	if !strings.Contains(requestedURL, "/bottelegram-test-token-fixture/sendMessage") {
		t.Fatalf("expected request URL to include the unsaved token override, got %q", requestedURL)
	}
	if got := requestForm.Get("chat_id"); got != "-1001234567890" {
		t.Fatalf("expected chat_id override, got %q", got)
	}
}

func TestTelegramNotifyService_SendTestMessageWithOverrides_KeepsSavedTokenWhenOnlyChatIDOverrides(t *testing.T) {
	repo := &telegramNotifyTestSettingRepo{values: map[string]string{
		SettingTelegramBotToken: "telegram-saved-token-fixture",
		SettingTelegramChatID:   "-1000000000000",
	}}
	svc := NewTelegramNotifyService(repo)

	var requestedURL string
	var requestForm url.Values
	svc.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedURL = req.URL.String()
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			requestForm, err = url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse request body: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	err := svc.SendTestMessageWithChatID(context.Background(), "-1001234567890")
	if err != nil {
		t.Fatalf("SendTestMessageWithChatID returned error: %v", err)
	}

	if !strings.Contains(requestedURL, "/bottelegram-saved-token-fixture/sendMessage") {
		t.Fatalf("expected request URL to include the saved token, got %q", requestedURL)
	}
	if got := requestForm.Get("chat_id"); got != "-1001234567890" {
		t.Fatalf("expected chat_id override, got %q", got)
	}
}
