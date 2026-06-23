package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// response.incomplete（生成超时/截断）应被识别为可重试的 502 上游错误，触发 failover。
func TestExtractImagesUpstreamError_IncompleteIsRetryable(t *testing.T) {
	body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n" +
		"data: {\"type\":\"response.incomplete\",\"response\":{\"id\":\"resp_1\",\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n"
	got := extractOpenAIImagesUpstreamError([]byte(body))
	if got == nil {
		t.Fatal("incomplete event should produce an upstream error, got nil")
	}
	if got.StatusCode != http.StatusBadGateway {
		t.Fatalf("incomplete(max_output_tokens) should be 502 retryable, got %d", got.StatusCode)
	}
	if !IsOpenAIImagesRetryableUpstreamError(got) {
		t.Fatal("incomplete(max_output_tokens) should be retryable for failover")
	}
	if got.Code != "response_incomplete" {
		t.Fatalf("unexpected code %q", got.Code)
	}
	if !strings.Contains(got.Message, "max_output_tokens") {
		t.Fatalf("message should carry reason, got %q", got.Message)
	}
}

// incomplete 因 content_filter → 400，重试无意义，不应触发 failover。
func TestExtractImagesUpstreamError_IncompleteContentFilterNotRetryable(t *testing.T) {
	body := "data: {\"type\":\"response.incomplete\",\"response\":{\"id\":\"r\",\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"content_filter\"}}}\n\n"
	got := extractOpenAIImagesUpstreamError([]byte(body))
	if got == nil {
		t.Fatal("content_filter incomplete should produce error")
	}
	if got.StatusCode != http.StatusBadRequest {
		t.Fatalf("content_filter should be 400 (non-retryable), got %d", got.StatusCode)
	}
	if IsOpenAIImagesRetryableUpstreamError(got) {
		t.Fatal("content_filter must NOT be retryable")
	}
}

// 旧行为不变：error / response.failed 仍按原逻辑识别。
func TestExtractImagesUpstreamError_ErrorAndFailedUnchanged(t *testing.T) {
	errBody := "data: {\"type\":\"error\",\"error\":{\"type\":\"image_generation_user_error\",\"code\":\"moderation_blocked\",\"message\":\"rejected\"}}\n\n"
	if got := extractOpenAIImagesUpstreamError([]byte(errBody)); got == nil || got.StatusCode != http.StatusBadRequest {
		t.Fatalf("moderation_blocked should still be 400, got %+v", got)
	}
}

// 上游既无图、又无任何可识别事件时，摘要函数应提取诊断信息。
func TestSummarizeNoOutputBody_ExtractsDiagnostics(t *testing.T) {
	body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n" +
		"data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"r\",\"status\":\"in_progress\"}}\n\n"
	summary := summarizeOpenAIImagesNoOutputBody([]byte(body))
	if !strings.HasPrefix(summary, "no_image_output") {
		t.Fatalf("summary should start with marker, got %q", summary)
	}
	if !strings.Contains(summary, "last_event=response.in_progress") {
		t.Fatalf("summary should capture last event type, got %q", summary)
	}
	if !strings.Contains(summary, "status=in_progress") {
		t.Fatalf("summary should capture response status, got %q", summary)
	}
}

// 摘要应能抓到 incomplete_reason 并对超长 body 截断。
func TestSummarizeNoOutputBody_IncompleteReasonAndTruncation(t *testing.T) {
	long := strings.Repeat("x", 2000)
	body := "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"junk\":\"" + long + "\"}}\n\n"
	summary := summarizeOpenAIImagesNoOutputBody([]byte(body))
	if !strings.Contains(summary, "incomplete_reason=max_output_tokens") {
		t.Fatalf("should capture incomplete reason, got %q", summary[:120])
	}
	if !strings.Contains(summary, "truncated") {
		t.Fatalf("oversized body should be truncated, len=%d", len(summary))
	}
}

// 软失败（上游 completed 但无图，如偶发路由到 mini 模型）应返回可重试的
// UpstreamFailoverError 且优先同账号重试，而非一次性失败。
func TestImagesOAuthNonStreaming_CompletedNoImageTriggersSameAccountRetry(t *testing.T) {
	// 上游 SSE：response.completed 但 output 为空（实测的真实失败形态）。
	upstreamSSE := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_x\",\"status\":\"in_progress\",\"model\":\"gpt-5.4-mini-2026-03-17\",\"output\":[]}}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_x\",\"status\":\"completed\",\"model\":\"gpt-5.4-mini-2026-03-17\",\"output\":[],\"tool_usage\":{\"image_gen\":{\"output_tokens\":0}}}}\n\n"

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
	}

	svc := &OpenAIGatewayService{}
	_, _, _, err := svc.handleOpenAIImagesOAuthNonStreamingResponse(resp, c, "b64_json", "gpt-image-2")

	if err == nil {
		t.Fatal("completed-but-no-image should return an error")
	}
	var failoverErr *UpstreamFailoverError
	if !errors.As(err, &failoverErr) {
		t.Fatalf("expected *UpstreamFailoverError to trigger retry, got %T: %v", err, err)
	}
	if failoverErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", failoverErr.StatusCode)
	}
	if !failoverErr.RetryableOnSameAccount {
		t.Fatal("soft-failure should prefer same-account retry (probabilistic upstream failure)")
	}
}
