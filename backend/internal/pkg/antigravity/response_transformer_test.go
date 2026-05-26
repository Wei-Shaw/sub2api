//go:build unit

package antigravity

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Task 7: 验证 generateRandomID 和降级碰撞防护 ---

func TestGenerateRandomID_Uniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		id := generateRandomID()
		require.Len(t, id, 12, "ID 长度应为 12")
		_, dup := seen[id]
		require.False(t, dup, "第 %d 次调用生成了重复 ID: %s", i, id)
		seen[id] = struct{}{}
	}
}

func TestFallbackCounter_Increments(t *testing.T) {
	// 验证 fallbackCounter 的原子递增行为确保降级分支不会生成相同 seed
	before := atomic.LoadUint64(&fallbackCounter)
	cnt1 := atomic.AddUint64(&fallbackCounter, 1)
	cnt2 := atomic.AddUint64(&fallbackCounter, 1)
	require.Equal(t, before+1, cnt1, "第一次递增应为 before+1")
	require.Equal(t, before+2, cnt2, "第二次递增应为 before+2")
	require.NotEqual(t, cnt1, cnt2, "连续两次递增的计数器值应不同")
}

func TestFallbackCounter_ConcurrentIncrements(t *testing.T) {
	// 验证并发递增的原子性 — 每次递增都应产生唯一值
	const goroutines = 50
	results := make([]uint64, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = atomic.AddUint64(&fallbackCounter, 1)
		}(i)
	}
	wg.Wait()

	// 所有结果应唯一
	seen := make(map[uint64]bool, goroutines)
	for _, v := range results {
		assert.False(t, seen[v], "并发递增产生了重复值: %d", v)
		seen[v] = true
	}
}

func TestGenerateRandomID_Charset(t *testing.T) {
	const validChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	validSet := make(map[byte]struct{}, len(validChars))
	for i := 0; i < len(validChars); i++ {
		validSet[validChars[i]] = struct{}{}
	}

	for i := 0; i < 50; i++ {
		id := generateRandomID()
		for j := 0; j < len(id); j++ {
			_, ok := validSet[id[j]]
			require.True(t, ok, "ID 包含非法字符: %c (ID=%s)", id[j], id)
		}
	}
}

func TestGenerateRandomID_Length(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := generateRandomID()
		assert.Len(t, id, 12, "每次生成的 ID 长度应为 12")
	}
}

func TestGenerateRandomID_ConcurrentUniqueness(t *testing.T) {
	// 验证并发调用不会产生重复 ID
	const goroutines = 100
	results := make([]string, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = generateRandomID()
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, goroutines)
	for _, id := range results {
		assert.False(t, seen[id], "并发调用产生了重复 ID: %s", id)
		seen[id] = true
	}
}

func BenchmarkGenerateRandomID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateRandomID()
	}
}
