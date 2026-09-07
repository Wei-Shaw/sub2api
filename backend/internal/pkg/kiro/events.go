package kiro

import (
	"encoding/json"
	"fmt"
)

// EventKind 是 Kiro event-stream 的事件语义分类。
type EventKind string

const (
	EventAssistantResponse EventKind = "assistantResponseEvent"
	EventToolUse           EventKind = "toolUseEvent"
	EventMetadata          EventKind = "metadataEvent"
	EventMetering          EventKind = "meteringEvent"
	EventContextUsage      EventKind = "contextUsageEvent"
	EventCodeReference     EventKind = "codeReferenceEvent"
	EventException         EventKind = "exception"
	EventUnknown           EventKind = "unknown"
)

// AssistantResponse 是助手输出的一个文本增量。
type AssistantResponse struct {
	Content string `json:"content"`
	ModelID string `json:"modelId"`
}

// ToolUse 是工具调用的一个增量。
// Input 是 JSON 字符串的**分片**，需累积到 Stop 为 true 才是完整参数。
type ToolUse struct {
	Name      string `json:"name"`
	ToolUseID string `json:"toolUseId"`
	Input     string `json:"input"`
	Stop      bool   `json:"stop"`
}

// Metadata 承载上游的终止原因。
//
// 注意：只有 Kiro-Go 处理了这个事件（proxy/kiro.go:677），kiro2cc-proxy 的事件
// 枚举中没有它。漏掉会导致 stop_reason 永远退化为 end_turn。
type Metadata struct {
	StopReason string `json:"stopReason"`
	// StopReasonSnake 兼容蛇形键名的上游变体。
	StopReasonSnake string `json:"stop_reason"`
}

// Metering 是计费事件。Usage 是消耗的 credits；
// 两个 cache 字段是上游给出的**真实** token 数，不需要估算。
type Metering struct {
	Unit                     string  `json:"unit"`
	UnitPlural               string  `json:"unitPlural"`
	Usage                    float64 `json:"usage"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens"`
}

// ContextUsage 是上下文占用比例。
type ContextUsage struct {
	Percentage float64 `json:"contextUsagePercentage"`
}

// Exception 是上游异常帧。
type Exception struct {
	Type    string `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Event 是解析后的具名事件。Kind 决定哪个指针字段非空。
type Event struct {
	Kind         EventKind
	Assistant    *AssistantResponse
	ToolUse      *ToolUse
	Metadata     *Metadata
	Metering     *Metering
	ContextUsage *ContextUsage
	Exception    *Exception
}

// ParseEvent 把一个帧解析为具名事件。
// 未知事件类型返回 EventUnknown 而非错误 —— 上游新增事件不应中断流。
func ParseEvent(f *Frame) (Event, error) {
	if f == nil {
		return Event{Kind: EventUnknown}, nil
	}

	if f.MessageType() == "exception" || f.ExceptionType() != "" {
		ex := &Exception{Type: f.ExceptionType()}
		if len(f.Payload) > 0 {
			if err := json.Unmarshal(f.Payload, ex); err != nil {
				// 异常帧的 payload 不一定是 JSON，退化为原文。
				ex.Message = string(f.Payload)
			}
		}
		return Event{Kind: EventException, Exception: ex}, nil
	}

	kind := EventKind(f.EventType())
	switch kind {
	case EventAssistantResponse:
		var v AssistantResponse
		if err := json.Unmarshal(f.Payload, &v); err != nil {
			return Event{}, fmt.Errorf("kiro: decode assistantResponseEvent: %w", err)
		}
		return Event{Kind: kind, Assistant: &v}, nil

	case EventToolUse:
		var v ToolUse
		if err := json.Unmarshal(f.Payload, &v); err != nil {
			return Event{}, fmt.Errorf("kiro: decode toolUseEvent: %w", err)
		}
		return Event{Kind: kind, ToolUse: &v}, nil

	case EventMetadata:
		var v Metadata
		if err := json.Unmarshal(f.Payload, &v); err != nil {
			return Event{}, fmt.Errorf("kiro: decode metadataEvent: %w", err)
		}
		if v.StopReason == "" {
			v.StopReason = v.StopReasonSnake
		}
		return Event{Kind: kind, Metadata: &v}, nil

	case EventMetering:
		var v Metering
		if err := json.Unmarshal(f.Payload, &v); err != nil {
			return Event{}, fmt.Errorf("kiro: decode meteringEvent: %w", err)
		}
		return Event{Kind: kind, Metering: &v}, nil

	case EventContextUsage:
		var v ContextUsage
		if err := json.Unmarshal(f.Payload, &v); err != nil {
			return Event{}, fmt.Errorf("kiro: decode contextUsageEvent: %w", err)
		}
		return Event{Kind: kind, ContextUsage: &v}, nil

	case EventCodeReference:
		// 开源许可合规追踪，与网关无关，只做识别不解析内容。
		return Event{Kind: kind}, nil

	default:
		return Event{Kind: EventUnknown}, nil
	}
}
