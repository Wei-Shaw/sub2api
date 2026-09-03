//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestPendingOrderDailyLimitSerializesUnderUserRowLock 钉住 M13 的并发正确性。
//
// service.PaymentService.checkDailyLimit 现在把「未过期的挂起订单」也算进日限额，
// 判定序列是：锁 users 行 -> 聚合已支付 + 未过期挂起 -> 插入订单。这个测试在真实
// PostgreSQL 上并发跑同一序列（checkDailyLimit 不可导出，这里复刻它依赖的 SQL 形状），
// 验证行锁之后的读能看到对方已提交的挂起订单，因此两笔 60 只有一笔能在 100 的限额里落地。
//
// 去掉 "SELECT 1 FROM users ... FOR UPDATE" 这一行，两个事务会各自读到 0 并双双插入。
func TestPendingOrderDailyLimitSerializesUnderUserRowLock(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	user := mustCreateUser(t, client, &service.User{
		Email: fmt.Sprintf("pending-limit-%d@example.com", time.Now().UnixNano()),
	})

	const (
		limit       = 100.0
		orderAmount = 60.0
		workers     = 2
	)

	start := make(chan struct{})
	results := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx] = createOrderIfPendingHeadroomAllows(ctx, client, user.ID, orderAmount, limit, idx)
		}(i)
	}
	close(start)
	wg.Wait()

	created := 0
	for _, err := range results {
		if err == nil {
			created++
			continue
		}
		require.ErrorIs(t, err, errPendingDailyLimitHeld)
	}
	require.Equal(t, 1, created, "两笔 60 的订单在 100 的日限额下只应落地一笔")

	persisted, err := client.PaymentOrder.Query().
		Where(paymentorder.UserIDEQ(user.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, persisted)
}

var errPendingDailyLimitHeld = fmt.Errorf("daily limit held by pending orders")

// createOrderIfPendingHeadroomAllows 复刻 checkDailyLimit 的判定序列：
// 先锁 users 行，再聚合「未过期的挂起订单」，最后插入订单。
func createOrderIfPendingHeadroomAllows(
	ctx context.Context,
	client *dbent.Client,
	userID int64,
	amount, limit float64,
	seq int,
) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.Client().QueryContext(ctx, "SELECT 1 FROM users WHERE id = $1 FOR UPDATE", userID)
	if err != nil {
		return err
	}
	for rows.Next() { //nolint:revive // 只为持锁
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	var agg []struct {
		Sum float64 `json:"sum"`
	}
	if err := tx.PaymentOrder.Query().
		Where(
			paymentorder.UserIDEQ(userID),
			paymentorder.StatusEQ(service.OrderStatusPending),
			paymentorder.ExpiresAtGT(time.Now()),
			paymentorder.OrderTypeEQ(payment.OrderTypeBalance),
		).
		Aggregate(dbent.As(dbent.Sum(paymentorder.FieldPayAmount), "sum")).
		Scan(ctx, &agg); err != nil {
		return err
	}
	held := 0.0
	if len(agg) > 0 {
		held = agg[0].Sum
	}
	if held+amount > limit {
		return errPendingDailyLimitHeld
	}

	if _, err := tx.PaymentOrder.Create().
		SetUserID(userID).
		SetUserEmail("pending-limit@example.com").
		SetUserName("pending-limit").
		SetAmount(amount).
		SetPayAmount(amount).
		SetFeeRate(0).
		SetRechargeCode(fmt.Sprintf("PAY-PENDLIMIT-%d-%d", time.Now().UnixNano(), seq)).
		SetOutTradeNo(fmt.Sprintf("sub2_pendlimit_%d_%d", time.Now().UnixNano(), seq)).
		SetPaymentType(payment.TypeSePayBankTransfer).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(service.OrderStatusPending).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		SetClientIP("127.0.0.1").
		SetSrcHost("app.example.com").
		Save(ctx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
