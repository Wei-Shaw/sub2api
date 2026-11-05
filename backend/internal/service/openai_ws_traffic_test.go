package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSAttemptExcludesUpstreamAcrossHTTPFallback(t *testing.T) {
	for _, collectorInCallContext := range []bool{false, true} {
		collector := traffic.NewCollector()
		requestContext := traffic.WithCollector(context.Background(), collector)
		ctx := context.Background()
		if collectorInCallContext {
			ctx = requestContext
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(requestContext)
		downstream := collector.Begin(traffic.DownstreamResponse, false)
		downstream.AddActual(7)
		downstream.AddOriginal(7)
		downstream.CompleteActual()
		downstream.CompleteOriginal()
		var service *OpenAIGatewayService
		_, err := service.forwardOpenAIWSV2(ctx, c, nil, nil, "", "", "", OpenAIWSProtocolDecision{}, false, false, "", "", time.Now(), 0, "", nil)
		require.Error(t, err) // invalid-state fallback still crossed the WS boundary
		// A successful later HTTP attempt cannot restore a partial upstream sum.
		for _, direction := range []traffic.Direction{traffic.UpstreamRequest, traffic.UpstreamResponse} {
			body := collector.Begin(direction, false)
			body.AddActual(13)
			body.AddOriginal(13)
			body.CompleteActual()
			body.CompleteOriginal()
		}
		collector.Finalize()
		usage := collector.Snapshot()
		require.Nil(t, usage.Upstream)
		require.EqualValues(t, 7, *usage.Downstream.Response.ActualBytes)
	}
}
