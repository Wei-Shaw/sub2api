package service

import (
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

	c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"b64_json": "aW1hZ2U="REDACTEDREDACTEDREDACTED)
	stop()

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	require.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
	require.True(t, rec.Flushed)
	require.True(t, json.Valid(rec.Body.Bytes()), rec.Body.String())
	require.Equal(t, "aW1hZ2U=", gjson.Get(rec.Body.String(), "data.0.b64_json").String())
	require.Greater(t, OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c), 0)
	require.Same(t, originalWriter, c.Writer)
REDACTED

func TestOpenAIImagesJSONKeepalive_DisabledIsNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	originalWriter := c.Writer

	stop := StartOpenAIImagesJSONKeepalive(c, 0)
	stop()
	c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "invalid request"REDACTEDREDACTED)

	require.Same(t, originalWriter, c.Writer)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid request", gjson.Get(rec.Body.String(), "error.message").String())
REDACTED

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
REDACTED)
	stop()

	require.True(t, wrote)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, strings.HasPrefix(rec.Body.String(), " \n"))
	require.Equal(t, "invalid size", gjson.Get(rec.Body.String(), "error.message").String())
REDACTED

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
REDACTED)

	require.True(t, wrote)
	require.Equal(t, http.StatusOK, rec.Code, "heartbeat already committed the status")
	require.True(t, json.Valid(rec.Body.Bytes()), rec.Body.String())
	require.Equal(t, "moderation_blocked", gjson.Get(rec.Body.String(), "error.code").String())
	require.Equal(t, "request rejected", gjson.Get(rec.Body.String(), "error.message").String())
REDACTED

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
REDACTED

func TestOpenAIImagesJSONKeepalive_KeepsOAuthNonStreamResponseValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	reader, writer := io.Pipe()
	go func() {
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(writer,
			"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000000,\"output\":[{\"type\":\"image_generation_call\",\"result\":\"aW1hZ2U=\",\"output_format\":\"png\"REDACTED]REDACTEDREDACTED\n\n"+
				"data: [DONE]\n\n",
		)
		_ = writer.Close()
REDACTED()

	stop := StartOpenAIImagesJSONKeepalive(c, 5*time.Millisecond)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       reader,
REDACTED
	svc := &OpenAIGatewayService{REDACTED
	_, imageCount, _, err := svc.handleOpenAIImagesOAuthNonStreamingResponse(resp, c, "b64_json", "gpt-image-2")
	stop()

REDACTED
	require.Equal(t, 1, imageCount)
	require.True(t, rec.Flushed)
	require.True(t, strings.HasPrefix(rec.Body.String(), " \n"), rec.Body.String())
	require.True(t, json.Valid(rec.Body.Bytes()), rec.Body.String())
	require.Equal(t, "aW1hZ2U=", gjson.Get(rec.Body.String(), "data.0.b64_json").String())
REDACTED

func TestOpenAIImagesJSONKeepaliveWriter_NilGuards(t *testing.T) {
	w := &openAIImagesJSONKeepaliveWriter{REDACTED
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
REDACTED)

	conn, _, err := w.Hijack()
REDACTED
	require.Nil(t, conn)
	select {
	case <-w.CloseNotify():
	default:
		t.Fatal("nil writer CloseNotify channel should be closed")
REDACTED
REDACTED

func waitForOpenAIImagesJSONKeepalive(t *testing.T, c *gin.Context) {
REDACTED
	k := openAIImagesJSONKeepaliveFromContext(c)
	require.NotNil(t, k)
	require.Eventually(t, func() bool {
		k.mu.Lock()
		defer k.mu.Unlock()
		return k.started
REDACTED, time.Second, time.Millisecond)
REDACTED
