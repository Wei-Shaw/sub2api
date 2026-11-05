package traffic

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestUnknownEmptyAndRatio(t *testing.T) {
	collector := NewCollector()
	b := collector.Begin(DownstreamRequest, false)
	if got := collector.Snapshot().Downstream.Request; got.OriginalBytes != nil || got.ActualBytes != nil {
		t.Fatal("unobserved body is not zero")
	}
	b.CompleteActual()
	b.CompleteOriginal()
	got := collector.Snapshot().Downstream.Request
	if *got.ActualBytes != 0 || *got.OriginalBytes != 0 || got.Ratio() != nil {
		t.Fatalf("empty: %+v", got)
	}
	original, actual := int64(1), int64(2)
	m := &Measurement{OriginalBytes: &original, ActualBytes: &actual, Compressed: true}
	if *m.Ratio() != -100 {
		t.Fatal("compression expansion must remain negative")
	}
	data, err := json.Marshal(&Usage{Downstream: &Link{Request: &Measurement{}}})
	if err != nil || !strings.Contains(string(data), `"actual_bytes":null`) {
		t.Fatalf("unknown JSON: %s %v", data, err)
	}
}

func TestAttemptAggregationPropagatesUnknown(t *testing.T) {
	collector := NewCollector()
	for _, size := range []int{3, 7} {
		counter := collector.Begin(UpstreamRequest, false)
		counter.AddOriginal(size)
		counter.AddActual(size)
		counter.CompleteOriginal()
		counter.CompleteActual()
	}
	if got := collector.Snapshot().Upstream.Request; *got.ActualBytes != 10 || *got.OriginalBytes != 10 {
		t.Fatal(got)
	}
	partial := collector.Begin(UpstreamRequest, true)
	partial.AddActual(5)
	got := collector.Snapshot().Upstream.Request
	if got.ActualBytes != nil || got.OriginalBytes != nil || !got.Compressed {
		t.Fatal("partial attempt must not be omitted", got)
	}
}

func TestFinalizeIsOnceReentrantAndIsolated(t *testing.T) {
	collector := NewCollector()
	counter := collector.Begin(DownstreamResponse, false)
	counter.AddActual(3)
	counter.AddOriginal(3)
	counter.CompleteActual()
	counter.CompleteOriginal()
	ctx := WithCollector(context.Background(), collector)
	var calls int
	AfterFinalize(ctx, func(u *Usage) {
		calls++
		*u.Downstream.Response.ActualBytes = 999
		collector.OnFinalize(func(other *Usage) {
			calls++
			if *other.Downstream.Response.ActualBytes != 3 {
				t.Fatal("callback alias")
			}
		})
	})
	collector.Finalize()
	collector.Finalize()
	counter.AddActual(100)
	if calls != 2 || *Snapshot(ctx).Downstream.Response.ActualBytes != 3 {
		t.Fatal("finalization not frozen")
	}
}

func TestConcurrentFinalizeRegistration(t *testing.T) {
	collector := NewCollector()
	var mu sync.Mutex
	calls := 0
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() { defer wg.Done(); collector.OnFinalize(func(*Usage) { mu.Lock(); calls++; mu.Unlock() }) }()
	}
	collector.Finalize()
	wg.Wait()
	if calls != 50 {
		t.Fatal(calls)
	}
}

func TestObservedBodyDoesNotEstimateUnreadBytes(t *testing.T) {
	collector := NewCollector()
	counter := collector.Begin(DownstreamRequest, false)
	body := ObserveBody(io.NopCloser(strings.NewReader("payload")), counter, true, 7)
	buffer := make([]byte, 2)
	_, _ = body.Read(buffer)
	_ = body.Close()
	if got := collector.Snapshot().Downstream.Request; got.ActualBytes != nil || got.OriginalBytes != nil {
		t.Fatal("partial read reported as complete", got)
	}
}

func TestExcludeAndMissingCollector(t *testing.T) {
	collector := NewCollector()
	collector.Begin(DownstreamRequest, false)
	collector.Exclude()
	collector.OnFinalize(func(u *Usage) {
		if u != nil {
			t.Fatal("WebSocket must stay unknown")
		}
	})
	collector.Finalize()
	AfterFinalize(context.Background(), func(u *Usage) {
		if u != nil {
			t.Fatal("missing collector must stay unknown")
		}
	})
}
