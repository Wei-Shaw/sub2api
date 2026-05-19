package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func floatPtr(v float64) *float64 {
	return &v
}

func TestApplyResolvedOAuthPauseConfigUsesStrictestGroupFallback(t *testing.T) {
	account := &service.Account{
		Extra: map[string]any{},
		Groups: []*service.Group{
			{
				OAuth5hPausePercent: floatPtr(95),
				OAuth5hPauseAmount:  floatPtr(20),
				OAuth7dPausePercent: floatPtr(90),
				OAuth7dPauseAmount:  floatPtr(100),
			},
			{
				OAuth5hPausePercent: floatPtr(80),
				OAuth5hPauseAmount:  floatPtr(30),
				OAuth7dPausePercent: floatPtr(85),
				OAuth7dPauseAmount:  floatPtr(50),
			},
		},
	}

	applyResolvedOAuthPauseConfig(account)

	assertExtraFloat(t, account.Extra, "effective_oauth_5h_pause_percent", 80)
	assertExtraFloat(t, account.Extra, "effective_oauth_5h_pause_amount_usd", 20)
	assertExtraFloat(t, account.Extra, "effective_oauth_7d_pause_percent", 85)
	assertExtraFloat(t, account.Extra, "effective_oauth_7d_pause_amount_usd", 50)
}

func TestApplyResolvedOAuthPauseConfigAccountLevelOverridesGroupFallback(t *testing.T) {
	account := &service.Account{
		Extra: map[string]any{
			"oauth_5h_pause_percent":           95.0,
			"effective_oauth_5h_pause_percent": 80.0,
		},
		Groups: []*service.Group{
			{OAuth5hPausePercent: floatPtr(80)},
		},
	}

	applyResolvedOAuthPauseConfig(account)

	if _, ok := account.Extra["effective_oauth_5h_pause_percent"]; ok {
		t.Fatal("expected account-level config to remove effective group fallback")
	}
}

func assertExtraFloat(t *testing.T, extra map[string]any, key string, want float64) {
	t.Helper()
	got, ok := extra[key].(float64)
	if !ok {
		t.Fatalf("expected %s to be float64, got %#v", key, extra[key])
	}
	if got != want {
		t.Fatalf("expected %s=%v, got %v", key, want, got)
	}
}
