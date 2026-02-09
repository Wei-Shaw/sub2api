//go:build unit

package service

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionMaintenanceQueue_TryEnqueue_QueueFull(t *testing.T) {
	q := NewSubscriptionMaintenanceQueue(1, 1)
	t.Cleanup(q.Stop)

	block := make(chan struct{REDACTED)
	var started atomic.Int32

	require.NoError(t, q.TryEnqueue(func() {
		started.Store(1)
		<-block
REDACTED))

	// Wait until worker started consuming the first task.
	require.Eventually(t, func() bool { return started.Load() == 1 REDACTED, time.Second, 10*time.Millisecond)

	// Queue size is 1; with the worker blocked, enqueueing one more should fill it.
	require.NoError(t, q.TryEnqueue(func() {REDACTED))

	// Now the queue is full; next enqueue must fail.
	err := q.TryEnqueue(func() {REDACTED)
REDACTED
	require.Contains(t, err.Error(), "full")

	close(block)
REDACTED

func TestSubscriptionMaintenanceQueue_TryEnqueue_PanicDoesNotKillWorker(t *testing.T) {
	q := NewSubscriptionMaintenanceQueue(1, 8)
	t.Cleanup(q.Stop)

	require.NoError(t, q.TryEnqueue(func() { panic("boom") REDACTED))

	done := make(chan struct{REDACTED)
	require.NoError(t, q.TryEnqueue(func() { close(done) REDACTED))

	select {
	case <-done:
		// ok
	case <-time.After(time.Second):
		t.Fatalf("worker did not continue after panic")
REDACTED
REDACTED
