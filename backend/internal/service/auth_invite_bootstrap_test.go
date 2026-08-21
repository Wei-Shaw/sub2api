package service

import "testing"

func TestSelectInviteBootstrapGroupsForRedeemChoosesBestActiveGroupPerPlatform(t *testing.T) {
	groups := []Group{
		{ID: 10, Platform: "openai", Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription, ActiveAccountCount: 1, DefaultValidityDays: 30, SortOrder: 2},
		{ID: 11, Platform: "openai", Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription, ActiveAccountCount: 1, DefaultValidityDays: 90, SortOrder: 9},
		{ID: 20, Platform: "gemini", Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription, ActiveAccountCount: 1, DefaultValidityDays: 30, SortOrder: 1},
		{ID: 21, Platform: "anthropic", Status: StatusDisabled, SubscriptionType: SubscriptionTypeSubscription, ActiveAccountCount: 1, DefaultValidityDays: 365},
		{ID: 22, Platform: "empty", Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription, ActiveAccountCount: 0, DefaultValidityDays: 365},
	}

	selected := selectInviteBootstrapGroupsForRedeem(&RedeemCode{Type: RedeemTypeInvitation}, groups)

	if got := selected["openai"].ID; got != 11 {
		t.Fatalf("openai group = %d, want 11", got)
	}
	if got := selected["gemini"].ID; got != 20 {
		t.Fatalf("gemini group = %d, want 20", got)
	}
	if _, ok := selected["anthropic"]; ok {
		t.Fatal("inactive group was selected")
	}
}
