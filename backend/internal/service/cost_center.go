package service

import (
	"context"
	"time"
)

const (
	CostEventIncome                  = "income"
	CostEventExpense                 = "expense"
	CostEventConsumption             = "consumption"
	CostEventPromotionalConsumption  = "promotional_consumption"
	CostEventSubscriptionRecognition = "subscription_recognition"
	CostEventReversal                = "reversal"
)

type CostCenterEvent struct {
	ID               int64          `json:"id"`
	EventType        string         `json:"event_type"`
	Status           string         `json:"status"`
	SourceType       string         `json:"source_type"`
	SourceID         *string        `json:"source_id,omitempty"`
	AccountID        *int64         `json:"account_id,omitempty"`
	AccountName      string         `json:"account_name,omitempty"`
	UserID           *int64         `json:"user_id,omitempty"`
	UserName         string         `json:"user_name,omitempty"`
	PlanID           *int64         `json:"plan_id,omitempty"`
	Platform         string         `json:"platform,omitempty"`
	GroupID          *int64         `json:"group_id,omitempty"`
	Model            string         `json:"model,omitempty"`
	Category         string         `json:"category"`
	AmountUSD        float64        `json:"amount_usd"`
	OriginalAmount   *float64       `json:"original_amount,omitempty"`
	OriginalCurrency *string        `json:"original_currency,omitempty"`
	FXRate           *float64       `json:"fx_rate,omitempty"`
	OccurredAt       time.Time      `json:"occurred_at"`
	Note             string         `json:"note"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	OperatorID       *int64         `json:"operator_id,omitempty"`
	OperatorName     string         `json:"operator_name,omitempty"`
	ReversalOf       *int64         `json:"reversal_of,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

type CreateCostCenterEventInput struct {
	EventType        string         `json:"event_type"`
	Status           string         `json:"status"`
	SourceType       string         `json:"source_type"`
	SourceID         *string        `json:"source_id"`
	IdempotencyKey   *string        `json:"idempotency_key"`
	AccountID        *int64         `json:"account_id"`
	UserID           *int64         `json:"user_id"`
	PlanID           *int64         `json:"plan_id"`
	Platform         string         `json:"platform"`
	GroupID          *int64         `json:"group_id"`
	Model            string         `json:"model"`
	Category         string         `json:"category"`
	AmountUSD        float64        `json:"amount_usd" binding:"required,gt=0"`
	OriginalAmount   *float64       `json:"original_amount"`
	OriginalCurrency *string        `json:"original_currency"`
	FXRate           *float64       `json:"fx_rate"`
	OccurredAt       *time.Time     `json:"occurred_at"`
	Note             string         `json:"note"`
	Metadata         map[string]any `json:"metadata"`
	OperatorID       *int64         `json:"-"`
	ReversalOf       *int64         `json:"reversal_of"`
}

type CostCenterReportFilter struct {
	Start      time.Time
	End        time.Time
	AccountID  *int64
	Category   string
	SourceType string
	Platform   string
	UserID     *int64
	GroupID    *int64
	Model      string
	PlanID     *int64
}

type CostCenterSummary struct {
	CashIncome              float64 `json:"cash_income"`
	RealizedIncome          float64 `json:"realized_income"`
	PromotionalConsumption  float64 `json:"promotional_consumption"`
	SettledExpenses         float64 `json:"settled_expenses"`
	RebateAmount            float64 `json:"rebate_amount"`
	PendingForecast         float64 `json:"pending_forecast"`
	CashProfit              float64 `json:"cash_profit"`
	OperatingProfit         float64 `json:"operating_profit"`
	ProfitMargin            float64 `json:"profit_margin"`
	UnknownSourceAmount     float64 `json:"unknown_source_amount"`
	DeferredSubscriptionUSD float64 `json:"deferred_subscription_usd"`
	ExpiredEntitlementUSD   float64 `json:"expired_entitlement_usd"`
}

type ExpensePlan struct {
	ID            int64      `json:"id"`
	AccountID     *int64     `json:"account_id,omitempty"`
	Category      string     `json:"category"`
	AmountUSD     float64    `json:"amount_usd"`
	IntervalUnit  string     `json:"interval_unit"`
	IntervalValue int        `json:"interval_value"`
	StartsAt      time.Time  `json:"starts_at"`
	EndsAt        *time.Time `json:"ends_at,omitempty"`
	NextDueAt     time.Time  `json:"next_due_at"`
	Active        bool       `json:"active"`
	Note          string     `json:"note"`
	OperatorID    *int64     `json:"operator_id,omitempty"`
}

type CreateExpensePlanInput struct {
	AccountID     *int64     `json:"account_id"`
	Category      string     `json:"category"`
	AmountUSD     float64    `json:"amount_usd"`
	IntervalUnit  string     `json:"interval_unit"`
	IntervalValue int        `json:"interval_value"`
	StartsAt      time.Time  `json:"starts_at"`
	EndsAt        *time.Time `json:"ends_at"`
	Note          string     `json:"note"`
	OperatorID    *int64     `json:"-"`
}

type SubscriptionEntitlementSnapshot struct {
	OrderID             int64
	UserID              int64
	PlanID              *int64
	GroupID             *int64
	PriceUSD            float64
	StandardQuotaTokens int64
	StartsAt            time.Time
	ExpiresAt           time.Time
}

type CostCenterRepository interface {
	CreateEvent(context.Context, *CreateCostCenterEventInput) (*CostCenterEvent, error)
	ListEvents(context.Context, CostCenterReportFilter, int, int) ([]CostCenterEvent, int64, error)
	Summarize(context.Context, CostCenterReportFilter) (*CostCenterSummary, error)
	CreateExpensePlan(context.Context, *CreateExpensePlanInput) (*ExpensePlan, error)
	MaterializeExpensePlans(context.Context, time.Time) (int, error)
	UpdateEventStatus(context.Context, int64, string, string, *int64) (*CostCenterEvent, error)
	ReverseEvent(context.Context, int64, string, *int64) (*CostCenterEvent, error)
	Reconcile(context.Context, CostCenterReportFilter) (*CostCenterReconciliation, error)
	SnapshotSubscriptionEntitlement(context.Context, *SubscriptionEntitlementSnapshot) error
	RecognizeSubscriptionUsage(context.Context, int64, string, int64, float64, time.Time) (*CostCenterEvent, error)
}

type CostCenterReconciliation struct {
	UnknownEvents int64 `json:"unknown_events"`
	PendingEvents int64 `json:"pending_events"`
	DuplicateKeys int64 `json:"duplicate_keys"`
}

type CostCenterWriter interface {
	CreateEvent(context.Context, *CreateCostCenterEventInput) (*CostCenterEvent, error)
}

type CostCenterService struct{ repo CostCenterRepository }

func (s *CostCenterService) CreateEvent(ctx context.Context, input *CreateCostCenterEventInput) (*CostCenterEvent, error) {
	if input == nil || input.AmountUSD <= 0 {
		return nil, ErrInvalidCostCenterAmount
	}
	if input.Status == "" {
		input.Status = "settled"
	}
	if input.EventType == CostEventExpense && input.AccountID != nil {
		input.SourceType = "account"
	} else if input.SourceType == "" {
		input.SourceType = "manual"
	}
	return s.repo.CreateEvent(ctx, input)
}

func NewCostCenterService(repo CostCenterRepository) *CostCenterService {
	return &CostCenterService{repo: repo}
}

func (s *CostCenterService) ListEvents(ctx context.Context, filter CostCenterReportFilter, page, pageSize int) ([]CostCenterEvent, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	return s.repo.ListEvents(ctx, filter, page, pageSize)
}

func (s *CostCenterService) Summarize(ctx context.Context, filter CostCenterReportFilter) (*CostCenterSummary, error) {
	return s.repo.Summarize(ctx, filter)
}

func (s *CostCenterService) CreateExpensePlan(ctx context.Context, in *CreateExpensePlanInput) (*ExpensePlan, error) {
	if in == nil || in.AmountUSD <= 0 || in.IntervalValue <= 0 {
		return nil, ErrInvalidCostCenterAmount
	}
	return s.repo.CreateExpensePlan(ctx, in)
}
func (s *CostCenterService) MaterializeExpensePlans(ctx context.Context, at time.Time) (int, error) {
	return s.repo.MaterializeExpensePlans(ctx, at)
}
func (s *CostCenterService) UpdateEventStatus(ctx context.Context, id int64, status, reason string, operator *int64) (*CostCenterEvent, error) {
	if status != "settled" && status != "cancelled" && status != "pending" {
		return nil, ErrInvalidCostCenterStatus
	}
	return s.repo.UpdateEventStatus(ctx, id, status, reason, operator)
}
func (s *CostCenterService) ReverseEvent(ctx context.Context, id int64, reason string, operator *int64) (*CostCenterEvent, error) {
	return s.repo.ReverseEvent(ctx, id, reason, operator)
}
func (s *CostCenterService) Reconcile(ctx context.Context, f CostCenterReportFilter) (*CostCenterReconciliation, error) {
	return s.repo.Reconcile(ctx, f)
}
func (s *CostCenterService) SnapshotSubscriptionEntitlement(ctx context.Context, in *SubscriptionEntitlementSnapshot) error {
	if in == nil || in.OrderID <= 0 {
		return ErrInvalidCostCenterAmount
	}
	return s.repo.SnapshotSubscriptionEntitlement(ctx, in)
}
func (s *CostCenterService) RecognizeSubscriptionUsage(ctx context.Context, entitlementID int64, requestID string, tokens int64, standardCost float64, occurred time.Time) (*CostCenterEvent, error) {
	if entitlementID <= 0 || requestID == "" || tokens <= 0 {
		return nil, ErrInvalidCostCenterAmount
	}
	return s.repo.RecognizeSubscriptionUsage(ctx, entitlementID, requestID, tokens, standardCost, occurred)
}

// RecognizeSubscriptionUsageForUsage resolves the frozen entitlement for a
// finalized usage row. It is intentionally kept as an optional extension so
// existing CostCenterRepository test doubles and integrations remain valid.
func (s *CostCenterService) RecognizeSubscriptionUsageForUsage(ctx context.Context, userID int64, groupID *int64, requestID string, tokens int64, standardCost float64, occurred time.Time) (*CostCenterEvent, error) {
	if userID <= 0 || groupID == nil || *groupID <= 0 || requestID == "" || tokens <= 0 || standardCost <= 0 {
		return nil, ErrInvalidCostCenterAmount
	}
	repo, ok := s.repo.(interface {
		RecognizeSubscriptionUsageForUsage(context.Context, int64, *int64, string, int64, float64, time.Time) (*CostCenterEvent, error)
	})
	if !ok {
		return nil, nil
	}
	return repo.RecognizeSubscriptionUsageForUsage(ctx, userID, groupID, requestID, tokens, standardCost, occurred)
}
