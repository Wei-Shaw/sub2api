package service

import (
	"context"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type BatchImageWorkerRuntime struct {
	worker *BatchImageWorker
	cfg    *config.Config

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
	pricing *BatchImageModelPricingResolver,
	cfg *config.Config,
) *BatchImageWorkerRuntime {
	processor := &BatchImagePipelineProcessor{
		ProviderProcessor: &BatchImageProviderProcessor{
			Repo:             repo,
			ProviderRegistry: NewDefaultBatchImageProviderRegistry(),
			AccountResolver:  &BatchImageAccountRepositoryResolver{Repo: accountRepoREDACTED,
	REDACTED,
		SettlementService: &BatchImageSettlementService{
			Repo:        repo,
			BillingRepo: billingRepo,
			Pricing:     pricing,
			Config:      cfg,
	REDACTED,
REDACTED
	runtime := NewBatchImageWorkerRuntime(NewBatchImageWorker(queue, processor, NewBatchImageWorkerOptionsFromConfig(cfg)), cfg)
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
	wg.Add(3)
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
		wg.Wait()
		close(done)
REDACTED()
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
