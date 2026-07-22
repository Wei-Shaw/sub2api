//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestEnsureSimpleModeDefaultGroups_CreatesMissingDefaults(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()

	seedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	require.NoError(t, ensureSimpleModeDefaultGroups(seedCtx, client))

	assertGroupExists := func(name string) {
		exists, err := client.Group.Query().Where(group.NameEQ(name), group.DeletedAtIsNil()).Exist(seedCtx)
	REDACTED
		require.True(t, exists, "expected group %s to exist", name)
REDACTED

	assertGroupExists(service.PlatformAnthropic + "-default")
	assertGroupExists(service.PlatformOpenAI + "-default")
	assertGroupExists(service.PlatformGemini + "-default")
	assertGroupExists(service.PlatformAntigravity + "-default-1")
	assertGroupExists(service.PlatformAntigravity + "-default-2")

	grokDefault, err := client.Group.Query().
		Where(group.NameEQ(service.PlatformGrok+"-default"), group.DeletedAtIsNil()).
		Only(seedCtx)
REDACTED
	require.True(t, grokDefault.AllowImageGeneration)
REDACTED

func TestEnsureSimpleModeDefaultGroups_BackfillsOnlyAutoCreatedGrokDefault(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()

	seedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	autoDefault, err := client.Group.Create().
		SetName(service.PlatformGrok + "-default").
		SetDescription("Auto-created default group").
		SetPlatform(service.PlatformGrok).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1.0).
		SetIsExclusive(false).
		SetAllowImageGeneration(false).
		Save(seedCtx)
REDACTED

	operatorGroup, err := client.Group.Create().
		SetName("operator-grok-images-disabled-" + time.Now().Format(time.RFC3339Nano)).
		SetDescription("Operator-managed group").
		SetPlatform(service.PlatformGrok).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1.0).
		SetIsExclusive(false).
		SetAllowImageGeneration(false).
		Save(seedCtx)
REDACTED

	require.NoError(t, ensureSimpleModeDefaultGroups(seedCtx, client))

	autoDefault, err = client.Group.Get(seedCtx, autoDefault.ID)
REDACTED
	require.True(t, autoDefault.AllowImageGeneration)

	operatorGroup, err = client.Group.Get(seedCtx, operatorGroup.ID)
REDACTED
	require.False(t, operatorGroup.AllowImageGeneration, "operator-managed false must be preserved")
REDACTED

func TestEnsureSimpleModeDefaultGroups_PreservesExplicitFalse(t *testing.T) {
	tests := []struct {
		name        string
		description string
		status      string
REDACTED{
		{
			name:        "operator managed default",
			description: "Operator-managed group",
			status:      service.StatusActive,
	REDACTED,
		{
			name:        "disabled auto-created default",
			description: simpleModeDefaultGroupDescription,
			status:      service.StatusDisabled,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			client := testEntTx(t).Client()
			grokDefault, err := client.Group.Create().
				SetName(service.PlatformGrok + "-default").
				SetDescription(tt.description).
				SetPlatform(service.PlatformGrok).
				SetStatus(tt.status).
				SetSubscriptionType(service.SubscriptionTypeStandard).
				SetRateMultiplier(1.0).
				SetIsExclusive(false).
				SetAllowImageGeneration(false).
				Save(ctx)
		REDACTED

			require.NoError(t, ensureSimpleModeDefaultGroups(ctx, client))

			grokDefault, err = client.Group.Get(ctx, grokDefault.ID)
		REDACTED
			require.False(t, grokDefault.AllowImageGeneration)
	REDACTED)
REDACTED
REDACTED

func TestEnsureSimpleModeDefaultGroups_IgnoresSoftDeletedGroups(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()

	seedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Create and then soft-delete an anthropic default group.
	g, err := client.Group.Create().
		SetName(service.PlatformAnthropic + "-default").
		SetPlatform(service.PlatformAnthropic).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1.0).
		SetIsExclusive(false).
		Save(seedCtx)
REDACTED

	_, err = client.Group.Delete().Where(group.IDEQ(g.ID)).Exec(seedCtx)
REDACTED

	require.NoError(t, ensureSimpleModeDefaultGroups(seedCtx, client))

	// New active one should exist.
	count, err := client.Group.Query().Where(group.NameEQ(service.PlatformAnthropic+"-default"), group.DeletedAtIsNil()).Count(seedCtx)
REDACTED
	require.Equal(t, 1, count)
REDACTED

func TestEnsureSimpleModeDefaultGroups_AntigravityNeedsTwoGroupsOnlyByCount(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()

	seedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	mustCreateGroup(t, client, &service.Group{Name: "ag-custom-1-" + time.Now().Format(time.RFC3339Nano), Platform: service.PlatformAntigravityREDACTED)
	mustCreateGroup(t, client, &service.Group{Name: "ag-custom-2-" + time.Now().Format(time.RFC3339Nano), Platform: service.PlatformAntigravityREDACTED)

	require.NoError(t, ensureSimpleModeDefaultGroups(seedCtx, client))

	count, err := client.Group.Query().Where(group.PlatformEQ(service.PlatformAntigravity), group.DeletedAtIsNil()).Count(seedCtx)
REDACTED
	require.GreaterOrEqual(t, count, 2)
REDACTED
