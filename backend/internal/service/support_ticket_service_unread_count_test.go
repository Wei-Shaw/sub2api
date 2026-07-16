//go:build unit

// Package service — support_ticket_service_unread_count_test.go
//
// 测试 SupportTicketService.CountUserUnreadTickets / CountAdminUnreadTickets 两个
// "未读工单数聚合"入口的 nil-safe + 透传行为。
//
// 底层 SQL 逻辑走 support_ticket_read_repo integration test 覆盖（真数据库跑）；
// 这里只关心：
//   - readRepo == nil：service 返回 0，不 panic；
//   - readRepo 非 nil：透传结果 + userID；
//   - readRepo 错误：透传错误。
package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubReadRepoForCount 只覆盖测试关心的三个方法。
type stubReadRepoForCount struct {
	userReturn   int64
	adminReturn  int64
	userErr      error
	adminErr     error
	lastUserID   int64
	lastAdminID  int64
	userCallCnt  int
	adminCallCnt int
}

func (s *stubReadRepoForCount) MarkTicketRead(_ context.Context, _, _ int64, _ time.Time) error {
	return nil
}

func (s *stubReadRepoForCount) CountUnreadForUser(_ context.Context, userID int64) (int64, error) {
	s.lastUserID = userID
	s.userCallCnt++
	return s.userReturn, s.userErr
}

func (s *stubReadRepoForCount) CountUnreadForAdmin(_ context.Context, adminID int64) (int64, error) {
	s.lastAdminID = adminID
	s.adminCallCnt++
	return s.adminReturn, s.adminErr
}

// newBareServiceForCountTest 装配一个仅够测 count 分支的 service。
// repo/settings 都传 nil / minimal fake，因为 CountUserUnreadTickets 只读 s.readRepo。
func newBareServiceForCountTest(readRepo SupportTicketReadRepository) *SupportTicketService {
	s := NewSupportTicketService(nil, &countSettingsStub{enabled: true}, nil, nil)
	s.AttachNotifier(nil, readRepo)
	return s
}

// countSettingsStub 是 SupportTicketSettingsReader 的极简实现。
type countSettingsStub struct {
	enabled bool
}

func (c *countSettingsStub) GetSupportTicketRuntime(context.Context) SupportTicketRuntime {
	return SupportTicketRuntime{
		Enabled:         c.enabled,
		Categories:      []string{"其他"},
		DefaultPriority: SupportTicketPriorityNormal,
	}
}

// ----------------------------------------------------------------------------
// CountUserUnreadTickets
// ----------------------------------------------------------------------------

func TestSupportTicketService_CountUserUnreadTickets_NilReadRepo_ReturnsZero(t *testing.T) {
	s := NewSupportTicketService(nil, &countSettingsStub{enabled: true}, nil, nil)
	// 显式不调 AttachNotifier → s.readRepo 保持 nil。

	got, err := s.CountUserUnreadTickets(context.Background(), 42)
	require.NoError(t, err)
	require.EqualValues(t, 0, got, "readRepo 未装配时 service 应返回 0 而非 panic")
}

func TestSupportTicketService_CountUserUnreadTickets_PassesThrough(t *testing.T) {
	stub := &stubReadRepoForCount{userReturn: 5}
	s := newBareServiceForCountTest(stub)

	got, err := s.CountUserUnreadTickets(context.Background(), 42)
	require.NoError(t, err)
	require.EqualValues(t, 5, got)
	require.EqualValues(t, 42, stub.lastUserID)
	require.Equal(t, 1, stub.userCallCnt)
	// 保证没错走 admin 分支
	require.Equal(t, 0, stub.adminCallCnt)
}

func TestSupportTicketService_CountUserUnreadTickets_PropagatesError(t *testing.T) {
	stub := &stubReadRepoForCount{userErr: errors.New("db down")}
	s := newBareServiceForCountTest(stub)

	_, err := s.CountUserUnreadTickets(context.Background(), 42)
	require.Error(t, err)
	require.Contains(t, err.Error(), "db down")
}

// ----------------------------------------------------------------------------
// CountAdminUnreadTickets
// ----------------------------------------------------------------------------

func TestSupportTicketService_CountAdminUnreadTickets_NilReadRepo_ReturnsZero(t *testing.T) {
	s := NewSupportTicketService(nil, &countSettingsStub{enabled: true}, nil, nil)
	got, err := s.CountAdminUnreadTickets(context.Background(), 99)
	require.NoError(t, err)
	require.EqualValues(t, 0, got)
}

func TestSupportTicketService_CountAdminUnreadTickets_PassesThrough(t *testing.T) {
	stub := &stubReadRepoForCount{adminReturn: 12}
	s := newBareServiceForCountTest(stub)

	got, err := s.CountAdminUnreadTickets(context.Background(), 99)
	require.NoError(t, err)
	require.EqualValues(t, 12, got)
	require.EqualValues(t, 99, stub.lastAdminID)
	require.Equal(t, 1, stub.adminCallCnt)
	require.Equal(t, 0, stub.userCallCnt)
}

func TestSupportTicketService_CountAdminUnreadTickets_PropagatesError(t *testing.T) {
	stub := &stubReadRepoForCount{adminErr: errors.New("timeout")}
	s := newBareServiceForCountTest(stub)

	_, err := s.CountAdminUnreadTickets(context.Background(), 99)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timeout")
}
