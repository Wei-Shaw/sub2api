package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func seedDramaTaskWithContext(t *testing.T, repo *memoryVideoGatewayRepo, createdBy int64, employeeAlias string) int64 {
	t.Helper()
	providerID := repo.seedMockProvider()
	taskID := repo.nextTaskID
	repo.nextTaskID++
	now := time.Now().UTC().Add(time.Duration(taskID) * time.Second)
	task := &VideoTask{
		ID:                taskID,
		ProviderAccountID: providerID,
		Provider:          VideoProviderMock,
		Model:             "mock-video-v1",
		TaskType:          VideoTaskTypeTextToVideo,
		Status:            VideoStatusQueued,
		CreatedBy:         createdBy,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	repo.tasks[taskID] = cloneVideoTask(task)
	require.NoError(t, repo.AddTaskEvent(context.Background(), &VideoTaskEvent{
		VideoTaskID: taskID,
		EventType:   "drama_context",
		Message:     "test drama context",
		Payload: map[string]any{
			"employee_alias":    employeeAlias,
			"selected_provider": "safe_demo_provider",
			"selected_model":    "mock-video-v1",
			"selected_mode":     "text-to-video",
		},
	}))
	return taskID
}

func TestListDramaTasks_FilteredPagination(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	ctx := context.Background()

	for range 5 {
		seedDramaTaskWithContext(t, repo, 101, "E001")
	}
	for range 3 {
		seedDramaTaskWithContext(t, repo, 101, "E002")
	}

	filters := map[string]string{"employee_alias": "E001"}
	baseParams := VideoTaskListParams{PageSize: 2, CreatedBy: 101, IsAdmin: true}

	page1, total, err := svc.ListDramaTasks(ctx, mergeVideoListPage(baseParams, 1), filters)
	require.NoError(t, err)
	require.Equal(t, int64(5), total, "total must count all filter matches, not just current page")
	require.Len(t, page1, 2)

	page2, total2, err := svc.ListDramaTasks(ctx, mergeVideoListPage(baseParams, 2), filters)
	require.NoError(t, err)
	require.Equal(t, int64(5), total2)
	require.Len(t, page2, 2)

	page3, total3, err := svc.ListDramaTasks(ctx, mergeVideoListPage(baseParams, 3), filters)
	require.NoError(t, err)
	require.Equal(t, int64(5), total3)
	require.Len(t, page3, 1)
}

func mergeVideoListPage(p VideoTaskListParams, page int) VideoTaskListParams {
	p.Page = page
	return p
}
