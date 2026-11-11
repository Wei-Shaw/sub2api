package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSyncGroupRateMultipliersCommitsInOneTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewUserGroupRateRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE user_group_rate_multipliers").
		WithArgs(int64(10), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM user_group_rate_multipliers").
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO user_group_rate_multipliers").
		WithArgs(int64(10), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repo.SyncGroupRateMultipliers(context.Background(), 10, []service.GroupRateMultiplierInput{
		{UserID: 1, RateMultiplier: 1.5},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncGroupRateMultipliersRollsBackOnInsertFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewUserGroupRateRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE user_group_rate_multipliers").
		WithArgs(int64(10), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM user_group_rate_multipliers").
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO user_group_rate_multipliers").
		WithArgs(int64(10), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	err = repo.SyncGroupRateMultipliers(context.Background(), 10, []service.GroupRateMultiplierInput{
		{UserID: 1, RateMultiplier: 1.5},
	})
	require.ErrorContains(t, err, "insert failed")
	require.NoError(t, mock.ExpectationsWereMet(), "the destructive UPDATE/DELETE must be rolled back")
}

func TestSyncGroupRPMOverridesCommitsInOneTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewUserGroupRateRepository(db)

	override := 5
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE user_group_rate_multipliers").
		WithArgs(int64(10), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM user_group_rate_multipliers").
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO user_group_rate_multipliers").
		WithArgs(int64(10), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repo.SyncGroupRPMOverrides(context.Background(), 10, []service.GroupRPMOverrideInput{
		{UserID: 1, RPMOverride: &override},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncGroupRPMOverridesRollsBackOnInsertFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewUserGroupRateRepository(db)

	override := 5
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE user_group_rate_multipliers").
		WithArgs(int64(10), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM user_group_rate_multipliers").
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO user_group_rate_multipliers").
		WithArgs(int64(10), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	err = repo.SyncGroupRPMOverrides(context.Background(), 10, []service.GroupRPMOverrideInput{
		{UserID: 1, RPMOverride: &override},
	})
	require.ErrorContains(t, err, "insert failed")
	require.NoError(t, mock.ExpectationsWereMet(), "the destructive UPDATE/DELETE must be rolled back")
}

func TestDedupeGroupRateMultiplierEntriesKeepsLastValue(t *testing.T) {
	got := dedupeGroupRateMultiplierEntries([]service.GroupRateMultiplierInput{
		{UserID: 1, RateMultiplier: 1},
		{UserID: 2, RateMultiplier: 2},
		{UserID: 1, RateMultiplier: 3},
	})
	require.Equal(t, []service.GroupRateMultiplierInput{
		{UserID: 1, RateMultiplier: 3},
		{UserID: 2, RateMultiplier: 2},
	}, got)
}

func TestDedupeGroupRPMOverrideEntriesKeepsLastValue(t *testing.T) {
	five := 5
	got := dedupeGroupRPMOverrideEntries([]service.GroupRPMOverrideInput{
		{UserID: 1},
		{UserID: 2, RPMOverride: &five},
		{UserID: 1, RPMOverride: &five},
	})
	require.Len(t, got, 2)
	require.Equal(t, int64(1), got[0].UserID)
	require.Equal(t, &five, got[0].RPMOverride)
	require.Equal(t, int64(2), got[1].UserID)
}
