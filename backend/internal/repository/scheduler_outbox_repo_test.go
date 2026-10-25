package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestSchedulerOutboxRepositoryFirstCreatedAtAfter(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &schedulerOutboxRepository{db: db}
	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	expectedSQL := `
		SELECT created_at
		FROM scheduler_outbox
		WHERE id > $1
		ORDER BY id ASC
		LIMIT 1
	`
	if IsMySQL() {
		expectedSQL = `
		SELECT created_at
		FROM scheduler_outbox
		WHERE id > ?
		ORDER BY id ASC
		LIMIT 1
	`
	}
	mock.ExpectQuery(regexp.QuoteMeta(expectedSQL)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(createdAt))

	got, ok, err := repo.FirstCreatedAtAfter(context.Background(), 42)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, createdAt, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerOutboxRepositoryFirstCreatedAtAfterReturnsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &schedulerOutboxRepository{db: db}
	expectedSQL := `
		SELECT created_at
		FROM scheduler_outbox
		WHERE id > $1
		ORDER BY id ASC
		LIMIT 1
	`
	if IsMySQL() {
		expectedSQL = `
		SELECT created_at
		FROM scheduler_outbox
		WHERE id > ?
		ORDER BY id ASC
		LIMIT 1
	`
	}
	mock.ExpectQuery(regexp.QuoteMeta(expectedSQL)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}))

	got, ok, err := repo.FirstCreatedAtAfter(context.Background(), 42)

	require.NoError(t, err)
	require.False(t, ok)
	require.True(t, got.IsZero())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerOutboxRepositoryDeleteConsumedUpToUsesBoundedCTE(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &schedulerOutboxRepository{db: db}
	// PG 版为 doomed CTE + DELETE..USING；MySQL 版为单表 DELETE + ORDER BY/LIMIT
	//（deleteConsumedUpToMySQL），语义等价：同条件、按 id 升序限量删除。
	expectedSQL := `
		WITH doomed AS (
			SELECT id
			FROM scheduler_outbox
			WHERE id <= $1
				AND created_at < NOW() - INTERVAL '10 seconds'
			ORDER BY id ASC
			LIMIT $2
		)
		DELETE FROM scheduler_outbox o
		USING doomed d
		WHERE o.id = d.id
	`
	if IsMySQL() {
		expectedSQL = `
		DELETE FROM scheduler_outbox
		WHERE id <= ?
			AND created_at < NOW(3) - INTERVAL 10 SECOND
		ORDER BY id ASC
		LIMIT ?
	`
	}
	mock.ExpectExec(regexp.QuoteMeta(expectedSQL)).
		WithArgs(int64(42), 5000).
		WillReturnResult(sqlmock.NewResult(0, 17))

	deleted, err := repo.DeleteConsumedUpTo(context.Background(), 42, 5000)

	require.NoError(t, err)
	require.EqualValues(t, 17, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerOutboxRepositoryDeleteConsumedUpToSkipsNonPositiveWatermark(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &schedulerOutboxRepository{db: db}

	deleted, err := repo.DeleteConsumedUpTo(context.Background(), 0, 5000)

	require.NoError(t, err)
	require.EqualValues(t, 0, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerOutboxRepositoryTryAcquireCleanupLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &schedulerOutboxRepository{db: db}
	if IsMySQL() {
		// MySQL 版：GET_LOCK/RELEASE_LOCK 命名锁（1=成功，0=被占用）。
		mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?, 0)")).
			WithArgs(schedulerOutboxCleanupLockNameMySQL).
			WillReturnRows(sqlmock.NewRows([]string{"GET_LOCK(?, 0)"}).AddRow(int64(1)))
		mock.ExpectExec(regexp.QuoteMeta("SELECT RELEASE_LOCK(?)")).
			WithArgs(schedulerOutboxCleanupLockNameMySQL).
			WillReturnResult(sqlmock.NewResult(0, 1))
	} else {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock(hashtext('scheduler_outbox_cleanup'))")).
			WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
		mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock(hashtext('scheduler_outbox_cleanup'))")).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	lease, acquired, err := repo.TryAcquireCleanupLock(context.Background())
	require.NoError(t, err)
	require.True(t, acquired)
	require.NotNil(t, lease)

	lease.Release()

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerOutboxRepositoryTryAcquireCleanupLockUnavailable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &schedulerOutboxRepository{db: db}
	if IsMySQL() {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?, 0)")).
			WithArgs(schedulerOutboxCleanupLockNameMySQL).
			WillReturnRows(sqlmock.NewRows([]string{"GET_LOCK(?, 0)"}).AddRow(int64(0)))
	} else {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock(hashtext('scheduler_outbox_cleanup'))")).
			WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))
	}

	lease, acquired, err := repo.TryAcquireCleanupLock(context.Background())
	require.NoError(t, err)
	require.False(t, acquired)
	require.Nil(t, lease)

	require.NoError(t, mock.ExpectationsWereMet())
}

// buildSchedulerGroupPayload 在 groupIDs 为空时必须返回 untyped nil（any），
// 否则 enqueueSchedulerOutbox 的 "payload != nil" 接口判空会被 typed-nil 欺骗，
// 把 payload marshal 成 "null" 写入 dedup_key 哈希，破坏与其他 nil-payload
// 调用的去重一致性。本测试用 ungrouped 账号场景验证两条路径的 dedup_key 一致。
func TestEnqueueSchedulerOutbox_UngroupedAccountDedupesWithLiteralNilPayload(t *testing.T) {
	accountID := int64(42)

	// Path A: 显式 nil payload（如 SetError、SetStatus 等调用模式）
	keyLiteralNil := schedulerOutboxDedupKey("account_changed", &accountID, nil, nil)

	// Path B: buildSchedulerGroupPayload(account.GroupIDs) 当账号没有任何分组
	emptyGroupsPayload := buildSchedulerGroupPayload(nil)
	require.Nil(t, emptyGroupsPayload,
		"buildSchedulerGroupPayload(empty) must return untyped-nil any to avoid typed-nil marshal")

	// 模拟 enqueueSchedulerOutbox 内部的判空逻辑
	var payloadJSON []byte
	if emptyGroupsPayload != nil {
		t.Fatalf("typed-nil regression: buildSchedulerGroupPayload(empty) interface should be nil")
	}
	keyEmptyGroups := schedulerOutboxDedupKey("account_changed", &accountID, nil, payloadJSON)

	require.Equal(t, keyLiteralNil, keyEmptyGroups,
		"ungrouped-account account_changed must share dedup_key with other nil-payload variants")
}
