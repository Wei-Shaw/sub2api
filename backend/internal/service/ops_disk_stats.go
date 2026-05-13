package service

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

// diskStatsTimeout bounds the live disk-usage syscall so a slow filesystem
// (e.g. an unresponsive network mount) cannot stall the dashboard response.
const diskStatsTimeout = 2 * time.Second

// attachLiveDiskStats populates the Disk* fields on the given snapshot with
// the current usage of the filesystem holding the working directory.
// Errors are non-fatal — the fields are left nil and a warning is logged.
//
// Values are reported in MB (parity with MemoryUsedMB/MemoryTotalMB) so the
// UI can format them uniformly. UsagePercent is rounded to one decimal place.
func attachLiveDiskStats(ctx context.Context, snapshot *OpsSystemMetricsSnapshot) {
	if snapshot == nil {
		return
	}
	subCtx, cancel := context.WithTimeout(ctx, diskStatsTimeout)
	defer cancel()

	path, err := os.Getwd()
	if err != nil || path == "" {
		path = "."
	}

	usage, err := disk.UsageWithContext(subCtx, path)
	if err != nil || usage == nil {
		slog.Warn("ops disk stats: lookup failed", "path", path, "error", err)
		return
	}

	usedMB := int64(usage.Used / bytesPerMB)
	totalMB := int64(usage.Total / bytesPerMB)
	usagePct := roundTo1DP(usage.UsedPercent)

	snapshot.DiskPath = &path
	snapshot.DiskUsedMB = &usedMB
	snapshot.DiskTotalMB = &totalMB
	snapshot.DiskUsagePercent = &usagePct
}
