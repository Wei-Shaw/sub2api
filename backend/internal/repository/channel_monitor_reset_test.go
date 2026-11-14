package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorResetHistory(t *testing.T) {
	for _, scenario := range []string{"success", "missing monitor", "rollup failure"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			repo := &channelMonitorRepository{db: db}
			mock.ExpectBegin()
			count := int64(2)
			if scenario == "missing monitor" {
				count = 1
			}
			mock.ExpectExec(`UPDATE channel_monitors SET last_checked_at = NULL`).WithArgs("{42,43}").WillReturnResult(sqlmock.NewResult(0, count))
			if scenario == "missing monitor" {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec(`DELETE FROM channel_monitor_histories WHERE monitor_id`).WithArgs("{42,43}").WillReturnResult(sqlmock.NewResult(0, 100))
				expect := mock.ExpectExec(`DELETE FROM channel_monitor_daily_rollups WHERE monitor_id`).WithArgs("{42,43}")
				if scenario == "rollup failure" {
					expect.WillReturnError(errors.New("rollup deletion failed"))
					mock.ExpectRollback()
				} else {
					expect.WillReturnResult(sqlmock.NewResult(0, 4))
					mock.ExpectCommit()
				}
			}
			err = repo.ResetHistory(context.Background(), []int64{42, 43})
			switch scenario {
			case "success":
				require.NoError(t, err)
			case "missing monitor":
				require.ErrorIs(t, err, service.ErrChannelMonitorNotFound)
			default:
				require.ErrorContains(t, err, "rollup deletion failed")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
