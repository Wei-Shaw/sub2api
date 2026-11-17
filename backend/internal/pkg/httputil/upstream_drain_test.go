package httputil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpstreamDrainStartsOnlyAfterClientCancellation(t *testing.T) {
	client, cancelClient := context.WithCancel(context.Background())
	defer cancelClient()
	upstream, release := NewUpstreamAttemptContext(DetachUpstreamContext(client), 30*time.Millisecond)
	defer release()
	time.Sleep(60 * time.Millisecond)
	require.NoError(t, upstream.Err())
	cancelClient()
	select {
	case <-upstream.Done():
		t.Fatal("cancelled before late usage drain window")
	case <-time.After(10 * time.Millisecond):
	}
	select {
	case <-upstream.Done():
		require.ErrorIs(t, upstream.Err(), context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("drain exceeded total budget")
	}
}

func TestUpstreamDrainOverrideAndNestedDetachKeepOriginalSignal(t *testing.T) {
	client, cancelClient := context.WithCancel(context.Background())
	parent := DetachUpstreamContext(DetachUpstreamContext(client, 20*time.Millisecond))
	upstream, release := NewUpstreamAttemptContext(parent, time.Second)
	defer release()
	cancelClient()
	select {
	case <-upstream.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("nested detach lost original cancellation or override")
	}
}

func TestUpstreamDrainReleaseDoesNotCancelCaller(t *testing.T) {
	client, cancelClient := context.WithCancel(context.Background())
	defer cancelClient()
	upstream, release := NewUpstreamAttemptContext(DetachUpstreamContext(client), time.Second)
	release()
	release()
	require.ErrorIs(t, upstream.Err(), context.Canceled)
	require.NoError(t, client.Err())
	cancelClient()
}
