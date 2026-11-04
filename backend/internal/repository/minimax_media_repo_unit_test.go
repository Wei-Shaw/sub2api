//go:build unit

package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func mediaRepoTask() *service.MiniMaxMediaTask {
	one, half := 1.0, .5
	return &service.MiniMaxMediaTask{ID: "mm_test", Kind: "video", Model: minimax.H3, Resolution: "2K", Duration: 6, UserID: 1, APIKeyID: 2, GroupID: 3, AccountID: 4, AccountType: "apikey", Origin: "https://api.minimax.cn", IdempotencyKey: "client-key", RequestHash: "hash", ReservedUSD: 5, Status: "succeeded", BillingState: "reserved", Pricing: service.MiniMaxMediaPricing{Resolution: "2K", CostCurrency: "USD", Multiplier: &one, Retail: service.MiniMaxMediaRates{OutputSecond: &half, InputSecond: &half, ExtraImage: &half}, Cost: service.MiniMaxMediaRates{OutputSecond: &half, InputSecond: &half, ExtraImage: &half}}}
}
func mediaRepoRow(t *testing.T, task *service.MiniMaxMediaTask) *sqlmock.Rows {
	t.Helper()
	snapshot, err := json.Marshal(task)
	require.NoError(t, err)
	var bill driver.Value
	if task.Bill != nil {
		b, err := json.Marshal(task.Bill)
		require.NoError(t, err)
		bill = b
	}
	return sqlmock.NewRows(strings.Split(mediaColumns, ",")).AddRow(task.ID, task.UserID, task.APIKeyID, task.GroupID, task.AccountID, task.SubscriptionID, task.AccountType, nil, task.Origin, task.Kind, task.IdempotencyKey, task.RequestHash, snapshot, task.ReservedUSD, []byte(`{}`), task.Status, task.BillingState, bill, nil, time.Now())
}
func TestMiniMaxMediaReserveInsufficientRollsBack(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	m.ExpectBegin()
	m.ExpectQuery("INSERT INTO minimax_media_tasks").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("mm_test"))
	m.ExpectQuery("SELECT id FROM users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	m.ExpectExec("UPDATE users SET balance=balance-").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectRollback()
	_, created, err := (&miniMaxMediaRepository{db: db}).Reserve(context.Background(), mediaRepoTask())
	require.ErrorIs(t, err, service.ErrMiniMaxMediaFunds)
	require.False(t, created)
	require.NoError(t, m.ExpectationsWereMet())
}
func TestMiniMaxMediaReserveIdempotentDoesNotTouchFunds(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "replay", true: "conflict"}[conflict], func(t *testing.T) {
			db, m, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			original := mediaRepoTask()
			if conflict {
				original.RequestHash = "different"
			}
			m.ExpectBegin()
			m.ExpectQuery("INSERT INTO minimax_media_tasks").WillReturnRows(sqlmock.NewRows([]string{"id"}))
			m.ExpectQuery("SELECT .* FROM minimax_media_tasks").WillReturnRows(mediaRepoRow(t, original))
			m.ExpectRollback()
			task, created, err := (&miniMaxMediaRepository{db: db}).Reserve(context.Background(), mediaRepoTask())
			require.False(t, created)
			if conflict {
				require.ErrorIs(t, err, service.ErrMiniMaxMediaConflict)
			} else {
				require.NoError(t, err)
				require.Equal(t, original.ID, task.ID)
			}
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}
func TestMiniMaxMediaSettleRequiresReceiptAndSkipsTerminal(t *testing.T) {
	for _, state := range []string{"reserved", "settled", "released"} {
		t.Run(state, func(t *testing.T) {
			db, m, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			task := mediaRepoTask()
			task.BillingState = state
			m.ExpectBegin()
			m.ExpectQuery("SELECT .* FROM minimax_media_tasks").WillReturnRows(mediaRepoRow(t, task))
			m.ExpectRollback()
			_, err = (&miniMaxMediaRepository{db: db}).Settle(context.Background(), task.ID)
			if state == "reserved" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}
func TestMiniMaxMediaSettlementFailureKeepsReservationAtomic(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	task := mediaRepoTask()
	task.Bill = &service.MiniMaxMediaBill{Usage: minimax.Usage{OutputSeconds: 6}}
	m.ExpectBegin()
	m.ExpectQuery("SELECT .* FROM minimax_media_tasks").WillReturnRows(mediaRepoRow(t, task))
	m.ExpectQuery("SELECT id FROM users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	m.ExpectExec("UPDATE users SET balance=balance\\+").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("UPDATE api_keys SET quota_used").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectQuery("INSERT INTO usage_billing_dedup").WillReturnError(errors.New("database unavailable"))
	m.ExpectRollback()
	_, err = (&miniMaxMediaRepository{db: db}).Settle(context.Background(), task.ID)
	require.Error(t, err)
	require.NoError(t, m.ExpectationsWereMet())
}
