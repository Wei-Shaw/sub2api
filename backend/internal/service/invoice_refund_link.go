package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OnOrderRefundedForInvoices reconciles invoice requests when a payment order is refunded.
//
// Behaviour by invoice request status:
//   - pending  : detach the order; recompute total; if no orders left, hard-delete the request.
//   - completed: flag has_refunded_orders=true; admin must manually void/reissue.
//   - rejected : ignore (the order is already disqualified for invoicing).
//
// Failures are logged but do not abort the refund — invoice bookkeeping is best-effort.
// Caller should pass the canonical orderID after the order has been marked refunded.
func (s *PaymentService) OnOrderRefundedForInvoices(ctx context.Context, orderID int64) {
	if orderID <= 0 {
		return
	}
	db, err := s.invoiceDB()
	if err != nil {
		slog.Warn("invoice refund hook: db unavailable", "order_id", orderID, "error", err)
		return
	}

	rows, err := db.QueryContext(ctx, `
		SELECT ir.id, ir.user_id, ir.serial_no, ir.status
		FROM invoice_request_orders iro
		JOIN invoice_requests ir ON ir.id = iro.invoice_request_id
		WHERE iro.payment_order_id = $1
	`, orderID)
	if err != nil {
		slog.Warn("invoice refund hook: failed to load invoice requests", "order_id", orderID, "error", err)
		return
	}
	type linked struct {
		ReqID    int64
		UserID   int64
		SerialNo string
		Status   string
	}
	var hits []linked
	for rows.Next() {
		var l linked
		if err := rows.Scan(&l.ReqID, &l.UserID, &l.SerialNo, &l.Status); err != nil {
			rows.Close()
			slog.Warn("invoice refund hook: scan failed", "order_id", orderID, "error", err)
			return
		}
		hits = append(hits, l)
	}
	rows.Close()
	if rowsErr := rows.Err(); rowsErr != nil {
		slog.Warn("invoice refund hook: rows error", "order_id", orderID, "error", rowsErr)
		return
	}
	if len(hits) == 0 {
		return
	}

	for _, h := range hits {
		switch h.Status {
		case InvoiceStatusPending:
			s.handleRefundPending(ctx, db, h.ReqID, h.UserID, h.SerialNo, orderID)
		case InvoiceStatusCompleted:
			s.handleRefundCompleted(ctx, db, h.ReqID, h.UserID, h.SerialNo, orderID)
		case InvoiceStatusRejected:
			// rejected: orders already returned to invoiceable pool; nothing to do
		default:
			slog.Warn("invoice refund hook: unexpected status", "request_id", h.ReqID, "status", h.Status)
		}
	}
}

// handleRefundPending detaches the refunded order from a pending request and recomputes the total.
// If the request becomes empty, it is hard-deleted (orders return to the invoiceable pool naturally).
func (s *PaymentService) handleRefundPending(ctx context.Context, db *sql.DB, reqID, userID int64, serialNo string, orderID int64) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		slog.Warn("invoice refund hook: begin tx failed", "request_id", reqID, "error", err)
		return
	}
	defer rollbackIfActive(tx)

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM invoice_request_orders
		WHERE invoice_request_id = $1 AND payment_order_id = $2
	`, reqID, orderID); err != nil {
		slog.Warn("invoice refund hook: detach failed", "request_id", reqID, "order_id", orderID, "error", err)
		return
	}

	var remaining int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM invoice_request_orders WHERE invoice_request_id = $1
	`, reqID).Scan(&remaining); err != nil {
		slog.Warn("invoice refund hook: count remaining failed", "request_id", reqID, "error", err)
		return
	}

	notifyTitle := ""
	notifyBody := ""

	if remaining == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM invoice_requests WHERE id = $1`, reqID); err != nil {
			slog.Warn("invoice refund hook: delete empty request failed", "request_id", reqID, "error", err)
			return
		}
		notifyTitle = "您的开票申请已撤销"
		notifyBody = fmt.Sprintf("申请单号 %s 关联的全部订单已退款，申请已自动撤销。", serialNo)
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE invoice_requests
			SET total_amount = (
				SELECT COALESCE(SUM(po.pay_amount), 0)
				FROM invoice_request_orders iro
				JOIN payment_orders po ON po.id = iro.payment_order_id
				WHERE iro.invoice_request_id = $1
			),
			    has_refunded_orders = true,
			    updated_at = NOW()
			WHERE id = $1
		`, reqID); err != nil {
			slog.Warn("invoice refund hook: recompute total failed", "request_id", reqID, "error", err)
			return
		}
		notifyTitle = "申请订单已部分退款"
		notifyBody = fmt.Sprintf("申请单号 %s 中的部分订单已退款，金额已重新计算。", serialNo)
	}

	if err := tx.Commit(); err != nil {
		slog.Warn("invoice refund hook: commit failed", "request_id", reqID, "error", err)
		return
	}

	s.writeRefundNotification(ctx, userID, reqID, serialNo, notifyTitle, notifyBody)
}

// handleRefundCompleted flags the completed request so admins can manually void/reissue.
func (s *PaymentService) handleRefundCompleted(ctx context.Context, db *sql.DB, reqID, userID int64, serialNo string, orderID int64) {
	res, err := db.ExecContext(ctx, `
		UPDATE invoice_requests
		SET has_refunded_orders = true,
		    updated_at = NOW()
		WHERE id = $1 AND has_refunded_orders = false
	`, reqID)
	if err != nil {
		slog.Warn("invoice refund hook: flag completed failed", "request_id", reqID, "error", err)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		// Already flagged from a previous refund — skip duplicate notification.
		return
	}

	s.writeRefundNotification(ctx, userID, reqID,
		serialNo,
		"已开具发票的订单发生退款",
		fmt.Sprintf("申请单号 %s 已开具发票，但其中订单 #%d 发生退款。请联系管理员协助开具红字发票。", serialNo, orderID),
	)
}

// writeRefundNotification persists an in-app notification for the user.
// Best-effort — failures are logged.
func (s *PaymentService) writeRefundNotification(ctx context.Context, userID, reqID int64, serialNo, title, body string) {
	if s == nil || s.invoiceNotifier == nil || userID <= 0 {
		return
	}
	if _, err := s.invoiceNotifier.Create(ctx, CreateUserNotificationInput{
		UserID:   userID,
		Category: NotificationCategoryInvoice,
		Title:    title,
		Body:     body,
		Link:     "/invoice",
		Metadata: map[string]any{"invoice_request_id": reqID, "serial_no": serialNo, "trigger": "refund"},
	}); err != nil {
		var apperr *infraerrors.Error
		if errors.As(err, &apperr) && apperr.Code == 0 {
			return
		}
		slog.Warn("invoice refund hook: notification failed", "user_id", userID, "request_id", reqID, "error", err)
	}
}
