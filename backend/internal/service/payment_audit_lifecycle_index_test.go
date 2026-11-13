//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

// 迁移 131 在 payment_audit_logs 上建立了覆盖全部 action 的唯一索引
// (order_id, action)。退款收敛路径在事务内写审计（markRefundPending /
// finalizeRefundFailedTx / markRefundOkTx），重试退款时同一 (order_id, action)
// 会被再次写入，唯一约束让整个资金事务回滚：保留的余额/订阅天数不再归还，
// 订单卡在 REFUND_PENDING/REFUNDING，重试还可能二次扣减。
//
// 迁移 251 把唯一约束收窄到“一单只发生一次”的动作（订阅发放认领、返利发放幂等），
// 退款类事件恢复为按每次尝试追加。下列测试锁定该索引形态。

func createPaymentAuditLogForTest(t *testing.T, ctx context.Context, client *dbent.Client, orderID int64, action string) error {
	t.Helper()
	_, err := client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(orderID, 10)).
		SetAction(action).
		SetDetail(`{"attempt":true}`).
		SetOperator("admin").
		Save(ctx)
	return err
}

func TestPaymentAuditLifecycleIndexAllowsRepeatedRefundAttempts(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	const orderID int64 = 9001
	// 同一订单的每次退款尝试都会写这些审计；重试必须能继续追加。
	for _, action := range []string{
		"REFUND_PENDING",
		"REFUND_FAILED",
		"REFUND_ROLLBACK_FAILED",
		"REFUND_GATEWAY_FAILED",
		"REFUND_SUCCESS",
	} {
		for attempt := 1; attempt <= 2; attempt++ {
			require.NoErrorf(t, createPaymentAuditLogForTest(t, ctx, client, orderID, action),
				"%s 第 %d 次写入不应被唯一约束拦截", action, attempt)
		}
	}

	rows, err := client.PaymentAuditLog.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 10, "退款审计应保留每次尝试的记录")
}

func TestPaymentAuditLifecycleIndexStillEnforcesOncePerOrderActions(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	// 订阅发放认领依赖约束错误做幂等（payment_fulfillment.go 捕获 IsConstraintError）。
	require.NoError(t, createPaymentAuditLogForTest(t, ctx, client, 9002, "SUBSCRIPTION_ASSIGNED"))
	err := createPaymentAuditLogForTest(t, ctx, client, 9002, "SUBSCRIPTION_ASSIGNED")
	require.Error(t, err)
	require.True(t, dbent.IsConstraintError(err), "SUBSCRIPTION_ASSIGNED 仍必须是一单一次，实际错误: %v", err)

	// 返利发放同样只允许一次，且跨 action 的互斥由 claim 查询的 NOT EXISTS 保证。
	require.NoError(t, createPaymentAuditLogForTest(t, ctx, client, 9003, "AFFILIATE_REBATE_APPLIED"))
	err = createPaymentAuditLogForTest(t, ctx, client, 9003, "AFFILIATE_REBATE_APPLIED")
	require.Error(t, err)
	require.True(t, dbent.IsConstraintError(err), "AFFILIATE_REBATE_APPLIED 仍必须是一单一次，实际错误: %v", err)

	// 不同订单互不影响。
	require.NoError(t, createPaymentAuditLogForTest(t, ctx, client, 9004, "SUBSCRIPTION_ASSIGNED"))
}

// ON CONFLICT 的冲突目标必须带上与部分唯一索引一致的谓词，否则 PostgreSQL /
// SQLite 都推断不到索引并直接报错。这里通过真实 claim 查询锁定两者一致。
// 这是线上真实可达的资金漏洞路径：REFUND_FAILED 允许管理员重试退款
// （PrepareRefund 的状态白名单包含 OrderStatusRefundFailed）。第一次退款挂起后查单失败，
// 第二次重试网关再次挂起时，旧的全量唯一索引让 REFUND_PENDING 写入失败并回滚事务：
// 订单停在 REFUND_FAILED、既没有 pending 记录也无法再收敛，而余额已经扣走。
func TestRefundRetryAfterGatewayFailureWritesSecondPendingAudit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	order := createCompletedRefundableOrderForTest(t, ctx, client, "retry-pending")

	wallet := &refundHoldWallet{balance: 100}
	prov := &pendingRefundProviderTestDouble{queryStatus: payment.ProviderStatusFailed}
	restore := replacePaymentProviderFactoryForTest(t, prov)
	defer restore()
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo:     wallet.repo(order.UserID),
	}

	// 第一次退款：网关挂起 → REFUND_PENDING，随后查单失败 → REFUND_FAILED 并归还扣减。
	plan, early, err := svc.PrepareRefund(ctx, order.ID, 0, "first", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	result, err := svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.False(t, result.Success)

	_, err = svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundFailed, reloaded.Status)
	require.Equal(t, 100.0, wallet.balance, "查单失败后保留的扣减必须归还")
	require.Equal(t, 1, countRefundAuditsForTest(t, ctx, client, order.ID, "REFUND_PENDING"))

	// 第二次退款（重试）：网关再次挂起。旧唯一索引在这里报约束错误并回滚整个事务。
	plan, early, err = svc.PrepareRefund(ctx, order.ID, 0, "retry", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	result, err = svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err, "退款重试写 REFUND_PENDING 不能被审计唯一约束拦截")
	require.False(t, result.Success)
	require.Contains(t, result.Warning, "pending confirmation")

	reloaded, err = client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	require.Equal(t, 2, countRefundAuditsForTest(t, ctx, client, order.ID, "REFUND_PENDING"))
	require.Equal(t, 2, wallet.deductCalls)
	require.Zero(t, wallet.balance, "pending 期间第二次扣减同样必须保留")
}

func TestAffiliateRebateAuditClaimQueryMatchesPartialIndex(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	svc := &PaymentService{entClient: client}

	claimed, err := svc.tryClaimAffiliateRebateAudit(ctx, client, 9005, 100)
	require.NoError(t, err)
	require.True(t, claimed, "首次申请返利应成功占位")

	claimed, err = svc.tryClaimAffiliateRebateAudit(ctx, client, 9005, 100)
	require.NoError(t, err)
	require.False(t, claimed, "重复申请返利必须被幂等拦截，而不是报错")
}
