package repository

import (
	"context"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// captureAllQueriesMatcher 记录 ent/原生 SQL 发出的每一条语句（captureEntQueryMatcher
// 只保留最后一条，这里需要看完整顺序）。
type captureAllQueriesMatcher struct {
	queries *[]string
}

func (m captureAllQueriesMatcher) Match(_, actual string) error {
	*m.queries = append(*m.queries, actual)
	return nil
}

func newAccrueQuotaMockRepo(t *testing.T, queries *[]string) (service.AffiliateRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureAllQueriesMatcher{queries: queries}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return NewAffiliateRepository(client, nil), mock
}

// TestAccrueQuotaLocksInviterRowBeforeReadingCap 钉住 H1 的修复机制：
// 上限校验必须发生在「锁定邀请人行」之后，而不是事务外的一次裸读。
func TestAccrueQuotaLocksInviterRowBeforeReadingCap(t *testing.T) {
	var queries []string
	repo, mock := newAccrueQuotaMockRepo(t, &queries)

	mock.ExpectBegin()
	mock.ExpectQuery("lock inviter row").
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectQuery("sum accrued").
		WithArgs(int64(11), int64(22)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(8.0))
	// 剩余额度只有 2，本次 5 必须被截断到 2 再入账。
	mock.ExpectExec("capped update").
		WithArgs(2.0, int64(11), 10.0, int64(22)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("ledger insert").
		WithArgs(int64(11), 2.0, int64(22), nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	applied, err := repo.AccrueQuota(context.Background(), 11, 22, 5, 0, nil, 10)
	require.NoError(t, err)
	require.InDelta(t, 2.0, applied, 1e-9)
	require.NoError(t, mock.ExpectationsWereMet())

	require.GreaterOrEqual(t, len(queries), 3)
	lockSQL := strings.ToUpper(normalizeSQLWhitespace(queries[0]))
	require.Contains(t, lockSQL, "USER_AFFILIATES")
	require.Contains(t, lockSQL, "FOR UPDATE")

	sumSQL := strings.ToUpper(normalizeSQLWhitespace(queries[1]))
	require.Contains(t, sumSQL, "USER_AFFILIATE_LEDGER")

	updateSQL := strings.ToUpper(normalizeSQLWhitespace(queries[2]))
	require.Contains(t, updateSQL, "UPDATE USER_AFFILIATES")
	// UPDATE 自带上限谓词，即使上面读到的值过期也不会超额入账。
	require.Contains(t, updateSQL, "SELECT SUM(L.AMOUNT)")
	require.Contains(t, updateSQL, "<= $3 +")
}

// TestAccrueQuotaSkipsWhenCapAlreadyReached 上限已用满时不应发出任何写语句。
func TestAccrueQuotaSkipsWhenCapAlreadyReached(t *testing.T) {
	var queries []string
	repo, mock := newAccrueQuotaMockRepo(t, &queries)

	mock.ExpectBegin()
	mock.ExpectQuery("lock inviter row").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectQuery("sum accrued").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(10.0))
	mock.ExpectCommit()

	applied, err := repo.AccrueQuota(context.Background(), 11, 22, 5, 0, nil, 10)
	require.NoError(t, err)
	require.Zero(t, applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAccrueQuotaCapPredicateRejectsStaleRead UPDATE 的上限谓词是最后一道闸门：
// 即便读到的累计值过期（并发事务刚提交了一笔），UPDATE 影响 0 行，本次就不入账，
// 也不会写流水。
func TestAccrueQuotaCapPredicateRejectsStaleRead(t *testing.T) {
	var queries []string
	repo, mock := newAccrueQuotaMockRepo(t, &queries)

	mock.ExpectBegin()
	mock.ExpectQuery("lock inviter row").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectQuery("sum accrued").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	mock.ExpectExec("capped update").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	applied, err := repo.AccrueQuota(context.Background(), 11, 22, 5, 0, nil, 10)
	require.NoError(t, err)
	require.Zero(t, applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAccrueQuotaWithoutInviterProfileDoesNotWrite 邀请人没有 profile 行时不入账。
func TestAccrueQuotaWithoutInviterProfileDoesNotWrite(t *testing.T) {
	var queries []string
	repo, mock := newAccrueQuotaMockRepo(t, &queries)

	mock.ExpectBegin()
	mock.ExpectQuery("lock inviter row").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}))
	mock.ExpectCommit()

	applied, err := repo.AccrueQuota(context.Background(), 11, 22, 5, 0, nil, 0)
	require.NoError(t, err)
	require.Zero(t, applied)
	require.NoError(t, mock.ExpectationsWereMet())
}
