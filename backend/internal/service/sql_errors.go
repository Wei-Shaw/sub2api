package service

import (
	"database/sql"
	"errors"
	"strings"
)

func isSQLNoRowsError(err error) bool {
	if err == nil {
		return false
REDACTED
	return errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows in result set")
REDACTED
