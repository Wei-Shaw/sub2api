//go:build unit

package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// =========================
// M8: 充值兑换码熵
// =========================

// TestGenerateRechargeCodeFormatAndEntropy 钉住 M8 的修复：
// 兑换码不再由订单 id + 5 位时间戳后缀拼出，而是 130 bit CSPRNG。
func TestGenerateRechargeCodeFormatAndEntropy(t *testing.T) {
	t.Parallel()

	code, err := generateRechargeCode()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(code, rechargeCodePrefix))
	require.Len(t, code, len(rechargeCodePrefix)+rechargeCodeRandomLen)
	// redeem_codes.code 是 VARCHAR(32)，超长会在兑换时被数据库拒绝。
	require.LessOrEqual(t, len(code), 32)
	for _, c := range []byte(code[len(rechargeCodePrefix):]) {
		require.Contains(t, string(rechargeCodeCharset), string(c))
	}
	// 130 bit：单条码不可枚举。旧格式只有约 17 bit。
	require.Greater(t, rechargeCodeRandomLen*5, 128)
}

// TestGenerateRechargeCodeIsUniqueUnderConcurrency 并发生成不得撞码，
// 也不得复现旧实现那种「同一纳秒截断成同一后缀」的可预测性。
func TestGenerateRechargeCodeIsUniqueUnderConcurrency(t *testing.T) {
	t.Parallel()

	const (
		goroutines     = 16
		codesPerWorker = 256
	)
	results := make([][]string, goroutines)
	errs := make([]error, goroutines)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			local := make([]string, 0, codesPerWorker)
			<-start
			for j := 0; j < codesPerWorker; j++ {
				code, err := generateRechargeCode()
				if err != nil {
					errs[idx] = err
					return
				}
				local = append(local, code)
			}
			results[idx] = local
		}(i)
	}
	close(start)
	wg.Wait()

	seen := make(map[string]struct{}, goroutines*codesPerWorker)
	for i := 0; i < goroutines; i++ {
		require.NoError(t, errs[i])
		for _, code := range results[i] {
			_, dup := seen[code]
			require.False(t, dup, "duplicate recharge code %q", code)
			seen[code] = struct{}{}
		}
	}
	require.Len(t, seen, goroutines*codesPerWorker)
}

// TestCreateOrderInTxRechargeCodeDoesNotLeakOrderID 建单写入的兑换码里不得再出现
// 订单 id —— 旧格式 "PAY-<orderID>-<5位>" 让攻击者可以直接构造受害者的候选码。
func TestCreateOrderInTxRechargeCodeDoesNotLeakOrderID(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("recharge-code@example.com").
		SetPasswordHash("hash").
		SetUsername("recharge-code-user").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	codes := make(map[string]struct{}, 3)
	legacyRechargeCodePattern := regexp.MustCompile(`^PAY-\d+-\d+$`)
	for i := 0; i < 3; i++ {
		order, err := svc.createOrderInTx(
			ctx,
			CreateOrderRequest{
				UserID:      user.ID,
				PaymentType: payment.TypeSePayBankTransfer,
				OrderType:   payment.OrderTypeBalance,
			},
			&User{ID: user.ID, Email: user.Email, Username: user.Username},
			nil,
			&PaymentConfig{MaxPendingOrders: 10, OrderTimeoutMin: 30},
			10, 10, 0, 10,
			nil,
		)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(order.RechargeCode, rechargeCodePrefix))
		// 旧格式 "PAY-<orderID>-<5位数字>" 可以被订单 id 直接推出来；新格式不再匹配它。
		require.NotRegexp(t, legacyRechargeCodePattern, order.RechargeCode)
		_, dup := codes[order.RechargeCode]
		require.False(t, dup)
		codes[order.RechargeCode] = struct{}{}
	}
}

// =========================
// M13: 建单限额的并发正确性
// =========================

type capturePaymentQueryMatcher struct {
	queries *[]string
}

func (m capturePaymentQueryMatcher) Match(_, actual string) error {
	*m.queries = append(*m.queries, actual)
	return nil
}

// TestLockUserRowForOrderLimitsEmitsRowLockOnPostgres 钉住 M13 的锁点：
// READ COMMITTED 下「聚合读 -> 判断 -> 插入订单」必须先拿到用户行锁才能串行化，
// 否则 N 个并发请求各自读到同一个旧值后一起通过。
func TestLockUserRowForOrderLimitsEmitsRowLockOnPostgres(t *testing.T) {
	var captured []string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(capturePaymentQueryMatcher{queries: &captured}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("lock user row").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectRollback()

	tx, err := client.Tx(context.Background())
	require.NoError(t, err)
	require.NoError(t, lockUserRowForOrderLimits(context.Background(), tx, 42))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, captured, 1)
	locked := strings.ToUpper(strings.Join(strings.Fields(captured[0]), " "))
	require.Contains(t, locked, "FROM USERS WHERE ID = $1")
	require.Contains(t, locked, "FOR UPDATE")
}

// TestLockUserRowForOrderLimitsSkipsNonPostgres SQLite（单元测试内存库）不支持
// FOR UPDATE，必须静默跳过而不是抛语法错误。
func TestLockUserRowForOrderLimitsSkipsNonPostgres(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	require.NoError(t, lockUserRowForOrderLimits(ctx, tx, 1))
}

// TestCheckDailyLimitAggregatesInDatabase 新的库内聚合必须和旧的「读全部订单再在 Go
// 里累加」结果一致：余额订单按实付金额计入，订阅订单按订单金额计入。
// 已过期的挂起订单不计入（它不会再被履约，继续占额度就成了永久封锁）。
func TestCheckDailyLimitAggregatesInDatabase(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	paidAt := time.Now().UTC()

	user, err := client.User.Create().
		SetEmail("daily-limit@example.com").
		SetPasswordHash("hash").
		SetUsername("daily-limit-user").
		Save(ctx)
	require.NoError(t, err)

	// 余额订单：amount=100（到账）但 pay_amount=30（实付），只应计入 30。
	mustCreatePaidPaymentOrder(t, ctx, client, user.ID, payment.OrderTypeBalance, 100, 30, OrderStatusCompleted, paidAt)
	// 订阅订单：按 amount=20 计入。
	mustCreatePaidPaymentOrder(t, ctx, client, user.ID, payment.OrderTypeSubscription, 20, 19, OrderStatusPaid, paidAt)
	// 已过期的挂起订单不计入。
	mustCreatePaymentOrderWithExpiry(t, ctx, client, user.ID, payment.OrderTypeBalance, 500, 500,
		OrderStatusPending, time.Time{}, time.Now().Add(-time.Minute))

	svc := &PaymentService{entClient: client}

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	// used = 30 + 20 = 50，加上本次 10 没有超过 60。
	require.NoError(t, svc.checkDailyLimit(ctx, tx, user.ID, 10, 60))
	require.NoError(t, tx.Rollback())

	tx, err = client.Tx(ctx)
	require.NoError(t, err)
	err = svc.checkDailyLimit(ctx, tx, user.ID, 11, 60)
	require.Error(t, err)
	require.Contains(t, err.Error(), "daily_limit_exceeded")
	require.NoError(t, tx.Rollback())

	// 没有任何已支付订单的用户：SUM 聚合返回 NULL，必须当成 0 处理。
	other, err := client.User.Create().
		SetEmail("daily-limit-empty@example.com").
		SetPasswordHash("hash").
		SetUsername("daily-limit-empty").
		Save(ctx)
	require.NoError(t, err)
	tx, err = client.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.checkDailyLimit(ctx, tx, other.ID, 59, 60))
	require.NoError(t, tx.Rollback())
}

var dailyLimitOrderSeq int64

func mustCreatePaidPaymentOrder(
	t *testing.T,
	ctx context.Context,
	client *dbent.Client,
	userID int64,
	orderType string,
	amount, payAmount float64,
	status string,
	paidAt time.Time,
) {
	t.Helper()
	mustCreatePaymentOrderWithExpiry(t, ctx, client, userID, orderType, amount, payAmount, status, paidAt,
		time.Now().Add(time.Hour))
}

func mustCreatePaymentOrderWithExpiry(
	t *testing.T,
	ctx context.Context,
	client *dbent.Client,
	userID int64,
	orderType string,
	amount, payAmount float64,
	status string,
	paidAt time.Time,
	expiresAt time.Time,
) {
	t.Helper()
	code, err := generateRechargeCode()
	require.NoError(t, err)
	b := client.PaymentOrder.Create().
		SetUserID(userID).
		SetUserEmail("daily@example.com").
		SetUserName("daily").
		SetAmount(amount).
		SetPayAmount(payAmount).
		SetFeeRate(0).
		SetRechargeCode(code).
		SetOutTradeNo(fmt.Sprintf("sub2_daily_%d_%d", userID, atomic.AddInt64(&dailyLimitOrderSeq, 1))).
		SetPaymentType(payment.TypeSePayBankTransfer).
		SetPaymentTradeNo("").
		SetOrderType(orderType).
		SetStatus(status).
		SetClientIP("127.0.0.1").
		SetSrcHost("app.example.com").
		SetExpiresAt(expiresAt)
	if !paidAt.IsZero() {
		b = b.SetPaidAt(paidAt)
	}
	_, err = b.Save(ctx)
	require.NoError(t, err)
}

// TestCheckDailyLimitCountsUnexpiredPendingOrders 钉住 M13 的收口：
// 日限额原本只统计 paid/recharging/completed，用户可以连开数个挂起订单
// （每个建单时都读到 used=0）再一次性付掉，实际入账远超日限额。
// 未过期的挂起订单现在预占额度，并且要给出「谁占着、什么时候释放」的错误。
func TestCheckDailyLimitCountsUnexpiredPendingOrders(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("daily-limit-pending@example.com").
		SetPasswordHash("hash").
		SetUsername("daily-limit-pending-user").
		Save(ctx)
	require.NoError(t, err)

	expiresAt := time.Now().Add(20 * time.Minute).UTC().Truncate(time.Second)
	mustCreatePaymentOrderWithExpiry(t, ctx, client, user.ID, payment.OrderTypeBalance, 60, 60,
		OrderStatusPending, time.Time{}, expiresAt)

	svc := &PaymentService{entClient: client}

	// 60（挂起预占）+ 30 <= 100：还放得下。
	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.checkDailyLimit(ctx, tx, user.ID, 30, 100))
	require.NoError(t, tx.Rollback())

	// 60（挂起预占）+ 50 > 100：必须拒绝，且不是「已支付超限」那种笼统提示。
	tx, err = client.Tx(ctx)
	require.NoError(t, err)
	err = svc.checkDailyLimit(ctx, tx, user.ID, 50, 100)
	require.NoError(t, tx.Rollback())
	require.Error(t, err)
	require.Equal(t, "DAILY_LIMIT_PENDING_HOLD", infraerrors.Reason(err))
	var appErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	meta := appErr.Metadata
	require.Equal(t, "60.00", meta["held"])
	require.Equal(t, "40.00", meta["remaining"])
	require.Equal(t, expiresAt.Format(time.RFC3339), meta["held_until"])
	require.NotEmpty(t, meta["retry_after_seconds"])
	require.Contains(t, infraerrors.Message(err), "pending orders are holding")
}

// TestCheckDailyLimitIgnoresExpiredPendingOrders 预占必须是自愈的：
// 订单一旦过了 OrderTimeoutMin 就不会再被履约，占用的额度必须立刻释放，
// 否则一个被遗弃的订单会把用户的日额度永久锁死。
func TestCheckDailyLimitIgnoresExpiredPendingOrders(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("daily-limit-expired-pending@example.com").
		SetPasswordHash("hash").
		SetUsername("daily-limit-expired-pending-user").
		Save(ctx)
	require.NoError(t, err)

	mustCreatePaymentOrderWithExpiry(t, ctx, client, user.ID, payment.OrderTypeBalance, 90, 90,
		OrderStatusPending, time.Time{}, time.Now().Add(-time.Second))

	svc := &PaymentService{entClient: client}
	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.checkDailyLimit(ctx, tx, user.ID, 100, 100))
	require.NoError(t, tx.Rollback())
}

// TestCreateOrderInTxPendingOrdersConsumeDailyLimit 端到端复现 M13 的绕过：
// 日限额 100，连下两笔 60 的订单。修复前两笔都建得出来（各自读到 used=0），
// 付掉就是 120；修复后第二笔被 DAILY_LIMIT_PENDING_HOLD 拒绝，
// 而第一笔过期之后额度又能重新用起来。
func TestCreateOrderInTxPendingOrdersConsumeDailyLimit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("daily-limit-createorder@example.com").
		SetPasswordHash("hash").
		SetUsername("daily-limit-createorder-user").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	cfg := &PaymentConfig{MaxPendingOrders: 10, OrderTimeoutMin: 30, DailyLimit: 100}
	req := CreateOrderRequest{
		UserID:      user.ID,
		PaymentType: payment.TypeSePayBankTransfer,
		OrderType:   payment.OrderTypeBalance,
	}
	svcUser := &User{ID: user.ID, Email: user.Email, Username: user.Username}

	first, err := svc.createOrderInTx(ctx, req, svcUser, nil, cfg, 60, 60, 0, 60, nil)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, first.Status)

	_, err = svc.createOrderInTx(ctx, req, svcUser, nil, cfg, 60, 60, 0, 60, nil)
	require.Error(t, err)
	require.Equal(t, "DAILY_LIMIT_PENDING_HOLD", infraerrors.Reason(err))

	// 让第一笔过期：占用的额度必须自动释放。
	_, err = client.PaymentOrder.UpdateOneID(first.ID).
		SetExpiresAt(time.Now().Add(-time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	third, err := svc.createOrderInTx(ctx, req, svcUser, nil, cfg, 60, 60, 0, 60, nil)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, third.Status)
}
