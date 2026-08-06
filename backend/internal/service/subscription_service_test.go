//go:build unit

package service

import (
	"errors"
	"testing"
	"time"
)

func TestValidateAndCheckLimits_TokenSubscriptionOverLimit(t *testing.T) {
	daily := int64(1000)
	grp := &Group{SubscriptionType: SubscriptionTypeSubscriptionToken, DailyLimitTokens: &daily}
	sub := &UserSubscription{
		Status:           SubscriptionStatusActive,
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		DailyWindowStart: ptrTime(time.Now()),
		DailyUsageTokens: 1001, // 超过 1000 上限
	}

	svc := &SubscriptionService{now: time.Now} // 内存检查不依赖 repo
	_, err := svc.ValidateAndCheckLimits(sub, grp)
	if !errors.Is(err, ErrDailyLimitExceeded) {
		t.Fatalf("预期 ErrDailyLimitExceeded，得到 %v", err)
	}
}

func TestValidateAndCheckLimits_TokenSubscriptionUnderLimit(t *testing.T) {
	daily := int64(1000)
	grp := &Group{SubscriptionType: SubscriptionTypeSubscriptionToken, DailyLimitTokens: &daily}
	sub := &UserSubscription{
		Status:           SubscriptionStatusActive,
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		DailyWindowStart: ptrTime(time.Now()),
		DailyUsageTokens: 500,
	}

	svc := &SubscriptionService{now: time.Now}
	_, err := svc.ValidateAndCheckLimits(sub, grp)
	if err != nil {
		t.Fatalf("未超限应放行，得到 %v", err)
	}
}

func TestCalculateProgress_TokenSubscription(t *testing.T) {
	daily := int64(1000)
	grp := &Group{SubscriptionType: SubscriptionTypeSubscriptionToken, DailyLimitTokens: &daily}
	start := time.Now().Add(-1 * time.Hour)
	sub := &UserSubscription{
		ID: 1, ExpiresAt: time.Now().Add(24 * time.Hour),
		DailyWindowStart: &start, DailyUsageTokens: 250,
	}
	svc := &SubscriptionService{now: time.Now}
	p := svc.calculateProgress(sub, grp)
	if p.Daily == nil {
		t.Fatal("token 订阅应产出 Daily 进度")
	}
	if p.Daily.LimitTokens != 1000 || p.Daily.UsedTokens != 250 {
		t.Errorf("token 进度应为 limit=1000 used=250，得到 %+v", p.Daily)
	}
	if p.Daily.LimitUSD != 0 || p.Daily.UsedUSD != 0 {
		t.Errorf("token 订阅 USD 维度应为 0，得到 %+v", p.Daily)
	}
}
