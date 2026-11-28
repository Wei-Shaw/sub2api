package repository

import (
	"sync"
	"time"
)

// usageLogRetryQueue holds the best-effort rows that failed with a transient error. Every row has
// its own backoff schedule (attempt and due), so a row that keeps failing does not slow down the
// others, and the rows are never merged into the batches of newly arriving rows.
type usageLogRetryQueue struct {
	mu   sync.Mutex
	rows []usageLogBestEffortRequest
	// wake has capacity 1 and is signalled when rows were added.
	wake chan struct{}
	// done is closed when the batcher stops.
	done chan struct{}
}

func newUsageLogRetryQueue() *usageLogRetryQueue {
	return &usageLogRetryQueue{wake: make(chan struct{}, 1), done: make(chan struct{})}
}

// schedule adds rows to the queue, each due after the next backoff step of its own.
func (q *usageLogRetryQueue) schedule(p usageLogWriteRetry, rows []usageLogBestEffortRequest) {
	if len(rows) == 0 {
		return
	}
	now := time.Now()
	q.mu.Lock()
	for _, req := range rows {
		req.due = now.Add(p.delay(req.attempt))
		req.attempt++
		q.rows = append(q.rows, req)
	}
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// takeDue removes up to limit rows that are due. next is the earliest due time of the rows that stay,
// or the zero time when none stay.
func (q *usageLogRetryQueue) takeDue(limit int) (batch []usageLogBestEffortRequest, next time.Time) {
	now := time.Now()
	q.mu.Lock()
	defer q.mu.Unlock()
	kept := q.rows[:0]
	for _, req := range q.rows {
		if len(batch) < limit && !req.due.After(now) {
			batch = append(batch, req)
			continue
		}
		kept = append(kept, req)
		if next.IsZero() || req.due.Before(next) {
			next = req.due
		}
	}
	clear(q.rows[len(kept):])
	q.rows = kept
	return batch, next
}

func (q *usageLogRetryQueue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.rows)
}

// waitBelow blocks while the queue holds limit rows or more. The batcher stops taking new rows from
// the queue meanwhile, so memory stays bounded and callers feel the back pressure.
func (q *usageLogRetryQueue) waitBelow(limit int) {
	for q.len() >= limit {
		time.Sleep(50 * time.Millisecond)
	}
}
