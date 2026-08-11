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
	if where != "occurred_at >= $1 AND occurred_at < $2 AND account_id=$3 AND source_type=$4" {
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
