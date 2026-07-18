//go:build unit

// Package service — support_ticket_inbox_bridge_test.go
//
// 覆盖 general-inbox 的工单→通用信箱双写桥接逻辑：
//   - inboxReady 开关（publisher != nil）；
//   - publishInboxToAdmins → PublishBroadcast，命名空间 / dedup_key / 管理员定向正确；
//   - publishInboxDirect → PublishToUser，recipient / dedup_key 正确，非法 recipient 跳过；
//   - publisher 报错被 swallow（不 panic、不返回）；
//   - payload 序列化字段完整。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/inbox"

	"github.com/stretchr/testify/require"
)

// fakeInboxPublisher 记录最后一次发布调用，用于断言。
type fakeInboxPublisher struct {
	broadcasts []inbox.PublishBroadcastInput
	directs    []inbox.PublishDirectInput
	err        error
}

func (f *fakeInboxPublisher) PublishToUser(_ context.Context, in inbox.PublishDirectInput) (bool, int64, error) {
	f.directs = append(f.directs, in)
	if f.err != nil {
		return false, 0, f.err
	}
	return true, 1, nil
}

func (f *fakeInboxPublisher) PublishBroadcast(_ context.Context, in inbox.PublishBroadcastInput) (bool, int64, error) {
	f.broadcasts = append(f.broadcasts, in)
	if f.err != nil {
		return false, 0, f.err
	}
	return true, 1, nil
}

func newBridgeCtx() SupportTicketEventContext {
	return SupportTicketEventContext{
		Ticket:  SupportTicket{ID: 42, Title: "打不开页面", UserID: 7},
		Excerpt: "点了半天没反应",
		ReplyID: 100,
	}
}

func TestInboxReadyPublisherOnly(t *testing.T) {
	svc := &SupportTicketNotificationService{}
	require.False(t, svc.inboxReady(), "未装配 publisher → false")

	svc.AttachInbox(nil)
	require.False(t, svc.inboxReady(), "publisher 为 nil → false")

	svc.AttachInbox(&fakeInboxPublisher{})
	require.True(t, svc.inboxReady(), "装配了 publisher → true")
}

func TestPublishInboxToAdmins(t *testing.T) {
	pub := &fakeInboxPublisher{}
	svc := &SupportTicketNotificationService{}
	svc.AttachInbox(pub)

	evt := newBridgeCtx()
	svc.publishInboxToAdmins(context.Background(), evt, "user_replied", "小明", "https://x/admin/42", evt.ReplyID)

	require.Len(t, pub.broadcasts, 1)
	b := pub.broadcasts[0]
	require.Equal(t, SupportTicketInboxNamespace, b.Namespace)
	require.Equal(t, "user_replied:100", b.DedupKey)
	require.JSONEq(t, `{"op":"equals","attr":"role","value":"admin"}`, string(b.Targeting))

	var p ticketInboxPayload
	require.NoError(t, json.Unmarshal(b.Payload, &p))
	require.Equal(t, int64(42), p.TicketID)
	require.Equal(t, "打不开页面", p.Title)
	require.Equal(t, "点了半天没反应", p.Excerpt)
	require.Equal(t, "小明", p.ActorName)
	require.Equal(t, "https://x/admin/42", p.PortalURL)
	require.Equal(t, "user_replied", p.Event)
}

func TestPublishInboxDirect(t *testing.T) {
	pub := &fakeInboxPublisher{}
	svc := &SupportTicketNotificationService{}
	svc.AttachInbox(pub)

	evt := newBridgeCtx()
	svc.publishInboxDirect(context.Background(), evt.Ticket.UserID, evt, "admin_replied", "客服", "https://x/tickets/42", evt.ReplyID)

	require.Len(t, pub.directs, 1)
	d := pub.directs[0]
	require.Equal(t, int64(7), d.RecipientID)
	require.Equal(t, SupportTicketInboxNamespace, d.Namespace)
	require.Equal(t, "admin_replied:100", d.DedupKey)
}

func TestPublishInboxDirectSkipsInvalidRecipient(t *testing.T) {
	pub := &fakeInboxPublisher{}
	svc := &SupportTicketNotificationService{}
	svc.AttachInbox(pub)

	evt := newBridgeCtx()
	svc.publishInboxDirect(context.Background(), 0, evt, "admin_replied", "客服", "url", evt.ReplyID)
	require.Empty(t, pub.directs, "recipient<=0 应直接跳过")
}

func TestPublishInboxSkipsWhenNoPublisher(t *testing.T) {
	svc := &SupportTicketNotificationService{} // 未装配 publisher
	evt := newBridgeCtx()
	require.NotPanics(t, func() {
		svc.publishInboxToAdmins(context.Background(), evt, "ticket_created", "小明", "url", evt.Ticket.ID)
		svc.publishInboxDirect(context.Background(), 7, evt, "admin_replied", "客服", "url", evt.ReplyID)
	})
}

func TestPublishInboxSwallowsPublisherError(t *testing.T) {
	pub := &fakeInboxPublisher{err: errors.New("redis down")}
	svc := &SupportTicketNotificationService{}
	svc.AttachInbox(pub)

	evt := newBridgeCtx()
	// 不应 panic，也不应把错误往外抛（方法无返回值）。
	require.NotPanics(t, func() {
		svc.publishInboxToAdmins(context.Background(), evt, "ticket_created", "小明", "url", evt.Ticket.ID)
		svc.publishInboxDirect(context.Background(), 7, evt, "admin_replied", "客服", "url", evt.ReplyID)
	})
	require.Len(t, pub.broadcasts, 1)
	require.Len(t, pub.directs, 1)
}

func TestTicketInboxDedupKey(t *testing.T) {
	require.Equal(t, "ticket_created:42", ticketInboxDedupKey("ticket_created", 42))
	require.Equal(t, "admin_replied:100", ticketInboxDedupKey("admin_replied", 100))
}
