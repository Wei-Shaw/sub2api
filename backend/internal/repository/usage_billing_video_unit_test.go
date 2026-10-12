//go:build unit

package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func videoBillingCommandForTest() *service.UsageBillingCommand {
	return &service.UsageBillingCommand{
		RequestID: "grok-video:atomic", APIKeyID: 2, UserID: 1, AccountID: 3,
		BalanceCost: 1.25, APIKeyQuotaCost: 1.25,
		VideoUsageLog: &service.UsageLog{
			RequestID: "grok-video:atomic", APIKeyID: 2, UserID: 1, AccountID: 3,
			Model: "grok-imagine-video", VideoCount: 1, ActualCost: 1.25,
		},
	}
}

func expectNewVideoBilling(mock sqlmock.Sqlmock, cmd *service.UsageBillingCommand) {
	cmd.Normalize()
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO usage_billing_dedup").
		WithArgs(cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT request_fingerprint FROM usage_billing_dedup_archive").
		WithArgs(cmd.RequestID, cmd.APIKeyID).
		WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}))
	mock.ExpectQuery(conditionalBalanceDeductSQL).
		WithArgs(cmd.BalanceCost, cmd.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(98.75))
	mock.ExpectQuery("UPDATE api_keys").
		WithArgs(cmd.APIKeyQuotaCost, cmd.APIKeyID, service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).
		WillReturnRows(sqlmock.NewRows([]string{"exhausted"}).AddRow(false))
}

func TestUsageBillingVideo_InsertFailureRollsBackThenRetries(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &usageBillingRepository{db: db}
	cmd := videoBillingCommandForTest()
	insertErr := errors.New("usage log insert failed")

	expectNewVideoBilling(mock, cmd)
	mock.ExpectQuery("INSERT INTO usage_logs").WillReturnError(insertErr)
	mock.ExpectRollback()
	result, err := repo.Apply(context.Background(), cmd)
	require.ErrorIs(t, err, insertErr)
	require.Nil(t, result)
	require.Zero(t, cmd.VideoUsageLog.ID)

	expectNewVideoBilling(mock, cmd)
	createdAt := time.Now()
	mock.ExpectQuery("INSERT INTO usage_logs").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(9, createdAt))
	mock.ExpectCommit()
	result, err = repo.Apply(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Equal(t, int64(9), cmd.VideoUsageLog.ID)
	require.Equal(t, createdAt, cmd.VideoUsageLog.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageBillingVideo_CommitFailureDoesNotPublishLogID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	cmd := videoBillingCommandForTest()
	expectNewVideoBilling(mock, cmd)
	mock.ExpectQuery("INSERT INTO usage_logs").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(9, time.Now()))
	commitErr := errors.New("commit failed")
	mock.ExpectCommit().WillReturnError(commitErr)
	result, err := (&usageBillingRepository{db: db}).Apply(context.Background(), cmd)
	require.ErrorIs(t, err, commitErr)
	require.Nil(t, result)
	require.Zero(t, cmd.VideoUsageLog.ID)
	require.True(t, cmd.VideoUsageLog.CreatedAt.IsZero())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageBillingVideo_DuplicateRequiresExistingLog(t *testing.T) {
	for _, archived := range []bool{false, true} {
		for _, missing := range []bool{false, true} {
			t.Run(fmtVideoDuplicateCase(archived, missing), func(t *testing.T) {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()
				cmd := videoBillingCommandForTest()
				cmd.Normalize()
				mock.ExpectBegin()
				claim := mock.ExpectQuery("INSERT INTO usage_billing_dedup")
				if archived {
					claim.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
					mock.ExpectQuery("SELECT request_fingerprint FROM usage_billing_dedup_archive").
						WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}).AddRow(cmd.RequestFingerprint))
				} else {
					claim.WillReturnRows(sqlmock.NewRows([]string{"id"}))
					mock.ExpectQuery("SELECT request_fingerprint FROM usage_billing_dedup").
						WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}).AddRow(cmd.RequestFingerprint))
				}
				rows := sqlmock.NewRows([]string{"id"})
				if !missing {
					rows.AddRow(9)
				}
				mock.ExpectQuery("SELECT id FROM usage_logs").WithArgs(cmd.RequestID, cmd.APIKeyID).WillReturnRows(rows)
				mock.ExpectRollback()
				result, err := (&usageBillingRepository{db: db}).Apply(context.Background(), cmd)
				if missing {
					require.ErrorIs(t, err, sql.ErrNoRows)
					require.Nil(t, result)
				} else {
					require.NoError(t, err)
					require.False(t, result.Applied)
				}
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func fmtVideoDuplicateCase(archived, missing bool) string {
	name := "active"
	if archived {
		name = "archived"
	}
	if missing {
		return name + "/missing_log"
	}
	return name + "/persisted_log"
}

func TestUsageBillingVideo_ExistingUnbilledLogRollsBackCharge(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	cmd := videoBillingCommandForTest()
	expectNewVideoBilling(mock, cmd)
	mock.ExpectQuery("INSERT INTO usage_logs").WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}))
	mock.ExpectQuery("SELECT id, created_at FROM usage_logs").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(9, time.Now()))
	mock.ExpectRollback()
	result, err := (&usageBillingRepository{db: db}).Apply(context.Background(), cmd)
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
	require.Nil(t, result)
	require.Zero(t, cmd.VideoUsageLog.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageBillingVideo_RejectsMismatchedLogBeforeBilling(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	cmd := videoBillingCommandForTest()
	cmd.VideoUsageLog.APIKeyID++
	_, err = (&usageBillingRepository{db: db}).Apply(context.Background(), cmd)
	require.ErrorContains(t, err, "does not match")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageBillingVideo_NonVideoApplyDoesNotInsertLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	cmd := videoBillingCommandForTest()
	cmd.VideoUsageLog = nil
	expectNewVideoBilling(mock, cmd)
	mock.ExpectCommit()
	result, err := (&usageBillingRepository{db: db}).Apply(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NoError(t, mock.ExpectationsWereMet())
}
