package service

import (
	"testing"
	"time"
)

func TestOpenAIProvisionPlanDoesNotProvisionWithoutRecentDemand(t *testing.T) {
	plan := calculateOpenAIProvisionPlan(OpenAIProvisionDemand{}, 1, 0, 100, time.Now().UTC())
	if plan.ShouldProvision {
		t.Fatalf("idle pool should not provision: %#v", plan)
	}
	if plan.RequestedCount != 0 {
		t.Fatalf("idle pool requested %d accounts, want 0", plan.RequestedCount)
	}
}

func TestOpenAIProvisionPlanDoesNotFillForSequentialUsers(t *testing.T) {
	demand := OpenAIProvisionDemand{ActiveUsers: 3, Requests: 30, Tokens: 900_000}
	plan := calculateOpenAIProvisionPlan(demand, 1, 1, 100, time.Now().UTC())
	if plan.ShouldProvision {
		t.Fatalf("one account with quota headroom should serve sequential users: %#v", plan)
	}
}

func TestOpenAIProvisionPlanRespondsToCapacityDeniedUsers(t *testing.T) {
	demand := OpenAIProvisionDemand{Requests: 2, CapacityDeniedUsers: 3, CapacityDeniedRequests: 6}
	plan := calculateOpenAIProvisionPlan(demand, 1, 1, 100, time.Now().UTC())
	if plan.RequestedCount != 1 {
		t.Fatalf("requested %d accounts, want one conservative step-up for blocked users", plan.RequestedCount)
	}
}

func TestOpenAIProvisionPlanRespondsToNearExhaustedQuotaWithTraffic(t *testing.T) {
	demand := OpenAIProvisionDemand{Requests: 4, Tokens: openAIProvisionMinimumPrewarmTokens}
	plan := calculateOpenAIProvisionPlan(demand, 1, 0.15, 10, time.Now().UTC())
	if plan.RequestedCount != 1 {
		t.Fatalf("requested %d accounts, want one prewarmed replacement for near-exhausted capacity", plan.RequestedCount)
	}
}

func TestOpenAIProvisionPlanIgnoresNegligibleNearExhaustedTraffic(t *testing.T) {
	plan := calculateOpenAIProvisionPlan(
		OpenAIProvisionDemand{Requests: 1, Tokens: openAIProvisionMinimumPrewarmTokens - 1},
		1,
		0.15,
		10,
		time.Now().UTC(),
	)
	if plan.ShouldProvision {
		t.Fatalf("negligible near-exhausted traffic should not prewarm: %#v", plan)
	}
}

func TestOpenAIProvisionPlanCapsBurstStepUp(t *testing.T) {
	plan := calculateOpenAIProvisionPlan(
		OpenAIProvisionDemand{Requests: 100, CapacityDeniedUsers: 100, CapacityDeniedRequests: 100},
		1,
		1,
		100,
		time.Now().UTC(),
	)
	if plan.RequestedCount != openAIProvisionMaxImmediateAccounts {
		t.Fatalf("requested %d accounts, want capped step-up of %d", plan.RequestedCount, openAIProvisionMaxImmediateAccounts)
	}
}

func TestOpenAIProvisionPlanDoesNotProvisionWithUsableQuotaHeadroom(t *testing.T) {
	plan := calculateOpenAIProvisionPlan(
		OpenAIProvisionDemand{ActiveUsers: 1, Requests: 1, Tokens: 100},
		1,
		0.80,
		10,
		time.Now().UTC(),
	)
	if plan.ShouldProvision {
		t.Fatalf("usable quota headroom should not provision: %#v", plan)
	}
}

func TestOpenAIPoolEffectiveCapacityUsesQuotaHeadroom(t *testing.T) {
	now := time.Now().UTC()
	baseExtra := map[string]any{
		"codex_usage_updated_at": now.Format(time.RFC3339),
		"codex_5h_reset_at":      now.Add(time.Hour).Format(time.RFC3339),
		"codex_5h_used_percent":  90.0,
	}
	if got := openAIPoolEffectiveCapacity([]Account{{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: baseExtra}}, now); got != 0 {
		t.Fatalf("near-exhausted account effective capacity = %v, want 0", got)
	}
	baseExtra["codex_5h_used_percent"] = 20.0
	if got := openAIPoolEffectiveCapacity([]Account{{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: baseExtra}}, now); got != 1 {
		t.Fatalf("healthy quota effective capacity = %v, want 1", got)
	}
	if got := openAIPoolEffectiveCapacity([]Account{{Platform: PlatformOpenAI, Type: AccountTypeOAuth}}, now); got != 1 {
		t.Fatalf("unknown quota snapshot effective capacity = %v, want 1", got)
	}
}

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

func TestCountProvisionableOpenAIOAuthAccountsExcludesQuotaExhausted(t *testing.T) {
	now := time.Now().UTC()
	accounts := []Account{
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Extra: map[string]any{
			"codex_usage_updated_at": now.Format(time.RFC3339),
			"codex_5h_reset_at":      now.Add(time.Hour).Format(time.RFC3339),
			"codex_5h_used_percent":  100.0,
		}},
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
	}
	if got := countProvisionableOpenAIOAuthAccounts(accounts, now); got != 1 {
		t.Fatalf("countProvisionableOpenAIOAuthAccounts() = %d, want 1", got)
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
