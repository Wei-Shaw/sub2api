package service_test

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/payment"
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
		ExternalPaymentID:  "bs-pay-1002",
		PaymentAmount:      100000,
		PaymentCurrency:    "VND",
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

	order := client.PaymentOrder.Query().Where(paymentorder.UserIDEQ(resp.Buyer.UserID)).OnlyX(ctx)
	require.Equal(t, service.OrderStatusCompleted, order.Status)
	require.Equal(t, payment.OrderTypeBalance, order.OrderType)
	require.Equal(t, service.BotSalesPaymentProvider, order.PaymentType)
	require.Equal(t, "VND", order.PaymentCurrency)
	require.Equal(t, float64(100000), order.PaymentAmount)
	require.Equal(t, pkg.AmountLedger, order.LedgerAmount)
	require.Equal(t, pkg.ActualCredits, *order.ActualCredits)
	require.Equal(t, group.ID, *order.BalanceGroupID)
	require.NotEmpty(t, order.OutTradeNo)
	require.NotNil(t, order.CompletedAt)
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
		ExternalPaymentID:  "bs-pay-new-device",
		PaymentAmount:      100000,
		PaymentCurrency:    "VND",
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
		ExternalPaymentID:  "bs-pay-topup-device",
		PaymentAmount:      100000,
		PaymentCurrency:    "VND",
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

func TestBotSalesFulfillmentIfMissingReusesExistingAPIKeyForTopup(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-balance-reuse-key", service.SubscriptionTypeNone)
	pkg := client.BalancePackage.Create().
		SetCode("standard_reuse_key").
		SetLabel("Standard Reuse Key").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	newResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-reuse-key-new",
		Operation:          service.BotSalesFulfillmentOperationNew,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: pkg.Code,
		ExternalPaymentID:  "bs-pay-reuse-key-new",
		PaymentAmount:      100000,
		PaymentCurrency:    "VND",
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:reuse-key-owner",
			Provider:       "telegram",
			ProviderUserID: "reuse-key-owner",
			TelegramID:     "reuse-key-owner",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyAlways, IssueDeviceCode: true},
	})
	require.NoError(t, err)
	require.NotNil(t, newResp.Delivery.APIKey)
	issuedKeyID := newResp.Delivery.APIKey.ID
	require.NotZero(t, issuedKeyID)
	require.Equal(t, 1, client.APIKey.Query().Where(apikey.UserIDEQ(newResp.Buyer.UserID)).CountX(ctx))

	topupResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-reuse-key-topup",
		Operation:          service.BotSalesFulfillmentOperationTopup,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: pkg.Code,
		ExternalPaymentID:  "bs-pay-reuse-key-topup",
		PaymentAmount:      100000,
		PaymentCurrency:    "VND",
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:reuse-key-owner",
			Provider:       "telegram",
			ProviderUserID: "reuse-key-owner",
			TelegramID:     "reuse-key-owner",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyIfMissing},
	})
	require.NoError(t, err)
	require.NotNil(t, topupResp.Delivery.APIKey)
	require.Equal(t, issuedKeyID, topupResp.Delivery.APIKey.ID)
	require.Equal(t, group.ID, *topupResp.Delivery.APIKey.GroupID)
	require.Equal(t, 1, client.APIKey.Query().Where(apikey.UserIDEQ(newResp.Buyer.UserID)).CountX(ctx))
	require.Equal(t, float64(40), client.User.GetX(ctx, newResp.Buyer.UserID).Balance)
}

func TestBotSalesFulfillmentIfMissingPreservesExistingAPIKeyForEquivalentTopupPlatform(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	oldGroup := createBotSalesGroupWithPlatform(t, client, "bot-balance-openai-a", service.PlatformOpenAI, service.SubscriptionTypeNone)
	newGroup := createBotSalesGroupWithPlatform(t, client, "bot-balance-openai-b", service.PlatformOpenAI, service.SubscriptionTypeNone)
	oldPkg := client.BalancePackage.Create().
		SetCode("standard_openai_a").
		SetLabel("Standard OpenAI A").
		SetAmountLedger(10).
		SetActualCredits(10000000).
		SetCreditUnit("tokens").
		SetGroupID(oldGroup.ID).
		SetForSale(true).
		SaveX(ctx)
	newPkg := client.BalancePackage.Create().
		SetCode("standard_openai_b").
		SetLabel("Standard OpenAI B").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(newGroup.ID).
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	firstResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-rebind-key-new",
		Operation:          service.BotSalesFulfillmentOperationNew,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: oldPkg.Code,
		ExternalPaymentID:  "bs-pay-rebind-key-new",
		PaymentAmount:      100000,
		PaymentCurrency:    "VND",
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:rebind-key-owner",
			Provider:       "telegram",
			ProviderUserID: "rebind-key-owner",
			TelegramID:     "rebind-key-owner",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyAlways},
	})
	require.NoError(t, err)
	require.NotNil(t, firstResp.Delivery.APIKey)
	issuedKeyID := firstResp.Delivery.APIKey.ID
	require.NotZero(t, issuedKeyID)
	require.Equal(t, oldGroup.ID, *firstResp.Delivery.APIKey.GroupID)

	topupResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-rebind-key-topup",
		Operation:          service.BotSalesFulfillmentOperationTopup,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: newPkg.Code,
		ExternalPaymentID:  "bs-pay-rebind-key-topup",
		PaymentAmount:      100000,
		PaymentCurrency:    "VND",
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:rebind-key-owner",
			Provider:       "telegram",
			ProviderUserID: "rebind-key-owner",
			TelegramID:     "rebind-key-owner",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyIfMissing},
	})
	require.NoError(t, err)
	require.NotNil(t, topupResp.Delivery.APIKey)
	require.Equal(t, issuedKeyID, topupResp.Delivery.APIKey.ID)
	require.Equal(t, oldGroup.ID, *topupResp.Delivery.APIKey.GroupID)
	require.Equal(t, 1, client.APIKey.Query().Where(apikey.UserIDEQ(firstResp.Buyer.UserID)).CountX(ctx))

	storedKey := client.APIKey.GetX(ctx, issuedKeyID)
	require.NotNil(t, storedKey.GroupID)
	require.Equal(t, oldGroup.ID, *storedKey.GroupID)
}

func TestBotSalesFulfillmentIfMissingRebindsExistingAPIKeyWhenTargetPlatformDiffers(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	oldGroup := createBotSalesGroupWithPlatform(t, client, "bot-balance-codex", service.PlatformOpenAI, service.SubscriptionTypeNone)
	newGroup := createBotSalesGroupWithPlatform(t, client, "bot-balance-claude", service.PlatformAnthropic, service.SubscriptionTypeNone)
	oldPkg := client.BalancePackage.Create().
		SetCode("standard_codex").
		SetLabel("Standard Codex").
		SetAmountLedger(10).
		SetActualCredits(10000000).
		SetCreditUnit("tokens").
		SetGroupID(oldGroup.ID).
		SetForSale(true).
		SaveX(ctx)
	newPkg := client.BalancePackage.Create().
		SetCode("standard_claude").
		SetLabel("Standard Claude").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(newGroup.ID).
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	firstResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-cross-platform-key-new",
		Operation:          service.BotSalesFulfillmentOperationNew,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: oldPkg.Code,
		ExternalPaymentID:  "bs-pay-cross-platform-key-new",
		PaymentAmount:      100000,
		PaymentCurrency:    "VND",
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:cross-platform-key-owner",
			Provider:       "telegram",
			ProviderUserID: "cross-platform-key-owner",
			TelegramID:     "cross-platform-key-owner",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyAlways},
	})
	require.NoError(t, err)
	require.NotNil(t, firstResp.Delivery.APIKey)
	issuedKeyID := firstResp.Delivery.APIKey.ID
	require.NotZero(t, issuedKeyID)
	require.Equal(t, oldGroup.ID, *firstResp.Delivery.APIKey.GroupID)

	topupResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-cross-platform-key-topup",
		Operation:          service.BotSalesFulfillmentOperationTopup,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: newPkg.Code,
		ExternalPaymentID:  "bs-pay-cross-platform-key-topup",
		PaymentAmount:      100000,
		PaymentCurrency:    "VND",
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:cross-platform-key-owner",
			Provider:       "telegram",
			ProviderUserID: "cross-platform-key-owner",
			TelegramID:     "cross-platform-key-owner",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyIfMissing},
	})
	require.NoError(t, err)
	require.NotNil(t, topupResp.Delivery.APIKey)
	require.Equal(t, issuedKeyID, topupResp.Delivery.APIKey.ID)
	require.Equal(t, newGroup.ID, *topupResp.Delivery.APIKey.GroupID)
	require.Equal(t, 1, client.APIKey.Query().Where(apikey.UserIDEQ(firstResp.Buyer.UserID)).CountX(ctx))

	storedKey := client.APIKey.GetX(ctx, issuedKeyID)
	require.NotNil(t, storedKey.GroupID)
	require.Equal(t, newGroup.ID, *storedKey.GroupID)
}

func TestBotSalesFulfillmentSubscriptionIfMissingPreservesExistingAPIKeyForEquivalentPlatform(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	oldGroup := createBotSalesGroupWithPlatform(t, client, "bot-sub-openai-a", service.PlatformOpenAI, service.SubscriptionTypeSubscription)
	newGroup := createBotSalesGroupWithPlatform(t, client, "bot-sub-openai-b", service.PlatformOpenAI, service.SubscriptionTypeSubscription)
	oldPlan := client.SubscriptionPlan.Create().
		SetGroupID(oldGroup.ID).
		SetName("Bot OpenAI A monthly").
		SetPrice(9.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		SaveX(ctx)
	newPlan := client.SubscriptionPlan.Create().
		SetGroupID(newGroup.ID).
		SetName("Bot OpenAI B monthly").
		SetPrice(9.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	firstResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:   "bs-order-sub-openai-a",
		ExternalPaymentID: "bs-pay-sub-openai-a",
		Operation:         service.BotSalesFulfillmentOperationNew,
		EntitlementKind:   service.BotSalesEntitlementSubscription,
		PlanID:            oldPlan.ID,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:sub-platform-owner",
			Provider:       "telegram",
			ProviderUserID: "sub-platform-owner",
			TelegramID:     "sub-platform-owner",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyAlways},
	})
	require.NoError(t, err)
	require.NotNil(t, firstResp.Delivery.APIKey)
	issuedKeyID := firstResp.Delivery.APIKey.ID
	require.NotZero(t, issuedKeyID)
	require.Equal(t, oldGroup.ID, *firstResp.Delivery.APIKey.GroupID)

	secondResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:   "bs-order-sub-openai-b",
		ExternalPaymentID: "bs-pay-sub-openai-b",
		Operation:         service.BotSalesFulfillmentOperationNew,
		EntitlementKind:   service.BotSalesEntitlementSubscription,
		PlanID:            newPlan.ID,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:sub-platform-owner",
			Provider:       "telegram",
			ProviderUserID: "sub-platform-owner",
			TelegramID:     "sub-platform-owner",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyIfMissing},
	})
	require.NoError(t, err)
	require.Equal(t, service.BotSalesEntitlementSubscription, secondResp.EntitlementKind)
	require.Equal(t, newGroup.ID, secondResp.Subscription.GroupID)
	require.NotNil(t, secondResp.Delivery.APIKey)
	require.Equal(t, issuedKeyID, secondResp.Delivery.APIKey.ID)
	require.Equal(t, oldGroup.ID, *secondResp.Delivery.APIKey.GroupID)
	require.Equal(t, 1, client.APIKey.Query().Where(apikey.UserIDEQ(firstResp.Buyer.UserID)).CountX(ctx))

	storedKey := client.APIKey.GetX(ctx, issuedKeyID)
	require.NotNil(t, storedKey.GroupID)
	require.Equal(t, oldGroup.ID, *storedKey.GroupID)
}

func TestBotSalesFulfillmentSubscriptionIfMissingRebindsExistingAPIKeyWhenCapabilityMissing(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	oldGroup := createBotSalesGroupWithPlatform(t, client, "bot-sub-openai-basic", service.PlatformOpenAI, service.SubscriptionTypeSubscription)
	newGroup := createBotSalesGroupWithPlatform(t, client, "bot-sub-openai-messages", service.PlatformOpenAI, service.SubscriptionTypeSubscription)
	_, err := client.Group.UpdateOneID(newGroup.ID).SetAllowMessagesDispatch(true).Save(ctx)
	require.NoError(t, err)
	oldPlan := client.SubscriptionPlan.Create().
		SetGroupID(oldGroup.ID).
		SetName("Bot OpenAI basic monthly").
		SetPrice(9.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		SaveX(ctx)
	newPlan := client.SubscriptionPlan.Create().
		SetGroupID(newGroup.ID).
		SetName("Bot OpenAI messages monthly").
		SetPrice(9.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	firstResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:   "bs-order-sub-openai-basic",
		ExternalPaymentID: "bs-pay-sub-openai-basic",
		Operation:         service.BotSalesFulfillmentOperationNew,
		EntitlementKind:   service.BotSalesEntitlementSubscription,
		PlanID:            oldPlan.ID,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:sub-capability-owner",
			Provider:       "telegram",
			ProviderUserID: "sub-capability-owner",
			TelegramID:     "sub-capability-owner",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyAlways},
	})
	require.NoError(t, err)
	require.NotNil(t, firstResp.Delivery.APIKey)
	issuedKeyID := firstResp.Delivery.APIKey.ID
	require.NotZero(t, issuedKeyID)
	require.Equal(t, oldGroup.ID, *firstResp.Delivery.APIKey.GroupID)

	secondResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:   "bs-order-sub-openai-messages",
		ExternalPaymentID: "bs-pay-sub-openai-messages",
		Operation:         service.BotSalesFulfillmentOperationNew,
		EntitlementKind:   service.BotSalesEntitlementSubscription,
		PlanID:            newPlan.ID,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:sub-capability-owner",
			Provider:       "telegram",
			ProviderUserID: "sub-capability-owner",
			TelegramID:     "sub-capability-owner",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyIfMissing},
	})
	require.NoError(t, err)
	require.Equal(t, service.BotSalesEntitlementSubscription, secondResp.EntitlementKind)
	require.Equal(t, newGroup.ID, secondResp.Subscription.GroupID)
	require.NotNil(t, secondResp.Delivery.APIKey)
	require.Equal(t, issuedKeyID, secondResp.Delivery.APIKey.ID)
	require.Equal(t, newGroup.ID, *secondResp.Delivery.APIKey.GroupID)
	require.Equal(t, 1, client.APIKey.Query().Where(apikey.UserIDEQ(firstResp.Buyer.UserID)).CountX(ctx))

	storedKey := client.APIKey.GetX(ctx, issuedKeyID)
	require.NotNil(t, storedKey.GroupID)
	require.Equal(t, newGroup.ID, *storedKey.GroupID)
}

func TestBotSalesFulfillmentBalanceTopupWithoutDeviceCodeCreditsCanonicalBuyerAcrossProviders(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-balance-topup-buyer", service.SubscriptionTypeNone)
	pkg := client.BalancePackage.Create().
		SetCode("standard_20_buyer").
		SetLabel("Standard 20 buyer").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	cases := []struct {
		name           string
		externalUserID string
		provider       string
		providerUserID string
		telegramID     string
	}{
		{name: "telegram", externalUserID: "channel:telegram:user:123456789", provider: "telegram", providerUserID: "123456789", telegramID: "123456789"},
		{name: "zalo", externalUserID: "channel:zalo:user:zalo-user-42", provider: "zalo", providerUserID: "zalo-user-42"},
		{name: "kakao", externalUserID: "channel:kakao:user:kakao-user-42", provider: "kakao", providerUserID: "kakao-user-42"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
				ExternalOrderID:    "bs-order-" + tc.name + "-new",
				Operation:          service.BotSalesFulfillmentOperationNew,
				EntitlementKind:    service.BotSalesEntitlementBalance,
				BalancePackageCode: pkg.Code,
				ExternalPaymentID:  "pay-" + tc.name + "-new",
				PaymentAmount:      100000,
				PaymentCurrency:    "VND",
				Buyer: service.BotSalesFulfillmentBuyer{
					ExternalUserID: tc.externalUserID,
					Provider:       tc.provider,
					ProviderUserID: tc.providerUserID,
					TelegramID:     tc.telegramID,
				},
				DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyAlways, IssueDeviceCode: true},
			})
			require.NoError(t, err)
			require.Equal(t, float64(20), client.User.GetX(ctx, newResp.Buyer.UserID).Balance)
			require.NotEmpty(t, newResp.Delivery.DeviceCode)

			topupResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
				ExternalOrderID:    "bs-order-" + tc.name + "-topup",
				Operation:          service.BotSalesFulfillmentOperationTopup,
				EntitlementKind:    service.BotSalesEntitlementBalance,
				BalancePackageCode: pkg.Code,
				ExternalPaymentID:  "pay-" + tc.name + "-topup",
				PaymentAmount:      100000,
				PaymentCurrency:    "VND",
				Buyer: service.BotSalesFulfillmentBuyer{
					ExternalUserID: tc.externalUserID,
					Provider:       tc.provider,
					ProviderUserID: tc.providerUserID,
					TelegramID:     tc.telegramID,
				},
				DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyIfMissing},
			})
			require.NoError(t, err)
			require.Equal(t, service.BotSalesFulfillmentOperationTopup, topupResp.Operation)
			require.Equal(t, newResp.Buyer.UserID, topupResp.Buyer.UserID)
			require.Equal(t, tc.externalUserID, topupResp.Buyer.ExternalUserID)
			require.Empty(t, topupResp.Delivery.DeviceCode)
			require.Equal(t, float64(40), client.User.GetX(ctx, newResp.Buyer.UserID).Balance)
		})
	}
}

func TestBotSalesFulfillmentCreditTopupCreditsDeviceOwnerWithoutPackageOrAPIKey(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	ownerGroup := createBotSalesGroup(t, client, "credit-topup-owner-group", service.SubscriptionTypeNone)
	owner := client.User.Create().
		SetEmail("credit-device-owner@example.test").
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetConcurrency(1).
		SaveX(ctx)
	deviceCode := "DLG-CRDT-TOPU-TEST"
	client.UserDevice.Create().
		SetUserID(owner.ID).
		SetDeviceCode(deviceCode).
		SetDeviceHash(strings.Repeat("a", 64)).
		SetPlatform("bot-sales").
		SetArch("api").
		SetStatus(service.UserDeviceStatusActive).
		SaveX(ctx)
	existingKey := client.APIKey.Create().
		SetUserID(owner.ID).
		SetKey("sk-existing-credit-topup-owner-key").
		SetName("existing-credit-topup-owner-key").
		SetGroupID(ownerGroup.ID).
		SetStatus(service.StatusAPIKeyActive).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	resp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:     "bs-order-credit-topup-device",
		ExternalOrderItemID: "line-credit-1",
		ExternalPaymentID:   "bs-pay-credit-topup-device",
		Operation:           service.BotSalesFulfillmentOperationTopup,
		EntitlementKind:     service.BotSalesEntitlementCreditTopup,
		PaymentAmount:       125000,
		PaymentCurrency:     "VND",
		DeviceCode:          strings.ToLower(deviceCode),
		AmountLedger:        12.5,
		ActualCredits:       15000000,
		CreditUnit:          "tokens",
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:credit-topup-payer",
			Email:          "credit-topup-payer@example.test",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyAlways},
	})
	require.NoError(t, err)
	require.Equal(t, service.BotSalesEntitlementCreditTopup, resp.EntitlementKind)
	require.Equal(t, service.BotSalesFulfillmentOperationTopup, resp.Operation)
	require.Equal(t, owner.ID, resp.Buyer.UserID)
	require.Equal(t, deviceCode, resp.DeviceCode)
	require.Equal(t, deviceCode, resp.Delivery.DeviceCode)
	require.Nil(t, resp.Delivery.APIKey)
	require.Equal(t, 1, client.APIKey.Query().CountX(ctx))
	storedKey := client.APIKey.GetX(ctx, existingKey.ID)
	require.NotNil(t, storedKey.GroupID)
	require.Equal(t, ownerGroup.ID, *storedKey.GroupID)
	require.InDelta(t, 12.5, client.User.GetX(ctx, owner.ID).Balance, 0.000001)
	require.InDelta(t, 0, client.User.Query().Where(dbuser.EmailEQ("credit-topup-payer@example.test")).OnlyX(ctx).Balance, 0.000001)

	order := client.PaymentOrder.Query().OnlyX(ctx)
	require.Equal(t, service.OrderStatusCompleted, order.Status)
	require.Equal(t, payment.OrderTypeBalance, order.OrderType)
	require.Equal(t, service.BotSalesPaymentProvider, order.PaymentType)
	require.Equal(t, "VND", order.PaymentCurrency)
	require.Equal(t, float64(125000), order.PaymentAmount)
	require.InDelta(t, 12.5, order.LedgerAmount, 0.000001)
	require.Nil(t, order.BalanceGroupID)
	require.NotNil(t, order.ActualCredits)
	require.Equal(t, int64(15000000), *order.ActualCredits)
	require.Equal(t, service.BotSalesEntitlementCreditTopup, order.ProviderSnapshot["entitlement_kind"])
	require.Equal(t, "tokens", order.ProviderSnapshot["credit_unit"])
}

func TestBotSalesFulfillmentBalancePaymentOrderRetryDoesNotDoubleCredit(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-balance-idempotent", service.SubscriptionTypeNone)
	pkg := client.BalancePackage.Create().
		SetCode("bot_idempotent_20").
		SetLabel("Bot idempotent 20").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	req := service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:      "bs-order-idempotent-balance",
		ExternalOrderItemID:  "item-1",
		ExternalPaymentID:    "bs-pay-idempotent-balance",
		Operation:            service.BotSalesFulfillmentOperationTopup,
		EntitlementKind:      service.BotSalesEntitlementBalance,
		BalancePackageCode:   pkg.Code,
		PaymentAmount:        100000,
		PaymentCurrency:      "VND",
		PaymentProvider:      "sepay",
		PaymentProviderTxnID: "bank-txn-1001",
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:idempotent",
			Email:          "bot-idempotent@example.test",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyNever},
	}

	first, err := svc.Fulfill(ctx, req)
	require.NoError(t, err)
	second, err := svc.Fulfill(ctx, req)
	require.NoError(t, err)
	require.Equal(t, first.Buyer.UserID, second.Buyer.UserID)
	require.Equal(t, pkg.AmountLedger, client.User.GetX(ctx, first.Buyer.UserID).Balance)
	require.Equal(t, 1, client.PaymentOrder.Query().CountX(ctx))

	order := client.PaymentOrder.Query().OnlyX(ctx)
	require.Equal(t, service.OrderStatusCompleted, order.Status)
	require.Equal(t, "sepay", order.ProviderSnapshot["provider_key"])
	require.Equal(t, "item-1", order.ProviderSnapshot["external_order_item_id"])
	logs := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10))).
		Order(dbent.Asc(paymentauditlog.FieldID)).
		AllX(ctx)
	require.Len(t, logs, 3)
	require.Equal(t, "ORDER_CREATED", logs[0].Action)
	require.Equal(t, "ORDER_PAID", logs[1].Action)
	require.Equal(t, "RECHARGE_SUCCESS", logs[2].Action)

	statsSvc := service.NewPaymentService(client, nil, nil, nil, nil, nil, nil, nil, nil)
	stats, err := statsSvc.GetDashboardStats(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, 1, stats.TotalCount)
	require.Len(t, stats.RevenueByCurrency, 1)
	require.Equal(t, "VND", stats.RevenueByCurrency[0].Currency)
	require.Equal(t, float64(100000), stats.RevenueByCurrency[0].TotalAmount)
	require.Equal(t, 1, stats.Deposits.PaidTopups)
}

func TestBotSalesFulfillmentBalanceAccountingFallsBackToPackageVNDOverride(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-balance-derived-accounting", service.SubscriptionTypeNone)
	pkg := client.BalancePackage.Create().
		SetCode("bot_derived_vnd_20").
		SetLabel("Bot derived VND 20").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetCurrencyOverrides(map[string]float64{"VND": 120000}).
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	resp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:     "bs-order-derived-accounting",
		ExternalOrderItemID: "line-1",
		Operation:           service.BotSalesFulfillmentOperationTopup,
		EntitlementKind:     service.BotSalesEntitlementBalance,
		BalancePackageCode:  pkg.Code,
		Quantity:            2,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:derived-accounting",
			Email:          "bot-derived-accounting@example.test",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyNever},
	})
	require.NoError(t, err)
	require.Equal(t, float64(40), resp.Balance.AmountLedger)
	require.Equal(t, int64(54000000), resp.Balance.ActualCredits)
	require.Equal(t, float64(40), client.User.GetX(ctx, resp.Buyer.UserID).Balance)

	order := client.PaymentOrder.Query().OnlyX(ctx)
	require.Equal(t, service.OrderStatusCompleted, order.Status)
	require.Equal(t, float64(240000), order.PaymentAmount)
	require.Equal(t, "VND", order.PaymentCurrency)
	require.Equal(t, float64(40), order.LedgerAmount)
	require.Equal(t, "USD", order.LedgerCurrency)
	require.NotNil(t, order.ActualCredits)
	require.Equal(t, int64(54000000), *order.ActualCredits)
	require.Equal(t, order.OutTradeNo, order.PaymentTradeNo)
	require.Equal(t, "balance_package.currency_overrides", order.ProviderSnapshot["payment_amount_source"])
	require.Equal(t, float64(2), order.ProviderSnapshot["quantity"])
	require.Equal(t, order.OutTradeNo, order.ProviderSnapshot["external_payment_id"])
}

func TestBotSalesFulfillmentBalanceAccountingRequiresAmountOrVNDOverride(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-balance-missing-accounting", service.SubscriptionTypeNone)
	pkg := client.BalancePackage.Create().
		SetCode("bot_missing_vnd").
		SetLabel("Bot missing VND").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	_, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-missing-accounting",
		Operation:          service.BotSalesFulfillmentOperationTopup,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: pkg.Code,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:missing-accounting",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyNever},
	})
	require.Error(t, err)
	require.Equal(t, "BOT_SALES_PACKAGE_VND_PRICE_REQUIRED", infraerrorsReason(err))
}

func TestBotSalesFulfillmentBalanceRejectsInvalidQuantity(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-balance-invalid-quantity", service.SubscriptionTypeNone)
	pkg := client.BalancePackage.Create().
		SetCode("bot_invalid_quantity").
		SetLabel("Bot invalid quantity").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetCurrencyOverrides(map[string]float64{"VND": 120000}).
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	cases := []struct {
		name       string
		quantity   int
		rawPayload map[string]any
	}{
		{name: "explicit_zero", quantity: 0, rawPayload: map[string]any{"quantity": float64(0)}},
		{name: "negative", quantity: -1},
		{name: "too_large", quantity: 1001},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
				ExternalOrderID:    "bs-order-invalid-quantity-" + tc.name,
				Operation:          service.BotSalesFulfillmentOperationTopup,
				EntitlementKind:    service.BotSalesEntitlementBalance,
				BalancePackageCode: pkg.Code,
				Quantity:           tc.quantity,
				RawPayload:         tc.rawPayload,
				Buyer: service.BotSalesFulfillmentBuyer{
					ExternalUserID: "channel:telegram:user:invalid-quantity-" + tc.name,
				},
				DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyNever},
			})
			require.Error(t, err)
			require.Equal(t, "BOT_SALES_QUANTITY_INVALID", infraerrorsReason(err))
		})
	}
}

func TestBotSalesFulfillmentBalanceReplayRejectsConflictingBuyerOrQuantity(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-balance-replay-conflict", service.SubscriptionTypeNone)
	pkg := client.BalancePackage.Create().
		SetCode("bot_replay_conflict").
		SetLabel("Bot replay conflict").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetCurrencyOverrides(map[string]float64{"VND": 120000}).
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	baseReq := service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:     "bs-order-replay-conflict",
		ExternalOrderItemID: "line-1",
		Operation:           service.BotSalesFulfillmentOperationTopup,
		EntitlementKind:     service.BotSalesEntitlementBalance,
		BalancePackageCode:  pkg.Code,
		Quantity:            1,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:replay-owner",
			Email:          "bot-replay-owner@example.test",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyNever},
	}
	_, err := svc.Fulfill(ctx, baseReq)
	require.NoError(t, err)
	require.Equal(t, float64(20), client.User.Query().Where(dbuser.EmailEQ("bot-replay-owner@example.test")).OnlyX(ctx).Balance)

	conflictingBuyer := baseReq
	conflictingBuyer.Buyer = service.BotSalesFulfillmentBuyer{
		ExternalUserID: "channel:telegram:user:replay-attacker",
		Email:          "bot-replay-attacker@example.test",
	}
	_, err = svc.Fulfill(ctx, conflictingBuyer)
	require.Error(t, err)
	require.Equal(t, "BOT_SALES_FULFILLMENT_CONFLICT", infraerrorsReason(err))
	require.Equal(t, 1, client.PaymentOrder.Query().CountX(ctx))
	require.Equal(t, float64(20), client.User.Query().Where(dbuser.EmailEQ("bot-replay-owner@example.test")).OnlyX(ctx).Balance)

	conflictingQuantity := baseReq
	conflictingQuantity.Quantity = 2
	_, err = svc.Fulfill(ctx, conflictingQuantity)
	require.Error(t, err)
	require.Equal(t, "BOT_SALES_FULFILLMENT_CONFLICT", infraerrorsReason(err))

	otherPkg := client.BalancePackage.Create().
		SetCode("bot_replay_other_package").
		SetLabel("Bot replay other package").
		SetAmountLedger(30).
		SetActualCredits(33000000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetCurrencyOverrides(map[string]float64{"VND": 130000}).
		SetForSale(true).
		SaveX(ctx)
	conflictingPackage := baseReq
	conflictingPackage.BalancePackageCode = otherPkg.Code
	_, err = svc.Fulfill(ctx, conflictingPackage)
	require.Error(t, err)
	require.Equal(t, "BOT_SALES_FULFILLMENT_CONFLICT", infraerrorsReason(err))

	conflictingOperation := baseReq
	conflictingOperation.Operation = service.BotSalesFulfillmentOperationNew
	_, err = svc.Fulfill(ctx, conflictingOperation)
	require.Error(t, err)
	require.Equal(t, "BOT_SALES_FULFILLMENT_CONFLICT", infraerrorsReason(err))
}

func TestBotSalesFulfillmentCreatesBuyerWithDefaultLimitsFromSettings(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	require.NoError(t, testsupport.SetSettings(ctx, client, map[string]string{
		service.SettingKeyDefaultConcurrency:  "5",
		service.SettingKeyDefaultUserRPMLimit: "123",
	}))
	group := createBotSalesGroup(t, client, "bot-default-limits", service.SubscriptionTypeSubscription)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Bot default limits monthly").
		SetPrice(9.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	resp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID: "bs-order-default-limits",
		Operation:       service.BotSalesFulfillmentOperationNew,
		EntitlementKind: service.BotSalesEntitlementSubscription,
		PlanID:          plan.ID,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:default-limits",
		},
	})
	require.NoError(t, err)

	created := client.User.GetX(ctx, resp.Buyer.UserID)
	require.Equal(t, 5, created.Concurrency)
	require.Equal(t, 123, created.RpmLimit)
}

func TestBotSalesFulfillmentDefaultsTelegramNotifyChatIDForNewBuyer(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-telegram-notify-default", service.SubscriptionTypeSubscription)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Bot telegram notify default").
		SetPrice(9.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	resp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID: "bs-order-telegram-notify-default",
		Operation:       service.BotSalesFulfillmentOperationNew,
		EntitlementKind: service.BotSalesEntitlementSubscription,
		PlanID:          plan.ID,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:123456789",
			Provider:       "telegram",
			ProviderUserID: "123456789",
			TelegramID:     "123456789",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyNever},
	})
	require.NoError(t, err)

	created := client.User.GetX(ctx, resp.Buyer.UserID)
	require.Equal(t, "123456789", created.BalanceNotifyTelegramChatID)
}

func TestBotSalesFulfillmentDefaultsTelegramNotifyChatIDFromLegacyExternalIDWithEmail(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-telegram-notify-legacy", service.SubscriptionTypeSubscription)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Bot telegram notify legacy").
		SetPrice(9.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	resp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID: "bs-order-telegram-notify-legacy",
		Operation:       service.BotSalesFulfillmentOperationNew,
		EntitlementKind: service.BotSalesEntitlementSubscription,
		PlanID:          plan.ID,
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "telegram:246810",
			Email:          "legacy-telegram-buyer@example.test",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyNever},
	})
	require.NoError(t, err)

	created := client.User.GetX(ctx, resp.Buyer.UserID)
	require.Equal(t, "legacy-telegram-buyer@example.test", created.Email)
	require.Equal(t, "246810", created.BalanceNotifyTelegramChatID)
}

func TestBotSalesFulfillmentFillsMissingTelegramNotifyChatIDForExistingSyntheticBuyer(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-telegram-notify-existing", service.SubscriptionTypeNone)
	pkg := client.BalancePackage.Create().
		SetCode("standard_20_telegram_notify_existing").
		SetLabel("Standard 20 telegram notify existing").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetForSale(true).
		SaveX(ctx)
	existing := client.User.Create().
		SetEmail("channel-telegram-user-987654321@bot-sales.local").
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetConcurrency(1).
		SetBalance(15).
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	resp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-telegram-notify-existing",
		Operation:          service.BotSalesFulfillmentOperationTopup,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: pkg.Code,
		PaymentAmount:      100000,
		PaymentCurrency:    "VND",
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:987654321",
			Provider:       "telegram",
			ProviderUserID: "987654321",
			TelegramID:     "987654321",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyNever},
	})
	require.NoError(t, err)
	require.Equal(t, existing.ID, resp.Buyer.UserID)

	updated := client.User.GetX(ctx, existing.ID)
	require.Equal(t, "987654321", updated.BalanceNotifyTelegramChatID)
}

func TestBotSalesFulfillmentPreservesExistingTelegramNotifyChatIDOverride(t *testing.T) {
	ctx := context.Background()
	client, db := newBotSalesFulfillmentEntClient(t)
	group := createBotSalesGroup(t, client, "bot-telegram-notify-override", service.SubscriptionTypeNone)
	pkg := client.BalancePackage.Create().
		SetCode("standard_20_telegram_notify_override").
		SetLabel("Standard 20 telegram notify override").
		SetAmountLedger(20).
		SetActualCredits(27000000).
		SetCreditUnit("tokens").
		SetGroupID(group.ID).
		SetForSale(true).
		SaveX(ctx)
	existing := client.User.Create().
		SetEmail("channel-telegram-user-111222333@bot-sales.local").
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetConcurrency(1).
		SetBalance(15).
		SetBalanceNotifyTelegramChatID("444555666").
		SaveX(ctx)

	svc := newBotSalesFulfillmentServiceForTest(client, db)
	resp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
		ExternalOrderID:    "bs-order-telegram-notify-override",
		Operation:          service.BotSalesFulfillmentOperationTopup,
		EntitlementKind:    service.BotSalesEntitlementBalance,
		BalancePackageCode: pkg.Code,
		PaymentAmount:      100000,
		PaymentCurrency:    "VND",
		Buyer: service.BotSalesFulfillmentBuyer{
			ExternalUserID: "channel:telegram:user:111222333",
			Provider:       "telegram",
			ProviderUserID: "111222333",
			TelegramID:     "111222333",
		},
		DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyNever},
	})
	require.NoError(t, err)
	require.Equal(t, existing.ID, resp.Buyer.UserID)

	updated := client.User.GetX(ctx, existing.ID)
	require.Equal(t, "444555666", updated.BalanceNotifyTelegramChatID)
}

func TestBotSalesFulfillmentBalanceTopupWithoutDeviceCodeReusesLegacyBuyerAcrossProviders(t *testing.T) {
	cases := []struct {
		name           string
		legacyEmail    string
		externalUserID string
		provider       string
		providerUserID string
		telegramID     string
	}{
		{
			name:           "telegram",
			legacyEmail:    "telegram-123456789@bot-sales.local",
			externalUserID: "channel:telegram:user:123456789",
			provider:       "telegram",
			providerUserID: "123456789",
			telegramID:     "123456789",
		},
		{
			name:           "zalo",
			legacyEmail:    "zalo-zalo-user-42@bot-sales.local",
			externalUserID: "channel:zalo:user:zalo-user-42",
			provider:       "zalo",
			providerUserID: "zalo-user-42",
		},
		{
			name:           "kakao",
			legacyEmail:    "kakao-kakao-user-42@bot-sales.local",
			externalUserID: "channel:kakao:user:kakao-user-42",
			provider:       "kakao",
			providerUserID: "kakao-user-42",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client, db := newBotSalesFulfillmentEntClient(t)
			group := createBotSalesGroup(t, client, "bot-balance-topup-legacy-buyer-"+tc.name, service.SubscriptionTypeNone)
			pkg := client.BalancePackage.Create().
				SetCode("standard_20_legacy_buyer_" + tc.name).
				SetLabel("Standard 20 legacy buyer " + tc.name).
				SetAmountLedger(20).
				SetActualCredits(27000000).
				SetCreditUnit("tokens").
				SetGroupID(group.ID).
				SetForSale(true).
				SaveX(ctx)
			legacyBuyer := client.User.Create().
				SetEmail(tc.legacyEmail).
				SetPasswordHash("test-password-hash").
				SetRole(service.RoleUser).
				SetStatus(service.StatusActive).
				SetConcurrency(1).
				SetBalance(15).
				SaveX(ctx)

			svc := newBotSalesFulfillmentServiceForTest(client, db)
			topupResp, err := svc.Fulfill(ctx, service.BotSalesTokenFulfillmentRequest{
				ExternalOrderID:    "bs-order-" + tc.name + "-legacy-topup",
				Operation:          service.BotSalesFulfillmentOperationTopup,
				EntitlementKind:    service.BotSalesEntitlementBalance,
				BalancePackageCode: pkg.Code,
				ExternalPaymentID:  "pay-" + tc.name + "-legacy-topup",
				PaymentAmount:      100000,
				PaymentCurrency:    "VND",
				Buyer: service.BotSalesFulfillmentBuyer{
					ExternalUserID: tc.externalUserID,
					Provider:       tc.provider,
					ProviderUserID: tc.providerUserID,
					TelegramID:     tc.telegramID,
				},
				DeliveryPolicy: service.BotSalesDeliveryPolicy{IssueAPIKey: service.BotSalesIssueAPIKeyIfMissing},
			})
			require.NoError(t, err)
			require.Equal(t, service.BotSalesFulfillmentOperationTopup, topupResp.Operation)
			require.Equal(t, legacyBuyer.ID, topupResp.Buyer.UserID)
			require.Empty(t, topupResp.Delivery.DeviceCode)
			require.Equal(t, float64(35), client.User.GetX(ctx, legacyBuyer.ID).Balance)
			require.Equal(t, 1, client.User.Query().CountX(ctx))
		})
	}
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
	return createBotSalesGroupWithPlatform(t, client, name, service.PlatformAnthropic, subscriptionType)
}

func createBotSalesGroupWithPlatform(t *testing.T, client *dbent.Client, name string, platform string, subscriptionType string) *dbent.Group {
	t.Helper()
	return client.Group.Create().
		SetName(name).
		SetPlatform(platform).
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
