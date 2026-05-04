package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
)

// GetDashboardStats produces the admin dashboard's aggregate view over
// the trailing `days` window. Pass <= 0 to fall back to the 30-day
// default.
func (s *PaymentService) GetDashboardStats(ctx context.Context, days int) (*DashboardStats, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("payment service not configured")
	}
	if days <= 0 {
		days = 30
	}
	now := time.Now()
	since := now.AddDate(0, 0, -days)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	paidStatuses := []string{OrderStatusCompleted, OrderStatusPaid, OrderStatusRecharging}
	orders, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.StatusIn(paidStatuses...),
			paymentorder.PaidAtGTE(since),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	st := &DashboardStats{}
	computeBasicStats(st, orders, todayStart)

	st.PendingOrders, err = s.entClient.PaymentOrder.Query().
		Where(paymentorder.StatusEQ(OrderStatusPending)).
		Count(ctx)
	if err != nil {
		return nil, err
	}

	st.DailySeries = buildDailySeries(orders, since, days)
	st.PaymentMethods = buildMethodDistribution(orders)
	st.TopUsers = buildTopUsers(orders)
	return st, nil
}

func computeBasicStats(st *DashboardStats, orders []*pluginent.PaymentOrder, todayStart time.Time) {
	totalAmount := decimal.Zero
	todayAmount := decimal.Zero
	var todayCount int
	for _, o := range orders {
		totalAmount = totalAmount.Add(o.PayAmount)
		if o.PaidAt != nil && !o.PaidAt.Before(todayStart) {
			todayAmount = todayAmount.Add(o.PayAmount)
			todayCount++
		}
	}
	st.TotalAmount = roundYuan(totalAmount)
	st.TodayAmount = roundYuan(todayAmount)
	st.TotalCount = len(orders)
	st.TodayCount = todayCount
	if st.TotalCount > 0 {
		st.AvgAmount = roundYuan(totalAmount.Div(decimal.NewFromInt(int64(st.TotalCount))))
	}
}

func buildDailySeries(orders []*pluginent.PaymentOrder, since time.Time, days int) []DailyStats {
	dailyMap := make(map[string]*DailyStats)
	for _, o := range orders {
		if o.PaidAt == nil {
			continue
		}
		date := o.PaidAt.Format("2006-01-02")
		ds, ok := dailyMap[date]
		if !ok {
			ds = &DailyStats{Date: date}
			dailyMap[date] = ds
		}
		ds.Amount = ds.Amount.Add(o.PayAmount)
		ds.Count++
	}
	series := make([]DailyStats, 0, days)
	for i := 0; i < days; i++ {
		date := since.AddDate(0, 0, i+1).Format("2006-01-02")
		if ds, ok := dailyMap[date]; ok {
			ds.Amount = roundYuan(ds.Amount)
			series = append(series, *ds)
		} else {
			series = append(series, DailyStats{Date: date})
		}
	}
	return series
}

func buildMethodDistribution(orders []*pluginent.PaymentOrder) []PaymentMethodStat {
	methodMap := make(map[string]*PaymentMethodStat)
	for _, o := range orders {
		ms, ok := methodMap[o.PaymentType]
		if !ok {
			ms = &PaymentMethodStat{Type: o.PaymentType}
			methodMap[o.PaymentType] = ms
		}
		ms.Amount = ms.Amount.Add(o.PayAmount)
		ms.Count++
	}
	methods := make([]PaymentMethodStat, 0, len(methodMap))
	for _, ms := range methodMap {
		ms.Amount = roundYuan(ms.Amount)
		methods = append(methods, *ms)
	}
	return methods
}

func buildTopUsers(orders []*pluginent.PaymentOrder) []TopUserStat {
	userMap := make(map[int64]*TopUserStat)
	for _, o := range orders {
		us, ok := userMap[o.UserID]
		if !ok {
			us = &TopUserStat{UserID: o.UserID, Email: o.UserEmail}
			userMap[o.UserID] = us
		}
		us.Amount = us.Amount.Add(o.PayAmount)
	}
	userList := make([]*TopUserStat, 0, len(userMap))
	for _, us := range userMap {
		us.Amount = roundYuan(us.Amount)
		userList = append(userList, us)
	}
	sort.Slice(userList, func(i, j int) bool {
		return userList[i].Amount.GreaterThan(userList[j].Amount)
	})
	limit := topUsersLimit
	if len(userList) < limit {
		limit = len(userList)
	}
	result := make([]TopUserStat, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, *userList[i])
	}
	return result
}

// writeAuditLog records a structured action on the order's audit trail.
// Errors are logged at warn level and swallowed — the audit log is a
// best-effort observability surface; failing to record one must not
// block the underlying operation.
func (s *PaymentService) writeAuditLog(ctx context.Context, oid int64, action, op string, detail map[string]any) {
	if s == nil || s.entClient == nil {
		return
	}
	dj, _ := json.Marshal(detail)
	_, err := s.entClient.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(oid, 10)).
		SetAction(action).
		SetDetail(string(dj)).
		SetOperator(op).
		Save(ctx)
	if err != nil && s.logger != nil {
		s.logger.Warn("audit log failed", "order_id", oid, "action", action, "error", err)
	}
}

// GetOrderAuditLogs returns every audit-log entry for the given order
// id, ordered by creation time ascending so the UI can display the
// timeline top-to-bottom.
func (s *PaymentService) GetOrderAuditLogs(ctx context.Context, oid int64) ([]*pluginent.PaymentAuditLog, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("payment service not configured")
	}
	return s.entClient.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(oid, 10))).
		Order(paymentauditlog.ByCreatedAt()).
		All(ctx)
}
