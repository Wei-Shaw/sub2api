//go:build unit

// Package handler — support_ticket_handler_test.go
//
// 用户端工单 handler 单测。覆盖：
//
//   - 鉴权缺失返回 401
//   - 非法 JSON / 路径参数返回 400
//   - feature_enabled = false 路径走 service ErrSupportFeatureDisabled → 404
//   - 创建成功 / 分类非法 / 标题缺失 / 内容超长等业务校验由 service 抛 sentinel，
//     handler 经 ErrorFrom 翻译为 4xx
//   - 列表路径 chat_context 字段编译期不存在（DTO 层保险），即使 service 注入也
//     不会落到响应 body
//   - 关闭工单 / 已关闭工单再回复 → 409
//   - 非 owner GET 返回 404（安全约束：不区分"不存在"与"无权限"）
//
// 实现思路：在 handler 包内重建轻量 in-memory repo + settings stub，构造真实
// SupportTicketService（entClient = nil → 退化为非事务两步执行）。这是项目里
// user_handler_test.go / available_channel_handler_test.go 等已有 handler 测试的
// 通用模式。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
// stubs（与 service 包的同名 stub 隔离，但行为保持一致）
// ----------------------------------------------------------------------------

// stHandlerSettingsStub 实现 service.SupportTicketSettingsReader。
type stHandlerSettingsStub struct {
	enabled         bool
	categories      []string
	defaultPriority string
}

func (s *stHandlerSettingsStub) GetSupportTicketRuntime(context.Context) service.SupportTicketRuntime {
	return service.SupportTicketRuntime{
		Enabled:         s.enabled,
		Categories:      s.categories,
		DefaultPriority: s.defaultPriority,
	}
}

func newStHandlerSettings(enabled bool) *stHandlerSettingsStub {
	return &stHandlerSettingsStub{
		enabled:         enabled,
		categories:      []string{"充值", "账号", "API", "Bug", "其他"},
		defaultPriority: service.SupportTicketPriorityNormal,
	}
}

// stHandlerRepoStub 是 handler 测试专用内存 repo。行为与 service 测试 stub 一致。
type stHandlerRepoStub struct {
	tickets []*service.SupportTicket
	replies []*service.SupportTicketReply
	tid     atomic.Int64
	rid     atomic.Int64
}

func newStHandlerRepoStub() *stHandlerRepoStub {
	return &stHandlerRepoStub{}
}

func (s *stHandlerRepoStub) Create(_ context.Context, t *service.SupportTicket) error {
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

func (s *stHandlerRepoStub) GetByID(_ context.Context, id int64) (*service.SupportTicket, error) {
	for _, t := range s.tickets {
		if t.ID == id {
			cp := *t
			return &cp, nil
		}
	}
	return nil, service.ErrSupportTicketNotFound
}

func (s *stHandlerRepoStub) ListByUser(
	_ context.Context,
	userID int64,
	params pagination.PaginationParams,
) ([]service.SupportTicket, *pagination.PaginationResult, error) {
	out := make([]service.SupportTicket, 0)
	for _, t := range s.tickets {
		if t.UserID == userID {
			cp := *t
			cp.ChatContext = nil // list 视图不带大字段
			out = append(out, cp)
		}
	}
	return out, &pagination.PaginationResult{
		Total:    int64(len(out)),
		Page:     params.Page,
		PageSize: params.Limit(),
	}, nil
}

func (s *stHandlerRepoStub) ListAdmin(
	_ context.Context,
	filters service.SupportTicketListFilters,
	params pagination.PaginationParams,
) ([]service.SupportTicket, *pagination.PaginationResult, error) {
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
		Total:    int64(len(out)),
		Page:     params.Page,
		PageSize: params.Limit(),
	}, nil
}

func (s *stHandlerRepoStub) UpdateFields(_ context.Context, id int64, patch service.SupportTicketPatch) error {
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

func (s *stHandlerRepoStub) AppendReply(_ context.Context, reply *service.SupportTicketReply) error {
	reply.ID = s.rid.Add(1)
	reply.CreatedAt = time.Now().UTC()
	cp := *reply
	s.replies = append(s.replies, &cp)
	return nil
}

func (s *stHandlerRepoStub) ListReplies(_ context.Context, ticketID int64) ([]service.SupportTicketReply, error) {
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

// newSupportTicketHandlerForTest 装配 handler + 真实 service + 内存 repo + settings stub。
func newSupportTicketHandlerForTest(t *testing.T, enabled bool) (
	*SupportTicketHandler,
	*stHandlerRepoStub,
	*stHandlerSettingsStub,
) {
	t.Helper()
	repo := newStHandlerRepoStub()
	settings := newStHandlerSettings(enabled)
	svc := service.NewSupportTicketService(repo, settings, nil)
	return NewSupportTicketHandler(svc), repo, settings
}

// makeAuthedJSONContext 构造带 AuthSubject + JSON body 的 gin.Context；method 与 url
// 仅影响日志/c.Request 路径解析，对 handler 主流程无影响。
func makeAuthedJSONContext(
	t *testing.T,
	userID int64,
	method, url string,
	body any,
	pathParams gin.Params,
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

	if userID > 0 {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID})
	}
	if pathParams != nil {
		c.Params = pathParams
	}
	return c, w
}

// decodeEnvelope 解析项目标准响应壳。data 用 json.RawMessage 保留以便上层按需 unmarshal。
type stRespEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) stRespEnvelope {
	t.Helper()
	var env stRespEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	return env
}

// ----------------------------------------------------------------------------
// Create
// ----------------------------------------------------------------------------

func TestSupportTicketHandler_Create_HappyPath(t *testing.T) {
	h, repo, _ := newSupportTicketHandlerForTest(t, true)
	cc := "  some chat context  "
	c, w := makeAuthedJSONContext(t, 42, http.MethodPost, "/api/v1/support/tickets",
		map[string]any{
			"title":        " 登录失败 ",
			"content":      "详细描述",
			"category":     "账号",
			"chat_context": cc,
		}, nil)

	h.Create(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w)
	require.Equal(t, 0, env.Code)

	var detail map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &detail))
	require.EqualValues(t, 1, detail["id"])
	require.Equal(t, "登录失败", detail["title"])
	require.Equal(t, "账号", detail["category"])
	require.Equal(t, "open", detail["status"])
	require.Equal(t, "normal", detail["priority"])
	require.Equal(t, "some chat context", detail["chat_context"]) // 经 service trim
	// 创建后无回复，应返回空数组（非 null）。
	replies, ok := detail["replies"].([]any)
	require.True(t, ok)
	require.Len(t, replies, 0)

	// repo 内已落库一条
	require.Len(t, repo.tickets, 1)
	require.Equal(t, int64(42), repo.tickets[0].UserID)
}

func TestSupportTicketHandler_Create_Unauthenticated_401(t *testing.T) {
	h, _, _ := newSupportTicketHandlerForTest(t, true)
	c, w := makeAuthedJSONContext(t, 0, http.MethodPost, "/api/v1/support/tickets",
		map[string]any{"title": "x", "content": "y", "category": "账号"}, nil)

	h.Create(c)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSupportTicketHandler_Create_BadJSON_400(t *testing.T) {
	h, _, _ := newSupportTicketHandlerForTest(t, true)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// 缺少必填 title
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/support/tickets",
		bytes.NewReader([]byte(`{"content":"x","category":"账号"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 1})

	h.Create(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSupportTicketHandler_Create_FeatureDisabled_404(t *testing.T) {
	h, _, _ := newSupportTicketHandlerForTest(t, false)
	c, w := makeAuthedJSONContext(t, 1, http.MethodPost, "/api/v1/support/tickets",
		map[string]any{"title": "x", "content": "y", "category": "账号"}, nil)

	h.Create(c)
	// service 抛 ErrSupportFeatureDisabled → infraerrors.NotFound → 404
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSupportTicketHandler_Create_InvalidCategory_400(t *testing.T) {
	h, _, _ := newSupportTicketHandlerForTest(t, true)
	c, w := makeAuthedJSONContext(t, 1, http.MethodPost, "/api/v1/support/tickets",
		map[string]any{"title": "x", "content": "y", "category": "不存在的分类"}, nil)

	h.Create(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// ----------------------------------------------------------------------------
// List
// ----------------------------------------------------------------------------

func TestSupportTicketHandler_List_HappyPath_OmitsChatContext(t *testing.T) {
	h, repo, _ := newSupportTicketHandlerForTest(t, true)

	cc := "secret-chat-context"
	tk := &service.SupportTicket{
		UserID:      7,
		Title:       "t",
		Content:     "c",
		Category:    "账号",
		Status:      service.SupportTicketStatusOpen,
		Priority:    service.SupportTicketPriorityNormal,
		ChatContext: &cc,
	}
	require.NoError(t, repo.Create(context.Background(), tk))

	c, w := makeAuthedJSONContext(t, 7, http.MethodGet, "/api/v1/support/tickets", nil, nil)
	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w)

	var paged struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &paged))
	require.EqualValues(t, 1, paged.Total)
	require.Len(t, paged.Items, 1)

	// 列表 DTO 编译期不含 chat_context 字段 —— 序列化结果不应包含该 key。
	_, exists := paged.Items[0]["chat_context"]
	require.Falsef(t, exists, "list DTO must not expose chat_context")
}

// ----------------------------------------------------------------------------
// Get
// ----------------------------------------------------------------------------

func TestSupportTicketHandler_Get_HappyPath_ReturnsChatContextAndReplies(t *testing.T) {
	h, repo, _ := newSupportTicketHandlerForTest(t, true)

	cc := "ctx"
	tk := &service.SupportTicket{
		UserID: 9, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityNormal,
		ChatContext: &cc,
	}
	require.NoError(t, repo.Create(context.Background(), tk))
	authorID := int64(9)
	require.NoError(t, repo.AppendReply(context.Background(), &service.SupportTicketReply{
		TicketID: tk.ID, AuthorID: &authorID, IsAdmin: false, Content: "hi",
	}))

	c, w := makeAuthedJSONContext(t, 9, http.MethodGet,
		"/api/v1/support/tickets/"+strconv.FormatInt(tk.ID, 10), nil,
		gin.Params{{Key: "id", Value: strconv.FormatInt(tk.ID, 10)}})
	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w)

	var detail map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &detail))
	require.Equal(t, "ctx", detail["chat_context"])
	replies, ok := detail["replies"].([]any)
	require.True(t, ok)
	require.Len(t, replies, 1)
}

func TestSupportTicketHandler_Get_NotOwner_404(t *testing.T) {
	// 安全约束：非 owner 应当返回 404 而非 403，避免泄露 ticket 存在性。
	h, repo, _ := newSupportTicketHandlerForTest(t, true)
	tk := &service.SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(context.Background(), tk))

	c, w := makeAuthedJSONContext(t, 999 /* 非 owner */, http.MethodGet, "/api/v1/support/tickets/1", nil,
		gin.Params{{Key: "id", Value: "1"}})
	h.Get(c)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSupportTicketHandler_Get_BadID_400(t *testing.T) {
	h, _, _ := newSupportTicketHandlerForTest(t, true)
	c, w := makeAuthedJSONContext(t, 1, http.MethodGet, "/api/v1/support/tickets/abc", nil,
		gin.Params{{Key: "id", Value: "abc"}})
	h.Get(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSupportTicketHandler_Get_NegativeID_400(t *testing.T) {
	// 负数 / 0 在 parseInt64Param fail-fast。
	h, _, _ := newSupportTicketHandlerForTest(t, true)
	c, w := makeAuthedJSONContext(t, 1, http.MethodGet, "/api/v1/support/tickets/0", nil,
		gin.Params{{Key: "id", Value: "0"}})
	h.Get(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// ----------------------------------------------------------------------------
// AppendReply
// ----------------------------------------------------------------------------

func TestSupportTicketHandler_AppendReply_HappyPath(t *testing.T) {
	h, repo, _ := newSupportTicketHandlerForTest(t, true)
	tk := &service.SupportTicket{UserID: 11, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(context.Background(), tk))

	c, w := makeAuthedJSONContext(t, 11, http.MethodPost,
		"/api/v1/support/tickets/1/replies",
		map[string]any{"content": "hi there"},
		gin.Params{{Key: "id", Value: "1"}})
	h.AppendReply(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w)
	var reply map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &reply))
	require.Equal(t, "hi there", reply["content"])
	require.Equal(t, false, reply["is_admin"])
}

func TestSupportTicketHandler_AppendReply_ClosedTicket_409(t *testing.T) {
	h, repo, _ := newSupportTicketHandlerForTest(t, true)
	closedAt := time.Now().UTC()
	tk := &service.SupportTicket{UserID: 11, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusClosed, Priority: service.SupportTicketPriorityNormal, ClosedAt: &closedAt}
	require.NoError(t, repo.Create(context.Background(), tk))

	c, w := makeAuthedJSONContext(t, 11, http.MethodPost,
		"/api/v1/support/tickets/1/replies",
		map[string]any{"content": "x"},
		gin.Params{{Key: "id", Value: "1"}})
	h.AppendReply(c)
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestSupportTicketHandler_AppendReply_EmptyContent_400(t *testing.T) {
	h, repo, _ := newSupportTicketHandlerForTest(t, true)
	tk := &service.SupportTicket{UserID: 11, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(context.Background(), tk))

	// gin binding: content `binding:"required"` —— 空字符串视为缺失，返回 400。
	c, w := makeAuthedJSONContext(t, 11, http.MethodPost,
		"/api/v1/support/tickets/1/replies",
		map[string]any{"content": ""},
		gin.Params{{Key: "id", Value: "1"}})
	h.AppendReply(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// ----------------------------------------------------------------------------
// Close
// ----------------------------------------------------------------------------

func TestSupportTicketHandler_Close_HappyPath(t *testing.T) {
	h, repo, _ := newSupportTicketHandlerForTest(t, true)
	tk := &service.SupportTicket{UserID: 7, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusOpen, Priority: service.SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(context.Background(), tk))

	c, w := makeAuthedJSONContext(t, 7, http.MethodPost,
		"/api/v1/support/tickets/1/close", nil,
		gin.Params{{Key: "id", Value: "1"}})
	h.Close(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, service.SupportTicketStatusClosed, repo.tickets[0].Status)
	require.NotNil(t, repo.tickets[0].ClosedAt)
}

func TestSupportTicketHandler_Close_AlreadyClosed_409(t *testing.T) {
	h, repo, _ := newSupportTicketHandlerForTest(t, true)
	cl := time.Now().UTC()
	tk := &service.SupportTicket{UserID: 7, Title: "t", Content: "c", Category: "账号",
		Status: service.SupportTicketStatusClosed, Priority: service.SupportTicketPriorityNormal, ClosedAt: &cl}
	require.NoError(t, repo.Create(context.Background(), tk))

	c, w := makeAuthedJSONContext(t, 7, http.MethodPost, "/api/v1/support/tickets/1/close", nil,
		gin.Params{{Key: "id", Value: "1"}})
	h.Close(c)
	require.Equal(t, http.StatusConflict, w.Code)
}

// ----------------------------------------------------------------------------
// ListCategories
// ----------------------------------------------------------------------------

func TestSupportTicketHandler_ListCategories_HappyPath(t *testing.T) {
	h, _, _ := newSupportTicketHandlerForTest(t, true)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/support/categories", nil)

	h.ListCategories(c)
	require.Equal(t, http.StatusOK, w.Code)

	env := decodeEnvelope(t, w)
	var resp struct {
		Categories      []string `json:"categories"`
		DefaultPriority string   `json:"default_priority"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &resp))
	require.ElementsMatch(t, []string{"充值", "账号", "API", "Bug", "其他"}, resp.Categories)
	require.Equal(t, "normal", resp.DefaultPriority)
}

func TestSupportTicketHandler_ListCategories_FeatureDisabled_404(t *testing.T) {
	h, _, _ := newSupportTicketHandlerForTest(t, false)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/support/categories", nil)

	h.ListCategories(c)
	require.Equal(t, http.StatusNotFound, w.Code)
}
