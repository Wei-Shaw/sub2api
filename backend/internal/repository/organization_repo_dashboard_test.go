package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOrganizationDashboardIncludesIAMUsersInAccountCounts(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &organizationRepository{db: db}
	effectiveAt := time.Now().UTC().Add(-24 * time.Hour)

	mock.ExpectQuery("SELECT o.id, o.account_id").
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{
			"organization_id", "account_id", "company_id", "owner_user_id", "name", "organization_status",
			"membership_id", "role", "membership_status", "authz_generation", "effective_at", "policy_names", "actions",
		}).AddRow(int64(7), "account-1", "company-1", int64(99), "Example", service.OrganizationStatusActive,
			int64(11), service.OrganizationRoleOwner, service.MembershipStatusActive, int64(1), effectiveAt, "{}", "{}"))
	mock.ExpectQuery("SELECT count\\(\\*\\), count\\(\\*\\) FILTER \\(WHERE created_at >= \\$2\\)").
		WithArgs(int64(7), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"total_users", "today_new_users", "total_iam_users", "active_iam_users"}).
			AddRow(int64(4), int64(0), int64(3), int64(2)))
	mock.ExpectQuery("FROM api_keys k JOIN organization_memberships").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"total_api_keys", "active_api_keys"}).AddRow(int64(0), int64(0)))
	mock.ExpectQuery("WITH used_accounts AS").
		WithArgs(int64(7), effectiveAt, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"total_accounts", "normal_accounts", "error_accounts", "ratelimit_accounts", "overload_accounts"}).
			AddRow(int64(2), int64(1), int64(0), int64(0), int64(0)))
	mock.ExpectQuery("SELECT count\\(\\*\\), COALESCE\\(sum\\(input_tokens\\),0\\)").
		WithArgs(int64(7), effectiveAt).
		WillReturnRows(sqlmock.NewRows([]string{"requests", "input", "output", "cache_creation", "cache_read", "cost", "actual_cost", "account_cost", "duration"}).
			AddRow(int64(0), int64(0), int64(0), int64(0), int64(0), float64(0), float64(0), float64(0), float64(0)))
	mock.ExpectQuery("SELECT count\\(\\*\\), count\\(DISTINCT user_id\\)").
		WithArgs(int64(7), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"requests", "active_users", "input", "output", "cache_creation", "cache_read", "cost", "actual_cost", "account_cost"}).
			AddRow(int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), float64(0), float64(0), float64(0)))
	mock.ExpectQuery("SELECT count\\(\\*\\), COALESCE\\(sum\\(input_tokens\\+output_tokens").
		WithArgs(int64(7), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"requests", "tokens"}).AddRow(int64(0), int64(0)))

	stats, err := repo.OrganizationDashboard(context.Background(), 99)
	require.NoError(t, err)
	require.EqualValues(t, 5, stats.TotalAccounts)
	require.EqualValues(t, 3, stats.NormalAccounts)
	require.NoError(t, mock.ExpectationsWereMet())
}
