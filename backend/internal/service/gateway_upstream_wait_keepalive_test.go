//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// redirectHTTPUpstream sends every upstream request over a real HTTP client to
// a local test server, whatever host the gateway built the URL for (OAuth and
// Bedrock requests target fixed public hosts).
type redirectHTTPUpstream struct {
	target *url.URL
	client *http.Client
}

func (u redirectHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	req.URL.Scheme = u.target.Scheme
	req.URL.Host = u.target.Host
	req.Host = ""
	return u.client.Do(req)
}

func (u redirectHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

const anthropicUpstreamWaitTestStream = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_slow","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","usage":{"input_tokens":3,"output_tokens":1}}}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"pong"}}` + "\n\n" +
	"event: content_block_stop\n" +
	`data: {"type":"content_block_stop","index":0}` + "\n\n" +
	"event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}` + "\n\n" +
	"event: message_stop\n" +
	`data: {"type":"message_stop"}` + "\n\n"

const anthropicUpstreamWaitTestMessage = `{"id":"msg_slow","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"pong"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`

// slowAnthropicUpstream holds the response headers back for delay, then calls
// answer to write the response.
func slowAnthropicUpstream(t *testing.T, delay time.Duration, answer func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		answer(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func answerAnthropic(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, "invoke-with-response-stream") {
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bedrockUpstreamWaitTestStream())
		return
	}
	var body struct {
		Stream bool `json:"stream"`
	}
	_ = jsonDecodeRequest(r, &body)
	if body.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(anthropicUpstreamWaitTestStream))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(anthropicUpstreamWaitTestMessage))
}

func answerStatus(status int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"upstream overloaded"}}`))
	}
}

// answerHeadersThenDrop sends 200 SSE headers and then drops the connection
// before any event, so reading the body fails.
func answerHeadersThenDrop(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()
	panic(http.ErrAbortHandler)
}

func jsonDecodeRequest(r *http.Request, v any) error {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r.Body); err != nil {
		return err
	}
	return json.Unmarshal(buf.Bytes(), v)
}

// bedrockUpstreamWaitTestStream encodes anthropicUpstreamWaitTestStream as the
// AWS EventStream frames Bedrock answers invoke-with-response-stream with.
func bedrockUpstreamWaitTestStream() []byte {
	var out bytes.Buffer
	for _, block := range strings.Split(strings.TrimSpace(anthropicUpstreamWaitTestStream), "\n\n") {
		data := block[strings.Index(block, "data: ")+len("data: "):]
		payload := `{"bytes":"` + base64.StdEncoding.EncodeToString([]byte(data)) + `"}`
		out.Write(bedrockEventStreamFrame("chunk", []byte(payload)))
	}
	return out.Bytes()
}

func bedrockEventStreamFrame(eventType string, payload []byte) []byte {
	var headers bytes.Buffer
	for _, h := range [][2]string{{":event-type", eventType}, {":message-type", "event"}} {
		headers.WriteByte(byte(len(h[0])))
		headers.WriteString(h[0])
		headers.WriteByte(7) // string value
		_ = binary.Write(&headers, binary.BigEndian, uint16(len(h[1])))
		headers.WriteString(h[1])
	}
	var frame bytes.Buffer
	_ = binary.Write(&frame, binary.BigEndian, uint32(12+headers.Len()+len(payload)+4))
	_ = binary.Write(&frame, binary.BigEndian, uint32(headers.Len()))
	_ = binary.Write(&frame, binary.BigEndian, crc32.ChecksumIEEE(frame.Bytes()))
	frame.Write(headers.Bytes())
	frame.Write(payload)
	_ = binary.Write(&frame, binary.BigEndian, crc32.ChecksumIEEE(frame.Bytes()))
	return frame.Bytes()
}

func gatewayUpstreamWaitTestService(srv *httptest.Server) *GatewayService {
	target, _ := url.Parse(srv.URL)
	cfg := &config.Config{
		Gateway:  config.GatewayConfig{StreamKeepaliveInterval: 1, MaxLineSize: defaultMaxLineSize},
		Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false, AllowInsecureHTTP: true}},
	}
	return &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         redirectHTTPUpstream{target: target, client: &http.Client{}},
		rateLimitService:     &RateLimitService{},
		deferredService:      &DeferredService{},
	}
}

type gatewayUpstreamWaitAccount struct {
	name    string
	account func(baseURL string) *Account
}

// The three send points of GatewayService.Forward: the OAuth / API-key
// request loop, API-key passthrough, and Bedrock.
var gatewayUpstreamWaitAccounts = []gatewayUpstreamWaitAccount{
	{name: "oauth", account: func(string) *Account {
		return &Account{ID: 301, Name: "slow-oauth", Platform: PlatformAnthropic, Type: AccountTypeOAuth, Concurrency: 1,
			Credentials: map[string]any{"access_token": "oauth-token"}, Status: StatusActive, Schedulable: true}
	}},
	{name: "apikey_passthrough", account: func(baseURL string) *Account {
		return &Account{ID: 302, Name: "slow-passthrough", Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{"api_key": "sk-test", "base_url": baseURL},
			Extra:       map[string]any{"anthropic_passthrough": true}, Status: StatusActive, Schedulable: true}
	}},
	{name: "bedrock", account: func(string) *Account {
		return &Account{ID: 303, Name: "slow-bedrock", Platform: PlatformAnthropic, Type: AccountTypeBedrock, Concurrency: 1,
			Credentials: map[string]any{"auth_mode": "apikey", "api_key": "bedrock-key", "aws_region": "us-east-1"},
			Status:      StatusActive, Schedulable: true}
	}},
}

func gatewayUpstreamWaitTestRequest(t *testing.T, stream bool) (*gin.Context, *httptest.ResponseRecorder, *ParsedRequest) {
	t.Helper()
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":32,"messages":[{"role":"user","content":"ping"}],"stream":false}`)
	if stream {
		body = bytes.Replace(body, []byte(`"stream":false`), []byte(`"stream":true`), 1)
	}
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec, parsed
}

func TestGatewayForwardPingsStreamingClientWhileUpstreamHeadersAreLate(t *testing.T) {
	for _, tc := range gatewayUpstreamWaitAccounts {
		t.Run(tc.name, func(t *testing.T) {
			srv := slowAnthropicUpstream(t, 2500*time.Millisecond, answerAnthropic)
			svc := gatewayUpstreamWaitTestService(srv)
			c, rec, parsed := gatewayUpstreamWaitTestRequest(t, true)

			result, err := svc.Forward(context.Background(), c, tc.account(srv.URL), parsed)

			require.NoError(t, err)
			require.NotNil(t, result)
			body := rec.Body.String()
			pings := anthropicUpstreamWaitPing + anthropicUpstreamWaitPing
			require.True(t, strings.HasPrefix(body, pings), "two pings precede the upstream answer, got %q", body)
			require.Greater(t, strings.Index(body, "message_start"), len(pings)-1)
			require.Contains(t, body, "pong")
			require.Equal(t, "text/event-stream", rec.Result().Header.Get("Content-Type"))
		})
	}
}

// After pings committed the response, an upstream error before any output must
// still leave the response unwritten in the failover sense, attempt after
// attempt. OpenAICompactKeepaliveAdjustedWrittenSize is the predicate the
// Messages handler compares across Forward.
func TestGatewayForwardLateUpstreamErrorAfterPingsStillFailsOver(t *testing.T) {
	for _, tc := range gatewayUpstreamWaitAccounts {
		t.Run(tc.name, func(t *testing.T) {
			srv := slowAnthropicUpstream(t, 1500*time.Millisecond, answerStatus(http.StatusBadGateway))
			svc := gatewayUpstreamWaitTestService(srv)
			c, rec, parsed := gatewayUpstreamWaitTestRequest(t, true)
			before := OpenAICompactKeepaliveAdjustedWrittenSize(c)

			for attempt := 1; attempt <= 2; attempt++ {
				_, err := svc.Forward(context.Background(), c, tc.account(srv.URL), parsed)
				var failoverErr *UpstreamFailoverError
				require.True(t, errors.As(err, &failoverErr), "attempt %d: want failover error, got %v", attempt, err)
				require.Equal(t, before, OpenAICompactKeepaliveAdjustedWrittenSize(c), "attempt %d: pings are not client output", attempt)
				require.Equal(t, strings.Repeat(anthropicUpstreamWaitPing, attempt), rec.Body.String())
			}
		})
	}
}

// A stream that breaks before its first event fails over even though the
// pings already wrote to the client.
func TestGatewayForwardStreamDroppedAfterPingsStillFailsOver(t *testing.T) {
	srv := slowAnthropicUpstream(t, 1500*time.Millisecond, answerHeadersThenDrop)
	svc := gatewayUpstreamWaitTestService(srv)
	c, rec, parsed := gatewayUpstreamWaitTestRequest(t, true)

	_, err := svc.Forward(context.Background(), c, gatewayUpstreamWaitAccounts[0].account(srv.URL), parsed)

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "want failover error, got %v", err)
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))
	require.Equal(t, anthropicUpstreamWaitPing, rec.Body.String())
}

func TestGatewayForwardNonStreamingClientGetsNoPings(t *testing.T) {
	for _, tc := range gatewayUpstreamWaitAccounts {
		t.Run(tc.name, func(t *testing.T) {
			srv := slowAnthropicUpstream(t, 1500*time.Millisecond, answerAnthropic)
			svc := gatewayUpstreamWaitTestService(srv)
			c, rec, parsed := gatewayUpstreamWaitTestRequest(t, false)

			_, err := svc.Forward(context.Background(), c, tc.account(srv.URL), parsed)

			require.NoError(t, err)
			require.True(t, strings.HasPrefix(rec.Body.String(), "{"), "non-streaming body is plain JSON, got %q", rec.Body.String())
			require.Contains(t, rec.Body.String(), "pong")
		})
	}
}
