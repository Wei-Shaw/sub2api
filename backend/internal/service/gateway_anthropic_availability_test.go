package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAnthropicAvailabilityCooldownRecovery(t *testing.T) {
	s := &GatewayService{}
	a := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
	s.recordAnthropicRateLimit(context.Background(), a, "claude-a", http.Header{"Retry-After": []string{"180"}})
	now := time.Now()
	state := s.anthropicStreamRecovery()
	require.True(t, state.isBlocked(42, "claude-a", now))
	require.False(t, state.isBlocked(42, "claude-b", now))
	snap := state.snapshot(now)
	require.Len(t, snap, 1)
	require.WithinDuration(t, now.Add(180*time.Second), snap[0].BlockUntil, time.Second)
	blocked, id := state.reserveProbe(42, "claude-a", now.Add(181*time.Second))
	require.False(t, blocked)
	require.NotZero(t, id)
	blocked, _ = state.reserveProbe(42, "claude-a", now.Add(181*time.Second))
	require.True(t, blocked)
	require.True(t, state.completeProbe(42, "claude-a", id, now.Add(181*time.Second)))
	require.Empty(t, state.snapshot(now))
}

func TestAnthropicAvailabilityBackoff(t *testing.T) {
	state := newOpenAIAccountModelTransientState(10)
	opts := openAITransientFailureOptions{requireProbe: true, backoffBase: 30 * time.Second, backoffCap: 5 * time.Minute, failureThreshold: 2}
	now := time.Now()
	for _, want := range []time.Duration{0, 30 * time.Second, 60 * time.Second, 120 * time.Second, 240 * time.Second, 300 * time.Second, 300 * time.Second} {
		d := state.recordFailureWithInitialCooldown(1, "m", now, 0, opts)
		require.Equal(t, want, d.Cooldown)
		now = now.Add(want + time.Second)
	}
	state2 := newOpenAIAccountModelTransientState(10)
	state2.recordFailureWithInitialCooldown(1, "m", now, 0, opts)
	state2.recordSuccess(1, "m")
	require.Empty(t, state2.snapshot(now))
}

func TestAnthropicAvailabilityTimeoutHeadersAndHeartbeats(t *testing.T) {
	for _, headers := range []bool{false, true} {
		t.Run(map[bool]string{false: "headers", true: "heartbeats"}[headers], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if headers {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "event: ping\ndata: {\"type\":\"ping\"}\n\n")
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			req, _ := http.NewRequest("GET", server.URL, nil)
			resp, err := guardAnthropicFirstOutput(context.Background(), req, 40*time.Millisecond, http.DefaultClient.Do, nil)
			require.Nil(t, resp)
			require.ErrorIs(t, err, context.DeadlineExceeded)
		})
	}
}

func TestAnthropicAvailabilityReplayAndStopDeadline(t *testing.T) {
	for _, event := range []string{
		`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`,
		`{"type":"content_block_start","content_block":{"type":"tool_use","id":"t","name":"run","input":{}}}`,
		`{"type":"message_start","message":{"usage":{"input_tokens":12}}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: "+event+"\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-time.After(100 * time.Millisecond):
				_, _ = io.WriteString(w, "data: {\"type\":\"message_stop\"}\n\n")
			case <-r.Context().Done():
			}
		}))
		req, _ := http.NewRequest("GET", server.URL, nil)
		resp, err := guardAnthropicFirstOutput(context.Background(), req, 50*time.Millisecond, http.DefaultClient.Do, nil)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, "data: "+event+"\n\ndata: {\"type\":\"message_stop\"}\n\n", string(body))
		server.Close()
	}
}

func TestAnthropicAvailabilityErrorBodyAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequest("GET", "http://localhost", nil)
	_, err := guardAnthropicFirstOutput(ctx, req, time.Second, func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() }, nil)
	require.ErrorIs(t, err, context.Canceled)
	resp, err := guardAnthropicFirstOutput(context.Background(), req, time.Second, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("error"))}, nil
	}, nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, "error", string(body))
}

func TestAnthropicAvailabilityPool429And503SwitchImmediately(t *testing.T) {
	account := newAnthropicAPIKeyAccountForTest()
	account.Extra["pool_mode"] = true
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"75"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`))}}
	svc := &GatewayService{httpUpstream: upstream}
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	resp, err := svc.doAnthropicAvailabilityRequest(context.Background(), req, "", account, "claude-a", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.True(t, svc.anthropicStreamRecovery().isBlocked(account.ID, "claude-a", time.Now()))
	for _, status := range []int{429, 503, 524} {
		require.False(t, svc.shouldRetryUpstreamError(account, status))
		require.False(t, account.IsPoolModeRetryableStatus(status))
	}
}
