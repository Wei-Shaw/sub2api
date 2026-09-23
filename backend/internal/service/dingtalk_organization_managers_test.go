package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestDingTalkManagerBudgetIncreaseAndIdempotency(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := NewDingTalkOrganizationService(db, nil)
	stamp := time.Now()
	for _, replay := range []bool{false, true} {
		mock.ExpectBegin()
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
		lookup := mock.ExpectQuery("SELECT id,manager_id,amount_cents").WithArgs(int64(1), "increase-request-0001")
		if replay {
			lookup.WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "amount_cents", "limit_cents_after", "created_at"}).AddRow(7, 2, 2500, 52500, stamp))
			mock.ExpectRollback()
		} else {
			lookup.WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery("UPDATE dingtalk_manager_budgets b SET limit_cents=limit_cents").WithArgs(int64(2), int64(2500), maxDingTalkBudgetCents).WillReturnRows(sqlmock.NewRows([]string{"limit_cents"}).AddRow(52500))
			mock.ExpectQuery("INSERT INTO dingtalk_manager_budget_increases").WithArgs(int64(1), int64(2), int64(2500), int64(52500), "increase-request-0001").WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(7, stamp))
			mock.ExpectCommit()
		}
		result, err := s.IncreaseManagerBudget(context.Background(), 1, 2, 2500, "increase-request-0001")
		require.NoError(t, err)
		require.EqualValues(t, 52500, result.LimitCentsAfter)
		require.EqualValues(t, 7, result.ID)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDingTalkBudgetIncreaseRollsBackWithoutAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id,manager_id,amount_cents").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("UPDATE dingtalk_manager_budgets").WillReturnRows(sqlmock.NewRows([]string{"limit_cents"}).AddRow(52500))
	mock.ExpectQuery("INSERT INTO dingtalk_manager_budget_increases").WillReturnError(errors.New("audit unavailable"))
	mock.ExpectRollback()
	_, err = NewDingTalkOrganizationService(db, nil).IncreaseManagerBudget(context.Background(), 1, 2, 2500, "increase-request-0001")
	require.ErrorContains(t, err, "audit unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDingTalkBudgetIncreaseRejectsChangedReplay(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id,manager_id,amount_cents").WillReturnRows(sqlmock.NewRows([]string{"id", "manager_id", "amount_cents", "limit_cents_after", "created_at"}).AddRow(7, 2, 2500, 52500, time.Now()))
	mock.ExpectRollback()
	_, err = NewDingTalkOrganizationService(db, nil).IncreaseManagerBudget(context.Background(), 1, 2, 3000, "increase-request-0001")
	require.ErrorContains(t, err, "another increase")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDingTalkManagerPermissionsDoNotWriteBudget(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT limit_cents,used_cents").WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"limit", "used"}).AddRow(60000, 10000))
	mock.ExpectExec("DELETE FROM dingtalk_department_managers").WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 2))
	departments := []DingTalkManagerDepartment{{AppID: "b", DepartmentID: 7}}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("b", int64(7)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO dingtalk_department_managers").WithArgs(int64(2), "b", int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, NewDingTalkOrganizationService(db, nil).PatchManager(context.Background(), 2, DingTalkManagerPatch{Departments: &departments}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDingTalkManagerEditRejectsStaleBudget(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT limit_cents,used_cents").WillReturnRows(sqlmock.NewRows([]string{"limit", "used"}).AddRow(60000, 10000))
	mock.ExpectRollback()
	expected, limit := int64(50000), int64(75000)
	err = NewDingTalkOrganizationService(db, nil).PatchManager(context.Background(), 2, DingTalkManagerPatch{LimitCents: &limit, ExpectedLimitCents: &expected})
	require.ErrorContains(t, err, "Budget changed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDingTalkManagerPagesAndHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("SELECT COUNT.*FROM dingtalk_manager_budgets").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
	mock.ExpectQuery("SELECT b.user_id.*LIMIT.*OFFSET").WithArgs(true, int64(0), 20, 20).WillReturnRows(sqlmock.NewRows([]string{"user_id", "username", "limit_cents", "used_cents", "enabled"}).AddRow(22, "Last manager", 50000, 0, true))
	mock.ExpectQuery("SELECT app_id,department_id").WithArgs(int64(22)).WillReturnRows(sqlmock.NewRows([]string{"app_id", "department_id"}))
	s := NewDingTalkOrganizationService(db, nil)
	managers, total, err := s.ManagerPage(context.Background(), 2)
	require.NoError(t, err)
	require.EqualValues(t, 21, total)
	require.Len(t, managers, 1)
	mock.ExpectQuery("SELECT COUNT.*FROM dingtalk_quota_grants WHERE actor_id").WithArgs(int64(22)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(121))
	mock.ExpectQuery("SELECT id,actor_id.*WHERE actor_id.*LIMIT.*OFFSET").WithArgs(int64(22), 20, 100).WillReturnRows(sqlmock.NewRows([]string{"id", "actor_id", "target_id", "app_id", "department_id", "amount_cents", "request_id", "created_at"}).AddRow(2, 22, 3, "a", 5, 100, "grant-history-0001", time.Now()))
	grants, total, err := s.ManagerGrants(context.Background(), 22, 6)
	require.NoError(t, err)
	require.EqualValues(t, 121, total)
	require.Len(t, grants, 1)
	require.EqualValues(t, 22, grants[0].ActorID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDingTalkCreateManagerCannotOverwriteExistingBudget(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status='active'").WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"active"}).AddRow(true))
	mock.ExpectQuery("INSERT INTO dingtalk_manager_budgets.*DO NOTHING").WithArgs(int64(2), int64(50000), true).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	err = NewDingTalkOrganizationService(db, nil).CreateManager(context.Background(), DingTalkManager{UserID: 2, LimitCents: 50000, Enabled: true})
	require.ErrorContains(t, err, "already exists")
	require.NoError(t, mock.ExpectationsWereMet())
}
