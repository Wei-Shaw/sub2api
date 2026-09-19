// chat.go 实现 GetChatMessage 的 wire 级请求编码与响应帧解码。
// 编码形态逐字段对齐 devin-connect stream.ts（devin 3000.10.21 抓包）：
//
//	request  = {1: metadata, 2: system+tools 描述, 3: prompts...,
//	            7: REQUEST_CASCADE(5), 8: completion, 10: tools...,
//	            [12: tool_choice], 15: trajectory, 16: cascade_id,
//	            20: PLANNER_DEFAULT(1), 21: model_uid, [26: assignment_jwt]}
//
// 抓包确认 13/18/22（cache options / provider_source / execution_id）
// 在真实 swe-2-high 与 swe-1-6-fast 抓包中从不出现——不发送。
package devin

import (
	"strings"
)

// 消息来源枚举（ExaCodeiumCommonPb_ChatMessageSource）。
const (
	SourceUser      = 1
	SourceAssistant = 2 // 上游命名 SYSTEM
	SourceTool      = 4
)

// 请求枚举常量。
const (
	RequestTypeCascade = 5 // CHAT_MESSAGE_REQUEST_TYPE_CASCADE
	PlannerModeDefault = 1 // CONVERSATIONAL_PLANNER_MODE_DEFAULT
	TrajectoryCascade  = 4 // CORTEX_TRAJECTORY_TYPE_CASCADE
	StepTypeUserInput  = 14
)

// StopReason 枚举值（ExaCodeiumCommonPb_StopReason）。
const (
	StopReasonUnspecified            = 0
	StopReasonIncomplete             = 1
	StopReasonStopPattern            = 2
	StopReasonMaxTokens              = 3
	StopReasonMinLogProb             = 4
	StopReasonMaxNewlines            = 5
	StopReasonExitScope              = 6
	StopReasonNonfiniteLogitOrProb   = 7
	StopReasonFirstNonWhitespaceLine = 8
	StopReasonPartial                = 9
	StopReasonFunctionCall           = 10
	StopReasonContentFilter          = 11
	StopReasonNonInsertion           = 12
	StopReasonError                  = 13
)

// ChatToolCall 是一次工具调用的 wire 投影。
type ChatToolCall struct {
	ID   string
	Name string
	// ArgumentsJSON 是 JSON 参数原文（custom 调用时为 invalid_json_str 载荷）。
	ArgumentsJSON string
	// Custom 为 true 时走 invalid_json_str(f4)+is_custom_tool_call(f6) 通道。
	Custom bool
}

// ChatImage 是一张图片的 wire 投影（纯 base64 + mime_type）。
type ChatImage struct {
	Data     string
	MimeType string
}

// ChatPrompt 是一条消息的 wire 投影。
type ChatPrompt struct {
	Source           int
	Text             string
	ToolCalls        []ChatToolCall
	ToolCallID       string
	ToolResultIsErr  bool
	Images           []ChatImage
	Thinking         string
	Signature        string
	SignatureType    string
	ThinkingRedacted bool
	OutputID         string
}

// ChatToolDef 是工具定义的 wire 投影。
type ChatToolDef struct {
	Name        string
	Description string
	SchemaJSON  string
}

// ChatRequestParams 是 MarshalGetChatMessage 的全部输入。
type ChatRequestParams struct {
	Token         string
	ClientVersion string
	OS            string

	SystemPrompt string
	Prompts      []ChatPrompt
	Tools        []ChatToolDef
	// ToolChoiceOption 取 "none"/"required"（option_name 通道）；
	// ToolChoiceTool 取工具名（tool_name 通道）。两者互斥，都空则不发。
	ToolChoiceOption string
	ToolChoiceTool   string

	MaxTokens    *int
	Temperature  *float64
	TopP         *float64
	TopK         *int
	StopPatterns []string
	Seed         *uint64

	DisableParallelToolCalls bool

	ModelUID      string
	AssignmentJWT string
	TrajectoryID  string
	StepIndex     int // 首个请求为 0 → 字段缺席（抓包形态）
	CascadeID     string
}

// MarshalGetChatMessage 构造 GetChatMessage 请求体（裸 protobuf；
// 发送时由 NewStreamRequest 包 envelope）。
func MarshalGetChatMessage(p ChatRequestParams) []byte {
	var body []byte
	body = appendMessage(body, 1, BuildMetadata(p.Token, p.ClientVersion, p.OS, false))
	body = appendString(body, 2, p.SystemPrompt)
	for _, prompt := range p.Prompts {
		body = appendMessage(body, 3, encodePrompt(prompt))
	}
	body = appendVarintField(body, 7, RequestTypeCascade)
	body = appendMessage(body, 8, encodeCompletion(p))
	for _, tool := range p.Tools {
		body = appendMessage(body, 10, encodeToolDef(tool))
	}
	if p.DisableParallelToolCalls {
		body = appendBool(body, 11, true)
	}
	if p.ToolChoiceOption != "" {
		body = appendMessage(body, 12, appendString(nil, 1, p.ToolChoiceOption))
	} else if p.ToolChoiceTool != "" {
		body = appendMessage(body, 12, appendString(nil, 2, p.ToolChoiceTool))
	}
	body = appendMessage(body, 15, encodeTrajectory(p.TrajectoryID, p.StepIndex))
	body = appendString(body, 16, p.CascadeID)
	// 13/18/22 抓包确认从不出现，不发送。
	body = appendVarintField(body, 20, PlannerModeDefault)
	body = appendString(body, 21, p.ModelUID)
	if p.AssignmentJWT != "" {
		body = appendString(body, 26, p.AssignmentJWT)
	}
	return body
}

// encodePrompt 编码 ChatMessagePrompt（f1 messageId, f2 source, f3 text,
// f6 toolCalls, f7 toolCallId, f9 toolError, f10 images, f11 thinking,
// f12 signature, f13 redacted, f15 output_id, f18 signature_type）。
func encodePrompt(p ChatPrompt) []byte {
	var buf []byte
	buf = appendString(buf, 1, UUID())
	buf = appendVarintField(buf, 2, uint64(p.Source))
	if p.Text != "" {
		buf = appendString(buf, 3, p.Text)
	}
	for _, call := range p.ToolCalls {
		buf = appendMessage(buf, 6, encodeToolCall(call))
	}
	if p.ToolCallID != "" {
		buf = appendString(buf, 7, p.ToolCallID)
	}
	if p.ToolResultIsErr {
		buf = appendBool(buf, 9, true)
	}
	for _, img := range p.Images {
		buf = appendMessage(buf, 10, encodeImage(img))
	}
	if p.Thinking != "" {
		buf = appendString(buf, 11, p.Thinking)
	}
	if p.Signature != "" {
		buf = appendString(buf, 12, p.Signature)
	}
	if p.ThinkingRedacted {
		buf = appendBool(buf, 13, true)
	}
	if p.OutputID != "" {
		buf = appendString(buf, 15, p.OutputID)
	}
	if p.SignatureType != "" {
		buf = appendString(buf, 18, p.SignatureType)
	}
	return buf
}

// encodeToolCall 编码 ChatToolCall（f1 id, f2 name, f3 arguments_json |
// f4 invalid_json_str + f6 is_custom_tool_call）。
func encodeToolCall(call ChatToolCall) []byte {
	var buf []byte
	buf = appendString(buf, 1, call.ID)
	buf = appendString(buf, 2, call.Name)
	if call.Custom {
		buf = appendString(buf, 4, call.ArgumentsJSON)
		buf = appendBool(buf, 6, true)
	} else {
		args := call.ArgumentsJSON
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		buf = appendString(buf, 3, args)
	}
	return buf
}

// encodeImage 编码 ImageData（f1 base64_data, f2 mime_type）。
// data: URL 前缀在此剥离（上游只收裸 base64）。
func encodeImage(img ChatImage) []byte {
	data := img.Data
	if strings.HasPrefix(data, "data:") {
		if i := strings.IndexByte(data, ','); i >= 0 {
			data = data[i+1:]
		}
	}
	mime := img.MimeType
	if mime == "" {
		mime = "image/png"
	}
	var buf []byte
	buf = appendString(buf, 1, data)
	buf = appendString(buf, 2, mime)
	return buf
}

// encodeToolDef 编码 ChatToolDefinition（f1 name, f2 description≤6995,
// f3 json_schema_string）。
func encodeToolDef(tool ChatToolDef) []byte {
	desc := tool.Description
	if len(desc) > 6995 {
		desc = desc[:6995]
	}
	var buf []byte
	buf = appendString(buf, 1, tool.Name)
	buf = appendString(buf, 2, desc)
	buf = appendString(buf, 3, tool.SchemaJSON)
	return buf
}

// encodeCompletion 编码 CompletionConfiguration。
// 抓包（swe-2-high 真实回合）：num=1, max_tokens=128000, max_newlines=400,
// temperature=1.0, top_k=40, top_p=0.95 以 f32→f64 存储（0.949999988079071）。
// temp=0/top_p=0 会被 swe-2 系以 invalid_argument 拒绝——不发送零值。
func encodeCompletion(p ChatRequestParams) []byte {
	maxTokens := uint64(128000)
	if p.MaxTokens != nil && *p.MaxTokens > 0 {
		maxTokens = uint64(*p.MaxTokens)
	}
	temperature := 1.0
	if p.Temperature != nil && *p.Temperature > 0 {
		temperature = *p.Temperature
	}
	topP := float64(float32(0.95)) // f32→f64 = 0.949999988079071
	if p.TopP != nil && *p.TopP > 0 {
		topP = *p.TopP
	}
	topK := uint64(40)
	if p.TopK != nil && *p.TopK > 0 {
		topK = uint64(*p.TopK)
	}
	var buf []byte
	buf = appendVarintField(buf, 1, 1)
	buf = appendVarintField(buf, 2, maxTokens)
	buf = appendVarintField(buf, 3, 400)
	buf = appendDouble(buf, 5, temperature)
	buf = appendVarintField(buf, 7, topK)
	buf = appendDouble(buf, 8, topP)
	for _, pattern := range p.StopPatterns {
		buf = appendString(buf, 9, pattern)
	}
	if p.Seed != nil {
		buf = appendVarintField(buf, 10, *p.Seed)
	}
	return buf
}

// encodeTrajectory 编码 CortexTrajectoryReference（f1 trajectory_id,
// [f2 step_index>0], f3 CORTEX_TRAJECTORY_TYPE_CASCADE(4),
// f4 CORTEX_STEP_TYPE_USER_INPUT(14)）。
func encodeTrajectory(id string, stepIndex int) []byte {
	var buf []byte
	buf = appendString(buf, 1, id)
	if stepIndex > 0 {
		buf = appendVarintField(buf, 2, uint64(stepIndex))
	}
	buf = appendVarintField(buf, 3, TrajectoryCascade)
	buf = appendVarintField(buf, 4, StepTypeUserInput)
	return buf
}

// ---------------------------------------------------------------------------
// 响应帧解码
// ---------------------------------------------------------------------------

// ChatUsage 是一帧 usage（ExaCodeiumCommonPb_ModelUsageStats 子集）。
type ChatUsage struct {
	Input      int64
	Output     int64
	CacheWrite int64
	CacheRead  int64
	ModelUID   string
	// ProviderRefusal 表示供应商拒绝了本次请求。
	ProviderRefusal bool
	// APIProvider / ProviderRequestID / ProviderMessageID / BillingModelUID
	// 是 provider 侧追踪信息（排障时可直接报给上游）。
	APIProvider       int64
	ProviderRequestID string
	ProviderMessageID string
	BillingModelUID   string
}

// ChatToolDelta 是一帧工具调用增量。
type ChatToolDelta struct {
	ID      string
	Name    string
	Args    string
	HasArgs bool
	Custom  bool
}

// ChatResponseFrame 是一个 GetChatMessageResponse 帧的解码结果。
type ChatResponseFrame struct {
	MessageID          string
	DeltaText          string
	DeltaTokens        int64
	StopReason         int
	ToolDeltas         []ChatToolDelta
	Usage              *ChatUsage
	DeltaThinking      string
	DeltaSignature     string
	ThinkingRedacted   bool
	OutputID           string
	RequestID          string
	DeltaSignatureType string
	ActualModelUID     string
	Phase              string
	TimestampUnixMS    int64
}

// DecodeChatResponseFrame 解析一帧 GetChatMessageResponse。
func DecodeChatResponseFrame(payload []byte) *ChatResponseFrame {
	frame := &ChatResponseFrame{}
	iterFields(payload, func(f protoField) bool {
		switch f.num {
		case 1:
			frame.MessageID = f.string()
		case 2: // timestamp{1:seconds, 2:nanos}
			var seconds, nanos int64
			iterFields(f.bytes(), func(inner protoField) bool {
				if inner.num == 1 {
					seconds = inner.int()
				}
				if inner.num == 2 {
					nanos = inner.int()
				}
				return true
			})
			frame.TimestampUnixMS = seconds*1000 + nanos/1e6
		case 3:
			frame.DeltaText = f.string()
		case 4:
			frame.DeltaTokens = f.int()
		case 5:
			frame.StopReason = int(f.int())
		case 6:
			frame.ToolDeltas = append(frame.ToolDeltas, decodeToolDelta(f.bytes()))
		case 7:
			frame.Usage = decodeUsage(f.bytes())
		case 9:
			frame.DeltaThinking = f.string()
		case 10:
			frame.DeltaSignature = f.string()
		case 11:
			frame.ThinkingRedacted = f.bool()
		case 15:
			frame.OutputID = f.string()
		case 17:
			frame.RequestID = f.string()
		case 21:
			frame.DeltaSignatureType = f.string()
		case 23:
			frame.ActualModelUID = f.string()
		case 25:
			frame.Phase = f.string()
		}
		return true
	})
	return frame
}

func decodeToolDelta(buf []byte) ChatToolDelta {
	var delta ChatToolDelta
	iterFields(buf, func(f protoField) bool {
		switch f.num {
		case 1:
			delta.ID = f.string()
		case 2:
			delta.Name = f.string()
		case 3:
			delta.Args = f.string()
			delta.HasArgs = true
		case 4:
			delta.Args = f.string()
			delta.HasArgs = true
			delta.Custom = true
		case 6:
			delta.Custom = delta.Custom || f.bool()
		}
		return true
	})
	return delta
}

func decodeUsage(buf []byte) *ChatUsage {
	usage := &ChatUsage{}
	var responseHeader map[string]string
	iterFields(buf, func(f protoField) bool {
		switch f.num {
		case 2:
			usage.Input = f.int()
		case 3:
			usage.Output = f.int()
		case 4:
			usage.CacheWrite = f.int()
		case 5:
			usage.CacheRead = f.int()
		case 6:
			usage.APIProvider = f.int()
		case 7:
			usage.ProviderMessageID = f.string()
		case 8: // map<string,string> response_header
			var key, value string
			iterFields(f.bytes(), func(kv protoField) bool {
				if kv.num == 1 {
					key = kv.string()
				}
				if kv.num == 2 {
					value = kv.string()
				}
				return true
			})
			if responseHeader == nil {
				responseHeader = make(map[string]string)
			}
			responseHeader[key] = value
		case 9:
			usage.ModelUID = f.string()
		case 10:
			usage.BillingModelUID = f.string()
		case 12:
			usage.ProviderRefusal = f.bool()
		}
		return true
	})
	usage.ProviderRequestID = responseHeader["x-request-id"]
	return usage
}

// MapStopReason 把上游 StopReason 枚举归一为语义名：
// "stop" / "length" / "toolUse" / "contentFilter" / "error"。
// 与 devin2api mapStopReason 一致（比插件多覆盖 CONTENT_FILTER）。
func MapStopReason(reason int) string {
	switch reason {
	case StopReasonMaxTokens, StopReasonMaxNewlines, StopReasonIncomplete, StopReasonPartial:
		return "length"
	case StopReasonFunctionCall:
		return "toolUse"
	case StopReasonContentFilter:
		return "contentFilter"
	case StopReasonError, StopReasonNonfiniteLogitOrProb:
		return "error"
	case StopReasonStopPattern, StopReasonMinLogProb, StopReasonExitScope,
		StopReasonFirstNonWhitespaceLine, StopReasonNonInsertion:
		return "stop"
	default:
		return "stop"
	}
}
