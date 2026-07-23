package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackgroundTaskServiceStopHonorsContextDeadline(t *testing.T) {
	service := &BackgroundTaskService{}
	service.jobWG.Add(1)
	t.Cleanup(service.jobWG.Done)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := service.Stop(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
