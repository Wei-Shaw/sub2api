package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestWeb3IdentityRepositoryGetAddressByUserID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &web3IdentityRepository{db: db}
	query := regexp.QuoteMeta(`
		SELECT address
		FROM web3_identities
		WHERE user_id = $1 AND deleted_at IS NULL
	`)
	mock.ExpectQuery(query).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"address"}).AddRow("0x52908400098527886e0f7030069857d2e4169ee7"))

	address, found, err := repo.GetAddressByUserID(context.Background(), 11)

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "0x52908400098527886e0f7030069857d2e4169ee7", address)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWeb3IdentityRepositoryGetAddressByUserIDReturnsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &web3IdentityRepository{db: db}
	mock.ExpectQuery("SELECT address").
		WithArgs(int64(12)).
		WillReturnError(sql.ErrNoRows)

	address, found, err := repo.GetAddressByUserID(context.Background(), 12)

	require.NoError(t, err)
	require.False(t, found)
	require.Empty(t, address)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWeb3IdentityRepositoryGetUserIDByAddressOnlyReturnsActiveIdentity(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &web3IdentityRepository{db: db}
	query := regexp.QuoteMeta(`
		SELECT wi.user_id
		FROM web3_identities wi
		JOIN users u ON u.id = wi.user_id
		WHERE wi.address = $1
		  AND wi.deleted_at IS NULL
		  AND u.deleted_at IS NULL
	`)
	mock.ExpectQuery(query).
		WithArgs("0x52908400098527886e0f7030069857d2e4169ee7").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(int64(11)))

	userID, err := repo.GetUserIDByAddress(context.Background(), "0x52908400098527886e0f7030069857d2e4169ee7")

	require.NoError(t, err)
	require.Equal(t, int64(11), userID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWeb3IdentityRepositoryExistsByAddressOnlyChecksActiveIdentity(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &web3IdentityRepository{db: db}
	query := regexp.QuoteMeta(`
		SELECT EXISTS (
			SELECT 1
			FROM web3_identities
			WHERE address = $1 AND deleted_at IS NULL
		)
	`)
	mock.ExpectQuery(query).
		WithArgs("0x52908400098527886e0f7030069857d2e4169ee7").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	exists, err := repo.ExistsByAddress(context.Background(), "0x52908400098527886e0f7030069857d2e4169ee7")

	require.NoError(t, err)
	require.False(t, exists)
	require.NoError(t, mock.ExpectationsWereMet())
}
