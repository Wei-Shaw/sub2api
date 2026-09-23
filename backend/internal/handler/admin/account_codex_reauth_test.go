package admin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func newCodexReauthTestAccount(accessToken string, extraCreds map[string]any) service.Account {
	creds := map[string]any{
		"chatgpt_account_id": "workspace-1",
		"chatgpt_user_id":    "user-1",
		"access_token":       accessToken,
		"model_mapping":      map[string]any{"gpt-5.5": "gpt-5.5"},
	}
	for k, v := range extraCreds {
		creds[k] = v
	}
	return service.Account{
		ID:          10,
		Name:        "existing",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusError,
		Credentials: creds,
	}
}

func buildCodexAuthJSON(t *testing.T, accessToken, refreshToken string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": nil,
		"tokens": map[string]any{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"account_id":    "workspace-1",
		},
		"last_refresh": time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("marshal auth.json: %v", err)
	}
	return string(raw)
}

func TestReauthCodexSessionReplacesCredentialsOfTargetAccountOnly(t *testing.T) {
	oldToken := buildCodexAccessToken(t, "workspace-1", "user-1", time.Now().Add(time.Hour))
	existing := newCodexReauthTestAccount(oldToken, map[string]any{"refresh_token": "rt-old", "client_id": "old-client"})
	svc := newCodexImportMemoryAdminService([]service.Account{existing})
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	newToken := buildCodexAccessToken(t, "workspace-1", "user-1", time.Now().Add(10*24*time.Hour))
	result, err := handler.reauthCodexSession(context.Background(), &existing, buildCodexAuthJSON(t, newToken, "rt-new"))
	if err != nil {
		t.Fatalf("reauthCodexSession error = %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if len(svc.createdAccounts) != 0 {
		t.Fatalf("created accounts = %d, want 0", len(svc.createdAccounts))
	}
	if len(svc.updatedAccounts) != 1 || svc.updatedAccounts[0].id != 10 {
		t.Fatalf("updated accounts = %+v, want only account 10", svc.updatedAccounts)
	}
	input := svc.updatedAccounts[0].input
	if got := input.Credentials["access_token"]; got != newToken {
		t.Fatalf("access_token not replaced")
	}
	if got := input.Credentials["refresh_token"]; got != "rt-new" {
		t.Fatalf("refresh_token = %v, want rt-new", got)
	}
	if _, ok := input.Credentials["model_mapping"]; !ok {
		t.Fatal("model_mapping should be preserved")
	}
	if _, ok := input.Credentials["expires_at"]; !ok {
		t.Fatal("expires_at should be derived from the access token")
	}
	if input.Concurrency != nil || input.Priority != nil || input.GroupIDs != nil || input.ProxyID != nil {
		t.Fatalf("scheduling fields must not be touched: %+v", input)
	}
	if input.ExpiresAt != nil || input.AutoPauseOnExpired != nil {
		t.Fatalf("account expiry must not be set when refresh_token is present: %+v", input)
	}
	if svc.updateAccountExtraCalls != 1 {
		t.Fatalf("UpdateAccountExtra calls = %d, want 1 (key-level merge)", svc.updateAccountExtraCalls)
	}
}

func TestReauthCodexSessionRejectsDifferentUser(t *testing.T) {
	oldToken := buildCodexAccessToken(t, "workspace-1", "user-1", time.Now().Add(time.Hour))
	existing := newCodexReauthTestAccount(oldToken, nil)
	svc := newCodexImportMemoryAdminService([]service.Account{existing})
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	otherToken := buildCodexAccessToken(t, "workspace-1", "user-2", time.Now().Add(time.Hour))
	_, err := handler.reauthCodexSession(context.Background(), &existing, buildCodexAuthJSON(t, otherToken, "rt-other"))
	if err == nil || !strings.Contains(err.Error(), "chatgpt_user_id") {
		t.Fatalf("err = %v, want chatgpt_user_id mismatch", err)
	}
	if len(svc.updatedAccounts) != 0 {
		t.Fatalf("updated accounts = %d, want 0", len(svc.updatedAccounts))
	}
}

func TestReauthCodexSessionRejectsMultipleEntries(t *testing.T) {
	oldToken := buildCodexAccessToken(t, "workspace-1", "user-1", time.Now().Add(time.Hour))
	existing := newCodexReauthTestAccount(oldToken, nil)
	svc := newCodexImportMemoryAdminService([]service.Account{existing})
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	a := buildCodexAuthJSON(t, buildCodexAccessToken(t, "workspace-1", "user-1", time.Now().Add(time.Hour)), "rt-a")
	b := buildCodexAuthJSON(t, buildCodexAccessToken(t, "workspace-1", "user-1", time.Now().Add(2*time.Hour)), "rt-b")
	_, err := handler.reauthCodexSession(context.Background(), &existing, "["+a+","+b+"]")
	if err == nil {
		t.Fatal("expected error for multiple entries")
	}
	if len(svc.updatedAccounts) != 0 {
		t.Fatalf("updated accounts = %d, want 0", len(svc.updatedAccounts))
	}
}

func TestReauthCodexSessionAccessTokenOnlyKeepsExistingRefreshToken(t *testing.T) {
	oldToken := buildCodexAccessToken(t, "workspace-1", "user-1", time.Now().Add(time.Hour))
	existing := newCodexReauthTestAccount(oldToken, map[string]any{"refresh_token": "rt-old", "client_id": "old-client"})
	svc := newCodexImportMemoryAdminService([]service.Account{existing})
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	newToken := buildCodexAccessToken(t, "workspace-1", "user-1", time.Now().Add(2*time.Hour))
	result, err := handler.reauthCodexSession(context.Background(), &existing, newToken)
	if err != nil {
		t.Fatalf("reauthCodexSession error = %v", err)
	}
	input := svc.updatedAccounts[0].input
	if got := input.Credentials["refresh_token"]; got != "rt-old" {
		t.Fatalf("refresh_token = %v, want rt-old", got)
	}
	if got := input.Credentials["client_id"]; got != "old-client" {
		t.Fatalf("client_id = %v, want old-client", got)
	}
	if input.ExpiresAt != nil || input.AutoPauseOnExpired != nil {
		t.Fatalf("account expiry must stay untouched when refresh_token is preserved: %+v", input)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected a warning about the preserved refresh_token")
	}
}
