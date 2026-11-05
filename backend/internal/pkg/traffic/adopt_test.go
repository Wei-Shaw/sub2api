package traffic

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectorAdoptMovesLiveCounters(t *testing.T) {
	target := NewCollector()
	probe := NewCollector()
	counter := probe.Begin(UpstreamRequest, true)
	counter.SetOriginal(100)
	body := ObserveBody(io.NopCloser(strings.NewReader("0123456789")), counter, false, -1)

	// Bytes read before adoption stay on the same counter, and bytes read after
	// adoption must keep landing in the destination.
	prefix := make([]byte, 4)
	_, err := io.ReadFull(body, prefix)
	require.NoError(t, err)

	require.True(t, target.Adopt(probe))
	require.Nil(t, probe.Snapshot().Upstream, "the source must not keep a second copy")

	rest, err := io.ReadAll(body)
	require.NoError(t, err)
	require.NoError(t, body.Close())

	usage := target.Snapshot()
	require.NotNil(t, usage.Upstream)
	require.NotNil(t, usage.Upstream.Request)
	require.True(t, usage.Upstream.Request.Compressed)
	require.EqualValues(t, 100, *usage.Upstream.Request.OriginalBytes)
	require.EqualValues(t, len(prefix)+len(rest), *usage.Upstream.Request.ActualBytes)
	require.NotNil(t, usage.Upstream.Request.Ratio())
}

func TestCollectorAdoptRefusesFinalizedCollectors(t *testing.T) {
	finalized := NewCollector()
	probe := NewCollector()
	counter := probe.Begin(UpstreamRequest, false)
	counter.AddActual(7)
	counter.CompleteActual()

	finalized.Finalize()
	require.False(t, finalized.Adopt(probe))
	require.NotNil(t, probe.Snapshot().Upstream, "refused adoption must not consume the source")

	source := NewCollector()
	source.Begin(UpstreamRequest, false).AddActual(9)
	source.Finalize()
	target := NewCollector()
	require.False(t, target.Adopt(source))
	require.Nil(t, target.Snapshot().Upstream, "published measurements must not be adopted twice")
}

func TestCollectorAdoptRejectsExcludedSources(t *testing.T) {
	target := NewCollector()
	probe := NewCollector()
	counter := probe.Begin(UpstreamRequest, false)
	counter.AddActual(5)
	counter.CompleteActual()
	probe.ExcludeUpstream()

	require.False(t, target.Adopt(probe))
	require.Nil(t, target.Snapshot().Upstream)
}

func TestCollectorAdoptIgnoresNilAndSelf(t *testing.T) {
	var nilCollector *Collector
	require.False(t, nilCollector.Adopt(NewCollector()))

	collector := NewCollector()
	counter := collector.Begin(DownstreamRequest, false)
	counter.AddActual(3)
	counter.CompleteActual()

	require.False(t, collector.Adopt(nil))
	require.False(t, collector.Adopt(collector))
	require.EqualValues(t, 3, *collector.Snapshot().Downstream.Request.ActualBytes)
}

func TestCollectorAdoptTwiceDoesNotDoubleCount(t *testing.T) {
	target := NewCollector()
	probe := NewCollector()
	counter := probe.Begin(UpstreamResponse, false)
	counter.AddActual(11)
	counter.AddOriginal(11)
	counter.CompleteActual()
	counter.CompleteOriginal()

	require.True(t, target.Adopt(probe))
	require.False(t, target.Adopt(probe))

	target.Finalize()
	usage := target.Snapshot()
	require.EqualValues(t, 11, *usage.Upstream.Response.ActualBytes)
	require.EqualValues(t, 11, *usage.Upstream.Response.OriginalBytes)
}
