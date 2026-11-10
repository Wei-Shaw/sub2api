package codexapp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrPoolExhausted = errors.New("codex app-server worker pool is exhausted")

// Worker is one long-lived, account-isolated app-server process.
type Worker interface {
	RunTurn(context.Context, TurnRequest, EventSink) (TurnResult, error)
	Close(context.Context) error
	Healthy() bool
}

type WorkerFactory func(ctx context.Context, workerKey string) (Worker, error)

type PoolConfig struct {
	MaxWorkers int
	IdleTTL    time.Duration
	Factory    WorkerFactory
}

// Pool keeps app-server processes warm by account key and keeps thread IDs
// sticky by session key. A worker handles one turn at a time because one
// app-server process owns the mutable live thread state for its account.
type Pool struct {
	mu         sync.Mutex
	workers    map[string]*poolWorker
	maxWorkers int
	idleTTL    time.Duration
	factory    WorkerFactory
	closed     bool
}

type poolWorker struct {
	key      string
	worker   Worker
	ready    chan struct{}
	startErr error
	active   int
	lastUsed time.Time
	turnMu   sync.Mutex
	sessions map[string]string
}

func NewPool(cfg PoolConfig) *Pool {
	maxWorkers := cfg.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 32
	}
	idleTTL := cfg.IdleTTL
	if idleTTL <= 0 {
		idleTTL = 10 * time.Minute
	}
	return &Pool{
		workers: make(map[string]*poolWorker), maxWorkers: maxWorkers,
		idleTTL: idleTTL, factory: cfg.Factory,
	}
}

func (p *Pool) RunTurn(
	ctx context.Context,
	workerKey string,
	sessionKey string,
	request TurnRequest,
	sink EventSink,
) (TurnResult, error) {
	if p == nil {
		return TurnResult{}, errors.New("codex app-server worker pool is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if workerKey == "" {
		return TurnResult{}, errors.New("codex app-server worker key is required")
	}

	entry, err := p.acquire(ctx, workerKey)
	if err != nil {
		return TurnResult{}, err
	}
	defer p.release(entry)

	entry.turnMu.Lock()
	defer entry.turnMu.Unlock()
	if sessionKey == "" {
		request.ThreadID = ""
		request.Ephemeral = true
	} else {
		request.ThreadID = entry.sessions[sessionKey]
		request.Ephemeral = false
	}
	result, err := entry.worker.RunTurn(ctx, request, sink)
	if err == nil && sessionKey != "" && result.ThreadID != "" {
		entry.sessions[sessionKey] = result.ThreadID
	}
	if !entry.worker.Healthy() {
		p.discard(entry)
	}
	return result, err
}

func (p *Pool) acquire(ctx context.Context, key string) (*poolWorker, error) {
	now := time.Now()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("codex app-server worker pool is closed")
	}
	p.evictIdleLocked(now)
	if existing := p.workers[key]; existing != nil {
		existing.active++
		ready := existing.ready
		p.mu.Unlock()
		select {
		case <-ready:
			if existing.startErr != nil {
				p.release(existing)
				return nil, existing.startErr
			}
			return existing, nil
		case <-ctx.Done():
			p.release(existing)
			return nil, ctx.Err()
		}
	}
	if len(p.workers) >= p.maxWorkers {
		p.evictLeastRecentlyUsedIdleLocked()
	}
	if len(p.workers) >= p.maxWorkers {
		p.mu.Unlock()
		return nil, ErrPoolExhausted
	}
	if p.factory == nil {
		p.mu.Unlock()
		return nil, errors.New("codex app-server worker factory is required")
	}
	entry := &poolWorker{
		key: key, ready: make(chan struct{}), active: 1, lastUsed: now,
		sessions: make(map[string]string),
	}
	p.workers[key] = entry
	p.mu.Unlock()

	worker, err := p.factory(ctx, key)
	p.mu.Lock()
	entry.worker = worker
	entry.startErr = err
	close(entry.ready)
	if err != nil {
		delete(p.workers, key)
	}
	p.mu.Unlock()
	if err != nil {
		p.release(entry)
		return nil, fmt.Errorf("start codex app-server worker %q: %w", key, err)
	}
	return entry, nil
}

func (p *Pool) release(entry *poolWorker) {
	if entry == nil {
		return
	}
	p.mu.Lock()
	if entry.active > 0 {
		entry.active--
	}
	entry.lastUsed = time.Now()
	p.mu.Unlock()
}

func (p *Pool) discard(entry *poolWorker) {
	if entry == nil {
		return
	}
	p.mu.Lock()
	if p.workers[entry.key] == entry {
		delete(p.workers, entry.key)
	}
	p.mu.Unlock()
	if entry.worker != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = entry.worker.Close(ctx)
	}
}

func (p *Pool) evictIdleLocked(now time.Time) {
	for key, entry := range p.workers {
		if entry.active != 0 || entry.worker == nil || now.Sub(entry.lastUsed) < p.idleTTL {
			continue
		}
		delete(p.workers, key)
		closeWorkerAsync(entry.worker)
	}
}

func (p *Pool) evictLeastRecentlyUsedIdleLocked() bool {
	var oldest *poolWorker
	for _, entry := range p.workers {
		if entry.active != 0 || entry.worker == nil {
			continue
		}
		if oldest == nil || entry.lastUsed.Before(oldest.lastUsed) {
			oldest = entry
		}
	}
	if oldest == nil {
		return false
	}
	delete(p.workers, oldest.key)
	closeWorkerAsync(oldest.worker)
	return true
}

func closeWorkerAsync(worker Worker) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = worker.Close(ctx)
	}()
}

func (p *Pool) Close(ctx context.Context) error {
	if p == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	workers := make([]*poolWorker, 0, len(p.workers))
	for _, entry := range p.workers {
		workers = append(workers, entry)
	}
	p.workers = make(map[string]*poolWorker)
	p.mu.Unlock()

	var joined error
	for _, entry := range workers {
		select {
		case <-entry.ready:
		case <-ctx.Done():
			return errors.Join(joined, ctx.Err())
		}
		if entry.worker != nil {
			joined = errors.Join(joined, entry.worker.Close(ctx))
		}
	}
	return joined
}
