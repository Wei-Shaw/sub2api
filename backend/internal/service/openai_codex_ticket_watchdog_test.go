package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func watchdogCompletedEvent(model string) string {
	return `data: {"type":"response.completed","response":{"status":"completed","model":"` + model + `"}}` + "\n\n"
}

func watchdogArmRequest(t *testing.T, s *OpenAIGatewayService, a *Account) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), "POST", "https://chatgpt.com/backend-api/codex/responses", strings.NewReader(`{"model":"gpt-6-astra"}`))
	require.NoError(t, err)
	require.NoError(t, s.applyOpenAICodexTicketToRequest(req.Context(), a, "gpt-6-astra", req))
	return req
}

func TestCodexTicketWatchdogResponseRecoversWithoutReplayingBusiness(t *testing.T) {
	for _, reason := range []string{"model_mismatch", "state_312"} {
		t.Run(reason, func(t *testing.T) {
			var business, probes atomic.Int64
			payload := watchdogCompletedEvent("gpt-5.6-luna")
			u := &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
				if req.Context().Value(codexTicketReceiptContextKey{}) != nil {
					business.Add(1)
					resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(iotest.OneByteReader(strings.NewReader(payload)))}
					if reason == "state_312" {
						resp.Header.Set(openAICodexTurnStateHeader, fakeCodexTicketState(312))
					}
					return resp, nil
				}
				probes.Add(1)
				return codexTicketResponse(), nil
			}}
			s, repo := ticketJobService(t, u)
			a, err := repo.GetByID(context.Background(), 41)
			require.NoError(t, err)
			old := verifiedTestTicket(a, 292)
			s.storeOpenAICodexTicket(context.Background(), a, old)
			// A successful harvest must not hold a cooldown against a real signal.
			s.openaiCodexAccountJobs = map[int64]*codexAccountTicketJob{41: {revision: old.ConfigRevision}}
			req := watchdogArmRequest(t, s, a)
			resp, err := s.doOpenAIUpstream(req, a.Proxy.URL(), a)
			require.NoError(t, err)
			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, payload, string(got), "watchdog must not rewrite a business response")
			s.openaiCodexAccountMu.Lock()
			job := s.openaiCodexAccountJobs[41]
			s.openaiCodexAccountMu.Unlock()
			waitCodexTicketJob(t, job)
			require.Equal(t, int64(1), business.Load())
			require.Equal(t, int64(2), probes.Load(), "only harvest + original fixed-proxy replay")
			status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
			require.NoError(t, err)
			require.Equal(t, "ready", status.State)
			require.True(t, status.Watchdog.Enabled)
			require.Equal(t, reason, status.Watchdog.LastReason)
			require.Equal(t, int64(1), status.Watchdog.TriggerCount)
			require.NotNil(t, status.Watchdog.LastTriggeredAt)
			// A response from the old ticket arrives after a replacement was saved.
			s.invalidateCodexTicketFromResponse(receiptForCodexTicket(old), "model_mismatch")
			status, err = s.GetCodexAccountTicketStatus(context.Background(), 41)
			require.NoError(t, err)
			require.Equal(t, "ready", status.State)
			require.Equal(t, int64(1), status.Watchdog.TriggerCount)
			live, err := repo.GetByID(context.Background(), 41)
			require.NoError(t, err)
			require.Equal(t, a.ProxyID, live.ProxyID)
			require.NotContains(t, RedactOpenAICodexTicketExtra(live.Extra), codexTicketWatchdogExtraKey)
			require.NotContains(t, MergeOpenAICodexTicketExtra(live.Extra, nil), codexTicketWatchdogExtraKey)
		})
	}
}

func TestCodexTicketWatchdogConcurrentSignalsDeduplicateAndBlockStaleTicket(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var calls atomic.Int64
	s, repo := ticketJobService(t, &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		once.Do(func() { close(started) })
		select {
		case <-release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return codexTicketResponse(), nil
	}})
	defer close(release)
	a, err := repo.GetByID(context.Background(), 41)
	require.NoError(t, err)
	old := verifiedTestTicket(a, 292)
	s.storeOpenAICodexTicket(context.Background(), a, old)
	a.Extra[openAICodexTicketExtraKey(old.Model)] = old
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.invalidateCodexTicketFromResponse(receiptForCodexTicket(old), "model_mismatch")
		}()
	}
	wg.Wait()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("watchdog did not start a recovery probe")
	}
	require.Equal(t, int64(1), calls.Load())
	require.Nil(t, s.lookupOpenAICodexTicket(a, old.Model), "old snapshot cannot reinsert the revoked ticket")
	require.ErrorIs(t, s.applyOpenAICodexTicket(context.Background(), a, old.Model, http.Header{}), ErrOpenAICodexTicketUnavailable)
	status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
	require.NoError(t, err)
	require.Equal(t, int64(1), status.Watchdog.TriggerCount)
	require.Equal(t, "harvesting", status.State)
}

func TestCodexTicketWatchdogIgnoresUnmanagedFailedAndDisabledResponses(t *testing.T) {
	for _, change := range []string{"client_header", "http_error", "account_off", "global_off", "other_model", "plan_changed"} {
		t.Run(change, func(t *testing.T) {
			s, repo := ticketJobService(t, &codexTicketFuncUpstream{do: func(*http.Request) (*http.Response, error) {
				t.Error("an ignored response must not start harvesting")
				return codexTicketResponse(), nil
			}})
			a, err := repo.GetByID(context.Background(), 41)
			require.NoError(t, err)
			old := verifiedTestTicket(a, 292)
			s.storeOpenAICodexTicket(context.Background(), a, old)
			req := watchdogArmRequest(t, s, a)
			resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(watchdogCompletedEvent("gpt-5.6-luna")))}
			resp.Header.Set(openAICodexTurnStateHeader, fakeCodexTicketState(312))
			switch change {
			case "client_header":
				req = req.WithContext(context.Background())
			case "http_error":
				resp.StatusCode = 429
			case "account_off", "plan_changed":
				ac := codexAccountTicketConfigOf(a)
				if change == "account_off" {
					ac.Enabled = false
				} else {
					ac.TicketPlan = "team"
					ac.Revision = "new"
				}
				require.NoError(t, repo.UpdateExtra(context.Background(), 41, map[string]any{codexAccountTicketConfigKey: ac}))
			case "global_off":
				s.cfg.Gateway.OpenAICodexTicket.Enabled = false
			case "other_model":
				req, err = http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)
				require.NoError(t, err)
				require.NoError(t, s.applyOpenAICodexTicketToRequest(req.Context(), a, "gpt-5.5", req))
			}
			s.observeCodexTicketResponse(req, resp)
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Empty(t, s.openaiCodexAccountJobs)
		})
	}
}

func TestCodexTicketWatchdogParserPreservesStreamAndUsesCompletedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"mismatch", watchdogCompletedEvent("gpt-5.6-luna"), 1},
		{"match", watchdogCompletedEvent("gpt-6-astra"), 0},
		{"created_only", `data: {"type":"response.created","response":{"model":"gpt-5.6-luna"}}` + "\n\n", 0},
		{"event_name_without_type", "event: response.completed\ndata: {\"response\":{\"model\":\"gpt-5.6-luna\"}}\n\n", 1},
		{"completed_without_status", `data: {"type":"response.completed","response":{"model":"gpt-5.6-luna"}}` + "\n\n", 1},
		{"failed", `data: {"type":"response.completed","response":{"status":"failed","model":"gpt-5.6-luna"}}` + "\n\n", 0},
		{"missing_model", `data: {"type":"response.completed","response":{"status":"completed"}}` + "\n\n", 0},
		{"output_text", `data: {"type":"response.output_text.delta","delta":"gpt-5.6-luna"}` + "\n\n", 0},
		{"json", `{"object":"response","status":"completed","model":"gpt-5.6-luna"}`, 1},
		{"truncated_json", `{"object":"response","status":"completed","model":"gpt-5.6-luna"`, 0},
		{"crlf_multiline", "event: response.completed\r\ndata: {\"type\":\"response.completed\",\r\ndata: \"response\":{\"status\":\"completed\",\"model\":\"gpt-5.6-luna\"}}\r\n\r\n", 1},
		{"oversized_then_recover", "data: " + strings.Repeat("x", codexTicketWatchdogBufferLimit+1) + "\n\n" + watchdogCompletedEvent("gpt-5.6-luna"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := 0
			b := &codexTicketWatchdogBody{ReadCloser: io.NopCloser(iotest.HalfReader(strings.NewReader(tc.body))), model: "gpt-6-astra", trigger: func(reason string) { require.Equal(t, "model_mismatch", reason); n++ }}
			got, err := io.ReadAll(b)
			require.NoError(t, err)
			require.Equal(t, tc.body, string(got))
			require.Equal(t, tc.want, n)
		})
	}
}

func TestCodexTicketWatchdogCloseObservesTerminalWithoutReadingAhead(t *testing.T) {
	terminal := strings.TrimSuffix(watchdogCompletedEvent("gpt-5.6-luna"), "\n")
	underlying := strings.NewReader(terminal + "\nUNREAD")
	n := 0
	b := &codexTicketWatchdogBody{ReadCloser: io.NopCloser(underlying), model: "gpt-6-astra", trigger: func(string) { n++ }}
	got := make([]byte, len(terminal))
	_, err := io.ReadFull(b, got)
	require.NoError(t, err)
	require.Zero(t, n)
	require.NoError(t, b.Close())
	require.Equal(t, 1, n)
	require.NoError(t, b.Close())
	require.Equal(t, 1, n)
	rest, err := io.ReadAll(underlying)
	require.NoError(t, err)
	require.Equal(t, "\nUNREAD", string(rest))
}

func TestCodexTicketWatchdogConcurrentReadClose(t *testing.T) {
	for i := 0; i < 32; i++ {
		pr, pw := io.Pipe()
		var triggers atomic.Int64
		body := &codexTicketWatchdogBody{ReadCloser: pr, model: "gpt-6-astra", trigger: func(string) { triggers.Add(1) }}
		readDone, writeDone := make(chan struct{}), make(chan struct{})
		go func() { defer close(readDone); _, _ = io.Copy(io.Discard, body) }()
		go func() {
			defer close(writeDone)
			_, _ = io.WriteString(pw, watchdogCompletedEvent("gpt-5.6-luna"))
			_ = pw.Close()
		}()
		require.NoError(t, body.Close())
		select {
		case <-readDone:
		case <-time.After(time.Second):
			t.Fatal("concurrent Read did not unblock")
		}
		select {
		case <-writeDone:
		case <-time.After(time.Second):
			t.Fatal("concurrent writer did not unblock")
		}
		require.LessOrEqual(t, triggers.Load(), int64(1))
	}
}

func TestCodexTicketWatchdogMultipleRevocationsCannotResurrectEarlierTicket(t *testing.T) {
	a := ticketTestAccount(41)
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, nil)
	s.accountRepo = &codexTicketRefreshRepo{accounts: []Account{*a}}
	first := verifiedTestTicket(a, 292)
	s.storeOpenAICodexTicket(context.Background(), a, first)
	a.Extra[openAICodexTicketExtraKey(first.Model)] = first
	s.openaiCodexAccountJobs = map[int64]*codexAccountTicketJob{41: {revision: first.ConfigRevision, harvestProxyURL: s.openAICodexTicketHarvestProxyURL(), retryAfter: time.Now().Add(time.Minute), lastError: "cooldown"}}
	s.invalidateCodexTicketFromResponse(receiptForCodexTicket(first), "model_mismatch")
	second := *first
	second.CapturedAt = second.CapturedAt.Add(time.Millisecond)
	second.ExpiresAt = second.CapturedAt.Add(time.Hour)
	s.storeOpenAICodexTicket(context.Background(), a, &second)
	s.invalidateCodexTicketFromResponse(receiptForCodexTicket(&second), "state_312")
	require.Nil(t, s.lookupOpenAICodexTicket(a, first.Model), "a stale snapshot of A stays revoked after revoking B")
	require.ErrorIs(t, s.applyOpenAICodexTicket(context.Background(), a, first.Model, http.Header{}), ErrOpenAICodexTicketUnavailable)
	status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
	require.NoError(t, err)
	require.Equal(t, int64(2), status.Watchdog.TriggerCount)
}

func TestCodexTicketWatchdogPreservesFailedHarvestCooldown(t *testing.T) {
	a := ticketTestAccount(41)
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, nil)
	s.accountRepo = &codexTicketRefreshRepo{accounts: []Account{*a}}
	old := verifiedTestTicket(a, 292)
	s.storeOpenAICodexTicket(context.Background(), a, old)
	job := &codexAccountTicketJob{revision: old.ConfigRevision, harvestProxyURL: s.openAICodexTicketHarvestProxyURL(), retryAfter: time.Now().Add(time.Minute), lastError: "previous failure"}
	s.openaiCodexAccountJobs = map[int64]*codexAccountTicketJob{41: job}
	s.invalidateCodexTicketFromResponse(receiptForCodexTicket(old), "model_mismatch")
	require.Same(t, job, s.openaiCodexAccountJobs[41])
	require.False(t, job.running)
	require.Nil(t, s.lookupOpenAICodexTicket(a, old.Model))
}

func TestCodexTicketWatchdogHarvestRejectsFixedReplay312Signal(t *testing.T) {
	var calls atomic.Int64
	s, _ := ticketJobService(t, &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		response := codexTicketResponse()
		if calls.Add(1) == 2 {
			require.NotEmpty(t, req.Header.Get(openAICodexTurnStateHeader))
			response.Header.Set(openAICodexTurnStateHeader, fakeCodexTicketState(312))
		}
		return response, nil
	}})
	job := s.startCodexAccountTicketJob(context.Background(), 41, true)
	waitCodexTicketJob(t, job)
	require.Equal(t, int64(4), calls.Load())
	require.Equal(t, 2, job.attempts)
	status, err := s.GetCodexAccountTicketStatus(context.Background(), 41)
	require.NoError(t, err)
	require.Equal(t, "ready", status.State)
}
