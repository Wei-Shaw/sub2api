//go:build unit

package service

import (
	"math"
	"testing"
)

// EnableOpenAIAdvancedSchedulerForTest gives a serial handler test the advanced
// scheduler without writing unrelated system settings into process-wide caches.
func EnableOpenAIAdvancedSchedulerForTest(t testing.TB) {
	t.Helper()
	previous, _ := openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting)
	t.Cleanup(func() { openAIAdvancedSchedulerSettingCache.Store(previous) })
	// Keep the fixture enabled until cleanup, independent of test duration.
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{enabled: true, expiresAt: math.MaxInt64})
}
