package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type videoTaskAdminStub struct {
	service.UsageBillingRepository // Any accidental billing call panics.
	filter                         service.VideoTaskAdminFilter
	items                          []service.VideoTaskAdminItem
	total                          int64
	err                            error
	calls                          int
}

func (s *videoTaskAdminStub) ListVideoTasks(ctx context.Context, f service.VideoTaskAdminFilter) ([]service.VideoTaskAdminItem, int64, error) {
	s.filter = f
	s.calls++
	if _, ok := ctx.Deadline(); !ok {
		return nil, 0, errors.New("missing query deadline")
	}
	return s.items, s.total, s.err
}

func videoTaskAdminRouter(repo service.UsageBillingRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	gateway := service.NewOpenAIGatewayService(nil, nil, repo, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewUsageHandler(nil, nil, nil, nil, gateway)
	r := gin.New()
	r.GET("/api/v1/admin/video-tasks", h.VideoTasks)
	return r
}

func TestVideoTasksValidation(t *testing.T) {
	stub := &videoTaskAdminStub{}
	r := videoTaskAdminRouter(stub)
	for _, query := range []string{
		"page=0", "page=-1", "page=abc", "page=", "page=2147483648", "page=1&page=2",
		"page_size=0", "page_size=101", "page_size=-1", "page_size=1.5", "page_size=",
		"days=0", "days=2", "days=8", "days=31", "days=abc", "days=", "days=1&days=7",
		"status=pending", "status=ready", "status=unknown", "status=failed&status=completed",
		"search=a&search=b", "search=%00", "search=%FF", "search=" + strings.Repeat("a", 101),
		"search=" + url.QueryEscape(strings.Repeat("视", 101)),
	} {
		t.Run(query, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/admin/video-tasks?"+query, nil))
			require.Equal(t, 400, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), `"code":400`)
		})
	}
	require.Zero(t, stub.calls)
}

func TestVideoTasksDefaultsAndEnvelope(t *testing.T) {
	stub := &videoTaskAdminStub{}
	r := videoTaskAdminRouter(stub)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/admin/video-tasks", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
	require.JSONEq(t, `{"code":0,"message":"success","data":{"items":[],"total":0,"page":1,"page_size":20,"pages":1}}`, w.Body.String())
	require.Equal(t, service.VideoTaskAdminFilter{Page: 1, PageSize: 20, Days: 7}, stub.filter)

	stub.total = 101
	stub.items = []service.VideoTaskAdminItem{{Kind: "submission", APIKeyID: 2, State: "submission_unknown", DisplayStatus: "submission_unknown", BillingStatus: "review"}}
	literal := `%_\'视`
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/admin/video-tasks?page=2&page_size=100&days=30&status=submission_unknown&search="+url.QueryEscape(literal), nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, service.VideoTaskAdminFilter{Page: 2, PageSize: 100, Days: 30, Status: "submission_unknown", Search: literal}, stub.filter)
	var body struct {
		Data struct {
			Items    []map[string]any `json:"items"`
			Total    int              `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
			Pages    int              `json:"pages"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, 101, body.Data.Total)
	require.Equal(t, 2, body.Data.Page)
	require.Equal(t, 100, body.Data.PageSize)
	require.Equal(t, 2, body.Data.Pages)
	row := body.Data.Items[0]
	fields := strings.Fields("kind task_id user_id user_email api_key_id api_key_name account_id account_name group_id group_name model state display_status reason_code created_at updated_at completed_at next_attempt_at attempts expected_cost charged_cost usage_log_id billing_status video_duration_seconds video_resolution")
	require.Len(t, row, len(fields), "exact allowlist, no raw/internal fields")
	for _, field := range fields {
		require.Contains(t, row, field)
	}
	for _, field := range strings.Fields("user_id account_id group_id model completed_at next_attempt_at expected_cost charged_cost usage_log_id video_duration_seconds") {
		require.Nil(t, row[field], field)
	}
}

func TestVideoTasksAcceptedFiltersAndSafeErrors(t *testing.T) {
	stub := &videoTaskAdminStub{}
	r := videoTaskAdminRouter(stub)
	for _, status := range strings.Fields("in_progress delayed query_retry billing_pending completed failed submission_unknown") {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/admin/video-tasks?days=1&status="+status+"&search="+url.QueryEscape(strings.Repeat("视", 100)), nil))
		require.Equal(t, 200, w.Code)
	}
	stub.err = errors.New("private upstream https://secret.example/key?prompt=hidden")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/admin/video-tasks", nil))
	require.Equal(t, 500, w.Code)
	require.NotContains(t, w.Body.String(), "secret")
	for _, gateway := range []*service.OpenAIGatewayService{nil, {}} {
		h := NewUsageHandler(nil, nil, nil, nil, gateway)
		w = httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/admin/video-tasks", nil)
		h.VideoTasks(c)
		require.Equal(t, 503, w.Code)
	}
}
