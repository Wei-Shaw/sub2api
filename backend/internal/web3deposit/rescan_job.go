package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	RescanJobStatusPending   RescanJobStatus = "pending"
	RescanJobStatusRunning   RescanJobStatus = "running"
	RescanJobStatusSucceeded RescanJobStatus = "succeeded"
	RescanJobStatusFailed    RescanJobStatus = "failed"

	DefaultRescanJobLease = 10 * time.Minute
	DefaultRescanJobPoll  = 2 * time.Second
)

var (
	ErrRescanJobNotFound     = errors.New("web3 deposit rescan job not found")
	ErrRescanJobUnavailable  = errors.New("web3 deposit rescan jobs are unavailable")
	ErrRescanJobClaimLost    = errors.New("web3 deposit rescan job claim lost")
	ErrRescanJobCreateFailed = errors.New("web3 deposit rescan job creation failed")
)

type RescanJobStatus string

type RescanJob struct {
	ID             int64           `json:"id"`
	NetworkKey     string          `json:"network_key"`
	AssetKey       string          `json:"asset_key"`
	FromBlock      uint64          `json:"from_block"`
	ToBlock        uint64          `json:"to_block"`
	Status         RescanJobStatus `json:"status"`
	RequestedBy    int64           `json:"requested_by"`
	AttemptCount   int             `json:"attempt_count"`
	EventCount     int             `json:"event_count"`
	MatchedCount   int             `json:"matched_count"`
	DepositCount   int             `json:"deposit_count"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	LeaseExpiresAt *time.Time      `json:"-"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type RescanJobStore interface {
	CreateRescanJob(context.Context, RescanJob) (RescanJob, error)
	ClaimRescanJobs(context.Context, time.Time, time.Duration, int) ([]RescanJob, error)
	RenewRescanJob(context.Context, RescanJob, time.Time, time.Duration) error
	CompleteRescanJob(context.Context, RescanJob, BoundedRescanResult, time.Time) error
	FailRescanJob(context.Context, RescanJob, error, time.Time) error
	ListRescanJobs(context.Context, string, string, int) ([]RescanJob, error)
	GetRescanJob(context.Context, int64) (RescanJob, error)
}

type RescanJobRuntime struct {
	store     RescanJobStore
	rescanner *BoundedRescannerRegistry
	enabled   bool
	poll      time.Duration
	lease     time.Duration
	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewRescanJobRuntime(store RescanJobStore, rescanner *BoundedRescannerRegistry, enabled bool) *RescanJobRuntime {
	return &RescanJobRuntime{store: store, rescanner: rescanner, enabled: enabled, poll: DefaultRescanJobPoll, lease: DefaultRescanJobLease}
}

func (r *RescanJobRuntime) Enqueue(ctx context.Context, networkKey, assetKey string, fromBlock, toBlock uint64, requestedBy int64) (RescanJob, error) {
	if r == nil || !r.enabled || r.store == nil || r.rescanner == nil {
		return RescanJob{}, ErrRescanJobUnavailable
	}
	if err := r.rescanner.Validate(ctx, networkKey, assetKey, fromBlock, toBlock); err != nil {
		return RescanJob{}, err
	}
	job, err := r.store.CreateRescanJob(ctx, RescanJob{NetworkKey: networkKey, AssetKey: assetKey, FromBlock: fromBlock, ToBlock: toBlock, RequestedBy: requestedBy})
	if err != nil {
		return RescanJob{}, fmt.Errorf("%w: %v", ErrRescanJobCreateFailed, err)
	}
	return job, nil
}

func (r *RescanJobRuntime) List(ctx context.Context, networkKey, assetKey string, limit int) ([]RescanJob, error) {
	return r.store.ListRescanJobs(ctx, networkKey, assetKey, limit)
}

func (r *RescanJobRuntime) Get(ctx context.Context, id int64) (RescanJob, error) {
	return r.store.GetRescanJob(ctx, id)
}

func (r *RescanJobRuntime) Start(ctx context.Context) {
	if r == nil || !r.enabled || r.store == nil || r.rescanner == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	go func() {
		defer close(r.done)
		r.run(runCtx)
	}()
}

func (r *RescanJobRuntime) run(ctx context.Context) {
	for ctx.Err() == nil {
		r.process(ctx)
		timer := time.NewTimer(r.poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (r *RescanJobRuntime) process(ctx context.Context) {
	now := time.Now()
	jobs, err := r.store.ClaimRescanJobs(ctx, now, r.lease, 1)
	if err != nil {
		slog.Error("web3_deposit_rescan_job_claim_failed", "error", err)
		return
	}
	for _, job := range jobs {
		r.execute(ctx, job)
	}
}

func (r *RescanJobRuntime) execute(ctx context.Context, job RescanJob) {
	jobCtx, cancel := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	go func() {
		err := r.heartbeat(jobCtx, job)
		if err != nil {
			cancel()
		}
		heartbeatDone <- err
	}()

	result, runErr := r.rescanner.Rescan(jobCtx, job.NetworkKey, job.AssetKey, job.FromBlock, job.ToBlock)
	cancel()
	if heartbeatErr := <-heartbeatDone; heartbeatErr != nil {
		slog.Error("web3_deposit_rescan_job_lease_lost", "job_id", job.ID, "attempt", job.AttemptCount, "error", heartbeatErr)
		return
	}
	if runErr != nil {
		if ctx.Err() != nil {
			return
		}
		if err := r.store.FailRescanJob(ctx, job, runErr, time.Now()); err != nil {
			slog.Error("web3_deposit_rescan_job_fail_update_failed", "job_id", job.ID, "attempt", job.AttemptCount, "error", err)
		}
		return
	}
	if err := r.store.CompleteRescanJob(ctx, job, result, time.Now()); err != nil {
		slog.Error("web3_deposit_rescan_job_complete_failed", "job_id", job.ID, "attempt", job.AttemptCount, "error", err)
	}
}

func (r *RescanJobRuntime) heartbeat(ctx context.Context, job RescanJob) error {
	interval := r.lease / 3
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if ctx.Err() != nil {
				return nil
			}
			if err := r.store.RenewRescanJob(ctx, job, now, r.lease); err != nil {
				return err
			}
		}
	}
}

func (r *RescanJobRuntime) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}
