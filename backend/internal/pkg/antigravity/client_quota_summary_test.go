//go:build unit

package antigravity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Client.RetrieveUserQuotaSummary - 官方 5h / weekly 配额摘要
// ---------------------------------------------------------------------------

func TestClient_RetrieveUserQuotaSummary_Success_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("请求方法不匹配: got %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1internal:retrieveUserQuotaSummary") {
			t.Errorf("URL 路径不匹配: got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("Authorization 不匹配: got %s", auth)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type 不匹配: got %s", ct)
		}
		if ua := r.Header.Get("User-Agent"); ua != GetUserAgent() {
			t.Errorf("User-Agent 不匹配: got %s, want %s", ua, GetUserAgent())
		}

		// Google One 个人账号可能没有 project_id：此时必须发送 {}，而不是 {"project":""}
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("读取请求体失败: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}
		if _, exists := payload["project"]; exists {
			t.Errorf("project 为空时不应出现在请求体中: %s", rawBody)
		}

		_, _ = io.WriteString(w, `{"groups":[{"displayName":"Gemini Models","buckets":[{"bucketId":"gemini-5h","window":"5h","remainingFraction":0.4,"resetTime":"2026-03-08T12:00:00Z"}]}]}`)
	}))
	defer server.Close()

	withMockBaseURLs(t, []string{server.URL})

	summary, err := mustNewClient(t, "").RetrieveUserQuotaSummary(context.Background(), "test-token", "", defaultFetchAvailableModelsBodyLimit)
	if err != nil {
		t.Fatalf("RetrieveUserQuotaSummary 失败: %v", err)
	}
	if len(summary.Groups) != 1 {
		t.Fatalf("groups 数量不匹配: got %d, want 1", len(summary.Groups))
	}
	group := summary.Groups[0]
	if group.DisplayName != "Gemini Models" || len(group.Buckets) != 1 {
		t.Fatalf("分组解析失败: %+v", group)
	}
	bucket := group.Buckets[0]
	if bucket.BucketID != "gemini-5h" || bucket.Window != "5h" {
		t.Fatalf("桶字段解析失败: %+v", bucket)
	}
	if bucket.RemainingFraction == nil || *bucket.RemainingFraction != 0.4 {
		t.Fatalf("remainingFraction 解析失败: %+v", bucket.RemainingFraction)
	}
	if bucket.ResetTime != "2026-03-08T12:00:00Z" {
		t.Fatalf("resetTime 解析失败: %q", bucket.ResetTime)
	}
}

func TestClient_RetrieveUserQuotaSummary_FallsBackToNextURL(t *testing.T) {
	var firstCalls int
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstCalls++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"boom"}`)
	}))
	defer first.Close()

	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body QuotaSummaryRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Project != "proj-1" {
			t.Errorf("unexpected project payload: %+v, %v", body, err)
		}
		_, _ = io.WriteString(w, `{"groups":[]}`)
	}))
	defer second.Close()

	withMockBaseURLs(t, []string{first.URL, second.URL})

	summary, err := mustNewClient(t, "").RetrieveUserQuotaSummary(context.Background(), "token", "proj-1", defaultFetchAvailableModelsBodyLimit)
	if err != nil {
		t.Fatalf("首个端点 500 时应回退到下一个端点: %v", err)
	}
	if summary == nil {
		t.Fatal("summary 不应为 nil")
	}
	if firstCalls != 1 {
		t.Fatalf("首个端点应只被调用一次: got %d", firstCalls)
	}
}

func TestClient_RetrieveUserQuotaSummary_RejectsInvalidLimitAndEmptyURLs(t *testing.T) {
	client := mustNewClient(t, "")
	if _, err := client.RetrieveUserQuotaSummaryWithURLs(context.Background(), []string{"https://example.com"}, "token", "", 0); err == nil {
		t.Fatal("非正数 body limit 必须报错")
	}
	if _, err := client.RetrieveUserQuotaSummaryWithURLs(context.Background(), nil, "token", "", defaultFetchAvailableModelsBodyLimit); err == nil {
		t.Fatal("空端点队列必须报错")
	}
}

func TestClient_FetchAvailableModels_OmitsEmptyProject(t *testing.T) {
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rawBody = string(body)
		_, _ = io.WriteString(w, `{"models":{}}`)
	}))
	defer server.Close()

	withMockBaseURLs(t, []string{server.URL})

	if _, _, err := mustNewClient(t, "").FetchAvailableModels(context.Background(), "token", "", defaultFetchAvailableModelsBodyLimit); err != nil {
		t.Fatalf("FetchAvailableModels 失败: %v", err)
	}
	if strings.Contains(rawBody, "project") {
		t.Fatalf("project 为空时不应出现在请求体中: %s", rawBody)
	}
}

func TestClient_FetchAvailableModelsWithURLs_UsesGivenOrder(t *testing.T) {
	var firstCalls, secondCalls int
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstCalls++
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"NOT_FOUND"}`)
	}))
	defer first.Close()

	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalls++
		_, _ = io.WriteString(w, `{"models":{"gemini-3-pro-high":{"quotaInfo":{"remainingFraction":1}}}}`)
	}))
	defer second.Close()

	response, _, err := mustNewClient(t, "").FetchAvailableModelsWithURLs(context.Background(), []string{first.URL, second.URL}, "token", "project", defaultFetchAvailableModelsBodyLimit)
	if err != nil {
		t.Fatalf("首个端点 404 时应回退: %v", err)
	}
	if _, ok := response.Models["gemini-3-pro-high"]; !ok {
		t.Fatalf("回退端点应返回模型目录: %+v", response.Models)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("端点调用次数不匹配: first=%d second=%d", firstCalls, secondCalls)
	}
}
