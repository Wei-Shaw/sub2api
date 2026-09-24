package dataset

import (
	"log"
	"sync"
)

// Collector collects LLM request/response pairs in-memory with bounded buffer.
// Designed to be fail-open: full buffer drops new entries without blocking.
type Collector struct {
	buffer     chan DatasetEntry
	maxItems   int
	mu         sync.RWMutex
	dropCount  uint64
	totalCount uint64
}

// NewCollector creates a new dataset collector with bounded buffer.
func NewCollector(maxItems int) *Collector {
	if maxItems <= 0 {
		maxItems = 1000
	}
	return &Collector{
		buffer:   make(chan DatasetEntry, maxItems),
		maxItems: maxItems,
	}
}

// Collect adds a dataset entry to the buffer in a non-blocking way.
// If buffer is full, the entry is dropped (fail-open).
func (c *Collector) Collect(entry *DatasetEntry) {
	if entry == nil {
		return
	}

	select {
	case c.buffer <- *entry:
		c.mu.Lock()
		c.totalCount++
		c.mu.Unlock()
	default:
		// Buffer full, drop entry
		c.mu.Lock()
		c.dropCount++
		dropped := c.dropCount
		c.mu.Unlock()

		// Log warning every 100 drops to avoid spam
		if dropped%100 == 1 {
			log.Printf("[Dataset] Buffer full, dropped %d entries (capacity: %d)", dropped, c.maxItems)
		}
	}
}

// Drain returns available entries (up to limit) and removes them from buffer.
// Non-blocking: returns immediately with whatever is available.
func (c *Collector) Drain(limit int) []DatasetEntry {
	if limit <= 0 {
		limit = c.maxItems
	}

	var entries []DatasetEntry
	for i := 0; i < limit; i++ {
		select {
		case entry := <-c.buffer:
			entries = append(entries, entry)
		default:
			return entries
		}
	}
	return entries
}

// Peek returns a snapshot of available entries without removing them.
// Used for preview before upload to allow retry on failure.
func (c *Collector) Peek(limit int) []DatasetEntry {
	if limit <= 0 {
		limit = c.maxItems
	}

	// Non-blocking snapshot
	buffered := len(c.buffer)
	if buffered == 0 {
		return nil
	}

	if limit > buffered {
		limit = buffered
	}

	entries := make([]DatasetEntry, 0, limit)
peekLoop:
	for i := 0; i < limit; i++ {
		select {
		case entry := <-c.buffer:
			entries = append(entries, entry)
		default:
			break peekLoop
		}
	}

	// Re-enqueue all peeked entries (preserve order)
	for i := range entries {
		select {
		case c.buffer <- entries[i]:
		default:
			// Buffer full during re-enqueue (shouldn't happen, but fail-open)
			log.Printf("[Dataset] WARN: Buffer full during peek re-enqueue, dropped %d entries", len(entries)-i)
			return entries[:i]
		}
	}

	return entries
}

// Clear removes N entries from the front of the buffer.
// Used after successful upload to confirm entries were persisted.
func (c *Collector) Clear(count int) {
	for i := 0; i < count; i++ {
		select {
		case <-c.buffer:
			// Successfully removed
		default:
			// Buffer already empty
			return
		}
	}
}

// Stats returns current statistics.
func (c *Collector) Stats() (total, dropped uint64, buffered int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.totalCount, c.dropCount, len(c.buffer)
}

// Len returns current buffer size.
func (c *Collector) Len() int {
	return len(c.buffer)
}
