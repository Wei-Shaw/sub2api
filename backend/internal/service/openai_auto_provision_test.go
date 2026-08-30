package service

import (
	"testing"
	"time"
)

func TestCountHealthyOpenAIOAuthAccounts(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, ParentAccountID: openAIInt64Ptr(99)},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusError, Schedulable: true},
		{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true},
		{ID: 5, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: false},
	}

	if got := countHealthyOpenAIOAuthAccounts(accounts); got != 1 {
		t.Fatalf("countHealthyOpenAIOAuthAccounts() = %d, want 1", got)
	}
}

func TestOpenAIProvisionDeficit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		healthy int
		target  int
		want    int
	}{
		{name: "below target", healthy: 3, target: 8, want: 5},
		{name: "at target", healthy: 8, target: 8, want: 0},
		{name: "above target", healthy: 9, target: 8, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := openAIProvisionDeficit(tc.healthy, tc.target); got != tc.want {
				t.Fatalf("openAIProvisionDeficit(%d, %d) = %d, want %d", tc.healthy, tc.target, got, tc.want)
			}
		})
	}
}

func TestConsumeAutoProvisionCallbackIsIdempotent(t *testing.T) {
	state := &autoProvisionState{
		Provision: &autoProvisionPending{RequestID: "req-1", RequestedCount: 4},
	}
	req := autoProvisionCallback{RequestID: "req-1", EventID: "req-1:completed", Kind: "registration", Status: "completed"}

	accepted, replay, err := consumeAutoProvisionCallback(state, req)
	if err != nil || !accepted || replay || state.Provision != nil {
		t.Fatalf("first callback = accepted %v, replay %v, err %v, state %#v", accepted, replay, err, state)
	}

	accepted, replay, err = consumeAutoProvisionCallback(state, req)
	if err != nil || !accepted || !replay {
		t.Fatalf("duplicate callback = accepted %v, replay %v, err %v", accepted, replay, err)
	}
}

func TestConsumeAutoProvisionRegistrationCallbackRequiresPendingRequest(t *testing.T) {
	callback := autoProvisionCallback{
		RequestID: "req-unknown", EventID: "req-unknown:registration:completed",
		Kind: "registration", Status: "completed",
	}
	accepted, replay, err := consumeAutoProvisionCallback(newAutoProvisionState(), callback)
	if err == nil || accepted || replay {
		t.Fatalf("unsolicited registration callback = accepted %v, replay %v, err %v", accepted, replay, err)
	}
}

func TestValidateReauthorizationIdentity(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"email":              "person@example.com",
			"chatgpt_account_id": "acct-1",
		},
	}
	valid := &OpenAITokenInfo{
		AccessToken:      "access-token",
		Email:            "PERSON@example.com",
		ChatGPTAccountID: "acct-1",
	}
	if err := validateReauthorizationIdentity(account, "person@example.com", valid); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
	for name, tokenInfo := range map[string]*OpenAITokenInfo{
		"email mismatch":       {AccessToken: "access-token", Email: "other@example.com", ChatGPTAccountID: "acct-1"},
		"account mismatch":     {AccessToken: "access-token", Email: "person@example.com", ChatGPTAccountID: "acct-2"},
		"missing access token": {Email: "person@example.com", ChatGPTAccountID: "acct-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateReauthorizationIdentity(account, "person@example.com", tokenInfo); err == nil {
				t.Fatal("expected identity validation error")
			}
		})
	}
}

func TestParseReauthorizationCallbackURL(t *testing.T) {
	code, state, err := parseReauthorizationCallbackURL("http://localhost:1455/auth/callback?code=authorization-code&state=oauth-state")
	if err != nil {
		t.Fatalf("valid callback URL rejected: %v", err)
	}
	if code != "authorization-code" || state != "oauth-state" {
		t.Fatalf("parsed callback = code %q state %q", code, state)
	}
	for _, raw := range []string{
		"http://localhost:1455/auth/callback?code=only-code",
		"https://user:pass@example.com/auth/callback?code=c&state=s",
		"ftp://localhost/auth/callback?code=c&state=s",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, _, err := parseReauthorizationCallbackURL(raw); err == nil {
				t.Fatal("expected invalid callback URL error")
			}
		})
	}
}

func TestConsumeAutoProvisionReauthorizationCompletionRequiresOAuthCallback(t *testing.T) {
	state := newAutoProvisionState()
	callback := autoProvisionCallback{
		RequestID: "req-reauth", EventID: "req-reauth:reauthorization:completed",
		Kind: "reauthorization", Status: "succeeded", AccountID: 42,
	}
	accepted, replay, err := consumeAutoProvisionCallback(state, callback)
	if err == nil || accepted || replay {
		t.Fatalf("unsolicited reauthorization completion = accepted %v, replay %v, err %v", accepted, replay, err)
	}

	state.ProcessedEvents["req-reauth:reauthorization:oauth-callback"] = time.Now().UTC()
	accepted, replay, err = consumeAutoProvisionCallback(state, callback)
	if err != nil || !accepted || replay {
		t.Fatalf("completion after oauth callback = accepted %v, replay %v, err %v", accepted, replay, err)
	}
}

func openAIInt64Ptr(value int64) *int64 { return &value }
