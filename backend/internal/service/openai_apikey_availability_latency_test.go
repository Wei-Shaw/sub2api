package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const availabilityLatencyText = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"first real text\"}\n\n"
const availabilityLatencyComplete = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"latency-test\",\"status\":\"completed\",\"model\":\"gpt-test\",\"usage\":{\"input_tokens\":5,\"output_tokens\":2}}}\n\n"

type availabilityFirstTextRecorder struct {
	*httptest.ResponseRecorder
	started          time.Time
	firstText        time.Duration
	flushed          bool
	firstTextFlushed chan struct{}
}

func (w *availabilityFirstTextRecorder) Flush() {
	w.ResponseRecorder.Flush()
	if !w.flushed && bytes.Contains(w.Body.Bytes(), []byte("first real text")) {
		w.firstText = time.Since(w.started)
		w.flushed = true
		if w.firstTextFlushed != nil {
			close(w.firstTextFlushed)
		}
	}
}

func newAvailabilityLatencyService(enabled bool) (*OpenAIGatewayService, *Account) {
	cfg := &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize:                     defaultMaxLineSize,
		OpenAIAPIKeyAvailabilityEnabled: enabled,
	}}
	return &OpenAIGatewayService{cfg: cfg}, &Account{
		ID: 50701, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
	}
}

func runAvailabilityLatencyStream(ctx context.Context, svc *OpenAIGatewayService, account *Account, passthrough bool, body io.ReadCloser, flushed chan struct{}) (*availabilityFirstTextRecorder, error) {
	w := &availabilityFirstTextRecorder{ResponseRecorder: httptest.NewRecorder(), firstTextFlushed: flushed}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
	w.started = time.Now()
	var err error
	if passthrough {
		_, err = svc.handleStreamingResponsePassthrough(ctx, resp, c, account, w.started, "gpt-test", "")
	} else {
		_, err = svc.handleStreamingResponse(ctx, resp, c, account, w.started, "gpt-test", "gpt-test")
	}
	return w, err
}

// The producer waits for an actual text flush before sending the terminal event.
// Buffering the whole answer would deadlock until the test's cancellation guard.
func TestOpenAIAPIKeyAvailabilityFirstTextBeforeCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough=%t/enabled=%t", passthrough, enabled), func(t *testing.T) {
				svc, account := newAvailabilityLatencyService(enabled)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				r, p := io.Pipe()
				defer func() { _ = r.Close() }()
				flushed := make(chan struct{})
				producer := make(chan error, 1)
				go func() {
					defer func() { _ = p.Close() }()
					if _, err := io.WriteString(p, availabilityLatencyText); err != nil {
						producer <- err
						return
					}
					select {
					case <-flushed:
						_, err := io.WriteString(p, availabilityLatencyComplete)
						producer <- err
					case <-ctx.Done():
						_ = p.CloseWithError(ctx.Err())
						producer <- ctx.Err()
					}
				}()
				w, err := runAvailabilityLatencyStream(ctx, svc, account, passthrough, r, flushed)
				require.NoError(t, err)
				require.NoError(t, <-producer)
				require.True(t, w.flushed, "actual text must be flushed before the terminal event")
				require.Equal(t, 1, strings.Count(w.Body.String(), "response.completed"))
			})
		}
	}
}

// Measures parser-to-Flush latency with an immediately readable synthetic stream.
// These figures exclude network, model generation, scheduling and client latency.
func BenchmarkOpenAIAPIKeyAvailabilityFirstText(b *testing.B) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			b.Run(fmt.Sprintf("passthrough=%t/enabled=%t", passthrough, enabled), func(b *testing.B) {
				svc, account := newAvailabilityLatencyService(enabled)
				samples := make([]time.Duration, b.N)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					body := io.NopCloser(strings.NewReader(availabilityLatencyText + availabilityLatencyComplete))
					w, err := runAvailabilityLatencyStream(context.Background(), svc, account, passthrough, body, nil)
					if err != nil || !w.flushed {
						b.Fatalf("missing successful text flush: %v", err)
					}
					samples[i] = w.firstText
				}
				b.StopTimer()
				sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
				b.ReportMetric(float64(samples[(len(samples)-1)*50/100].Nanoseconds()), "first_text_p50_ns")
				b.ReportMetric(float64(samples[(len(samples)-1)*95/100].Nanoseconds()), "first_text_p95_ns")
			})
		}
	}
}

func BenchmarkOpenAIAPIKeyAvailabilityConcurrentStream(b *testing.B) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			b.Run(fmt.Sprintf("passthrough=%t/enabled=%t", passthrough, enabled), func(b *testing.B) {
				svc, account := newAvailabilityLatencyService(enabled)
				var elapsed atomic.Int64
				b.SetParallelism(4)
				b.ReportAllocs()
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						body := io.NopCloser(strings.NewReader(availabilityLatencyText + availabilityLatencyComplete))
						w, err := runAvailabilityLatencyStream(context.Background(), svc, account, passthrough, body, nil)
						if err != nil || !w.flushed {
							b.Errorf("missing successful text flush: %v", err)
							return
						}
						elapsed.Add(w.firstText.Nanoseconds())
					}
				})
				b.ReportMetric(float64(elapsed.Load())/float64(b.N), "first_text_mean_ns")
			})
		}
	}
}
