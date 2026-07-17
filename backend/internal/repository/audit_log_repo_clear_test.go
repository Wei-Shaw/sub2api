package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAuditLogRepositoryClearAllCommitsTraceAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &auditLogRepository{db: db}
	trace := &service.AuditLog{CreatedAt: time.Unix(100, 0), Action: service.AuditActionAuditLogClear}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("LOCK TABLE audit_logs IN ACCESS EXCLUSIVE MODE")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM audit_logs")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectExec(regexp.QuoteMeta("TRUNCATE TABLE audit_logs")).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(auditLogInsertValuesMatcher(3)...).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	deleted, err := repo.ClearAll(context.Background(), trace)

	require.NoError(t, err)
	require.Equal(t, int64(3), deleted)
	require.Nil(t, trace.Extra, "repository must not mutate caller-owned trace")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAuditLogRepositoryClearAllRollsBackWhenTraceInsertFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &auditLogRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("LOCK TABLE audit_logs IN ACCESS EXCLUSIVE MODE")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM audit_logs")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectExec(regexp.QuoteMeta("TRUNCATE TABLE audit_logs")).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	deleted, err := repo.ClearAll(context.Background(), &service.AuditLog{})

	require.Error(t, err)
	require.Zero(t, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func auditLogInsertValuesMatcher(deleted int64) []driver.Value {
	args := make([]driver.Value, 16)
	for index := range args {
		args[index] = sqlmock.AnyArg()
	}
	args[15] = regexpJSONMatcher{expected: `{"deleted_rows":` + strconv.FormatInt(deleted, 10) + `}`}
	return args
}

type regexpJSONMatcher struct {
	expected string
}

func (m regexpJSONMatcher) Match(value driver.Value) bool {
	text, ok := value.(string)
	return ok && text == m.expected
}
