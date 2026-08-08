package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultScannerLeaseTTL             = 45 * time.Second
	DefaultScannerLeaseRenewInterval   = 15 * time.Second
	DefaultScannerLeaseAcquireInterval = 5 * time.Second
	defaultScannerLeaseReleaseTimeout  = 5 * time.Second
)

var (
	ErrScannerRuntimeOptionsInvalid = errors.New("web3 scanner runtime options are invalid")
	ErrScannerCreditMustBeDisabled  = errors.New("web3 scanner runtime requires crediting to remain disabled")
)

type ScannerRunner interface {
	ScanNext(ctx context.Context, leaseToken string, now time.Time) (ScannerResult, error)
}

type ScannerLeaseStore interface {
	Initialize(ctx context.Context, scannerKey string, chainID uint64, tokenContract string, scanStartBlock uint64) (ScannerCursor, error)
	AcquireLease(ctx context.Context, scannerKey string, owner string, token string, now time.Time, ttl time.Duration) (bool, error)
	RenewLease(ctx context.Context, scannerKey string, owner string, token string, now time.Time, ttl time.Duration) (bool, error)
	ReleaseLease(ctx context.Context, scannerKey string, owner string, token string) (bool, error)
	RecordError(ctx context.Context, scannerKey string, token string, now time.Time, message string) error
}

type ScannerRuntimeState string

const (
	ScannerRuntimeStateDisabled  ScannerRuntimeState = "disabled"
	ScannerRuntimeStateStandby   ScannerRuntimeState = "standby"
	ScannerRuntimeStateLeader    ScannerRuntimeState = "leader"
	ScannerRuntimeStateUnhealthy ScannerRuntimeState = "unhealthy"
	ScannerRuntimeStateStopped   ScannerRuntimeState = "stopped"
)

type ScannerRuntimeStatus struct {
	State      ScannerRuntimeState
	LeaseHeld  bool
	LastError  string
	LastResult ScannerResult
}

type ScannerRuntimeOptions struct {
	Enabled         bool
	ScannerKey      string
	Owner           string
	ChainID         uint64
	TokenContract   string
	ScanStartBlock  uint64
	PollInterval    time.Duration
	LeaseTTL        time.Duration
	RenewInterval   time.Duration
	AcquireInterval time.Duration
	now             func() time.Time
	newToken        func() string
	wait            func(context.Context, time.Duration) bool
}

type ScannerRuntime struct {
	scanner ScannerRunner
	leases  ScannerLeaseStore
	options ScannerRuntimeOptions

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	status ScannerRuntimeStatus
}

func NewScannerRuntime(scanner ScannerRunner, leases ScannerLeaseStore, options ScannerRuntimeOptions) (*ScannerRuntime, error) {
	applyScannerRuntimeDefaults(&options)
	runtime := &ScannerRuntime{
		scanner: scanner,
		leases:  leases,
		options: options,
		status:  ScannerRuntimeStatus{State: ScannerRuntimeStateStopped},
	}
	if !options.Enabled {
		runtime.status.State = ScannerRuntimeStateDisabled
		return runtime, nil
	}
	if scanner == nil || leases == nil ||
		options.ScannerKey == "" || options.Owner == "" ||
		options.ChainID == 0 || options.TokenContract == "" ||
		options.PollInterval <= 0 || options.LeaseTTL <= 0 ||
		options.RenewInterval <= 0 || options.RenewInterval >= options.LeaseTTL ||
		options.AcquireInterval <= 0 {
		return nil, ErrScannerRuntimeOptionsInvalid
	}
	return runtime, nil
}

func applyScannerRuntimeDefaults(options *ScannerRuntimeOptions) {
	if options.LeaseTTL <= 0 {
		options.LeaseTTL = DefaultScannerLeaseTTL
	}
	if options.RenewInterval <= 0 {
		options.RenewInterval = DefaultScannerLeaseRenewInterval
	}
	if options.AcquireInterval <= 0 {
		options.AcquireInterval = DefaultScannerLeaseAcquireInterval
	}
	if options.now == nil {
		options.now = time.Now
	}
	if options.newToken == nil {
		options.newToken = uuid.NewString
	}
	if options.wait == nil {
		options.wait = waitForScannerRuntime
	}
}

func (r *ScannerRuntime) Start(ctx context.Context) error {
	if r == nil || !r.options.Enabled {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return nil
	}
	if _, err := r.leases.Initialize(
		ctx,
		r.options.ScannerKey,
		r.options.ChainID,
		r.options.TokenContract,
		r.options.ScanStartBlock,
	); err != nil {
		r.status = ScannerRuntimeStatus{State: ScannerRuntimeStateUnhealthy, LastError: err.Error()}
		return fmt.Errorf("initialize web3 scanner runtime: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	r.cancel = cancel
	r.done = done
	r.status = ScannerRuntimeStatus{State: ScannerRuntimeStateStandby}
	go func() {
		defer close(done)
		r.run(runCtx)
		r.mu.Lock()
		r.cancel = nil
		r.done = nil
		if r.status.State != ScannerRuntimeStateDisabled {
			r.status.State = ScannerRuntimeStateStopped
			r.status.LeaseHeld = false
		}
		r.mu.Unlock()
	}()
	return nil
}

func (r *ScannerRuntime) run(ctx context.Context) {
	for ctx.Err() == nil {
		token := r.options.newToken()
		now := r.options.now()
		acquired, err := r.leases.AcquireLease(
			ctx,
			r.options.ScannerKey,
			r.options.Owner,
			token,
			now,
			r.options.LeaseTTL,
		)
		if err != nil {
			r.setRuntimeError(err, false)
			if !r.options.wait(ctx, r.options.AcquireInterval) {
				return
			}
			continue
		}
		if !acquired {
			r.setRuntimeState(ScannerRuntimeStateStandby, false)
			if !r.options.wait(ctx, r.options.AcquireInterval) {
				return
			}
			continue
		}

		r.setRuntimeState(ScannerRuntimeStateLeader, true)
		r.runLeaseSession(ctx, token)
		r.releaseLease(token)
		if ctx.Err() != nil {
			return
		}
		r.setRuntimeState(ScannerRuntimeStateStandby, false)
		if !r.options.wait(ctx, r.options.AcquireInterval) {
			return
		}
	}
}

func (r *ScannerRuntime) runLeaseSession(ctx context.Context, token string) {
	nextRenewal := r.options.now().Add(r.options.RenewInterval)
	for ctx.Err() == nil {
		now := r.options.now()
		if !now.Before(nextRenewal) {
			renewed, err := r.leases.RenewLease(
				ctx,
				r.options.ScannerKey,
				r.options.Owner,
				token,
				now,
				r.options.LeaseTTL,
			)
			if err != nil {
				r.setRuntimeError(err, false)
				return
			}
			if !renewed {
				r.setRuntimeError(ErrLeaseNotHeld, false)
				return
			}
			nextRenewal = now.Add(r.options.RenewInterval)
		}

		result, err := r.scanner.ScanNext(ctx, token, now)
		if err != nil {
			if errors.Is(err, ErrLeaseNotHeld) {
				r.setRuntimeError(err, false)
				return
			}
			r.setRuntimeError(err, true)
			if recordErr := r.leases.RecordError(ctx, r.options.ScannerKey, token, now, err.Error()); recordErr != nil && errors.Is(recordErr, ErrLeaseNotHeld) {
				r.setRuntimeError(recordErr, false)
				return
			}
		} else {
			r.setRuntimeSuccess(result)
		}

		waitDuration := r.options.PollInterval
		untilRenewal := nextRenewal.Sub(r.options.now())
		if untilRenewal < waitDuration {
			waitDuration = untilRenewal
		}
		if waitDuration < 0 {
			waitDuration = 0
		}
		if !r.options.wait(ctx, waitDuration) {
			return
		}
	}
}

func (r *ScannerRuntime) releaseLease(token string) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultScannerLeaseReleaseTimeout)
	defer cancel()
	_, _ = r.leases.ReleaseLease(ctx, r.options.ScannerKey, r.options.Owner, token)
}

func (r *ScannerRuntime) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel := r.cancel
	done := r.done
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (r *ScannerRuntime) Status() ScannerRuntimeStatus {
	if r == nil {
		return ScannerRuntimeStatus{State: ScannerRuntimeStateDisabled}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *ScannerRuntime) setRuntimeState(state ScannerRuntimeState, leaseHeld bool) {
	r.mu.Lock()
	r.status.State = state
	r.status.LeaseHeld = leaseHeld
	r.mu.Unlock()
}

func (r *ScannerRuntime) setRuntimeError(err error, leaseHeld bool) {
	web3RuntimeMetrics.scannerFailures.Add(1)
	r.mu.Lock()
	r.status.State = ScannerRuntimeStateUnhealthy
	r.status.LeaseHeld = leaseHeld
	r.status.LastError = err.Error()
	r.mu.Unlock()
}

func (r *ScannerRuntime) setRuntimeSuccess(result ScannerResult) {
	lag := uint64(0)
	if result.HeadBlock > result.ToBlock {
		lag = result.HeadBlock - result.ToBlock
	}
	web3RuntimeMetrics.scannerLag.Store(lag)
	r.mu.Lock()
	r.status.State = ScannerRuntimeStateLeader
	r.status.LeaseHeld = true
	r.status.LastError = ""
	r.status.LastResult = result
	r.mu.Unlock()
}

func waitForScannerRuntime(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
