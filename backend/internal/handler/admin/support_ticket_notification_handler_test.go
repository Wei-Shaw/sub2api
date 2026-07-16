//go:build unit

// Package admin — support_ticket_notification_handler_test.go
//
// admin 端"工单通知 & 未读计数" handler 单测。
//
// 覆盖：
//   - UnreadCount 走 admin 视角（CountUnreadForAdmin）而非 user 视角；
//   - List / MarkOneRead / MarkAllRead 与用户端语义对称，主要断言 recipient 隔离
//     （subject.UserID 作为 admin 自己）。
//
// 用户端 handler 已有更完整的 case 集（handler/support_ticket_notification_handler_test.go），
// admin 这里只做"差异"和 smoke，避免重复。
package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// fakes
// ----------------------------------------------------------------------------

type adminNotifRepoFake struct {
	unreadCount       int64
	markAllAffected   int64
	listItems         []service.SupportTicketNotification
	listPage          *pagination.PaginationResult
	markOneErr        error
	lastListParams    service.SupportTicketNotificationListParams
	lastMarkOneID     int64
	lastMarkOneUserID int64
	lastMarkAllUserID int64
}

func (f *adminNotifRepoFake) Insert(_ context.Context, _ *service.SupportTicketNotification) error {
	return nil
}

func (f *adminNotifRepoFake) ListByRecipient(
	_ context.Context,
	params service.SupportTicketNotificationListParams,
) ([]service.SupportTicketNotification, *pagination.PaginationResult, error) {
	f.lastListParams = params
	if f.listPage == nil {
		f.listPage = &pagination.PaginationResult{Total: int64(len(f.listItems))}
	}
	return f.listItems, f.listPage, nil
}

func (f *adminNotifRepoFake) CountUnreadByRecipient(_ context.Context, _ int64) (int64, error) {
	return f.unreadCount, nil
}

func (f *adminNotifRepoFake) MarkOneRead(_ context.Context, id, userID int64, _ time.Time) error {
	f.lastMarkOneID = id
	f.lastMarkOneUserID = userID
	return f.markOneErr
}

func (f *adminNotifRepoFake) MarkAllRead(_ context.Context, userID int64, _ time.Time) (int64, error) {
	f.lastMarkAllUserID = userID
	return f.markAllAffected, nil
}

// adminReadRepoFake 关键：UnreadCount 走 admin 分支，用 adminCount 记录调用。
type adminReadRepoFake struct {
	userCount  int64
	adminCount int64
	// countCalledFor 标注最近一次被调的方法："user" / "admin"
	countCalledFor string
}

func (f *adminReadRepoFake) MarkTicketRead(_ context.Context, _, _ int64, _ time.Time) error {
	return nil
}

func (f *adminReadRepoFake) CountUnreadForUser(_ context.Context, _ int64) (int64, error) {
	f.countCalledFor = "user"
	return f.userCount, nil
}

func (f *adminReadRepoFake) CountUnreadForAdmin(_ context.Context, _ int64) (int64, error) {
	f.countCalledFor = "admin"
	return f.adminCount, nil
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func newAdminSupportTicketNotificationHandlerForTest(t *testing.T) (
	*SupportTicketNotificationHandler,
	*adminNotifRepoFake,
	*adminReadRepoFake,
) {
	t.Helper()
	notifRepo := &adminNotifRepoFake{}
	readRepo := &adminReadRepoFake{}
	notifSvc := service.NewSupportTicketNotificationService(notifRepo, nil, nil, nil)

	ticketSvc := service.NewSupportTicketService(nil, newAdminStSettings(true), nil, nil)
	ticketSvc.AttachNotifier(nil, readRepo)

	h := NewSupportTicketNotificationHandler(ticketSvc, notifSvc)
	return h, notifRepo, readRepo
}

func makeAdminAuthedContext(
	t *testing.T,
	adminID int64,
	method, url string,
	pathParams gin.Params,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(method, url, bytes.NewReader(nil))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	if adminID > 0 {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: adminID})
	}
	if pathParams != nil {
		c.Params = pathParams
	}
	return c, w
}

// decodeAdminEnvelope 复用 support_ticket_handler_test.go 中的定义。

// ----------------------------------------------------------------------------
// UnreadCount：与用户端最核心的差异——走 admin 分支
// ----------------------------------------------------------------------------

func TestAdminSupportTicketNotificationHandler_UnreadCount_UsesAdminAggregation(t *testing.T) {
	h, _, readRepo := newAdminSupportTicketNotificationHandlerForTest(t)
	readRepo.adminCount = 7
	readRepo.userCount = 999 // 不应被读到——否则说明 handler 走错分支

	c, w := makeAdminAuthedContext(t, 42, http.MethodGet, "/api/v1/admin/support/tickets/unread-count", nil)
	h.UnreadCount(c)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeAdminEnvelope(t, w)
	require.Equal(t, 0, env.Code)
	require.JSONEq(t, `{"count":7}`, string(env.Data))
	require.Equal(t, "admin", readRepo.countCalledFor,
		"admin handler 必须走 CountUnreadForAdmin 而非 CountUnreadForUser")
}

func TestAdminSupportTicketNotificationHandler_UnreadCount_Unauthenticated_401(t *testing.T) {
	h, _, _ := newAdminSupportTicketNotificationHandlerForTest(t)
	c, w := makeAdminAuthedContext(t, 0, http.MethodGet, "/api/v1/admin/support/tickets/unread-count", nil)
	h.UnreadCount(c)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// ----------------------------------------------------------------------------
// List / MarkOne / MarkAll：admin subject 传递校验（recipient 隔离）
// ----------------------------------------------------------------------------

func TestAdminSupportTicketNotificationHandler_List_UsesAdminSubjectAsRecipient(t *testing.T) {
	h, notifRepo, _ := newAdminSupportTicketNotificationHandlerForTest(t)
	notifRepo.listItems = nil
	notifRepo.listPage = &pagination.PaginationResult{Total: 0}

	c, w := makeAdminAuthedContext(t, 99, http.MethodGet,
		"/api/v1/admin/support/tickets/notifications?only_unread=true", nil)
	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.EqualValues(t, 99, notifRepo.lastListParams.RecipientUserID)
	require.True(t, notifRepo.lastListParams.OnlyUnread)
}

func TestAdminSupportTicketNotificationHandler_MarkOneRead_HappyPath(t *testing.T) {
	h, notifRepo, _ := newAdminSupportTicketNotificationHandlerForTest(t)

	c, w := makeAdminAuthedContext(t, 99, http.MethodPost,
		"/api/v1/admin/support/tickets/notifications/555/read",
		gin.Params{{Key: "id", Value: "555"}})
	h.MarkOneRead(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.EqualValues(t, 555, notifRepo.lastMarkOneID)
	require.EqualValues(t, 99, notifRepo.lastMarkOneUserID)
}

func TestAdminSupportTicketNotificationHandler_MarkOneRead_NotFound_404(t *testing.T) {
	h, notifRepo, _ := newAdminSupportTicketNotificationHandlerForTest(t)
	notifRepo.markOneErr = service.ErrSupportTicketNotificationNotFound

	c, w := makeAdminAuthedContext(t, 99, http.MethodPost,
		"/api/v1/admin/support/tickets/notifications/999/read",
		gin.Params{{Key: "id", Value: "999"}})
	h.MarkOneRead(c)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminSupportTicketNotificationHandler_MarkAllRead_HappyPath(t *testing.T) {
	h, notifRepo, _ := newAdminSupportTicketNotificationHandlerForTest(t)
	notifRepo.markAllAffected = 3

	c, w := makeAdminAuthedContext(t, 99, http.MethodPost,
		"/api/v1/admin/support/tickets/notifications/read-all", nil)
	h.MarkAllRead(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"affected":3}`, string(decodeAdminEnvelope(t, w).Data))
	require.EqualValues(t, 99, notifRepo.lastMarkAllUserID)
}
