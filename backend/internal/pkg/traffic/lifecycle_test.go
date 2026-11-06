package traffic

import (
	"testing"
	"time"
)

func TestFinalizedAtIsFrozenAndReadableInCallback(t *testing.T) {
	collector := NewCollector()
	if collector.Finalized() || !collector.FinalizedAt().IsZero() {
		t.Fatal("not finalized")
	}
	var observed time.Time
	collector.OnFinalize(func(*Usage) { observed = collector.FinalizedAt() })
	collector.Finalize()
	if !collector.Finalized() || observed.IsZero() || observed.Location() != time.UTC {
		t.Fatal("finalized timestamp missing or not UTC")
	}
	collector.Finalize()
	if collector.FinalizedAt() != observed {
		t.Fatal("timestamp changed after repeated Finalize")
	}
	var missing *Collector
	if !missing.Finalized() || !missing.FinalizedAt().IsZero() {
		t.Fatal("nil timestamp")
	}
}

func TestExcludedCollectorDrainsOnceWithoutSessionEnd(t *testing.T) {
	collector := NewCollector()
	calls := 0
	callback := func(usage *Usage) {
		calls++
		if usage != nil {
			t.Fatal("excluded data")
		}
	}
	collector.OnFinalize(callback)
	collector.Exclude()
	if !collector.Finalized() || calls != 1 {
		t.Fatal("registered callback retained until session end")
	}
	collector.OnFinalize(callback)
	if calls != 2 {
		t.Fatal("WS turn callback retained until session end")
	}
	collector.Exclude()
	collector.Finalize()
	if calls != 2 {
		t.Fatal("callback double invocation")
	}
}

func TestUpstreamExclusionPreservesDownstreamAndIsSticky(t *testing.T) {
	collector := NewCollector()
	collector.ExcludeUpstream()
	for _, direction := range []Direction{UpstreamRequest, UpstreamResponse, DownstreamRequest, DownstreamResponse} {
		body := collector.Begin(direction, false)
		body.AddOriginal(8)
		body.AddActual(8)
		body.CompleteOriginal()
		body.CompleteActual()
	}
	collector.Finalize()
	usage := collector.Snapshot()
	if usage.Upstream != nil || usage.Downstream == nil || *usage.Downstream.Response.ActualBytes != 8 {
		t.Fatal("upstream WS fallback must remain excluded without dropping downstream")
	}
}
