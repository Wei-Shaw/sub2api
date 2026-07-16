//go:build unit

// Package handler — support_ticket_notification_handler_test.go
//
// 用户端"工单通知 & 未读计数" handler 单测。
//
// 覆盖：
//   - GET  /unread-count         happy / 401
//   - GET  /notifications        happy（DTO 展平，含 nil → 0 / zero-time）+ only_unread 参数透传
//   - POST /notifications/:id/read happy / 无效 id 400 / not-found 404 / 401
//   - POST /notifications/read-all happy / 401
//
// 装配方式：注入内存 fake（stNotifRepoFake / stReadRepoFake）到 SupportTicketNotificationService
// 与 SupportTicketService，避免拉起 ent。SupportTicketService 只在这些 handler 里被 CountUserUnreadTickets
// 触到，无需真正的 ticket 数据。
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stPaginatedEnvelope 与 response.PaginatedData 对齐；items 是 []map[string]any
// 便于 JSON 字段级断言（避免依赖具体 DTO 类型）。
type stPaginatedEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Items    []map[string]any `json:"items"`
		Total    int64            `json:"total"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Pages    int              `json:"pages"`
	} `json:"data"`
}

func decodePaginatedEnvelope(t *testing.T, w *httptest.ResponseRecorder) stPaginatedEnvelope {
	t.Helper()
	var env stPaginatedEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	return env
}

// decodeDataString 返回 response.data 的 JSON 字符串（用于 JSONEq 断言）。
func decodeDataString(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	env := decodeEnvelope(t, w)
	return string(env.Data)
}

// ginParams 构造 gin.Params。语法糖，避免每个 case 都写字面值。
func ginParams(pairs ...string) gin.Params {
	if len(pairs)%2 != 0 {
		panic("ginParams: pairs 必须是偶数（key,value 交替）")
	}
	out := make(gin.Params, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, gin.Param{Key: pairs[i], Value: pairs[i+1]})
	}
	return out
}

// silence unused-import warnings for strconv when tests are trimmed; strconv
// might be needed by future case expansions (list_test 里已用过)。
var _ = strconv.Itoa

// ----------------------------------------------------------------------------
// fakes
// ----------------------------------------------------------------------------

// stNotifRepoFake 是 SupportTicketNotificationRepository 的内存实现。
// 每个字段都是"调用记录 + 返回值"双用途，方便断言。
type stNotifRepoFake struct {
	// 出参：ListByRecipient / CountUnread / MarkAllRead 的返回值
	listItems         []service.SupportTicketNotification
	listPage          *pagination.PaginationResult
	unreadCount       int64
	markAllAffected   int64
	markOneErr        error // 传 nil 表示成功；传 service.ErrSupportTicketNotificationNotFound 触发 404
	listErr           error
	countErr          error
	markAllErr        error
	// 入参：最后一次调用的参数快照，用于断言
	lastListParams    service.SupportTicketNotificationListParams
	lastMarkOneID     int64
	lastMarkOneUserID int64
	lastMarkAllUserID int64
	// insertCalled 只在触发通知副作用的其他测试里断言用；本文件不断言。
	insertCalled int
}

func (f *stNotifRepoFake) Insert(_ context.Context, _ *service.SupportTicketNotification) error {
	f.insertCalled++
	return nil
}

func (f *stNotifRepoFake) ListByRecipient(
	_ context.Context,
	params service.SupportTicketNotificationListParams,
) ([]service.SupportTicketNotification, *pagination.PaginationResult, error) {
	f.lastListParams = params
	if f.listErr != nil {
		return nil, nil, f.listErr
	}
	// 保证返回 non-nil result（handler 会读 Total 传给 response.Paginated）
	if f.listPage == nil {
		f.listPage = &pagination.PaginationResult{Total: int64(len(f.listItems))}
	}
	return f.listItems, f.listPage, nil
}

func (f *stNotifRepoFake) CountUnreadByRecipient(_ context.Context, _ int64) (int64, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	return f.unreadCount, nil
}

func (f *stNotifRepoFake) MarkOneRead(_ context.Context, id, userID int64, _ time.Time) error {
	f.lastMarkOneID = id
	f.lastMarkOneUserID = userID
	return f.markOneErr
}

func (f *stNotifRepoFake) MarkAllRead(_ context.Context, userID int64, _ time.Time) (int64, error) {
	f.lastMarkAllUserID = userID
	if f.markAllErr != nil {
		return 0, f.markAllErr
	}
	return f.markAllAffected, nil
}

// stReadRepoFake 是 SupportTicketReadRepository 的内存实现，
// 只关心 CountUnreadForUser / CountUnreadForAdmin 的返回值。
type stReadRepoFake struct {
	userCount  int64
	adminCount int64
	userErr    error
	adminErr   error
	// 记录 MarkTicketRead 是否被调用（当前 test 里 handler 路径不触发，仅装配需要）
	markCount int
}

func (f *stReadRepoFake) MarkTicketRead(_ context.Context, _, _ int64, _ time.Time) error {
	f.markCount++
	return nil
}

func (f *stReadRepoFake) CountUnreadForUser(_ context.Context, _ int64) (int64, error) {
	if f.userErr != nil {
		return 0, f.userErr
	}
	return f.userCount, nil
}

func (f *stReadRepoFake) CountUnreadForAdmin(_ context.Context, _ int64) (int64, error) {
	if f.adminErr != nil {
		return 0, f.adminErr
	}
	return f.adminCount, nil
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// newSupportTicketNotificationHandlerForTest 装配用户端通知 handler，返回 handler + 两个 fake。
//
// notifier 内部依赖 users/emailer 在 CRUD 端点上不会被触发（4 个方法都直接走 notifRepo），
// 因此简单传 nil 即可（生产 wire 会保证注入）。
func newSupportTicketNotificationHandlerForTest(t *testing.T) (
	*SupportTicketNotificationHandler,
	*stNotifRepoFake,
	*stReadRepoFake,
) {
	t.Helper()
	notifRepo := &stNotifRepoFake{}
	readRepo := &stReadRepoFake{}
	notifSvc := service.NewSupportTicketNotificationService(notifRepo, nil, nil, nil)

	// ticketService 只用来做 CountUserUnreadTickets → CountUnreadForUser；
	// repo / settings 传 nil / stub 即可，因为通知 handler 不会调其他方法。
	ticketSvc := service.NewSupportTicketService(nil, newStHandlerSettings(true), nil, nil)
	ticketSvc.AttachNotifier(nil, readRepo)

	h := NewSupportTicketNotificationHandler(ticketSvc, notifSvc)
	return h, notifRepo, readRepo
}

// ----------------------------------------------------------------------------
// UnreadCount
// ----------------------------------------------------------------------------

func TestSupportTicketNotificationHandler_UnreadCount_HappyPath(t *testing.T) {
	h, _, readRepo := newSupportTicketNotificationHandlerForTest(t)
	readRepo.userCount = 3

	c, w := makeAuthedJSONContext(t, 42, http.MethodGet, "/api/v1/support/tickets/unread-count", nil, nil)
	h.UnreadCount(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w)
	require.Equal(t, 0, env.Code)
	require.JSONEq(t, `{"count":3}`, string(env.Data))
}

func TestSupportTicketNotificationHandler_UnreadCount_Unauthenticated_401(t *testing.T) {
	h, _, _ := newSupportTicketNotificationHandlerForTest(t)
	c, w := makeAuthedJSONContext(t, 0, http.MethodGet, "/api/v1/support/tickets/unread-count", nil, nil)
	h.UnreadCount(c)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// ----------------------------------------------------------------------------
// List
// ----------------------------------------------------------------------------

func TestSupportTicketNotificationHandler_List_HappyPath_FlattensDTO(t *testing.T) {
	h, notifRepo, _ := newSupportTicketNotificationHandlerForTest(t)

	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	readAt := now.Add(time.Minute)
	actorID := int64(7)
	notifRepo.listItems = []service.SupportTicketNotification{
		{
			ID:              101,
			RecipientUserID: 42,
			TicketID:        555,
			EventType:       "admin_replied",
			TitleSnapshot:   "登录失败",
			Excerpt:         "已收到您的工单",
			ActorUserID:     &actorID,
			IsRead:          true,
			CreatedAt:       now,
			ReadAt:          &readAt,
		},
		{
			// 未读 + Actor 匿名（nil）：DTO 应展平为 actor_user_id=0 / read_at=zero
			ID:              102,
			RecipientUserID: 42,
			TicketID:        556,
			EventType:       "admin_replied",
			TitleSnapshot:   "计费问题",
			Excerpt:         "请提供订单号",
			ActorUserID:     nil,
			IsRead:          false,
			CreatedAt:       now.Add(time.Second),
			ReadAt:          nil,
		},
	}
	notifRepo.listPage = &pagination.PaginationResult{Total: 2}

	c, w := makeAuthedJSONContext(t, 42, http.MethodGet,
		"/api/v1/support/tickets/notifications?page=1&page_size=20", nil, nil)
	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodePaginatedEnvelope(t, w)
	require.Equal(t, 0, env.Code)
	require.EqualValues(t, 2, env.Data.Total)
	require.Len(t, env.Data.Items, 2)

	// item[0]：已读，Actor 有值
	item0 := env.Data.Items[0]
	require.EqualValues(t, 101, item0["id"])
	require.EqualValues(t, 555, item0["ticket_id"])
	require.Equal(t, "admin_replied", item0["event_type"])
	require.Equal(t, "登录失败", item0["title_snapshot"])
	require.Equal(t, true, item0["is_read"])
	require.EqualValues(t, 7, item0["actor_user_id"])

	// item[1]：未读 + Actor=nil → actor_user_id=0，read_at=zero time
	item1 := env.Data.Items[1]
	require.EqualValues(t, 102, item1["id"])
	require.Equal(t, false, item1["is_read"])
	require.EqualValues(t, 0, item1["actor_user_id"])
	require.Equal(t, "0001-01-01T00:00:00Z", item1["read_at"]) // zero time JSON

	// 参数透传：默认 only_unread=false
	require.False(t, notifRepo.lastListParams.OnlyUnread)
	require.EqualValues(t, 42, notifRepo.lastListParams.RecipientUserID)
}

func TestSupportTicketNotificationHandler_List_OnlyUnreadFilter_PassesThrough(t *testing.T) {
	h, notifRepo, _ := newSupportTicketNotificationHandlerForTest(t)
	notifRepo.listItems = nil // 空结果也 OK
	notifRepo.listPage = &pagination.PaginationResult{Total: 0}

	c, w := makeAuthedJSONContext(t, 42, http.MethodGet,
		"/api/v1/support/tickets/notifications?only_unread=true", nil, nil)
	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, notifRepo.lastListParams.OnlyUnread,
		"only_unread=true 必须透传到 repo params")
}

func TestSupportTicketNotificationHandler_List_Unauthenticated_401(t *testing.T) {
	h, _, _ := newSupportTicketNotificationHandlerForTest(t)
	c, w := makeAuthedJSONContext(t, 0, http.MethodGet, "/api/v1/support/tickets/notifications", nil, nil)
	h.List(c)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// ----------------------------------------------------------------------------
// MarkOneRead
// ----------------------------------------------------------------------------

func TestSupportTicketNotificationHandler_MarkOneRead_HappyPath(t *testing.T) {
	h, notifRepo, _ := newSupportTicketNotificationHandlerForTest(t)

	c, w := makeAuthedJSONContext(t, 42, http.MethodPost,
		"/api/v1/support/tickets/notifications/101/read", nil,
		ginParams("id", "101"))
	h.MarkOneRead(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w)
	require.Equal(t, 0, env.Code)
	require.JSONEq(t, `{"id":101}`, string(env.Data))
	require.EqualValues(t, 101, notifRepo.lastMarkOneID)
	require.EqualValues(t, 42, notifRepo.lastMarkOneUserID)
}

func TestSupportTicketNotificationHandler_MarkOneRead_InvalidID_400(t *testing.T) {
	h, _, _ := newSupportTicketNotificationHandlerForTest(t)
	// id 非数字
	c, w := makeAuthedJSONContext(t, 42, http.MethodPost,
		"/api/v1/support/tickets/notifications/abc/read", nil,
		ginParams("id", "abc"))
	h.MarkOneRead(c)
	require.Equal(t, http.StatusBadRequest, w.Code)

	// id=0 也算 invalid（handler 校验 <=0）
	c2, w2 := makeAuthedJSONContext(t, 42, http.MethodPost,
		"/api/v1/support/tickets/notifications/0/read", nil,
		ginParams("id", "0"))
	h.MarkOneRead(c2)
	require.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestSupportTicketNotificationHandler_MarkOneRead_NotFound_404(t *testing.T) {
	h, notifRepo, _ := newSupportTicketNotificationHandlerForTest(t)
	// service 层从 repo 拿到 NotFound → handler ErrorFrom 翻 404。
	notifRepo.markOneErr = service.ErrSupportTicketNotificationNotFound

	c, w := makeAuthedJSONContext(t, 42, http.MethodPost,
		"/api/v1/support/tickets/notifications/999/read", nil,
		ginParams("id", "999"))
	h.MarkOneRead(c)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSupportTicketNotificationHandler_MarkOneRead_Unauthenticated_401(t *testing.T) {
	h, _, _ := newSupportTicketNotificationHandlerForTest(t)
	c, w := makeAuthedJSONContext(t, 0, http.MethodPost,
		"/api/v1/support/tickets/notifications/101/read", nil,
		ginParams("id", "101"))
	h.MarkOneRead(c)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// ----------------------------------------------------------------------------
// MarkAllRead
// ----------------------------------------------------------------------------

func TestSupportTicketNotificationHandler_MarkAllRead_HappyPath(t *testing.T) {
	h, notifRepo, _ := newSupportTicketNotificationHandlerForTest(t)
	notifRepo.markAllAffected = 5

	c, w := makeAuthedJSONContext(t, 42, http.MethodPost,
		"/api/v1/support/tickets/notifications/read-all", nil, nil)
	h.MarkAllRead(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w)
	require.Equal(t, 0, env.Code)
	require.JSONEq(t, `{"affected":5}`, string(env.Data))
	require.EqualValues(t, 42, notifRepo.lastMarkAllUserID)
}

func TestSupportTicketNotificationHandler_MarkAllRead_ZeroAffected_StillOK(t *testing.T) {
	// 幂等：即使没有任何未读通知，端点也应 200 + affected=0。
	h, notifRepo, _ := newSupportTicketNotificationHandlerForTest(t)
	notifRepo.markAllAffected = 0

	c, w := makeAuthedJSONContext(t, 42, http.MethodPost,
		"/api/v1/support/tickets/notifications/read-all", nil, nil)
	h.MarkAllRead(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"affected":0}`, decodeDataString(t, w))
}

func TestSupportTicketNotificationHandler_MarkAllRead_Unauthenticated_401(t *testing.T) {
	h, _, _ := newSupportTicketNotificationHandlerForTest(t)
	c, w := makeAuthedJSONContext(t, 0, http.MethodPost,
		"/api/v1/support/tickets/notifications/read-all", nil, nil)
	h.MarkAllRead(c)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// ----------------------------------------------------------------------------
// UnreadCount 错误分支
// ----------------------------------------------------------------------------

func TestSupportTicketNotificationHandler_UnreadCount_RepoError_500(t *testing.T) {
	h, _, readRepo := newSupportTicketNotificationHandlerForTest(t)
	readRepo.userErr = errors.New("db down")

	c, w := makeAuthedJSONContext(t, 42, http.MethodGet, "/api/v1/support/tickets/unread-count", nil, nil)
	h.UnreadCount(c)
	require.Equal(t, http.StatusInternalServerError, w.Code,
		"repo 未知错误应 500 而非 200；ErrorFrom 兜底走 InternalError")
}


