package networkstats

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Collector, *time.Time, func(uint64, uint64)) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "eth0"), 0700); err != nil {
		t.Fatal(err)
	}
	write := func(rx, tx uint64) {
		for name, v := range map[string]uint64{"rx_bytes": rx, "tx_bytes": tx} {
			if err := os.WriteFile(filepath.Join(root, "eth0", name), []byte(strconv.FormatUint(v, 10)), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	now := time.Unix(100000, 0)
	c := New(root)
	c.now = func() time.Time { return now }
	write(1000, 2000)
	return c, &now, write
}

func TestRatesAndSharedSampling(t *testing.T) {
	c, now, write := fixture(t)
	first := c.Snapshot()
	if !first.Available || first.Ready {
		t.Fatal("initial snapshot must be collecting")
	}
	*now = now.Add(2 * time.Second)
	write(5000, 8000)
	s := c.Snapshot()
	if !s.Ready || s.DownloadBPS != 2000 || s.UploadBPS != 3000 || s.TotalSent != 8000 {
		t.Fatalf("incorrect rates: %+v", s)
	}
	s.Samples[0].UploadBPS = 99
	if next := c.Snapshot(); len(next.Samples) != 1 || next.Samples[0].UploadBPS != 3000 {
		t.Fatal("snapshot mutation or extra polling changed history")
	}
}

func TestResetAndPollingGapNeverSpike(t *testing.T) {
	c, now, write := fixture(t)
	c.Snapshot()
	*now = now.Add(2 * time.Second)
	write(1, 2)
	if s := c.Snapshot(); s.Ready || s.UploadBPS != 0 {
		t.Fatal("reset produced a spike")
	}
	*now = now.Add(2 * time.Second)
	write(21, 42)
	if s := c.Snapshot(); !s.Ready || s.UploadBPS != 20 {
		t.Fatal("sampling did not recover")
	}
	*now = now.Add(time.Minute)
	write(10000000, 20000000)
	if s := c.Snapshot(); s.Ready || len(s.Samples) != 0 {
		t.Fatal("stale average reported as live traffic")
	}
}

func TestMissingOrCorruptHostMetrics(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "missing"))
	if c.Snapshot().Available {
		t.Fatal("container fallback must not be used")
	}
	c, now, _ := fixture(t)
	c.Snapshot()
	*now = now.Add(2 * time.Second)
	if err := os.WriteFile(filepath.Join(c.root, "eth0", "rx_bytes"), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().Available {
		t.Fatal("invalid counters reported as available")
	}
}

func TestInterfaceChangeAndHistoryBound(t *testing.T) {
	c, now, write := fixture(t)
	c.Snapshot()
	for i := 1; i <= 200; i++ {
		*now = now.Add(2 * time.Second)
		write(uint64(1000+i*10), uint64(2000+i*20))
		c.Snapshot()
	}
	if n := len(c.Snapshot().Samples); n > 151 || n < 150 {
		t.Fatalf("history length %d", n)
	}
	if err := os.Rename(filepath.Join(c.root, "eth0"), filepath.Join(c.root, "eth1")); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(2 * time.Second)
	if s := c.Snapshot(); s.Ready || len(s.Samples) != 0 {
		t.Fatal("interface replacement must reset baseline")
	}
}
