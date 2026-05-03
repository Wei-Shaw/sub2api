package service

import (
	"context"
	"fmt"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
)

// GetOrder returns an order owned by userID.
func (s *PaymentService) GetOrder(ctx context.Context, orderID, userID int64) (*pluginent.PaymentOrder, error) {
	if s == nil || s.entClient == nil {
		return nil, errPaymentServiceUnavailable
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, int(orderID))
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
	}
	return o, nil
}

// GetOrderByID returns an order regardless of owner (admin view).
func (s *PaymentService) GetOrderByID(ctx context.Context, orderID int64) (*pluginent.PaymentOrder, error) {
	if s == nil || s.entClient == nil {
		return nil, errPaymentServiceUnavailable
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, int(orderID))
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	return o, nil
}

// GetUserOrders returns the authenticated user's orders, paginated.
func (s *PaymentService) GetUserOrders(ctx context.Context, userID int64, p OrderListParams) ([]*pluginent.PaymentOrder, int, error) {
	if s == nil || s.entClient == nil {
		return nil, 0, nil
	}
	q := s.entClient.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID))
	q = applyOrderListFilters(q, p)
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count user orders: %w", err)
	}
	ps, pg := applyPagination(p.PageSize, p.Page)
	orders, err := q.Order(pluginent.Desc(paymentorder.FieldCreatedAt)).
		Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query user orders: %w", err)
	}
	return orders, total, nil
}

// AdminListOrders returns a paginated list of orders. If userID > 0, filters by user.
func (s *PaymentService) AdminListOrders(ctx context.Context, userID int64, p OrderListParams) ([]*pluginent.PaymentOrder, int, error) {
	if s == nil || s.entClient == nil {
		return nil, 0, nil
	}
	q := s.entClient.PaymentOrder.Query()
	if userID > 0 {
		q = q.Where(paymentorder.UserIDEQ(userID))
	}
	q = applyOrderListFilters(q, p)
	if p.Keyword != "" {
		q = q.Where(paymentorder.Or(
			paymentorder.OutTradeNoContainsFold(p.Keyword),
			paymentorder.UserEmailContainsFold(p.Keyword),
			paymentorder.UserNameContainsFold(p.Keyword),
		))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count admin orders: %w", err)
	}
	ps, pg := applyPagination(p.PageSize, p.Page)
	orders, err := q.Order(pluginent.Desc(paymentorder.FieldCreatedAt)).
		Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin orders: %w", err)
	}
	return orders, total, nil
}

// applyOrderListFilters narrows the query by status / order_type /
// payment_type. Each filter is independent — empty values are skipped.
func applyOrderListFilters(q *pluginent.PaymentOrderQuery, p OrderListParams) *pluginent.PaymentOrderQuery {
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
	}
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
	}
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
	}
	return q
}
