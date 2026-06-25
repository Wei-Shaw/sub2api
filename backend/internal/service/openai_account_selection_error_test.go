//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
)

func TestRenderOpenAIAccountSelectionErrorMessage(t *testing.T) {
	msg := RenderOpenAIAccountSelectionErrorMessage(
		"账户额度已用完；最快重置：{{next_reset_at}}；账号：{{next_reset_account}}；详情：{{account_summary}}",
		OpenAIAccountSelectionErrorTemplateData{
			NextResetAt:      "2026-06-12T21:00:00+08:00",
			NextResetAccount: "team",
			AccountSummary:   "team 5h 100% 5h reset 2026-06-12T21:00:00+08:00",
		},
	)

	for _, want := range []string{"账户额度已用完", "2026-06-12T21:00:00+08:00", "team 5h 100%"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q does not contain %q", msg, want)
		}
	}
}

func TestBuildOpenAIAccountQuotaSummary(t *testing.T) {
	now := time.Date(2026, 6, 12, 16, 0, 0, 0, time.UTC)
	reset5h := now.Add(2 * time.Hour).Format(time.RFC3339)
	reset7d := now.Add(24 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       42,
		Name:     "team",
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_5h_used_percent": "100",
			"codex_5h_reset_at":     reset5h,
			"codex_7d_used_percent": 88.0,
			"codex_7d_reset_at":     reset7d,
		},
	}

	item := buildAccountSelectionQuotaSummary(account, now)
	if item.NextResetAt == nil || !item.NextResetAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("NextResetAt = %v, want %v", item.NextResetAt, now.Add(2*time.Hour))
	}
	summary := joinAccountSelectionQuotaSummaries([]accountSelectionQuotaSummary{item})
	if !strings.Contains(summary, "team") || !strings.Contains(summary, "5h 100%") || !strings.Contains(summary, "7d 88%") {
		t.Fatalf("unexpected summary: %s", summary)
	}
}

func TestAccountSelectionErrorTemplateDataIncludesNonOpenAIAccounts(t *testing.T) {
	now := time.Now()
	repo := &openAISelectionErrorAccountRepo{
		accounts: []Account{
			{
				ID:          10,
				Name:        "gemini-api",
				Platform:    PlatformGemini,
				Status:      StatusActive,
				Schedulable: false,
				Extra: map[string]any{
					"quota_daily_limit":    10.0,
					"quota_daily_used":     10.0,
					"quota_daily_reset_at": now.Add(time.Hour).Format(time.RFC3339),
				},
			},
			{
				ID:          11,
				Name:        "anthropic-api",
				Platform:    PlatformAnthropic,
				Status:      StatusActive,
				Schedulable: false,
			},
		},
	}
	groupID := int64(18)
	svc := &GatewayService{accountRepo: repo}

	data := svc.AccountSelectionErrorTemplateData(context.Background(), PlatformGemini, &groupID)
	if data.NextResetAccount != "gemini-api" {
		t.Fatalf("NextResetAccount = %q, want gemini-api", data.NextResetAccount)
	}
	for _, want := range []string{"gemini-api", "platform gemini", "daily_quota 10/10", "daily_reset"} {
		if !strings.Contains(data.AccountSummary, want) {
			t.Fatalf("AccountSummary %q does not contain %q", data.AccountSummary, want)
		}
	}
	if strings.Contains(data.AccountSummary, "anthropic-api") {
		t.Fatalf("AccountSummary should prefer requested platform accounts, got %q", data.AccountSummary)
	}
}

type openAISelectionErrorAccountRepo struct {
	accountRepoStub
	accounts []Account
}

func (r *openAISelectionErrorAccountRepo) ListByGroup(context.Context, int64) ([]Account, error) {
	return r.accounts, nil
}

func TestOpenAIAccountSelectionErrorTemplateData(t *testing.T) {
	now := time.Now()
	repo := &openAISelectionErrorAccountRepo{
		accounts: []Account{
			{
				ID:       2,
				Name:     "later",
				Platform: PlatformOpenAI,
				Extra: map[string]any{
					"codex_5h_used_percent": 100.0,
					"codex_5h_reset_at":     now.Add(2 * time.Hour).Format(time.RFC3339),
				},
			},
			{
				ID:       1,
				Name:     "soon",
				Platform: PlatformOpenAI,
				Extra: map[string]any{
					"codex_7d_used_percent": 99.0,
					"codex_7d_reset_at":     now.Add(30 * time.Minute).Format(time.RFC3339),
				},
			},
			{
				ID:       3,
				Name:     "anthropic",
				Platform: PlatformAnthropic,
			},
		},
	}
	groupID := int64(18)
	svc := &OpenAIGatewayService{accountRepo: repo}

	data := svc.OpenAIAccountSelectionErrorTemplateData(context.Background(), &groupID)
	if data.NextResetAccount != "soon" {
		t.Fatalf("NextResetAccount = %q, want soon", data.NextResetAccount)
	}
	if data.NextResetAt == "" {
		t.Fatal("NextResetAt is empty")
	}
	if !strings.Contains(data.AccountSummary, "soon") || !strings.Contains(data.AccountSummary, "later") {
		t.Fatalf("unexpected AccountSummary: %s", data.AccountSummary)
	}
}

func TestOpenAIAccountSelectionNoAvailableMatchesErrorPassthroughRule(t *testing.T) {
	custom := "账户额度已用完，最快重置：{{next_reset_at}}"
	respCode := 429
	svc := &ErrorPassthroughService{}
	svc.setLocalCache([]*model.ErrorPassthroughRule{
		{
			ID:              1,
			Name:            "local OpenAI no available accounts",
			Enabled:         true,
			Priority:        0,
			ErrorCodes:      []int{OpenAIAccountSelectionNoAvailableStatus},
			Keywords:        []string{OpenAIAccountSelectionNoAvailableMessage},
			MatchMode:       model.MatchModeAll,
			Platforms:       []string{PlatformOpenAI},
			PassthroughCode: false,
			ResponseCode:    &respCode,
			PassthroughBody: false,
			CustomMessage:   &custom,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		},
	})

	matched := svc.MatchRule(PlatformOpenAI, OpenAIAccountSelectionNoAvailableStatus, []byte(OpenAIAccountSelectionNoAvailableMessage))
	if matched == nil {
		t.Fatal("expected local no available accounts rule to match")
	}
	if matched.CustomMessage == nil || *matched.CustomMessage != custom {
		t.Fatalf("CustomMessage = %v, want %q", matched.CustomMessage, custom)
	}
	if matched.ResponseCode == nil || *matched.ResponseCode != respCode {
		t.Fatalf("ResponseCode = %v, want %d", matched.ResponseCode, respCode)
	}
}
