package securityaudit

import (
	"context"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func memoryTestPromptService(cfg ActiveConfig, repo JobRepository) *PromptService {
	store := &fakeConfigStore{cfg: cfg, active: true}
	metrics := NewAtomicMetrics()
	return &PromptService{config: store, metrics: metrics, background: context.Background(), enqueueSlots: make(chan struct{}, 128),
		enqueuer: NewEnqueuer(store, repo, &fakePayloadStore{}, metrics)}
}

func TestAsyncAuditOutOfScopeSkipsLargeBodyCopy(t *testing.T) {
	cfg := asyncConfig()
	cfg.AllGroups = false
	cfg.GroupIDs = []int64{1}
	groupID := int64(2)
	svc := memoryTestPromptService(cfg, &fakeJobRepository{})
	req := Request{GroupID: &groupID, Protocol: "openai_responses", Body: []byte(`{"input":"` + strings.Repeat("x", 1<<20) + `"}`)}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	decision := NewCoordinator(nil, svc).Check(context.Background(), req)
	svc.enqueueWG.Wait()
	runtime.ReadMemStats(&after)
	require.True(t, decision.AllowNextStage)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(len(req.Body)/2), "out-of-scope audit must not copy the body")
	runtime.KeepAlive(req)
}

func TestTrimRunesMatchesOriginalWithoutAllocatingTheWholePrompt(t *testing.T) {
	for _, value := range []string{"", "abc", "中文😀suffix", "a\xff\xfebc", strings.Repeat("x", 1<<20)} {
		for _, limit := range []int{0, 1, 3, 96} {
			want := ""
			if limit > 0 {
				runes := []rune(value)
				want = value
				if len(runes) > limit {
					want = string(runes[:limit]) + "…"
				}
			}
			require.Equal(t, want, TrimRunes(value, limit))
		}
	}
	value := strings.Repeat("x", 1<<20)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out := TrimRunes(value, 96)
	runtime.ReadMemStats(&after)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(64<<10))
	runtime.KeepAlive(out)
}

func TestSplitRunesPreservesChunksWithoutWholeTranscriptCopies(t *testing.T) {
	for _, value := range []string{"", "abcdef", "中文😀suffix", "a\xff\xfebc", "abc" + promptAuditPrioritySeparator + "中文😀"} {
		for _, limit := range []int{1, 3, 4000} {
			want := []string{}
			for _, segment := range strings.Split(value, promptAuditPrioritySeparator) {
				runes := []rune(segment)
				for start := 0; start < len(runes); start += limit {
					want = append(want, string(runes[start:min(start+limit, len(runes))]))
				}
			}
			require.Equal(t, want, SplitRunes(value, limit))
		}
	}
	value := strings.Repeat("x", 1<<20)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	chunks := SplitRunes(value, 4000)
	runtime.ReadMemStats(&after)
	require.Equal(t, value, strings.Join(chunks, ""))
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(64<<10))
	runtime.KeepAlive(chunks)
}

func TestRedactPreviewPreservesCompleteInputReplacementOrder(t *testing.T) {
	for _, value := range []string{
		"Bearer abcdefghijklmnop token=abcdefghijkl USER_CANARY_example user@example.org +86 138 1234 5678",
		strings.Repeat("x", 1<<20),
		strings.Repeat("前缀😀", 40) + " Bearer abcdefghijklmnop",
		"short password:abcdefghijkl suffix",
	} {
		want := bearerPattern.ReplaceAllString(value, "Bearer ***")
		want = apiKeyPattern.ReplaceAllStringFunc(want, func(match string) string {
			if index := strings.IndexAny(match, ":= \t"); index >= 0 {
				return match[:index+1] + "***"
			}
			return "***"
		})
		want = canaryPattern.ReplaceAllString(want, "${1}***")
		want = emailPattern.ReplaceAllString(want, "***@***")
		want = phonePattern.ReplaceAllString(want, "***PHONE***")
		require.Equal(t, TrimRunes(want, 96), RedactPreview(value, 96))
	}
	value := strings.Repeat("x", 1<<20)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out := RedactPreview(value, 96)
	runtime.ReadMemStats(&after)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(64<<10))
	runtime.KeepAlive(out)
}

type memoryTestGatedConfig struct {
	*fakeConfigStore
	reads  atomic.Int32
	resume <-chan struct{}
}

func (c *memoryTestGatedConfig) Active() (ActiveConfig, bool) {
	if c.reads.Add(1) >= 2 {
		<-c.resume
	}
	return c.fakeConfigStore.Active()
}

type memoryTestLegacyMutation struct{ mutate func(Request) }

func (l memoryTestLegacyMutation) Check(_ context.Context, req Request) (*LegacyDecision, error) {
	l.mutate(req)
	return &LegacyDecision{Allowed: true}, nil
}

func TestAsyncAuditOwnsCopyBeforeLegacyOrHandlerCanMutateInput(t *testing.T) {
	cfg := asyncConfig()
	cfg.AllGroups = false
	cfg.GroupIDs = []int64{1}
	resume := make(chan struct{})
	store := &memoryTestGatedConfig{fakeConfigStore: &fakeConfigStore{cfg: cfg, active: true}, resume: resume}
	repo := &fakeJobRepository{}
	svc := memoryTestPromptService(cfg, repo)
	svc.config, svc.enqueuer.config = store, store
	groupID := int64(1)
	req := asyncRequest()
	req.GroupID = &groupID
	want, err := ExtractPromptSnapshot(req)
	require.NoError(t, err)
	legacy := memoryTestLegacyMutation{mutate: func(borrowed Request) {
		borrowed.Body[0] = '!'
		*borrowed.GroupID = 2
		close(resume)
	}}
	decision := NewCoordinator(legacy, svc).Check(context.Background(), req)
	svc.enqueueWG.Wait()
	require.True(t, decision.AllowNextStage)
	require.Equal(t, AuditMetricsSnapshot{Enqueued: 1}, svc.metrics.AuditSnapshot())
	require.Equal(t, want.PromptHash, repo.createdSnapshot.PromptHash)
	require.Equal(t, int64(1), *repo.createdSnapshot.GroupID)
}

func TestAsyncAuditConcurrentRequestsStillEnqueueWithoutNewDrops(t *testing.T) {
	svc := memoryTestPromptService(asyncConfig(), &fakeJobRepository{})
	coordinator := NewCoordinator(nil, svc)
	req := asyncRequest()
	var callers sync.WaitGroup
	for i := 0; i < 8; i++ {
		callers.Add(1)
		go func() { defer callers.Done(); coordinator.Check(context.Background(), req) }()
	}
	callers.Wait()
	svc.enqueueWG.Wait()
	require.Equal(t, AuditMetricsSnapshot{Enqueued: 8}, svc.metrics.AuditSnapshot())
}

func originalTrimRunesForMemoryTest(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func originalRedactForMemoryTest(value string, limit int) string {
	value = bearerPattern.ReplaceAllString(value, "Bearer ***")
	value = apiKeyPattern.ReplaceAllStringFunc(value, func(match string) string {
		if index := strings.IndexAny(match, ":= \t"); index >= 0 {
			return match[:index+1] + "***"
		}
		return "***"
	})
	value = canaryPattern.ReplaceAllString(value, "${1}***")
	value = emailPattern.ReplaceAllString(value, "***@***")
	value = phonePattern.ReplaceAllString(value, "***PHONE***")
	return originalTrimRunesForMemoryTest(value, limit)
}

func TestAuditTextOptimizationsMatchOriginalOnDeterministicCorpus(t *testing.T) {
	rng := rand.New(rand.NewSource(20261111))
	atoms := []string{"x", "中文", "😀", " ", "\xff\xfe", "\x00", "Bearer abcdefghijklmnop", " secret:abcdefghijkl ", " user@example.org ", " +86 138 1234 5678 ", promptAuditPrioritySeparator}
	for sample := 0; sample < 100; sample++ {
		var builder strings.Builder
		for n := rng.Intn(128); n > 0; n-- {
			_, _ = builder.WriteString(atoms[rng.Intn(len(atoms))])
		}
		value := builder.String()
		for _, limit := range []int{0, 1, 24, 96, 4000} {
			require.Equal(t, originalTrimRunesForMemoryTest(value, limit), TrimRunes(value, limit))
			require.Equal(t, originalRedactForMemoryTest(value, limit), RedactPreview(value, limit))
			var want []string
			if limit > 0 {
				want = []string{}
				for _, segment := range strings.Split(value, promptAuditPrioritySeparator) {
					runes := []rune(segment)
					for start := 0; start < len(runes); start += limit {
						want = append(want, string(runes[start:min(start+limit, len(runes))]))
					}
				}
			}
			require.Equal(t, want, SplitRunes(value, limit))
		}
	}
}
