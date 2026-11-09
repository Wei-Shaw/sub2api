package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupQuotaPolicyRepositoryAbsentIsDisabled(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`SELECT p.group_id, p.revision, p.runtime_version, p.config, p.runtime`).WithArgs(int64(4)).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "revision", "runtime_version", "config", "runtime"}))
	record, err := NewGroupQuotaPolicyRepository(db).Get(context.Background(), 4)
	require.NoError(t, err)
	require.Equal(t, int64(0), record.Revision)
	require.NotNil(t, record.Config.Rules)
	require.Empty(t, record.Config.Rules)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupQuotaPolicyRepositoryConfigCAS(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	record := service.EmptyGroupQuotaPolicyRecord(4)
	mock.ExpectExec(`INSERT INTO group_quota_policies[\s\S]*WHERE group_quota_policies.revision = \$2 AND group_quota_policies.runtime_version = \$3`).
		WithArgs(int64(4), int64(2), int64(9), `{"rules":[]}`, `{"snapshots":[],"rules":[],"events":[]}`).WillReturnResult(sqlmock.NewResult(0, 0))
	ok, err := NewGroupQuotaPolicyRepository(db).SaveConfig(context.Background(), record, 2, 9)
	require.NoError(t, err)
	require.False(t, ok, "stale writes must be reported as conflicts")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupQuotaPolicyRepositoryRuntimeCASDoesNotWriteConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	record := service.EmptyGroupQuotaPolicyRecord(4)
	mock.ExpectExec(`UPDATE group_quota_policies SET runtime = \$4::jsonb, runtime_version = runtime_version \+ 1, updated_at = NOW\(\)[\s\S]*WHERE group_id = \$1 AND revision = \$2 AND runtime_version = \$3`).
		WithArgs(int64(4), int64(2), int64(9), `{"snapshots":[],"rules":[],"events":[]}`).WillReturnResult(sqlmock.NewResult(0, 1))
	ok, err := NewGroupQuotaPolicyRepository(db).SaveRuntime(context.Background(), record, 2, 9)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupQuotaPolicyRepositoryRejectsCorruptRuntime(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`SELECT p.group_id, p.revision, p.runtime_version, p.config, p.runtime`).WithArgs(int64(4)).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "revision", "runtime_version", "config", "runtime"}).AddRow(4, 2, 9, `{"rules":[]}`, `{broken`))
	record, err := NewGroupQuotaPolicyRepository(db).Get(context.Background(), 4)
	require.Error(t, err)
	require.Nil(t, record)
	require.NoError(t, mock.ExpectationsWereMet())
}
