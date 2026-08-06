//go:build unit

package service

import "testing"

func TestUserSubscriptionTokenLimitCheck(t *testing.T) {
	daily := int64(1000)
	grp := &Group{SubscriptionType: SubscriptionTypeSubscriptionToken, DailyLimitTokens: &daily}
	sub := &UserSubscription{DailyUsageTokens: 900}

	if !sub.CheckDailyTokenLimit(grp, 50) {
		t.Error("未超限应放行")
	}
	if sub.CheckDailyTokenLimit(grp, 101) {
		t.Error("超限应拦截")
	}

	grpNoLimit := &Group{SubscriptionType: SubscriptionTypeSubscriptionToken}
	if !sub.CheckDailyTokenLimit(grpNoLimit, 999999) {
		t.Error("未设限额应始终放行")
	}
}
