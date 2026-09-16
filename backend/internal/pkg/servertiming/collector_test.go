package servertiming

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCollectorHeaderValueAggregatesIntervals(t *testing.T) {
	startedAt := time.Unix(100, 0)
	collector := New(startedAt)
	collector.Record(MetricDatabase, startedAt.Add(10*time.Millisecond), startedAt.Add(40*time.Millisecond), 2)
	collector.Record(MetricRedis, startedAt.Add(30*time.Millisecond), startedAt.Add(50*time.Millisecond), 3)
	collector.Record(dependencyMetricName("openai"), startedAt.Add(70*time.Millisecond), startedAt.Add(100*time.Millisecond), 1)
	collector.Record(dependencyMetricName("github"), startedAt.Add(60*time.Millisecond), startedAt.Add(90*time.Millisecond), 1)

	got := collector.HeaderValue(startedAt.Add(120*time.Millisecond), "miss")
	want := `total;dur=120.0, app;dur=40.0, db;dur=30.0;desc="queries=2", redis;dur=20.0;desc="commands=3", cache;desc="miss", deps;dur=40.0;desc="calls=2", dep_github;dur=30.0;desc="calls=1", dep_openai;dur=30.0;desc="calls=1"`
	if got != want {
		t.Fatalf("HeaderValue() = %q, want %q", got, want)
	}
}

func TestRecordIntervalDoesNotIncrementCount(t *testing.T) {
	startedAt := time.Unix(200, 0)
	collector := New(startedAt)
	ctx := WithCollector(context.Background(), collector)

	Record(ctx, MetricDatabase, startedAt.Add(10*time.Millisecond), startedAt.Add(20*time.Millisecond), 1)
	RecordInterval(ctx, MetricDatabase, startedAt.Add(30*time.Millisecond), startedAt.Add(40*time.Millisecond))

	header := HeaderValue(ctx, startedAt.Add(100*time.Millisecond), "hit")
	if !strings.Contains(header, `db;dur=20.0;desc="queries=1"`) {
		t.Fatalf("header %q does not contain one query with both blocking intervals", header)
	}
	if !strings.Contains(header, "app;dur=80.0") {
		t.Fatalf("header %q does not subtract the interval union from app time", header)
	}
}

func TestCollectorCacheStatusFallback(t *testing.T) {
	startedAt := time.Unix(300, 0)
	collector := New(startedAt)
	ctx := WithCollector(context.Background(), collector)

	SetCacheStatus(ctx, " HIT ")
	if got := HeaderValue(ctx, startedAt.Add(time.Millisecond), "invalid"); !strings.Contains(got, `cache;desc="hit"`) {
		t.Fatalf("HeaderValue() = %q, want stored cache hit", got)
	}

	other := New(startedAt)
	if got := other.HeaderValue(startedAt.Add(time.Millisecond), "invalid"); !strings.Contains(got, `cache;desc="bypass"`) {
		t.Fatalf("HeaderValue() = %q, want cache bypass", got)
	}
}

func TestCollectorSanitizesDependencyMetric(t *testing.T) {
	startedAt := time.Unix(400, 0)
	collector := New(startedAt)
	ctx := WithCollector(context.Background(), collector)
	RecordDependency(ctx, "GitHub API\r\nInjected;dur=999", startedAt, startedAt.Add(time.Millisecond))

	header := HeaderValue(ctx, startedAt.Add(2*time.Millisecond), "bypass")
	if strings.ContainsAny(header, "\r\n") || strings.Contains(header, ";dur=999") {
		t.Fatalf("unsafe metric content reached header: %q", header)
	}
	if !strings.Contains(header, "dep_githubapiinjecteddur999;dur=1.0") {
		t.Fatalf("sanitized dependency metric missing from header: %q", header)
	}
}

func TestCollectorBoundsHeaderLength(t *testing.T) {
	startedAt := time.Unix(500, 0)
	collector := New(startedAt)
	for i := 0; i < 300; i++ {
		collector.Record(
			dependencyMetricName(fmt.Sprintf("module_%03d_with_a_deliberately_long_name", i)),
			startedAt,
			startedAt.Add(time.Millisecond),
			1,
		)
	}

	header := collector.HeaderValue(startedAt.Add(2*time.Millisecond), "bypass")
	if len(header) > maxHeaderLength {
		t.Fatalf("header length = %d, want <= %d", len(header), maxHeaderLength)
	}
	if !strings.Contains(header, "total;dur=2.0") || !strings.Contains(header, "deps;dur=1.0") {
		t.Fatalf("bounded header lost fixed metrics: %q", header)
	}
}

func TestCollectorConcurrentRecording(t *testing.T) {
	startedAt := time.Now()
	collector := New(startedAt)
	ctx := WithCollector(context.Background(), collector)

	const workers = 25
	const recordsPerWorker = 100
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < recordsPerWorker; j++ {
				Record(ctx, MetricDatabase, startedAt, startedAt.Add(time.Microsecond), 1)
			}
		}()
	}
	wg.Wait()

	header := HeaderValue(ctx, startedAt.Add(time.Millisecond), "bypass")
	want := fmt.Sprintf(`queries=%d`, workers*recordsPerWorker)
	if !strings.Contains(header, want) {
		t.Fatalf("header %q does not contain %q", header, want)
	}
}

func TestContextHelpersHandleMissingCollector(t *testing.T) {
	if Active(context.Background()) {
		t.Fatal("context without collector reported active")
	}
	if got := HeaderValue(context.Background(), time.Now(), "hit"); got != "" {
		t.Fatalf("HeaderValue() = %q without collector, want empty", got)
	}
}
