package service

import (
	"testing"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/require"
)

const (
	testMiB = 1024 * 1024
	testGiB = 1024 * testMiB
)

// TestResolveMemoryStatsCgroupUsageButUnlimitedFallsBackToHost is the core
// regression: in Docker + cgroup v2 with NO memory limit, memory.current is a
// small container number while memory.max = "max" (so cgroupTotal == 0). The
// old code reported the container "used" against the host "total", producing a
// misleadingly tiny percentage (~0.3%). The fix must fall back ENTIRELY to host
// metrics instead of mixing the two sources.
func TestResolveMemoryStatsCgroupUsageButUnlimitedFallsBackToHost(t *testing.T) {
	const cgroupUsed = uint64(64573440) // memory.current, ~61 MiB
	host := &mem.VirtualMemoryStat{
		Used:        16 * testGiB,
		Total:       24 * testGiB,
		UsedPercent: 66.7,
	}

	usedMB, totalMB, pct := resolveMemoryStats(cgroupUsed, 0 /* memory.max = "max" */, true, host)

	require.NotNil(t, usedMB)
	require.NotNil(t, totalMB)
	require.NotNil(t, pct)

	// Everything must come from the host, not from the cgroup container value.
	require.Equal(t, int64(16*1024), *usedMB, "used must be host used, not container used")
	require.Equal(t, int64(24*1024), *totalMB, "total must be host total")
	require.InDelta(t, 66.7, *pct, 0.05, "percent must be host-derived, not container/host mix")

	// Guard against the specific bug: container used (~61 MiB) vs host total (~0.3%).
	require.NotEqual(t, int64(cgroupUsed/testMiB), *usedMB, "must not report the container used value")
	require.Greater(t, *pct, 1.0, "percent must not collapse to the ~0.3%% mixed value")
}

// TestResolveMemoryStatsExplicitContainerLimitUsesCgroup covers the case where
// a real container memory limit is set: memory.current = 512 MiB and
// memory.max = 2 GiB must yield ~25% entirely from cgroup, ignoring the host.
func TestResolveMemoryStatsExplicitContainerLimitUsesCgroup(t *testing.T) {
	host := &mem.VirtualMemoryStat{
		Used:        16 * testGiB, // deliberately different; must be ignored
		Total:       24 * testGiB,
		UsedPercent: 66.7,
	}

	usedMB, totalMB, pct := resolveMemoryStats(512*testMiB, 2*testGiB, true, host)

	require.NotNil(t, usedMB)
	require.NotNil(t, totalMB)
	require.NotNil(t, pct)

	require.Equal(t, int64(512), *usedMB)
	require.Equal(t, int64(2048), *totalMB)
	require.InDelta(t, 25.0, *pct, 0.05)
}

// TestResolveMemoryStatsNoCgroupUsesHost covers bare-metal / no-cgroup hosts:
// all three values come from the host reading.
func TestResolveMemoryStatsNoCgroupUsesHost(t *testing.T) {
	host := &mem.VirtualMemoryStat{
		Used:        16 * testGiB,
		Total:       24 * testGiB,
		UsedPercent: 66.7,
	}

	usedMB, totalMB, pct := resolveMemoryStats(0, 0, false, host)

	require.NotNil(t, usedMB)
	require.NotNil(t, totalMB)
	require.NotNil(t, pct)
	require.Equal(t, int64(16*1024), *usedMB)
	require.Equal(t, int64(24*1024), *totalMB)
	require.InDelta(t, 66.7, *pct, 0.05)
}

// TestResolveMemoryStatsNoDataReturnsNil: when neither cgroup nor host data is
// available, all outputs are nil (best-effort, nothing persisted).
func TestResolveMemoryStatsNoDataReturnsNil(t *testing.T) {
	usedMB, totalMB, pct := resolveMemoryStats(0, 0, false, nil)
	require.Nil(t, usedMB)
	require.Nil(t, totalMB)
	require.Nil(t, pct)
}

// TestResolveMemoryStatsHostWithoutTotalKeepsGopsutilPercent: degenerate host
// reading with no total still yields a used value and gopsutil's own percentage.
func TestResolveMemoryStatsHostWithoutTotalKeepsGopsutilPercent(t *testing.T) {
	host := &mem.VirtualMemoryStat{
		Used:        8 * testGiB,
		Total:       0,
		UsedPercent: 42.5,
	}

	usedMB, totalMB, pct := resolveMemoryStats(0, 0, false, host)

	require.NotNil(t, usedMB)
	require.Nil(t, totalMB)
	require.NotNil(t, pct)
	require.Equal(t, int64(8*1024), *usedMB)
	require.InDelta(t, 42.5, *pct, 0.05)
}
