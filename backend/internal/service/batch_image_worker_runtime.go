package service

import (
	"context"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type BatchImageWorkerRuntime struct {
	worker          *BatchImageWorker
	billingRecovery *BatchImageBillingRecoveryService
	cfg             *config.Config

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func NewBatchImageWorkerRuntime(worker *BatchImageWorker, cfg *config.Config) *BatchImageWorkerRuntime {
	return &BatchImageWorkerRuntime{worker: worker, cfg: cfg}
}

func ProvideBatchImageWorkerRuntime(
	repo BatchImageRepository,
	accountRepo AccountRepository,
	queue BatchImageQueue,
	billingRepo UsageBillingRepository,
	usageLogRepo UsageLogRepository,
	pricing *BatchImageModelPricingResolver,
	authCache APIKeyAuthCacheInvalidator,
	cfg *config.Config,
) *BatchImageWorkerRuntime {
	processor := &BatchImagePipelineProcessor{
		ProviderProcessor: &BatchImageProviderProcessor{
			Repo:             repo,
			ProviderRegistry: NewBatchImageProviderRegistryFromConfig(cfg),
			AccountResolver:  &BatchImageAccountRepositoryResolver{Repo: accountRepo},
			BillingRepo:      billingRepo,
			AuthCache:        authCache,
		},
		SettlementService: &BatchImageSettlementService{
			Repo:         repo,
			BillingRepo:  billingRepo,
			UsageLogRepo: usageLogRepo,
			Pricing:      pricing,
			AuthCache:    authCache,
			Config:       cfg,
		},
	}
	runtime := NewBatchImageWorkerRuntime(NewBatchImageWorker(queue, processor, NewBatchImageWorkerOptionsFromConfig(cfg)), cfg)
	runtime.billingRecovery = &BatchImageBillingRecoveryService{
		Repo:       repo,
		Billing:    billingRepo,
		AuthCache:  authCache,
		Queue:      queue,
		StaleAfter: NewBatchImageWorkerOptionsFromConfig(cfg).StaleActiveAfter,
		Limit:      NewBatchImageWorkerOptionsFromConfig(cfg).RecoverLimit,
	}
	runtime.Start()
	return runtime
}

func (r *BatchImageWorkerRuntime) Start() {
	if r == nil || r.worker == nil || r.cfg == nil || !r.cfg.BatchImage.QueueEnabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.cancel = cancel
	r.done = done

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		r.worker.Run(ctx)
	}()
	go func() {
		defer wg.Done()
		r.worker.RunDelayedMover(ctx)
	}()
	go func() {
		defer wg.Done()
		r.worker.RunStaleActiveRecovery(ctx)
	}()
	go func() {
		defer wg.Done()
		r.runBillingRecovery(ctx)
	}()
	go func() {
		wg.Wait()
		close(done)
	}()
}

func (r *BatchImageWorkerRuntime) runBillingRecovery(ctx context.Context) {
	if r == nil || r.worker == nil || r.billingRecovery == nil {
		return
	}
	interval := r.worker.opts.RecoveryInterval
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		_, _ = r.billingRecovery.ReleaseStaleUnsubmittedOnce(ctx)
		sleepOrDone(ctx, interval)
	}
}

func (r *BatchImageWorkerRuntime) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel := r.cancel
	done := r.done
	r.cancel = nil
	r.done = nil
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (r *BatchImageWorkerRuntime) Running() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancel != nil
}
