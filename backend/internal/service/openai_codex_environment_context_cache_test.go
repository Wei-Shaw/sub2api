//go:build unit

package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func resetCodexLocationCacheForTest(t *testing.T) {
	t.Helper()
	reset := func() {
		codexEnvironmentLocationCache.Range(func(key, _ any) bool {
			codexEnvironmentLocationCache.Delete(key)
			return true
		})
		codexEnvironmentLocationCacheSize.Store(0)
	}
	reset()
	t.Cleanup(reset)
}

func codexLocationCacheLen() int {
	n := 0
	codexEnvironmentLocationCache.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

func TestCodexLocationCacheDoesNotStoreInvalidNames(t *testing.T) {
	resetCodexLocationCacheForTest(t)

	for i := 0; i < 100000; i++ {
		require.Nil(t, loadCodexLocationCached(fmt.Sprintf("Garbage/Zone_%d", i)))
	}
	require.Nil(t, loadCodexLocationCached(strings.Repeat("A", codexTimezoneNameMaxLen+1)))
	require.Zero(t, codexLocationCacheLen(), "invalid client timezones must not be cached")

	require.NotNil(t, loadCodexLocationCached("Asia/Shanghai"))
	require.NotNil(t, loadCodexLocationCached(" Asia/Shanghai "))
	require.Equal(t, 1, codexLocationCacheLen())
}

func TestCodexLocationCacheIsBounded(t *testing.T) {
	resetCodexLocationCacheForTest(t)
	codexEnvironmentLocationCacheSize.Store(codexEnvironmentLocationCacheMax)

	loc := loadCodexLocationCached("Asia/Tokyo")
	require.NotNil(t, loc, "valid names still resolve once the cache is full")
	require.Equal(t, "Asia/Tokyo", loc.String())
	require.Zero(t, codexLocationCacheLen(), "a full cache must not grow")
	require.Equal(t, int64(codexEnvironmentLocationCacheMax), codexEnvironmentLocationCacheSize.Load())
}
