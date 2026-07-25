package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
)

// --- Dashboard & Analytics ---

func (s *PaymentService) GetDashboardStats(ctx context.Context, days int) (*DashboardStats, error) {
	if days <= 0 {
		days = 30
REDACTED
	now := time.Now()
	since := now.AddDate(0, 0, -days)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	paidStatuses := []string{OrderStatusCompleted, OrderStatusPaid, OrderStatusRechargingREDACTED

	orders, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.StatusIn(paidStatuses...),
			paymentorder.PaidAtGTE(since),
		).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	st := &DashboardStats{REDACTED
	computeBasicStats(st, orders, todayStart)

	st.PendingOrders, err = s.entClient.PaymentOrder.Query().
		Where(paymentorder.StatusEQ(OrderStatusPending)).
		Count(ctx)
	if err != nil {
		return nil, err
REDACTED

	st.DailySeries = buildDailySeries(orders, since, days)
	st.PaymentMethods = buildMethodDistribution(orders)
	st.TopUsers = buildTopUsers(orders)

	return st, nil
REDACTED

func computeBasicStats(st *DashboardStats, orders []*dbent.PaymentOrder, todayStart time.Time) {
	st.TotalAmount = make(CurrencyAmounts)
	st.TodayAmount = make(CurrencyAmounts)
	st.AvgAmount = make(CurrencyAmounts)
	currencyCounts := make(map[string]int)
	var todayCount int
	for _, o := range orders {
		currency := PaymentOrderCurrency(o)
		st.TotalAmount[currency] += o.PayAmount
		currencyCounts[currency]++
		if o.PaidAt != nil && !o.PaidAt.Before(todayStart) {
			st.TodayAmount[currency] += o.PayAmount
			todayCount++
	REDACTED
REDACTED
	st.TotalCount = len(orders)
	st.TodayCount = todayCount
	for currency, totalAmount := range st.TotalAmount {
		st.AvgAmount[currency] = roundAmount(totalAmount / float64(currencyCounts[currency]))
REDACTED
	roundCurrencyAmounts(st.TotalAmount)
	roundCurrencyAmounts(st.TodayAmount)
REDACTED

func buildDailySeries(orders []*dbent.PaymentOrder, since time.Time, days int) []DailyStats {
	dailyMap := make(map[string]*DailyStats)
	for _, o := range orders {
		if o.PaidAt == nil {
			continue
	REDACTED
		date := o.PaidAt.Format("2006-01-02")
		ds, ok := dailyMap[date]
		if !ok {
			ds = &DailyStats{Date: date, Amount: make(CurrencyAmounts)REDACTED
			dailyMap[date] = ds
	REDACTED
		ds.Amount[PaymentOrderCurrency(o)] += o.PayAmount
		ds.Count++
REDACTED
	series := make([]DailyStats, 0, days)
	for i := 0; i < days; i++ {
		date := since.AddDate(0, 0, i+1).Format("2006-01-02")
		if ds, ok := dailyMap[date]; ok {
			roundCurrencyAmounts(ds.Amount)
			series = append(series, *ds)
	REDACTED else {
			series = append(series, DailyStats{Date: date, Amount: make(CurrencyAmounts)REDACTED)
	REDACTED
REDACTED
	return series
REDACTED

func buildMethodDistribution(orders []*dbent.PaymentOrder) []PaymentMethodStat {
	methodMap := make(map[string]*PaymentMethodStat)
	for _, o := range orders {
		ms, ok := methodMap[o.PaymentType]
		if !ok {
			ms = &PaymentMethodStat{Type: o.PaymentType, Amount: make(CurrencyAmounts)REDACTED
			methodMap[o.PaymentType] = ms
	REDACTED
		ms.Amount[PaymentOrderCurrency(o)] += o.PayAmount
		ms.Count++
REDACTED
	methods := make([]PaymentMethodStat, 0, len(methodMap))
	for _, ms := range methodMap {
		roundCurrencyAmounts(ms.Amount)
		methods = append(methods, *ms)
REDACTED
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Type < methods[j].Type
REDACTED)
	return methods
REDACTED

func buildTopUsers(orders []*dbent.PaymentOrder) TopUsersByCurrency {
	userMap := make(map[string]map[int64]*TopUserStat)
	for _, o := range orders {
		currency := PaymentOrderCurrency(o)
		users, ok := userMap[currency]
		if !ok {
			users = make(map[int64]*TopUserStat)
			userMap[currency] = users
	REDACTED
		us, ok := users[o.UserID]
		if !ok {
			us = &TopUserStat{UserID: o.UserID, Email: o.UserEmailREDACTED
			users[o.UserID] = us
	REDACTED
		us.Amount += o.PayAmount
REDACTED
	result := make(TopUsersByCurrency, len(userMap))
	for currency, users := range userMap {
		userList := make([]*TopUserStat, 0, len(users))
		for _, us := range users {
			us.Amount = roundAmount(us.Amount)
			userList = append(userList, us)
	REDACTED
		sort.Slice(userList, func(i, j int) bool {
			return userList[i].Amount > userList[j].Amount
	REDACTED)
		limit := topUsersLimit
		if len(userList) < limit {
			limit = len(userList)
	REDACTED
		result[currency] = make([]TopUserStat, 0, limit)
		for i := 0; i < limit; i++ {
			result[currency] = append(result[currency], *userList[i])
	REDACTED
REDACTED
	return result
REDACTED

func roundCurrencyAmounts(amounts CurrencyAmounts) {
	for currency, amount := range amounts {
		amounts[currency] = roundAmount(amount)
REDACTED
REDACTED

func roundAmount(amount float64) float64 {
	return math.Round(amount*100) / 100
REDACTED

// --- Audit Logs ---

func (s *PaymentService) writeAuditLog(ctx context.Context, oid int64, action, op string, detail map[string]any) {
	dj, _ := json.Marshal(detail)
	_, err := s.entClient.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(oid, 10)).SetAction(action).SetDetail(string(dj)).SetOperator(op).Save(ctx)
	if err != nil {
		slog.Error("audit log failed", "orderID", oid, "action", action, "error", err)
REDACTED
REDACTED

func (s *PaymentService) GetOrderAuditLogs(ctx context.Context, oid int64) ([]*dbent.PaymentAuditLog, error) {
	return s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(oid, 10))).Order(paymentauditlog.ByCreatedAt()).All(ctx)
REDACTED
