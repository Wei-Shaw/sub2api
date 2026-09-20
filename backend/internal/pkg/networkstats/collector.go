// Package networkstats reads explicitly mounted host interface counters.
// It never falls back to container interfaces, which would count database traffic.
package networkstats

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Sample struct {
	Timestamp   int64   `json:"timestamp"`
	UploadBPS   float64 `json:"upload_bps"`
	DownloadBPS float64 `json:"download_bps"`
}

type Snapshot struct {
	Available     bool     `json:"available"`
	Ready         bool     `json:"ready"`
	Interfaces    []string `json:"interfaces"`
	Timestamp     int64    `json:"timestamp"`
	UploadBPS     float64  `json:"upload_bps"`
	DownloadBPS   float64  `json:"download_bps"`
	TotalSent     uint64   `json:"total_sent"`
	TotalReceived uint64   `json:"total_received"`
	Samples       []Sample `json:"samples"`
}

type counters struct{ sent, received uint64 }

type Collector struct {
	mu       sync.Mutex
	root     string
	now      func() time.Time
	last     time.Time
	previous map[string]counters
	value    Snapshot
}

func New(root string) *Collector {
	if root == "" {
		root = "/host-network"
	}
	return &Collector{root: root, now: time.Now}
}

// Snapshot samples at most once every 1.5 seconds across all dashboard clients.
// A long polling gap or a counter reset starts a new baseline rather than
// presenting a long-term average or an unsigned wrap as instantaneous traffic.
func (c *Collector) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if !c.last.IsZero() && now.Sub(c.last) >= 0 && now.Sub(c.last) < 1500*time.Millisecond {
		return c.copyValue()
	}
	entries, err := os.ReadDir(c.root)
	current := make(map[string]counters)
	var sent, received uint64
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			rx, rxErr := readCounter(filepath.Join(c.root, entry.Name(), "rx_bytes"))
			tx, txErr := readCounter(filepath.Join(c.root, entry.Name(), "tx_bytes"))
			if rxErr != nil || txErr != nil {
				err = os.ErrInvalid
				break
			}
			current[entry.Name()] = counters{tx, rx}
			sent += tx
			received += rx
		}
	}
	if err != nil || len(current) == 0 {
		c.last = time.Time{}
		c.previous = nil
		c.value = Snapshot{Interfaces: []string{}, Samples: []Sample{}}
		return c.copyValue()
	}
	names := make([]string, 0, len(current))
	for name := range current {
		names = append(names, name)
	}
	sort.Strings(names)
	elapsed := now.Sub(c.last)
	ready := !c.last.IsZero() && elapsed >= 1500*time.Millisecond && elapsed <= 10*time.Second && len(current) == len(c.previous)
	var sentDelta, receivedDelta uint64
	for name, value := range current {
		old, ok := c.previous[name]
		if !ok || value.sent < old.sent || value.received < old.received {
			ready = false
			continue
		}
		sentDelta += value.sent - old.sent
		receivedDelta += value.received - old.received
	}
	history := c.value.Samples
	var up, down float64
	if ready {
		up = float64(sentDelta) / elapsed.Seconds()
		down = float64(receivedDelta) / elapsed.Seconds()
		history = append(history, Sample{Timestamp: now.UnixMilli(), UploadBPS: up, DownloadBPS: down})
		for len(history) > 0 && (history[0].Timestamp < now.Add(-5*time.Minute).UnixMilli() || len(history) > 180) {
			history = history[1:]
		}
	} else {
		history = []Sample{}
	}
	c.value = Snapshot{Available: true, Ready: ready, Interfaces: names, Timestamp: now.UnixMilli(), UploadBPS: up, DownloadBPS: down, TotalSent: sent, TotalReceived: received, Samples: history}
	c.last = now
	c.previous = current
	return c.copyValue()
}

func (c *Collector) copyValue() Snapshot {
	v := c.value
	v.Interfaces = append([]string{}, c.value.Interfaces...)
	v.Samples = append([]Sample{}, c.value.Samples...)
	return v
}

func readCounter(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
}
