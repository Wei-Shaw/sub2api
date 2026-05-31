package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	paymentdomain "github.com/Wei-Shaw/sub2api/internal/payment"
)

// --- Dashboard & Analytics ---

func (s *PaymentService) GetDashboardStats(ctx context.Context, days int) (*DashboardStats, error) {
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
	depositStats, err := s.buildDepositStats(ctx, since, paidStatuses)
	if err != nil {
		return nil, err
	}
	st.Deposits = *depositStats

	return st, nil
}

func computeBasicStats(st *DashboardStats, orders []*dbent.PaymentOrder, todayStart time.Time) {
	st.TotalAmount = make(CurrencyAmounts)
	st.TodayAmount = make(CurrencyAmounts)
	st.AvgAmount = make(CurrencyAmounts)
	currencyCounts := make(map[string]int)
	currencyRevenue := make(map[string]*CurrencyRevenue)
	var todayCount int

	for _, o := range orders {
		currency := PaymentOrderCurrency(o)
		amount := o.PayAmount
		st.TotalAmount[currency] += amount
		currencyCounts[currency]++

		revenue, ok := currencyRevenue[currency]
		if !ok {
			revenue = &CurrencyRevenue{Currency: currency}
			currencyRevenue[currency] = revenue
		}
		revenue.TotalAmount += amount
		revenue.TotalCount++

		if o.PaidAt != nil && !o.PaidAt.Before(todayStart) {
			st.TodayAmount[currency] += amount
			revenue.TodayAmount += amount
			revenue.TodayCount++
			todayCount++
		}
	}

	st.TotalCount = len(orders)
	st.TodayCount = todayCount
	for currency, totalAmount := range st.TotalAmount {
		st.AvgAmount[currency] = roundAmount(totalAmount / float64(currencyCounts[currency]))
	}
	finalizeRevenueStats(st, currencyRevenue)
}

func finalizeRevenueStats(st *DashboardStats, currencyRevenue map[string]*CurrencyRevenue) {
	roundCurrencyAmounts(st.TotalAmount)
	roundCurrencyAmounts(st.TodayAmount)

	st.RevenueByCurrency = make([]CurrencyRevenue, 0, len(currencyRevenue))
	for _, revenue := range currencyRevenue {
		revenue.TotalAmount = roundAmount(revenue.TotalAmount)
		revenue.TodayAmount = roundAmount(revenue.TodayAmount)
		st.RevenueByCurrency = append(st.RevenueByCurrency, *revenue)
	}
	sort.Slice(st.RevenueByCurrency, func(i, j int) bool {
		if st.RevenueByCurrency[i].TotalCount == st.RevenueByCurrency[j].TotalCount {
			return st.RevenueByCurrency[i].Currency < st.RevenueByCurrency[j].Currency
		}
		return st.RevenueByCurrency[i].TotalCount > st.RevenueByCurrency[j].TotalCount
	})
}

func buildDailySeries(orders []*dbent.PaymentOrder, since time.Time, days int) []DailyStats {
	dailyMap := make(map[string]*DailyStats)
	for _, o := range orders {
		if o.PaidAt == nil {
			continue
		}
		date := o.PaidAt.Format("2006-01-02")
		ds, ok := dailyMap[date]
		if !ok {
			ds = &DailyStats{Date: date, Amount: make(CurrencyAmounts)}
			dailyMap[date] = ds
		}
		ds.Amount[PaymentOrderCurrency(o)] += o.PayAmount
		ds.Count++
	}
	series := make([]DailyStats, 0, days)
	for i := 0; i < days; i++ {
		date := since.AddDate(0, 0, i+1).Format("2006-01-02")
		if ds, ok := dailyMap[date]; ok {
			roundCurrencyAmounts(ds.Amount)
			series = append(series, *ds)
		} else {
			series = append(series, DailyStats{Date: date, Amount: make(CurrencyAmounts)})
		}
	}
	return series
}

func buildMethodDistribution(orders []*dbent.PaymentOrder) []PaymentMethodStat {
	methodMap := make(map[string]*PaymentMethodStat)
	for _, o := range orders {
		method, ok := methodMap[o.PaymentType]
		if !ok {
			method = &PaymentMethodStat{Type: o.PaymentType, Amount: make(CurrencyAmounts)}
			methodMap[o.PaymentType] = method
		}
		method.Amount[PaymentOrderCurrency(o)] += o.PayAmount
		method.Count++
	}

	methods := make([]PaymentMethodStat, 0, len(methodMap))
	for _, method := range methodMap {
		roundCurrencyAmounts(method.Amount)
		methods = append(methods, *method)
	}
	sort.Slice(methods, func(i, j int) bool {
		if methods[i].Count == methods[j].Count {
			return methods[i].Type < methods[j].Type
		}
		return methods[i].Count > methods[j].Count
	})
	return methods
}

func buildTopUsers(orders []*dbent.PaymentOrder) TopUsersByCurrency {
	userMap := make(map[string]map[int64]*TopUserStat)
	for _, o := range orders {
		currency := PaymentOrderCurrency(o)
		users, ok := userMap[currency]
		if !ok {
			users = make(map[int64]*TopUserStat)
			userMap[currency] = users
		}
		us, ok := users[o.UserID]
		if !ok {
			us = &TopUserStat{UserID: o.UserID, Email: o.UserEmail}
			users[o.UserID] = us
		}
		us.Amount += o.PayAmount
	}
	result := make(TopUsersByCurrency, len(userMap))
	for currency, users := range userMap {
		userList := make([]*TopUserStat, 0, len(users))
		for _, us := range users {
			us.Amount = roundAmount(us.Amount)
			userList = append(userList, us)
		}
		sort.Slice(userList, func(i, j int) bool {
			return userList[i].Amount > userList[j].Amount
		})
		limit := topUsersLimit
		if len(userList) < limit {
			limit = len(userList)
		}
		result[currency] = make([]TopUserStat, 0, limit)
		for i := 0; i < limit; i++ {
			result[currency] = append(result[currency], *userList[i])
		}
	}
	return result
}

func roundCurrencyAmounts(amounts CurrencyAmounts) {
	for currency, amount := range amounts {
		amounts[currency] = roundAmount(amount)
	}
}

func addCurrencyAmount(amounts CurrencyAmounts, currency string, amount float64) {
	amounts[normalizeCurrencyCode(currency, "USD")] += amount
}

func roundAmount(amount float64) float64 {
	return math.Round(amount*100) / 100
}

const (
	depositSourcePaidBalanceOrder         = "paid_balance_order"
	depositSourcePaidSubscriptionOrder    = "paid_subscription_order"
	depositSourceRedeemBalance            = "redeem_balance"
	depositSourceRedeemSubscription       = "redeem_subscription"
	depositSourceRedeemAffiliateBalance   = "redeem_affiliate_balance"
	depositSourceAdminBalanceAdjustment   = "admin_balance_adjustment"
	depositSourceManualSubscriptionAssign = "manual_subscription_assignment"
	depositSourceAutoSubscriptionAssign   = "auto_subscription_assignment"
	depositReferencePaymentOrder          = "payment_order"
	depositReferenceRedeemCode            = "redeem_code"
	depositReferenceUserSubscription      = "user_subscription"
	recentDepositEventsLimit              = 20
)

type depositAccumulator struct {
	stats      DepositStats
	bySource   map[string]*DepositSourceStat
	recipients map[int64]*DepositRecipientStat
	events     []DepositEventStat
}

func newDepositAccumulator() *depositAccumulator {
	return &depositAccumulator{
		stats:      DepositStats{TotalLedgerAmount: make(CurrencyAmounts)},
		bySource:   make(map[string]*DepositSourceStat),
		recipients: make(map[int64]*DepositRecipientStat),
	}
}

func (s *PaymentService) buildDepositStats(ctx context.Context, since time.Time, paidStatuses []string) (*DepositStats, error) {
	acc := newDepositAccumulator()

	orders, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.StatusIn(paidStatuses...),
			paymentorder.PaidAtGTE(since),
			paymentorder.OrderTypeIn(paymentdomain.OrderTypeBalance, paymentdomain.OrderTypeSubscription),
		).
		WithUser().
		Order(paymentorder.ByPaidAt(sql.OrderDesc(), sql.OrderNullsLast()), paymentorder.ByID(sql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, order := range orders {
		if order.PaidAt == nil {
			continue
		}
		source := depositSourcePaidBalanceOrder
		subscriptionAssignments := 0
		groupID := order.BalanceGroupID
		if order.OrderType == paymentdomain.OrderTypeSubscription {
			source = depositSourcePaidSubscriptionOrder
			subscriptionAssignments = 1
			groupID = order.SubscriptionGroupID
		}
		identity := paymentOrderDepositUserIdentity(order)
		acc.add(DepositEventStat{
			Source:                  source,
			UserID:                  order.UserID,
			Email:                   identity.Email,
			Username:                identity.Username,
			LedgerAmount:            paymentOrderLedgerDepositAmount(order),
			Credits:                 paymentOrderDepositCredits(order),
			Currency:                paymentOrderLedgerCurrency(order),
			SubscriptionAssignments: subscriptionAssignments,
			GroupID:                 cloneDepositInt64Ptr(groupID),
			PaymentType:             order.PaymentType,
			ReferenceType:           depositReferencePaymentOrder,
			ReferenceID:             strconv.FormatInt(order.ID, 10),
			OccurredAt:              *order.PaidAt,
		})
	}

	redeems, err := s.entClient.RedeemCode.Query().
		Where(
			redeemcode.StatusEQ(StatusUsed),
			redeemcode.UsedByNotNil(),
			redeemcode.UsedAtGTE(since),
			redeemcode.TypeIn(RedeemTypeBalance, RedeemTypeSubscription, RedeemTypeAffiliateBalance, AdjustmentTypeAdminBalance),
		).
		WithUser().
		WithCreator().
		WithGroup().
		Order(redeemcode.ByUsedAt(sql.OrderDesc(), sql.OrderNullsLast()), redeemcode.ByID(sql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, rc := range redeems {
		if rc.UsedAt == nil || rc.UsedBy == nil || !isDepositRedeem(rc) {
			continue
		}
		identity := redeemDepositUserIdentity(rc)
		operator := depositOperatorIdentity(rc.CreatedBy, rc.Edges.Creator)
		groupID, groupName, platform := redeemDepositGroupIdentity(rc)
		amount := 0.0
		credits := 0.0
		subscriptionAssignments := 0
		validityDays := 0
		source := depositSourceRedeemBalance
		switch rc.Type {
		case AdjustmentTypeAdminBalance:
			source = depositSourceAdminBalanceAdjustment
			amount = rc.Value
			credits = rc.Value
		case RedeemTypeAffiliateBalance:
			source = depositSourceRedeemAffiliateBalance
			amount = rc.Value
			credits = rc.Value
		case RedeemTypeSubscription:
			source = depositSourceRedeemSubscription
			subscriptionAssignments = 1
			validityDays = rc.ValidityDays
		default:
			amount = rc.Value
			credits = rc.Value
		}
		acc.add(DepositEventStat{
			Source:                  source,
			UserID:                  *rc.UsedBy,
			Email:                   identity.Email,
			Username:                identity.Username,
			LedgerAmount:            amount,
			Credits:                 credits,
			Currency:                "USD",
			SubscriptionAssignments: subscriptionAssignments,
			ValidityDays:            validityDays,
			GroupID:                 groupID,
			GroupName:               groupName,
			Platform:                platform,
			OperatorID:              operator.UserID,
			OperatorEmail:           operator.Email,
			ReferenceType:           depositReferenceRedeemCode,
			ReferenceID:             strconv.FormatInt(rc.ID, 10),
			OccurredAt:              *rc.UsedAt,
		})
	}

	subscriptions, err := s.entClient.UserSubscription.Query().
		Where(usersubscription.AssignedAtGTE(since)).
		WithUser().
		WithGroup().
		WithAssignedByUser().
		Order(usersubscription.ByAssignedAt(sql.OrderDesc()), usersubscription.ByID(sql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, sub := range subscriptions {
		if subscriptionAssignmentAlreadyCounted(stringFromPtr(sub.Notes)) {
			continue
		}
		source := depositSourceAutoSubscriptionAssign
		if sub.AssignedBy != nil && *sub.AssignedBy > 0 {
			source = depositSourceManualSubscriptionAssign
		}
		identity := subscriptionDepositUserIdentity(sub)
		operator := depositOperatorIdentity(sub.AssignedBy, sub.Edges.AssignedByUser)
		groupID, groupName, platform := subscriptionDepositGroupIdentity(sub)
		acc.add(DepositEventStat{
			Source:                  source,
			UserID:                  sub.UserID,
			Email:                   identity.Email,
			Username:                identity.Username,
			SubscriptionAssignments: 1,
			ValidityDays:            subscriptionValidityDays(sub),
			GroupID:                 groupID,
			GroupName:               groupName,
			Platform:                platform,
			OperatorID:              operator.UserID,
			OperatorEmail:           operator.Email,
			ReferenceType:           depositReferenceUserSubscription,
			ReferenceID:             strconv.FormatInt(sub.ID, 10),
			OccurredAt:              sub.AssignedAt,
		})
	}

	stats := acc.finalize()
	return &stats, nil
}

func (a *depositAccumulator) add(event DepositEventStat) {
	if event.Source == "" || event.UserID <= 0 || event.OccurredAt.IsZero() {
		return
	}
	event.LedgerAmount = roundDepositNumber(event.LedgerAmount)
	event.Credits = roundDepositNumber(event.Credits)
	event.Currency = normalizeCurrencyCode(event.Currency, "USD")
	a.events = append(a.events, event)
	a.stats.TotalEvents++
	addCurrencyAmount(a.stats.TotalLedgerAmount, event.Currency, event.LedgerAmount)
	a.stats.TotalCredits += event.Credits
	a.stats.SubscriptionAssignments += event.SubscriptionAssignments
	switch event.Source {
	case depositSourcePaidBalanceOrder, depositSourcePaidSubscriptionOrder:
		a.stats.PaidTopups++
	case depositSourceRedeemBalance, depositSourceRedeemSubscription, depositSourceRedeemAffiliateBalance:
		a.stats.RedeemDeposits++
	case depositSourceAdminBalanceAdjustment:
		a.stats.AdminAdjustments++
	case depositSourceManualSubscriptionAssign:
		a.stats.ManualAssignments++
	case depositSourceAutoSubscriptionAssign:
		a.stats.AutoAssignments++
	}

	source := a.bySource[event.Source]
	if source == nil {
		source = &DepositSourceStat{Source: event.Source, LedgerAmount: make(CurrencyAmounts)}
		a.bySource[event.Source] = source
	}
	source.Count++
	addCurrencyAmount(source.LedgerAmount, event.Currency, event.LedgerAmount)
	source.Credits += event.Credits
	source.SubscriptionAssignments += event.SubscriptionAssignments
	if source.LastDepositAt == nil || event.OccurredAt.After(*source.LastDepositAt) {
		t := event.OccurredAt
		source.LastDepositAt = &t
	}

	recipient := a.recipients[event.UserID]
	if recipient == nil {
		recipient = &DepositRecipientStat{UserID: event.UserID, Email: event.Email, Username: event.Username, LedgerAmount: make(CurrencyAmounts)}
		a.recipients[event.UserID] = recipient
	}
	if recipient.Email == "" {
		recipient.Email = event.Email
	}
	if recipient.Username == "" {
		recipient.Username = event.Username
	}
	recipient.Count++
	addCurrencyAmount(recipient.LedgerAmount, event.Currency, event.LedgerAmount)
	recipient.Credits += event.Credits
	recipient.SubscriptionAssignments += event.SubscriptionAssignments
	if recipient.LastDepositAt == nil || event.OccurredAt.After(*recipient.LastDepositAt) {
		t := event.OccurredAt
		recipient.LastDepositAt = &t
		recipient.LastSource = event.Source
	}
}

func (a *depositAccumulator) finalize() DepositStats {
	roundCurrencyAmounts(a.stats.TotalLedgerAmount)
	a.stats.TotalCredits = roundDepositNumber(a.stats.TotalCredits)
	a.stats.BySource = make([]DepositSourceStat, 0, len(a.bySource))
	for _, source := range a.bySource {
		roundCurrencyAmounts(source.LedgerAmount)
		source.Credits = roundDepositNumber(source.Credits)
		a.stats.BySource = append(a.stats.BySource, *source)
	}
	sort.Slice(a.stats.BySource, func(i, j int) bool {
		if a.stats.BySource[i].Count == a.stats.BySource[j].Count {
			return a.stats.BySource[i].Source < a.stats.BySource[j].Source
		}
		return a.stats.BySource[i].Count > a.stats.BySource[j].Count
	})

	recipients := make([]DepositRecipientStat, 0, len(a.recipients))
	for _, recipient := range a.recipients {
		roundCurrencyAmounts(recipient.LedgerAmount)
		recipient.Credits = roundDepositNumber(recipient.Credits)
		recipients = append(recipients, *recipient)
	}
	sort.Slice(recipients, func(i, j int) bool {
		if recipients[i].Credits != recipients[j].Credits {
			return recipients[i].Credits > recipients[j].Credits
		}
		if recipients[i].SubscriptionAssignments != recipients[j].SubscriptionAssignments {
			return recipients[i].SubscriptionAssignments > recipients[j].SubscriptionAssignments
		}
		if recipients[i].Count != recipients[j].Count {
			return recipients[i].Count > recipients[j].Count
		}
		return recipients[i].UserID < recipients[j].UserID
	})
	limit := topUsersLimit
	if len(recipients) < limit {
		limit = len(recipients)
	}
	a.stats.TopRecipients = recipients[:limit]

	sort.Slice(a.events, func(i, j int) bool {
		return a.events[i].OccurredAt.After(a.events[j].OccurredAt)
	})
	limit = recentDepositEventsLimit
	if len(a.events) < limit {
		limit = len(a.events)
	}
	a.stats.RecentEvents = a.events[:limit]
	return a.stats
}

type depositUserIdentity struct {
	Email    string
	Username string
}

type depositOperator struct {
	UserID *int64
	Email  string
}

func paymentOrderDepositUserIdentity(order *dbent.PaymentOrder) depositUserIdentity {
	identity := depositUserIdentity{Email: order.UserEmail}
	if order.Edges.User != nil {
		identity = depositUserIdentity{Email: order.Edges.User.Email, Username: order.Edges.User.Username}
		if identity.Email == "" {
			identity.Email = order.UserEmail
		}
	}
	return identity
}

func redeemDepositUserIdentity(code *dbent.RedeemCode) depositUserIdentity {
	if code.Edges.User == nil {
		return depositUserIdentity{}
	}
	return depositUserIdentity{Email: code.Edges.User.Email, Username: code.Edges.User.Username}
}

func subscriptionDepositUserIdentity(sub *dbent.UserSubscription) depositUserIdentity {
	if sub.Edges.User == nil {
		return depositUserIdentity{}
	}
	return depositUserIdentity{Email: sub.Edges.User.Email, Username: sub.Edges.User.Username}
}

func depositOperatorIdentity(id *int64, user *dbent.User) depositOperator {
	operator := depositOperator{UserID: cloneDepositInt64Ptr(id)}
	if user != nil {
		operator.Email = user.Email
	}
	return operator
}

func paymentOrderLedgerDepositAmount(order *dbent.PaymentOrder) float64 {
	amount := order.LedgerAmount
	if amount == 0 {
		amount = order.PayAmount
	}
	return amount
}

func paymentOrderLedgerCurrency(order *dbent.PaymentOrder) string {
	if order.LedgerCurrency != "" {
		return order.LedgerCurrency
	}
	if order.PaymentCurrency != "" {
		return order.PaymentCurrency
	}
	return "USD"
}

func paymentOrderDepositCredits(order *dbent.PaymentOrder) float64 {
	if order.ActualCredits == nil || *order.ActualCredits <= 0 {
		return 0
	}
	return float64(*order.ActualCredits)
}

func isDepositRedeem(code *dbent.RedeemCode) bool {
	if code.Type == RedeemTypeSubscription {
		return code.ValidityDays > 0
	}
	return code.Value > 0
}

func redeemDepositGroupIdentity(code *dbent.RedeemCode) (*int64, string, string) {
	if code.Edges.Group != nil {
		id := code.Edges.Group.ID
		return &id, code.Edges.Group.Name, code.Edges.Group.Platform
	}
	return cloneDepositInt64Ptr(code.GroupID), "", ""
}

func subscriptionDepositGroupIdentity(sub *dbent.UserSubscription) (*int64, string, string) {
	if sub.Edges.Group != nil {
		id := sub.Edges.Group.ID
		return &id, sub.Edges.Group.Name, sub.Edges.Group.Platform
	}
	return cloneDepositInt64Value(sub.GroupID), "", ""
}

func subscriptionValidityDays(sub *dbent.UserSubscription) int {
	if sub.ExpiresAt.IsZero() || sub.StartsAt.IsZero() || !sub.ExpiresAt.After(sub.StartsAt) {
		return 0
	}
	return int(math.Round(sub.ExpiresAt.Sub(sub.StartsAt).Hours() / 24))
}

func subscriptionAssignmentAlreadyCounted(notes string) bool {
	lower := strings.ToLower(strings.TrimSpace(notes))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "payment order") ||
		strings.Contains(lower, "redeem code") ||
		strings.Contains(lower, "through redeem") ||
		strings.Contains(notes, "兑换码")
}

func cloneDepositInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	return cloneDepositInt64Value(*v)
}

func cloneDepositInt64Value(v int64) *int64 {
	copy := v
	return &copy
}

func stringFromPtr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func roundDepositNumber(v float64) float64 {
	return math.Round(v*100) / 100
}

// --- Audit Logs ---

func (s *PaymentService) writeAuditLog(ctx context.Context, oid int64, action, op string, detail map[string]any) {
	dj, _ := json.Marshal(detail)
	_, err := s.entClient.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(oid, 10)).SetAction(action).SetDetail(string(dj)).SetOperator(op).Save(ctx)
	if err != nil {
		slog.Error("audit log failed", "orderID", oid, "action", action, "error", err)
	}
}

func (s *PaymentService) GetOrderAuditLogs(ctx context.Context, oid int64) ([]*dbent.PaymentAuditLog, error) {
	return s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(oid, 10))).Order(paymentauditlog.ByCreatedAt()).All(ctx)
}
