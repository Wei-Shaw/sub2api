package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func strPtr(value string) *string {
	return &value
}

func telegramBoolPtr(value bool) *bool {
	return &value
}

func TestApplyTelegramSettingsFromRequestMapsAllNotificationFields(t *testing.T) {
	previous := &service.SystemSettings{
		TelegramBotToken:             "tg-fixture",
		TelegramBotTokenConfigured:   true,
		TelegramChatID:               "prev-chat",
		TelegramNotifyNewUser:        false,
		TelegramNotifyAccountError:   false,
		TelegramNotifyAccountExpired: false,
		TelegramNotifyPaymentSuccess: false,
		TelegramNotifyPaymentFailed:  false,
		TelegramNotifyRefund:         false,
		TelegramNotifySubExpired:     false,
		TelegramNotifyBalanceLow:     false,
		TelegramNotifyOpsAlert:       false,
		TelegramNotifyProxyExpired:   false,
	}
	settings := &service.SystemSettings{}
	req := UpdateSettingsRequest{
		TelegramChatID:               strPtr("new-chat"),
		TelegramNotifyNewUser:        telegramBoolPtr(true),
		TelegramNotifyAccountError:   telegramBoolPtr(true),
		TelegramNotifyAccountExpired: telegramBoolPtr(true),
		TelegramNotifyPaymentSuccess: telegramBoolPtr(true),
		TelegramNotifyPaymentFailed:  telegramBoolPtr(true),
		TelegramNotifyRefund:         telegramBoolPtr(true),
		TelegramNotifySubExpired:     telegramBoolPtr(true),
		TelegramNotifyBalanceLow:     telegramBoolPtr(true),
		TelegramNotifyOpsAlert:       telegramBoolPtr(true),
		TelegramNotifyProxyExpired:   telegramBoolPtr(true),
	}

	applyTelegramSettingsFromRequest(settings, req, previous)

	if settings.TelegramBotToken != previous.TelegramBotToken {
		t.Fatalf("expected omitted bot token to preserve previous token")
	}
	if !settings.TelegramBotTokenConfigured {
		t.Fatalf("expected omitted bot token to preserve configured flag")
	}
	if settings.TelegramChatID != "new-chat" {
		t.Fatalf("expected chat id from request, got %q", settings.TelegramChatID)
	}
	assertTelegramNotifications(t, settings, true)
}

func TestApplyTelegramSettingsFromRequestPreservesOmittedNotificationFields(t *testing.T) {
	previous := &service.SystemSettings{
		TelegramBotToken:             "tg-fixture",
		TelegramBotTokenConfigured:   true,
		TelegramChatID:               "prev-chat",
		TelegramNotifyNewUser:        true,
		TelegramNotifyAccountError:   true,
		TelegramNotifyAccountExpired: true,
		TelegramNotifyPaymentSuccess: true,
		TelegramNotifyPaymentFailed:  true,
		TelegramNotifyRefund:         true,
		TelegramNotifySubExpired:     true,
		TelegramNotifyBalanceLow:     true,
		TelegramNotifyOpsAlert:       true,
		TelegramNotifyProxyExpired:   true,
	}
	settings := &service.SystemSettings{}

	applyTelegramSettingsFromRequest(settings, UpdateSettingsRequest{}, previous)

	if settings.TelegramBotToken != previous.TelegramBotToken {
		t.Fatalf("expected omitted bot token to preserve previous token")
	}
	if !settings.TelegramBotTokenConfigured {
		t.Fatalf("expected omitted bot token to preserve configured flag")
	}
	if settings.TelegramChatID != previous.TelegramChatID {
		t.Fatalf("expected omitted chat id to preserve previous chat id")
	}
	assertTelegramNotifications(t, settings, true)
}

func assertTelegramNotifications(t *testing.T, settings *service.SystemSettings, expected bool) {
	t.Helper()

	checks := map[string]bool{
		"TelegramNotifyNewUser":        settings.TelegramNotifyNewUser,
		"TelegramNotifyAccountError":   settings.TelegramNotifyAccountError,
		"TelegramNotifyAccountExpired": settings.TelegramNotifyAccountExpired,
		"TelegramNotifyPaymentSuccess": settings.TelegramNotifyPaymentSuccess,
		"TelegramNotifyPaymentFailed":  settings.TelegramNotifyPaymentFailed,
		"TelegramNotifyRefund":         settings.TelegramNotifyRefund,
		"TelegramNotifySubExpired":     settings.TelegramNotifySubExpired,
		"TelegramNotifyBalanceLow":     settings.TelegramNotifyBalanceLow,
		"TelegramNotifyOpsAlert":       settings.TelegramNotifyOpsAlert,
		"TelegramNotifyProxyExpired":   settings.TelegramNotifyProxyExpired,
	}
	for name, got := range checks {
		if got != expected {
			t.Fatalf("%s = %v, want %v", name, got, expected)
		}
	}
}
