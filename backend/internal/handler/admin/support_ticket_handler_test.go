//go:build unit

// Package admin — support_ticket_handler_test.go
//
// admin 端工单 handler 单测。覆盖：
//
//   - 列表过滤参数解析（status / priority / category / user_id / q）
//   - user_id 非法（非数字 / 0 / 负数）→ 400
//   - GET 详情成功路径返回 chat_context + replies
//   - AppendReply 触发 open → in_progress 转移
//   - AppendReply 不卡 feature_enabled（spec 7.2：admin 路径始终可用）
//   - AppendReply 已关闭工单 → 409
//   - PATCH 无字段 → 400；closed → 非 closed → 409；非法 priority → gin oneof 拦截 400
//   - PATCH 成功路径返回更新后的列表元素 DTO（不含 chat_context）
//
// 与用户端 handler 测试一致，使用真实 SupportTicketService + 内存 repo + settings stub。
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// stubs（与 handler 包同名 stub 隔离；行为对齐）
// ----------------------------------------------------------------------------

type adminStSettingsStub struct {
	enabled         bool
	categories      []string
	defaultPriority string
}

func (s *adminStSettingsStub) GetSupportTicketRuntime(context.Context) service.SupportTicketRuntime {
	return service.SupportTicketRuntime{
		Enabled:         s.enabled,
		Categories:      s.categories,
		DefaultPriority: s.defaultPriority,
	}
}

func newAdminStSettings(enabled bool) *adminStSettingsStub {
	return &adminStSettingsStub{
		enabled:         enabled,
		categories:      []string{"充值", "账号", "API", "Bug", "其他"},
		defaultPriority: service.SupportTicketPriorityNormal,
	}
}

type adminStRepoStub struct {
	tickets []*service.SupportTicket
	replies []*service.SupportTicketReply
	tid     atomic.Int64
	rid     atomic.Int64
	// lastListFilters 记录最近一次 ListAdmin 的入参，便于断言 handler 是否正确解析 query。
	lastListFilters service.SupportTicketListFilters
}

func newAdminStRepoStub() *adminStRepoStub {
	return &adminStRepoStub{}
}

func (s *adminStRepoStub) Create(_ context.Context, t *service.SupportTicket) error {
	t.ID = s.tid.Add(1)
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}
	if t.Status == "" {
		t.Status = service.SupportTicketStatusOpen
	}
	cp := *t
	s.tickets = append(s.tickets, &cp)
	return nil
}

func (s *adminStRepoStub) GetByID(_ context.Context, id int64) (*service.SupportTicket, error) {
	for _, t := range s.tickets {
		if t.ID == id {
			cp := *t
			return &cp, nil
		}
	}
	return nil, service.ErrSupportTicketNotFound
}

func (s *adminStRepoStub) ListByUser(
	_ context.Context,
	userID int64,
	params pagination.PaginationParams,
) ([]service.SupportTicket, *pagination.PaginationResult, error) {
	out := make([]service.SupportTicket, 0)
	for _, t := range s.tickets {
		if t.UserID == userID {
			cp := *t
			cp.ChatContext = nil
			out = append(out, cp)
		}
	}
	return out, &pagination.PaginationResult{
		Total: int64(len(out)), Page: params.Page, PageSize: params.Limit(),
	}, nil
}

func (s *adminStRepoStub) ListAdmin(
	_ context.Context,
	filters service.SupportTicketListFilters,
	params pagination.PaginationParams,
) ([]service.SupportTicket, *pagination.PaginationResult, error) {
	s.lastListFilters = filters
	out := make([]service.SupportTicket, 0)
	for _, t := range s.tickets {
		if filters.UserID != nil && t.UserID != *filters.UserID {
			continue
		}
		if filters.Status != "" && t.Status != filters.Status {
			continue
		}
		if filters.Priority != "" && t.Priority != filters.Priority {
			continue
		}
		if filters.Category != "" && t.Category != filters.Category {
			continue
		}
		if kw := strings.TrimSpace(filters.Search); kw != "" {
			lc := strings.ToLower(kw)
			if !strings.Contains(strings.ToLower(t.Title), lc) &&
				!strings.Contains(strings.ToLower(t.Content), lc) {
				continue
			}
		}
		cp := *t
		cp.ChatContext = nil
		out = append(out, cp)
	}
	return out, &pagination.PaginationResult{
		Total: int64(len(out)), Page: params.Page, PageSize: params.Limit(),
	}, nil
}

func (s *adminStRepoStub) UpdateFields(_ context.Context, id int64, patch service.SupportTicketPatch) error {
	for _, t := range s.tickets {
		if t.ID == id {
			if patch.Status != nil {
				t.Status = *patch.Status
			}
			if patch.Priority != nil {
				t.Priority = *patch.Priority
			}
			if patch.Category != nil {
				t.Category = *patch.Category
			}
			if patch.ClosedAt != nil {
				cp := *patch.ClosedAt
				t.ClosedAt = &cp
			}
			t.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return service.ErrSupportTicketNotFound
}

func (s *adminStRepoStub) AppendReply(_ context.Context, reply *service.SupportTicketReply) error {
	reply.ID = s.rid.Add(1)
	reply.CreatedAt = time.Now().UTC()
	cp := *reply
	s.replies = append(s.replies, &cp)
	return nil
}

func (s *adminStRepoStub) ListReplies(_ context.Context, ticketID int64) ([]service.SupportTicketReply, error) {
	out := make([]service.SupportTicketReply, 0)
	for _, r := range s.replies {
		if r.TicketID == ticketID {
			out = append(out, *r)
		}
	}
	return out, nil
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func newAdminSupportTicketHandlerForTest(t *testing.T, enabled bool) (
	*SupportTicketHandler,
	*adminStRepoStub,
	*adminStSettingsStub,
) {
	t.Helper()
	repo := newAdminStRepoStub()
	settings := newAdminStSettings(enabled)
	svc := service.NewSupportTicketService(repo, settings, nil)
	return NewSupportTicketHandler(svc), repo, settings
}

func makeAdminJSONContext(
	t *testing.T,
	method, url string,
	body any,
	pathParams gin.Params,
	authedUserID int64,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, url, reader)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	if authedUserID > 0 {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: authedUserID})
	}
	if pathParams != nil {
		c.Params = pathParams
	}
	return c, w
}

type adminEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decodeAdminEnvelope(t *testing.T, w *httptest.ResponseRecorder) adminEnvelope {
	t.Helper()
	var env adminEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	return env
}

// ----------------------------------------------------------------------------
// List
// ----------------------------------------------------------------------------

func TestAdminSupportTicketHandler_List_FiltersParsed(t *testing.T) {
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, true)

	// 准备 3 条工单：不同 user_id / status / priority / category，验证过滤命中。
	cc := "ctx"
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 1, Title: "需要充值帮助", Content: "找不到入口", Category: "充值",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityHigh, ChatContext: &cc,
	}))
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 2, Title: "无关", Content: "无关", Category: "账号",
		Status: service.SupportTicketStatusClosed, Priority: service.SupportTicketPriorityLow,
	}))
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 1, Title: "API 报错", Content: "500", Category: "API",
		Status: service.SupportTicketStatusInProgress, Priority: service.SupportTicketPriorityNormal,
	}))

	// query: user_id=1&status=open&priority=high&category=充值&q=充值
	c, w := makeAdminJSONContext(t, http.MethodGet,
		"/api/v1/admin/support/tickets?user_id=1&status=open&priority=high&category=%E5%85%85%E5%80%BC&q=%E5%85%85%E5%80%BC",
		nil, nil, 100)
	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeAdminEnvelope(t, w)
	var paged struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &paged))
	require.EqualValues(t, 1, paged.Total)
	require.Len(t, paged.Items, 1)
	require.EqualValues(t, 1, paged.Items[0]["id"])

	// 验证 handler 把 query 正确翻译成 service.SupportTicketListFilters。
	require.NotNil(t, repo.lastListFilters.UserID)
	require.Equal(t, int64(1), *repo.lastListFilters.UserID)
	require.Equal(t, "open", repo.lastListFilters.Status)
	require.Equal(t, "high", repo.lastListFilters.Priority)
	require.Equal(t, "充值", repo.lastListFilters.Category)
	require.Equal(t, "充值", repo.lastListFilters.Search)

	// 列表 DTO 不含 chat_context。
	_, exists := paged.Items[0]["chat_context"]
	require.Falsef(t, exists, "admin list DTO must not expose chat_context")
}

func TestAdminSupportTicketHandler_List_BadUserID_400(t *testing.T) {
	h, _, _ := newAdminSupportTicketHandlerForTest(t, true)
	for _, raw := range []string{"abc", "0", "-3"} {
		c, w := makeAdminJSONContext(t, http.MethodGet,
			"/api/v1/admin/support/tickets?user_id="+raw, nil, nil, 100)
		h.List(c)
		require.Equalf(t, http.StatusBadRequest, w.Code, "user_id=%q should yield 400", raw)
	}
}

func TestAdminSupportTicketHandler_List_LongSearchTruncated(t *testing.T) {
	// q 超过 200 字符时 handler 截断到 200。这里通过直接检查 lastListFilters.Search 长度。
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, true)
	long := strings.Repeat("a", 250)
	c, w := makeAdminJSONContext(t, http.MethodGet,
		"/api/v1/admin/support/tickets?q="+long, nil, nil, 100)
	h.List(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, repo.lastListFilters.Search, 200)
}

// ----------------------------------------------------------------------------
// Get
// ----------------------------------------------------------------------------

func TestAdminSupportTicketHandler_Get_HappyPath(t *testing.T) {
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, true)
	cc := "secret-ctx"
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 5, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityNormal, ChatContext: &cc,
	}))
	c, w := makeAdminJSONContext(t, http.MethodGet, "/api/v1/admin/support/tickets/1", nil,
		gin.Params{{Key: "id", Value: "1"}}, 100)
	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeAdminEnvelope(t, w)
	var detail map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &detail))
	require.Equal(t, "secret-ctx", detail["chat_context"])
}

func TestAdminSupportTicketHandler_Get_NotFound_404(t *testing.T) {
	h, _, _ := newAdminSupportTicketHandlerForTest(t, true)
	c, w := makeAdminJSONContext(t, http.MethodGet, "/api/v1/admin/support/tickets/999", nil,
		gin.Params{{Key: "id", Value: "999"}}, 100)
	h.Get(c)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminSupportTicketHandler_Get_BadID_400(t *testing.T) {
	h, _, _ := newAdminSupportTicketHandlerForTest(t, true)
	c, w := makeAdminJSONContext(t, http.MethodGet, "/api/v1/admin/support/tickets/abc", nil,
		gin.Params{{Key: "id", Value: "abc"}}, 100)
	h.Get(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// ----------------------------------------------------------------------------
// AppendReply
// ----------------------------------------------------------------------------

func TestAdminSupportTicketHandler_AppendReply_TransitionsOpenToInProgress(t *testing.T) {
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, true)
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 5, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityNormal,
	}))

	c, w := makeAdminJSONContext(t, http.MethodPost,
		"/api/v1/admin/support/tickets/1/replies",
		map[string]any{"content": "正在处理"},
		gin.Params{{Key: "id", Value: "1"}}, 100 /* admin user_id */)
	h.AppendReply(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeAdminEnvelope(t, w)
	var reply map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &reply))
	require.Equal(t, true, reply["is_admin"])
	require.EqualValues(t, 100, reply["author_id"])

	// open → in_progress 跃迁
	require.Equal(t, service.SupportTicketStatusInProgress, repo.tickets[0].Status)
}

func TestAdminSupportTicketHandler_AppendReply_FeatureDisabledStillWorks(t *testing.T) {
	// admin 路径不卡 feature_enabled（spec 7.2）。
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, false)
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 5, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityNormal,
	}))

	c, w := makeAdminJSONContext(t, http.MethodPost, "/api/v1/admin/support/tickets/1/replies",
		map[string]any{"content": "x"},
		gin.Params{{Key: "id", Value: "1"}}, 100)
	h.AppendReply(c)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestAdminSupportTicketHandler_AppendReply_ClosedTicket_409(t *testing.T) {
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, true)
	cl := time.Now().UTC()
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 5, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusClosed, Priority: service.SupportTicketPriorityNormal, ClosedAt: &cl,
	}))
	c, w := makeAdminJSONContext(t, http.MethodPost, "/api/v1/admin/support/tickets/1/replies",
		map[string]any{"content": "x"}, gin.Params{{Key: "id", Value: "1"}}, 100)
	h.AppendReply(c)
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestAdminSupportTicketHandler_AppendReply_Unauthenticated_401(t *testing.T) {
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, true)
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 5, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityNormal,
	}))

	c, w := makeAdminJSONContext(t, http.MethodPost, "/api/v1/admin/support/tickets/1/replies",
		map[string]any{"content": "x"}, gin.Params{{Key: "id", Value: "1"}}, 0 /* no auth */)
	h.AppendReply(c)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// ----------------------------------------------------------------------------
// Patch
// ----------------------------------------------------------------------------

func TestAdminSupportTicketHandler_Patch_NoFields_400(t *testing.T) {
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, true)
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 5, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityNormal,
	}))
	c, w := makeAdminJSONContext(t, http.MethodPatch, "/api/v1/admin/support/tickets/1",
		map[string]any{}, gin.Params{{Key: "id", Value: "1"}}, 100)
	h.Patch(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminSupportTicketHandler_Patch_ReopenClosed_409(t *testing.T) {
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, true)
	cl := time.Now().UTC()
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 5, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusClosed, Priority: service.SupportTicketPriorityNormal, ClosedAt: &cl,
	}))
	c, w := makeAdminJSONContext(t, http.MethodPatch, "/api/v1/admin/support/tickets/1",
		map[string]any{"status": "open"},
		gin.Params{{Key: "id", Value: "1"}}, 100)
	h.Patch(c)
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestAdminSupportTicketHandler_Patch_InvalidPriority_BindRejects400(t *testing.T) {
	// gin oneof binding 会在反序列化阶段直接拒绝非法 priority 值。
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, true)
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 5, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityNormal,
	}))
	c, w := makeAdminJSONContext(t, http.MethodPatch, "/api/v1/admin/support/tickets/1",
		map[string]any{"priority": "urgent"},
		gin.Params{{Key: "id", Value: "1"}}, 100)
	h.Patch(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminSupportTicketHandler_Patch_HappyPath(t *testing.T) {
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, true)
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 5, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusInProgress, Priority: service.SupportTicketPriorityNormal,
	}))

	c, w := makeAdminJSONContext(t, http.MethodPatch, "/api/v1/admin/support/tickets/1",
		map[string]any{"priority": "high", "category": "API"},
		gin.Params{{Key: "id", Value: "1"}}, 100)
	h.Patch(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeAdminEnvelope(t, w)
	var item map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &item))
	require.Equal(t, "high", item["priority"])
	require.Equal(t, "API", item["category"])
	// 列表元素 DTO 不含 chat_context（即使存在也不返回）。
	_, exists := item["chat_context"]
	require.False(t, exists)

	require.Equal(t, service.SupportTicketPriorityHigh, repo.tickets[0].Priority)
	require.Equal(t, "API", repo.tickets[0].Category)
}

func TestAdminSupportTicketHandler_Patch_CloseSetsClosedAt(t *testing.T) {
	h, repo, _ := newAdminSupportTicketHandlerForTest(t, true)
	require.NoError(t, repo.Create(context.Background(), &service.SupportTicket{
		UserID: 5, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusInProgress, Priority: service.SupportTicketPriorityNormal,
	}))
	c, w := makeAdminJSONContext(t, http.MethodPatch, "/api/v1/admin/support/tickets/1",
		map[string]any{"status": "closed"},
		gin.Params{{Key: "id", Value: "1"}}, 100)
	h.Patch(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, service.SupportTicketStatusClosed, repo.tickets[0].Status)
	require.NotNil(t, repo.tickets[0].ClosedAt)
}

func TestAdminSupportTicketHandler_Patch_BadID_400(t *testing.T) {
	h, _, _ := newAdminSupportTicketHandlerForTest(t, true)
	c, w := makeAdminJSONContext(t, http.MethodPatch, "/api/v1/admin/support/tickets/abc",
		map[string]any{"status": "closed"},
		gin.Params{{Key: "id", Value: "abc"}}, 100)
	// strconv.ParseInt 失败 / 0 / 负数 → 400（由 parseAdminTicketID 守卫）
	h.Patch(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
