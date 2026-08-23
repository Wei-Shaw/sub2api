package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestCostCenterWhereUsesInclusiveStartExclusiveEnd(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	accountID := int64(7)
	where, args := costCenterWhere(service.CostCenterReportFilter{Start: start, End: end, AccountID: &accountID, SourceType: "subscription"})
	if where != "cost_center_events.occurred_at >= $1 AND cost_center_events.occurred_at < $2 AND cost_center_events.source_type <> 'upstream' AND NOT EXISTS (SELECT 1 FROM cost_center_events upstream_event WHERE upstream_event.id=cost_center_events.reversal_of AND upstream_event.source_type='upstream') AND cost_center_events.account_id=$3 AND cost_center_events.source_type=$4" {
		t.Fatalf("unexpected report predicate: %s", where)
	}
	if len(args) != 4 || args[0] != start || args[1] != end || args[2] != accountID || args[3] != "subscription" {
		t.Fatalf("unexpected report args: %#v", args)
	}
}

func TestCreateEventPersistsIdempotentSettledEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	repo := NewCostCenterRepository(db)
	occurred := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	key := "expense-plan:11:2026-01-02"
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO cost_center_events")).
		WithArgs("expense", "pending", "recurring", "expense-plan:11:2026-01-02", &key, nil, nil, nil, "", nil, "", "proxy", 10.0, nil, nil, nil, occurred, "", sqlmock.AnyArg(), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_type", "status", "source_type", "source_id", "account_id", "user_id", "plan_id", "platform", "group_id", "model", "category", "amount_usd", "original_amount", "original_currency", "fx_rate", "occurred_at", "note", "metadata", "operator_id", "reversal_of", "created_at"}).AddRow(3, "expense", "pending", "recurring", "expense-plan:11:2026-01-02", nil, nil, nil, "", nil, "", "proxy", 10.0, nil, nil, nil, occurred, "", []byte(`{}`), nil, nil, occurred))
	_, err = repo.CreateEvent(context.Background(), &service.CreateCostCenterEventInput{EventType: "expense", Status: "pending", SourceType: "recurring", SourceID: &key, IdempotencyKey: &key, Category: "proxy", AmountUSD: 10, OccurredAt: &occurred})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListCostCenterEventsExcludesUpstreamAndHydratesNames(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	repo := NewCostCenterRepository(db)
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	accountID, userID, operatorID := int64(7), int64(8), int64(9)
	wherePattern := regexp.QuoteMeta("cost_center_events.source_type <> 'upstream'")
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM cost_center_events WHERE .*"+wherePattern).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	columns := []string{"id", "event_type", "status", "source_type", "source_id", "account_id", "user_id", "plan_id", "platform", "group_id", "model", "category", "amount_usd", "original_amount", "original_currency", "fx_rate", "occurred_at", "note", "metadata", "operator_id", "reversal_of", "created_at", "account_name", "user_name", "operator_name"}
	mock.ExpectQuery("SELECT cost_center_events.id.*LEFT JOIN accounts.*LEFT JOIN users u.*LEFT JOIN users op.*"+wherePattern).
		WithArgs(start, end, 50, 0).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "expense", "settled", "account", nil, accountID, userID, nil, "openai", nil, "gpt", "account_expense", 12.0, nil, nil, nil, start, "renewal", []byte(`{}`), operatorID, nil, start, "primary-account", "consumer@example.com", "admin@example.com"))

	events, total, err := repo.ListEvents(context.Background(), service.CostCenterReportFilter{Start: start, End: end}, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(events) != 1 {
		t.Fatalf("unexpected page: total=%d events=%+v", total, events)
	}
	if events[0].AccountName != "primary-account" || events[0].UserName != "consumer@example.com" || events[0].OperatorName != "admin@example.com" {
		t.Fatalf("names were not hydrated: %+v", events[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSummarizeIncludesSettledRebateAmountNetOfReversals(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	repo := NewCostCenterRepository(db)
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	mock.ExpectQuery("SELECT .*category='rebate'.*FROM cost_center_events WHERE").
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"cash", "realized", "promo", "expenses", "rebate", "pending", "unknown"}).
			AddRow(100.0, 80.0, 5.0, 30.0, 12.5, 4.0, 0.0))
	mock.ExpectQuery("SELECT .*FROM cost_center_subscription_entitlements WHERE").
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"deferred", "expired"}).AddRow(20.0, 3.0))

	summary, err := repo.Summarize(context.Background(), service.CostCenterReportFilter{Start: start, End: end})
	if err != nil {
		t.Fatal(err)
	}
	if summary.RebateAmount != 12.5 {
		t.Fatalf("unexpected rebate amount: %v", summary.RebateAmount)
	}
	if summary.CashProfit != 70 || summary.OperatingProfit != 50 {
		t.Fatalf("rebate breakdown must not be subtracted twice: %+v", summary)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReverseEventAppendsCompensatingEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	repo := NewCostCenterRepository(db)
	occurred := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	eventColumns := []string{"id", "event_type", "status", "source_type", "source_id", "account_id", "user_id", "plan_id", "platform", "group_id", "model", "category", "amount_usd", "original_amount", "original_currency", "fx_rate", "occurred_at", "note", "metadata", "operator_id", "reversal_of", "created_at"}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,event_type,status,source_type,source_id,account_id,user_id,plan_id,platform,group_id,model,category,amount_usd,original_amount,original_currency,fx_rate,occurred_at,note,metadata,operator_id,reversal_of,created_at FROM cost_center_events WHERE id=$1")).
		WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows(eventColumns).AddRow(9, "expense", "settled", "manual", "source-9", nil, nil, nil, "", nil, "", "proxy", 5.0, nil, nil, nil, occurred, "", []byte(`{}`), nil, nil, occurred))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO cost_center_events")).WillReturnRows(sqlmock.NewRows(eventColumns).AddRow(10, "reversal", "settled", "reversal", "reversal:9", nil, nil, nil, "", nil, "", "proxy", 5.0, nil, nil, nil, occurred, "bad", []byte(`{"reason":"bad"}`), nil, 9, occurred))
	_, err = repo.ReverseEvent(context.Background(), 9, "bad", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
