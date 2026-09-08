package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type codexLeaseLifecycleCache struct {
	ConcurrencyCache

	mu            sync.Mutex
	owners        map[string]string
	expiresAt     map[string]time.Time
	leaseTTL      time.Duration
	refreshResult bool
	refreshSignal chan time.Time
}

func (c *codexLeaseLifecycleCache) AcquireCodexDeviceConversationLease(_ context.Context, slotKey, leaseID string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owners == nil {
		c.owners = make(map[string]string)
	}
	if c.expiresAt == nil {
		c.expiresAt = make(map[string]time.Time)
	}
	if c.owners[slotKey] != "" {
		if c.leaseTTL <= 0 || time.Now().Before(c.expiresAt[slotKey]) {
			return false, nil
		}
		delete(c.owners, slotKey)
		delete(c.expiresAt, slotKey)
	}
	c.owners[slotKey] = leaseID
	if c.leaseTTL > 0 {
		c.expiresAt[slotKey] = time.Now().Add(c.leaseTTL)
	}
	return true, nil
}

func (c *codexLeaseLifecycleCache) RefreshCodexDeviceConversationLease(_ context.Context, slotKey, leaseID string) (bool, error) {
	c.mu.Lock()
	if c.owners[slotKey] != leaseID {
		c.mu.Unlock()
		return false, nil
	}
	refreshedAt := time.Now()
	if c.refreshResult && c.leaseTTL > 0 {
		c.expiresAt[slotKey] = refreshedAt.Add(c.leaseTTL)
	}
	refreshResult := c.refreshResult
	c.mu.Unlock()
	if refreshResult && c.refreshSignal != nil {
		select {
		case c.refreshSignal <- refreshedAt:
		default:
		}
	}
	return refreshResult, nil
}

func (c *codexLeaseLifecycleCache) ReleaseCodexDeviceConversationLease(_ context.Context, slotKey, leaseID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owners[slotKey] == leaseID {
		delete(c.owners, slotKey)
		delete(c.expiresAt, slotKey)
	}
	return nil
}

var _ CodexDeviceConversationLeaseCache = (*codexLeaseLifecycleCache)(nil)

func newTestCodexDeviceConversationLease(
	parent context.Context,
	cache CodexDeviceConversationLeaseCache,
	refreshInterval, leaseTTL time.Duration,
) (*CodexDeviceConversationLease, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	attemptCtx, attemptCancel := context.WithCancelCause(parent)
	refreshCtx, refreshCancel := context.WithCancelCause(context.Background())
	upstreamCtx, upstreamCancel := context.WithCancelCause(context.WithoutCancel(parent))
	lease := &CodexDeviceConversationLease{
		ctx:             attemptCtx,
		cancel:          attemptCancel,
		refreshCtx:      refreshCtx,
		refreshCancel:   refreshCancel,
		upstreamCtx:     upstreamCtx,
		upstreamCancel:  upstreamCancel,
		cache:           cache,
		slotKey:         "test-slot",
		leaseID:         "test-lease",
		stopCh:          make(chan struct{}),
		refreshDone:     make(chan struct{}),
		refreshInterval: refreshInterval,
		leaseTTL:        leaseTTL,
		operationTO:     20 * time.Millisecond,
	}
	go lease.refreshLoop()
	return lease, func() { lease.Release() }
}

func TestCodexDeviceConversationLeaseRefreshOutlivesClientCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	const leaseTTL = 100 * time.Millisecond
	refreshSignal := make(chan time.Time, 4)
	cache := &codexLeaseLifecycleCache{
		leaseTTL:      leaseTTL,
		refreshResult: true,
		refreshSignal: refreshSignal,
	}
	acquired, err := cache.AcquireCodexDeviceConversationLease(context.Background(), "test-slot", "test-lease")
	require.NoError(t, err)
	require.True(t, acquired)
	lease, cleanup := newTestCodexDeviceConversationLease(parent, cache, 5*time.Millisecond, leaseTTL)
	defer cleanup()

	select {
	case <-refreshSignal:
	case <-time.After(time.Second):
		t.Fatal("device lease refresh loop did not start")
	}
	for len(refreshSignal) > 0 {
		<-refreshSignal
	}

	canceledAt := time.Now()
	cancelParent()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case refreshedAt := <-refreshSignal:
			if !refreshedAt.Before(canceledAt.Add(leaseTTL)) {
				goto refreshedBeyondTTL
			}
		case <-deadline:
			t.Fatal("device lease refresh stopped before the detached upstream drain crossed the lease TTL")
		}
	}

refreshedBeyondTTL:
	secondAcquired, err := cache.AcquireCodexDeviceConversationLease(context.Background(), "test-slot", "other-owner")
	require.NoError(t, err)
	require.False(t, secondAcquired, "a detached upstream drain must keep the physical slot occupied")

	upstreamCtx, release := detachUpstreamContext(withCodexDeviceConversationLeaseContext(lease.Context(), lease))
	defer release()
	require.NoError(t, upstreamCtx.Err(), "detached upstream context must outlive client cancellation")
}

func TestCodexDeviceConversationLeaseLossCancelsDetachedUpstreamContext(t *testing.T) {
	cache := &codexLeaseLifecycleCache{refreshResult: false}
	lease, cleanup := newTestCodexDeviceConversationLease(context.Background(), cache, 5*time.Millisecond, time.Second)
	defer cleanup()

	upstreamCtx := lease.UpstreamContext()
	select {
	case <-upstreamCtx.Done():
		require.ErrorIs(t, context.Cause(upstreamCtx), ErrCodexDeviceConversationLeaseLost)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("detached upstream context was not canceled after lease ownership was lost")
	}
}

func TestCodexDeviceConversationLeaseKeepsSlotBusyAfterClientCancellation(t *testing.T) {
	cache := &codexLeaseLifecycleCache{refreshResult: true}
	service := NewConcurrencyService(cache)
	parent, cancelParent := context.WithCancel(context.Background())
	first, acquired, err := service.AcquireCodexDeviceConversationLease(parent, "shared-slot")
	require.NoError(t, err)
	require.True(t, acquired)
	cancelParent()

	second, acquired, err := service.AcquireCodexDeviceConversationLease(context.Background(), "shared-slot")
	require.NoError(t, err)
	require.False(t, acquired)
	require.Nil(t, second)

	first.Release()
	third, acquired, err := service.AcquireCodexDeviceConversationLease(context.Background(), "shared-slot")
	require.NoError(t, err)
	require.True(t, acquired)
	require.NotNil(t, third)
	third.Release()
	require.NotErrorIs(t, context.Cause(first.UpstreamContext()), ErrCodexDeviceConversationLeaseLost)
	require.False(t, errors.Is(first.Context().Err(), ErrCodexDeviceConversationLeaseLost))
}
