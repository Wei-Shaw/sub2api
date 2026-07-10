package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type domainOutboxWorkerRepoFake struct {
	mu        sync.Mutex
	events    []*DomainOutboxEvent
	claimed   int
	completed int
	retried   []struct {
		dead bool
		next time.Time
		err  string
	}
}

func (f *domainOutboxWorkerRepoFake) Enqueue(context.Context, *DomainOutboxEvent) (*DomainOutboxEvent, error) {
	return nil, nil
}
func (f *domainOutboxWorkerRepoFake) GetByID(context.Context, int64) (*DomainOutboxEvent, error) {
	return nil, nil
}
func (f *domainOutboxWorkerRepoFake) ClaimBatch(_ context.Context, _ string, _ time.Time, limit int, _ time.Duration) ([]*DomainOutboxEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimed++
	if len(f.events) > limit {
		return f.events[:limit], nil
	}
	return f.events, nil
}
func (f *domainOutboxWorkerRepoFake) Complete(context.Context, int64, string, time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completed++
	return true, nil
}
func (f *domainOutboxWorkerRepoFake) Retry(_ context.Context, _ int64, _ string, next time.Time, dead bool, err string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retried = append(f.retried, struct {
		dead bool
		next time.Time
		err  string
	}{dead, next, err})
	return true, nil
}
func (f *domainOutboxWorkerRepoFake) ReapExpiredLeases(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (f *domainOutboxWorkerRepoFake) Counts(context.Context) (DomainOutboxCounts, error) {
	return DomainOutboxCounts{}, nil
}

type domainOutboxWorkerHandlerFake struct {
	err   error
	panic bool
}

func (h domainOutboxWorkerHandlerFake) Handle(context.Context, *DomainOutboxEvent) error {
	if h.panic {
		panic("authorization=secret https://example.test/a?token=secret")
	}
	return h.err
}

func TestDomainOutboxWorkerRetriesWithDeterministicBackoff(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	repo := &domainOutboxWorkerRepoFake{events: []*DomainOutboxEvent{{ID: 1, EventType: "x", AttemptCount: 1}}}
	w := NewDomainOutboxWorker(repo, domainOutboxWorkerHandlerFake{err: errors.New("temporary")}, nil, DomainOutboxWorkerOptions{WorkerID: "test", Now: func() time.Time { return now }})
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.retried) != 1 || repo.retried[0].dead {
		t.Fatalf("retry = %#v", repo.retried)
	}
	if got, want := repo.retried[0].next, now.Add(5*time.Second); !got.Equal(want) {
		t.Fatalf("next = %s, want %s", got, want)
	}
}

func TestDomainOutboxWorkerPanicRetriesAndUnknownDeadLetters(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	panicRepo := &domainOutboxWorkerRepoFake{events: []*DomainOutboxEvent{{ID: 1, EventType: "x", AttemptCount: 1}}}
	panicWorker := NewDomainOutboxWorker(panicRepo, domainOutboxWorkerHandlerFake{panic: true}, nil, DomainOutboxWorkerOptions{Now: func() time.Time { return now }})
	_ = panicWorker.RunOnce(context.Background())
	if len(panicRepo.retried) != 1 || panicRepo.retried[0].dead {
		t.Fatalf("panic retry = %#v", panicRepo.retried)
	}
	unknownRepo := &domainOutboxWorkerRepoFake{events: []*DomainOutboxEvent{{ID: 2, EventType: "unknown", AttemptCount: 1}}}
	unknownWorker := NewDomainOutboxWorker(unknownRepo, DomainOutboxHandlerRegistry{}, nil, DomainOutboxWorkerOptions{Now: func() time.Time { return now }})
	_ = unknownWorker.RunOnce(context.Background())
	if len(unknownRepo.retried) != 1 || !unknownRepo.retried[0].dead {
		t.Fatalf("unknown = %#v", unknownRepo.retried)
	}
}

func TestSanitizeOutboxLogError(t *testing.T) {
	got := sanitizeOutboxLogError(errors.New("request https://example.test/a?token=secret authorization=BearerSecret"))
	if strings.Contains(got, "token=secret") || strings.Contains(got, "BearerSecret") {
		t.Fatalf("unsanitized log = %q", got)
	}
}

func TestDomainOutboxWorkerStopWithoutStartDoesNotClaim(t *testing.T) {
	repo := &domainOutboxWorkerRepoFake{}
	w := NewDomainOutboxWorker(repo, domainOutboxWorkerHandlerFake{}, nil)
	w.Stop()
	if repo.claimed != 0 {
		t.Fatalf("stop claimed events: %d", repo.claimed)
	}
}
