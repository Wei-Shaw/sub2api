package repository

import (
	"context"

	"github.com/lib/pq"
)

// Reconcile funded debts when the supplier reads earnings; the panel polls this endpoint.
func (r *relayRepository) settlePendingEarnings(ctx context.Context, supplierID int64) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT e.id,e.consumer_user_id FROM relay_earnings e
		JOIN users u ON u.id=e.consumer_user_id
		WHERE e.supplier_user_id=$1 AND e.credited_at IS NULL AND u.balance>=0
		ORDER BY e.id LIMIT 50 FOR UPDATE OF e SKIP LOCKED`, supplierID)
	if err != nil {
		return false, err
	}
	type pending struct{ id, buyer int64 }
	entries := []pending{}
	userIDs := []int64{supplierID}
	for rows.Next() {
		var v pending
		if err = rows.Scan(&v.id, &v.buyer); err != nil {
			rows.Close()
			return false, err
		}
		entries = append(entries, v)
		userIDs = append(userIDs, v.buyer)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if len(entries) == 0 {
		return false, nil
	}
	rows, err = tx.QueryContext(ctx, `SELECT id,balance FROM users WHERE id=ANY($1) ORDER BY id FOR UPDATE`, pq.Array(userIDs))
	if err != nil {
		return false, err
	}
	balances := map[int64]float64{}
	for rows.Next() {
		var id int64
		var balance float64
		if err = rows.Scan(&id, &balance); err != nil {
			rows.Close()
			return false, err
		}
		balances[id] = balance
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	ids := []int64{}
	for _, v := range entries {
		if balance, ok := balances[v.buyer]; ok && balance >= 0 {
			ids = append(ids, v.id)
		}
	}
	if len(ids) == 0 {
		return false, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET balance=balance+(SELECT COALESCE(SUM(amount),0) FROM relay_earnings WHERE id=ANY($2)),updated_at=NOW() WHERE id=$1`, supplierID, pq.Array(ids)); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE relay_earnings SET credited_at=NOW() WHERE id=ANY($1)`, pq.Array(ids)); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
