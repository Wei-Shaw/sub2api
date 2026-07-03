package admin

import (
	"strconv"
	"strings"
	"time"
)

var accountPerformanceStatsBatchCache = newSnapshotCache(30 * time.Second)

func buildAccountPerformanceStatsBatchCacheKey(accountIDs []int64, windowHours int) string {
	if len(accountIDs) == 0 {
		return "accounts_performance_stats_empty"
	}
	var b strings.Builder
	b.Grow(len(accountIDs)*6 + 32)
	_, _ = b.WriteString("accounts_performance_stats:")
	_, _ = b.WriteString(strconv.Itoa(windowHours))
	_ = b.WriteByte(':')
	for i, id := range accountIDs {
		if i > 0 {
			_ = b.WriteByte(',')
		}
		_, _ = b.WriteString(strconv.FormatInt(id, 10))
	}
	return b.String()
}
