package repository

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRenameTransaction(t *testing.T) {
	for _, scenario := range []string{"success", "retry", "stale", "missing", "collision", "outbox failure"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			repo := newAccountRepositoryWithSQL(client, db, nil)
			plan := []service.AccountRenameItem{{AccountID: 1, OldName: "old", NewName: "GPT-Free-01"}}
			mock.ExpectBegin()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			rows := sqlmock.NewRows([]string{"id", "name"})
			switch scenario {
			case "missing":
			case "retry":
				rows.AddRow(1, "GPT-Free-01")
			case "stale":
				rows.AddRow(1, "admin changed")
			default:
				rows.AddRow(1, "old")
			}
			mock.ExpectQuery("SELECT id, name FROM accounts.*ORDER BY id FOR UPDATE").WithArgs("{1}").WillReturnRows(rows)
			if scenario != "missing" && scenario != "stale" {
				collisions := sqlmock.NewRows([]string{"id"})
				if scenario == "collision" {
					collisions.AddRow(9)
				}
				mock.ExpectQuery("SELECT id FROM accounts.*name = ANY").WithArgs(`{"GPT-Free-01"}`, "{1}").WillReturnRows(collisions)
				if scenario == "success" || scenario == "outbox failure" {
					// No credentials, extra, status or schedulable columns may be written.
					mock.ExpectExec(`UPDATE accounts SET name = \$1, updated_at = NOW\(\) WHERE id = \$2 AND deleted_at IS NULL`).WithArgs("GPT-Free-01", int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
					outbox := mock.ExpectExec("INSERT INTO scheduler_outbox")
					if scenario == "outbox failure" {
						outbox.WillReturnError(errors.New("outbox unavailable"))
					} else {
						outbox.WillReturnResult(sqlmock.NewResult(1, 1))
					}
				}
			}
			if scenario == "success" || scenario == "retry" {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err = repo.ApplyAccountRenames(context.Background(), plan)
			switch scenario {
			case "success", "retry":
				require.NoError(t, err)
			case "missing":
				require.ErrorIs(t, err, service.ErrAccountNotFound)
			case "stale", "collision":
				require.ErrorIs(t, err, service.ErrAccountRenameConflict)
			default:
				require.Error(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
