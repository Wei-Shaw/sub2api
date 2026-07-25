//go:build unit

package service

import (
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestComputeBasicStatsGroupsAmountsByCurrency(t *testing.T) {
	t.Parallel()

	todayStart := time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC)
	yesterday := todayStart.Add(-time.Hour)
	today := todayStart.Add(time.Hour)
	orders := []*dbent.PaymentOrder{
		paymentStatsTestOrder(1, "alice@example.com", "CNY", 10, &today),
		paymentStatsTestOrder(2, "bob@example.com", "USD", 10, &today),
		paymentStatsTestOrder(1, "alice@example.com", "CNY", 5, &yesterday),
REDACTED

	stats := &DashboardStats{REDACTED
	computeBasicStats(stats, orders, todayStart)

	require.Equal(t, CurrencyAmounts{"CNY": 15, "USD": 10REDACTED, stats.TotalAmount)
	require.Equal(t, CurrencyAmounts{"CNY": 10, "USD": 10REDACTED, stats.TodayAmount)
	require.Equal(t, CurrencyAmounts{"CNY": 7.5, "USD": 10REDACTED, stats.AvgAmount)
	require.Equal(t, 3, stats.TotalCount)
	require.Equal(t, 2, stats.TodayCount)
REDACTED

func TestPaymentDashboardBreakdownsGroupAmountsAndRankingsByCurrency(t *testing.T) {
	t.Parallel()

	firstDay := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	secondDay := firstDay.AddDate(0, 0, 1)
	orders := []*dbent.PaymentOrder{
		paymentStatsTestOrder(1, "alice@example.com", "CNY", 5.555, &firstDay),
		paymentStatsTestOrder(2, "bob@example.com", "CNY", 10, &firstDay),
		paymentStatsTestOrder(1, "alice@example.com", "USD", 20, &secondDay),
		paymentStatsTestOrder(2, "bob@example.com", "USD", 10, &secondDay),
REDACTED
	orders[0].PaymentType = "stripe"
	orders[1].PaymentType = "stripe"
	orders[2].PaymentType = "stripe"
	orders[3].PaymentType = "alipay"

	daily := buildDailySeries(orders, firstDay.AddDate(0, 0, -1), 2)
	require.Equal(t, []DailyStats{
		{Date: "2026-07-24", Amount: CurrencyAmounts{"CNY": 15.56REDACTED, Count: 2REDACTED,
		{Date: "2026-07-25", Amount: CurrencyAmounts{"USD": 30REDACTED, Count: 2REDACTED,
REDACTED, daily)

	methods := buildMethodDistribution(orders)
	require.Equal(t, []PaymentMethodStat{
		{Type: "alipay", Amount: CurrencyAmounts{"USD": 10REDACTED, Count: 1REDACTED,
		{Type: "stripe", Amount: CurrencyAmounts{"CNY": 15.56, "USD": 20REDACTED, Count: 3REDACTED,
REDACTED, methods)

	users := buildTopUsers(orders)
	require.Equal(t, TopUsersByCurrency{
		"CNY": {
			{UserID: 2, Email: "bob@example.com", Amount: 10REDACTED,
			{UserID: 1, Email: "alice@example.com", Amount: 5.56REDACTED,
	REDACTED,
		"USD": {
			{UserID: 1, Email: "alice@example.com", Amount: 20REDACTED,
			{UserID: 2, Email: "bob@example.com", Amount: 10REDACTED,
	REDACTED,
REDACTED, users)
REDACTED

func paymentStatsTestOrder(userID int64, email, currency string, amount float64, paidAt *time.Time) *dbent.PaymentOrder {
	return &dbent.PaymentOrder{
		UserID:           userID,
		UserEmail:        email,
		PayAmount:        amount,
		PaidAt:           paidAt,
		ProviderSnapshot: map[string]any{"currency": currencyREDACTED,
REDACTED
REDACTED
