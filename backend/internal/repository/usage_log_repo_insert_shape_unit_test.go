//go:build unit

package repository

import (
	"context"
	"database/sql"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

var (
	usageLogStaticInsertRe      = regexp.MustCompile(`(?s)INSERT INTO usage_logs \((.*?)\) VALUES \((.*?)\)`)
	usageLogInsertPlaceholderRe = regexp.MustCompile(`\$(\d+)`)
)

// newCapturingSQLMock 返回一个把实际下发 SQL 记录到 captured 的 sqlmock，
// 任何语句都视为匹配，参数校验仍由 WithArgs 承担。
func newCapturingSQLMock(t *testing.T, captured *[]string) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	matcher := sqlmock.QueryMatcherFunc(func(_, actualSQL string) error {
		*captured = append(*captured, actualSQL)
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

// requireUsageLogStaticInsertShape 断言静态拼写的 INSERT：列清单长度与
// VALUES 占位符数量都等于 usageLogInsertArgTypes，且占位符恰为 $1..$N 各一次。
func requireUsageLogStaticInsertShape(t *testing.T, query string) {
	t.Helper()
	m := usageLogStaticInsertRe.FindStringSubmatch(query)
	require.Len(t, m, 3, "unrecognised INSERT shape:\n%s", query)

	want := len(usageLogInsertArgTypes)

	columns := 0
	for _, col := range strings.Split(m[1], ",") {
		if strings.TrimSpace(col) != "" {
			columns++
		}
	}
	require.Equal(t, want, columns, "INSERT column list must match usageLogInsertArgTypes")

	seen := make(map[int]struct{}, want)
	for _, ph := range usageLogInsertPlaceholderRe.FindAllStringSubmatch(m[2], -1) {
		n, err := strconv.Atoi(ph[1])
		require.NoError(t, err)
		_, dup := seen[n]
		require.False(t, dup, "duplicate placeholder $%d", n)
		seen[n] = struct{}{}
	}
	require.Len(t, seen, want, "VALUES placeholder count must match usageLogInsertArgTypes")
	for i := 1; i <= want; i++ {
		_, ok := seen[i]
		require.True(t, ok, "missing placeholder $%d", i)
	}
}

// TestUsageLogStaticInsertQueries_PlaceholdersMatchArgTypes 覆盖两条不经
// 占位符生成器、直接手写 $1..$N 的 INSERT 路径，防止列清单加列后占位符漏补。
func TestUsageLogStaticInsertQueries_PlaceholdersMatchArgTypes(t *testing.T) {
	log := &service.UsageLog{
		UserID:      1,
		APIKeyID:    2,
		AccountID:   3,
		RequestID:   "client:req-static-insert-shape",
		Model:       "claude-3",
		InputTokens: 10,
		CreatedAt:   time.Date(2025, 1, 7, 12, 0, 0, 0, time.UTC),
	}
	prepared := prepareUsageLogInsert(log)
	args := anySliceToDriverValues(prepared.args)

	t.Run("createSingle", func(t *testing.T) {
		var captured []string
		db, mock := newCapturingSQLMock(t, &captured)
		repo := &usageLogRepository{sql: db}

		mock.ExpectQuery("INSERT INTO usage_logs").
			WithArgs(args...).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), log.CreatedAt))

		inserted, err := repo.Create(context.Background(), log)
		require.NoError(t, err)
		require.True(t, inserted)
		require.NoError(t, mock.ExpectationsWereMet())
		require.Len(t, captured, 1)
		requireUsageLogStaticInsertShape(t, captured[0])
	})

	t.Run("execUsageLogInsertNoResult", func(t *testing.T) {
		var captured []string
		db, mock := newCapturingSQLMock(t, &captured)

		mock.ExpectExec("INSERT INTO usage_logs").
			WithArgs(args...).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, execUsageLogInsertNoResult(context.Background(), db, prepared))
		require.NoError(t, mock.ExpectationsWereMet())
		require.Len(t, captured, 1)
		requireUsageLogStaticInsertShape(t, captured[0])
	})
}
