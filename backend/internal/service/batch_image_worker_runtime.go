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
	done   chan struct{REDACTED
REDACTED

func NewBatchImageWorkerRuntime(worker *BatchImageWorker, cfg *config.Config) *BatchImageWorkerRuntime {
	return &BatchImageWorkerRuntime{worker: worker, cfg: cfgREDACTED
REDACTED

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
			AccountResolver:  &BatchImageAccountRepositoryResolver{Repo: accountRepoREDACTED,
			BillingRepo:      billingRepo,
			AuthCache:        authCache,
	REDACTED,
		SettlementService: &BatchImageSettlementService{
			Repo:         repo,
			BillingRepo:  billingRepo,
			UsageLogRepo: usageLogRepo,
			Pricing:      pricing,
			AuthCache:    authCache,
			Config:       cfg,
	REDACTED,
REDACTED
	runtime := NewBatchImageWorkerRuntime(NewBatchImageWorker(queue, processor, NewBatchImageWorkerOptionsFromConfig(cfg)), cfg)
	runtime.billingRecovery = &BatchImageBillingRecoveryService{
		Repo:       repo,
		Billing:    billingRepo,
		AuthCache:  authCache,
		StaleAfter: NewBatchImageWorkerOptionsFromConfig(cfg).StaleActiveAfter,
		Limit:      NewBatchImageWorkerOptionsFromConfig(cfg).RecoverLimit,
REDACTED
	runtime.Start()
	return runtime
REDACTED

func (r *BatchImageWorkerRuntime) Start() {
	if r == nil || r.worker == nil || r.cfg == nil || !r.cfg.BatchImage.QueueEnabled {
		return
REDACTED
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return
REDACTED

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{REDACTED)
	r.cancel = cancel
	r.done = done

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		r.worker.Run(ctx)
REDACTED()
	go func() {
		defer wg.Done()
		r.worker.RunDelayedMover(ctx)
REDACTED()
	go func() {
		defer wg.Done()
		r.worker.RunStaleActiveRecovery(ctx)
REDACTED()
	go func() {
		defer wg.Done()
		r.runBillingRecovery(ctx)
REDACTED()
	go func() {
		wg.Wait()
		close(done)
REDACTED()
REDACTED

func (r *BatchImageWorkerRuntime) runBillingRecovery(ctx context.Context) {
	if r == nil || r.worker == nil || r.billingRecovery == nil {
		return
REDACTED
	interval := r.worker.opts.RecoveryInterval
	for {
		if err := ctx.Err(); err != nil {
			return
	REDACTED
		_, _ = r.billingRecovery.ReleaseStaleUnsubmittedOnce(ctx)
		sleepOrDone(ctx, interval)
REDACTED
REDACTED

func (r *BatchImageWorkerRuntime) Stop() {
	if r == nil {
		return
REDACTED
	r.mu.Lock()
	cancel := r.cancel
	done := r.done
	r.cancel = nil
	r.done = nil
	r.mu.Unlock()

	if cancel != nil {
		cancel()
REDACTED
	if done != nil {
		<-done
REDACTED
REDACTED

func (r *BatchImageWorkerRuntime) Running() bool {
	if r == nil {
		return false
REDACTED
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancel != nil
REDACTED
