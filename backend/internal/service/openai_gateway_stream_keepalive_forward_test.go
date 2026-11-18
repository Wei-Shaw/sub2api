package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// keepaliveForwardBody holds the upstream stream open until the gateway closes it.
type keepaliveForwardBody struct {
	io.ReadCloser
	closed chan struct{}
	once   sync.Once
}

func (b *keepaliveForwardBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return b.ReadCloser.Close()
}

// stalledKeepaliveResponse returns an upstream response that sends the preamble and the given
// keepalive frames, then produces no semantic output until it is closed.
func stalledKeepaliveResponse(frames ...string) *http.Response {
	pr, pw := io.Pipe()
	body := &keepaliveForwardBody{ReadCloser: pr, closed: make(chan struct{})}
	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_stalled\"}}\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_stalled\"}}\n\n"))
		for _, frame := range frames {
			_, _ = pw.Write([]byte(frame))
		}
		select {
		case <-body.closed:
		case <-time.After(5 * time.Second):
		}
	}()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":                   []string{"text/event-stream"},
			"X-Request-Id":                   []string{"request-stalled"},
			"X-Ratelimit-Remaining-Requests": []string{"1"},
		},
		Body: body,
	}
}

func newKeepaliveForwardService(firstOutputTimeoutSeconds int) *OpenAIGatewayService {
	cfg := &config.Config{Gateway: config.GatewayConfig{
		OpenAIFirstOutputTimeoutSeconds: firstOutputTimeoutSeconds,
		MaxLineSize:                     defaultMaxLineSize,
	}}
	return &OpenAIGatewayService{cfg: cfg, responseHeaderFilter: compileResponseHeaderFilter(cfg)}
}

func newKeepaliveForwardContext() (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

const (
	upstreamKeepaliveData  = `data: {"type":"keepalive","sequence_number":3}`
	upstreamKeepaliveEvent = "event: keepalive\n" + upstreamKeepaliveData + "\n\n"
)

// Codex resets its stream idle timer only on parsed SSE events, so the upstream keepalive event
// must reach the client before the first semantic output, while the attempt's preamble and
// account headers stay private and the bytes still do not count as client output.
func TestOpenAINativeStreamForwardsUpstreamKeepaliveBeforeFirstOutput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame string
	}{
		{"event_and_data", upstreamKeepaliveEvent},
		{"data_only", upstreamKeepaliveData + "\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newKeepaliveForwardService(1)
			c, rec := newKeepaliveForwardContext()

			_, err := svc.handleStreamingResponse(c.Request.Context(), stalledKeepaliveResponse(tc.frame), c,
				&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model")

			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr, "no semantic output: the first-output timeout must fail over")
			require.Equal(t, tc.frame, rec.Body.String(), "only the upstream keepalive event reaches the client, verbatim")
			require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c), "keepalive bytes are not client output")
			require.Empty(t, rec.Result().Header.Values("X-Request-Id"), "account headers stay private before output")
			require.Empty(t, rec.Result().Header.Values("X-Ratelimit-Remaining-Requests"))
		})
	}
}

// After a pre-output failover the next account's stream continues the same response: the client
// sees the first account's keepalive, then the second account's complete stream from its start.
func TestOpenAINativeStreamKeepaliveThenFailoverContinuesWithNextAccount(t *testing.T) {
	svc := newKeepaliveForwardService(1)
	c, rec := newKeepaliveForwardContext()

	_, firstErr := svc.handleStreamingResponse(c.Request.Context(), stalledKeepaliveResponse(upstreamKeepaliveEvent), c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model")
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, firstErr, &failoverErr)

	secondResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"request-second"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_second"}}`,
			"",
			`data: {"type":"response.output_text.delta","delta":"hello"}`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_second","usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
		}, "\n"))),
	}
	result, secondErr := svc.handleStreamingResponse(c.Request.Context(), secondResp, c,
		&Account{ID: 2, Platform: PlatformOpenAI}, time.Now(), "model", "model")
	require.NoError(t, secondErr)
	require.NotNil(t, result)

	body := rec.Body.String()
	require.True(t, strings.HasPrefix(body, upstreamKeepaliveEvent), "first account's keepalive comes first, got %q", body)
	require.NotContains(t, body, "resp_stalled", "the failed attempt's preamble never reaches the client")
	created := strings.Index(body, `"id":"resp_second"`)
	require.Greater(t, created, len(upstreamKeepaliveEvent)-1)
	require.Less(t, created, strings.Index(body, `"delta":"hello"`))
	require.Empty(t, rec.Result().Header.Values("X-Request-Id"), "headers were committed by the keepalive, not the second account")
}

// Keepalive events after output started keep their place in the stream: not dropped, not repeated.
func TestOpenAINativeStreamKeepaliveAfterOutputStaysInOrder(t *testing.T) {
	svc := newKeepaliveForwardService(0)
	c, rec := newKeepaliveForwardContext()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_order"}}`,
			"",
			`data: {"type":"response.output_text.delta","delta":"a"}`,
			"",
			"event: keepalive",
			upstreamKeepaliveData,
			"",
			`data: {"type":"response.output_text.delta","delta":"b"}`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_order","usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model")
	require.NoError(t, err)

	body := rec.Body.String()
	require.Equal(t, 1, strings.Count(body, `"type":"keepalive"`))
	a, ka, b := strings.Index(body, `"delta":"a"`), strings.Index(body, upstreamKeepaliveEvent), strings.Index(body, `"delta":"b"`)
	require.True(t, a < ka && ka < b, "keepalive must stay between the deltas, got %q", body)
}

// Forwarded keepalive events are not output: they must not postpone the first-output timeout,
// so an attempt whose upstream only keeps the stream alive still fails over on time.
func TestOpenAINativeStreamKeepaliveDoesNotResetFirstOutputTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	body := &keepaliveForwardBody{ReadCloser: pr, closed: make(chan struct{})}
	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_paced\"}}\n\n"))
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.After(4 * time.Second)
		for {
			select {
			case <-body.closed:
				return
			case <-deadline:
				return
			case <-ticker.C:
				if _, err := pw.Write([]byte(upstreamKeepaliveEvent)); err != nil {
					return
				}
			}
		}
	}()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       body,
	}
	svc := newKeepaliveForwardService(1)
	c, rec := newKeepaliveForwardContext()

	started := time.Now()
	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, started, "model", "model")
	elapsed := time.Since(started)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Less(t, elapsed, 2500*time.Millisecond, "keepalive events must not reset the first-output timer")
	require.GreaterOrEqual(t, strings.Count(rec.Body.String(), upstreamKeepaliveEvent), 2, "keepalives reached the client before the timeout")
	require.NotContains(t, rec.Body.String(), "resp_paced")
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))
}
