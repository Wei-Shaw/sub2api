package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReadOpenAIImagesNonStreamingResponseBodySendsJSONHeartbeat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	reader, writer := io.Pipe()
	expected := []byte(`{"data":[{"url":"https://example.com/image.png"}]}`)

	go func() {
		time.Sleep(35 * time.Millisecond)
		_, _ = writer.Write(expected)
		_ = writer.Close()
	}()

	svc := &OpenAIGatewayService{}
	body, err := svc.readOpenAIImagesNonStreamingResponseBodyWithInterval(reader, c, 10*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, expected, body)
	require.Contains(t, recorder.Body.String(), openAIImagesJSONHeartbeatPayload)
	require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	require.Equal(t, "no", recorder.Header().Get("X-Accel-Buffering"))

	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
	require.True(t, json.Valid(bytes.TrimSpace(recorder.Body.Bytes())))
}

func TestReadOpenAIImagesNonStreamingResponseBodyHeartbeatBypassesProbeBuffer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	buffered := newOpenAIImagesBufferedResponseWriter(c.Writer)
	c.Writer = buffered
	reader, writer := io.Pipe()
	expected := []byte(`{"data":[{"b64_json":"aGVsbG8="}]}`)

	go func() {
		time.Sleep(25 * time.Millisecond)
		_, _ = writer.Write(expected)
		_ = writer.Close()
	}()

	svc := &OpenAIGatewayService{}
	body, err := svc.readOpenAIImagesNonStreamingResponseBodyWithInterval(reader, c, 10*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, expected, body)
	require.False(t, buffered.Committed())
	require.NotEmpty(t, recorder.Body.String())

	_, err = buffered.Write(body)
	require.NoError(t, err)
	require.NoError(t, buffered.Commit())
	require.True(t, json.Valid(bytes.TrimSpace(recorder.Body.Bytes())))
}

func TestReadOpenAIImagesNonStreamingResponseBodyErrorAfterHeartbeatRemainsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	reader, writer := io.Pipe()

	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = writer.CloseWithError(io.ErrUnexpectedEOF)
	}()

	svc := &OpenAIGatewayService{}
	body, err := svc.readOpenAIImagesNonStreamingResponseBodyWithInterval(reader, c, 10*time.Millisecond)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Nil(t, body)
	require.Contains(t, recorder.Body.String(), openAIImagesJSONHeartbeatPayload)

	c.JSON(http.StatusBadGateway, gin.H{
		"error": gin.H{
			"message": "upstream image response failed",
			"type":    "upstream_error",
		},
	})
	require.True(t, json.Valid(bytes.TrimSpace(recorder.Body.Bytes())))
}
