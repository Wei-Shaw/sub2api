package repository

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

type responseWaitAfterPrefixBody struct {
	ctx     context.Context
	prefix  *strings.Reader
	waiting chan struct{}
	once    sync.Once
}

func (b *responseWaitAfterPrefixBody) Read(p []byte) (int, error) {
	if b.prefix.Len() > 0 {
		return b.prefix.Read(p)
	}
	b.once.Do(func() { close(b.waiting) })
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (b *responseWaitAfterPrefixBody) Close() error { return nil }

// The gateway's SSE scanner can already be waiting for the next read when its
// terminal-event handler closes the response. That cancellation is not a reason
// to discard response bytes delivered by earlier successful reads.
func TestUpstreamTrafficCloseCancelsReadWithoutDiscardingPrefix(t *testing.T) {
	const prefix = "data: [DONE]\n\n"
	waiting := make(chan struct{})
	client := &http.Client{Transport: trafficTestRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: -1,
			Body: &responseWaitAfterPrefixBody{ctx: req.Context(), prefix: strings.NewReader(prefix), waiting: waiting},
		}, nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	collector := traffic.NewCollector()
	req, err := http.NewRequestWithContext(traffic.WithCollector(ctx, collector), http.MethodGet, "http://unused.test", nil)
	require.NoError(t, err)
	resp, err := doUpstreamRequest(client, req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, err = io.ReadFull(resp.Body, make([]byte, len(prefix)))
	require.NoError(t, err)
	readResult := make(chan error, 1)
	go func() {
		_, err := resp.Body.Read(make([]byte, 1))
		readResult <- err
	}()
	<-waiting
	started := time.Now()
	require.NoError(t, resp.Body.Close())
	require.Less(t, time.Since(started), time.Second)
	require.ErrorIs(t, <-readResult, context.Canceled)
	collector.Finalize()
	measured := collector.Snapshot().Upstream.Response
	require.NotNil(t, measured.ActualBytes)
	require.NotNil(t, measured.OriginalBytes)
	require.EqualValues(t, len(prefix), *measured.ActualBytes)
	require.EqualValues(t, len(prefix), *measured.OriginalBytes)
}
