package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Summary supplies the existing usage page without loading provider source rows
// or every per-key allocation. Each query observes a single database snapshot.
func (s *UpstreamReconciliationService) Summary(ctx context.Context, month string, userID int64, isAdmin bool) (*BillingReport, error) {
	if _, _, err := billingMonth(month, s.now(), billingLocation("tencent")); err != nil {
		return nil, err
	}
	if !isAdmin && userID <= 0 {
		return nil, infraerrors.Forbidden("UPSTREAM_BILLING_FORBIDDEN", "authentication is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	query := `SELECT b.currency,0::numeric::text,SUM(a.allocated_cost)::text,0::numeric::text
	 FROM upstream_billing_months m JOIN upstream_billing_bills b ON b.run_id=m.active_run_id
	 JOIN upstream_billing_allocations a ON a.bill_id=b.id
	 WHERE m.month=$1 AND a.user_id=$2 AND a.method<>'unmatched' GROUP BY b.currency ORDER BY b.currency`
	args := []any{month, userID}
	if isAdmin {
		query = `SELECT currency,SUM(official)::text,SUM(allocated)::text,SUM(unmatched)::text FROM (
		 SELECT b.currency,SUM(b.amount) AS official,0::numeric AS allocated,0::numeric AS unmatched
		 FROM upstream_billing_months m JOIN upstream_billing_bills b ON b.run_id=m.active_run_id
		 WHERE m.month=$1 GROUP BY b.currency
		 UNION ALL
		 SELECT b.currency,0::numeric,SUM(CASE WHEN a.method<>'unmatched' THEN a.allocated_cost ELSE 0 END),
		 SUM(CASE WHEN a.method='unmatched' THEN a.allocated_cost ELSE 0 END)
		 FROM upstream_billing_months m JOIN upstream_billing_bills b ON b.run_id=m.active_run_id
		 JOIN upstream_billing_allocations a ON a.bill_id=b.id WHERE m.month=$1 GROUP BY b.currency
		 ) totals GROUP BY currency ORDER BY currency`
		args = []any{month}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ErrBillingStorage
	}
	defer rows.Close()
	report := &BillingReport{Month: month, IsAdmin: isAdmin, Items: []BillingReportItem{}, Totals: []BillingReportTotal{}}
	for rows.Next() {
		var total BillingReportTotal
		if err := rows.Scan(&total.Currency, &total.OfficialCost, &total.AllocatedCost, &total.UnmatchedCost); err != nil {
			return nil, ErrBillingStorage
		}
		if !isAdmin {
			total.OfficialCost, total.UnmatchedCost = "", ""
		}
		report.Totals = append(report.Totals, total)
	}
	if rows.Err() != nil {
		return nil, ErrBillingStorage
	}
	return report, nil
}
