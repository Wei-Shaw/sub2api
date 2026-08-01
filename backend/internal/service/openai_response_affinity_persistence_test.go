package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type responseAccountBindProbe struct {
	OpenAIWSStateStore
	calls       int
	ctxErr      error
	hasDeadline bool
	deadline    time.Time
	value       any
}

type responseAccountBindProbeContextKey struct{}

func (p *responseAccountBindProbe) BindResponseAccount(ctx context.Context, _ int64, _ string, _ int64, _ time.Duration) error {
	p.calls++
	p.ctxErr = ctx.Err()
	p.deadline, p.hasDeadline = ctx.Deadline()
	p.value = ctx.Value(responseAccountBindProbeContextKey{})
	return p.ctxErr
}

func TestBindOpenAIWSResponseAccountDetachesCanceledRequest(t *testing.T) {
	parent := context.WithValue(context.Background(), responseAccountBindProbeContextKey{}, "request-value")
	parent, cancel := context.WithCancel(parent)
	cancel()
	probe := &responseAccountBindProbe{}
	before := time.Now()

	bindOpenAIWSResponseAccount(parent, probe, 4201, "resp_detached", 37001, time.Hour)

	require.Equal(t, 1, probe.calls)
	require.NoError(t, probe.ctxErr)
	require.True(t, probe.hasDeadline)
	require.Equal(t, "request-value", probe.value)
	require.WithinDuration(t, before.Add(openAIWSBindResponseAccountTimeout), probe.deadline, time.Second)
}

func TestBindOpenAIWSResponseAccountReplacesExpiredParentDeadline(t *testing.T) {
	parent := context.WithValue(context.Background(), responseAccountBindProbeContextKey{}, "deadline-value")
	parent, cancel := context.WithDeadline(parent, time.Now().Add(-time.Minute))
	defer cancel()
	require.ErrorIs(t, parent.Err(), context.DeadlineExceeded)
	probe := &responseAccountBindProbe{}
	before := time.Now()

	bindOpenAIWSResponseAccount(parent, probe, 4201, "resp_deadline", 37001, time.Hour)

	require.Equal(t, 1, probe.calls)
	require.NoError(t, probe.ctxErr)
	require.True(t, probe.hasDeadline)
	require.True(t, probe.deadline.After(before))
	require.Equal(t, "deadline-value", probe.value)
	require.WithinDuration(t, before.Add(openAIWSBindResponseAccountTimeout), probe.deadline, time.Second)
}
