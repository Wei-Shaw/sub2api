package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const openAIUpstreamKeepaliveFixture = "data: {\"type\":\"keepalive\",\"sequence_number\":1}\n\n"

func TestOpenAIUnrecognizedKeepaliveDiagnostics(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprintf("passthrough=%t", passthrough), func(t *testing.T) {
			logSink, restore := captureStructuredLog(t)
			defer restore()
			reader, writer := io.Pipe()
			body := &capacityBlockingBody{PipeReader: reader, closed: make(chan struct{})}
			t.Cleanup(func() { _ = writer.Close(); _ = body.Close() })
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}
			payload := `{"type":"keepalive","timestamp":1234567,"message":"private-user-text","metadata":{"secret":"private-key"}}`
			go func() {
				defer func() { _ = writer.Close() }()
				_, _ = io.WriteString(writer, openAIUpstreamKeepaliveFixture+"data: "+payload+"\n\ndata: "+payload+"\n\ndata: "+capacityFailedUsageFixture+"\n\n")
			}()
			var gotErr error
			if passthrough {
				_, gotErr = svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, capacityRuleAccount(), time.Now(), "public", "upstream")
			} else {
				_, gotErr = svc.handleStreamingResponse(c.Request.Context(), resp, c, capacityRuleAccount(), time.Now(), "public", "upstream")
			}
			var failover *UpstreamFailoverError
			require.Error(t, gotErr)
			require.False(t, errors.As(gotErr, &failover), "diagnostics must not relax replay safety")
			require.Contains(t, rec.Body.String(), payload, "unknown frames remain untouched")
			logSink.mu.Lock()
			defer logSink.mu.Unlock()
			count := 0
			for _, event := range logSink.events {
				if event.Message != "openai.unrecognized_keepalive" {
					continue
				}
				count++
				encoded, err := json.Marshal(event.Fields)
				require.NoError(t, err)
				require.Contains(t, string(encoded), `"timestamp":"number"`)
				require.Contains(t, string(encoded), `"message":"string"`)
				require.Contains(t, string(encoded), `"metadata":"object"`)
				for _, secret := range []string{"1234567", "private-user-text", "private-key", `"secret"`} {
					require.NotContains(t, string(encoded), secret)
				}
			}
			require.Equal(t, 1, count, "log once per attempt, not once per heartbeat")
		})
	}
}

func TestOpenAIUnrecognizedKeepaliveDiagnosticsSkipsOAuth(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()
	var diag openAICapacityStreamDiagnostics
	diag.unrecognizedKeepalive(context.Background(), &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "native_sse", []byte(`{"type":"keepalive","timestamp":1}`))
	require.False(t, logSink.ContainsMessage("openai.unrecognized_keepalive"))
}

func TestOpenAIUpstreamKeepaliveOutputClassification(t *testing.T) {
	for _, tc := range []struct {
		name, data, eventType string
		wantOutput            bool
	}{
		{"typed heartbeat", `{"type":"keepalive"}`, "keepalive", false},
		{"data-only heartbeat", `{"type":"keepalive"}`, "", false},
		{"named heartbeat", `{}`, "keepalive", false},
		{"sequenced heartbeat", `{"type":"keepalive","sequence_number":1}`, "keepalive", false},
		{"data-only sequenced heartbeat", `{"sequence_number":0,"type":"keepalive"}`, "", false},
		{"named sequenced heartbeat", `{"sequence_number":2}`, "keepalive", false},
		{"sequence is not an event type", `{"sequence_number":1}`, "", true},
		{"string sequence", `{"type":"keepalive","sequence_number":"1"}`, "keepalive", true},
		{"negative sequence", `{"type":"keepalive","sequence_number":-1}`, "keepalive", true},
		{"fractional sequence", `{"type":"keepalive","sequence_number":1.5}`, "keepalive", true},
		{"sequence with content", `{"type":"keepalive","sequence_number":1,"delta":"hello"}`, "keepalive", true},
		{"sequence with metadata", `{"type":"keepalive","sequence_number":1,"metadata":{}}`, "keepalive", true},
		{"empty data", "", "keepalive", false},
		{"unknown event", `{}`, "vendor.keepalive", true},
		{"conflicting event", `{"type":"keepalive"}`, "response.output_text.delta", true},
		{"conflicting type", `{"type":"response.output_text.delta","delta":"hello"}`, "keepalive", true},
		{"extra content", `{"type":"keepalive","delta":"hello"}`, "keepalive", true},
		{"unknown metadata", `{"type":"keepalive","metadata":{}}`, "keepalive", true},
		{"malformed payload", `{"type":"keepalive"`, "keepalive", true},
		{"non-object payload", `[]`, "keepalive", true},
		{"unknown empty object", `{}`, "", true},
		{"text output", `{"type":"response.output_text.delta","delta":"hello"}`, "response.output_text.delta", true},
		{"reasoning output", `{"type":"response.reasoning_summary_text.delta","delta":"thinking"}`, "response.reasoning_summary_text.delta", true},
		{"tool arguments", `{"type":"response.function_call_arguments.delta","delta":"{}"}`, "response.function_call_arguments.delta", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantOutput, openAIStreamDataStartsClientOutput(tc.data, tc.eventType))
			require.Equal(t, tc.wantOutput, openAIStreamDataStartsSemanticTTFT(tc.data, tc.eventType))
		})
	}
}

func TestOpenAIUpstreamKeepalivePreservesPreOutputFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, named := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough=%t/named=%t", passthrough, named), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					reader, writer := io.Pipe()
					body := &capacityBlockingBody{PipeReader: reader, closed: make(chan struct{})}
					t.Cleanup(func() { _ = writer.Close(); _ = body.Close() })
					repo := &capacityRecoveryRepo{}
					svc := &OpenAIGatewayService{
						cfg:              &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 180}},
						rateLimitService: &RateLimitService{accountRepo: repo},
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}
					done := make(chan struct{})
					var gotErr error
					var firstTokenMs *int
					go func() {
						defer close(done)
						defer func() { _ = body.Close() }()
						if passthrough {
							result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, capacityRuleAccount(), time.Now(), "public", "upstream")
							gotErr, firstTokenMs = err, result.firstTokenMs
						} else {
							result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, capacityRuleAccount(), time.Now(), "public", "upstream")
							gotErr, firstTokenMs = err, result.firstTokenMs
						}
					}()
					heartbeat := openAIUpstreamKeepaliveFixture
					if named {
						heartbeat = "event: keepalive\ndata: {\"sequence_number\":1}\n\n"
					}
					_, err := io.WriteString(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_attempt\"}}\n\n"+heartbeat)
					require.NoError(t, err)
					synctest.Wait()
					require.Empty(t, rec.Body.String(), "heartbeat must not flush the attempt preamble")
					started := time.Now()
					_, err = io.WriteString(writer, "data: "+capacityBareFixture+"\n\n")
					require.NoError(t, err)
					// The upstream deliberately remains open after the error.
					<-done
					var failover *UpstreamFailoverError
					require.ErrorAs(t, gotErr, &failover)
					require.False(t, failover.RetryableOnSameAccount)
					require.Zero(t, time.Since(started), "pre-output failure must not wait for the usage drain")
					require.Nil(t, firstTokenMs)
					require.Empty(t, rec.Body.String())
					require.Equal(t, []string{"upstream"}, repo.models)
				})
			})
		}
	}
}

func TestOpenAIUpstreamKeepaliveDoesNotDisarmFirstOutputTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	synctest.Test(t, func(t *testing.T) {
		reader, writer := io.Pipe()
		body := &capacityBlockingBody{PipeReader: reader, closed: make(chan struct{})}
		t.Cleanup(func() { _ = writer.Close(); _ = body.Close() })
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
			OpenAIFirstOutputTimeoutSeconds: 2,
			StreamKeepaliveInterval:         1,
		}}}
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}
		writerDone := make(chan struct{})
		go func() {
			defer close(writerDone)
			for range 10 {
				if _, err := io.WriteString(writer, openAIUpstreamKeepaliveFixture); err != nil {
					return
				}
				select {
				case <-body.closed:
					return
				case <-time.After(250 * time.Millisecond):
				}
			}
			_ = writer.Close()
		}()
		started := time.Now()
		result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, capacityRuleAccount(), started, "public", "upstream")
		var failover *UpstreamFailoverError
		require.ErrorAs(t, err, &failover)
		require.Contains(t, string(failover.ResponseBody), "first_output_timeout")
		require.True(t, failover.SafeToFailoverAfterWrite)
		require.Equal(t, 2*time.Second, time.Since(started))
		require.Nil(t, result.firstTokenMs)
		require.NotEmpty(t, rec.Body.String(), "gateway keepalive must remain enabled")
		for line := range strings.SplitSeq(rec.Body.String(), "\n") {
			require.True(t, line == "" || strings.HasPrefix(line, ":"), "only gateway comments may precede failover: %q", line)
		}
		<-writerDone
	})
}
