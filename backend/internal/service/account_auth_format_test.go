package service

import (
	"testing"
	"time"
)

func TestAccountGetCredentialAsTimeFallsBackToExpired(t *testing.T) {
	expired := "2026-06-05T16:30:21+08:00"
	account := &Account{
		Credentials: map[string]any{
			"expired": expired,
		},
	}

	got := account.GetCredentialAsTime("expires_at")
	if got == nil {
		t.Fatal("GetCredentialAsTime returned nil")
	}
	want, err := time.Parse(time.RFC3339, expired)
	if err != nil {
		t.Fatalf("parse expected time: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("GetCredentialAsTime = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestAccountGetChatGPTAccountIDFallsBackToAccountID(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"account_id": "acct-current-format",
		},
	}

	if got := account.GetChatGPTAccountID(); got != "acct-current-format" {
		t.Fatalf("GetChatGPTAccountID = %q, want acct-current-format", got)
	}
}
