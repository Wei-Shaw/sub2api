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
REDACTED
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()REDACTED)
	return &billingCache{rdb: rdbREDACTED, mr
REDACTED

func TestUserPlatformQuotaCache_GetMissReturnsNotFound(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	entry, ok, err := c.GetUserPlatformQuotaCache(context.Background(), 1, "anthropic")
	if err != nil {
		t.Fatal(err)
REDACTED
	if ok || entry != nil {
		t.Errorf("expected miss, got ok=%v entry=%v", ok, entry)
REDACTED
REDACTED

func TestUserPlatformQuotaCache_SetThenGet(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	ctx := context.Background()
	dailyLimit := 20.0
	ts := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	in := &service.UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    1.5,
		WeeklyUsageUSD:   3.0,
		MonthlyUsageUSD:  10.0,
		Version:          7,
		SchemaVersion:    service.UserPlatformQuotaCacheSchemaV1,
		DailyLimitUSD:    &dailyLimit,
		DailyWindowStart: &ts,
REDACTED
	if err := c.SetUserPlatformQuotaCache(ctx, 1, "openai", in, time.Minute); err != nil {
		t.Fatal(err)
REDACTED
	got, ok, err := c.GetUserPlatformQuotaCache(ctx, 1, "openai")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
REDACTED
	if got.DailyUsageUSD != 1.5 || got.WeeklyUsageUSD != 3.0 || got.MonthlyUsageUSD != 10.0 || got.Version != 7 {
		t.Errorf("got = %+v, want %+v", got, in)
REDACTED
	if got.SchemaVersion != service.UserPlatformQuotaCacheSchemaV1 {
		t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, service.UserPlatformQuotaCacheSchemaV1)
REDACTED
	if got.DailyLimitUSD == nil || *got.DailyLimitUSD != dailyLimit {
		t.Errorf("DailyLimitUSD = %v, want %v", got.DailyLimitUSD, dailyLimit)
REDACTED
	if got.DailyWindowStart == nil || !got.DailyWindowStart.Equal(ts) {
		t.Errorf("DailyWindowStart = %v, want %v", got.DailyWindowStart, ts)
REDACTED
REDACTED

func TestUserPlatformQuotaCache_NilLimitSetThenGet(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	ctx := context.Background()
	in := &service.UserPlatformQuotaCacheEntry{
		DailyUsageUSD: 1.0,
		SchemaVersion: service.UserPlatformQuotaCacheSchemaV1,
		// DailyLimitUSD nil → 无限额
REDACTED
	if err := c.SetUserPlatformQuotaCache(ctx, 1, "openai", in, time.Minute); err != nil {
		t.Fatal(err)
REDACTED
	got, ok, err := c.GetUserPlatformQuotaCache(ctx, 1, "openai")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
REDACTED
	if got.DailyLimitUSD != nil {
		t.Errorf("DailyLimitUSD should be nil for unlimited, got %v", got.DailyLimitUSD)
REDACTED
REDACTED

func TestUserPlatformQuotaCache_IncrMissIsNoop(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	if err := c.IncrUserPlatformQuotaUsageCache(context.Background(), 1, "openai", 0.5, time.Minute, false); err != nil {
		t.Fatal(err)
REDACTED
	_, ok, _ := c.GetUserPlatformQuotaCache(context.Background(), 1, "openai")
	if ok {
		t.Error("expected key absent after no-op incr")
REDACTED
REDACTED

func TestUserPlatformQuotaCache_IncrHitAccumulates(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	ctx := context.Background()
	// SchemaVersion 必须显式设为 V1,否则 Lua 脚本会因 schema 不匹配而 return 0,跳过累加。
	_ = c.SetUserPlatformQuotaCache(ctx, 1, "openai", &service.UserPlatformQuotaCacheEntry{
		Version:       1,
		SchemaVersion: service.UserPlatformQuotaCacheSchemaV1,
REDACTED, time.Minute)
	if err := c.IncrUserPlatformQuotaUsageCache(ctx, 1, "openai", 0.5, time.Minute, false); err != nil {
		t.Fatal(err)
REDACTED
	if err := c.IncrUserPlatformQuotaUsageCache(ctx, 1, "openai", 0.25, time.Minute, false); err != nil {
		t.Fatal(err)
REDACTED
	got, _, _ := c.GetUserPlatformQuotaCache(ctx, 1, "openai")
	if got.DailyUsageUSD != 0.75 || got.WeeklyUsageUSD != 0.75 || got.MonthlyUsageUSD != 0.75 {
		t.Errorf("got %+v, want daily/weekly/monthly=0.75", got)
REDACTED
	if got.Version != 3 {
		t.Errorf("version = %d, want 3 (initial 1 + 2 incr)", got.Version)
REDACTED
REDACTED

func TestUserPlatformQuotaCache_Delete(t *testing.T) {
	c, _ := newMiniRedisCache(t)
	ctx := context.Background()
	_ = c.SetUserPlatformQuotaCache(ctx, 1, "openai", &service.UserPlatformQuotaCacheEntry{Version: 1REDACTED, time.Minute)
	if err := c.DeleteUserPlatformQuotaCache(ctx, 1, "openai"); err != nil {
		t.Fatal(err)
REDACTED
	_, ok, _ := c.GetUserPlatformQuotaCache(ctx, 1, "openai")
	if ok {
		t.Error("expected miss after delete")
REDACTED
REDACTED
