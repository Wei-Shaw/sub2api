package handler

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestFlushOpenAIDownstreamSurfacesRealNetworkFailure runs the production
// writer chain over real TCP: RawResponseWriterHandler (as mounted by
// ProvideHTTPServer) -> gin -> OpsErrorLoggerMiddleware capture writer ->
// ServerTiming writer. gin's Flush has no error, so only the raw net/http
// FlushError (or the request cancellation it triggers) can reveal the broken
// connection. A successful flush is never treated as client receipt.
func TestFlushOpenAIDownstreamSurfacesRealNetworkFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type outcome struct {
		flushErr  error
		ctxErr    error
		chunks    int
		sawTiming bool
	}
	done := make(chan outcome, 1)
	engine := gin.New()
	engine.Use(OpsErrorLoggerMiddleware(nil), middleware2.ServerTiming(true))
	engine.GET("/api/v1/admin/stream", func(c *gin.Context) {
		_, sawTiming := c.Writer.(interface{ Unwrap() http.ResponseWriter })
		chunk := strings.Repeat("x", 512) // below bufio size: Write succeeds, flush must report
		var res outcome
		res.sawTiming = sawTiming
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := c.Writer.WriteString(chunk); err != nil {
				res.flushErr = err
				break
			}
			res.chunks++
			if err := service.FlushOpenAIDownstream(c); err != nil {
				res.flushErr = err
				break
			}
			if err := c.Request.Context().Err(); err != nil {
				res.ctxErr = err
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		done <- res
	})
	server := httptest.NewServer(middleware2.RawResponseWriterHandler(engine))
	defer server.Close()

	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	require.NoError(t, err)
	_, err = conn.Write([]byte("GET /api/v1/admin/stream HTTP/1.1\r\nHost: test\r\n" + servertiming.AdminUIHeader + ": 1\r\n\r\n"))
	require.NoError(t, err)
	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, status, "200")
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetLinger(0) // RST like a timed-out proxy
	}
	require.NoError(t, conn.Close())

	select {
	case res := <-done:
		require.True(t, res.sawTiming, "server timing writer must be in the chain")
		require.True(t, res.flushErr != nil || res.ctxErr != nil, "broken downstream connection was never surfaced (chunks=%d)", res.chunks)
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not observe the closed client connection")
	}
}

func newDeliveryHandlerContext(t *testing.T, ctx context.Context) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil).WithContext(ctx)
	return c, rec
}

func TestHandleOpenAIDeliveryFailureUnknownConsumptionIsAuditedNotBilled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, rec := newDeliveryHandlerContext(t, ctx)
	submitted := 0
	err := &service.OpenAIDeliveryError{Outcome: service.ResponseOutcomeClientCancelled, Phase: service.OpenAIDeliveryPhaseBuffering, Cause: context.Canceled}

	handled := handleOpenAIDeliveryFailure(c, zap.NewNop(), &service.Account{ID: 1}, nil, err, func(*service.OpenAIForwardResult) { submitted++ })

	require.True(t, handled)
	require.Zero(t, submitted, "no fake zero-consumption usage row")
	require.Equal(t, statusClientClosedRequest, c.Writer.Status())
	require.Empty(t, rec.Body.String(), "no extra write to a disconnected client")
	require.Contains(t, c.GetString(service.OpsDeliveryAuditKey), "consumption=unknown")
}

func TestHandleOpenAIDeliveryFailureObservedConsumptionRecordedOnce(t *testing.T) {
	c, _ := newDeliveryHandlerContext(t, context.Background())
	c.Writer.WriteHeaderNow() // headers were already committed when the write failed
	result := &service.OpenAIForwardResult{Stream: true, Usage: service.OpenAIUsage{InputTokens: 50, OutputTokens: 4}}
	var submittedResults []*service.OpenAIForwardResult
	err := &service.OpenAIDeliveryError{Outcome: service.ResponseOutcomeWriteFailed, Phase: service.OpenAIDeliveryPhaseStreaming}

	handled := handleOpenAIDeliveryFailure(c, zap.NewNop(), &service.Account{ID: 1}, result, err, func(r *service.OpenAIForwardResult) {
		submittedResults = append(submittedResults, r)
	})

	require.True(t, handled)
	require.Len(t, submittedResults, 1)
	require.Equal(t, service.ResponseOutcomeWriteFailed, submittedResults[0].ResponseOutcome)
	streamErrs := service.GetOpsStreamErrors(c)
	require.Len(t, streamErrs, 1, "exactly one request-scoped error row for the committed response")
	require.Equal(t, statusClientClosedRequest, streamErrs[0].IntendedStatus)
	require.True(t, streamErrs[0].RequestScoped)
}

func TestHandleOpenAIDeliveryFailureIgnoresOtherErrors(t *testing.T) {
	c, _ := newDeliveryHandlerContext(t, context.Background())
	require.False(t, handleOpenAIDeliveryFailure(c, zap.NewNop(), &service.Account{ID: 1}, nil, errors.New("upstream boom"), func(*service.OpenAIForwardResult) {
		t.Fatal("must not submit")
	}))
	require.False(t, handleOpenAIDeliveryFailure(c, zap.NewNop(), &service.Account{ID: 1}, nil, &service.UpstreamFailoverError{StatusCode: 502}, func(*service.OpenAIForwardResult) {
		t.Fatal("must not submit")
	}))
}

func TestHandleOpenAIUpstreamFailedWithUsageStopsFailoverAndBillsOnce(t *testing.T) {
	c, rec := newDeliveryHandlerContext(t, context.Background())
	h := &OpenAIGatewayHandler{}
	result := &service.OpenAIForwardResult{Usage: service.OpenAIUsage{InputTokens: 120, OutputTokens: 7}}
	err := &service.OpenAIUpstreamFailedWithUsageError{Failover: &service.UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: []byte(`{"error":{"type":"upstream_error","message":"boom"}}`),
	}}
	submitted, reported := 0, 0

	handled := h.handleOpenAIUpstreamFailedWithUsage(c, zap.NewNop(), &service.Account{ID: 1}, result, err, false,
		func(error) { reported++ },
		func(r *service.OpenAIForwardResult) {
			submitted++
			require.Equal(t, service.ResponseOutcomeUpstreamFailed, r.ResponseOutcome)
		})

	require.True(t, handled)
	require.Equal(t, 1, submitted, "observed consumption recorded exactly once")
	require.LessOrEqual(t, reported, 1)
	require.NotEmpty(t, rec.Body.String(), "client receives the upstream error once")
}
