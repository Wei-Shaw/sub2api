package service

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type bedrockCompatHTTPUpstreamRecorder struct {
	lastReq  *http.Request
	lastBody []byte
	resp     *http.Response
}

func (u *bedrockCompatHTTPUpstreamRecorder) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.lastReq = req
	if req != nil && req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		u.lastBody = b
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(b))
	}
	return u.resp, nil
}

func (u *bedrockCompatHTTPUpstreamRecorder) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestGatewayService_ForwardAsChatCompletions_BedrockAPIKeyUsesBedrockRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &bedrockCompatHTTPUpstreamRecorder{
		resp: newBedrockEventStreamResponse(
			`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"us.anthropic.claude-sonnet-4-6","stop_reason":"","usage":{"input_tokens":3,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
			`{"type":"message_stop"}`,
		),
	}
	svc := &GatewayService{httpUpstream: upstream}
	account := newBedrockAPIKeyCompatAccountForTest()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	body := []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"max_tokens":128}`)
	result, err := svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Contains(t, upstream.lastReq.URL.String(), "https://bedrock-runtime.us-east-1.amazonaws.com/model/us.anthropic.claude-sonnet-4-6/invoke-with-response-stream")
	require.Equal(t, "Bearer bedrock-test-key", upstream.lastReq.Header.Get("Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("x-api-key"))
	require.Equal(t, "claude-sonnet-4-6", gjson.Get(rec.Body.String(), "model").String())
	require.Equal(t, "assistant", gjson.Get(rec.Body.String(), "choices.0.message.role").String())
	require.Equal(t, "hello", gjson.Get(rec.Body.String(), "choices.0.message.content").String())
	require.Equal(t, int64(3), gjson.Get(rec.Body.String(), "usage.prompt_tokens").Int())
	require.Equal(t, int64(1), gjson.Get(rec.Body.String(), "usage.completion_tokens").Int())
}

func TestGatewayService_ForwardAsResponses_BedrockAPIKeyUsesBedrockRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &bedrockCompatHTTPUpstreamRecorder{
		resp: newBedrockEventStreamResponse(
			`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"us.anthropic.claude-sonnet-4-6","stop_reason":"","usage":{"input_tokens":3,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
			`{"type":"message_stop"}`,
		),
	}
	svc := &GatewayService{httpUpstream: upstream}
	account := newBedrockAPIKeyCompatAccountForTest()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	body := []byte(`{"model":"claude-sonnet-4-6","input":"hi","max_output_tokens":128}`)
	result, err := svc.ForwardAsResponses(c.Request.Context(), c, account, body, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Contains(t, upstream.lastReq.URL.String(), "https://bedrock-runtime.us-east-1.amazonaws.com/model/us.anthropic.claude-sonnet-4-6/invoke-with-response-stream")
	require.Equal(t, "Bearer bedrock-test-key", upstream.lastReq.Header.Get("Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("x-api-key"))
	require.Contains(t, rec.Body.String(), `"model":"claude-sonnet-4-6"`)
	require.Contains(t, rec.Body.String(), `"text":"hello"`)
}

func newBedrockAPIKeyCompatAccountForTest() *Account {
	return &Account{
		ID:          301,
		Name:        "bedrock-compat-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeBedrock,
		Concurrency: 1,
		Credentials: map[string]any{
			"auth_mode":  "apikey",
			"api_key":    "bedrock-test-key",
			"aws_region": "us-east-1",
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func newBedrockEventStreamResponse(events ...string) *http.Response {
	var body bytes.Buffer
	for _, event := range events {
		chunkPayload := `{"bytes":"` + base64.StdEncoding.EncodeToString([]byte(event)) + `"}`
		_, _ = body.Write(buildBedrockEventStreamFrameForCompatTest("chunk", []byte(chunkPayload)))
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"x-amzn-requestid": []string{"bedrock-request-id"},
		},
		Body: io.NopCloser(bytes.NewReader(body.Bytes())),
	}
}

func buildBedrockEventStreamFrameForCompatTest(eventType string, payload []byte) []byte {
	crc32IeeeTab := crc32.MakeTable(crc32.IEEE)
	var headersBuf bytes.Buffer
	_ = headersBuf.WriteByte(byte(len(":event-type")))
	_, _ = headersBuf.WriteString(":event-type")
	_ = headersBuf.WriteByte(7)
	_ = binary.Write(&headersBuf, binary.BigEndian, uint16(len(eventType)))
	_, _ = headersBuf.WriteString(eventType)
	_ = headersBuf.WriteByte(byte(len(":message-type")))
	_, _ = headersBuf.WriteString(":message-type")
	_ = headersBuf.WriteByte(7)
	_ = binary.Write(&headersBuf, binary.BigEndian, uint16(len("event")))
	_, _ = headersBuf.WriteString("event")

	headers := headersBuf.Bytes()
	headersLen := uint32(len(headers))
	totalLen := uint32(12 + len(headers) + len(payload) + 4)

	var preludeBuf bytes.Buffer
	_ = binary.Write(&preludeBuf, binary.BigEndian, totalLen)
	_ = binary.Write(&preludeBuf, binary.BigEndian, headersLen)
	preludeBytes := preludeBuf.Bytes()
	preludeCRC := crc32.Checksum(preludeBytes, crc32IeeeTab)

	var frame bytes.Buffer
	_, _ = frame.Write(preludeBytes)
	_ = binary.Write(&frame, binary.BigEndian, preludeCRC)
	_, _ = frame.Write(headers)
	_, _ = frame.Write(payload)
	messageCRC := crc32.Checksum(frame.Bytes(), crc32IeeeTab)
	_ = binary.Write(&frame, binary.BigEndian, messageCRC)
	return frame.Bytes()
}
