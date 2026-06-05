//go:build unit

package service

import (
	"testing"
)

// TestBuildServiceQuotaRecordRequest_PreservesPathFields locks in the fix that
// the Record-side service_quota request retains the Model / ChannelID / AccountID
// originally supplied by the gateway handler. Without this, findMatchingPaths
// is unable to match RPM/TPM/TPD/daily_usd rules whose path criteria reference
// model patterns or channel IDs (CountOnArrival=false rule type), causing all
// counters to remain at 0 across OpenAI ChatCompletions / Responses /
// WebSocket Realtime / Images and Anthropic chat-completions / responses
// adapter paths.
func TestBuildServiceQuotaRecordRequest_PreservesPathFields(t *testing.T) {
	t.Parallel()

	groupID := int64(7)
	const (
		userID    = int64(1)
		apiKeyID  = int64(2)
		accountID = int64(3)
		channelID = int64(99)
		model     = "gpt-5-mini"
		platform  = "openai"
	)

	p := &postUsageBillingParams{
		Cost:    &CostBreakdown{TotalCost: 1.5, ActualCost: 1.0},
		User:    &User{ID: userID},
		APIKey:  &APIKey{ID: apiKeyID, GroupID: &groupID, Group: &Group{ID: groupID, Platform: platform}},
		Account: &Account{ID: accountID},
		ServiceQuotaRequest: ServiceQuotaCheckRequest{
			Model:     model,
			AccountID: accountID,
			ChannelID: channelID,
		},
		InputTokens:         100,
		OutputTokens:        200,
		CacheCreationTokens: 50,
		CacheReadTokens:     25,
	}

	req, ok := buildServiceQuotaRecordRequest(p)
	if !ok {
		t.Fatal("buildServiceQuotaRecordRequest returned ok=false; expected true with all fields populated")
	}

	// Path-matching fields — the actual bug fixed by Commits 1 & 2.
	if req.Model != model {
		t.Errorf("req.Model = %q, want %q (findMatchingPaths needs non-empty model to evaluate model_pattern rules)", req.Model, model)
	}
	if req.ChannelID != channelID {
		t.Errorf("req.ChannelID = %d, want %d (findMatchingPaths needs ChannelID to match channel-scoped rules)", req.ChannelID, channelID)
	}
	if req.AccountID != accountID {
		t.Errorf("req.AccountID = %d, want %d", req.AccountID, accountID)
	}

	// Server-side overrides (UserID / GroupID / Platform sourced from APIKey/User).
	if req.UserID != userID {
		t.Errorf("req.UserID = %d, want %d", req.UserID, userID)
	}
	if req.GroupID != groupID {
		t.Errorf("req.GroupID = %d, want %d", req.GroupID, groupID)
	}
	if req.Platform != platform {
		t.Errorf("req.Platform = %q, want %q", req.Platform, platform)
	}

	// Token components — Commits 1 must populate these so TPM/TPD limiters
	// can sum the configured token_components. Anthropic native /v1/messages
	// path already populated them; OpenAI path was missing them entirely.
	if req.InputTokens != 100 {
		t.Errorf("req.InputTokens = %d, want 100", req.InputTokens)
	}
	if req.OutputTokens != 200 {
		t.Errorf("req.OutputTokens = %d, want 200", req.OutputTokens)
	}
	if req.CacheCreationTokens != 50 {
		t.Errorf("req.CacheCreationTokens = %d, want 50", req.CacheCreationTokens)
	}
	if req.CacheReadTokens != 25 {
		t.Errorf("req.CacheReadTokens = %d, want 25", req.CacheReadTokens)
	}
	if req.Cost != 1.0 {
		t.Errorf("req.Cost = %v, want 1.0 (daily_usd limiter uses ActualCost)", req.Cost)
	}
}

// TestOpenAIRecordUsageInput_HasServiceQuotaRequestField is a compile-time
// guard ensuring OpenAIRecordUsageInput exposes the ServiceQuotaRequest field
// the handler layer must populate. If this test fails to compile, someone has
// removed the field and reintroduced the bug fixed by Commit 1.
func TestOpenAIRecordUsageInput_HasServiceQuotaRequestField(t *testing.T) {
	t.Parallel()

	in := OpenAIRecordUsageInput{
		ServiceQuotaRequest: ServiceQuotaCheckRequest{
			Model:     "gpt-5",
			AccountID: 42,
			ChannelID: 7,
		},
	}
	if in.ServiceQuotaRequest.Model != "gpt-5" {
		t.Fatalf("ServiceQuotaRequest.Model not preserved on OpenAIRecordUsageInput: got %q", in.ServiceQuotaRequest.Model)
	}
	if in.ServiceQuotaRequest.ChannelID != 7 {
		t.Fatalf("ServiceQuotaRequest.ChannelID not preserved on OpenAIRecordUsageInput: got %d", in.ServiceQuotaRequest.ChannelID)
	}
}
