//go:build unit

package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type ccPingResponseWriter struct {
	*httptest.ResponseRecorder
	flushed           chan string
	failPing          bool
	failed            bool
	writesAfterError  int
	flushesAfterError int
}

func (writer *ccPingResponseWriter) Write(data []byte) (int, error) {
	if writer.failed {
		writer.writesAfterError++
	}
	if writer.failPing && string(data) == ": ping\n\n" {
		writer.failed = true
		return 0, errors.New("heartbeat write failed")
	}
	return writer.ResponseRecorder.Write(data)
}

func (writer *ccPingResponseWriter) Flush() {
	if writer.failed {
		writer.flushesAfterError++
	}
	writer.ResponseRecorder.Flush()
	if writer.flushed != nil {
		writer.flushed <- writer.Body.String()
	}
}

func TestHandleCCStreamingFromAnthropic_PingFlushesImmediately(t *testing.T) {
	writer := &ccPingResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		flushed:          make(chan string, 32),
	}
	context, _ := gin.CreateTestContext(writer)
	reader, upstream := io.Pipe()
	type outcome struct {
		result *ForwardResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := (&GatewayService{}).handleCCStreamingFromAnthropic(
			&http.Response{Body: reader}, context, "gpt-5", "claude-sonnet-4.5", nil, time.Now(),
		)
		finished <- outcome{result: result, err: err}
	}()
	t.Cleanup(func() {
		_ = upstream.Close()
		_ = reader.Close()
	})

	awaitFlush := func() string {
		t.Helper()
		select {
		case body := <-writer.flushed:
			return body
		case <-time.After(5 * time.Second):
			t.Fatal("event did not flush before the next upstream event or EOF")
			return ""
		}
	}
	var body string
	send := func(eventType, payload string) {
		t.Helper()
		_, err := io.WriteString(upstream, "event: "+eventType+"\ndata: "+payload+"\n\n")
		require.NoError(t, err)
		body = awaitFlush()
	}
	ping := func() {
		t.Helper()
		previous := body
		send("ping", `{"type":"ping"}`)
		require.Equal(t, previous+": ping\n\n", body)
	}

	ping()
	ping()
	send("message_start", `{"type":"message_start","message":{"id":"msg_ping","role":"assistant","content":[],"usage":{"input_tokens":10}}}`)
	send("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	send("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`)
	require.Contains(t, body, `"content":"hello"`)
	ping()
	send("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`)
	require.Contains(t, body, `"content":" world"`)
	send("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`)
	send("message_stop", `{"type":"message_stop"}`)
	ping()
	require.NotContains(t, body, "[DONE]")
	require.NoError(t, upstream.Close())

	select {
	case got := <-finished:
		require.NoError(t, got.err)
		require.Equal(t, 10, got.result.Usage.InputTokens)
		require.Equal(t, 7, got.result.Usage.OutputTokens)
		require.NotNil(t, got.result.FirstTokenMs)
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not finish after EOF")
	}
	body = writer.Body.String()
	require.NotContains(t, body, "event:")
	require.NotContains(t, body, `"type":"ping"`)
	require.Equal(t, 4, strings.Count(body, ": ping\n\n"))
	require.Equal(t, 1, strings.Count(body, "data: [DONE]\n\n"))
	require.True(t, strings.HasSuffix(body, "data: [DONE]\n\n"))
	var content strings.Builder
	var usage *apicompat.ChatUsage
	var finishedChoice bool
	for _, frame := range strings.Split(body, "\n\n") {
		if frame == "" || frame == ": ping" || frame == "data: [DONE]" {
			continue
		}
		require.True(t, strings.HasPrefix(frame, "data: "))
		var chunk apicompat.ChatCompletionsChunk
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &chunk))
		require.Equal(t, "chat.completion.chunk", chunk.Object)
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != nil {
				content.WriteString(*choice.Delta.Content)
			}
			if choice.FinishReason != nil {
				require.Equal(t, "stop", *choice.FinishReason)
				finishedChoice = true
			}
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	require.Equal(t, "hello world", content.String())
	require.True(t, finishedChoice)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 7, usage.CompletionTokens)
	require.Equal(t, 17, usage.TotalTokens)
}

func TestHandleCCStreamingFromAnthropic_PingDoesNotSetFirstToken(t *testing.T) {
	writer := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(writer)
	result, err := (&GatewayService{}).handleCCStreamingFromAnthropic(
		&http.Response{Body: io.NopCloser(strings.NewReader("event: ping\ndata: {\"type\":\"ping\"}\n\n"))},
		context, "gpt-5", "claude-sonnet-4.5", nil, time.Now(),
	)
	require.NoError(t, err)
	require.Nil(t, result.FirstTokenMs)
	require.True(t, strings.HasPrefix(writer.Body.String(), ": ping\n\n"))
}

func TestHandleCCStreamingFromAnthropic_PingWriteFailureStopsDownstream(t *testing.T) {
	for _, withUsage := range []bool{false, true} {
		name := "before_message_start"
		if withUsage {
			name = "after_message_start"
		}
		t.Run(name, func(t *testing.T) {
			writer := &ccPingResponseWriter{ResponseRecorder: httptest.NewRecorder(), failPing: true}
			context, _ := gin.CreateTestContext(writer)
			stream := ""
			if withUsage {
				stream += "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_failure\",\"usage\":{\"input_tokens\":10}}}\n\n"
			}
			stream += "event: ping\ndata: {\"type\":\"ping\"}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
			result, err := (&GatewayService{}).handleCCStreamingFromAnthropic(
				&http.Response{Body: io.NopCloser(strings.NewReader(stream))},
				context, "gpt-5", "claude-sonnet-4.5", nil, time.Now(),
			)
			require.NoError(t, err)
			require.True(t, writer.failed)
			require.True(t, result.ClientDisconnect)
			require.Zero(t, writer.writesAfterError)
			require.Zero(t, writer.flushesAfterError)
			require.NotContains(t, writer.Body.String(), "[DONE]")
			require.Equal(t, 7, result.Usage.OutputTokens, "usage after the failed heartbeat must still be billed")
			if withUsage {
				require.Equal(t, 10, result.Usage.InputTokens)
			} else {
				require.Zero(t, result.Usage.InputTokens)
				require.Nil(t, result.FirstTokenMs)
			}
		})
	}
}
