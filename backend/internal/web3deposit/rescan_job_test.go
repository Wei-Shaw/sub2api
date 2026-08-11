package web3deposit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRescanJobRuntimeCompletesClaimedJob(t *testing.T) {
	store := &rescanJobStoreStub{claimed: []RescanJob{{ID: 7, NetworkKey: "network", AssetKey: "asset", FromBlock: 10, ToBlock: 11}}}
	registry := &BoundedRescannerRegistry{rescanners: map[RuntimeKey]*BoundedRescanner{
		{NetworkKey: "network", AssetKey: "asset"}: {
			transfer:       &boundedRescanTransferStub{},
			finality:       &boundedRescanFinalityStub{finalized: 20},
			matcher:        &boundedRescanMatcherStub{},
			persister:      NewDepositEventPersister(&boundedRescanStoreStub{}, ChainConfig{ChainID: 1, TokenDecimals: 6}),
			maxRange:       10,
			scanStartBlock: 1,
		},
	}}
	runtime := NewRescanJobRuntime(store, registry, true)

	runtime.process(context.Background())

	require.Equal(t, int64(7), store.completedID)
	require.Zero(t, store.failedID)
	require.Equal(t, 1, store.claimCalls)
}

func TestRescanJobRuntimePersistsExecutionFailure(t *testing.T) {
	wantErr := errors.New("rpc unavailable")
	store := &rescanJobStoreStub{claimed: []RescanJob{{ID: 8, NetworkKey: "network", AssetKey: "asset", FromBlock: 10, ToBlock: 11}}}
	registry := &BoundedRescannerRegistry{rescanners: map[RuntimeKey]*BoundedRescanner{
		{NetworkKey: "network", AssetKey: "asset"}: {initErr: wantErr},
	}}
	runtime := NewRescanJobRuntime(store, registry, true)

	runtime.process(context.Background())

	require.Equal(t, int64(8), store.failedID)
	require.ErrorIs(t, store.failure, wantErr)
	require.Zero(t, store.completedID)
}

func TestRescanJobRuntimeRejectsInvalidRangeBeforeCreatingJob(t *testing.T) {
	store := &rescanJobStoreStub{}
	registry := &BoundedRescannerRegistry{rescanners: map[RuntimeKey]*BoundedRescanner{
		{NetworkKey: "network", AssetKey: "asset"}: {
			finality:       &boundedRescanFinalityStub{finalized: 200},
			maxRange:       10,
			scanStartBlock: 100,
		},
	}}
	runtime := NewRescanJobRuntime(store, registry, true)

	_, err := runtime.Enqueue(context.Background(), "network", "asset", 100, 110, 77)

	require.ErrorIs(t, err, ErrRescanRangeTooLarge)
	require.Zero(t, store.createCalls)
}

func TestRescanJobRuntimeRenewsLeaseWhileScanIsRunning(t *testing.T) {
	release := make(chan struct{})
	store := &rescanJobStoreStub{renewed: make(chan struct{}, 1)}
	registry := &BoundedRescannerRegistry{rescanners: map[RuntimeKey]*BoundedRescanner{
		{NetworkKey: "network", AssetKey: "asset"}: {
			transfer:       blockingRescanTransferStub{release: release},
			finality:       &boundedRescanFinalityStub{finalized: 20},
			matcher:        &boundedRescanMatcherStub{},
			persister:      NewDepositEventPersister(&boundedRescanStoreStub{}, ChainConfig{ChainID: 1, TokenDecimals: 6}),
			maxRange:       10,
			scanStartBlock: 1,
		},
	}}
	runtime := NewRescanJobRuntime(store, registry, true)
	runtime.lease = 30 * time.Millisecond
	done := make(chan struct{})
	go func() {
		runtime.execute(context.Background(), RescanJob{ID: 9, NetworkKey: "network", AssetKey: "asset", FromBlock: 10, ToBlock: 11, AttemptCount: 1})
		close(done)
	}()

	select {
	case <-store.renewed:
	case <-time.After(time.Second):
		t.Fatal("rescan job lease was not renewed")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("rescan job did not finish")
	}
	require.Equal(t, int64(9), store.completedID)
	require.GreaterOrEqual(t, store.renewCalls, 1)
}

type blockingRescanTransferStub struct{ release <-chan struct{} }

func (s blockingRescanTransferStub) LatestBlock(context.Context) (uint64, error) { return 0, nil }
func (s blockingRescanTransferStub) Fetch(ctx context.Context, _, _ uint64) ([]TransferEvent, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return nil, nil
	}
}

type rescanJobStoreStub struct {
	claimed     []RescanJob
	claimCalls  int
	completedID int64
	failedID    int64
	failure     error
	createCalls int
	renewCalls  int
	renewed     chan struct{}
}

func (s *rescanJobStoreStub) CreateRescanJob(context.Context, RescanJob) (RescanJob, error) {
	s.createCalls++
	return RescanJob{ID: 1, Status: RescanJobStatusPending}, nil
}

func (s *rescanJobStoreStub) ClaimRescanJobs(context.Context, time.Time, time.Duration, int) ([]RescanJob, error) {
	s.claimCalls++
	jobs := s.claimed
	s.claimed = nil
	return jobs, nil
}

func (s *rescanJobStoreStub) RenewRescanJob(context.Context, RescanJob, time.Time, time.Duration) error {
	s.renewCalls++
	if s.renewed != nil {
		select {
		case s.renewed <- struct{}{}:
		default:
		}
	}
	return nil
}

func (s *rescanJobStoreStub) CompleteRescanJob(_ context.Context, job RescanJob, _ BoundedRescanResult, _ time.Time) error {
	s.completedID = job.ID
	return nil
}

func (s *rescanJobStoreStub) FailRescanJob(_ context.Context, job RescanJob, failure error, _ time.Time) error {
	s.failedID = job.ID
	s.failure = failure
	return nil
}

func (s *rescanJobStoreStub) ListRescanJobs(context.Context, string, string, int) ([]RescanJob, error) {
	return nil, nil
}

func (s *rescanJobStoreStub) GetRescanJob(context.Context, int64) (RescanJob, error) {
	return RescanJob{}, ErrRescanJobNotFound
}
