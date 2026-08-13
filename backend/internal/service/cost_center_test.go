package service

import (
	"context"
	"testing"
	"time"
)

type costCenterRepoFake struct{ events []CostCenterEvent }

func (f *costCenterRepoFake) CreateEvent(_ context.Context, in *CreateCostCenterEventInput) (*CostCenterEvent, error) {
	e := CostCenterEvent{EventType: in.EventType, Status: in.Status, SourceType: in.SourceType, AmountUSD: in.AmountUSD}
	f.events = append(f.events, e)
	return &e, nil
}
func (f *costCenterRepoFake) ListEvents(context.Context, CostCenterReportFilter, int, int) ([]CostCenterEvent, int64, error) {
	return f.events, int64(len(f.events)), nil
}
func (f *costCenterRepoFake) Summarize(context.Context, CostCenterReportFilter) (*CostCenterSummary, error) {
	return &CostCenterSummary{CashIncome: 100, RealizedIncome: 42, SettledExpenses: 10}, nil
}

func TestWriteCostCenterUsageEventsDoesNotCreateAutomaticUpstreamExpense(t *testing.T) {
	repo := &costCenterRepoFake{}
	when := time.Date(2026, 8, 14, 1, 2, 3, 0, time.UTC)
	rate := 2.0
	usage := &UsageLog{
		RequestID:             "req-cost-center-manual-upstream",
		UserID:                11,
		AccountID:             22,
		Model:                 "test-model",
		ActualCost:            3,
		TotalCost:             4,
		AccountRateMultiplier: &rate,
		CreatedAt:             when,
	}

	writeCostCenterUsageEvents(context.Background(), repo, usage, false)

	if len(repo.events) != 1 {
		t.Fatalf("expected only the consumption event, got %d events: %+v", len(repo.events), repo.events)
	}
	if repo.events[0].EventType != CostEventConsumption || repo.events[0].SourceType != "paid_balance" {
		t.Fatalf("unexpected cost-center event: %+v", repo.events[0])
	}
}

func TestAsyncVideoCostCenterEventsDoNotCreateAutomaticUpstreamExpense(t *testing.T) {
	repo := &costCenterRepoFake{}
	finished := time.Date(2026, 8, 14, 2, 3, 4, 0, time.UTC)
	accountID := int64(33)
	model := "video-model"
	svc := &AsyncVideoService{costCenter: repo}
	svc.writeCostCenterEvents(context.Background(), &AsyncVideoTask{
		InternalRequestID: "req-video-manual-upstream",
		UserID:            12,
		AccountID:         &accountID,
		RequestedModel:    model,
		FinishedAt:        &finished,
		UpstreamCost:      99,
	}, 5)

	if len(repo.events) != 1 {
		t.Fatalf("expected only the video consumption event, got %d events: %+v", len(repo.events), repo.events)
	}
	if repo.events[0].EventType != CostEventConsumption || repo.events[0].SourceType != "paid_balance" {
		t.Fatalf("unexpected video cost-center event: %+v", repo.events[0])
	}
}
func (f *costCenterRepoFake) CreateExpensePlan(context.Context, *CreateExpensePlanInput) (*ExpensePlan, error) {
	return nil, nil
}
func (f *costCenterRepoFake) MaterializeExpensePlans(context.Context, time.Time) (int, error) {
	return 0, nil
}
func (f *costCenterRepoFake) UpdateEventStatus(context.Context, int64, string, string, *int64) (*CostCenterEvent, error) {
	return nil, nil
}
func (f *costCenterRepoFake) ReverseEvent(context.Context, int64, string, *int64) (*CostCenterEvent, error) {
	return nil, nil
}
func (f *costCenterRepoFake) Reconcile(context.Context, CostCenterReportFilter) (*CostCenterReconciliation, error) {
	return &CostCenterReconciliation{}, nil
}
func (f *costCenterRepoFake) SnapshotSubscriptionEntitlement(context.Context, *SubscriptionEntitlementSnapshot) error {
	return nil
}
func (f *costCenterRepoFake) RecognizeSubscriptionUsage(context.Context, int64, string, int64, float64, time.Time) (*CostCenterEvent, error) {
	return nil, nil
}

func TestCostCenterServiceDefaultsAndSummary(t *testing.T) {
	repo := &costCenterRepoFake{}
	svc := NewCostCenterService(repo)
	when := time.Now()
	e, err := svc.CreateEvent(context.Background(), &CreateCostCenterEventInput{EventType: CostEventExpense, AmountUSD: 10, OccurredAt: &when})
	if err != nil {
		t.Fatal(err)
	}
	if e.Status != "settled" || e.SourceType != "manual" {
		t.Fatalf("defaults not applied: %+v", e)
	}
	s, err := svc.Summarize(context.Background(), CostCenterReportFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if s.CashIncome-s.SettledExpenses != 90 {
		t.Fatalf("cash profit formula inputs changed: %+v", s)
	}
}

func TestCostCenterServiceClassifiesAccountTargetedExpense(t *testing.T) {
	repo := &costCenterRepoFake{}
	svc := NewCostCenterService(repo)
	accountID := int64(17)

	event, err := svc.CreateEvent(context.Background(), &CreateCostCenterEventInput{
		EventType:  CostEventExpense,
		SourceType: "manual",
		AccountID:  &accountID,
		AmountUSD:  8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.SourceType != "account" {
		t.Fatalf("expected account source type, got %q", event.SourceType)
	}
}

func TestCostCenterServiceRejectsNonPositiveAmount(t *testing.T) {
	_, err := NewCostCenterService(&costCenterRepoFake{}).CreateEvent(context.Background(), &CreateCostCenterEventInput{EventType: CostEventExpense, AmountUSD: 0})
	if err != ErrInvalidCostCenterAmount {
		t.Fatalf("expected invalid amount, got %v", err)
	}
}

func TestCostCenterServiceRejectsInvalidStatus(t *testing.T) {
	_, err := NewCostCenterService(&costCenterRepoFake{}).UpdateEventStatus(context.Background(), 1, "bogus", "reason", nil)
	if err != ErrInvalidCostCenterStatus {
		t.Fatalf("expected invalid status, got %v", err)
	}
}

func TestCostCenterServiceRequiresRecognitionInputs(t *testing.T) {
	_, err := NewCostCenterService(&costCenterRepoFake{}).RecognizeSubscriptionUsage(context.Background(), 0, "", 0, 1, time.Now())
	if err != ErrInvalidCostCenterAmount {
		t.Fatalf("expected invalid recognition input, got %v", err)
	}
}
