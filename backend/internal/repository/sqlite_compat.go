package repository

import (
	"context"
	"database/sql/driver"
	"time"

	"modernc.org/sqlite"
)

func init() {
	// Register a NOW() scalar function for SQLite so that any DDL we run
	// against an in-memory SQLite test DB (e.g. user_ldap_profiles table
	// creation) doesn't fail on an unknown function.
	_ = sqlite.RegisterFunction("now", &sqlite.FunctionImpl{
		NArgs:         0,
		Deterministic: false,
		Scalar: func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			return time.Now().UTC().Format("2006-01-02 15:04:05"), nil
		},
	})
}

// isSQLite returns true when the given executor is backed by a SQLite database.
func isSQLite(sqlq sqlExecutor) bool {
	if sqlq == nil {
		return false
	}
	rows, err := sqlq.QueryContext(context.Background(), "SELECT sqlite_version()")
	if err != nil {
		return false
	}
	_ = rows.Close()
	return true
}
