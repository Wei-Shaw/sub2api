//go:build unit

package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestQueuePingFormat(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
		t.Run(path, func(t *testing.T) {
			c, _ := newHelperTestContext(http.MethodPost, path)
			want := SSEPingFormatComment
			if path == "/v1/messages" {
				want = SSEPingFormatClaude
			}
			require.Equal(t, want, queuePingFormat(c, SSEPingFormatClaude))
			require.Equal(t, SSEPingFormatNone, queuePingFormat(c, SSEPingFormatNone))
			require.Equal(t, SSEPingFormatComment, queuePingFormat(c, SSEPingFormatComment))
		})
	}
	require.Equal(t, SSEPingFormatClaude, queuePingFormat(nil, SSEPingFormatClaude))
}

func TestSharedGatewayQueueHeartbeatMatchesDownstreamProtocol(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
		for _, slot := range []string{"user", "account"} {
			t.Run(path+"/"+slot, func(t *testing.T) {
				cache := &helperConcurrencyCacheStub{userSeq: []bool{false}, accountSeq: []bool{false}}
				helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatClaude, time.Millisecond)
				c, rec := newHelperTestContext(http.MethodPost, path)
				started := false
				release, err := helper.waitForSlotWithPingTimeout(c, slot, 101, 1, 30*time.Millisecond, true, &started, true)
				require.Nil(t, release)
				var concurrencyErr *ConcurrencyError
				require.ErrorAs(t, err, &concurrencyErr)
				require.True(t, started)
				require.True(t, rec.Flushed)
				if path == "/v1/messages" {
					require.Contains(t, rec.Body.String(), string(SSEPingFormatClaude))
				} else {
					require.Contains(t, rec.Body.String(), string(SSEPingFormatComment))
					require.NotContains(t, rec.Body.String(), "data:")
					require.NotContains(t, rec.Body.String(), `"type"`)
				}
			})
		}
	}
}
