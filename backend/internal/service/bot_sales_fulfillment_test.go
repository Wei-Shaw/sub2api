package service_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testsupport"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestBotSalesFulfillmentAssignsSubscriptionFromPlanGroupWithoutTargetGroup(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-subscription", service.SubscriptionTypeSubscription)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Bot monthly").
		SetPrice(9.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	resp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:   "bs-order-1001",
		ExternalPaymentID: "bs-pay-1001",
		Operation:         service.BotSalesFulfillmentOperationNew,
		EntitlementKind:   service.BotSalesEntitlementSubscription,
		PlanID:            plan.ID,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "telegram:123456",
			Email:          "bot-user-1001@example.test",
			DisplayName:    "Bot User 1001",
		},
		Affiliate:      &service.BotSalesFulfillmentAffiliate{AffCode: "AFFBOT01"},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyAlways},
	})
	require.NoError(t, err)
	require.Equal(t, service.BotSalesEntitlementSubscription, resp.EntitlementKind)
	require.NotZero(t, resp.Buyer.UserID)
	require.NotZero(t, resp.Subscription.ID)
	require.Equal(t, group.ID, resp.Subscription.GroupID)
	require.NotNil(t, resp.Delivery.APIKey)
	require.NotEmpty(t, resp.Delivery.APIKey.Key)
	require.Equal(t, group.ID, *resp.Delivery.APIKey.GroupID)

	sub := client.UserSubscription.Query().OnlyX(ctx)
	require.Equal(t, resp.Buyer.UserID, sub.UserID)
	require.Equal(t, group.ID, sub.GroupID)

	apiKey := client.APIKey.Query().OnlyX(ctx)
	require.Equal(t, resp.Buyer.UserID, apiKey.UserID)
	require.NotNil(t, apiKey.GroupID)
	require.Equal(t, group.ID, *apiKey.GroupID)
}

func TestBotSalesFulfillmentAllowsMissingAffiliateAndCreditsBalancePackageGroup(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-balance", service.SubscriptionTypeNone)
	pkg := client.BalancePackage.Create().
		SetCode("bot_100k").
		SetLabel("Bot 100k").
		SetAmountLedger(25).
		SetActualCredits(100000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	resp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-1002",
		Operation:          service.BotSalesFulfillmentOperationNew,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: pkg.Code,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "telegram:654321",
			Email:          "bot-user-1002@example.test",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyAlways},
	})
	require.NoError(t, err)
	require.Equal(t, service.BotSalesEntitlementBalance, resp.EntitlementKind)
	require.Equal(t, group.ID, resp.Balance.GroupID)
	require.Equal(t, pkg.AmountLedger, resp.Balance.AmountLedger)
	require.Equal(t, pkg.ActualCredits, resp.Balance.ActualCredits)
	require.NotNil(t, resp.Delivery.APIKey)
	require.Equal(t, group.ID, *resp.Delivery.APIKey.GroupID)

	user := client.User.GetX(ctx, resp.Buyer.UserID)
	require.Equal(t, pkg.AmountLedger, user.Balance)
}

func TestBotSalesFulfillmentBalanceNewIssuesDeviceCodeAndTopupCreditsExistingDeviceUser(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-balance-topup", service.SubscriptionTypeNone)
	pkg := client.BalancePackage.Create().
		SetCode("standard_20").
		SetLabel("Standard 20").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	newResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-new-device",
		Operation:          service.BotSalesFulfillmentOperationNew,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: pkg.Code,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "telegram:device-owner",
			Email:          "bot-device-owner@example.test",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyAlways, IssueDeviceCode: true},
	})
	require.NoError(t, err)
	require.Regexp(t, `^DLG-[A-Z2-9]{4}-[A-Z2-9]{4}-[A-Z2-9]{4}$`, newResp.Delivery.DeviceCode)
	require.Equal(t, newResp.Delivery.DeviceCode, newResp.DeviceCode)

	device := client.UserDevice.Query().OnlyX(ctx)
	require.NotNil(t, device.DeviceCode)
	require.Equal(t, newResp.Delivery.DeviceCode, *device.DeviceCode)
	require.Equal(t, newResp.Buyer.UserID, device.UserID)
	require.Equal(t, float64(20), client.User.GetX(ctx, newResp.Buyer.UserID).Balance)

	topupResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-topup-device",
		Operation:          service.BotSalesFulfillmentOperationTopup,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: pkg.Code,
		DeviceCode:         strings.ToLower(newResp.Delivery.DeviceCode),
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "telegram:topup-payer",
			Email:          "bot-topup-payer@example.test",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyIfMissing},
	})
	require.NoError(t, err)
	require.Equal(t, service.BotSalesFulfillmentOperationTopup, topupResp.Operation)
	require.Equal(t, newResp.Delivery.DeviceCode, topupResp.Delivery.DeviceCode)
	require.Equal(t, newResp.Delivery.DeviceCode, topupResp.DeviceCode)
	require.Equal(t, newResp.Buyer.UserID, topupResp.Buyer.UserID)
	require.Equal(t, float64(40), client.User.GetX(ctx, newResp.Buyer.UserID).Balance)
}

func TestBotSalesFulfillmentDoesNotAcceptTargetGroupInput(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	svc := newBotSalesFulfillmentServiceForTest(client, db)

	_, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID: "bs-order-bad-group",
		Operation:       service.BotSalesFulfillmentOperationNew,
		EntitlementKind: service.BotSalesEntitlementSubscription,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "telegram:no-group",
			Email:          "bot-user-bad-group@example.test",
		},
		RawPayload: map[string]any{"target_group_id": float64(999)},
	})
	require.Error(t, err)
	require.Equal(t, "UNSUPPORTED_FIELD", infraerrorsReason(err))
}

func TestBotSalesFulfillmentMissingAffiliateIsNotRejected(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-no-affiliate", service.SubscriptionTypeSubscription)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Bot no affiliate").
		SetPrice(5).
		SetValidityDays(7).
		SetValidityUnit("day").
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	_, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID: "bs-order-no-affiliate",
		Operation:       service.BotSalesFulfillmentOperationNew,
		EntitlementKind: service.BotSalesEntitlementSubscription,
		PlanID:          plan.ID,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "telegram:no-affiliate",
			Email:          "bot-user-no-affiliate@example.test",
		},
	})
	require.NoError(t, err)
}

func newBotSalesFulfillmentServiceForTest(client *dbent.Client, db *sql.DB) *service.BotSalesFulfillmentService {
	return testsupport.NewBotSalesFulfillmentService(client, db)
}

func newBotSalesFulfillmentEntClient(t *testing.T) (*dbent.Client, *sql.DB) {
	t.Helper()
	dbName := "file:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared&_fk=1"
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client, db
}

func createBotSalesGroup(t *testing.T, client *dbent.Client, name string, subscriptionType string) *dbent.Group {
	t.Helper()
	return client.Group.Create().
		SetName(name).
		SetPlatform("claude").
		SetStatus(service.StatusActive).
		SetSubscriptionType(subscriptionType).
		SetRateMultiplier(1).
		SaveX(context.Background())
}

func infraerrorsReason(err error) string {
	if err == nil {
		return ""
	}
	return infraerrors.Reason(err)
}
