// Package service — affiliate_recharge_inbox_bridge.go
//
// 邀请返利 × 通用信箱：被邀请人(invitee)充值成功并产生返利后，向邀请人(inviter)
// 发送一条通用信箱(inbox)单播通知。邀请人在"邀请返利列表"里据此把对应被邀请人置顶，
// 并显示"有新充值"的红色向上箭头（未读态由 inbox 累积 ack 水位驱动，前端渲染）。
//
// 设计要点（对齐 support_ticket_inbox_bridge.go）：
//   - 单播 PublishToUser，收件人 = inviter；namespace = affiliate_recharge；
//   - dedup_key = "recharge:<order_id>"，保证重复触发（重试）幂等；
//   - payload 只含 invitee_id / amount / order_id（前端列表按 invitee_id 匹配置顶/画箭头，
//     不落敏感信息）；
//   - fail-open：发布失败只记 warn，绝不影响充值/返利主流程；
//   - 仅当装配了 inboxPub（inbox 模块已接线）时发布；未接线则跳过。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/inbox"
)

// AffiliateRechargeInboxNamespace 是"被邀请人充值"事件在通用信箱中的命名空间。
const AffiliateRechargeInboxNamespace = "affiliate_recharge"

// affiliateRechargeInboxPayload 是发给邀请人的信箱 payload。
type affiliateRechargeInboxPayload struct {
	Namespace string  `json:"namespace"`
	Event     string  `json:"event"`      // 固定 "invitee_recharged"
	InviteeID int64   `json:"invitee_id"` // 充值的被邀请人 id（前端列表按此匹配置顶/箭头）
	Amount    float64 `json:"amount"`     // 本次充值入账金额（用于文案展示）
	OrderID   int64   `json:"order_id"`   // 充值订单 id（dedup 依据）
}

// inboxRechargeReady 表示 inbox 发布出口是否已装配。
func (s *AffiliateService) inboxRechargeReady() bool {
	return s != nil && s.inboxPub != nil
}

// publishInviteeRechargeToInviter 向邀请人单播一条"被邀请人充值"通知。
// 任何前置条件不满足或发布失败都静默返回（fail-open）。
func (s *AffiliateService) publishInviteeRechargeToInviter(ctx context.Context, inviterID, inviteeID, orderID int64, amount float64) {
	if !s.inboxRechargeReady() {
		return
	}
	if inviterID <= 0 || inviteeID <= 0 || orderID <= 0 {
		return
	}
	payload, err := json.Marshal(affiliateRechargeInboxPayload{
		Namespace: AffiliateRechargeInboxNamespace,
		Event:     "invitee_recharged",
		InviteeID: inviteeID,
		Amount:    amount,
		OrderID:   orderID,
	})
	if err != nil {
		slog.Warn("affiliate_recharge_inbox: marshal payload failed",
			"inviter_id", inviterID, "invitee_id", inviteeID, "order_id", orderID, "err", err)
		return
	}
	if _, _, err := s.inboxPub.PublishToUser(ctx, inbox.PublishDirectInput{
		RecipientID: inviterID,
		Namespace:   AffiliateRechargeInboxNamespace,
		DedupKey:    fmt.Sprintf("recharge:%d", orderID),
		Payload:     payload,
	}); err != nil {
		slog.Warn("affiliate_recharge_inbox: publish direct failed",
			"inviter_id", inviterID, "invitee_id", inviteeID, "order_id", orderID, "err", err)
	}
}
