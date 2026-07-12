package repository

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestSchedulerOutboxRepositoryDeleteConsumedUpToUsesBoundedCTE(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &schedulerOutboxRepository{db: db}
	const expectedSQL = `
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
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock(hashtext('scheduler_outbox_cleanup'))")).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock(hashtext('scheduler_outbox_cleanup'))")).
		WillReturnResult(sqlmock.NewResult(0, 1))

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
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock(hashtext('scheduler_outbox_cleanup'))")).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

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
