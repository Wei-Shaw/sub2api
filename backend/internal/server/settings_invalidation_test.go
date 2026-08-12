package server

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeSettingsInvalidationBus struct {
	mu           sync.Mutex
	subscribers  []func()
	publishErr   error
	subscribeErr error
}

func (b *fakeSettingsInvalidationBus) Publish(context.Context) error {
	if b.publishErr != nil {
		return b.publishErr
	}
	b.mu.Lock()
	subscribers := append([]func(){}, b.subscribers...)
	b.mu.Unlock()
	for _, subscriber := range subscribers {
		subscriber()
	}
	return nil
}

func (b *fakeSettingsInvalidationBus) Subscribe(_ context.Context, handler func()) error {
	if b.subscribeErr != nil {
		return b.subscribeErr
	}
	b.mu.Lock()
	b.subscribers = append(b.subscribers, handler)
	b.mu.Unlock()
	return nil
}

func TestConfigureSettingsInvalidation_BroadcastsToEveryReplica(t *testing.T) {
	bus := &fakeSettingsInvalidationBus{}
	var replicaAInvalidations int
	var replicaBInvalidations int

	notifyA := configureSettingsInvalidation(context.Background(), bus, func() {
		replicaAInvalidations++
	}, nil)
	_ = configureSettingsInvalidation(context.Background(), bus, func() {
		replicaBInvalidations++
	}, nil)

	notifyA()

	require.Equal(t, 2, replicaAInvalidations)
	require.Equal(t, 1, replicaBInvalidations)
}

func TestConfigureSettingsInvalidation_PublishFailureKeepsLocalInvalidation(t *testing.T) {
	bus := &fakeSettingsInvalidationBus{publishErr: errors.New("redis unavailable")}
	var invalidations int
	var receivedErr error

	notify := configureSettingsInvalidation(context.Background(), bus, func() {
		invalidations++
	}, func(err error) {
		receivedErr = err
	})
	notify()

	require.Equal(t, 1, invalidations)
	require.ErrorContains(t, receivedErr, "publish settings invalidation")
}

func TestConfigureSettingsInvalidation_SubscribeFailureIsReported(t *testing.T) {
	bus := &fakeSettingsInvalidationBus{subscribeErr: errors.New("subscribe failed")}
	var receivedErr error

	_ = configureSettingsInvalidation(context.Background(), bus, nil, func(err error) {
		receivedErr = err
	})

	require.ErrorContains(t, receivedErr, "subscribe failed")
}
