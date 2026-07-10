//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestVideoGatewayRepositoryListDramaTasks_FilteredPagination_Integration(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	user := mustCreateUser(t, tx.Client(), &service.User{
		Username: "drama-list-" + uuid.NewString(),
	})

	repo := NewVideoGatewayRepository(integrationDB).(*videoGatewayRepository)

	var providerAccountID int64
	err := integrationDB.QueryRowContext(ctx, `
		INSERT INTO video_provider_accounts (provider, display_name, enabled, default_model)
		VALUES ($1, $2, true, $3)
		RETURNING id`,
		service.VideoProviderMock,
		"mock-drama-list-"+uuid.NewString(),
		"mock-video-v1",
	).Scan(&providerAccountID)
	require.NoError(t, err)

	seededTaskIDs := make([]int64, 0, 9)
	t.Cleanup(func() {
		for _, id := range seededTaskIDs {
			_, _ = integrationDB.ExecContext(context.Background(),
				`DELETE FROM video_task_events WHERE video_task_id = $1`, id)
			_, _ = integrationDB.ExecContext(context.Background(),
				`DELETE FROM video_tasks WHERE id = $1`, id)
		}
		_, _ = integrationDB.ExecContext(context.Background(),
			`DELETE FROM video_provider_accounts WHERE id = $1`, providerAccountID)
	})

	baseTime := time.Now().UTC()
	for i := range 5 {
		id := insertIntegrationDramaTask(t, ctx, providerAccountID, user.ID, "E001", baseTime.Add(time.Duration(i)*time.Second))
		seededTaskIDs = append(seededTaskIDs, id)
	}
	for i := range 3 {
		id := insertIntegrationDramaTask(t, ctx, providerAccountID, user.ID, "E002", baseTime.Add(time.Duration(10+i)*time.Second))
		seededTaskIDs = append(seededTaskIDs, id)
	}
	plainTaskID := insertIntegrationPlainVideoTask(t, ctx, providerAccountID, user.ID, baseTime.Add(30*time.Second))
	seededTaskIDs = append(seededTaskIDs, plainTaskID)

	filters := map[string]string{"employee_alias": "E001"}
	baseParams := service.VideoTaskListParams{
		PageSize:  2,
		CreatedBy: user.ID,
		IsAdmin:   true,
	}

	page1, total, err := repo.ListDramaTasks(ctx, mergeVideoListPageParams(baseParams, 1), filters)
	require.NoError(t, err)
	require.Equal(t, int64(5), total)
	require.Len(t, page1, 2)

	page2, total2, err := repo.ListDramaTasks(ctx, mergeVideoListPageParams(baseParams, 2), filters)
	require.NoError(t, err)
	require.Equal(t, int64(5), total2)
	require.Len(t, page2, 2)

	page3, total3, err := repo.ListDramaTasks(ctx, mergeVideoListPageParams(baseParams, 3), filters)
	require.NoError(t, err)
	require.Equal(t, int64(5), total3)
	require.Len(t, page3, 1)

	allIDs, totalAll, err := repo.ListDramaTasks(ctx, service.VideoTaskListParams{
		Page: 1, PageSize: 50, CreatedBy: user.ID, IsAdmin: true,
	}, map[string]string{})
	require.NoError(t, err)
	require.Equal(t, int64(8), totalAll, "only tasks with drama_context events are listed")
	require.Len(t, allIDs, 8)
}

func mergeVideoListPageParams(p service.VideoTaskListParams, page int) service.VideoTaskListParams {
	p.Page = page
	return p
}

func insertIntegrationDramaTask(
	t *testing.T,
	ctx context.Context,
	providerAccountID, createdBy int64,
	employeeAlias string,
	createdAt time.Time,
) int64 {
	t.Helper()
	var taskID int64
	err := integrationDB.QueryRowContext(ctx, `
		INSERT INTO video_tasks (
			provider_account_id, provider, model, task_type, prompt, status,
			created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		RETURNING id`,
		providerAccountID,
		service.VideoProviderMock,
		"mock-video-v1",
		service.VideoTaskTypeTextToVideo,
		fmt.Sprintf("drama prompt %s", employeeAlias),
		service.VideoStatusQueued,
		createdBy,
		createdAt,
	).Scan(&taskID)
	require.NoError(t, err)

	payload, err := json.Marshal(map[string]any{
		"employee_alias":    employeeAlias,
		"selected_provider": "safe_demo_provider",
		"selected_model":    "mock-video-v1",
		"selected_mode":     "text-to-video",
	})
	require.NoError(t, err)

	_, err = integrationDB.ExecContext(ctx, `
		INSERT INTO video_task_events (video_task_id, event_type, message, payload_json, created_at)
		VALUES ($1, 'drama_context', 'integration seed', $2::jsonb, $3)`,
		taskID, string(payload), createdAt,
	)
	require.NoError(t, err)
	return taskID
}

func insertIntegrationPlainVideoTask(
	t *testing.T,
	ctx context.Context,
	providerAccountID, createdBy int64,
	createdAt time.Time,
) int64 {
	t.Helper()
	var taskID int64
	err := integrationDB.QueryRowContext(ctx, `
		INSERT INTO video_tasks (
			provider_account_id, provider, model, task_type, prompt, status,
			created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		RETURNING id`,
		providerAccountID,
		service.VideoProviderMock,
		"mock-video-v1",
		service.VideoTaskTypeTextToVideo,
		"plain video task without drama context",
		service.VideoStatusQueued,
		createdBy,
		createdAt,
	).Scan(&taskID)
	require.NoError(t, err)
	return taskID
}
