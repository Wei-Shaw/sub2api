package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestDingTalkSelfGrantUsesBudgetScopeAndIdempotency(t *testing.T) {
	for _, scenario := range []string{"success", "replay", "exceeds-budget", "revoked", "outside-scope", "unlinked"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			g := DingTalkQuotaGrant{ActorID: 2, TargetID: 2, AppID: "a", DepartmentID: 3, AmountCents: 1000, RequestID: "self-grant-request-0001"}
			mock.ExpectBegin()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
			prior := mock.ExpectQuery("SELECT id,target_id").WithArgs(int64(2), g.RequestID)
			if scenario == "replay" {
				prior.WillReturnRows(sqlmock.NewRows([]string{"id", "target_id", "app_id", "department_id", "amount_cents", "created_at"}).AddRow(1, 2, "a", 3, 1000, time.Now()))
				mock.ExpectRollback()
			} else {
				prior.WillReturnError(sql.ErrNoRows)
				used := int64(0)
				if scenario == "exceeds-budget" {
					used = 49500
				}
				mock.ExpectQuery("SELECT limit_cents,used_cents,enabled").WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"limit", "used", "enabled"}).AddRow(50000, used, scenario != "revoked"))
				if scenario != "exceeds-budget" && scenario != "revoked" {
					mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("a").WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectQuery("SELECT EXISTS.*dingtalk_directory_snapshots").WithArgs("a").WillReturnRows(sqlmock.NewRows([]string{"fresh"}).AddRow(true))
					mock.ExpectQuery("WITH RECURSIVE scope").WithArgs(int64(2), "a", int64(3)).WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(scenario != "outside-scope"))
					if scenario != "outside-scope" {
						mock.ExpectQuery("SELECT EXISTS.*dingtalk_members").WithArgs("a", int64(3), int64(2), "dingtalk:a").WillReturnRows(sqlmock.NewRows([]string{"member"}).AddRow(scenario != "unlinked"))
						if scenario == "success" {
							mock.ExpectExec("UPDATE users SET balance").WithArgs(int64(2), int64(1000)).WillReturnResult(sqlmock.NewResult(0, 1))
							mock.ExpectExec("UPDATE dingtalk_manager_budgets SET used_cents").WithArgs(int64(2), int64(1000)).WillReturnResult(sqlmock.NewResult(0, 1))
							mock.ExpectQuery("INSERT INTO dingtalk_quota_grants").WithArgs(int64(2), int64(2), "a", int64(3), int64(1000), g.RequestID).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, time.Now()))
						}
					}
				}
				if scenario == "success" {
					mock.ExpectCommit()
				} else {
					mock.ExpectRollback()
				}
			}
			result, err := NewDingTalkOrganizationService(db, nil).Grant(context.Background(), g, false)
			if scenario == "success" || scenario == "replay" {
				require.NoError(t, err)
				require.Equal(t, result.ActorID, result.TargetID)
			} else {
				require.Error(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
