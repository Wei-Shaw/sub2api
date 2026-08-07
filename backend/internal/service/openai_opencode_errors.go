package service

// opencode zen 上游的账号级错误语义（2026-08-07 实测）：
//   - 401 CreditsError "No payment method ..."：账号未绑定支付方式，
//     属永久性账号问题，充值/绑卡前不可用 -> 停用账号，避免 failover 反复打它
//   - 401 CreditsError "Insufficient balance ..."：余额耗尽，充值前不可用
//     -> 停用账号
//
// 通用路径把两者映射成 generic 502/401 且不处置账号状态，导致失效账号
// 持续参与调度并耗尽 failover 配额。本模块在 opencode 上游分支识别并处置。

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const (
	openCodeCreditsErrorType            = "CreditsError"
	openCodeCreditsNoPaymentMethod      = "No payment method"
	openCodeCreditsInsufficientBalance  = "Insufficient balance"
	openCodeBlockReasonNoPaymentMethod  = "opencode_no_payment_method"
	openCodeBlockReasonInsufficientBal  = "opencode_insufficient_balance"
)

// parseOpenCodeCreditsError 从 zen 错误响应体提取 CreditsError 类型与消息。
// 响应形状: {"type":"error","error":{"type":"CreditsError","message":"..."}}
func parseOpenCodeCreditsError(body []byte) (typ, message string) {
	typ = gjson.GetBytes(body, "error.type").String()
	if typ == "" {
		typ = gjson.GetBytes(body, "type").String()
	}
	if typ != openCodeCreditsErrorType {
		return "", ""
	}
	return typ, gjson.GetBytes(body, "error.message").String()
}

// handleOpenCodeAccountUpstreamError 处置 opencode 上游的账号级错误。
// 命中 CreditsError 时按语义停用账号并返回 true（调用方跳过通用账号处置）；
// 未命中返回 false（继续走通用 failover 路径）。
func (s *OpenAIGatewayService) handleOpenCodeAccountUpstreamError(
	ctx context.Context,
	account *Account,
	statusCode int,
	responseBody []byte,
) bool {
	if s == nil || account == nil || !account.IsOpenCodeUpstream() {
		return false
	}
	if statusCode != http.StatusUnauthorized && statusCode != http.StatusForbidden {
		return false
	}
	typ, msg := parseOpenCodeCreditsError(responseBody)
	if typ != openCodeCreditsErrorType {
		return false
	}
	switch {
	case strings.Contains(msg, openCodeCreditsNoPaymentMethod):
		slog.Warn("opencode_account_no_payment_method",
			"account_id", account.ID,
			"account_name", account.Name,
			"reason", openCodeBlockReasonNoPaymentMethod,
		)
		s.BlockAccountScheduling(account, time.Time{}, openCodeBlockReasonNoPaymentMethod)
		return true
	case strings.Contains(msg, openCodeCreditsInsufficientBalance):
		slog.Warn("opencode_account_insufficient_balance",
			"account_id", account.ID,
			"account_name", account.Name,
			"reason", openCodeBlockReasonInsufficientBal,
		)
		s.BlockAccountScheduling(account, time.Time{}, openCodeBlockReasonInsufficientBal)
		return true
	default:
		return false
	}
}
