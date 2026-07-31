//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestRejectRefundRequestRestoresCompletedAndNotifiesUser(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-rejected@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-rejected-user").
		Save(ctx)
	require.NoError(t, err)
	requestedAt := time.Now().Add(-5 * time.Minute)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-REJECTED").
		SetOutTradeNo("sub2_refund_rejected").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-rejected").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefundRequested).
		SetRefundAmount(88).
		SetRefundRequestedAt(requestedAt).
		SetRefundRequestReason("changed my mind").
		SetRefundRequestedBy(strconv.FormatInt(user.ID, 10)).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now().Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	settingRepo := newNotificationEmailMemorySettingRepo()
	require.NoError(t, settingRepo.Set(ctx, SettingRefundResultUserEmailEnabled, "true"))
	notificationSvc := NewNotificationEmailService(settingRepo, nil)
	_, err = notificationSvc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{
		Channels: []NotificationEmailChannelPolicy{{
			ID:                 NotificationEmailChannelRefundUser,
			Enabled:            true,
			IncludeUserPrimary: true,
		}},
	})
	require.NoError(t, err)
	deliveryRepo := newFakeNotificationEmailDeliveryRepository()
	userRepo := &mockUserRepo{getByIDUser: &User{ID: user.ID, Balance: 123}}
	svc := &PaymentService{
		entClient:                   client,
		configService:               NewPaymentConfigService(client, settingRepo, nil),
		userRepo:                    userRepo,
		notificationEmailService:    notificationSvc,
		notificationEmailDispatcher: NewNotificationEmailDispatcher(deliveryRepo, notificationSvc),
	}
	require.NoError(t, svc.RejectRefundRequest(ctx, order.ID, 42, "不符合退款条件"))

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Zero(t, reloaded.RefundAmount)
	require.Nil(t, reloaded.RefundRequestedAt)
	require.Nil(t, reloaded.RefundRequestReason)
	require.Nil(t, reloaded.RefundRequestedBy)
	require.Equal(t, 123.0, userRepo.getByIDUser.Balance)

	deliveryRepo.mu.Lock()
	require.Len(t, deliveryRepo.items, 1)
	require.Equal(t, NotificationEmailEventRefundRejectedUser, deliveryRepo.items[0].Event)
	require.Equal(t, user.Email, deliveryRepo.items[0].RecipientEmail)
	require.Equal(t, "100.00", deliveryRepo.items[0].Variables["refund_amount"])
	require.Equal(t, "不符合退款条件", deliveryRepo.items[0].Variables["refund_reason"])
	deliveryRepo.mu.Unlock()

	logs, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_REQUEST_REJECTED")).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, "admin:42", logs[0].Operator)
	var detail map[string]any
	require.NoError(t, json.Unmarshal([]byte(logs[0].Detail), &detail))
	require.Equal(t, "changed my mind", detail["request_reason"])
	require.Equal(t, "不符合退款条件", detail["rejection_reason"])

	require.Error(t, svc.RejectRefundRequest(ctx, order.ID, 42, "duplicate"))
	deliveryRepo.mu.Lock()
	require.Len(t, deliveryRepo.items, 1)
	deliveryRepo.mu.Unlock()
}

func TestRejectRefundRequestRequiresReasonAndPendingRequest(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("refund-reject-validation@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-reject-validation-user").
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(10).
		SetPayAmount(10).
		SetFeeRate(0).
		SetRechargeCode("REFUND-REJECT-VALIDATION").
		SetOutTradeNo("sub2_refund_reject_validation").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client}

	require.Error(t, svc.RejectRefundRequest(ctx, order.ID, 1, " "))
	require.Error(t, svc.RejectRefundRequest(ctx, order.ID, 1, "not pending"))
}

func TestRejectRefundRequestMakesPreparedApprovalStale(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("refund-reject-race@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-reject-race-user").
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(10).
		SetPayAmount(10).
		SetFeeRate(0).
		SetRechargeCode("REFUND-REJECT-RACE").
		SetOutTradeNo("sub2_refund_reject_race").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefundRequested).
		SetRefundAmount(10).
		SetRefundRequestedAt(time.Now().Add(-time.Minute)).
		SetRefundRequestReason("duplicate payment").
		SetRefundRequestedBy(strconv.FormatInt(user.ID, 10)).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	firstRequest := order
	staleApproval := &RefundPlan{OrderID: order.ID, Order: firstRequest}
	require.NoError(t, svc.RejectRefundRequest(ctx, order.ID, 42, "not eligible"))

	secondRequestedAt := time.Now().Add(time.Second)
	secondRequestIdentity, identityErr := newRefundRequestIdentity()
	require.NoError(t, identityErr)
	updated, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetStatus(OrderStatusRefundRequested).
		SetRefundAmount(7).
		SetRefundRequestedAt(secondRequestedAt).
		SetRefundRequestReason("second request").
		SetRefundRequestedBy(secondRequestIdentity).
		Save(ctx)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundRequested, updated.Status)

	_, err = svc.ExecuteRefund(ctx, staleApproval)
	require.Error(t, err)
	require.Error(t, svc.rejectRefundRequestDecision(ctx, firstRequest, 42, "stale rejection"))
	reloaded, reloadErr := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, reloadErr)
	require.Equal(t, OrderStatusRefundRequested, reloaded.Status)
	require.Equal(t, 7.0, reloaded.RefundAmount)
	require.Equal(t, "second request", psStringValue(reloaded.RefundRequestReason))

	exec, ok := client.Driver().(refundNotificationSQLExecutor)
	require.True(t, ok)
	_, err = exec.ExecContext(ctx, `
		CREATE TRIGGER fail_reject_audit
		BEFORE INSERT ON payment_audit_logs
		WHEN NEW.action = 'REFUND_REQUEST_REJECTED'
		BEGIN
			SELECT RAISE(FAIL, 'refund rejection audit unavailable');
		END;
	`)
	require.NoError(t, err)
	require.Error(t, svc.RejectRefundRequest(ctx, order.ID, 42, "current rejection"))
	reloaded, reloadErr = client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, reloadErr)
	require.Equal(t, OrderStatusRefundRequested, reloaded.Status)
	require.Equal(t, "second request", psStringValue(reloaded.RefundRequestReason))
}

func TestRequestRefundQueuesDurablyWithoutRollingBackOrDuplicatingRequest(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-email-failure@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-email-failure-user").
		Save(ctx)
	require.NoError(t, err)
	providerInstance, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("refund-email-provider").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-EMAIL-FAILURE").
		SetOutTradeNo("sub2_refund_email_failure").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-email-failure").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(providerInstance.ID, 10)).
		Save(ctx)
	require.NoError(t, err)

	settingRepo := newNotificationEmailMemorySettingRepo()
	require.NoError(t, settingRepo.Set(ctx, SettingRefundRequestUserEmailEnabled, "true"))
	require.NoError(t, settingRepo.Set(ctx, SettingRefundRequestAdminEmailEnabled, "true"))
	notificationSvc := NewNotificationEmailService(settingRepo, nil)
	_, err = notificationSvc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{
		Channels: []NotificationEmailChannelPolicy{
			{ID: NotificationEmailChannelRefundUser, Enabled: true, IncludeUserPrimary: true},
			{ID: NotificationEmailChannelRefundAdmin, Enabled: true, RecipientGroup: NotificationEmailRecipientGroupFinance},
		},
		RecipientGroups: []NotificationEmailRecipientGroup{{
			ID:      NotificationEmailRecipientGroupFinance,
			Members: []NotificationEmailRecipientMember{{Email: "finance@example.com", Enabled: true}},
		}},
	})
	require.NoError(t, err)
	deliveryRepo := newFakeNotificationEmailDeliveryRepository()

	svc := &PaymentService{
		entClient:                   client,
		configService:               NewPaymentConfigService(client, settingRepo, nil),
		userRepo:                    &mockUserRepo{getByIDUser: &User{ID: user.ID, Balance: 200}},
		notificationEmailService:    notificationSvc,
		notificationEmailDispatcher: NewNotificationEmailDispatcher(deliveryRepo, notificationSvc),
	}
	require.NoError(t, svc.RequestRefund(ctx, order.ID, user.ID, "duplicate charge"))

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundRequested, reloaded.Status)
	require.NotNil(t, reloaded.RefundRequestedBy)
	require.True(t, strings.HasPrefix(*reloaded.RefundRequestedBy, "r:"))
	require.Len(t, *reloaded.RefundRequestedBy, 18)
	requestAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_REQUESTED")).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, requestAudits, 1)
	require.Equal(t, fmt.Sprintf("user:%d", user.ID), requestAudits[0].Operator)
	require.Error(t, svc.RequestRefund(ctx, order.ID, user.ID, "duplicate charge"))

	deliveryRepo.mu.Lock()
	require.Len(t, deliveryRepo.items, 2)
	require.Equal(t, NotificationEmailEventRefundRequestedUser, deliveryRepo.items[0].Event)
	require.Equal(t, NotificationEmailDeliveryStatusPending, deliveryRepo.items[0].Status)
	require.Equal(t, "100.00", deliveryRepo.items[0].Variables["refund_amount"])
	require.Equal(t, NotificationEmailEventRefundRequestedAdmin, deliveryRepo.items[1].Event)
	require.Equal(t, "finance@example.com", deliveryRepo.items[1].RecipientEmail)
	require.Equal(t, strconv.FormatInt(user.ID, 10), deliveryRepo.items[1].Variables["refund_user_id"])
	require.Equal(t, user.Email, deliveryRepo.items[1].Variables["refund_user_email"])
	deliveryRepo.mu.Unlock()

	reloaded, err = client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundRequested, reloaded.Status)
}

func TestRequestRefundRollsBackOrderWhenOutboxInsertFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("refund-outbox-rollback@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-outbox-rollback-user").
		Save(ctx)
	require.NoError(t, err)
	providerInstance, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("refund-outbox-rollback-provider").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("REFUND-OUTBOX-ROLLBACK").
		SetOutTradeNo("sub2_refund_outbox_rollback").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-outbox-rollback").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(providerInstance.ID, 10)).
		Save(ctx)
	require.NoError(t, err)

	settingRepo := newNotificationEmailMemorySettingRepo()
	require.NoError(t, settingRepo.Set(ctx, SettingRefundRequestUserEmailEnabled, "true"))
	notificationSvc := NewNotificationEmailService(settingRepo, nil)
	_, err = notificationSvc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{Channels: []NotificationEmailChannelPolicy{{
		ID: NotificationEmailChannelRefundUser, Enabled: true, IncludeUserPrimary: true,
	}}})
	require.NoError(t, err)
	deliveryRepo := newFakeNotificationEmailDeliveryRepository()
	deliveryRepo.enqueueErr = errors.New("outbox unavailable")
	svc := &PaymentService{
		entClient: client, configService: NewPaymentConfigService(client, settingRepo, nil),
		userRepo:                    &mockUserRepo{getByIDUser: &User{ID: user.ID, Balance: 200}},
		notificationEmailDispatcher: NewNotificationEmailDispatcher(deliveryRepo, notificationSvc),
	}

	err = svc.RequestRefund(ctx, order.ID, user.ID, "duplicate charge")
	require.ErrorContains(t, err, "outbox unavailable")
	reloaded, reloadErr := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, reloadErr)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
}

func TestRefundResultCommitsWhenOutboxInsertFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("refund-result-outbox@example.com").SetPasswordHash("hash").SetUsername("refund-result-user").Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(88).SetPayAmount(100).SetFeeRate(0).SetRechargeCode("REFUND-RESULT-OUTBOX").
		SetOutTradeNo("sub2_refund_result_outbox").SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-result-outbox").SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefunding).SetExpiresAt(time.Now().Add(time.Hour)).SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").SetSrcHost("api.example.com").Save(ctx)
	require.NoError(t, err)

	settingRepo := newNotificationEmailMemorySettingRepo()
	require.NoError(t, settingRepo.Set(ctx, SettingRefundResultUserEmailEnabled, "true"))
	notificationSvc := NewNotificationEmailService(settingRepo, nil)
	_, err = notificationSvc.UpdatePolicy(ctx, NotificationEmailPolicyUpdate{Channels: []NotificationEmailChannelPolicy{{
		ID: NotificationEmailChannelRefundUser, Enabled: true, IncludeUserPrimary: true,
	}}})
	require.NoError(t, err)
	deliveryRepo := newFakeNotificationEmailDeliveryRepository()
	deliveryRepo.enqueueErr = errors.New("outbox unavailable")
	svc := &PaymentService{
		entClient: client, configService: NewPaymentConfigService(client, settingRepo, nil),
		notificationEmailDispatcher: NewNotificationEmailDispatcher(deliveryRepo, notificationSvc),
	}

	result, err := svc.markRefundOk(ctx, &RefundPlan{
		OrderID: order.ID, Order: order, RefundAmount: order.Amount, GatewayAmount: order.PayAmount, Reason: "duplicate charge",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunded, reloaded.Status)
	auditCount, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_EMAIL_ENQUEUE_FAILED")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, auditCount)
}

func TestValidateRefundRequestRejectsLegacyGuessedProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-legacy@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-legacy-user").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("REFUND-LEGACY-ORDER").
		SetOutTradeNo("sub2_refund_legacy_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-legacy-refund").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	_, err = svc.validateRefundRequest(ctx, order.ID, user.ID)
	require.Error(t, err)
	require.Equal(t, "USER_REFUND_DISABLED", infraerrors.Reason(err))
}

func TestPrepareRefundRejectsLegacyGuessedProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-legacy-admin@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-legacy-admin-user").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-admin-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(188).
		SetPayAmount(188).
		SetFeeRate(0).
		SetRechargeCode("REFUND-LEGACY-ADMIN-ORDER").
		SetOutTradeNo("sub2_refund_legacy_admin_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-legacy-admin-refund").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_DISABLED", infraerrors.Reason(err))
}

func TestPrepareRefundEnforcesA5FullRefundSafety(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	configService := &PaymentConfigService{entClient: client, encryptionKey: key}
	config := validEasyPayProviderConfig(t)
	config["compatibilityMode"] = "a5"
	instance, err := configService.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey:    payment.TypeEasyPay,
		Name:           "a5-refund-safety",
		Config:         config,
		SupportedTypes: []string{payment.TypeAlipay, payment.TypeWxpay},
		Enabled:        true,
		RefundEnabled:  true,
	})
	require.NoError(t, err)
	order := createProviderRefundOrder(t, ctx, client, instance, OrderStatusCompleted, "a5-partial-refund")
	order, err = client.PaymentOrder.UpdateOneID(order.ID).
		SetProviderSnapshot(map[string]any{
			"schema_version":       3,
			"provider_instance_id": strconv.FormatInt(instance.ID, 10),
			"provider_key":         payment.TypeEasyPay,
			"compatibility_mode":   provider.EasyPayCompatibilityA5,
		}).
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: payment.NewDefaultLoadBalancer(client, key),
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 50, "partial", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "A5_PARTIAL_REFUND_UNSUPPORTED", infraerrors.Reason(err))
	for _, amount := range []float64{99.991, 100.009} {
		plan, result, err = svc.PrepareRefund(ctx, order.ID, amount, "full within currency tolerance", false, false)
		require.NoError(t, err)
		require.Nil(t, result)
		require.NotNil(t, plan)
		require.Equal(t, 100.0, plan.RefundAmount)
		require.Equal(t, 100.0, plan.GatewayAmount)
	}

	_, err = client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRefundPending).Save(ctx)
	require.NoError(t, err)
	plan, result, err = svc.PrepareRefund(ctx, order.ID, 100, "retry", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "A5_REFUND_PENDING_RETRY_BLOCKED", infraerrors.Reason(err))
}

func TestGwRefundRejectsAlipayMerchantIdentitySnapshotMismatch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-snapshot-mismatch@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-snapshot-mismatch-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-mismatch-instance").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{
			"appId":      "runtime-alipay-app",
			"privateKey": "runtime-private-key",
		})).
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("REFUND-SNAPSHOT-MISMATCH-ORDER").
		SetOutTradeNo("sub2_refund_snapshot_mismatch_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-snapshot-mismatch").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		SetProviderSnapshot(map[string]any{
			"schema_version":       2,
			"provider_instance_id": instID,
			"provider_key":         payment.TypeAlipay,
			"merchant_app_id":      "expected-alipay-app",
		}).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
	}

	_, err = svc.gwRefund(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  order.Amount,
		GatewayAmount: order.Amount,
		Reason:        "snapshot mismatch",
	})
	require.ErrorContains(t, err, "alipay app_id mismatch")
}

func TestGwRefundA5UsesMerchantOrderWhenTradeNumberIsMissing(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	configService := &PaymentConfigService{entClient: client, encryptionKey: key}
	config := validEasyPayProviderConfig(t)
	config["compatibilityMode"] = "a5"
	instance, err := configService.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey:    payment.TypeEasyPay,
		Name:           "a5-no-trade-number",
		Config:         config,
		SupportedTypes: []string{payment.TypeAlipay},
		Enabled:        true,
		RefundEnabled:  true,
	})
	require.NoError(t, err)
	order := createProviderRefundOrder(t, ctx, client, instance, OrderStatusCompleted, "a5-no-trade-number")
	order, err = client.PaymentOrder.UpdateOneID(order.ID).
		SetPaymentTradeNo("").
		SetProviderSnapshot(map[string]any{
			"schema_version":       3,
			"provider_instance_id": strconv.FormatInt(instance.ID, 10),
			"provider_key":         payment.TypeEasyPay,
			"compatibility_mode":   provider.EasyPayCompatibilityA5,
		}).
		Save(ctx)
	require.NoError(t, err)

	capture := &refundCaptureProviderTestDouble{providerKey: payment.TypeEasyPay}
	restore := replacePaymentProviderFactoryForTest(t, capture)
	defer restore()
	svc := &PaymentService{entClient: client, loadBalancer: payment.NewDefaultLoadBalancer(client, key)}
	_, err = svc.gwRefund(ctx, &RefundPlan{
		OrderID: order.ID, Order: order, RefundAmount: order.Amount,
		GatewayAmount: order.PayAmount, Reason: "full refund",
	})
	require.NoError(t, err)
	require.Equal(t, order.OutTradeNo, capture.request.OrderID)
	require.Empty(t, capture.request.TradeNo)
	require.True(t, capture.request.FullRefund)
}

func TestGwRefundStandardEasyPayWithoutTradeNumberDoesNotLoadCurrentConfig(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	configService := &PaymentConfigService{entClient: client, encryptionKey: key}
	instance, err := configService.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey:    payment.TypeEasyPay,
		Name:           "standard-no-trade-number",
		Config:         validEasyPayProviderConfig(t),
		SupportedTypes: []string{payment.TypeAlipay},
		Enabled:        true,
		RefundEnabled:  true,
	})
	require.NoError(t, err)
	order := createProviderRefundOrder(t, ctx, client, instance, OrderStatusCompleted, "standard-no-trade-number")
	order, err = client.PaymentOrder.UpdateOneID(order.ID).
		SetPaymentTradeNo("").
		SetProviderSnapshot(map[string]any{
			"schema_version":       3,
			"provider_instance_id": strconv.FormatInt(instance.ID, 10),
			"provider_key":         payment.TypeEasyPay,
			"compatibility_mode":   provider.EasyPayCompatibilityStandard,
		}).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	resp, err := svc.gwRefund(ctx, &RefundPlan{
		OrderID: order.ID, Order: order, RefundAmount: order.Amount,
		GatewayAmount: order.PayAmount, Reason: "legacy standard refund",
	})
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, resp.Status)
}

func TestHistoricalEasyPayOrdersPinModeForQueryAndRefund(t *testing.T) {
	for _, tc := range []struct {
		name          string
		schemaVersion int
		snapshotMode  string
		currentMode   string
		wantMode      string
	}{
		{
			name:          "A5 order after instance switched to standard",
			schemaVersion: 3,
			snapshotMode:  provider.EasyPayCompatibilityA5,
			currentMode:   provider.EasyPayCompatibilityStandard,
			wantMode:      provider.EasyPayCompatibilityA5,
		},
		{
			name:          "standard order after instance switched to A5",
			schemaVersion: 3,
			snapshotMode:  provider.EasyPayCompatibilityStandard,
			currentMode:   provider.EasyPayCompatibilityA5,
			wantMode:      provider.EasyPayCompatibilityStandard,
		},
		{
			name:          "ts3 order follows upgraded A5 instance",
			schemaVersion: 2,
			currentMode:   provider.EasyPayCompatibilityA5,
			wantMode:      provider.EasyPayCompatibilityA5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			user, err := client.User.Create().
				SetEmail("historical-mode-" + tc.snapshotMode + "@example.com").
				SetPasswordHash("hash").
				SetUsername("historical-mode-" + tc.snapshotMode).
				Save(ctx)
			require.NoError(t, err)
			instance, err := client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeEasyPay).
				SetName("historical-mode-" + tc.snapshotMode).
				SetConfig("{}").
				SetSupportedTypes(payment.TypeAlipay).
				SetEnabled(true).
				SetRefundEnabled(true).
				Save(ctx)
			require.NoError(t, err)
			instanceID := strconv.FormatInt(instance.ID, 10)
			snapshot := map[string]any{
				"schema_version":       tc.schemaVersion,
				"provider_instance_id": instanceID,
				"provider_key":         payment.TypeEasyPay,
			}
			if tc.snapshotMode != "" {
				snapshot["compatibility_mode"] = tc.snapshotMode
			}
			order, err := client.PaymentOrder.Create().
				SetUserID(user.ID).
				SetUserEmail(user.Email).
				SetUserName(user.Username).
				SetAmount(10).
				SetPayAmount(10).
				SetRechargeCode("HISTORICAL-" + tc.snapshotMode).
				SetOutTradeNo("sub2_historical_" + tc.snapshotMode).
				SetPaymentType(payment.TypeAlipay).
				SetPaymentTradeNo("trade-historical-" + tc.snapshotMode).
				SetOrderType(payment.OrderTypeBalance).
				SetStatus(OrderStatusCompleted).
				SetExpiresAt(time.Now().Add(time.Hour)).
				SetClientIP("127.0.0.1").
				SetSrcHost("api.example.com").
				SetProviderInstanceID(instanceID).
				SetProviderKey(payment.TypeEasyPay).
				SetProviderSnapshot(snapshot).
				Save(ctx)
			require.NoError(t, err)

			var capturedModes []string
			originalFactory := createPaymentProviderFromInstance
			createPaymentProviderFromInstance = func(_ string, _ string, config map[string]string) (payment.Provider, error) {
				capturedModes = append(capturedModes, provider.EasyPayCompatibilityMode(config))
				return refundProviderTestDouble{}, nil
			}
			defer func() { createPaymentProviderFromInstance = originalFactory }()
			svc := &PaymentService{
				entClient: client,
				loadBalancer: &refundConfigLoadBalancer{config: map[string]string{
					provider.EasyPayCompatibilityModeKey: tc.currentMode,
				}},
			}
			_, err = svc.getOrderProvider(ctx, order)
			require.NoError(t, err)
			_, err = svc.getRefundProvider(ctx, order)
			require.NoError(t, err)
			require.Equal(t, []string{tc.wantMode, tc.wantMode}, capturedModes)
		})
	}
}

func TestTS3EasyPayOrderUsesA5ProviderAfterUpgrade(t *testing.T) {
	ctx := context.Background()
	instance := &dbent.PaymentProviderInstance{ID: 73, ProviderKey: payment.TypeEasyPay}
	order := &dbent.PaymentOrder{
		ID: 84,
		ProviderSnapshot: map[string]any{
			"schema_version":       2,
			"provider_instance_id": "73",
			"provider_key":         payment.TypeEasyPay,
		},
	}
	svc := &PaymentService{loadBalancer: &refundConfigLoadBalancer{config: map[string]string{
		"pid":                                "merchant-ts3-upgrade",
		"pkey":                               "test-key",
		"apiBase":                            "https://pay.example.com",
		"notifyUrl":                          "https://api.example.com/notify",
		"returnUrl":                          "https://api.example.com/return",
		provider.EasyPayCompatibilityModeKey: provider.EasyPayCompatibilityA5,
	}}}

	prov, err := svc.createOrderBoundProvider(ctx, instance, order)
	require.NoError(t, err)
	require.IsType(t, &provider.A5EasyPay{}, prov)
	_, supportsQuery := prov.(payment.RefundQueryProvider)
	require.True(t, supportsQuery)
}

func TestCalculateGatewayRefundAmountUsesCurrencyPrecision(t *testing.T) {
	require.InDelta(t, 6.173, calculateGatewayRefundAmount(100, 12.345, 50, "KWD"), 1e-12)
	require.InDelta(t, 12.345, calculateGatewayRefundAmount(100, 12.345, 100, "KWD"), 1e-12)
	require.InDelta(t, 52, calculateGatewayRefundAmount(100, 103, 50, "JPY"), 1e-12)
}

func TestFormatGatewayRefundAmountUsesOrderCurrency(t *testing.T) {
	order := &dbent.PaymentOrder{
		ProviderSnapshot: map[string]any{
			"currency": "KWD",
		},
	}

	require.Equal(t, "12.345", formatGatewayRefundAmount(12.345, order))
}

func TestValidateRefundProviderResponseAcceptsPending(t *testing.T) {
	require.NoError(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusPending}))
	require.NoError(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusSuccess}))
	require.Error(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusFailed}))
	require.Error(t, validateRefundProviderResponse(nil))
}

func TestFinishRefundPendingMarksOrderPendingAndRollsBackDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-pending@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-pending-user").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-PENDING-ORDER").
		SetOutTradeNo("sub2_refund_pending_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("pi_refund_pending").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefunding).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	var rolledBack float64
	userRepo := &mockUserRepo{}
	userRepo.adjustBalanceFn = func(ctx context.Context, id int64, amount float64) (BalanceChange, error) {
		require.Equal(t, user.ID, id)
		rolledBack += amount
		return BalanceChange{}, nil
	}
	svc := &PaymentService{
		entClient: client,
		userRepo:  userRepo,
	}
	plan := &RefundPlan{
		OrderID:         order.ID,
		Order:           order,
		RefundAmount:    40,
		GatewayAmount:   40,
		Reason:          "gateway accepted but not final",
		Force:           true,
		DeductBalance:   true,
		DeductionType:   payment.DeductionTypeBalance,
		BalanceToDeduct: 40,
	}

	result, err := svc.finishRefund(ctx, plan, &payment.RefundResponse{Status: payment.ProviderStatusPending})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Warning, "pending confirmation")
	require.Equal(t, 40.0, rolledBack)
	require.Zero(t, plan.BalanceToDeduct)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	require.Equal(t, 40.0, reloaded.RefundAmount)
	require.NotNil(t, reloaded.RefundReason)
	require.Equal(t, "gateway accepted but not final", *reloaded.RefundReason)
	require.Nil(t, reloaded.RefundAt)

	pendingAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Only(ctx)
	require.NoError(t, err)
	var pendingDetail map[string]any
	require.NoError(t, json.Unmarshal([]byte(pendingAudit.Detail), &pendingDetail))
	require.Equal(t, true, pendingDetail["deductBalance"])
	require.Equal(t, payment.DeductionTypeBalance, pendingDetail["deductionType"])
	require.Equal(t, 40.0, pendingDetail["balanceToDeduct"])
	require.Equal(t, true, pendingDetail["deductionRollbackOK"])
	successAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, successAudits)
}

func TestFinishRefundPendingRequiresAtomicStateAudit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("refund-pending-audit@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-pending-audit").
		SetBalance(60).
		SetTotalRecharged(100).
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(10).
		SetPayAmount(10).
		SetRechargeCode("REFUND-PENDING-AUDIT").
		SetOutTradeNo("sub2_refund_pending_audit").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("pi_refund_pending_audit").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefunding).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)
	exec, ok := client.Driver().(refundNotificationSQLExecutor)
	require.True(t, ok)
	_, err = exec.ExecContext(ctx, `
		CREATE TRIGGER fail_refund_pending_audit
		BEFORE INSERT ON payment_audit_logs
		WHEN NEW.action = 'REFUND_PENDING'
		BEGIN
			SELECT RAISE(FAIL, 'refund pending audit unavailable');
		END;
	`)
	require.NoError(t, err)

	userRepo := &mockUserRepo{adjustBalanceFn: func(callCtx context.Context, id int64, delta float64) (BalanceChange, error) {
		callClient := client
		if tx := dbent.TxFromContext(callCtx); tx != nil {
			callClient = tx.Client()
		}
		before, getErr := callClient.User.Get(callCtx, id)
		if getErr != nil {
			return BalanceChange{}, getErr
		}
		after, updateErr := callClient.User.UpdateOneID(id).AddBalance(delta).Save(callCtx)
		if updateErr != nil {
			return BalanceChange{}, updateErr
		}
		return BalanceChange{Old: before.Balance, New: after.Balance}, nil
	}}
	svc := &PaymentService{entClient: client, userRepo: userRepo}
	plan := &RefundPlan{
		OrderID: order.ID, Order: order, RefundAmount: 10, GatewayAmount: 10,
		Reason: "pending audit failure", DeductBalance: true,
		DeductionType: payment.DeductionTypeBalance, BalanceToDeduct: 10,
	}
	result, err := svc.finishRefund(ctx, plan, &payment.RefundResponse{Status: payment.ProviderStatusPending})
	require.Nil(t, result)
	require.ErrorContains(t, err, "persist refund pending state")
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunding, reloaded.Status)
	reloadedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, 60.0, reloadedUser.Balance)
	require.Equal(t, 100.0, reloadedUser.TotalRecharged)
}

func TestFinishRefundSuccessStatusesFinalize(t *testing.T) {
	for _, status := range []string{payment.ProviderStatusSuccess, payment.ProviderStatusRefunded} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)

			user, err := client.User.Create().
				SetEmail("refund-success-" + status + "@example.com").
				SetPasswordHash("hash").
				SetUsername("refund-success-" + status).
				Save(ctx)
			require.NoError(t, err)

			order, err := client.PaymentOrder.Create().
				SetUserID(user.ID).
				SetUserEmail(user.Email).
				SetUserName(user.Username).
				SetAmount(100).
				SetPayAmount(100).
				SetFeeRate(0).
				SetRechargeCode("REFUND-SUCCESS-" + status).
				SetOutTradeNo("sub2_refund_success_" + status).
				SetPaymentType(payment.TypeStripe).
				SetPaymentTradeNo("pi_refund_success_" + status).
				SetOrderType(payment.OrderTypeBalance).
				SetStatus(OrderStatusRefunding).
				SetExpiresAt(time.Now().Add(time.Hour)).
				SetPaidAt(time.Now()).
				SetClientIP("127.0.0.1").
				SetSrcHost("api.example.com").
				Save(ctx)
			require.NoError(t, err)

			svc := &PaymentService{entClient: client}
			plan := &RefundPlan{
				OrderID:         order.ID,
				Order:           order,
				RefundAmount:    100,
				GatewayAmount:   100,
				Reason:          "final success",
				DeductionType:   payment.DeductionTypeBalance,
				BalanceToDeduct: 100,
			}

			result, err := svc.finishRefund(ctx, plan, &payment.RefundResponse{
				RefundID:               "rf_a5_123",
				RefundIDProviderIssued: true,
				Status:                 status,
				Message:                "退款成功",
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.Success)
			require.Equal(t, 100.0, result.BalanceDeducted)

			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusRefunded, reloaded.Status)
			require.NotNil(t, reloaded.RefundAt)

			successAudit, err := client.PaymentAuditLog.Query().
				Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
				Only(ctx)
			require.NoError(t, err)
			var detail map[string]any
			require.NoError(t, json.Unmarshal([]byte(successAudit.Detail), &detail))
			require.Equal(t, "rf_a5_123", detail["refundID"])
			require.Equal(t, "退款成功", detail["providerMessage"])
			pendingAudits, err := client.PaymentAuditLog.Query().
				Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
				Count(ctx)
			require.NoError(t, err)
			require.Zero(t, pendingAudits)
		})
	}
}

func TestQueryAndFinalizeRefundFinalizesProviderStatuses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		wantStatus string
		wantDeduct float64
	}{
		{name: "success", status: payment.ProviderStatusSuccess, wantStatus: OrderStatusRefunded, wantDeduct: 100},
		{name: "failed", status: payment.ProviderStatusFailed, wantStatus: OrderStatusRefundFailed},
		{name: "pending", status: payment.ProviderStatusPending, wantStatus: OrderStatusRefundPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-"+tc.name)

			var deducted float64
			svc := &PaymentService{
				entClient:    client,
				loadBalancer: &captureLoadBalancer{},
				userRepo: &mockUserRepo{deductBalanceFn: func(ctx context.Context, id int64, amount float64) error {
					deducted += amount
					return nil
				}},
			}
			restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
				refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: tc.status},
			})
			defer restore()

			result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tc.status == payment.ProviderStatusSuccess, result.Success)
			require.Equal(t, tc.wantDeduct, deducted)

			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, reloaded.Status)
		})
	}
}

func TestQueryAndFinalizeRefundUnsupportedProviderReturnsClearError(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-unsupported")
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}
	restore := replacePaymentProviderFactoryForTest(t, refundProviderTestDouble{})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_QUERY_UNSUPPORTED", infraerrors.Reason(err))
}

func TestQueryAndFinalizeRefundHonorsPersistedDeductionState(t *testing.T) {
	for index, tc := range []struct {
		name   string
		detail string
	}{
		{
			name:   "administrator disabled balance deduction",
			detail: `{"refundID":"rf_no_deduct","deductBalance":false,"deductionType":"none","balanceToDeduct":0,"subDaysToDeduct":0,"subscriptionID":0,"deductionRollbackOK":true}`,
		},
		{
			name:   "initial deduction rollback failed",
			detail: `{"refundID":"rf_rollback_failed","deductBalance":true,"deductionType":"balance","balanceToDeduct":100,"subDaysToDeduct":0,"subscriptionID":0,"deductionRollbackOK":false}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPendingRefundOrderForTest(t, ctx, client, fmt.Sprintf("persisted-deduction-%d", index))
			_, err := client.PaymentAuditLog.Update().
				Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
				SetDetail(tc.detail).
				Save(ctx)
			require.NoError(t, err)

			var deducted float64
			svc := &PaymentService{
				entClient:    client,
				loadBalancer: &captureLoadBalancer{},
				userRepo: &mockUserRepo{deductBalanceFn: func(context.Context, int64, float64) error {
					deducted++
					return nil
				}},
			}
			restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
				refundResponse: &payment.RefundResponse{RefundID: "rf_final", Status: payment.ProviderStatusRefunded},
			})
			defer restore()

			result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.NoError(t, err)
			require.True(t, result.Success)
			require.Zero(t, deducted)
		})
	}
}

func TestQueryAndFinalizeRefundSupportsTS3PendingAudit(t *testing.T) {
	for index, tc := range []struct {
		name       string
		detail     string
		wantDeduct float64
	}{
		{
			name:       "successful balance rollback is deducted once after confirmation",
			detail:     `{"refundID":"rf_legacy_ok","refundAmount":100,"balanceDeducted":0,"subDaysDeducted":0,"balanceRolledBack":37,"subDaysRolledBack":0,"deductionRollbackOK":true}`,
			wantDeduct: 37,
		},
		{
			name:       "failed balance rollback is not deducted again",
			detail:     `{"refundID":"rf_legacy_failed","refundAmount":100,"balanceDeducted":37,"subDaysDeducted":0,"balanceRolledBack":37,"subDaysRolledBack":0,"deductionRollbackOK":false}`,
			wantDeduct: 0,
		},
		{
			name:       "administrator selected no deduction",
			detail:     `{"refundID":"rf_legacy_none","refundAmount":100,"balanceDeducted":0,"subDaysDeducted":0,"balanceRolledBack":0,"subDaysRolledBack":0,"deductionRollbackOK":true}`,
			wantDeduct: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPendingRefundOrderForTest(t, ctx, client, fmt.Sprintf("legacy-pending-%d", index))
			_, err := client.PaymentAuditLog.Update().
				Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
				SetDetail(tc.detail).
				Save(ctx)
			require.NoError(t, err)

			var deducted float64
			userRepo := &mockUserRepo{deductBalanceFn: func(_ context.Context, _ int64, amount float64) error {
				deducted += amount
				return nil
			}}
			svc := &PaymentService{
				entClient:    client,
				loadBalancer: &captureLoadBalancer{},
				userRepo:     userRepo,
			}
			restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
				refundResponse: &payment.RefundResponse{RefundID: "rf_legacy_final", Status: payment.ProviderStatusRefunded},
			})
			defer restore()

			result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.NoError(t, err)
			require.True(t, result.Success)
			require.Equal(t, tc.wantDeduct, deducted)
		})
	}
}

func TestQueryAndFinalizeRefundFailsClosedWithoutValidPendingState(t *testing.T) {
	for index, tc := range []struct {
		name   string
		detail *string
	}{
		{name: "missing audit"},
		{name: "malformed audit", detail: func() *string { value := `{"deductionRollbackOK":true}`; return &value }()},
		{name: "incomplete normalized audit", detail: func() *string {
			value := `{"deductBalance":true,"deductionType":"balance","balanceToDeduct":100,"subDaysToDeduct":0,"balanceRolledBack":100,"subDaysRolledBack":0,"deductionRollbackOK":true}`
			return &value
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPendingRefundOrderForTest(t, ctx, client, fmt.Sprintf("invalid-pending-%d", index))
			query := client.PaymentAuditLog.Query().Where(
				paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)),
				paymentauditlog.ActionEQ("REFUND_PENDING"),
			)
			entry, err := query.Only(ctx)
			require.NoError(t, err)
			if tc.detail == nil {
				require.NoError(t, client.PaymentAuditLog.DeleteOne(entry).Exec(ctx))
			} else {
				_, err = client.PaymentAuditLog.UpdateOne(entry).SetDetail(*tc.detail).Save(ctx)
				require.NoError(t, err)
			}

			svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}
			result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.Nil(t, result)
			require.Error(t, err)
			require.Equal(t, "REFUND_PENDING_STATE_INVALID", infraerrors.Reason(err))
			reloaded, reloadErr := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, reloadErr)
			require.Equal(t, OrderStatusRefundPending, reloaded.Status)
		})
	}
}

func TestQueryAndFinalizeRefundPendingAuditIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-pending-idempotent")
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_pending", Status: payment.ProviderStatusPending, Message: "processing"},
	})
	defer restore()

	for range 2 {
		result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.Success)
	}
	count, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_QUERY_PENDING")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestFinalizeQueriedRefundClaimsPendingOrderOnce(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-claim")
	var deducted float64
	svc := &PaymentService{
		entClient: client,
		userRepo: &mockUserRepo{deductBalanceFn: func(context.Context, int64, float64) error {
			deducted++
			return nil
		}},
	}
	resp := &payment.RefundResponse{RefundID: "rf_claim", Status: payment.ProviderStatusRefunded}

	pendingDetail, err := svc.latestRefundPendingDetail(ctx, order.ID)
	require.NoError(t, err)
	plan, err := svc.refundFinalizePlan(ctx, order, pendingDetail)
	require.NoError(t, err)
	result, err := svc.finalizeQueriedRefund(ctx, order, plan, resp)
	require.NoError(t, err)
	require.True(t, result.Success)
	result, err = svc.finalizeQueriedRefund(ctx, order, plan, resp)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_FINALIZE_CONFLICT", infraerrors.Reason(err))
	require.Equal(t, 1.0, deducted)
}

func TestFinalizeQueriedRefundRollsBackBalanceWhenAuditFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-audit-rollback")
	_, err := client.User.UpdateOneID(order.UserID).SetBalance(100).Save(ctx)
	require.NoError(t, err)
	exec, ok := client.Driver().(refundNotificationSQLExecutor)
	require.True(t, ok)
	_, err = exec.ExecContext(ctx, `
		CREATE TRIGGER fail_refund_success_audit
		BEFORE INSERT ON payment_audit_logs
		WHEN NEW.action = 'REFUND_SUCCESS'
		BEGIN
			SELECT RAISE(FAIL, 'refund success audit unavailable');
		END;
	`)
	require.NoError(t, err)

	userRepo := &mockUserRepo{deductBalanceFn: func(callCtx context.Context, id int64, amount float64) error {
		callClient := client
		if tx := dbent.TxFromContext(callCtx); tx != nil {
			callClient = tx.Client()
		}
		_, updateErr := callClient.User.UpdateOneID(id).AddBalance(-amount).Save(callCtx)
		return updateErr
	}}
	svc := &PaymentService{entClient: client, userRepo: userRepo}
	resp := &payment.RefundResponse{RefundID: "rf_rollback", RefundIDProviderIssued: true, Status: payment.ProviderStatusRefunded}

	pendingDetail, err := svc.latestRefundPendingDetail(ctx, order.ID)
	require.NoError(t, err)
	plan, err := svc.refundFinalizePlan(ctx, order, pendingDetail)
	require.NoError(t, err)
	result, err := svc.finalizeQueriedRefund(ctx, order, plan, resp)
	require.Nil(t, result)
	require.Error(t, err)
	reloadedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloadedOrder.Status)
	reloadedUser, err := client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 100.0, reloadedUser.Balance)
}

func createPendingRefundOrderForTest(t *testing.T, ctx context.Context, client *dbent.Client, suffix string) *dbent.PaymentOrder {
	t.Helper()

	user, err := client.User.Create().
		SetEmail(suffix + "@example.com").
		SetPasswordHash("hash").
		SetUsername(suffix).
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName(suffix + "-provider").
		SetConfig("{}").
		SetSupportedTypes("stripe").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-" + suffix).
		SetOutTradeNo("sub2_" + suffix).
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("pi_" + suffix).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefundPending).
		SetRefundAmount(100).
		SetRefundReason("pending refund").
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_PENDING").
		SetOperator("admin").
		SetDetail(`{"refundID":"rf_test","deductBalance":true,"deductionType":"balance","balanceToDeduct":100,"subDaysToDeduct":0,"subscriptionID":0,"deductionRollbackOK":true}`).
		Save(ctx)
	require.NoError(t, err)
	return order
}

func replacePaymentProviderFactoryForTest(t *testing.T, prov payment.Provider) func() {
	t.Helper()
	original := createPaymentProviderFromInstance
	createPaymentProviderFromInstance = func(providerKey, instanceID string, config map[string]string) (payment.Provider, error) {
		return prov, nil
	}
	return func() { createPaymentProviderFromInstance = original }
}

type refundProviderTestDouble struct{}

func (refundProviderTestDouble) Name() string { return "refund-test" }
func (refundProviderTestDouble) ProviderKey() string {
	return payment.TypeStripe
}
func (refundProviderTestDouble) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripe}
}
func (refundProviderTestDouble) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, nil
}
func (refundProviderTestDouble) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, nil
}
func (refundProviderTestDouble) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	return nil, nil
}
func (refundProviderTestDouble) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, nil
}

type refundCaptureProviderTestDouble struct {
	refundProviderTestDouble
	providerKey string
	request     payment.RefundRequest
}

func (p *refundCaptureProviderTestDouble) ProviderKey() string { return p.providerKey }
func (p *refundCaptureProviderTestDouble) Refund(_ context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	p.request = req
	return &payment.RefundResponse{Status: payment.ProviderStatusSuccess}, nil
}

type refundQueryProviderTestDouble struct {
	refundProviderTestDouble
	refundResponse *payment.RefundResponse
}

func (p *refundQueryProviderTestDouble) QueryRefund(context.Context, payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	return p.refundResponse, nil
}

type refundConfigLoadBalancer struct {
	captureLoadBalancer
	config map[string]string
}

func (l *refundConfigLoadBalancer) GetInstanceConfig(context.Context, int64) (map[string]string, error) {
	config := make(map[string]string, len(l.config))
	for key, value := range l.config {
		config[key] = value
	}
	return config, nil
}
