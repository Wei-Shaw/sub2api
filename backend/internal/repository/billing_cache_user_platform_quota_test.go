//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func newMiniRedisCache(t *testing.T) (*billingCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &billingCache{rdb: rdb}, mr
}

func TestUserPlatformQuotaCache_GetMissReturnsNotFound(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	entry, ok, err := c.GetUserPlatformQuotaCache(context.Background(), 1, "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if ok || entry != nil {
		t.Errorf("expected miss, got ok=%v entry=%v", ok, entry)
	}
}

func TestUserPlatformQuotaCache_SetThenGet(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	ctx := context.Background()
	fiveHourLimit := 4.0
	dailyLimit := 20.0
	ts := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	in := &service.UserPlatformQuotaCacheEntry{
		FiveHourUsageUSD:    0.75,
		DailyUsageUSD:       1.5,
		WeeklyUsageUSD:      3.0,
		MonthlyUsageUSD:     10.0,
		Version:             7,
		SchemaVersion:       service.UserPlatformQuotaCacheSchemaV2,
		ResetGeneration:     3,
		FiveHourLimitUSD:    &fiveHourLimit,
		DailyLimitUSD:       &dailyLimit,
		FiveHourWindowStart: &ts,
		DailyWindowStart:    &ts,
	}
	if err := c.SetUserPlatformQuotaCache(ctx, 1, "openai", in, time.Minute); err != nil {
		t.Fatal(err)
	}
	got, ok, err := c.GetUserPlatformQuotaCache(ctx, 1, "openai")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.FiveHourUsageUSD != 0.75 || got.DailyUsageUSD != 1.5 || got.WeeklyUsageUSD != 3.0 || got.MonthlyUsageUSD != 10.0 || got.Version != 7 {
		t.Errorf("got = %+v, want %+v", got, in)
	}
	if got.SchemaVersion != service.UserPlatformQuotaCacheSchemaV2 {
		t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, service.UserPlatformQuotaCacheSchemaV2)
	}
	if got.ResetGeneration != 3 {
		t.Errorf("ResetGeneration = %d, want 3", got.ResetGeneration)
	}
	if got.FiveHourLimitUSD == nil || *got.FiveHourLimitUSD != fiveHourLimit {
		t.Errorf("FiveHourLimitUSD = %v, want %v", got.FiveHourLimitUSD, fiveHourLimit)
	}
	if got.DailyLimitUSD == nil || *got.DailyLimitUSD != dailyLimit {
		t.Errorf("DailyLimitUSD = %v, want %v", got.DailyLimitUSD, dailyLimit)
	}
	if got.DailyWindowStart == nil || !got.DailyWindowStart.Equal(ts) {
		t.Errorf("DailyWindowStart = %v, want %v", got.DailyWindowStart, ts)
	}
}

func TestUserPlatformQuotaCache_NilLimitSetThenGet(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	ctx := context.Background()
	in := &service.UserPlatformQuotaCacheEntry{
		DailyUsageUSD: 1.0,
		SchemaVersion: service.UserPlatformQuotaCacheSchemaV2,
		// DailyLimitUSD nil → 无限额
	}
	if err := c.SetUserPlatformQuotaCache(ctx, 1, "openai", in, time.Minute); err != nil {
		t.Fatal(err)
	}
	got, ok, err := c.GetUserPlatformQuotaCache(ctx, 1, "openai")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.DailyLimitUSD != nil {
		t.Errorf("DailyLimitUSD should be nil for unlimited, got %v", got.DailyLimitUSD)
	}
}

func TestUserPlatformQuotaCache_IncrMissIsNoop(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	if err := c.IncrUserPlatformQuotaUsageCache(context.Background(), 1, "openai", 0.5, time.Minute, false); err != nil {
		t.Fatal(err)
	}
	_, ok, _ := c.GetUserPlatformQuotaCache(context.Background(), 1, "openai")
	if ok {
		t.Error("expected key absent after no-op incr")
	}
}

func TestUserPlatformQuotaCache_IncrHitAccumulates(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	ctx := context.Background()
	now := time.Now().UTC()
	// SchemaVersion 必须显式设为 V2,否则 Lua 脚本会因 schema 不匹配而 return 0,跳过累加。
	_ = c.SetUserPlatformQuotaCache(ctx, 1, "openai", &service.UserPlatformQuotaCacheEntry{
		Version:             1,
		SchemaVersion:       service.UserPlatformQuotaCacheSchemaV2,
		FiveHourWindowStart: &now,
	}, time.Minute)
	if err := c.IncrUserPlatformQuotaUsageCache(ctx, 1, "openai", 0.5, time.Minute, false); err != nil {
		t.Fatal(err)
	}
	if err := c.IncrUserPlatformQuotaUsageCache(ctx, 1, "openai", 0.25, time.Minute, false); err != nil {
		t.Fatal(err)
	}
	got, _, _ := c.GetUserPlatformQuotaCache(ctx, 1, "openai")
	if got.FiveHourUsageUSD != 0.75 || got.DailyUsageUSD != 0.75 || got.WeeklyUsageUSD != 0.75 || got.MonthlyUsageUSD != 0.75 {
		t.Errorf("got %+v, want all windows=0.75", got)
	}
	if got.Version != 3 {
		t.Errorf("version = %d, want 3 (initial 1 + 2 incr)", got.Version)
	}
}

func TestUserPlatformQuotaCache_IncrResetsExpiredFiveHourWindow(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	ctx := context.Background()
	start := time.Unix(time.Now().Unix()-int64((5*time.Hour).Seconds()), 0).UTC()
	_ = c.SetUserPlatformQuotaCache(ctx, 1, "openai", &service.UserPlatformQuotaCacheEntry{
		FiveHourUsageUSD:    9,
		DailyUsageUSD:       12,
		SchemaVersion:       service.UserPlatformQuotaCacheSchemaV2,
		FiveHourWindowStart: &start,
	}, time.Minute)

	if err := c.IncrUserPlatformQuotaUsageCache(ctx, 1, "openai", 0.5, time.Minute, false); err != nil {
		t.Fatal(err)
	}
	got, _, _ := c.GetUserPlatformQuotaCache(ctx, 1, "openai")
	if got.FiveHourUsageUSD != 0.5 {
		t.Errorf("expired five-hour usage = %v, want 0.5", got.FiveHourUsageUSD)
	}
	if got.DailyUsageUSD != 12.5 {
		t.Errorf("daily usage = %v, want 12.5", got.DailyUsageUSD)
	}
	if got.FiveHourWindowStart == nil || !got.FiveHourWindowStart.After(start) {
		t.Errorf("five-hour window start was not advanced: %v", got.FiveHourWindowStart)
	}
}

func TestUserPlatformQuotaCache_IncrOldSchemaIsNoop(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	ctx := context.Background()
	_ = c.SetUserPlatformQuotaCache(ctx, 1, "openai", &service.UserPlatformQuotaCacheEntry{
		DailyUsageUSD: 2,
		SchemaVersion: service.UserPlatformQuotaCacheSchemaV1,
	}, time.Minute)

	if err := c.IncrUserPlatformQuotaUsageCache(ctx, 1, "openai", 0.5, time.Minute, false); err != nil {
		t.Fatal(err)
	}
	got, _, _ := c.GetUserPlatformQuotaCache(ctx, 1, "openai")
	if got.DailyUsageUSD != 2 {
		t.Errorf("old schema entry should not be incremented, got %v", got.DailyUsageUSD)
	}
}

func TestUserPlatformQuotaCache_Delete(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	ctx := context.Background()
	_ = c.SetUserPlatformQuotaCache(ctx, 1, "openai", &service.UserPlatformQuotaCacheEntry{Version: 1}, time.Minute)
	if err := c.DeleteUserPlatformQuotaCache(ctx, 1, "openai"); err != nil {
		t.Fatal(err)
	}
	_, ok, _ := c.GetUserPlatformQuotaCache(ctx, 1, "openai")
	if ok {
		t.Error("expected miss after delete")
	}
}

func TestUserPlatformQuotaCache_BatchDelete(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	ctx := context.Background()
	for _, key := range []service.UserPlatformQuotaKey{{UserID: 1, Platform: "openai"}, {UserID: 2, Platform: "anthropic"}} {
		if err := c.SetUserPlatformQuotaCache(ctx, key.UserID, key.Platform, &service.UserPlatformQuotaCacheEntry{SchemaVersion: service.UserPlatformQuotaCacheSchemaV2}, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.DeleteUserPlatformQuotaCaches(ctx, []service.UserPlatformQuotaKey{{UserID: 1, Platform: "openai"}, {UserID: 2, Platform: "anthropic"}}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []service.UserPlatformQuotaKey{{UserID: 1, Platform: "openai"}, {UserID: 2, Platform: "anthropic"}} {
		if _, ok, _ := c.GetUserPlatformQuotaCache(ctx, key.UserID, key.Platform); ok {
			t.Errorf("cache key should be deleted: %+v", key)
		}
	}
}
