//go:build unit

package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newInvalidAuthLimiterForTest(threshold, capacity int) *invalidAuthAbuseLimiter {
	cfg := &config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{
		InvalidAbuse: config.InvalidAuthAbuseConfig{
			Enabled: true, Threshold: threshold, WindowSeconds: 60, BlockSeconds: 10, Capacity: capacity,
	REDACTED,
REDACTEDREDACTED
	return newInvalidAuthAbuseLimiter(cfg)
REDACTED

func TestInvalidAuthAbuseLimiterBlocksAndExpires(t *testing.T) {
	l := newInvalidAuthLimiterForTest(3, 16)
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now REDACTED

	for range 3 {
		l.record("203.0.113.1")
REDACTED
	retry, blocked := l.check("203.0.113.1")
	require.True(t, blocked)
	require.Equal(t, 10*time.Second, retry)

	now = now.Add(11 * time.Second)
	_, blocked = l.check("203.0.113.1")
	require.False(t, blocked)
	now = now.Add(61 * time.Second)
	_, blocked = l.check("203.0.113.1")
	require.False(t, blocked)
	require.Zero(t, l.health().Tracked)
REDACTED

func TestInvalidAuthAbuseLimiterCapacityUsesBoundedOverflowProtection(t *testing.T) {
	l := newInvalidAuthLimiterForTest(2, 2)
	now := time.Now()
	l.now = func() time.Time { return now REDACTED
	l.record("198.51.100.1")
	l.record("198.51.100.2")
	l.record("198.51.100.3")
	l.record("198.51.100.4")

	_, blocked := l.check("198.51.100.5")
	require.True(t, blocked)
	_, trackedBlocked := l.check("198.51.100.1")
	require.False(t, trackedBlocked, "global overflow protection should spare existing tracked NATs")
	health := l.health()
	require.Equal(t, int64(2), health.Tracked)
	require.Equal(t, uint64(2), health.Overflowed)
	require.Equal(t, uint64(1), health.GlobalBlocked)
REDACTED

func TestInvalidAuthAbuseLimiterConcurrentCapacityIsBounded(t *testing.T) {
	const capacity = 64
	l := newInvalidAuthLimiterForTest(1000, capacity)
	var wg sync.WaitGroup
	for i := range 1000 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l.record(fmt.Sprintf("198.51.100.%d", i))
	REDACTED(i)
REDACTED
	wg.Wait()
	health := l.health()
	require.LessOrEqual(t, health.Tracked, int64(capacity))
	require.Equal(t, uint64(1000), health.Recorded)
	require.Equal(t, uint64(1000-capacity), health.Overflowed)
REDACTED

func TestInvalidAuthAbuseLimiterReclaimsExpiredCapacity(t *testing.T) {
	const capacity = 16
	l := newInvalidAuthLimiterForTest(100, capacity)
	now := time.Now()
	l.now = func() time.Time { return now REDACTED
	for i := range capacity {
		l.record(fmt.Sprintf("source-%d", i))
REDACTED
	require.Equal(t, int64(capacity), l.health().Tracked)

	now = now.Add(61 * time.Second)
	for i := range invalidAuthAbuseShardCount {
		l.check(fmt.Sprintf("new-source-%d", i))
		now = now.Add(101 * time.Millisecond)
REDACTED
	require.Less(t, l.health().Tracked, int64(capacity))
	l.record("fresh-source")
	require.LessOrEqual(t, l.health().Tracked, int64(capacity))
REDACTED
