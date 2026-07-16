//go:build unit

// Package dto — support_ticket_notification_test.go
//
// 覆盖 SupportTicketNotificationItemFromService / SupportTicketNotificationItemsFromService 的
// nil-safe 展平语义：
//   - service.SupportTicketNotification.ActorUserID *int64（nil / 非 nil）
//   - service.SupportTicketNotification.ReadAt *time.Time（nil / 非 nil）
// 前端会对 DTO 字段做直接 v-for，因此必须保证：
//   - nil → 0 / zero time 而不是 undefined；
//   - 空切片 → []T{} 而不是 null。
package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

func TestSupportTicketNotificationItemFromService_FullFields(t *testing.T) {
	actorID := int64(7)
	readAt := time.Date(2026, 7, 16, 10, 30, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)

	got := SupportTicketNotificationItemFromService(service.SupportTicketNotification{
		ID:              101,
		RecipientUserID: 42,
		TicketID:        555,
		EventType:       "admin_replied",
		TitleSnapshot:   "登录失败",
		Excerpt:         "已收到您的工单",
		ActorUserID:     &actorID,
		IsRead:          true,
		CreatedAt:       createdAt,
		ReadAt:          &readAt,
	})

	require.Equal(t, SupportTicketNotificationItem{
		ID:              101,
		RecipientUserID: 42,
		TicketID:        555,
		EventType:       "admin_replied",
		TitleSnapshot:   "登录失败",
		Excerpt:         "已收到您的工单",
		ActorUserID:     7,
		IsRead:          true,
		CreatedAt:       createdAt,
		ReadAt:          readAt,
	}, got)
}

func TestSupportTicketNotificationItemFromService_NilActorAndReadAt_FlattenToZero(t *testing.T) {
	createdAt := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	got := SupportTicketNotificationItemFromService(service.SupportTicketNotification{
		ID:              102,
		RecipientUserID: 42,
		TicketID:        556,
		EventType:       "ticket_created",
		TitleSnapshot:   "计费问题",
		Excerpt:         "请提供订单号",
		ActorUserID:     nil,
		IsRead:          false,
		CreatedAt:       createdAt,
		ReadAt:          nil,
	})

	require.EqualValues(t, 0, got.ActorUserID,
		"ActorUserID *int64 = nil 必须展平为 0（前端 v-for 依赖非空数值）")
	require.True(t, got.ReadAt.IsZero(),
		"ReadAt *time.Time = nil 必须展平为 zero time；前端根据 IsRead 判断是否解读 ReadAt")
	require.False(t, got.IsRead)
}

func TestSupportTicketNotificationItemsFromService_EmptyInputReturnsEmptySlice(t *testing.T) {
	// 无论 nil 还是空 slice，输出都是 []Item{} 而不是 nil。
	// 前端 v-for 依赖 items.length；null 会触发 undefined。
	require.NotNil(t, SupportTicketNotificationItemsFromService(nil))
	require.Empty(t, SupportTicketNotificationItemsFromService(nil))

	require.NotNil(t, SupportTicketNotificationItemsFromService([]service.SupportTicketNotification{}))
	require.Empty(t, SupportTicketNotificationItemsFromService([]service.SupportTicketNotification{}))
}

func TestSupportTicketNotificationItemsFromService_PreservesOrder(t *testing.T) {
	// 保序：service 层保证 created_at DESC，DTO 转换不能重排。
	in := []service.SupportTicketNotification{
		{ID: 3, CreatedAt: time.Unix(3000, 0)},
		{ID: 2, CreatedAt: time.Unix(2000, 0)},
		{ID: 1, CreatedAt: time.Unix(1000, 0)},
	}
	out := SupportTicketNotificationItemsFromService(in)
	require.Len(t, out, 3)
	require.EqualValues(t, 3, out[0].ID)
	require.EqualValues(t, 2, out[1].ID)
	require.EqualValues(t, 1, out[2].ID)
}
