package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIngressRejectAccessSamplerConcurrentGlobalLimit(t *testing.T) {
	sampler := newIngressRejectAccessSampler(10, time.Hour, time.Minute)
	now := time.Now()
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := sampler.allow(now); ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(10), allowed.Load())
}

func TestLoggerIngressRejectSamplingIsBoundedAndSummarySkipsOpsSink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := globalIngressRejectAccessSampler
	globalIngressRejectAccessSampler = newIngressRejectAccessSampler(2, time.Hour, time.Hour)
	t.Cleanup(func() { globalIngressRejectAccessSampler = original })
	sink := initMiddlewareTestLogger(t)
	router := gin.New()
	router.Use(Logger())
	router.GET("/v1/messages", func(c *gin.Context) {
		MarkIngressRejected(c, IngressRejectInvalidAPIKey)
		c.Status(http.StatusUnauthorized)
	})
	for i := 0; i < 20; i++ {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/messages", nil))
	}
	var accessEvents, summaries int
	for _, event := range sink.list() {
		switch event.Message {
		case "http request completed":
			accessEvents++
		case "ingress rejection access logs dropped":
			summaries++
			if skipped, _ := event.Fields[logger.OpsSystemLogSkipField].(bool); !skipped {
				t.Fatalf("dropped summary must skip ops system log sink")
			}
		}
	}
	require.Equal(t, 2, accessEvents)
	require.Equal(t, 1, summaries)
}

func TestIngressRejectAccessSamplerDroppedSummaryIsLowFrequency(t *testing.T) {
	sampler := newIngressRejectAccessSampler(1, time.Hour, time.Second)
	now := time.Now()
	allowed, summary := sampler.allow(now)
	require.True(t, allowed)
	require.Zero(t, summary)

	allowed, summary = sampler.allow(now.Add(100 * time.Millisecond))
	require.False(t, allowed)
	require.Equal(t, uint64(1), summary)
	allowed, summary = sampler.allow(now.Add(200 * time.Millisecond))
	require.False(t, allowed)
	require.Zero(t, summary)
	allowed, summary = sampler.allow(now.Add(2 * time.Second))
	require.False(t, allowed)
	require.Equal(t, uint64(2), summary)
}
