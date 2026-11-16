//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func metaFloatTimestampSSE(resetAt int64) string {
	reset := strconv.FormatInt(resetAt, 10)
	response := `{"id":"resp_m","object":"response","created_at":1794289515.0,"model":"muse-wonder-1.0-contributor","status":"%s","output":%s,"usage":{"input_tokens":9,"output_tokens":48,"total_tokens":57}}`
	message := `[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hi there","annotations":[]}]}]`
	created := strings.Replace(strings.Replace(response, "%s", "in_progress", 1), "%s", "[]", 1)
	completed := strings.Replace(strings.Replace(response, "%s", "completed", 1), "%s", message, 1)
	return "event: response.created\ndata: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":" + created + "}\n\n" +
		"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"Hi there\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":" + completed + "}\n\n" +
		"event: response.subscription_usage\ndata: {\"type\":\"response.subscription_usage\",\"subscription\":{\"tier\":\"muse-pro\",\"window\":{\"resets_at\":" + reset + ",\"used_percent\":7}}}\n\n" +
		"data: [DONE]\n\n"
}

func metaCompatUpstreamResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestMetaCompatBodyLetsChatCompletionsFindFloatTimestampTerminal(t *testing.T) {
	svc, repo, c, rec := newMetaUsageHandlerTest()
	account := metaUsageTestAccount()
	resp := metaCompatUpstreamResponse(metaFloatTimestampSSE(time.Now().Add(time.Hour).Unix()))
	svc.wrapMetaCompatResponse(context.Background(), account, resp)

	_, err := svc.handleChatBufferedStreamingResponse(resp, c, account, "muse-wonder-1.0-contributor", "muse-wonder-1.0-contributor", "muse-wonder-1.0-contributor", time.Now())
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "Hi there", gjson.Get(rec.Body.String(), "choices.0.message.content").String())
	require.NotContains(t, rec.Body.String(), "subscription")
	require.Len(t, repo.updates, 1)
	require.InDelta(t, 0.07, repo.updates[0][metaUsageWindowUtilizationKey], 0.0001)
}

func TestMetaCompatBodyLetsAnthropicMessagesFindFloatTimestampTerminal(t *testing.T) {
	svc, _, c, rec := newMetaUsageHandlerTest()
	account := metaUsageTestAccount()
	resp := metaCompatUpstreamResponse(metaFloatTimestampSSE(time.Now().Add(time.Hour).Unix()))
	svc.wrapMetaCompatResponse(context.Background(), account, resp)

	_, err := svc.handleAnthropicBufferedStreamingResponse(resp, c, account, "muse-wonder-1.0-contributor", "muse-wonder-1.0-contributor", "muse-wonder-1.0-contributor", time.Now())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "Hi there", gjson.Get(rec.Body.String(), "content.0.text").String())
}

func TestMetaCompatBodyLeavesOtherPlatformsAndErrorsUntouched(t *testing.T) {
	svc, _, _, _ := newMetaUsageHandlerTest()
	openai := metaCompatUpstreamResponse("data: {}\n\n")
	original := openai.Body
	svc.wrapMetaCompatResponse(context.Background(), &Account{Platform: PlatformOpenAI}, openai)
	require.Equal(t, original, openai.Body)
	_, wrapped := openai.Body.(*metaCompatResponseBody)
	require.False(t, wrapped)

	failed := metaCompatUpstreamResponse("{}")
	failed.StatusCode = http.StatusTooManyRequests
	original = failed.Body
	svc.wrapMetaCompatResponse(context.Background(), metaUsageTestAccount(), failed)
	_, wrapped = failed.Body.(*metaCompatResponseBody)
	require.False(t, wrapped)
}

func TestNormalizeMetaCompatTimestamps(t *testing.T) {
	out, changed := normalizeMetaCompatTimestamps([]byte(`{"type":"response.completed","response":{"created_at":1794289515.0,"temperature":1.0}}`))
	require.True(t, changed)
	require.Equal(t, `1794289515`, gjson.GetBytes(out, "response.created_at").Raw)
	require.Equal(t, `1.0`, gjson.GetBytes(out, "response.temperature").Raw)

	_, changed = normalizeMetaCompatTimestamps([]byte(`{"response":{"created_at":1794289515}}`))
	require.False(t, changed)
}
