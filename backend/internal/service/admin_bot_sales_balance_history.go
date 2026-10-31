package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// The fulfillment is the credit event. A payment order may be missing for older
// successful transfers, so the left join only enriches its timestamp and ID.
func (s *adminServiceImpl) listBotSalesBalanceHistory(ctx context.Context, userID int64, params pagination.PaginationParams) ([]RedeemCode, int64, error) {
	if s == nil || s.entClient == nil || userID <= 0 {
		return nil, 0, nil
	}

	rows, err := s.entClient.QueryContext(ctx, `
SELECT f.id, f.external_order_code, f.external_order_item_id,
       f.balance_amount::double precision,
       COALESCE(o.completed_at, f.updated_at), f.created_at, o.id
FROM bot_sales_fulfillments f
LEFT JOIN payment_orders o ON o.id = f.payment_order_id AND o.user_id = f.user_id
WHERE f.user_id = $1 AND f.status = 'succeeded'
ORDER BY COALESCE(o.completed_at, f.updated_at) DESC, f.id DESC
OFFSET $2
LIMIT $3`, userID, params.Offset(), params.Limit())
	if err != nil {
		return nil, 0, fmt.Errorf("list bot-sales balance history: %w", err)
	}
	codes := make([]RedeemCode, 0, params.Limit())
	for rows.Next() {
		var id int64
		var orderCode, itemID string
		var amount float64
		var usedAt, createdAt time.Time
		var paymentOrderID sql.NullInt64
		if err := rows.Scan(&id, &orderCode, &itemID, &amount, &usedAt, &createdAt, &paymentOrderID); err != nil {
			_ = rows.Close()
			return nil, 0, fmt.Errorf("scan bot-sales balance history: %w", err)
		}
		code := RedeemCode{
			ID:                  -1_000_000_000_000 - id, // Synthetic history ID; stays in JavaScript's safe integer range.
			Code:                orderCode,
			ExternalOrderItemID: itemID,
			Type:                RedeemTypeBotSalesBalance,
			Value:               amount,
			Status:              StatusUsed,
			UsedBy:              &userID,
			UsedAt:              &usedAt,
			CreatedAt:           createdAt,
		}
		if paymentOrderID.Valid {
			code.PaymentOrderID = &paymentOrderID.Int64
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, fmt.Errorf("iterate bot-sales balance history: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, 0, fmt.Errorf("close bot-sales balance history: %w", err)
	}

	countRows, err := s.entClient.QueryContext(ctx, `
SELECT COUNT(*) FROM bot_sales_fulfillments
WHERE user_id = $1 AND status = 'succeeded'`, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("count bot-sales balance history: %w", err)
	}
	defer func() { _ = countRows.Close() }()
	var count int64
	if countRows.Next() {
		if err := countRows.Scan(&count); err != nil {
			return nil, 0, fmt.Errorf("scan bot-sales balance history count: %w", err)
		}
	}
	if err := countRows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate bot-sales balance history count: %w", err)
	}
	return codes, count, nil
}

func (s *adminServiceImpl) listBotSalesBalanceHistoryForMerge(ctx context.Context, userID int64, needed int) ([]RedeemCode, int64, error) {
	if needed <= 0 {
		return nil, 0, nil
	}
	var out []RedeemCode
	var total int64
	for page := 1; len(out) < needed; page++ {
		params := pagination.PaginationParams{Page: page, PageSize: 1000}
		codes, count, err := s.listBotSalesBalanceHistory(ctx, userID, params)
		if err != nil {
			return nil, 0, err
		}
		total = count
		out = append(out, codes...)
		if len(codes) < params.Limit() || int64(len(out)) >= total {
			break
		}
	}
	if len(out) > needed {
		out = out[:needed]
	}
	return out, total, nil
}

func (s *adminServiceImpl) sumBotSalesBalanceHistory(ctx context.Context, userID int64) (float64, error) {
	if s == nil || s.entClient == nil || userID <= 0 {
		return 0, nil
	}
	rows, err := s.entClient.QueryContext(ctx, `
SELECT COALESCE(SUM(balance_amount), 0)::double precision
FROM bot_sales_fulfillments
WHERE user_id = $1 AND status = 'succeeded' AND balance_amount > 0`, userID)
	if err != nil {
		return 0, fmt.Errorf("sum bot-sales balance history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var total float64
	if rows.Next() {
		if err := rows.Scan(&total); err != nil {
			return 0, fmt.Errorf("scan bot-sales balance history sum: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate bot-sales balance history sum: %w", err)
	}
	return total, nil
}

func (s *adminServiceImpl) totalRechargedWithBotSales(ctx context.Context, userID int64) (float64, error) {
	redeemTotal, err := s.redeemCodeRepo.SumPositiveBalanceByUser(ctx, userID)
	if err != nil {
		return 0, err
	}
	botSalesTotal, err := s.sumBotSalesBalanceHistory(ctx, userID)
	if err != nil {
		return 0, err
	}
	return redeemTotal + botSalesTotal, nil
}
