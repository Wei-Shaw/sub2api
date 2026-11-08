package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type opsFeishuSettingRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func newOpsFeishuSettingRepo() *opsFeishuSettingRepo {
	return &opsFeishuSettingRepo{values: map[string]string{}}
}

func (r *opsFeishuSettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if value, ok := r.values[key]; ok {
		return &Setting{Key: key, Value: value}, nil
	}
	return nil, ErrSettingNotFound
}

func (r *opsFeishuSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (r *opsFeishuSettingRepo) Set(ctx context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *opsFeishuSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *opsFeishuSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = map[string]string{}
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *opsFeishuSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *opsFeishuSettingRepo) Delete(ctx context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

type opsFeishuCapture struct {
	mu     sync.Mutex
	bodies []string
	hits   int32
}

func newOpsFeishuTestServer(t *testing.T, responseBody string) (*httptest.Server, *opsFeishuCapture) {
	t.Helper()
	capture := &opsFeishuCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capture.mu.Lock()
		capture.bodies = append(capture.bodies, string(raw))
		capture.mu.Unlock()
		atomic.AddInt32(&capture.hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(server.Close)
	return server, capture
}

func (c *opsFeishuCapture) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.bodies))
	copy(out, c.bodies)
	return out
}

func opsFeishuTestEntry(status int, severity, phase, errType, model, message string, at time.Time) *OpsInsertErrorLogInput {
	upstreamStatus := status
	upstreamMessage := message
	return &OpsInsertErrorLogInput{
		RequestID:            "req-" + model,
		Platform:             "openai",
		Model:                model,
		RequestedModel:       model,
		InboundEndpoint:      "/v1/responses",
		UpstreamEndpoint:     "/v1/responses",
		ErrorPhase:           phase,
		ErrorType:            errType,
		Severity:             severity,
		StatusCode:           status,
		ErrorMessage:         message,
		UpstreamStatusCode:   &upstreamStatus,
		UpstreamErrorMessage: &upstreamMessage,
		CreatedAt:            at,
	}
}

func TestOpsFeishuNotificationConfigDefaultsAndValidation(t *testing.T) {
	repo := newOpsFeishuSettingRepo()
	svc := &OpsService{settingRepo: repo}
	ctx := context.Background()

	cfg, err := svc.GetFeishuNotificationConfig(ctx)
	require.NoError(t, err)
	require.False(t, cfg.Enabled, "默认不开启，避免没配地址就空转")
	require.Equal(t, "P1", cfg.MinSeverity)
	require.Equal(t, 60, cfg.BatchingWindowSeconds)
	require.Equal(t, 60, cfg.RateLimitPerHour)
	require.Contains(t, repo.values, SettingKeyOpsFeishuNotificationConfig, "首次读取要把默认值写回 settings")

	enabled := true
	_, err = svc.UpdateFeishuNotificationConfig(ctx, &OpsFeishuNotificationConfigUpdateRequest{Enabled: &enabled})
	require.Error(t, err, "开了开关却没填 Webhook 必须拒绝")

	badURL := "open.feishu.cn/open-apis/bot/v2/hook/xxx"
	_, err = svc.UpdateFeishuNotificationConfig(ctx, &OpsFeishuNotificationConfigUpdateRequest{WebhookURL: &badURL})
	require.Error(t, err, "地址必须以 http(s):// 开头")

	badSeverity := "P9"
	_, err = svc.UpdateFeishuNotificationConfig(ctx, &OpsFeishuNotificationConfigUpdateRequest{MinSeverity: &badSeverity})
	require.Error(t, err)

	goodURL := "https://open.feishu.cn/open-apis/bot/v2/hook/test-token"
	severity := "P2"
	updated, err := svc.UpdateFeishuNotificationConfig(ctx, &OpsFeishuNotificationConfigUpdateRequest{
		Enabled:     &enabled,
		WebhookURL:  &goodURL,
		MinSeverity: &severity,
	})
	require.NoError(t, err)
	require.True(t, updated.Enabled)
	require.Equal(t, goodURL, updated.WebhookURL)
	require.Equal(t, "P2", updated.MinSeverity)

	reloaded, err := svc.GetFeishuNotificationConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, goodURL, reloaded.WebhookURL)
}

func TestSendFeishuTestNotificationRequiresWebhook(t *testing.T) {
	repo := newOpsFeishuSettingRepo()
	svc := &OpsService{settingRepo: repo}
	require.Error(t, svc.SendFeishuTestNotification(context.Background()))
}

func TestBuildFeishuErrorTextIncludesDiagnostics(t *testing.T) {
	userID := int64(42)
	entry := opsFeishuTestEntry(503, "P1", "upstream", "upstream_error", "gpt-6-luna",
		"Service temporarily unavailable", time.Date(2026, 11, 5, 21, 43, 54, 0, time.Local))
	entry.UserID = &userID

	text := buildFeishuErrorText(entry, 12, entry.CreatedAt)
	require.Contains(t, text, "【辰数告警】请求错误")
	require.Contains(t, text, "503")
	require.Contains(t, text, "P1")
	require.Contains(t, text, "upstream")
	require.Contains(t, text, "gpt-6-luna")
	require.Contains(t, text, "入站 /v1/responses")
	require.Contains(t, text, "Service temporarily unavailable")
	require.Contains(t, text, "用户ID：42")
	require.Contains(t, text, "次数：12")
}

func TestOpsFeishuNotifierAggregatesDuplicateErrors(t *testing.T) {
	server, capture := newOpsFeishuTestServer(t, "{\"code\":0,\"msg\":\"success\"}")

	notifier := newOpsFeishuNotifier(nil)
	notifier.setConfig(&OpsFeishuNotificationConfig{
		Enabled:               true,
		WebhookURL:            server.URL,
		MinSeverity:           "P1",
		BatchingWindowSeconds: 60,
		RateLimitPerHour:      0,
	})

	now := time.Now()
	entries := []*OpsInsertErrorLogInput{
		opsFeishuTestEntry(503, "P1", "upstream", "upstream_error", "gpt-6-luna", "Service temporarily unavailable", now),
		opsFeishuTestEntry(503, "P1", "upstream", "upstream_error", "gpt-6-luna", "Service temporarily unavailable", now.Add(time.Second)),
		opsFeishuTestEntry(503, "P1", "upstream", "upstream_error", "gpt-6-luna", "Service temporarily unavailable", now.Add(2*time.Second)),
		opsFeishuTestEntry(500, "P1", "internal", "api_error", "gpt-6-luna", "internal error", now.Add(3*time.Second)),
	}
	notifier.enqueue(context.Background(), entries, now)

	require.Empty(t, notifier.flush(now.Add(30*time.Second)), "窗口没到不能发")

	sent := notifier.flush(now.Add(61 * time.Second))
	require.Len(t, sent, 2, "3 条相同错误合并成 1 条，另 1 条不同错误单独发")

	joined := strings.Join(sent, "\n---\n")
	require.Contains(t, joined, "次数：3")
	require.Equal(t, int32(2), atomic.LoadInt32(&capture.hits))

	bodies := capture.snapshot()
	require.Len(t, bodies, 2)
	var payload struct {
		MsgType string `json:"msg_type"`
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal([]byte(bodies[0]), &payload))
	require.Equal(t, "text", payload.MsgType)
	require.Contains(t, payload.Content.Text, "gpt-6-luna")
	require.NotContains(t, payload.Content.Text, "sk-")
}

func TestOpsFeishuNotifierFiltersSeverityAndRateLimit(t *testing.T) {
	server, capture := newOpsFeishuTestServer(t, "{\"code\":0}")

	notifier := newOpsFeishuNotifier(nil)
	notifier.setConfig(&OpsFeishuNotificationConfig{
		Enabled:               true,
		WebhookURL:            server.URL,
		MinSeverity:           "P1",
		BatchingWindowSeconds: 60,
		RateLimitPerHour:      1,
	})

	now := time.Now()
	notifier.enqueue(context.Background(), []*OpsInsertErrorLogInput{
		opsFeishuTestEntry(400, "P3", "request", "invalid_request_error", "gpt-6-luna", "bad request", now),
		opsFeishuTestEntry(503, "P1", "upstream", "upstream_error", "gpt-6-luna", "boom-a", now),
		opsFeishuTestEntry(503, "P1", "upstream", "upstream_error", "gpt-6-luna", "boom-b", now),
	}, now)

	sent := notifier.flush(now.Add(61 * time.Second))
	require.Len(t, sent, 1, "P3 被级别过滤掉；两条 P1 里每小时只放行 1 条")
	require.Equal(t, int32(1), atomic.LoadInt32(&capture.hits))
}

// TestSendFeishuWebhookTextLiveSmoke 是可选的真实链路冒烟：默认跳过，
// 只有显式给环境变量时才往真群发一条，避免把 Webhook 写进仓库。
//
//	$env:OPS_FEISHU_TEST_WEBHOOK="https://open.feishu.cn/open-apis/bot/v2/hook/..."
//	go test ./internal/service/ -run LiveSmoke -count=1 -v
func TestSendFeishuWebhookTextLiveSmoke(t *testing.T) {
	webhookURL := strings.TrimSpace(os.Getenv("OPS_FEISHU_TEST_WEBHOOK"))
	if webhookURL == "" {
		t.Skip("set OPS_FEISHU_TEST_WEBHOOK to run the live Feishu webhook smoke test")
	}
	require.NotContains(t, webhookURL, " ", "webhook url must not contain spaces")
	text := "【辰数告警】飞书通知测试\n时间：" + time.Now().Format("2006-01-02 15:04:05") +
		"\n说明：这是本地跑通链路时发的一条测试消息。"
	require.NoError(t, sendFeishuWebhookText(context.Background(), webhookURL, text))
}

func TestSendFeishuWebhookTextDetectsBusinessError(t *testing.T) {
	server, _ := newOpsFeishuTestServer(t, "{\"code\":9499,\"msg\":\"param invalid\"}")
	err := sendFeishuWebhookText(context.Background(), server.URL, "hello")
	require.Error(t, err, "HTTP 200 但 code 非 0 仍然是失败")
	require.Contains(t, err.Error(), "9499")
}

func TestRecordErrorBatchPushesToFeishu(t *testing.T) {
	server, capture := newOpsFeishuTestServer(t, "{\"code\":0}")

	settingRepo := newOpsFeishuSettingRepo()
	settingRepo.values[SettingKeyOpsFeishuNotificationConfig] = `{"enabled":true,"webhook_url":"` + server.URL + `","min_severity":"P1","rate_limit_per_hour":0,"batching_window_seconds":0}`

	var inserted int64
	opsRepo := &opsRepoMock{
		InsertErrorLogFn: func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
			atomic.AddInt64(&inserted, 1)
			return 1, nil
		},
	}

	svc := &OpsService{opsRepo: opsRepo, settingRepo: settingRepo}
	svc.feishuNotifier = newOpsFeishuNotifier(svc.GetFeishuNotificationConfig)

	entry := opsFeishuTestEntry(503, "P1", "upstream", "upstream_error", "gpt-6-luna", "Service temporarily unavailable", time.Now())
	err := svc.RecordErrorBatch(context.Background(), []*OpsInsertErrorLogInput{entry})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&capture.hits) == 1
	}, 3*time.Second, 20*time.Millisecond, "落库后应异步推送到飞书")
	require.Equal(t, int64(1), atomic.LoadInt64(&inserted))

	body := capture.snapshot()[0]
	require.Contains(t, body, "gpt-6-luna")
	require.Contains(t, body, "503")
}
