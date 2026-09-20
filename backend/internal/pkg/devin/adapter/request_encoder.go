// request_encoder.go 把中间 llm.RequestMessages 投影为 Devin Connect 的
// GetChatMessageRequest wire 参数：metadata/completion 参数、会话轨迹 ID
// 派生、逐消息内容转换（文本/thinking/工具调用/工具结果/图片）、
// 工具调用-结果配对修复。响应方向的解码见 response_decoder.go。
//
// 转换逻辑移植 WncFht/devin2api 的 internal/adapter/devin/request_encoder.go
// （MIT），wire 编码改用本仓库 pkg/devin 的手写 protobuf（字段号与
// devin-connect 插件/devin 3000.10.21 抓包逐字节对齐）。
package adapter

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// buildRequestParams 把中间请求转换为 MarshalGetChatMessage 的入参。
// trajectory/cascade 按会话复用；step_index 走插件语义（0 → 缺席 → 2,3,…）。
// modelAssignmentJWT 由调用方在 router uid 经 AssignModel 解析后传入。
func buildRequestParams(request llm.RequestMessages, token, clientVersion, os, modelUID, assignmentJWT string) (devin.ChatRequestParams, llm.RequestRepairs, error) {
	var repairs llm.RequestRepairs
	trajectoryID, cascadeID := deriveSessionIDs(request)

	params := devin.ChatRequestParams{
		Token:         token,
		ClientVersion: clientVersion,
		OS:            os,
		// 上游 prompt 前缀缓存：system prompt 是稳定前缀。
		SystemPrompt: withToolDescriptions(request.SystemPrompt, request.Tools),
		ModelUID:     modelUID,
		TrajectoryID: trajectoryID,
		StepIndex:    nextStepIndex(trajectoryID),
		CascadeID:    cascadeID,

		MaxTokens:                request.MaxTokens,
		Temperature:              request.Temperature,
		TopP:                     request.TopP,
		TopK:                     request.TopK,
		StopPatterns:             request.StopSequences,
		DisableParallelToolCalls: request.DisableParallelToolCalls,
	}
	if request.Seed != nil {
		seed := uint64(*request.Seed)
		params.Seed = &seed
	}

	// 上游实测：option_name 合法值为 none/auto/required；Anthropic 的 "any"
	// 在本层已归一为 required。auto 不发送，与上游缺省行为一致。
	if choice := request.ToolChoice; choice != nil {
		switch choice.Mode {
		case llm.ToolChoiceNone, llm.ToolChoiceRequired:
			params.ToolChoiceOption = string(choice.Mode)
		case llm.ToolChoiceNamed:
			// 指名调用必须命中 tools 表：上游对不存在的目标只回模糊的
			// invalid_argument，本地提前报成可读错误。
			found := false
			for _, tool := range request.Tools {
				if tool.Name == choice.ToolName {
					found = true
					break
				}
			}
			if !found {
				return params, repairs, fmt.Errorf("invalid_argument: tool_choice names tool %q which is not in the tools list", choice.ToolName)
			}
			params.ToolChoiceTool = choice.ToolName
		}
	}

	// Devin/Cascade 只可靠接受「当前轮」图片；历史图进 Images 会 invalid_argument。
	// 当前轮 = 最后一条 AssistantMessage 之后的所有 user/tool 消息。
	// Anthropic 客户端常把 image 和 tool_result 放在同一条 user 消息里，
	// 解码后拆成 UserMessage + ToolResultMessage 两条；仅挂最后一条会丢失图片。
	lastAssistantIndex := -1
	for index, message := range request.Messages {
		if _, ok := message.(llm.AssistantMessage); ok {
			lastAssistantIndex = index
		}
	}
	for index, message := range request.Messages {
		converted, err := convertMessage(message, index > lastAssistantIndex, &repairs)
		if err != nil {
			return params, repairs, fmt.Errorf("message %d: %w", index, err)
		}
		// 完全空的助手消息会被跳过（上游见空回复退化），计入修复量。
		if _, isAssistant := message.(llm.AssistantMessage); isAssistant && len(converted) == 0 {
			repairs.DroppedEmptyAssistant++
		}
		params.Prompts = append(params.Prompts, converted...)
	}
	// 上游要求 call→result 紧邻配对：assistant 发出的每个 tool call 必须紧跟
	// 它的 TOOL 结果，否则 invalid_argument。客户端历史（OpenAI/Anthropic）是
	// 「全部调用 → 全部结果」的分组结构，这里按 call id 重排成交错配对。
	params.Prompts, repairs.ReorderedPrompts = pairToolCallsWithResults(params.Prompts)
	params.Prompts, repairs.DemotedOrphanResults = demoteOrphanToolResults(params.Prompts)
	for _, tool := range request.Tools {
		converted, err := convertToolDefinition(tool)
		if err != nil {
			return params, repairs, err
		}
		params.Tools = append(params.Tools, converted)
	}
	params.AssignmentJWT = assignmentJWT
	return params, repairs, nil
}

// convertMessage 将中间消息转为 Devin ChatPrompt。
// attachImages 为 true 时才把 ImageContent 写入 Images（仅最新用户轮）；历史图改成文本占位。
// repairs 累计转换中发生的静默修复（历史图剥离等）。
func convertMessage(message llm.Message, attachImages bool, repairs *llm.RequestRepairs) ([]devin.ChatPrompt, error) {
	switch message := message.(type) {
	case llm.UserMessage:
		return []devin.ChatPrompt{promptForContent(devin.SourceUser, message.Content, attachImages, repairs)}, nil
	case llm.AssistantMessage:
		// Wire 实证（chisel 3000.2.17 抓包）：一个助手回合合并为单条
		// prompt——prompt/thinking/signature/toolCalls 同体携带，无文本时
		// prompt 字段缺席；真实客户端从不产生相邻 SYSTEM 消息。拆成多条会
		// 在渲染上下文里引入回合边界，模型在「宣告文本」后采到 EOS 提前收轮。
		var signature, signatureType string
		var redacted bool
		var text, thinking strings.Builder
		var calls []llm.ToolCall
		for _, block := range message.Content {
			switch typed := block.(type) {
			case llm.TextContent:
				_, _ = text.WriteString(typed.Text)
			case llm.ThinkingContent:
				// 一条 assistant 消息可带多个 thinking 块（interleaved）；
				// wire 模型每 prompt 只有单份 thinking，顺序拼接、签名取最后非空。
				if thinking.Len() > 0 && typed.Thinking != "" {
					_, _ = thinking.WriteString("\n")
				}
				_, _ = thinking.WriteString(typed.Thinking)
				if typed.ThinkingSignature != "" {
					signature = typed.ThinkingSignature
					signatureType = typed.SignatureType
				}
				redacted = redacted || typed.Redacted
			case llm.ToolCall:
				calls = append(calls, typed)
			}
		}
		// 完全空的助手消息会诱发上游反复返回空回复，跳过。
		if text.Len() == 0 && len(calls) == 0 {
			return nil, nil
		}
		prompt := devin.ChatPrompt{Source: devin.SourceAssistant}
		if text.Len() > 0 {
			prompt.Text = text.String()
		}
		// signature_type 与 output_id 必须随签名原样回传：实测错配
		// signature_type 会触发上游 invalid_argument。
		if thinking.Len() > 0 || redacted || signature != "" || message.OutputID != "" {
			if thinking.Len() > 0 {
				prompt.Thinking = thinking.String()
			}
			prompt.Signature = signature
			prompt.SignatureType = signatureType
			prompt.OutputID = message.OutputID
			prompt.ThinkingRedacted = redacted
		}
		for _, call := range calls {
			toolCall := devin.ChatToolCall{ID: call.ID, Name: call.Name}
			if call.Custom {
				// custom/freeform 调用的参数体不是 JSON，走 invalid_json_str
				// 通道原样回传（上游对该字段实测容忍非 JSON 原文）。
				toolCall.Custom = true
				toolCall.ArgumentsJSON = string(call.Arguments)
			} else {
				toolCall.ArgumentsJSON = string(call.Arguments)
			}
			prompt.ToolCalls = append(prompt.ToolCalls, toolCall)
		}
		return []devin.ChatPrompt{prompt}, nil
	case llm.ToolResultMessage:
		prompt := promptForContent(devin.SourceTool, message.Content, attachImages, repairs)
		if prompt.Text == "" {
			// 上游不接受空的工具结果文本，对齐 WindsurfAPI 的占位。
			prompt.Text = "[tool result]"
		}
		prompt.ToolCallID = message.ToolCallID
		prompt.ToolResultIsErr = message.IsError
		return []devin.ChatPrompt{prompt}, nil
	default:
		return nil, fmt.Errorf("unsupported message type %T", message)
	}
}

// pairToolCallsWithResults 把「连续调用消息 + 连续结果消息」的分组序列
// 重排为 call_i, result_i, call_j, result_j 的交错序列。
// 已配对的交错序列保持不变；找不到匹配结果的调用原样保留位置。
// 第二个返回值是位置发生变化的 prompt 数（已交错的历史为 0）。
func pairToolCallsWithResults(prompts []devin.ChatPrompt) ([]devin.ChatPrompt, int) {
	isCallPrompt := func(p devin.ChatPrompt) bool {
		return p.Source == devin.SourceAssistant && len(p.ToolCalls) > 0
	}
	isResultPrompt := func(p devin.ChatPrompt) bool {
		return p.Source == devin.SourceTool
	}
	var out []devin.ChatPrompt
	for i := 0; i < len(prompts); {
		if !isCallPrompt(prompts[i]) {
			out = append(out, prompts[i])
			i++
			continue
		}
		var calls []devin.ChatPrompt
		for i < len(prompts) && isCallPrompt(prompts[i]) {
			calls = append(calls, prompts[i])
			i++
		}
		byID := make(map[string]*devin.ChatPrompt)
		j := i
		for j < len(prompts) && isResultPrompt(prompts[j]) {
			byID[prompts[j].ToolCallID] = &prompts[j]
			j++
		}
		consumed := make(map[string]struct{}, len(calls))
		for _, callPrompt := range calls {
			out = append(out, callPrompt)
			for _, call := range callPrompt.ToolCalls {
				id := call.ID
				if result, ok := byID[id]; ok {
					out = append(out, *result)
					consumed[id] = struct{}{}
					// 重复 call-id 实测被上游容忍但按位置绑定：同 id 的第二个
					// 调用不应再挂到同一份结果上，消费后即删除。
					delete(byID, id)
				}
			}
		}
		// 未能配对的孤立结果按原序保留，不丢消息。
		for k := i; k < j; k++ {
			if _, ok := consumed[prompts[k].ToolCallID]; !ok {
				out = append(out, prompts[k])
			}
		}
		i = j
	}
	return out, countMovedPrompts(prompts, out)
}

// countMovedPrompts 统计重排后位置发生变化的 prompt 数，作为配对修复量。
// 以值索引（同切片内 prompt 不可区分），用内容指纹比较近似——prompt 指针
// 唯一性在我们的切片语义下不成立，改用 (source,text,toolCallID) 序对计数。
func countMovedPrompts(in, out []devin.ChatPrompt) int {
	// 与 devin2api 等价的指针语义用值对 (index) 比较替代：为每个 out 元素
	// 找其源下标。值可能重复（两条相同 prompt），用消耗队列保序。
	type key struct {
		source     int
		text       string
		toolCallID string
		calls      int
	}
	queues := make(map[key][]int, len(in))
	for index, p := range in {
		k := key{p.Source, p.Text, p.ToolCallID, len(p.ToolCalls)}
		queues[k] = append(queues[k], index)
	}
	moved := 0
	for position, p := range out {
		k := key{p.Source, p.Text, p.ToolCallID, len(p.ToolCalls)}
		q := queues[k]
		if len(q) == 0 {
			moved++
			continue
		}
		original := q[0]
		queues[k] = q[1:]
		if original != position {
			moved++
		}
	}
	return moved
}

// demoteOrphanToolResults 把找不到对应 tool call 的孤立 TOOL 结果
// （客户端压缩丢掉 function_call 时产生）降级为 USER 文本消息。
// 上游对无配对的 TOOL prompt 返回 invalid_argument；降级保住结果内容。
// 第二个返回值是被降级的结果数。
func demoteOrphanToolResults(prompts []devin.ChatPrompt) ([]devin.ChatPrompt, int) {
	callIDs := make(map[string]struct{})
	for _, prompt := range prompts {
		for _, call := range prompt.ToolCalls {
			callIDs[call.ID] = struct{}{}
		}
	}
	demotedCount := 0
	for index, prompt := range prompts {
		if prompt.Source != devin.SourceTool {
			continue
		}
		if _, ok := callIDs[prompt.ToolCallID]; ok {
			continue
		}
		slog.Warn("devin: demoted orphan tool result to user text", "tool_call_id", prompt.ToolCallID)
		demotedCount++
		prompts[index] = devin.ChatPrompt{
			Source: devin.SourceUser,
			Text:   "[tool result, original call lost]\n" + prompt.Text,
			Images: prompt.Images,
		}
	}
	return prompts, demotedCount
}

func promptForContent(source int, content []llm.Content, attachImages bool, repairs *llm.RequestRepairs) devin.ChatPrompt {
	prompt := devin.ChatPrompt{Source: source}
	var text strings.Builder
	for _, block := range content {
		switch block := block.(type) {
		case llm.TextContent:
			_, _ = text.WriteString(block.Text)
		case llm.ThinkingContent:
			prompt.Thinking = block.Thinking
			prompt.Signature = block.ThinkingSignature
			prompt.SignatureType = block.SignatureType
			prompt.ThinkingRedacted = block.Redacted
		case llm.ImageContent:
			if !attachImages {
				// 与 WindsurfAPI 一致：历史图不进 Images，避免上游 invalid_argument。
				if text.Len() > 0 {
					_ = text.WriteByte('\n')
				}
				_, _ = text.WriteString("[Image omitted from history]")
				repairs.OmittedHistoryImages++
				continue
			}
			prompt.Images = append(prompt.Images, devin.ChatImage{
				Data:     block.Data,
				MimeType: block.MIMEType,
			})
		}
	}
	prompt.Text = text.String()
	return prompt
}
