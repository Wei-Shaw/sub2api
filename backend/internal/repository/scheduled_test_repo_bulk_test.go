package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestScheduledTestPlanRepositoryCreateMissingForAllAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	defaults := service.ScheduledTestBulkDefaults{
		ModelByPlatform: map[string]string{
			service.PlatformAnthropic: "claude-haiku",
			service.PlatformOpenAI:    "gpt-mini",
		},
		CronExpression: "*/30 * * * *",
		MaxResults:     50,
		AutoRecover:    true,
		SpreadMinutes:  30,
	}

	mock.ExpectExec(`(?s)WITH candidates AS .*NOT EXISTS.*INSERT INTO scheduled_test_plans`).
		WithArgs(
			`{"anthropic":"claude-haiku","openai":"gpt-mini"}`,
			"*/30 * * * *",
			50,
			true,
			30,
		).
		WillReturnResult(sqlmock.NewResult(0, 3))

	repo := NewScheduledTestPlanRepository(db)
	created, err := repo.CreateMissingForAllAccounts(context.Background(), defaults)

	require.NoError(t, err)
	require.Equal(t, int64(3), created)
	require.NoError(t, mock.ExpectationsWereMet())
}
