package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIImagesJSONKeepalive_PreservesValidJSONResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	originalWriter := c.Writer

	stop := StartOpenAIImagesJSONKeepalive(c, 5*time.Millisecond)
	waitForOpenAIImagesJSONKeepalive(t, c)
	require.Equal(t, -1, OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c))

	c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"b64_json": "aW1hZ2U="}}})
	stop()

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	require.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
	require.True(t, rec.Flushed)
	require.True(t, json.Valid(rec.Body.Bytes()), rec.Body.String())
	require.Equal(t, "aW1hZ2U=", gjson.Get(rec.Body.String(), "data.0.b64_json").String())
	require.Greater(t, OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c), 0)
	require.Same(t, originalWriter, c.Writer)
}

func TestOpenAIImagesJSONKeepalive_DisabledIsNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	originalWriter := c.Writer

	stop := StartOpenAIImagesJSONKeepalive(c, 0)
	stop()
	c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "invalid request"}})

	require.Same(t, originalWriter, c.Writer)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid request", gjson.Get(rec.Body.String(), "error.message").String())
}

func TestOpenAIImagesJSONKeepalive_FastErrorPreservesStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	stop := StartOpenAIImagesJSONKeepalive(c, time.Second)
	wrote := writeOpenAIImagesUpstreamErrorResponse(c, &OpenAIImagesUpstreamError{
		StatusCode: http.StatusBadRequest,
		ErrorType:  "invalid_request_error",
		Message:    "invalid size",
	})
	stop()

	require.True(t, wrote)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, strings.HasPrefix(rec.Body.String(), " \n"))
	require.Equal(t, "invalid size", gjson.Get(rec.Body.String(), "error.message").String())
}

func TestOpenAIImagesJSONKeepalive_LateErrorRemainsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	stop := StartOpenAIImagesJSONKeepalive(c, 5*time.Millisecond)
	defer stop()
	waitForOpenAIImagesJSONKeepalive(t, c)

	wrote := writeOpenAIImagesUpstreamErrorResponse(c, &OpenAIImagesUpstreamError{
		StatusCode: http.StatusBadRequest,
		ErrorType:  "image_generation_user_error",
		Code:       "moderation_blocked",
		Message:    "request rejected",
	})

	require.True(t, wrote)
	require.Equal(t, http.StatusOK, rec.Code, "heartbeat already committed the status")
	require.True(t, json.Valid(rec.Body.Bytes()), rec.Body.String())
	require.Equal(t, "moderation_blocked", gjson.Get(rec.Body.String(), "error.code").String())
	require.Equal(t, "request rejected", gjson.Get(rec.Body.String(), "error.message").String())
}

func TestOpenAIImagesJSONKeepalive_DoesNotBlockFailoverDetection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	stop := StartOpenAIImagesJSONKeepalive(c, 5*time.Millisecond)
	waitForOpenAIImagesJSONKeepalive(t, c)

	before := OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c)
	require.Equal(t, -1, before)
	require.True(t, c.Writer.Written())
	require.Equal(t, before, OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c))
	stop()
	require.True(t, strings.TrimSpace(rec.Body.String()) == "")
}

func TestOpenAIImagesJSONKeepalive_KeepsOAuthNonStreamResponseValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	reader, writer := io.Pipe()
	go func() {
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(writer,
			"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000000,\"output\":[{\"type\":\"image_generation_call\",\"result\":\"aW1hZ2U=\",\"output_format\":\"png\"}]}}\n\n"+
				"data: [DONE]\n\n",
		)
		_ = writer.Close()
	}()

	stop := StartOpenAIImagesJSONKeepalive(c, 5*time.Millisecond)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       reader,
	}
	svc := &OpenAIGatewayService{}
	_, imageCount, _, err := svc.handleOpenAIImagesOAuthNonStreamingResponse(resp, c, "b64_json", "gpt-image-2")
	stop()

	require.NoError(t, err)
	require.Equal(t, 1, imageCount)
	require.True(t, rec.Flushed)
	require.True(t, strings.HasPrefix(rec.Body.String(), " \n"), rec.Body.String())
	require.True(t, json.Valid(rec.Body.Bytes()), rec.Body.String())
	require.Equal(t, "aW1hZ2U=", gjson.Get(rec.Body.String(), "data.0.b64_json").String())
}

func TestOpenAIImagesJSONKeepaliveWriter_NilGuards(t *testing.T) {
	w := &openAIImagesJSONKeepaliveWriter{}
	require.NotPanics(t, func() {
		require.NotNil(t, w.Header())
		_, _ = w.Write([]byte("test"))
		_, _ = w.WriteString("test")
		w.WriteHeader(http.StatusOK)
		w.WriteHeaderNow()
		w.Flush()
		require.Equal(t, 0, w.Status())
		require.Equal(t, 0, w.Size())
		require.False(t, w.Written())
		require.Nil(t, w.Pusher())
	})

	conn, _, err := w.Hijack()
	require.Error(t, err)
	require.Nil(t, conn)
	select {
	case <-w.CloseNotify():
	default:
		t.Fatal("nil writer CloseNotify channel should be closed")
	}
}

// 回归：failover 第 2+ 轮时，上一轮心跳残留的空白字节不得被误判为"已写响应"，
// 可重试上游错误必须仍转换为 UpstreamFailoverError（而非裸错误吞掉换号）。
func TestOpenAIImagesJSONKeepalive_HeartbeatBeforeForwardStillFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","response_format":"b64_json"}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{
		httpUpstream: &httpUpstreamRecorder{
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
					"X-Request-Id": []string{"req_img_heartbeat_failover"},
				},
				Body: io.NopCloser(strings.NewReader(
					"data: {\"type\":\"response.created\",\"response\":{\"created_at\":1710000021}}\n\n" +
						"data: {\"type\":\"error\",\"error\":{\"type\":\"server_error\",\"code\":\"server_error\",\"message\":\"The image service is temporarily unavailable.\"}}\n\n",
				)),
			},
		},
	}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	// 模拟上一轮 failover 已发生：心跳已提交 200 并写出空白字节。
	stop := StartOpenAIImagesJSONKeepalive(c, 5*time.Millisecond)
	defer stop()
	waitForOpenAIImagesJSONKeepalive(t, c)

	account := &Account{
		ID:       22,
		Name:     "openai-oauth-heartbeat-failover",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token-123",
		},
	}

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "temporarily unavailable")
	require.Empty(t, strings.TrimSpace(rec.Body.String()), "only heartbeat whitespace may reach the client")

	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "failover", events[0].Kind)
	require.Equal(t, account.ID, events[0].AccountID)
	require.Equal(t, http.StatusBadGateway, events[0].UpstreamStatusCode)
}

func waitForOpenAIImagesJSONKeepalive(t *testing.T, c *gin.Context) {
	t.Helper()
	k := openAIImagesJSONKeepaliveFromContext(c)
	require.NotNil(t, k)
	require.Eventually(t, func() bool {
		k.mu.Lock()
		defer k.mu.Unlock()
		return k.started
	}, time.Second, time.Millisecond)
}
