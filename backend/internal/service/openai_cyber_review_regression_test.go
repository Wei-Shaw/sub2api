package service

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/tidwall/gjson"
)

func TestCyberPolicyUsageSnapshotsAreImmutable(t *testing.T) {
	c, _ := newCyberBlockTestCtx(nil, `{}`)
	MarkOpsCyberPolicy(c, CyberPolicyMark{Message: "first"})
	first := GetOpsCyberPolicy(c)
	var workers sync.WaitGroup
	for i := 1; i <= 32; i++ {
		workers.Add(1)
		go func(tokens int) {
			defer workers.Done()
			MarkOpsCyberPolicy(c, CyberPolicyMark{Message: "later", UpstreamInTok: tokens})
		}(i)
	}
	workers.Wait()
	require.Zero(t, first.UpstreamInTok, "published snapshots must not be mutated by later events")
	require.Equal(t, 32, GetOpsCyberPolicy(c).UpstreamInTok)
	require.Equal(t, "first", GetOpsCyberPolicy(c).Message)
}

func TestCyberPolicyBeatsGenericWSRetryHints(t *testing.T) {
	require.False(t, isOpenAIWSRateLimitError("cyber_policy", "rate_limit_error", "rate limit exceeded"))
	_, retry := classifyOpenAIWSErrorEventFromRaw("cyber_policy", "server_error", "upgrade required")
	require.False(t, retry)
}

func TestCyberSessionSettingsRefreshAffectsNewHits(t *testing.T) {
	combo := &comboCacheAndStore{}
	svc := enabledCyberSessionTestService(combo)
	c, body := newCyberBlockTestCtx(nil, `{"input":"test"}`)
	identity := svc.PrepareCyberSessionIdentity(context.Background(), 7, 11, c, body, "", "")
	BindCyberSessionIdentity(c, identity)
	svc.settingService.refreshCachedSettings(&SystemSettings{CyberSessionBlockEnabled: true, CyberSessionBlockTTLSeconds: 120})
	enabled, ttl := svc.CyberSessionBlockRuntime(context.Background())
	require.True(t, enabled)
	require.Equal(t, 120*time.Second, ttl)
	MarkOpsCyberPolicy(c, CyberPolicyMark{Message: "blocked"})
	require.InDelta(t, 120, time.Until(identity.blockedUntil).Seconds(), 1)
}

func TestCyberSessionDeadlineExpiresWithoutBeingRestarted(t *testing.T) {
	combo := &comboCacheAndStore{}
	svc := enabledCyberSessionTestService(combo)
	svc.settingService.settingRepo = &fakeSettingRepo{vals: map[string]string{
		SettingKeyCyberSessionBlockEnabled: "true", SettingKeyCyberSessionBlockTTLSeconds: "1",
	}}
	c, body := newCyberBlockTestCtx(nil, `{"input":"test"}`)
	identity := svc.PrepareCyberSessionIdentity(context.Background(), 7, 11, c, body, "", "")
	BindCyberSessionIdentity(c, identity)
	MarkOpsCyberPolicy(c, CyberPolicyMark{Message: "blocked"})
	require.True(t, CyberSessionBlockStillActive(c))
	deadline := identity.blockedUntil
	RetryCyberSessionBlock(c)
	require.Equal(t, deadline, identity.blockedUntil)
	time.Sleep(time.Until(deadline) + 10*time.Millisecond)
	require.False(t, CyberSessionBlockStillActive(c), "WS connection flags must expire with the configured block")
	before := len(combo.events)
	RetryCyberSessionBlock(c)
	require.Len(t, combo.events, before, "late handler cleanup must not re-create an expired block")
}

type reviewCyber429Dialer struct{}

func (*reviewCyber429Dialer) Dial(context.Context, string, http.Header, string) (openAIWSClientConn, int, http.Header, error) {
	return nil, 429, http.Header{}, &openAIWSHandshakeError{Body: []byte(openAIWSHandshakeCyberBody), Err: errors.New("handshake rejected")}
}
func TestReviewCyber429HandshakeMustNotFailover(t *testing.T) {
	control, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), newStagedPassthroughConn())
	svc.openaiWSPassthroughDialer = &reviewCyber429Dialer{}
	observed := make(chan error, 1)
	server, _ := startPassthroughLifecycleServerWithHooks(t, control, svc, passthroughLifecycleAccount(), func(c *gin.Context) *OpenAIWSIngressHooks {
		return &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, err error) { observed <- err }}
	})
	defer server.Close()
	client := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = client.CloseNow() }()
	select {
	case err := <-observed:
		var failover *UpstreamFailoverError
		require.False(t, errors.As(err, &failover), "cyber must beat a wrapper's HTTP 429")
	case <-time.After(3 * time.Second):
		t.Fatal("missing AfterTurn")
	}
}

func TestReviewCyberBufferedSSE(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	body := []byte("event: error\ndata: " + cpaLegacyCyberEvent + "\n\n")
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}}
	_, err := svc.handleSSEToJSON(resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, body, "gpt-5.6-sol", "gpt-5.6-sol")
	t.Logf("err=%v mark=%+v status=%d body=%s", err, GetOpsCyberPolicy(c), c.Writer.Status(), recorder.Body.String())
	require.NotNil(t, GetOpsCyberPolicy(c), "buffered cyber must activate the session block")
	require.Equal(t, 400, recorder.Code)
	require.Equal(t, "cyber_policy", gjson.Get(recorder.Body.String(), "error.code").String())
}

func TestCyberPolicyBufferedResponsesShapes(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, payload := range []string{openAIWSHandshakeCyberBody, cpaLegacyCyberEvent, `{"type":"response.failed","response":{"id":"resp_json","error":{"code":"cyber_policy","message":"blocked"},"usage":{"input_tokens":9,"output_tokens":2}}}`} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("passthrough=%v/sse=%v/%s", passthrough, stream, payload), func(t *testing.T) {
					combo := &comboCacheAndStore{}
					svc := enabledCyberSessionTestService(combo)
					svc.cfg = &config.Config{}
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
					identity := svc.PrepareCyberSessionIdentity(context.Background(), 7, 11, c, []byte(`{"input":"trigger"}`), "", "")
					BindCyberSessionIdentity(c, identity)
					contentType, body := "application/json", payload
					if stream {
						contentType = "text/event-stream"
						body = "event: error\ndata: " + payload + "\n\n"
					}
					resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
					account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
					if passthrough {
						_, _ = svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "gpt-5.6-sol", "gpt-5.6-sol")
					} else {
						_, _ = svc.handleNonStreamingResponse(context.Background(), resp, c, account, "gpt-5.6-sol", "gpt-5.6-sol")
					}
					require.Equal(t, 400, recorder.Code)
					require.Equal(t, "cyber_policy", gjson.Get(recorder.Body.String(), "error.code").String())
					require.True(t, combo.store.blocked[identity.lineageRoot])
				})
			}
		}
	}
}

func TestReviewCyberBridgeHTTPError(t *testing.T) {
	c, _ := newCyberBlockTestCtx(nil, `{}`)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(openAIWSHandshakeCyberBody)),
	}}}
	request := []byte(`{"type":"response.create","model":"gpt-5.6-sol","input":"hi"}`)
	var frames [][]byte
	_, _ = svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}, "sk-test", request, len(request), "gpt-5.6-sol", "", "", "", "", 1, func(b []byte) error { frames = append(frames, b); return nil })
	require.Len(t, frames, 1)
	t.Logf("mark=%+v frame=%s", GetOpsCyberPolicy(c), frames[0])
	require.Equal(t, "cyber_policy", gjson.GetBytes(frames[0], "error.code").String())
}

func TestReviewCyberPairUsage(t *testing.T) {
	c, _ := newCyberBlockTestCtx(nil, `{}`)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	body := "event: error\ndata: " + cpaLegacyCyberEvent + "\n\n" +
		`event: response.failed` + "\n" + `data: {"type":"response.failed","response":{"id":"resp_final","status":"failed","error":{"code":"cyber_policy","message":"blocked"},"usage":{"input_tokens":9,"output_tokens":2}}}` + "\n\n"
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
	result, _ := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")
	t.Logf("result usage=%+v mark=%+v", result.usage, GetOpsCyberPolicy(c))
	require.Equal(t, 9, GetOpsCyberPolicy(c).UpstreamInTok)
}

func TestReviewCyberPassthroughSuccessfulLineage(t *testing.T) {
	control, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.created","response":{"id":"resp_success"}}`)
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_success","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`)
	combo := &comboCacheAndStore{}
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
	svc.cache = combo
	svc.settingService = enabledCyberSessionTestService(combo).settingService
	observed := make(chan int, 1)
	server, _ := startPassthroughLifecycleServerWithHooks(t, control, svc, passthroughLifecycleAccount(), func(c *gin.Context) *OpenAIWSIngressHooks {
		BindCyberSessionIdentity(c, svc.PrepareCyberSessionIdentity(control, 7, 11, c, []byte(`{"input":"hello"}`), "", ""))
		return &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, _ error) { observed <- combo.lineageSets }}
	})
	defer server.Close()
	client := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = client.CloseNow() }()
	for i := 0; i < 2; i++ {
		_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
		require.NoError(t, err)
	}
	select {
	case writes := <-observed:
		require.Greater(t, writes, 0, "successful Responses IDs must be bound before next turn")
	case <-time.After(3 * time.Second):
		t.Fatal("missing AfterTurn")
	}
}
