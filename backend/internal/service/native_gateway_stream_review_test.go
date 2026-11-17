//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func nativeReviewStreamAccount(t *testing.T, base, profile string) *Account {
	mode, pass := "force_responses", true
	if profile == GatewayLegacyBridgeProfile || profile == GatewayNVIDIABuildChatProfile {
		mode, pass = "force_chat_completions", false
	}
	a := &Account{ID: 17, CreatedAt: time.Date(2026, 10, 3, 0, 0, 0, 123000, time.UTC), Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"api_key": "sandbox-fixture", "base_url": base},
		Extra: map[string]any{GatewayGenerationExtraKey: "99999999-9999-4999-8999-999999999999", GatewayProfileExtraKey: profile, GatewayModelExtraKey: "mimo-test",
			"openai_responses_mode": mode, "openai_passthrough": pass, "native_api_key_cancel_on_disconnect": true, "openai_preserve_compatible_reasoning": true}}
	if profile != "" {
		nativeFixtureSealAccount(t, a)
	}
	return a
}

const nativeReviewCCDelta = "data: {\"id\":\"sandbox\",\"model\":\"mimo-test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"fixture\"},\"finish_reason\":null}]}\n\n"
const nativeReviewCCFinish = "data: {\"id\":\"sandbox\",\"model\":\"mimo-test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"
const nativeReviewDone = "data: [DONE]\n\n"
const nativeReviewCompleted = "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n"

type nativeReviewWriteFailure struct {
	*httptest.ResponseRecorder
	failOn string
}

func (w *nativeReviewWriteFailure) Write(b []byte) (int, error) {
	if w.failOn == "all" || bytes.Contains(b, []byte(w.failOn)) {
		return 0, errors.New("sandbox delivery failure")
	}
	return w.ResponseRecorder.Write(b)
}

func TestGatewayNativeReviewLegacyStrictTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name, stream, failOn string
		success              bool
	}{
		{"empty", "", "", false},
		{"partial EOF", nativeReviewCCDelta, "", false},
		{"malformed", "data: {invalid\n\n", "", false},
		{"malformed after finish", nativeReviewCCFinish + "data: {invalid\n\n" + nativeReviewDone, "", false},
		{"provider error", "data: {\"error\":{\"message\":\"fixture\"}}\n\n", "", false},
		{"DONE without finish", nativeReviewCCDelta + nativeReviewDone, "", false},
		{"finish without DONE", nativeReviewCCDelta + nativeReviewCCFinish, "", false},
		{"delivery failure", nativeReviewCCDelta + nativeReviewCCFinish + nativeReviewDone, "all", false},
		{"completed delivery failure", nativeReviewCCDelta + nativeReviewCCFinish + nativeReviewDone, "response.completed", false},
		{"DONE delivery failure", nativeReviewCCDelta + nativeReviewCCFinish + nativeReviewDone, "[DONE]", false},
		{"qualified finish and terminal", nativeReviewCCDelta + nativeReviewCCFinish + nativeReviewDone, "", true},
		{"empty length terminal", strings.Replace(nativeReviewCCFinish, "stop", "length", 1) + nativeReviewDone, "", true},
		{"text length terminal", nativeReviewCCDelta + strings.Replace(nativeReviewCCFinish, "stop", "length", 1) + nativeReviewDone, "", true},
		{"unknown terminal", nativeReviewCCDelta + strings.Replace(nativeReviewCCFinish, "stop", "content_filter", 1) + nativeReviewDone, "", false},
		{"length without DONE", nativeReviewCCDelta + strings.Replace(nativeReviewCCFinish, "stop", "length", 1), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, tc.stream)
			}))
			defer upstream.Close()
			a := nativeReviewStreamAccount(t, upstream.URL, GatewayLegacyBridgeProfile)
			route, err := GatewayNativeDescriptor(a)
			require.NoError(t, err)
			svc := &OpenAIGatewayService{accountRepo: &gatewayIdentityRepoFixture{row: a}, httpUpstream: &gatewayIdentityRealHTTP{client: upstream.Client()}, cfg: rawChatCompletionsTestConfig()}
			rec := httptest.NewRecorder()
			var writer http.ResponseWriter = rec
			if tc.failOn != "" {
				writer = &nativeReviewWriteFailure{ResponseRecorder: rec, failOn: tc.failOn}
			}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(http.MethodPost, "/private/native/v1/responses", nil)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, entered, err := svc.ForwardGatewayRoute(nativeFixtureContext(t, ctx), c, route, []byte(`{"model":"mimo-test","input":"sandbox","stream":true,"store":false,"service_tier":"default"}`))
			require.True(t, entered)
			require.EqualValues(t, 1, calls.Load())
			if tc.success {
				require.NoError(t, err)
				require.Contains(t, rec.Body.String(), "response.completed")
				require.Contains(t, rec.Body.String(), "[DONE]")
				if strings.Contains(tc.name, "length terminal") {
					require.Contains(t, rec.Body.String(), `"status":"incomplete"`)
					require.Contains(t, rec.Body.String(), `"reason":"max_output_tokens"`)
				} else {
					require.NotContains(t, rec.Body.String(), `"status":"incomplete"`)
				}
			} else {
				require.ErrorIs(t, err, ErrGatewayNativeEffectUnknown)
				// A failure writing DONE occurs after a genuinely qualified completed event.
				if tc.failOn != "[DONE]" {
					require.NotContains(t, rec.Body.String(), "response.completed")
				}
				require.NotContains(t, rec.Body.String(), "[DONE]")
			}
		})
	}
}

// Ordinary bridge behavior remains compatible even on historical incomplete SSE.
func TestGatewayNativeReviewOrdinaryBridgeCompatibility(t *testing.T) {
	for _, stream := range []string{"", nativeReviewCCDelta, "data: {invalid\n\n"} {
		t.Run("historical "+strconv.Itoa(len(stream)), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, stream)
			}))
			defer upstream.Close()
			a := &Account{ID: 23, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Credentials: map[string]any{"api_key": "sandbox-fixture", "base_url": upstream.URL}}
			svc := &OpenAIGatewayService{httpUpstream: &gatewayIdentityRealHTTP{client: upstream.Client()}, cfg: rawChatCompletionsTestConfig()}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			_, err := svc.forwardResponsesViaRawChatCompletions(context.Background(), c, a, []byte(`{"model":"mimo-test","input":"sandbox","stream":true}`))
			require.NoError(t, err)
			require.EqualValues(t, 1, calls.Load())
			require.Contains(t, rec.Body.String(), "response.completed")
			require.Contains(t, rec.Body.String(), "[DONE]")
		})
	}
}

func TestGatewayNativeReviewResponsesSSEFrames(t *testing.T) {
	tool := ": comment\ndata: {\"type\":\"response.output_item.done\",\ndata: \"item\":{\"type\":\"custom_tool_call\",\"name\":\"exec\",\"input\":\"fixture\"}}\n\n"
	multi := ": terminal comment\nevent: response.completed\ndata: {\"type\":\"response.completed\",\ndata: \"response\":{\"status\":\"completed\"}}\n\n"
	sized := func(n int) string {
		return ":" + strings.Repeat("x", n-len(nativeReviewCompleted)-2) + "\n" + nativeReviewCompleted
	}
	heartbeat := ":" + strings.Repeat("x", gatewayNativeSSEFrameLimit-3) + "\n\n"
	cases := []struct {
		name, wire string
		success    bool
	}{
		{"LF joined tool and terminal", tool + multi, true},
		{"CRLF with comments", strings.ReplaceAll(tool+multi, "\n", "\r\n"), true},
		{"malformed joined JSON", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\ndata: {}\n\n", false},
		{"unterminated completed event", strings.TrimSuffix(nativeReviewCompleted, "\n"), false},
		{"incomplete EOF", "data: {\"type\":\"response.created\"}\n\n", false},
		{"unqualified DONE", nativeReviewDone, false},
		{"exact frame cap", sized(gatewayNativeSSEFrameLimit), true},
		{"over frame cap", sized(gatewayNativeSSEFrameLimit + 1), false},
		{"exact total cap", strings.Repeat(heartbeat, 7) + sized(gatewayNativeSSEFrameLimit), true},
		{"over total cap", strings.Repeat(heartbeat, 8) + nativeReviewCompleted, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, tc.wire)
			}))
			defer upstream.Close()
			a := nativeReviewStreamAccount(t, upstream.URL, GatewayMiMoResponsesProfile)
			route, err := GatewayNativeDescriptor(a)
			require.NoError(t, err)
			svc := &OpenAIGatewayService{accountRepo: &gatewayIdentityRepoFixture{row: a}, httpUpstream: &gatewayIdentityRealHTTP{client: upstream.Client()}, cfg: rawChatCompletionsTestConfig()}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/private/native/v1/responses", nil)
			_, entered, err := svc.ForwardGatewayRoute(nativeFixtureContext(t, context.Background()), c, route, []byte(`{"model":"mimo-test","stream":true,"store":false}`))
			require.True(t, entered)
			require.EqualValues(t, 1, calls.Load())
			if tc.success {
				require.NoError(t, err)
				require.Equal(t, tc.wire, rec.Body.String(), "original bounded frames must survive byte for byte")
			} else {
				require.ErrorIs(t, err, ErrGatewayNativeEffectUnknown)
				require.NotContains(t, rec.Body.String(), "response.completed")
			}
		})
	}
}

// Both HTTP hops are real. A live upstream remains open beyond completion until
// Close cancels it. Settlement must arrive first, with no EOF wait or replay.
func TestGatewayNativeReviewCompletionClosesLiveBody(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(map[bool]string{false: "keepalive", true: "read failure after completion"}[reset], func(t *testing.T) {
			var calls atomic.Int32
			closed := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, nativeReviewCompleted)
				flusher, ok := w.(http.Flusher)
				if !ok {
					t.Error("local HTTP peer has no flusher")
					close(closed)
					return
				}
				flusher.Flush()
				if reset {
					hijacker, ok := w.(http.Hijacker)
					if !ok {
						t.Error("local HTTP peer has no hijacker")
						close(closed)
						return
					}
					conn, _, err := hijacker.Hijack()
					if err == nil {
						_ = conn.Close()
					}
					close(closed)
					return
				}
				<-r.Context().Done()
				close(closed)
			}))
			defer upstream.Close()
			a := nativeReviewStreamAccount(t, upstream.URL, GatewayMiMoResponsesProfile)
			route, err := GatewayNativeDescriptor(a)
			require.NoError(t, err)
			svc := &OpenAIGatewayService{accountRepo: &gatewayIdentityRepoFixture{row: a}, httpUpstream: &gatewayIdentityRealHTTP{client: upstream.Client()}, cfg: rawChatCompletionsTestConfig()}
			type outcome struct {
				entered bool
				err     error
			}
			settled := make(chan outcome, 1)
			router := gin.New()
			router.POST("/sandbox", func(c *gin.Context) {
				_, entered, err := svc.ForwardGatewayRoute(nativeFixtureContext(t, c.Request.Context()), c, route, []byte(`{"model":"mimo-test","stream":true}`))
				settled <- outcome{entered, err}
			})
			downstream := httptest.NewServer(router)
			defer downstream.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, downstream.URL+"/sandbox", nil)
			require.NoError(t, err)
			resp, err := downstream.Client().Do(req)
			require.NoError(t, err)
			defer func() { require.NoError(t, resp.Body.Close()) }()
			select {
			case got := <-settled:
				require.True(t, got.entered)
				require.NoError(t, got.err)
			case <-time.After(500 * time.Millisecond):
				cancel()
				t.Fatal("settlement waited for upstream EOF")
			}
			output, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, nativeReviewCompleted, string(output))
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("upstream body did not close")
			}
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestGatewayNativeReviewCompletionDeliveryFailure(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, nativeReviewCompleted)
	}))
	defer upstream.Close()
	a := nativeReviewStreamAccount(t, upstream.URL, GatewayMiMoResponsesProfile)
	route, err := GatewayNativeDescriptor(a)
	require.NoError(t, err)
	svc := &OpenAIGatewayService{accountRepo: &gatewayIdentityRepoFixture{row: a}, httpUpstream: &gatewayIdentityRealHTTP{client: upstream.Client()}, cfg: rawChatCompletionsTestConfig()}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(&nativeReviewWriteFailure{ResponseRecorder: rec, failOn: "response.completed"})
	c.Request = httptest.NewRequest(http.MethodPost, "/private/native/v1/responses", nil)
	_, entered, err := svc.ForwardGatewayRoute(nativeFixtureContext(t, context.Background()), c, route, []byte(`{"model":"mimo-test","stream":true}`))
	require.True(t, entered)
	require.ErrorIs(t, err, ErrGatewayNativeEffectUnknown)
	require.Empty(t, rec.Body.String())
	require.EqualValues(t, 1, calls.Load())
}

type nativeReviewFlushFailure struct{ gin.ResponseWriter }

func (w nativeReviewFlushFailure) FlushError() error { return errors.New("sandbox flush failure") }
func TestGatewayNativeReviewCompletionFlushFailure(t *testing.T) {
	for _, profile := range []string{GatewayMiMoResponsesProfile, GatewayLegacyBridgeProfile} {
		t.Run(profile, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				if profile == GatewayMiMoResponsesProfile {
					_, _ = io.WriteString(w, nativeReviewCompleted)
				} else {
					_, _ = io.WriteString(w, nativeReviewCCDelta+nativeReviewCCFinish+nativeReviewDone)
				}
			}))
			defer upstream.Close()
			a := nativeReviewStreamAccount(t, upstream.URL, profile)
			route, err := GatewayNativeDescriptor(a)
			require.NoError(t, err)
			svc := &OpenAIGatewayService{accountRepo: &gatewayIdentityRepoFixture{row: a}, httpUpstream: &gatewayIdentityRealHTTP{client: upstream.Client()}, cfg: rawChatCompletionsTestConfig()}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Writer = nativeReviewFlushFailure{c.Writer}
			c.Request = httptest.NewRequest(http.MethodPost, "/private/native/v1/responses", nil)
			_, entered, err := svc.ForwardGatewayRoute(nativeFixtureContext(t, context.Background()), c, route, []byte(`{"model":"mimo-test","stream":true}`))
			require.True(t, entered)
			require.ErrorIs(t, err, ErrGatewayNativeEffectUnknown)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}
