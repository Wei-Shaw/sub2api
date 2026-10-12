package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoTaskTrackingRecordsNeverEnterDurableSettlement(t *testing.T) {
	task := videoTaskTestFixture()
	task.Pending.TrackingOnly = true
	repo := &videoTaskTestRepo{task: task}
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	loaded, err := svc.LoadDurableVideoTask(context.Background(), task.TaskID, task.UserID, task.APIKeyID)
	require.NoError(t, err)
	require.Nil(t, loaded)
	require.NoError(t, svc.ReconcileOwnedVideoTask(context.Background(), task.TaskID, task.UserID, task.APIKeyID))
	require.Empty(t, repo.events)
	task.Pending.TrackingOnly = false
	loaded, err = svc.LoadDurableVideoTask(context.Background(), task.TaskID, task.UserID, task.APIKeyID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
}
