package httputil

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONProcessingBudgetSharedHighWaterAndRelease(t *testing.T) {
	body := []byte(`"` + strings.Repeat("x", 126) + `"`) // 128 bytes, 768 processing bytes.
	budget := NewBodyMemoryBudget(1536)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
	require.NoError(t, ReserveRequestBodyMemory(ctx, 128))
	require.NoError(t, ReserveJSONProcessingMemory(ctx, body))
	for i := 0; i < 3; i++ {
		require.NoError(t, ReserveJSONProcessingMemory(ctx, body))
	}
	require.EqualValues(t, 896, budget.used)
	other, releaseOther := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer releaseOther()
	require.ErrorIs(t, ReserveJSONProcessingMemory(other, body), ErrRequestBodyMemoryBudget)
	require.EqualValues(t, 896, budget.used, "failed admission must not leak a partial charge")
	release()
	release()
	require.NoError(t, ReserveJSONProcessingMemory(other, body))
	require.ErrorIs(t, ReserveJSONProcessingMemory(ctx, body), ErrRequestBodyMemoryBudget)
	releaseOther()
	require.Zero(t, budget.used)
}

func TestJSONProcessingBudgetSettlesToOneBodyAfterUpstreamHandoff(t *testing.T) {
	body := []byte(`"` + strings.Repeat("x", 126) + `"`) // 128 bytes, 768 processing bytes.
	budget := NewBodyMemoryBudget(1200)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer release()
	require.NoError(t, ReserveRequestBodyMemory(ctx, 128))
	require.NoError(t, ReserveJSONProcessingMemory(ctx, body))
	require.EqualValues(t, 896, budget.Used())

	other, releaseOther := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer releaseOther()
	require.NoError(t, ReserveRequestBodyMemory(other, 128))
	require.ErrorIs(t, ReserveJSONProcessingMemory(other, body), ErrRequestBodyMemoryBudget,
		"a second transform cannot overlap the first one's peak")

	SettleJSONProcessingMemory(ctx)
	require.EqualValues(t, 128+128+128, budget.Used(), "streaming keeps ingress plus one outgoing copy")
	SettleJSONProcessingMemory(ctx)
	require.EqualValues(t, 384, budget.Used(), "settle is idempotent")
	require.NoError(t, ReserveJSONProcessingMemory(other, body), "the freed peak admits the next transform")
	require.EqualValues(t, 128+128+128+768, budget.Used())

	SettleJSONProcessingMemory(context.Background())
	release()
	releaseOther()
	require.Zero(t, budget.Used())
}

func TestJSONProcessingBudgetRejectsNeverFittingBodyAsTooLarge(t *testing.T) {
	body := []byte(`"` + strings.Repeat("x", 126) + `"`) // 128 bytes, 768 processing bytes.
	budget := NewBodyMemoryBudget(800)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer release()
	require.NoError(t, ReserveRequestBodyMemory(ctx, 128))
	err := ReserveJSONProcessingMemory(ctx, body)
	require.ErrorIs(t, err, ErrRequestBodyMemoryTooLarge, "128 ingress + 768 processing can never fit in 800")
	require.NotErrorIs(t, err, ErrRequestBodyMemoryBudget, "too large must not be retried as transient overload")
	require.EqualValues(t, 128, budget.Used())

	shared := NewBodyMemoryBudget(1024)
	busy, releaseBusy := WithRequestBodyMemoryBudget(context.Background(), shared)
	defer releaseBusy()
	require.NoError(t, ReserveRequestBodyMemory(busy, 512))
	next, releaseNext := WithRequestBodyMemoryBudget(context.Background(), shared)
	defer releaseNext()
	require.NoError(t, ReserveRequestBodyMemory(next, 128))
	require.ErrorIs(t, ReserveJSONProcessingMemory(next, body), ErrRequestBodyMemoryBudget,
		"a body that fits an idle instance is transient overload while others hold memory")
}

func BenchmarkJSONProcessingBudget(b *testing.B) {
	for _, size := range []int{1 << 10, 1 << 20, 8 << 20} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			body := []byte(`"` + strings.Repeat("A", size) + `"`)
			ctx, release := WithRequestBodyMemoryBudget(context.Background(), NewBodyMemoryBudget(256<<20))
			defer release()
			if err := ReserveJSONProcessingMemory(ctx, body); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := ReserveJSONProcessingMemory(ctx, body); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestJSONProcessingBudgetCountsStructureButNotQuotedPunctuation(t *testing.T) {
	budget := NewBodyMemoryBudget(4096)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer release()
	body := []byte(`"{[,:\\\"{}],"`)
	require.NoError(t, ReserveJSONProcessingMemory(ctx, body))
	require.EqualValues(t, len(body)*6, budget.used)
	structured := []byte(`{"a":[1,2]}`)
	child, releaseChild := WithRequestBodyMemoryScope(ctx)
	require.NoError(t, ReserveJSONProcessingMemory(child, structured))
	require.EqualValues(t, len(body)*6+len(structured)*6+4*128, budget.used)
	release() // Background work still owns its independent scope.
	require.EqualValues(t, len(structured)*6+4*128, budget.used)
	releaseChild()
	require.Zero(t, budget.used)
}

func TestJSONProcessingBudgetConcurrentAdmissionAndCancellation(t *testing.T) {
	budget := NewBodyMemoryBudget(1024)
	body := []byte(`"` + strings.Repeat("x", 126) + `"`)
	var accepted atomic.Int32
	var callers sync.WaitGroup
	var releasesMu sync.Mutex
	var releases []func()
	for i := 0; i < 12; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
			if err := ReserveJSONProcessingMemory(ctx, body); err == nil {
				accepted.Add(1)
				releasesMu.Lock()
				releases = append(releases, release)
				releasesMu.Unlock()
			} else {
				if !errors.Is(err, ErrRequestBodyMemoryBudget) {
					t.Error(err)
				}
				release()
			}
		}()
	}
	callers.Wait()
	require.EqualValues(t, 1, accepted.Load())
	for _, release := range releases {
		release()
	}
	require.Zero(t, budget.used)
	ctx, cancel := context.WithCancel(context.Background())
	ctx, release := WithRequestBodyMemoryBudget(ctx, budget)
	defer release()
	cancel()
	require.ErrorIs(t, ReserveJSONProcessingMemory(ctx, body), context.Canceled)
	require.Zero(t, budget.used)
	ctx, release = WithRequestBodyMemoryBudget(context.Background(), NewBodyMemoryBudget(0))
	defer release()
	require.NoError(t, ReserveJSONProcessingMemory(ctx, body), "explicit budget=0 keeps the opt-out")
}
