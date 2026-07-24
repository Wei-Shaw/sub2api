package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/backgroundruntime"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const defaultBackgroundStopTimeout = 10 * time.Second

var ErrDeploymentStandby = infraerrors.ServiceUnavailable(
	"DEPLOYMENT_STANDBY",
	"deployment candidate is not active",
)

func requireActiveDeploymentRuntime() error {
	if backgroundruntime.IsActive() {
		return nil
	}
	return ErrDeploymentStandby
}

func waitForBackgroundWorkers(ctx context.Context, wg *sync.WaitGroup) error {
	if wg == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
