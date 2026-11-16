//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type metaUsageRecordingRepo struct {
	AccountRepository
	updates []map[string]any
}

func (r *metaUsageRecordingRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = append(r.updates, updates)
	return nil
}

func newMetaUsageHandlerTest() (*OpenAIGatewayService, *metaUsageRecordingRepo, *gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	repo := &metaUsageRecordingRepo{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: repo}
	return svc, repo, c, rec
}

func metaUsageTestAccount() *Account {
	return &Account{ID: 9, Platform: PlatformMeta, Type: AccountTypeOAuth, Credentials: map[string]any{"api_key": "sk-muse"}}
}

func metaUsageSSE(resetAt int64) string {
	reset := strconv.FormatInt(resetAt, 10)
	subscription := "{\"tier\":\"muse-pro\",\"window\":{\"resets_at\":" + reset + ",\"used_percent\":42}}"
	created := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_x\"}}\n\n"
	usageEvent := "event: response.subscription_usage\ndata: {\"type\":\"response.subscription_usage\",\"subscription\":" + subscription + "}\n\n"
	completed := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_x\",\"object\":\"response\",\"status\":\"completed\",\"output\":[],\"subscription\":" + subscription + ",\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n"
	return created + usageEvent + completed
}

func TestHandleNonStreamingResponse_MetaSSEToJSONStripsSubscription(t *testing.T) {
	svc, repo, c, rec := newMetaUsageHandlerTest()
	body := metaUsageSSE(time.Now().Add(2 * time.Hour).Unix())
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, metaUsageTestAccount(), "muse-spark-1.3", "muse-spark-1.3")
	require.NoError(t, err)

	payload := rec.Body.String()
	require.NotContains(t, payload, "subscription")
	require.NotContains(t, payload, "muse-pro")
	require.Equal(t, float64(3), gjson.Get(payload, "usage.input_tokens").Float())
	require.Equal(t, "resp_x", gjson.Get(payload, "id").String())
	require.Len(t, repo.updates, 1)
	require.Equal(t, "muse-pro", repo.updates[0]["meta_usage_tier"])
	require.InDelta(t, 0.42, repo.updates[0]["meta_usage_window_utilization"], 0.0001)
}

func TestHandleNonStreamingResponse_MetaRawSSEDropsUsageEvent(t *testing.T) {
	svc, repo, c, rec := newMetaUsageHandlerTest()
	resetAt := time.Now().Add(3 * time.Hour).Unix()
	subscription := "{\"tier\":\"muse-pro\",\"weekly\":{\"resets_at\":" + strconv.FormatInt(resetAt, 10) + ",\"used_percent\":80}}"
	body := "event: response.subscription_usage\ndata: {\"type\":\"response.subscription_usage\",\"subscription\":" + subscription + "}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, metaUsageTestAccount(), "muse-spark-1.3", "muse-spark-1.3")
	require.NoError(t, err)

	payload := rec.Body.String()
	require.NotContains(t, payload, "subscription")
	require.NotContains(t, payload, "muse-pro")
	require.Len(t, repo.updates, 1)
	require.InDelta(t, 0.8, repo.updates[0]["meta_usage_weekly_utilization"], 0.0001)
}
