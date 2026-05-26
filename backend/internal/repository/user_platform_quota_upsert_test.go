//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/userplatformquota"
	"github.com/stretchr/testify/require"
)

func TestUpsertForUser_NewUserInsertsAllRecords(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	userID := mustCreateUserForQuota(t, client)
	repo := NewUserPlatformQuotaRepository(client)

	daily := 10.0
	weekly := 50.0
	monthly := 200.0
	records := []UserPlatformQuotaRecord{
		{UserID: userID, Platform: "anthropic", DailyLimitUSD: &daily, WeeklyLimitUSD: &weekly, MonthlyLimitUSD: &monthlyREDACTED,
		{UserID: userID, Platform: "openai", DailyLimitUSD: &dailyREDACTED,
REDACTED
	require.NoError(t, repo.UpsertForUser(ctx, userID, records))

	got, err := repo.ListByUser(ctx, userID)
REDACTED
	require.Len(t, got, 2)
REDACTED

func TestUpsertForUser_PartialUpdateSoftDeletesMissingPlatforms(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	userID := mustCreateUserForQuota(t, client)
	repo := NewUserPlatformQuotaRepository(client)

	d1 := 10.0
	d2 := 20.0
	require.NoError(t, repo.UpsertForUser(ctx, userID, []UserPlatformQuotaRecord{
		{UserID: userID, Platform: "anthropic", DailyLimitUSD: &d1REDACTED,
		{UserID: userID, Platform: "openai", DailyLimitUSD: &d1REDACTED,
REDACTED))
	require.NoError(t, repo.UpsertForUser(ctx, userID, []UserPlatformQuotaRecord{
		{UserID: userID, Platform: "anthropic", DailyLimitUSD: &d2REDACTED,
		{UserID: userID, Platform: "gemini", DailyLimitUSD: &d1REDACTED,
REDACTED))

	active, err := repo.ListByUser(ctx, userID)
REDACTED
	platforms := map[string]float64{REDACTED
	for _, r := range active {
		require.NotNil(t, r.DailyLimitUSD)
		platforms[r.Platform] = *r.DailyLimitUSD
REDACTED
	require.Len(t, platforms, 2)
	require.InDelta(t, 20.0, platforms["anthropic"], 1e-9)
	require.InDelta(t, 10.0, platforms["gemini"], 1e-9)
	_, openaiActive := platforms["openai"]
	require.False(t, openaiActive, "openai should be soft-deleted")
REDACTED

func TestUpsertForUser_PreservesUsageAndWindowStart(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	userID := mustCreateUserForQuota(t, client)
	repo := NewUserPlatformQuotaRepository(client)

	d := 10.0
	require.NoError(t, repo.UpsertForUser(ctx, userID, []UserPlatformQuotaRecord{
		{UserID: userID, Platform: "anthropic", DailyLimitUSD: &dREDACTED,
REDACTED))

	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	require.NoError(t, repo.IncrementUsageWithReset(ctx, userID, "anthropic", 3.5, now))

	newD := 50.0
	require.NoError(t, repo.UpsertForUser(ctx, userID, []UserPlatformQuotaRecord{
		{UserID: userID, Platform: "anthropic", DailyLimitUSD: &newDREDACTED,
REDACTED))

	rec, err := repo.GetByUserPlatform(ctx, userID, "anthropic")
REDACTED
	require.NotNil(t, rec)
	require.InDelta(t, 50.0, *rec.DailyLimitUSD, 1e-9, "limit should update")
	require.InDelta(t, 3.5, rec.DailyUsageUSD, 1e-9, "usage must be preserved")
	require.NotNil(t, rec.DailyWindowStart, "window_start must be preserved")
REDACTED

func TestUpsertForUser_ReactivatesSoftDeleted(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	userID := mustCreateUserForQuota(t, client)
	repo := NewUserPlatformQuotaRepository(client)

	d := 10.0
	require.NoError(t, repo.UpsertForUser(ctx, userID, []UserPlatformQuotaRecord{
		{UserID: userID, Platform: "anthropic", DailyLimitUSD: &dREDACTED,
REDACTED))
	require.NoError(t, repo.UpsertForUser(ctx, userID, []UserPlatformQuotaRecord{REDACTED))

	gone, err := repo.GetByUserPlatform(ctx, userID, "anthropic")
REDACTED
	require.Nil(t, gone, "anthropic should be soft-deleted (not active)")

	d2 := 20.0
	require.NoError(t, repo.UpsertForUser(ctx, userID, []UserPlatformQuotaRecord{
		{UserID: userID, Platform: "anthropic", DailyLimitUSD: &d2REDACTED,
REDACTED))

	back, err := repo.GetByUserPlatform(ctx, userID, "anthropic")
REDACTED
	require.NotNil(t, back, "anthropic should be active again")
	require.InDelta(t, 20.0, *back.DailyLimitUSD, 1e-9)

	allRows, err := client.UserPlatformQuota.Query().
		Where(userplatformquota.UserIDEQ(userID), userplatformquota.PlatformEQ("anthropic")).
		All(ctx)
REDACTED
	activeCount := 0
	for _, r := range allRows {
		if r.DeletedAt == nil {
			activeCount++
	REDACTED
REDACTED
	require.Equal(t, 1, activeCount, "should have exactly one active row")
REDACTED

func TestUpsertForUser_EmptyClearsAll(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	userID := mustCreateUserForQuota(t, client)
	repo := NewUserPlatformQuotaRepository(client)

	d := 10.0
	require.NoError(t, repo.UpsertForUser(ctx, userID, []UserPlatformQuotaRecord{
		{UserID: userID, Platform: "anthropic", DailyLimitUSD: &dREDACTED,
		{UserID: userID, Platform: "openai", DailyLimitUSD: &dREDACTED,
REDACTED))

	require.NoError(t, repo.UpsertForUser(ctx, userID, []UserPlatformQuotaRecord{REDACTED))

	got, err := repo.ListByUser(ctx, userID)
REDACTED
	require.Empty(t, got)
REDACTED
