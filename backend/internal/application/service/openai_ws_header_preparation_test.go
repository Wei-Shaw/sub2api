package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Exercise the real forwarding/header pipeline without network or retaining
// every benchmark write in a capture connection.
type openAIWSHeaderFixtureConn struct{}

func (*openAIWSHeaderFixtureConn) WriteJSON(context.Context, any) error { return nil }
func (*openAIWSHeaderFixtureConn) Ping(context.Context) error           { return nil }
func (*openAIWSHeaderFixtureConn) Close() error                         { return nil }
func (*openAIWSHeaderFixtureConn) ReadMessage(context.Context) ([]byte, error) {
	return []byte(`{"type":"response.completed","response":{"id":"resp_header","model":"gpt-5.1","usage":{"input_tokens":12,"output_tokens":7}}}`), nil
}

type openAIWSHeaderFixtureDialer struct{}

func (*openAIWSHeaderFixtureDialer) Dial(context.Context, string, http.Header, string) (openAIWSClientConn, int, http.Header, error) {
	return &openAIWSHeaderFixtureConn{}, http.StatusSwitchingProtocols, nil, nil
}

func newOpenAIWSHeaderFixture(tb testing.TB) (*OpenAIGatewayService, *Account) {
	tb.Helper()
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	svc := &OpenAIGatewayService{cfg: cfg, toolCorrector: NewCodexToolCorrector()}
	pool := svc.getOpenAIWSConnPool()
	pool.setClientDialerForTest(&openAIWSHeaderFixtureDialer{})
	tb.Cleanup(pool.Close)
	return svc, &Account{ID: 1001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
}

func TestOpenAIWSHeaderPreparationPreservesMetadataAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		svc, account := newOpenAIWSHeaderFixture(t)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set(openAIWSTurnMetadataHeader, `{"trace":"header-fixture"}`)
		payload := map[string]any{
			"model": "gpt-5.1", "stream": stream, "input": "你好",
			"client_metadata": map[string]any{responsesLiteWSMetadataKey: "true", "nonce": int64(9007199254740993)},
		}
		result, err := svc.forwardOpenAIWSV2(context.Background(), c, account, payload, "fixture-token", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, false, stream, "gpt-5.1", "gpt-5.1", time.Now(), 1, "", nil, nil, false)
		require.NoError(t, err)
		require.Equal(t, 12, result.Usage.InputTokens)
		require.Equal(t, 7, result.Usage.OutputTokens)
		staged := stagedCodexOutboundSessionBody(c)
		require.Equal(t, `{"trace":"header-fixture"}`, gjson.GetBytes(staged, "client_metadata."+openAIWSTurnMetadataHeader).String())
		require.Equal(t, "9007199254740993", gjson.GetBytes(staged, "client_metadata.nonce").Raw)
		require.Equal(t, stream, gjson.GetBytes(staged, "stream").Bool())
		require.Equal(t, "response.create", gjson.GetBytes(staged, "type").String())
	}
}

func BenchmarkOpenAIWSHeaderPreparation(b *testing.B) {
	gin.SetMode(gin.TestMode)
	svc, account := newOpenAIWSHeaderFixture(b)
	payload := map[string]any{"model": "gpt-5.1", "input": strings.Repeat("prompt ", (256<<10)/7)}
	decision := OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		_, err := svc.forwardOpenAIWSV2(context.Background(), c, account, payload, "fixture-token", decision, false, false, "gpt-5.1", "gpt-5.1", time.Now(), 1, "", nil, nil, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}
