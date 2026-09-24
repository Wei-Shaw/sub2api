package dataset

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mockDriveClient simulates Google Drive upload with controlled failure.
type mockDriveClient struct {
	mu              sync.Mutex
	uploadCount     int
	failUntil       int // Fail uploads until this count
	uploadedBatches [][]DatasetEntry
}

func (m *mockDriveClient) UploadBatch(_ context.Context, entries []DatasetEntry) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.uploadCount++
	if m.uploadCount <= m.failUntil {
		return "", errors.New("simulated Drive failure")
	}

	// Success: record batch
	batch := make([]DatasetEntry, len(entries))
	copy(batch, entries)
	m.uploadedBatches = append(m.uploadedBatches, batch)

	return "mock-file-id", nil
}

func TestWorker_RetainsEntriesOnUploadFailure(t *testing.T) {
	collector := NewCollector(10)
	mockDrive := &mockDriveClient{failUntil: 999} // Always fail

	worker := NewWorker(collector, mockDrive, 5, 10, 1)

	// Add entries
	for i := 0; i < 3; i++ {
		collector.Collect(&DatasetEntry{
			Timestamp: time.Now(),
			Endpoint:  "chat.completions",
			Request:   map[string]any{"id": i},
		})
	}

	// Flush should fail
	worker.flushBatch(context.Background())

	// Entries should remain in buffer
	require.Equal(t, 3, collector.Len(), "failed upload should retain entries")
	require.Empty(t, mockDrive.uploadedBatches, "no batch should be recorded on failure")
}

func TestWorker_RetriesUploadWithBackoff(t *testing.T) {
	collector := NewCollector(10)
	mockDrive := &mockDriveClient{failUntil: 2} // Fail first 2 attempts, succeed on 3rd

	worker := NewWorker(collector, mockDrive, 5, 10, 1)

	// Add entry
	collector.Collect(&DatasetEntry{
		Timestamp: time.Now(),
		Endpoint:  "chat.completions",
		Request:   map[string]any{"test": true},
	})

	start := time.Now()
	worker.flushBatch(context.Background())
	elapsed := time.Since(start)

	// Should retry with backoff: ~2s + ~4s = ~6s total
	require.GreaterOrEqual(t, elapsed, 5*time.Second, "should have retried with backoff")
	require.Equal(t, 3, mockDrive.uploadCount, "should retry 3 times")
	require.Len(t, mockDrive.uploadedBatches, 1, "should succeed on 3rd attempt")
	require.Equal(t, 0, collector.Len(), "should clear buffer after successful upload")
}

func TestWorker_ClearsEntriesOnlyAfterSuccessfulUpload(t *testing.T) {
	collector := NewCollector(10)
	mockDrive := &mockDriveClient{failUntil: 0} // Always succeed

	worker := NewWorker(collector, mockDrive, 5, 10, 1)

	// Add entries
	for i := 0; i < 3; i++ {
		collector.Collect(&DatasetEntry{
			Timestamp: time.Now(),
			Endpoint:  "chat.completions",
			Request:   map[string]any{"id": i},
		})
	}

	// Flush should succeed
	worker.flushBatch(context.Background())

	// Buffer should be empty after successful upload
	require.Equal(t, 0, collector.Len(), "successful upload should clear buffer")
	require.Len(t, mockDrive.uploadedBatches, 1, "should record uploaded batch")
	require.Len(t, mockDrive.uploadedBatches[0], 3, "should upload all entries")
}

func TestCollector_PeekDoesNotRemoveEntries(t *testing.T) {
	collector := NewCollector(10)

	// Add entries
	for i := 0; i < 5; i++ {
		collector.Collect(&DatasetEntry{
			Timestamp: time.Now(),
			Endpoint:  "chat.completions",
			Request:   map[string]any{"id": i},
		})
	}

	// Peek should return entries without removing
	peeked := collector.Peek(3)
	require.Len(t, peeked, 3, "should peek 3 entries")
	require.Equal(t, 5, collector.Len(), "buffer size should remain unchanged")

	// Peek again should return same entries
	peeked2 := collector.Peek(3)
	require.Len(t, peeked2, 3, "should peek same entries again")
	require.Equal(t, 5, collector.Len(), "buffer size should still be 5")
}

func TestCollector_ClearRemovesExactCount(t *testing.T) {
	collector := NewCollector(10)

	// Add entries
	for i := 0; i < 5; i++ {
		collector.Collect(&DatasetEntry{
			Timestamp: time.Now(),
			Endpoint:  "chat.completions",
			Request:   map[string]any{"id": i},
		})
	}

	// Clear 3 entries
	collector.Clear(3)
	require.Equal(t, 2, collector.Len(), "should have 2 entries remaining")

	// Clear beyond available
	collector.Clear(10)
	require.Equal(t, 0, collector.Len(), "should clear all remaining entries")
}
