package inbox

import (
	"encoding/json"
	"time"
)

// 字段长度与大小限制。与 184_add_general_inbox.sql 中的列定义 / CHECK 约束保持一致。
const (
	// MaxNamespaceLen 是 namespace 的最大长度（VARCHAR(64)）。
	MaxNamespaceLen = 64
	// MaxDedupKeyLen 是 dedup_key 的最大长度（VARCHAR(128)）。
	MaxDedupKeyLen = 128
	// MaxPayloadBytes 是 payload 序列化后的字节上限（CHECK octet_length <= 8192）。
	MaxPayloadBytes = 8192
)

// seq 编码：高位毫秒时间戳 << SeqTimestampShift | 同毫秒内 INCR 计数。
// 2^20 ≈ 100 万，远超单毫秒实际并发上限；时间戳单调保证跨毫秒严格递增。
const SeqTimestampShift = 20

// Scope 标识消息投递范围。
type Scope string

const (
	// ScopeDirect 单播：投递给指定用户，fan-out on write。
	ScopeDirect Scope = "direct"
	// ScopeBroadcast 广播：全局一份，读取时按 targeting 匹配，fan-out on read。
	ScopeBroadcast Scope = "broadcast"
)

// ClientType 标识连接来源端类型，作为连接元数据（v1 用于踢出提示文案，未来可作
// 多端并存的隔离维度）。
type ClientType string

const (
	ClientWeb     ClientType = "web"
	ClientIOS     ClientType = "ios"
	ClientAndroid ClientType = "android"
	ClientDesktop ClientType = "desktop"
	ClientUnknown ClientType = "unknown"
)

// NormalizeClientType 把客户端上报的原始字符串规整为已知枚举，未知一律归为
// ClientUnknown，避免非法值渗入。
func NormalizeClientType(raw string) ClientType {
	switch ClientType(raw) {
	case ClientWeb, ClientIOS, ClientAndroid, ClientDesktop:
		return ClientType(raw)
	default:
		return ClientUnknown
	}
}

// DirectMessage 是 direct_messages 表的一行。
type DirectMessage struct {
	Seq       int64           `json:"seq"`
	UserID    int64           `json:"user_id"`
	Namespace string          `json:"namespace"`
	DedupKey  string          `json:"dedup_key"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// Broadcast 是 broadcasts 表的一行。
type Broadcast struct {
	Seq       int64           `json:"seq"`
	Namespace string          `json:"namespace"`
	DedupKey  string          `json:"dedup_key"`
	Targeting json.RawMessage `json:"targeting"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// Message 是单播/广播统一后的对外视图，用于 catchup 响应与 WS 推送。
// 客户端算法完全不区分 scope，只按 seq 处理。
type Message struct {
	Seq       int64           `json:"seq"`
	Scope     Scope           `json:"scope"`
	Namespace string          `json:"namespace"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// PushMessage 是 WebSocket 下行的一帧。Type="notification" 为消息通知；
// Type="kicked" 为单连接策略下的踢出控制帧（仅 Reason 有意义）。
type PushMessage struct {
	Type      string          `json:"type"`
	Seq       int64           `json:"seq,omitempty"`
	Scope     Scope           `json:"scope,omitempty"`
	Namespace string          `json:"namespace,omitempty"`
	Unacked   []int64         `json:"unacked,omitempty"`   // 服务端认为客户端尚未 ack 的 seq 列表
	Truncated bool            `json:"truncated,omitempty"` // unacked 被 LIMIT 截断时为 true，触发客户端拉取
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"created_at,omitempty"`
	Reason    string          `json:"reason,omitempty"` // 仅 Type="kicked" 时有意义
}

// PushMessage.Type 的取值。
const (
	// PushTypeNotification 消息通知帧。
	PushTypeNotification = "notification"
	// PushTypeKicked 踢出控制帧（新连接替换旧连接）。
	PushTypeKicked = "kicked"
)

// CatchupResult 是 catchup 拉取的结果。
type CatchupResult struct {
	Messages []Message `json:"messages"`
	AckedSeq int64     `json:"acked_seq"` // 客户端持久化为 local_ack_seq
	HasMore  bool      `json:"has_more"`  // 候选未耗尽，客户端可继续 catchup
}

// PublishDirectInput 是发布单播消息的入参。
type PublishDirectInput struct {
	RecipientID int64
	Namespace   string
	DedupKey    string
	Payload     json.RawMessage
}

// PublishBroadcastInput 是发布广播消息的入参。
type PublishBroadcastInput struct {
	Namespace string
	DedupKey  string
	Targeting json.RawMessage
	Payload   json.RawMessage
}
