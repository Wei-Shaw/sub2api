//go:build unit

package service_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

func createUserAllowedAccountsTestTable(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS user_allowed_accounts (
	user_id INTEGER NOT NULL,
	account_id INTEGER NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (user_id, account_id)
)`)
	require.NoError(t, err)
}
