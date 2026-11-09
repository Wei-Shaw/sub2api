package sysutil

import (
	"math"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/mem"
)

const cgroupRoot = "/sys/fs/cgroup"

// MemoryLimitBytes returns the memory available to this process: the smallest
// of physical memory, the cgroup (container/systemd) limit and GOMEMLIMIT.
// It returns 0 when none of them can be determined.
func MemoryLimitBytes() uint64 {
	var physical uint64
	if vm, err := mem.VirtualMemory(); err == nil {
		physical = vm.Total
	}
	goLimit := debug.SetMemoryLimit(-1)
	return minPositive(physical, cgroupMemoryLimit(cgroupRoot, "/proc/self/cgroup"), goMemoryLimit(goLimit))
}

// cgroupMemoryLimit returns the tightest memory limit on this process's cgroup
// path, or 0 when unlimited or unavailable. A parent's limit applies to its
// children (e.g. a systemd slice), so every level up to the mount root is
// checked, for both the v2 unified hierarchy and the v1 memory controller.
func cgroupMemoryLimit(root, selfCgroupFile string) uint64 {
	var limit uint64
	if raw, err := os.ReadFile(selfCgroupFile); err == nil {
		// Each line is "hierarchy-ID:controller-list:cgroup-path".
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.SplitN(strings.TrimSpace(line), ":", 3)
			if len(fields) != 3 {
				continue
			}
			switch {
			case fields[1] == "":
				limit = minPositive(limit, cgroupPathMemoryLimit(root, fields[2], "memory.max"))
			case slices.Contains(strings.Split(fields[1], ","), "memory"):
				limit = minPositive(limit, cgroupPathMemoryLimit(filepath.Join(root, "memory"), fields[2], "memory.limit_in_bytes"))
			}
		}
	}
	// Mount roots cover an unreadable /proc/self/cgroup.
	limit = minPositive(limit, cgroupPathMemoryLimit(root, "/", "memory.max"))
	return minPositive(limit, cgroupPathMemoryLimit(filepath.Join(root, "memory"), "/", "memory.limit_in_bytes"))
}

// cgroupPathMemoryLimit walks from the cgroup up to the mount root. Levels not
// visible in this mount (e.g. a host path inside a container) are skipped.
// Reads go through os.Root, so a cgroup path from /proc/self/cgroup can never
// resolve outside the mount.
func cgroupPathMemoryLimit(mount, cgroupPath, limitFile string) uint64 {
	mountRoot, err := os.OpenRoot(mount)
	if err != nil {
		return 0
	}
	defer func() { _ = mountRoot.Close() }()
	var limit uint64
	for dir := path.Clean("/" + cgroupPath); ; dir = path.Dir(dir) {
		name := filepath.Join(filepath.FromSlash(strings.TrimPrefix(dir, "/")), limitFile)
		limit = minPositive(limit, readCgroupLimitFile(mountRoot, name))
		if dir == "/" {
			return limit
		}
	}
}

func readCgroupLimitFile(mountRoot *os.Root, name string) uint64 {
	raw, err := mountRoot.ReadFile(name)
	if err != nil {
		return 0
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
	// v2 reports "max" when unlimited; v1 reports a page-rounded huge value.
	if err != nil || v >= 1<<60 {
		return 0
	}
	return v
}

func goMemoryLimit(limit int64) uint64 {
	if limit <= 0 || limit == math.MaxInt64 {
		return 0
	}
	return uint64(limit)
}

func minPositive(values ...uint64) uint64 {
	var out uint64
	for _, v := range values {
		if v > 0 && (out == 0 || v < out) {
			out = v
		}
	}
	return out
}
