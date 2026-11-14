//go:build unit

package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestIsTransientUsageLogWriteError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"attempt deadline", fmt.Errorf("wrapped: %w", context.DeadlineExceeded), true},
		{"bad conn", driver.ErrBadConn, true},
		{"eof", io.ErrUnexpectedEOF, true},
		{"net", &net.OpError{Op: "read", Err: errors.New("i/o timeout")}, true},
		{"query canceled while waiting for lock", &pq.Error{Code: "57014"}, true},
		{"connection failure", &pq.Error{Code: "08006"}, true},
		{"deadlock", &pq.Error{Code: "40P01"}, true},
		{"admin shutdown", &pq.Error{Code: "57P01"}, true},
		{"too many connections", &pq.Error{Code: "53300"}, true},
		{"lock not available", &pq.Error{Code: "55P03"}, true},
		{"foreign key", &pq.Error{Code: "23503"}, false},
		{"not null", &pq.Error{Code: "23502"}, false},
		{"numeric out of range", &pq.Error{Code: "22003"}, false},
		{"undefined column", &pq.Error{Code: "42703"}, false},
		{"database dropped", &pq.Error{Code: "57P04"}, false},
		{"plain", errors.New("sql: database is closed"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isTransientUsageLogWriteError(tc.err))
		})
	}
}

func TestUsageLogWriteRetryDelayIsFullJitterUnderCappedExponential(t *testing.T) {
	p := usageLogWriteRetry{baseDelay: 50 * time.Millisecond, maxDelay: 20 * time.Second}
	for retry, ceiling := range map[int]time.Duration{
		0:  50 * time.Millisecond,
		3:  400 * time.Millisecond,
		8:  12800 * time.Millisecond,
		9:  20 * time.Second,
		40: 20 * time.Second,
	} {
		var maxSeen time.Duration
		for i := 0; i < 2000; i++ {
			d := p.delay(retry)
			require.GreaterOrEqual(t, d, time.Duration(0))
			require.LessOrEqual(t, d, ceiling)
			maxSeen = max(maxSeen, d)
		}
		require.Greater(t, maxSeen, ceiling/2, "retry=%d", retry)
	}
}
