package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	dbusagecleanuptask "github.com/Wei-Shaw/sub2api/ent/usagecleanuptask"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newUsageCleanupEntRepo(t *testing.T) (*usageCleanupRepository, *dbent.Client) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:usage_cleanup?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	repo := &usageCleanupRepository{client: client, sql: db}
	return repo, client
}

func TestUsageCleanupRepositoryEntCreateAndList(t *testing.T) {
	repo, _ := newUsageCleanupEntRepo(t)

	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	task := &service.UsageCleanupTask{
		Status:    service.UsageCleanupStatusPending,
		Filters:   service.UsageCleanupFilters{StartTime: start, EndTime: end},
		CreatedBy: 9,
	}
	require.NoError(t, repo.CreateTask(context.Background(), task))
	require.NotZero(t, task.ID)

	task2 := &service.UsageCleanupTask{
		Status:    service.UsageCleanupStatusRunning,
		Filters:   service.UsageCleanupFilters{StartTime: start.Add(-24 * time.Hour), EndTime: end.Add(-24 * time.Hour)},
		CreatedBy: 10,
	}
	require.NoError(t, repo.CreateTask(context.Background(), task2))

	tasks, result, err := repo.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	require.Equal(t, int64(2), result.Total)
	require.Greater(t, tasks[0].ID, tasks[1].ID)
	require.Equal(t, start, tasks[1].Filters.StartTime)
	require.Equal(t, end, tasks[1].Filters.EndTime)
}

func TestUsageCleanupRepositoryEntListEmpty(t *testing.T) {
	repo, _ := newUsageCleanupEntRepo(t)

	tasks, result, err := repo.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Empty(t, tasks)
	require.Equal(t, int64(0), result.Total)
}

func TestUsageCleanupRepositoryEntGetStatusAndProgress(t *testing.T) {
	repo, client := newUsageCleanupEntRepo(t)

	task := &service.UsageCleanupTask{
		Status:    service.UsageCleanupStatusPending,
		Filters:   service.UsageCleanupFilters{StartTime: time.Now().UTC(), EndTime: time.Now().UTC().Add(time.Hour)},
		CreatedBy: 3,
	}
	require.NoError(t, repo.CreateTask(context.Background(), task))

	status, err := repo.GetTaskStatus(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, service.UsageCleanupStatusPending, status)

	_, err = repo.GetTaskStatus(context.Background(), task.ID+99)
	require.ErrorIs(t, err, sql.ErrNoRows)

	require.NoError(t, repo.UpdateTaskProgress(context.Background(), task.ID, 42))
	loaded, err := client.UsageCleanupTask.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(42), loaded.DeletedRows)
}

func TestUsageCleanupRepositoryEntCancelAndFinish(t *testing.T) {
	repo, client := newUsageCleanupEntRepo(t)

	task := &service.UsageCleanupTask{
		Status:    service.UsageCleanupStatusPending,
		Filters:   service.UsageCleanupFilters{StartTime: time.Now().UTC(), EndTime: time.Now().UTC().Add(time.Hour)},
		CreatedBy: 5,
	}
	require.NoError(t, repo.CreateTask(context.Background(), task))

	ok, err := repo.CancelTask(context.Background(), task.ID, 7)
	require.NoError(t, err)
	require.True(t, ok)

	loaded, err := client.UsageCleanupTask.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, service.UsageCleanupStatusCanceled, loaded.Status)
	require.NotNil(t, loaded.CanceledBy)
	require.NotNil(t, loaded.CanceledAt)
	require.NotNil(t, loaded.FinishedAt)

	loaded.Status = service.UsageCleanupStatusSucceeded
	_, err = client.UsageCleanupTask.Update().Where(dbusagecleanuptask.IDEQ(task.ID)).SetStatus(loaded.Status).Save(context.Background())
	require.NoError(t, err)

	ok, err = repo.CancelTask(context.Background(), task.ID, 7)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestUsageCleanupRepositoryEntCancelError(t *testing.T) {
	repo, client := newUsageCleanupEntRepo(t)

	task := &service.UsageCleanupTask{
		Status:    service.UsageCleanupStatusPending,
		Filters:   service.UsageCleanupFilters{StartTime: time.Now().UTC(), EndTime: time.Now().UTC().Add(time.Hour)},
		CreatedBy: 5,
	}
	require.NoError(t, repo.CreateTask(context.Background(), task))

	require.NoError(t, client.Close())
	_, err := repo.CancelTask(context.Background(), task.ID, 7)
	require.Error(t, err)
}

func TestUsageCleanupRepositoryEntMarkResults(t *testing.T) {
	repo, client := newUsageCleanupEntRepo(t)

	task := &service.UsageCleanupTask{
		Status:    service.UsageCleanupStatusRunning,
		Filters:   service.UsageCleanupFilters{StartTime: time.Now().UTC(), EndTime: time.Now().UTC().Add(time.Hour)},
		CreatedBy: 12,
	}
	require.NoError(t, repo.CreateTask(context.Background(), task))

	require.NoError(t, repo.MarkTaskSucceeded(context.Background(), task.ID, 6))
	loaded, err := client.UsageCleanupTask.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, service.UsageCleanupStatusSucceeded, loaded.Status)
	require.Equal(t, int64(6), loaded.DeletedRows)
	require.NotNil(t, loaded.FinishedAt)

	task2 := &service.UsageCleanupTask{
		Status:    service.UsageCleanupStatusRunning,
		Filters:   service.UsageCleanupFilters{StartTime: time.Now().UTC(), EndTime: time.Now().UTC().Add(time.Hour)},
		CreatedBy: 12,
	}
	require.NoError(t, repo.CreateTask(context.Background(), task2))

	require.NoError(t, repo.MarkTaskFailed(context.Background(), task2.ID, 4, "boom"))
	loaded2, err := client.UsageCleanupTask.Get(context.Background(), task2.ID)
	require.NoError(t, err)
	require.Equal(t, service.UsageCleanupStatusFailed, loaded2.Status)
	require.Equal(t, "boom", *loaded2.ErrorMessage)
}

func TestUsageCleanupRepositoryEntInvalidStatus(t *testing.T) {
	repo, _ := newUsageCleanupEntRepo(t)

	task := &service.UsageCleanupTask{
		Status:    "invalid",
		Filters:   service.UsageCleanupFilters{StartTime: time.Now().UTC(), EndTime: time.Now().UTC().Add(time.Hour)},
		CreatedBy: 1,
	}
	require.Error(t, repo.CreateTask(context.Background(), task))
}

func TestUsageCleanupRepositoryEntListInvalidFilters(t *testing.T) {
	repo, client := newUsageCleanupEntRepo(t)

	now := time.Now().UTC()
	driver, ok := client.Driver().(*entsql.Driver)
	require.True(t, ok)
	_, err := driver.DB().ExecContext(
		context.Background(),
		`INSERT INTO usage_cleanup_tasks (status, filters, created_by, deleted_rows, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		service.UsageCleanupStatusPending,
		[]byte("invalid-json"),
		int64(1),
		int64(0),
		now,
		now,
	)
	require.NoError(t, err)

	_, _, err = repo.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 10})
	require.Error(t, err)
}

func TestUsageCleanupTaskFromEntFull(t *testing.T) {
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	errMsg := "failed"
	canceledBy := int64(2)
	canceledAt := start.Add(time.Minute)
	startedAt := start.Add(2 * time.Minute)
	finishedAt := start.Add(3 * time.Minute)
	filters := service.UsageCleanupFilters{StartTime: start, EndTime: end}
	filtersJSON, err := json.Marshal(filters)
	require.NoError(t, err)

	task, err := usageCleanupTaskFromEnt(&dbent.UsageCleanupTask{
		ID:           10,
		Status:       service.UsageCleanupStatusFailed,
		Filters:      filtersJSON,
		CreatedBy:    11,
		DeletedRows:  7,
		ErrorMessage: &errMsg,
		CanceledBy:   &canceledBy,
		CanceledAt:   &canceledAt,
		StartedAt:    &startedAt,
		FinishedAt:   &finishedAt,
		CreatedAt:    start,
		UpdatedAt:    end,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), task.ID)
	require.Equal(t, service.UsageCleanupStatusFailed, task.Status)
	require.NotNil(t, task.ErrorMsg)
	require.NotNil(t, task.CanceledBy)
	require.NotNil(t, task.CanceledAt)
	require.NotNil(t, task.StartedAt)
	require.NotNil(t, task.FinishedAt)
}

func TestUsageCleanupTaskFromEntInvalidFilters(t *testing.T) {
	task, err := usageCleanupTaskFromEnt(&dbent.UsageCleanupTask{
		Filters: json.RawMessage("invalid-json"),
	})
	require.Error(t, err)
	require.Empty(t, task)
}
