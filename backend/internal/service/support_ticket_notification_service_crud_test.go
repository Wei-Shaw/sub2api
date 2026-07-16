//go:build unit

// Package service — support_ticket_notification_service_crud_test.go
//
// 覆盖 SupportTicketNotificationService 的 4 个 CRUD 方法：
//   - ListNotifications
//   - CountUnread
//   - MarkOneRead
//   - MarkAllRead
//
// 三条主线：
//  1. recipientUserID = 0 → 立即返回 ErrSupportTicketNotificationRecipientRequired（不走 repo）；
//  2. 参数透传（pagination / onlyUnread / readAt 传得完整）；
//  3. 错误透传（repo 报错时不被 swallow）。
//
// Notify* 三个方法（fan-out + 邮件）行为复杂，落在独立集成测试里覆盖。
package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"github.com/stretchr/testify/require"
)

// crudNotifRepoStub 只关心方法入参 & 返回值。
type crudNotifRepoStub struct {
	listItems       []SupportTicketNotification
	listPage        *pagination.PaginationResult
	unreadCount     int64
	markAllAffected int64

	listErr     error
	countErr    error
	markOneErr  error
	markAllErr  error

	// 入参快照
	lastListParams    SupportTicketNotificationListParams
	lastMarkOneID     int64
	lastMarkOneUserID int64
	lastMarkOneAt     time.Time
	lastMarkAllUserID int64
	lastMarkAllAt     time.Time
	insertCalls       int
}

func (s *crudNotifRepoStub) Insert(_ context.Context, _ *SupportTicketNotification) error {
	s.insertCalls++
	return nil
}

func (s *crudNotifRepoStub) ListByRecipient(
	_ context.Context,
	params SupportTicketNotificationListParams,
) ([]SupportTicketNotification, *pagination.PaginationResult, error) {
	s.lastListParams = params
	if s.listErr != nil {
		return nil, nil, s.listErr
	}
	return s.listItems, s.listPage, nil
}

func (s *crudNotifRepoStub) CountUnreadByRecipient(_ context.Context, _ int64) (int64, error) {
	if s.countErr != nil {
		return 0, s.countErr
	}
	return s.unreadCount, nil
}

func (s *crudNotifRepoStub) MarkOneRead(_ context.Context, id, userID int64, at time.Time) error {
	s.lastMarkOneID = id
	s.lastMarkOneUserID = userID
	s.lastMarkOneAt = at
	return s.markOneErr
}

func (s *crudNotifRepoStub) MarkAllRead(_ context.Context, userID int64, at time.Time) (int64, error) {
	s.lastMarkAllUserID = userID
	s.lastMarkAllAt = at
	if s.markAllErr != nil {
		return 0, s.markAllErr
	}
	return s.markAllAffected, nil
}

// newBareNotifService 构造仅够测 CRUD 的 service（settings/users/emailer 全 nil）。
func newBareNotifService(repo SupportTicketNotificationRepository) *SupportTicketNotificationService {
	return NewSupportTicketNotificationService(repo, nil, nil, nil)
}

// ----------------------------------------------------------------------------
// ListNotifications
// ----------------------------------------------------------------------------

func TestSupportTicketNotificationService_List_RecipientZero_RejectsFast(t *testing.T) {
	repo := &crudNotifRepoStub{}
	s := newBareNotifService(repo)

	items, page, err := s.ListNotifications(context.Background(), 0, false,
		pagination.PaginationParams{Page: 1, PageSize: 20})
	require.ErrorIs(t, err, ErrSupportTicketNotificationRecipientRequired)
	require.Nil(t, items)
	require.Nil(t, page)
	require.Zero(t, repo.lastListParams.RecipientUserID,
		"recipient=0 时必须在 service 层直接拒绝，不能触到 repo")
}

func TestSupportTicketNotificationService_List_PassesParams(t *testing.T) {
	repo := &crudNotifRepoStub{
		listItems: []SupportTicketNotification{{ID: 1, RecipientUserID: 42}},
		listPage:  &pagination.PaginationResult{Total: 1},
	}
	s := newBareNotifService(repo)

	items, page, err := s.ListNotifications(context.Background(), 42, true,
		pagination.PaginationParams{Page: 2, PageSize: 30})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.EqualValues(t, 1, page.Total)

	require.EqualValues(t, 42, repo.lastListParams.RecipientUserID)
	require.True(t, repo.lastListParams.OnlyUnread)
	require.Equal(t, 2, repo.lastListParams.Params.Page)
	require.Equal(t, 30, repo.lastListParams.Params.PageSize)
}

func TestSupportTicketNotificationService_List_PropagatesRepoError(t *testing.T) {
	repo := &crudNotifRepoStub{listErr: errors.New("connection reset")}
	s := newBareNotifService(repo)
	_, _, err := s.ListNotifications(context.Background(), 42, false,
		pagination.PaginationParams{Page: 1, PageSize: 20})
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection reset")
}

// ----------------------------------------------------------------------------
// CountUnread
// ----------------------------------------------------------------------------

func TestSupportTicketNotificationService_CountUnread_RecipientZero_Rejects(t *testing.T) {
	repo := &crudNotifRepoStub{unreadCount: 999}
	s := newBareNotifService(repo)
	_, err := s.CountUnread(context.Background(), 0)
	require.ErrorIs(t, err, ErrSupportTicketNotificationRecipientRequired)
}

func TestSupportTicketNotificationService_CountUnread_PassesThrough(t *testing.T) {
	repo := &crudNotifRepoStub{unreadCount: 7}
	s := newBareNotifService(repo)
	got, err := s.CountUnread(context.Background(), 42)
	require.NoError(t, err)
	require.EqualValues(t, 7, got)
}

// ----------------------------------------------------------------------------
// MarkOneRead
// ----------------------------------------------------------------------------

func TestSupportTicketNotificationService_MarkOneRead_RecipientZero_Rejects(t *testing.T) {
	repo := &crudNotifRepoStub{}
	s := newBareNotifService(repo)
	err := s.MarkOneRead(context.Background(), 555, 0)
	require.ErrorIs(t, err, ErrSupportTicketNotificationRecipientRequired)
	require.Zero(t, repo.lastMarkOneID, "recipient=0 时不能触到 repo")
}

func TestSupportTicketNotificationService_MarkOneRead_PassesParams(t *testing.T) {
	repo := &crudNotifRepoStub{}
	s := newBareNotifService(repo)

	before := time.Now().UTC()
	err := s.MarkOneRead(context.Background(), 555, 42)
	after := time.Now().UTC()
	require.NoError(t, err)
	require.EqualValues(t, 555, repo.lastMarkOneID)
	require.EqualValues(t, 42, repo.lastMarkOneUserID)
	require.True(t, !repo.lastMarkOneAt.Before(before) && !repo.lastMarkOneAt.After(after.Add(time.Second)),
		"readAt 应在调用瞬间的时间窗内")
}

func TestSupportTicketNotificationService_MarkOneRead_PropagatesNotFound(t *testing.T) {
	repo := &crudNotifRepoStub{markOneErr: ErrSupportTicketNotificationNotFound}
	s := newBareNotifService(repo)
	err := s.MarkOneRead(context.Background(), 999, 42)
	require.ErrorIs(t, err, ErrSupportTicketNotificationNotFound)
}

// ----------------------------------------------------------------------------
// MarkAllRead
// ----------------------------------------------------------------------------

func TestSupportTicketNotificationService_MarkAllRead_RecipientZero_Rejects(t *testing.T) {
	repo := &crudNotifRepoStub{markAllAffected: 999}
	s := newBareNotifService(repo)
	got, err := s.MarkAllRead(context.Background(), 0)
	require.ErrorIs(t, err, ErrSupportTicketNotificationRecipientRequired)
	require.EqualValues(t, 0, got)
	require.Zero(t, repo.lastMarkAllUserID)
}

func TestSupportTicketNotificationService_MarkAllRead_PassesUserID(t *testing.T) {
	repo := &crudNotifRepoStub{markAllAffected: 5}
	s := newBareNotifService(repo)
	got, err := s.MarkAllRead(context.Background(), 42)
	require.NoError(t, err)
	require.EqualValues(t, 5, got)
	require.EqualValues(t, 42, repo.lastMarkAllUserID)
}

func TestSupportTicketNotificationService_MarkAllRead_ZeroAffectedIsFine(t *testing.T) {
	// 幂等语义：无未读时 affected=0 且不报错。
	repo := &crudNotifRepoStub{markAllAffected: 0}
	s := newBareNotifService(repo)
	got, err := s.MarkAllRead(context.Background(), 42)
	require.NoError(t, err)
	require.EqualValues(t, 0, got)
}
