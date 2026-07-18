// Package inbox 实现通用信箱（general-inbox）：账号级站内消息系统，支持单播
// （direct）与广播（broadcast）两类消息，客户端通过 WebSocket 实时接收并以累积
// ack 水位标记已读，断线时通过 catchup 拉取补齐。
//
// 设计文档：openspec/changes/general-inbox/design.md
//
// 本文件集中定义模块对外暴露的稳定错误。所有错误都是 *infraerrors.ApplicationError，
// 携带 HTTP 状态码与稳定的 reason 字符串，便于 handler 直接透传给前端并保持契约稳定。
package inbox

import infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

// 稳定 reason 前缀，全部以 INBOX_ 打头，避免与其它模块冲突。
const (
	reasonInvalidNamespace = "INBOX_INVALID_NAMESPACE"
	reasonInvalidDedupKey  = "INBOX_INVALID_DEDUP_KEY"
	reasonInvalidPayload   = "INBOX_INVALID_PAYLOAD"
	reasonPayloadTooLarge  = "INBOX_PAYLOAD_TOO_LARGE"
	reasonInvalidTargeting = "INBOX_INVALID_TARGETING"
	reasonInvalidRecipient = "INBOX_INVALID_RECIPIENT"
	reasonSeqExhausted     = "INBOX_SEQ_ALLOC_EXHAUSTED"
	reasonSeqAllocFailed   = "INBOX_SEQ_ALLOC_FAILED"
	reasonInvalidAck       = "INBOX_INVALID_ACK"
)

// 校验类错误（400）。发布 / catchup / ack 入参不合法时返回。
var (
	// ErrInvalidNamespace 表示 namespace 为空或超过长度限制。
	ErrInvalidNamespace = infraerrors.BadRequest(reasonInvalidNamespace, "namespace 不能为空且长度需 <= 64")
	// ErrInvalidDedupKey 表示 dedup_key 为空或超过长度限制。
	ErrInvalidDedupKey = infraerrors.BadRequest(reasonInvalidDedupKey, "dedup_key 不能为空且长度需 <= 128")
	// ErrInvalidPayload 表示 payload 不是合法 JSON 或为空。
	ErrInvalidPayload = infraerrors.BadRequest(reasonInvalidPayload, "payload 必须是非空的合法 JSON")
	// ErrPayloadTooLarge 表示 payload 超过 8KB 上限。
	ErrPayloadTooLarge = infraerrors.BadRequest(reasonPayloadTooLarge, "payload 超过 8KB 上限")
	// ErrInvalidTargeting 表示广播 targeting 表达式不合法。
	ErrInvalidTargeting = infraerrors.BadRequest(reasonInvalidTargeting, "targeting 表达式不合法")
	// ErrInvalidRecipient 表示单播 recipient 用户 ID 不合法。
	ErrInvalidRecipient = infraerrors.BadRequest(reasonInvalidRecipient, "单播消息 recipient 用户 ID 必须为正")
	// ErrInvalidAck 表示 ack seq 不合法（非正数）。
	ErrInvalidAck = infraerrors.BadRequest(reasonInvalidAck, "ack seq 必须为正整数")
)

// seq 分配相关错误（500）。属于基础设施异常，业务侧一般应记 warning 并降级。
var (
	// ErrSeqExhausted 表示连续多次分配的 seq 都与已有记录主键冲突，极罕见，需要告警。
	ErrSeqExhausted = infraerrors.InternalServer(reasonSeqExhausted, "seq 分配连续冲突，已耗尽重试次数")
	// ErrSeqAllocFailed 表示 Redis seq 分配调用失败。
	ErrSeqAllocFailed = infraerrors.InternalServer(reasonSeqAllocFailed, "seq 分配失败")
)
