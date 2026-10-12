package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const availabilityIOPreamble = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n"
const availabilityIOText = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"real text\"}\n\n"
const availabilityIOComplete = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\"}}\n\n"

type availabilityTimeoutSettingsSpy struct {
	SettingRepository
	value string
	calls atomic.Int64
}

func (s *availabilityTimeoutSettingsSpy) GetValue(_ context.Context, key string) (string, error) {
	if key != SettingKeyStreamTimeoutSettings {
		return "", ErrSettingNotFound
	}
	s.calls.Add(1)
	return s.value, nil
}

type availabilityTimeoutCounterSpy struct {
	TimeoutCounterCache
	increments atomic.Int64
	resets     atomic.Int64
}

func (s *availabilityTimeoutCounterSpy) IncrementTimeoutCount(context.Context, int64, int) (int64, error) {
	return s.increments.Add(1), nil
}

func (s *availabilityTimeoutCounterSpy) ResetTimeoutCount(context.Context, int64) error {
	s.resets.Add(1)
	return nil
}

func installAvailabilityTimeoutPolicy(t *testing.T, svc *OpenAIGatewayService, threshold int) (*availabilityTimeoutSettingsSpy, *availabilityTimeoutCounterSpy) {
	t.Helper()
	value, err := json.Marshal(StreamTimeoutSettings{
		Enabled: true, Action: StreamTimeoutActionError,
		ThresholdCount: threshold, ThresholdWindowMinutes: 5, TempUnschedMinutes: 1,
	})
	require.NoError(t, err)
	settings := &availabilityTimeoutSettingsSpy{value: string(value)}
	counter := &availabilityTimeoutCounterSpy{}
	svc.rateLimitService.SetSettingService(&SettingService{settingRepo: settings})
	svc.rateLimitService.SetTimeoutCounterCache(counter)
	return settings, counter
}

func runAvailabilityIOStream(t *testing.T, svc *OpenAIGatewayService, account *Account, ctx context.Context, passthrough bool, body io.ReadCloser) (*httptest.ResponseRecorder, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
	var err error
	if passthrough {
		_, err = svc.handleStreamingResponsePassthrough(ctx, resp, c, account, time.Now(), "alias", "")
	} else {
		_, err = svc.handleStreamingResponse(ctx, resp, c, account, time.Now(), "alias", "upstream")
	}
	return w, err
}

func TestOpenAIAvailabilityIOFailures(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, cause := range []error{io.EOF, io.ErrUnexpectedEOF, context.DeadlineExceeded, syscall.ECONNRESET} {
			for _, stage := range []string{"preamble", "text", "completed"} {
				name := map[bool]string{false: "native/", true: "passthrough/"}[passthrough] + stage + "/" + cause.Error()
				t.Run(name, func(t *testing.T) {
					svc, account := newAPIKeyAvailabilityTestService(true)
					account.Credentials = map[string]any{"model_mapping": map[string]any{"alias": "upstream"}}
					payload := availabilityIOPreamble
					if stage != "preamble" {
						payload += availabilityIOText
					}
					if stage == "completed" {
						payload += availabilityIOComplete
					}
					body := &openAIStreamReadThenErrorCloser{reader: strings.NewReader(payload), err: cause}
					w, err := runAvailabilityIOStream(t, svc, account, context.Background(), passthrough, body)
					if stage == "completed" {
						require.NoError(t, err)
						require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "alias"))
						return
					}
					require.Error(t, err)
					var failover *UpstreamFailoverError
					require.Equal(t, stage == "preamble", errors.As(err, &failover))
					if stage == "preamble" {
						require.Empty(t, w.Body.String())
						require.False(t, failover.RetryableOnSameAccount)
					} else {
						require.Equal(t, 1, strings.Count(w.Body.String(), "real text"))
					}
					require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "upstream").failureStreak)
					require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "alias"))
					require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "other"))
				})
			}
		}
	}
}

func TestOpenAIAvailabilityIOCancellationAndLimits(t *testing.T) {
	for _, cause := range []error{context.Canceled, io.ErrUnexpectedEOF, context.DeadlineExceeded, ErrUpstreamResponseBodyTooLarge} {
		for _, parentCanceled := range []bool{false, true} {
			svc, account := newAPIKeyAvailabilityTestService(true)
			ctx, cancel := context.WithCancel(context.Background())
			if parentCanceled {
				cancel()
			}
			svc.recordOpenAIAvailabilityTransportFailure(ctx, nil, account, "model", cause)
			want := !parentCanceled && cause != context.Canceled && cause != ErrUpstreamResponseBodyTooLarge
			require.Equal(t, want, svc.isOpenAIAccountModelRuntimeBlocked(account, "model"))
			cancel()
		}
	}
	for _, passthrough := range []bool{false, true} {
		for _, cause := range []error{io.EOF, io.ErrUnexpectedEOF, context.DeadlineExceeded, syscall.ECONNRESET} {
			svc, account := newAPIKeyAvailabilityTestService(true)
			account.Credentials = map[string]any{"model_mapping": map[string]any{"alias": "upstream"}}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := runAvailabilityIOStream(t, svc, account, ctx, passthrough, &openAIStreamReadThenErrorCloser{reader: strings.NewReader(availabilityIOPreamble), err: cause})
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
			require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "alias"))
		}
	}
}

func TestOpenAIAvailabilityNonStreamingReadFailure(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		svc, account := newAPIKeyAvailabilityTestService(true)
		account.Credentials = map[string]any{"model_mapping": map[string]any{"alias": "upstream"}}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: &openAIStreamReadThenErrorCloser{reader: strings.NewReader(`{"id":`), err: io.ErrUnexpectedEOF}}
		var err error
		if passthrough {
			_, err = svc.handleNonStreamingResponsePassthrough(c.Request.Context(), resp, c, account, "alias", "")
		} else {
			_, err = svc.handleNonStreamingResponse(c.Request.Context(), resp, c, account, "alias", "upstream")
		}
		var failover *UpstreamFailoverError
		require.ErrorAs(t, err, &failover)
		require.Empty(t, w.Body.String())
		require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "upstream").failureStreak)
	}
}

func TestOpenAIAvailabilityNonStreamingReadFailureAfterCompactKeepalive(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "passthrough"}[passthrough], func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(true)
			account.Credentials = map[string]any{"model_mapping": map[string]any{"alias": "upstream"}}
			c, recorder := newCompactBridgeTestContext(t, true)
			stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
			defer stop()
			waitForKeepaliveBeats()
			require.True(t, StopOpenAICompactSSEKeepaliveCommitted(c))
			require.True(t, c.Writer.Written())
			require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))
			before := recorder.Body.String()
			require.Contains(t, before, ": keepalive\n\n")
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
				Body: &openAIStreamReadThenErrorCloser{reader: strings.NewReader(`{"id":`), err: io.ErrUnexpectedEOF}}
			var err error
			if passthrough {
				_, err = svc.handleNonStreamingResponsePassthrough(c.Request.Context(), resp, c, account, "alias", "")
			} else {
				_, err = svc.handleNonStreamingResponse(c.Request.Context(), resp, c, account, "alias", "upstream")
			}
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover, "keepalive bytes must not prevent next-account failover")
			require.False(t, failover.RetryableOnSameAccount)
			require.Equal(t, before, recorder.Body.String(), "failed attempt must add no error or response body")
			require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "upstream").failureStreak)
			require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "alias"))
		})
	}
}

func TestOpenAIAvailabilityFirstOutputTimeoutDoesNotPunishCancellation(t *testing.T) {
	for _, cancellation := range []string{"request", "forward"} {
		t.Run(cancellation, func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(true)
			settings, counter := installAvailabilityTimeoutPolicy(t, svc, 1)
			repo, ok := svc.rateLimitService.accountRepo.(*availabilityAccountRepo)
			require.True(t, ok)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			cancelled, cancel := context.WithCancel(context.Background())
			cancel()
			forwardCtx := context.Background()
			if cancellation == "request" {
				c.Request = c.Request.WithContext(cancelled)
			} else {
				forwardCtx = cancelled
			}
			err := svc.newOpenAIFirstOutputTimeoutError(forwardCtx, c, account, opsUpstreamProxyID(account), opsUpstreamProxyName(account), time.Now(), "model", "", 30*time.Second, "semantic_output", nil)
			require.Error(t, err)
			require.Zero(t, settings.calls.Load(), "cancelled caller must not invoke administrator timeout policy")
			require.Zero(t, counter.increments.Load())
			require.Zero(t, counter.resets.Load())
			require.Zero(t, repo.authErrors)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "model"))
		})
	}
	t.Run("live caller still reaches enabled policy", func(t *testing.T) {
		svc, account := newAPIKeyAvailabilityTestService(true)
		settings, counter := installAvailabilityTimeoutPolicy(t, svc, 1)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		err := svc.newOpenAIFirstOutputTimeoutError(c.Request.Context(), c, account, opsUpstreamProxyID(account), opsUpstreamProxyName(account), time.Now(), "model", "", 30*time.Second, "semantic_output", nil)
		require.Error(t, err)
		require.EqualValues(t, 1, settings.calls.Load())
		require.EqualValues(t, 1, counter.increments.Load())
		require.EqualValues(t, 1, counter.resets.Load())
		repo, ok := svc.rateLimitService.accountRepo.(*availabilityAccountRepo)
		require.True(t, ok)
		require.Equal(t, 1, repo.authErrors)
	})
}

func TestOpenAIAvailabilityFirstOutputTimeoutUsesActualCanonicalModel(t *testing.T) {
	svc, account := newAPIKeyAvailabilityTestService(true)
	account.Credentials = map[string]any{"model_mapping": map[string]any{"A": "C", "B": "D"}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	err := svc.newOpenAIFirstOutputTimeoutError(c.Request.Context(), c, account, opsUpstreamProxyID(account), opsUpstreamProxyName(account), time.Now(), "A", "", 30*time.Second, "semantic_output", nil, "B")
	require.Error(t, err)
	state := svc.getOpenAIAccountModelTransientState()
	require.True(t, state.isBlocked(account.ID, "B", time.Now()))
	for _, unaffected := range []string{"A", "C", "D"} {
		require.False(t, state.isBlocked(account.ID, unaffected, time.Now()), "must cool only the actual upstream model")
	}
	require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "B").failureStreak)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestOpenAIAvailabilityConfiguredTimeouts(t *testing.T) {
	t.Run("first output", func(t *testing.T) {
		svc, account := newAPIKeyAvailabilityTestService(true)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		err := svc.newOpenAIFirstOutputTimeoutError(c.Request.Context(), c, account, opsUpstreamProxyID(account), opsUpstreamProxyName(account), time.Now(), "model", "", 30*time.Second, "response_headers", http.Header{})
		require.True(t, err.SafeToFailoverAfterWrite)
		require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "model"))
	})
	for _, textStarted := range []bool{false, true} {
		t.Run(map[bool]string{false: "idle before output", true: "idle after output"}[textStarted], func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(true)
			settings, counter := installAvailabilityTimeoutPolicy(t, svc, 2)
			svc.cfg.Gateway.StreamDataIntervalTimeout = 1
			account.Credentials = map[string]any{"model_mapping": map[string]any{"alias": "upstream"}}
			r, w := io.Pipe()
			t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
			go func() {
				payload := availabilityIOPreamble
				if textStarted {
					payload += availabilityIOText
				}
				_, _ = io.WriteString(w, payload)
			}()
			_, err := runAvailabilityIOStream(t, svc, account, context.Background(), false, r)
			var failover *UpstreamFailoverError
			require.Error(t, err)
			require.Equal(t, !textStarted, errors.As(err, &failover))
			require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "upstream").failureStreak)
			require.EqualValues(t, 1, settings.calls.Load(), "native idle failover must preserve administrator policy")
			require.EqualValues(t, 1, counter.increments.Load())
			require.Zero(t, counter.resets.Load(), "threshold has not been reached")
			repo, ok := svc.rateLimitService.accountRepo.(*availabilityAccountRepo)
			require.True(t, ok)
			require.Zero(t, repo.authErrors)
		})
	}
}
