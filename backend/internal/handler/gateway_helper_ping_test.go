package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyWaitPingInterval(t *testing.T) {
	for _, slotType := range []string{"user", "account"} {
		for _, format := range []SSEPingFormat{SSEPingFormatClaude, SSEPingFormatComment} {
			for _, interval := range []time.Duration{0, time.Millisecond} {
				t.Run(slotType+"/"+string(format)+"/"+interval.String(), func(t *testing.T) {
					cache := &helperConcurrencyCacheStub{}
					helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), format, interval)
					require.Equal(t, interval, helper.pingInterval)
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
					started := false
					_, err := helper.waitForSlotWithPingTimeout(c, slotType, 1, 1, 150*time.Millisecond, true, &started, false)
					var concurrencyErr *ConcurrencyError
					require.ErrorAs(t, err, &concurrencyErr)
					require.True(t, concurrencyErr.IsTimeout)
					if interval == 0 {
						require.False(t, started)
						require.False(t, c.Writer.Written())
						require.False(t, recorder.Flushed)
						require.Empty(t, recorder.Body.String())
						require.Empty(t, recorder.Header().Get("Content-Type"))
						c.JSON(http.StatusTooManyRequests, gin.H{"error": "busy"})
						require.Equal(t, http.StatusTooManyRequests, recorder.Code)
					} else {
						require.True(t, started)
						require.True(t, recorder.Flushed)
						require.Contains(t, recorder.Body.String(), string(format))
					}
				})
			}
		}
	}
}

func TestConcurrencyWaitWithoutPingAcquiresSlot(t *testing.T) {
	for _, slotType := range []string{"user", "account"} {
		t.Run(slotType, func(t *testing.T) {
			cache := &helperConcurrencyCacheStub{userSeq: []bool{true}, accountSeq: []bool{true}}
			helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatClaude, 0)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			started := false
			release, err := helper.waitForSlotWithPingTimeout(c, slotType, 1, 1, time.Second, true, &started, false)
			require.NoError(t, err)
			require.NotNil(t, release)
			release()
			require.False(t, started)
			require.False(t, c.Writer.Written())
			require.Empty(t, recorder.Body.String())
		})
	}
}
