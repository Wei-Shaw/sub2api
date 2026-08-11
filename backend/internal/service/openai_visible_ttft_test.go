package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIVisibleOutputClassification(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		eventType string
		want      bool
REDACTED{
		{name: "keepalive", data: `{"type":"keepalive"REDACTED`, want: falseREDACTED,
		{name: "created", data: `{"type":"response.created"REDACTED`, want: falseREDACTED,
		{name: "empty output item", data: `{"type":"response.output_item.added","item":{"id":"item_test","type":"reasoning","summary":[]REDACTEDREDACTED`, want: falseREDACTED,
		{name: "empty delta", data: `{"type":"response.output_text.delta","delta":""REDACTED`, want: falseREDACTED,
		{name: "text delta", data: `{"type":"response.output_text.delta","delta":"test output"REDACTED`, want: trueREDACTED,
		{name: "tool arguments", data: `{"type":"response.function_call_arguments.delta","delta":"{REDACTED"REDACTED`, want: trueREDACTED,
		{name: "partial image", data: `{"type":"response.image_generation_call.partial_image","partial_image_b64":"dGVzdA=="REDACTED`, want: trueREDACTED,
		{name: "completed image item", data: `{"type":"response.output_item.done","item":{"id":"item_test","type":"image_generation_call","result":"dGVzdA=="REDACTEDREDACTED`, want: trueREDACTED,
		{name: "empty completed", data: `{"type":"response.completed","response":{"id":"resp_test","output":[]REDACTEDREDACTED`, want: falseREDACTED,
		{name: "completed with output usage only", data: `{"type":"response.completed","response":{"id":"resp_test","usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTEDREDACTED`, want: falseREDACTED,
		{name: "completed with text", data: `{"type":"response.completed","response":{"id":"resp_test","output":[{"type":"message","content":[{"type":"output_text","text":"test output"REDACTED]REDACTED]REDACTEDREDACTED`, want: trueREDACTED,
		{name: "done marker", data: `[DONE]`, want: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIStreamDataStartsVisibleOutput(tt.data, tt.eventType))
	REDACTED)
REDACTED
REDACTED

func TestOpenAIResponsesTTFTStartsAtVisibleOutput(t *testing.T) {
	for _, passthrough := range []bool{false, trueREDACTED {
		name := "native"
		if passthrough {
			name = "passthrough"
	REDACTED
		t.Run(name, func(t *testing.T) {
			result := runSyntheticVisibleTTFTStream(t, passthrough, 120*time.Millisecond, 0,
				`{"type":"response.output_text.delta","delta":"test output"REDACTED`)
			require.NotNil(t, result.firstTokenMs)
			require.GreaterOrEqual(t, *result.firstTokenMs, 100)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIResponsesTTFTStartsAtCompletedImage(t *testing.T) {
	for _, passthrough := range []bool{false, trueREDACTED {
		name := "native"
		if passthrough {
			name = "passthrough"
	REDACTED
		t.Run(name, func(t *testing.T) {
			result := runSyntheticVisibleTTFTStream(t, passthrough, 120*time.Millisecond, 0,
				`{"type":"response.output_item.done","item":{"id":"item_test","type":"image_generation_call","result":"dGVzdA=="REDACTEDREDACTED`)
			require.NotNil(t, result.firstTokenMs)
			require.GreaterOrEqual(t, *result.firstTokenMs, 100)
	REDACTED)
REDACTED
REDACTED

func TestOpenAINativeProgressDisarmsTimeoutWithoutStartingTTFT(t *testing.T) {
	result := runSyntheticVisibleTTFTStream(t, false, 1200*time.Millisecond, 1,
		`{"type":"response.output_text.delta","delta":"test output"REDACTED`)
	require.NotNil(t, result.firstTokenMs)
	require.GreaterOrEqual(t, *result.firstTokenMs, 1100)
REDACTED

func runSyntheticVisibleTTFTStream(t *testing.T, passthrough bool, visibleDelay time.Duration, timeoutSeconds int, visibleEvent string) *openaiStreamingResult {
REDACTED
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize:                     defaultMaxLineSize,
		OpenAIFirstOutputTimeoutSeconds: timeoutSeconds,
REDACTEDREDACTEDREDACTED
	reader, writer := io.Pipe()
	writerDone := make(chan struct{REDACTED)
	go func() {
		defer close(writerDone)
		defer func() { _ = writer.Close() REDACTED()
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\"REDACTEDREDACTED\n\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"item_test\",\"type\":\"reasoning\",\"summary\":[]REDACTEDREDACTED\n\n")
		time.Sleep(visibleDelay)
		_, _ = io.WriteString(writer, "data: "+visibleEvent+"\n\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1REDACTEDREDACTEDREDACTED\n\n")
REDACTED()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: readerREDACTED
	account := &Account{ID: 1, Name: "account_test", Platform: PlatformOpenAIREDACTED
	started := time.Now()

	var result *openaiStreamingResult
	var err error
	if passthrough {
		var passthroughResult *openaiStreamingResultPassthrough
		passthroughResult, err = svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, started, "test-model", "test-model")
		if passthroughResult != nil {
			result = &openaiStreamingResult{firstTokenMs: passthroughResult.firstTokenMsREDACTED
	REDACTED
REDACTED else {
		result, err = svc.handleStreamingResponse(context.Background(), resp, c, account, started, "test-model", "test-model")
REDACTED
REDACTED
	require.NotNil(t, result)
	require.Contains(t, recorder.Body.String(), `"type":"response.output_item.added"`)
	require.Contains(t, recorder.Body.String(), visibleEvent)
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("synthetic upstream writer did not exit")
REDACTED
	return result
REDACTED
