package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestLoadEnterpriseIdentities(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`(?s)SELECT u\.id, COALESCE\(o\.company_id, ''\), o\.name, m\.role.*JOIN organization_memberships m ON m\.user_id = u\.id.*JOIN organizations o ON o\.id = m\.organization_id.*u\.id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "company_id", "company_name", "organization_role"}).
			AddRow(int64(41), "c123456789012345", "Acme AI", "owner").
			AddRow(int64(42), "c123456789012345", "Acme AI", "member").
			AddRow(int64(43), "", "Legacy Company", "member"))

	repo := &userRepository{sql: db}
	identities, err := repo.loadEnterpriseIdentities(context.Background(), []int64{41, 42, 43})
	require.NoError(t, err)
	require.Equal(t, map[int64]userEnterpriseIdentity{
		41: {CompanyID: "c123456789012345", CompanyName: "Acme AI", OrganizationRole: "owner"},
		42: {CompanyID: "c123456789012345", CompanyName: "Acme AI", OrganizationRole: "member"},
	}, identities)
	require.NoError(t, mock.ExpectationsWereMet())
}
