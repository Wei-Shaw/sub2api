package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type antigravityBlockingErrUpstream struct {
	calls atomic.Int32
}

func (u *antigravityBlockingErrUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func (u *antigravityBlockingErrUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestAntigravityFirstByteGuardHeaderWaitTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	guardCtx, guard := newAntigravityFirstByteGuard(ctx, 30*time.Millisecond)
	req, err := http.NewRequestWithContext(guardCtx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	_, err = http.DefaultClient.Do(req)
	require.Error(t, err)
	require.True(t, guard.Fired())
	guard.abort()
}

func TestAntigravityFirstByteGuardStalledBodyAfterHeaders(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	guardCtx, guard := newAntigravityFirstByteGuard(ctx, 30*time.Millisecond)
	req, err := http.NewRequestWithContext(guardCtx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	body := guard.wrap(resp.Body)

	_, err = body.Read(make([]byte, 16))
	require.ErrorIs(t, err, errAntigravityFirstByteTimeout)
	require.True(t, guard.Fired())
}

func TestAntigravityFirstByteGuardBodyTimeout(t *testing.T) {
	reader, writer := io.Pipe()
	_, guard := newAntigravityFirstByteGuard(context.Background(), 30*time.Millisecond)
	body := guard.wrap(reader)

	_, err := body.Read(make([]byte, 16))
	require.ErrorIs(t, err, errAntigravityFirstByteTimeout)
	require.True(t, guard.Fired())

	_ = writer.Close()
	_ = body.Close()
}

func TestAntigravityFirstByteGuardDisarmsOnFirstByte(t *testing.T) {
	reader, writer := io.Pipe()
	_, guard := newAntigravityFirstByteGuard(context.Background(), 40*time.Millisecond)
	body := guard.wrap(reader)

	go func() {
		_, _ = io.WriteString(writer, "data")
		time.Sleep(100 * time.Millisecond)
		_ = writer.Close()
	}()

	buf := make([]byte, 8)
	n, err := body.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "data", string(buf[:n]))
	require.False(t, guard.Fired())

	n, err = body.Read(buf)
	require.Zero(t, n)
	require.ErrorIs(t, err, io.EOF)
	require.False(t, guard.Fired())
	_ = body.Close()
}

func TestAntigravityRetryLoopFirstByteHeaderTimeoutFailsOver(t *testing.T) {
	upstream := &antigravityBlockingErrUpstream{}
	svc := newAntigravityTestService(&config.Config{Gateway: config.GatewayConfig{
		AntigravityFirstByteTimeout: 1,
	}})
	svc.httpUpstream = upstream
	account := &Account{ID: 7, Name: "first-byte", Platform: PlatformAntigravity, Concurrency: 1}

	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "test",
		account:        account,
		accessToken:    "token",
		action:         "streamGenerateContent",
		body:           []byte(`{"model":"gemini-3.8-flash-high"}`),
		httpUpstream:   upstream,
		settingService: svc.settingService,
	})

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
	require.False(t, failoverErr.RetryableOnSameAccount)
	require.True(t, failoverErr.SafeToFailoverAfterWrite)
	require.Equal(t, int32(1), upstream.calls.Load())
}

func TestAntigravityRetryLoopClientCancelIsNotFirstByteTimeout(t *testing.T) {
	upstream := &antigravityBlockingErrUpstream{}
	svc := newAntigravityTestService(&config.Config{Gateway: config.GatewayConfig{
		AntigravityFirstByteTimeout: 1,
	}})
	svc.httpUpstream = upstream
	account := &Account{ID: 8, Name: "client-cancel", Platform: PlatformAntigravity, Concurrency: 1}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            ctx,
		prefix:         "test",
		account:        account,
		accessToken:    "token",
		action:         "streamGenerateContent",
		body:           []byte(`{"model":"gemini-3.8-flash-high"}`),
		httpUpstream:   upstream,
		settingService: svc.settingService,
	})

	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, int32(1), upstream.calls.Load())
}

func TestAntigravityGeminiStreamingFirstByteTimeoutFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityTestService(&config.Config{Gateway: config.GatewayConfig{
		MaxLineSize: defaultMaxLineSize,
	}})
	reader, writer := io.Pipe()
	_, guard := newAntigravityFirstByteGuard(context.Background(), 30*time.Millisecond)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       guard.wrap(reader),
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/antigravity/v1beta/models/gemini-3.8-flash-high:streamGenerateContent", nil)

	result, err := svc.handleGeminiStreamingResponse(c, resp, time.Now())

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
	require.False(t, failoverErr.RetryableOnSameAccount)
	require.True(t, failoverErr.SafeToFailoverAfterWrite)
	require.Empty(t, recorder.Body.String())
	_ = writer.Close()
}

func TestAntigravityCompatFirstByteTimeoutAfterKeepaliveFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	reader, writer := io.Pipe()
	_, guard := newAntigravityFirstByteGuard(context.Background(), 40*time.Millisecond)
	resp := &http.Response{StatusCode: http.StatusOK, Body: guard.wrap(reader)}

	result, err := svc.handleAntigravityCompatStreamWithKeepaliveInterval(
		c, resp, time.Now(), "gemini-3.1-pro",
		newAntigravityResponsesStreamAdapter("gemini-3.1-pro"), "test",
		10*time.Millisecond, time.Second,
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.False(t, failoverErr.RetryableOnSameAccount)
	require.True(t, failoverErr.SafeToFailoverAfterWrite)
	require.Contains(t, recorder.Body.String(), ": ping\n\n")
	require.NotContains(t, recorder.Body.String(), "event: error")
	_ = writer.Close()
}

func TestAntigravityFirstByteTimeoutErrorIsNotClientCancellation(t *testing.T) {
	reader, writer := io.Pipe()
	_, guard := newAntigravityFirstByteGuard(context.Background(), 20*time.Millisecond)
	body := guard.wrap(reader)

	_, err := body.Read(make([]byte, 8))
	require.ErrorIs(t, err, errAntigravityFirstByteTimeout)
	require.False(t, errors.Is(err, context.Canceled))
	_ = writer.Close()
	_ = body.Close()
}
