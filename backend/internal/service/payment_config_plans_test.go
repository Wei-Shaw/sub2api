package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newPaymentConfigPlansTestService(t *testing.T) *PaymentConfigService {
	t.Helper()

	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return NewPaymentConfigService(client, nil, nil)
}

func TestCreatePlanRejectsStandardGroup(t *testing.T) {
	t.Parallel()

	svc := newPaymentConfigPlansTestService(t)
	ctx := context.Background()

	standardGroup, err := svc.entClient.Group.Create().
		SetName("Standard Group").
		SetPlatform(PlatformOpenAI).
		SetDescription("standard group").
		SetRateMultiplier(1).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeStandard).
		Save(ctx)
	require.NoError(t, err)

	_, err = svc.CreatePlan(ctx, CreatePlanRequest{
		GroupID:      standardGroup.ID,
		Name:         "Invalid Plan",
		Description:  "should fail",
		Price:        19.9,
		ValidityDays: 30,
		ValidityUnit: "days",
		Features:     "Feature A",
		ProductName:  "Invalid Product",
		ForSale:      true,
		SortOrder:    1,
	})
	require.ErrorIs(t, err, ErrGroupNotSubscriptionType)
}

func TestUpdatePlanRejectsChangingToStandardGroup(t *testing.T) {
	t.Parallel()

	svc := newPaymentConfigPlansTestService(t)
	ctx := context.Background()

	subscriptionGroup, err := svc.entClient.Group.Create().
		SetName("Subscription Group").
		SetPlatform(PlatformOpenAI).
		SetDescription("subscription group").
		SetRateMultiplier(1).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	standardGroup, err := svc.entClient.Group.Create().
		SetName("Standard Group").
		SetPlatform(PlatformOpenAI).
		SetDescription("standard group").
		SetRateMultiplier(1).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeStandard).
		Save(ctx)
	require.NoError(t, err)

	plan, err := svc.entClient.SubscriptionPlan.Create().
		SetGroupID(subscriptionGroup.ID).
		SetName("Valid Plan").
		SetDescription("valid plan").
		SetPrice(29.9).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetFeatures("Feature A").
		SetProductName("Valid Product").
		SetForSale(true).
		SetSortOrder(1).
		Save(ctx)
	require.NoError(t, err)

	_, err = svc.UpdatePlan(ctx, int64(plan.ID), UpdatePlanRequest{
		GroupID: &standardGroup.ID,
	})
	require.ErrorIs(t, err, ErrGroupNotSubscriptionType)
}

func TestCreatePlanAllowsDialogOnlyExternalSubscribeTarget(t *testing.T) {
	t.Parallel()

	svc := newPaymentConfigPlansTestService(t)
	ctx := context.Background()

	subscriptionGroup, err := svc.entClient.Group.Create().
		SetName("Subscription Group").
		SetPlatform(PlatformOpenAI).
		SetDescription("subscription group").
		SetRateMultiplier(1).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	plan, err := svc.CreatePlan(ctx, CreatePlanRequest{
		GroupID:                     subscriptionGroup.ID,
		Name:                        "Dialog Plan",
		Description:                 "dialog action",
		Price:                       29.9,
		ValidityDays:                30,
		ValidityUnit:                "days",
		Features:                    "Feature A",
		ProductName:                 "Dialog Product",
		ExternalSubscribeEnabled:    true,
		ExternalSubscribeDialogText: "联系客服开通",
		ForSale:                     true,
		SortOrder:                   1,
	})
	require.NoError(t, err)
	require.Equal(t, "联系客服开通", plan.ExternalSubscribeDialogText)
	require.Equal(t, "", plan.ExternalSubscribeURL)
}

func TestCreatePlanRejectsMissingExternalSubscribeTarget(t *testing.T) {
	t.Parallel()

	svc := newPaymentConfigPlansTestService(t)
	ctx := context.Background()

	subscriptionGroup, err := svc.entClient.Group.Create().
		SetName("Subscription Group").
		SetPlatform(PlatformOpenAI).
		SetDescription("subscription group").
		SetRateMultiplier(1).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	_, err = svc.CreatePlan(ctx, CreatePlanRequest{
		GroupID:                  subscriptionGroup.ID,
		Name:                     "Invalid Dialog Plan",
		Description:              "missing target",
		Price:                    19.9,
		ValidityDays:             30,
		ValidityUnit:             "days",
		Features:                 "Feature A",
		ProductName:              "Invalid Product",
		ExternalSubscribeEnabled: true,
		ForSale:                  true,
		SortOrder:                1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "PLAN_EXTERNAL_SUBSCRIBE_TARGET_REQUIRED")
}

func TestCreatePlanRejectsBothExternalSubscribeTargets(t *testing.T) {
	t.Parallel()

	svc := newPaymentConfigPlansTestService(t)
	ctx := context.Background()

	subscriptionGroup, err := svc.entClient.Group.Create().
		SetName("Subscription Group").
		SetPlatform(PlatformOpenAI).
		SetDescription("subscription group").
		SetRateMultiplier(1).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	_, err = svc.CreatePlan(ctx, CreatePlanRequest{
		GroupID:                     subscriptionGroup.ID,
		Name:                        "Conflict Dialog Plan",
		Description:                 "conflicting targets",
		Price:                       19.9,
		ValidityDays:                30,
		ValidityUnit:                "days",
		Features:                    "Feature A",
		ProductName:                 "Conflict Product",
		ExternalSubscribeEnabled:    true,
		ExternalSubscribeURL:        "https://example.com/subscribe",
		ExternalSubscribeDialogText: "联系客服开通",
		ForSale:                     true,
		SortOrder:                   1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "PLAN_EXTERNAL_SUBSCRIBE_TARGET_CONFLICT")
}
