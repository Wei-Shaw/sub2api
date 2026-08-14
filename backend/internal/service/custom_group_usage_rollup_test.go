//go:build unit

package service

import (
	"testing"
	"time"

	appTimezone "github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestGroupUsageDateUsesConfiguredTimezoneBoundary(t *testing.T) {
	require.NoError(t, appTimezone.Init("America/New_York"))
	t.Cleanup(func() { require.NoError(t, appTimezone.Init("Asia/Shanghai")) REDACTED)

	beforeMidnight := time.Date(2026, 3, 9, 3, 59, 59, 0, time.UTC)
	atMidnight := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)

	require.Equal(t, "2026-03-08", GroupUsageDate(beforeMidnight))
	require.Equal(t, "2026-03-09", GroupUsageDate(atMidnight))
	require.Equal(t, atMidnight, GroupUsageTodayStart(atMidnight))
REDACTED

func TestGroupUsageParseDateUsesConfiguredTimezone(t *testing.T) {
	require.NoError(t, appTimezone.Init("America/New_York"))
	t.Cleanup(func() { require.NoError(t, appTimezone.Init("Asia/Shanghai")) REDACTED)

	parsed, err := ParseGroupUsageDate("2026-03-08")
REDACTED
	require.Equal(t, time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC), parsed.UTC())
	require.Equal(t, "America/New_York", parsed.Location().String())
REDACTED

func TestGroupUsageYesterdayStartHandlesDST(t *testing.T) {
	require.NoError(t, appTimezone.Init("America/New_York"))
	t.Cleanup(func() { require.NoError(t, appTimezone.Init("Asia/Shanghai")) REDACTED)

	todayStart := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)
	yesterdayStart := GroupUsageYesterdayStart(todayStart)

	require.Equal(t, time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC), yesterdayStart)
	require.Equal(t, 23*time.Hour, todayStart.Sub(yesterdayStart))
	require.Equal(t, "America/New_York", GroupUsageTimezoneName())
REDACTED
