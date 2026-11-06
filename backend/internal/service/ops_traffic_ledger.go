package service

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/google/uuid"
)

const (
	opsTrafficQueueCapacity = 1024
	opsTrafficBatchSize     = 64
	opsTrafficBatchWindow   = 25 * time.Millisecond
	opsTrafficWriteTimeout  = 3 * time.Second
	opsTrafficDrainTimeout  = 5 * time.Second
)

// OpsTrafficEvent is an append-only request-level snapshot, not a usage record.
// Account/platform reflect the final route; traffic includes every retry.
// No payload, headers, API key, user identity or client IP belongs here.
type OpsTrafficEvent struct {
	EventID   string
	RequestID string
	GroupID   *int64
	AccountID *int64
	Platform  string
	CreatedAt time.Time
	Traffic   *traffic.Usage
}

// Kept separate from OpsRepository so existing consumers/mocks need not grow.
type OpsTrafficLedgerRepository interface {
	BatchInsertTrafficEvents(context.Context, []*OpsTrafficEvent) error
}

// Health is local to this process lifetime, NOT global or time-filtered. Failed
// is sticky: persisted rows alone cannot claim complete request coverage after
// a database outage, saturated queue, or forced shutdown.
type OpsTrafficLedgerHealth struct {
	Scope   string `json:"scope"`
	Pending int64  `json:"pending"`
	Failed  int64  `json:"failed"`
	Flushed int64  `json:"flushed"`
}

type opsTrafficLedger struct {
	repo     OpsTrafficLedgerRepository
	queue    chan *OpsTrafficEvent
	mu       sync.RWMutex
	stopping bool
	stopOnce sync.Once
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	pending  atomic.Int64
	failed   atomic.Int64
	flushed  atomic.Int64
}

func newOpsTrafficLedger(repo OpsTrafficLedgerRepository, capacity int) *opsTrafficLedger {
	ctx, cancel := context.WithCancel(context.Background())
	l := &opsTrafficLedger{repo: repo, queue: make(chan *OpsTrafficEvent, capacity), done: make(chan struct{}), ctx: ctx, cancel: cancel}
	go l.run()
	return l
}

// RecordTrafficEvent tries to queue bounded telemetry work without waiting for
// capacity or database I/O on the HTTP finalization path. Rejections are counted
// and returned to the caller; the sole worker retries writes using the SAME IDs.
func (s *OpsService) RecordTrafficEvent(ctx context.Context, input *OpsTrafficEvent) error {
	if s == nil || input == nil || input.Traffic == nil {
		return nil
	}
	if !s.IsMonitoringEnabled(ctx) {
		return nil
	}
	s.trafficLedgerMu.Lock()
	if s.trafficLedger == nil {
		repo, ok := s.opsRepo.(OpsTrafficLedgerRepository)
		if !ok {
			s.trafficLedgerMu.Unlock()
			return errors.New("ops traffic repository unavailable")
		}
		if s.trafficLedgerStopped {
			s.trafficLedgerMu.Unlock()
			return errors.New("ops traffic ledger stopped")
		}
		s.trafficLedger = newOpsTrafficLedger(repo, opsTrafficQueueCapacity)
	}
	ledger := s.trafficLedger
	s.trafficLedgerMu.Unlock()
	event := cloneOpsTrafficEvent(input)
	if event.EventID == "" {
		event.EventID = uuid.NewString()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return ledger.enqueue(ctx, event)
}

func cloneOpsTrafficEvent(input *OpsTrafficEvent) *OpsTrafficEvent {
	event := *input
	event.Traffic = traffic.Clone(input.Traffic)
	if input.GroupID != nil {
		n := *input.GroupID
		event.GroupID = &n
	}
	if input.AccountID != nil {
		n := *input.AccountID
		event.AccountID = &n
	}
	event.RequestID = boundedTrafficDimension(event.RequestID, 128)
	event.Platform = strings.ToLower(boundedTrafficDimension(event.Platform, 64))
	return &event
}

func boundedTrafficDimension(value string, limit int) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, ""))
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func (l *opsTrafficLedger) enqueue(ctx context.Context, event *OpsTrafficEvent) error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.stopping {
		l.failed.Add(1)
		return errors.New("ops traffic ledger stopped")
	}
	// Check before select: a cancelled caller must not randomly win an
	// otherwise writable channel case and get reported as accepted.
	if err := ctx.Err(); err != nil {
		l.failed.Add(1)
		return err
	}
	l.pending.Add(1)
	select {
	case l.queue <- event:
		return nil
	default:
		l.pending.Add(-1)
		l.failed.Add(1)
		return errors.New("ops traffic ledger queue full")
	}
}

func (l *opsTrafficLedger) run() {
	defer close(l.done)
	for first := range l.queue {
		if l.ctx.Err() != nil {
			// stop closes the queue before cancelling the drain. Account for all
			// remaining accepted events without a cancelled DB call/log per row.
			n := int64(1)
			for range l.queue {
				n++
			}
			l.failed.Add(n)
			l.pending.Add(-n)
			log.Printf("[OpsTrafficLedger] shutdown could not persist %d request events; coverage is incomplete", n)
			return
		}
		batch := make([]*OpsTrafficEvent, 0, opsTrafficBatchSize)
		batch = append(batch, first)
		timer := time.NewTimer(opsTrafficBatchWindow)
	collect:
		for len(batch) < opsTrafficBatchSize {
			select {
			case next, ok := <-l.queue:
				if !ok {
					break collect
				}
				batch = append(batch, next)
			case <-timer.C:
				break collect
			case <-l.ctx.Done():
				break collect
			}
		}
		timer.Stop()
		l.flush(batch)
	}
}

func (l *opsTrafficLedger) flush(batch []*OpsTrafficEvent) {
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(l.ctx, opsTrafficWriteTimeout)
		err = l.repo.BatchInsertTrafficEvents(ctx, batch)
		cancel()
		if err == nil || l.ctx.Err() != nil {
			break
		}
	}
	n := int64(len(batch))
	if err != nil {
		l.failed.Add(n)
		// No payload/dimensions or driver error (which can echo SQL values).
		log.Printf("[OpsTrafficLedger] failed to persist %d request events after bounded retries; coverage is incomplete", n)
	} else {
		l.flushed.Add(n)
	}
	l.pending.Add(-n)
}

func (s *OpsService) GetTrafficLedgerHealth() *OpsTrafficLedgerHealth {
	if s == nil {
		return nil
	}
	s.trafficLedgerMu.Lock()
	ledger := s.trafficLedger
	s.trafficLedgerMu.Unlock()
	if ledger == nil {
		return nil
	}
	return &OpsTrafficLedgerHealth{Scope: "instance_lifetime", Pending: ledger.pending.Load(), Failed: ledger.failed.Load(), Flushed: ledger.flushed.Load()}
}

func (s *OpsService) StopTrafficLedger() {
	if s == nil {
		return
	}
	s.trafficLedgerMu.Lock()
	s.trafficLedgerStopped = true
	ledger := s.trafficLedger
	s.trafficLedgerMu.Unlock()
	if ledger != nil {
		ledger.stop()
	}
}

func (l *opsTrafficLedger) stop() {
	l.stopOnce.Do(func() {
		l.mu.Lock()
		l.stopping = true
		close(l.queue)
		l.mu.Unlock()
		timer := time.NewTimer(opsTrafficDrainTimeout)
		defer timer.Stop()
		select {
		case <-l.done:
		case <-timer.C:
			l.cancel()
			<-l.done
		}
		l.cancel()
	})
}
