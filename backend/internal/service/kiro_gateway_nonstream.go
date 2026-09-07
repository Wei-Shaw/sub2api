package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/gin-gonic/gin"
)

// nonStreamToClient 处理客户端 stream:false 的请求（I3）。
//
// Kiro 上游本身只支持流式返回（event-stream），没有原生的非流式端点——本
// 网关对外呈现的是 Anthropic 兼容协议，客户端有权利发 stream:false 并期待
// 一次性收到完整 JSON。做法是照常消费上游的流式响应，但不逐块 SSE 写给
// 客户端，而是把全部事件收集起来，最后折叠成一个 *apicompat.AnthropicResponse
// 一次性写出。这与 Antigravity 解决同一个问题的
// handleClaudeStreamToNonStreaming/collectClaudeStreamResponse 是同一个思路
// （见该函数文档），只是 Kiro 这边的上游协议不同，因此不能直接复用其实现。
//
// 读循环的形状、错误分类、断连处理都与 streamToClient 完全一致，通过共享的
// feedTranslatorChunk（见其文档）保证两条路径不会在下一次修 bug 时走偏——
// 这里只在"逐块 SSE 写出"还是"攒成一个切片，最后统一转换"这一点上不同。
func (s *KiroGatewayService) nonStreamToClient(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	resp *http.Response,
	translator *kiro.StreamTranslator,
	inbound *apicompat.AnthropicRequest,
	upstreamModel string,
	startTime time.Time,
	outputProtocol kiroOutputProtocol,
) (*ForwardResult, error) {
	var firstTokenMs *int
	var disconnect bool
	var events []apicompat.AnthropicStreamEvent

	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if firstTokenMs == nil {
				// 非流式响应没有"逐块写给客户端"这个动作可以挂
				// beforeFirstWrite 钩子（streamToClient 那样），首字节时间
				// 只能在这里、拿到上游第一块字节时直接测量——语义上与
				// streamToClient 的 firstTokenMs 一致：都是"从发起请求到
				// 收到上游第一个字节"的耗时，不是"到客户端收到响应"的耗时
				// （客户端要等全部收集完、Finalize 之后才会收到任何字节）。
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			chunkEvents, feedErr, done := s.feedTranslatorChunk(ctx, account, translator, buf[:n])
			events = append(events, chunkEvents...)
			if feedErr != nil {
				return nil, feedErr
			}
			if done {
				break
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				// 非流式路径在拿到完整响应前不会碰客户端连接，没有
				// streamToClient 那样的增量写出器，因此没有 cw.Disconnected()
				// 这个独立信号——传 false；客户端真正断连的主要检测路径
				// （errors.Is(err, context.Canceled/DeadlineExceeded)）已经
				// 在 handleStreamReadError 内部覆盖，forwardCallEndpoint
				// 用的是同一个请求级 ctx。
				if d, handled := handleStreamReadError(readErr, false, "kiro upstream (non-stream)"); handled {
					disconnect = d
					break
				}
				if !translator.SawContent() {
					return nil, readErr
				}
			}
			break
		}
	}

	events = append(events, translator.Finalize()...)

	anthropicResp, err := kiroAccumulateAnthropicResponse(events)
	if err != nil {
		return nil, fmt.Errorf("kiro: accumulate non-stream response: %w", err)
	}

	// usage 直接用 translator.Usage()，不让累积器从 message_delta 事件里
	// 重新反推一遍——两者本就是同一份数据（Finalize 的 message_delta 事件
	// 的 Usage 字段就是调用 t.Usage() 算出来的，见 stream.go），没有必要在
	// 累积器里再维护一条平行的合并逻辑；也让这里和 ForwardResult.Usage
	// （下面）天然保持一致，不会出现客户端 JSON 里的 usage 和计费用量对
	// 不上的情况。
	usage := translator.Usage()
	cache5m, cache1h := translator.PromptCacheCreationSplit()
	anthropicResp.Usage = usage

	var body []byte
	if outputProtocol == kiroOutputResponses {
		// Codex 客户端期待 Responses 形态——复用与 Antigravity
		// ForwardAsResponses 相同的纯转换函数，不新写协议逻辑。
		body, err = json.Marshal(apicompat.AnthropicToResponsesResponse(anthropicResp))
	} else {
		body, err = json.Marshal(anthropicResp)
	}
	if err != nil {
		return nil, fmt.Errorf("kiro: marshal non-stream response: %w", err)
	}

	c.Data(http.StatusOK, "application/json", body)

	return &ForwardResult{
		Model:            inbound.Model,
		UpstreamModel:    upstreamModel,
		Stream:           false,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: disconnect,
		Usage: ClaudeUsage{
			InputTokens:              usage.InputTokens,
			OutputTokens:             usage.OutputTokens,
			CacheCreationInputTokens: usage.CacheCreationInputTokens,
			CacheReadInputTokens:     usage.CacheReadInputTokens,
			CacheCreation5mTokens:    cache5m,
			CacheCreation1hTokens:    cache1h,
		},
	}, nil
}

// kiroAccumulateAnthropicResponse 把一段完整的 Anthropic SSE 事件序列折叠成
// 一个非流式的 *apicompat.AnthropicResponse。纯函数，不依赖任何网络/IO，
// 单独可测。
//
// 调用方（nonStreamToClient）传入的 events 总是以 message_start 开头——
// StreamTranslator.Finalize 保证了这一点，哪怕上游全程没有产出任何内容
// （见 stream.go 的 ensureStarted 文档）。这里仍然对 message_start 缺失的
// 情况做防御：不 panic，只是返回一个零值 AnthropicResponse。
func kiroAccumulateAnthropicResponse(events []apicompat.AnthropicStreamEvent) (*apicompat.AnthropicResponse, error) {
	resp := &apicompat.AnthropicResponse{}

	// pendingToolInput 按 content_block 的 index 累积 input_json_delta 分片，
	// 在对应 content_block_stop 时冲刷成最终的 json.RawMessage——跟
	// stream.go 里 ToolUse.Input 的分片累积语义（stop=true 之前只是分片）
	// 一一对应，只是这里是在客户端一侧、针对已经翻译好的 Anthropic 事件
	// 做同样的累积，而不是针对 Kiro 原始事件。
	pendingToolInput := make(map[int]*strings.Builder)

	for _, ev := range events {
		switch ev.Type {
		case "message_start":
			if ev.Message == nil {
				continue
			}
			// 浅拷贝足够——Message 内嵌的 Content 切片在正常构造下本就是
			// 空的（ensureStarted 里 message_start 恒为
			// Content: []apicompat.AnthropicContentBlock{}），这里再显式
			// 重置一次纯粹是防御性的：不管上游/翻译器将来是否变化，累积器
			// 自己的 Content 必须完全由后续的 content_block_* 事件重建，
			// 不能信任 message_start 里可能带的任何内容。
			//
			// 必须重置成空切片而不是 nil（复查发现的 Important 级问题）：
			// AnthropicResponse.Content 的 json tag 没有 omitempty，nil 切片
			// 序列化成 "content":null。上游一个 content 帧都没产出时（空回复/
			// 静默截断——stream.go 的 Finalize 注释明确关心的场景），后面的
			// content_block_* 事件从不触发，Content 会一直是这里设置的初始
			// 值，直接把 null 发给客户端，违反 Anthropic 线协议（严格客户端/
			// SDK 校验 content 必须是数组）。
			cp := *ev.Message
			cp.Content = []apicompat.AnthropicContentBlock{}
			resp = &cp

		case "content_block_start":
			if ev.Index == nil || ev.ContentBlock == nil {
				continue
			}
			idx := *ev.Index
			kiroGrowAnthropicContent(&resp.Content, idx)
			resp.Content[idx] = *ev.ContentBlock

		case "content_block_delta":
			if ev.Index == nil || ev.Delta == nil {
				continue
			}
			idx := *ev.Index
			kiroGrowAnthropicContent(&resp.Content, idx)
			switch ev.Delta.Type {
			case "text_delta":
				resp.Content[idx].Text += ev.Delta.Text
			case "thinking_delta":
				resp.Content[idx].Thinking += ev.Delta.Thinking
			case "signature_delta":
				resp.Content[idx].Signature += ev.Delta.Signature
			case "input_json_delta":
				b, ok := pendingToolInput[idx]
				if !ok {
					b = &strings.Builder{}
					pendingToolInput[idx] = b
				}
				_, _ = b.WriteString(ev.Delta.PartialJSON)
			}

		case "content_block_stop":
			if ev.Index == nil {
				continue
			}
			idx := *ev.Index
			b, ok := pendingToolInput[idx]
			if !ok {
				continue
			}
			delete(pendingToolInput, idx)
			if idx >= len(resp.Content) {
				continue
			}
			s := b.String()
			if s == "" || !json.Valid([]byte(s)) {
				// 与 stream.go 的 handleToolUse 一致：一个 tool_use 块从未
				// 收到过任何 input_json_delta 分片（或分片拼接结果恰好是
				// 空字符串）时，Input 落到 "{}"，而不是空字节串（无效 JSON）
				// 或 nil（json.RawMessage 的 nil 在 omitempty 下会让
				// "input" 字段整个消失，跟"有 input_json_delta 才关心分片"
				// 的既有网关行为不一致）。
				//
				// !json.Valid 分支是复查发现的 Minor 级问题的修复：上游在
				// input_json_delta 中途断流时（feedTranslatorChunk 的
				// sawContent-优雅截断路径，流式侧本就能正常截断返回），分片
				// 拼接结果是半截 JSON（如 `{"pa`），不做校验会让下面的
				// json.Marshal(anthropicResp) 直接失败——已经收到的正文/
				// usage 全部作废，nonStreamToClient 返回一个未分类的裸
				// error，而同样的上游行为在流式路径下是成功返回截断内容。
				// 校验失败时同样落到 "{}"，保持与流式路径一致的"尽力截断"
				// 语义，而不是让整个响应因为一个字段无效而全盘皆输。
				s = "{}"
			}
			resp.Content[idx].Input = json.RawMessage(s)

		case "message_delta":
			if ev.Delta == nil {
				continue
			}
			if ev.Delta.StopReason != "" {
				resp.StopReason = apicompat.AnthropicStopReasonPtr(ev.Delta.StopReason)
			}
			if ev.Delta.StopSequence != nil {
				resp.StopSequence = ev.Delta.StopSequence
			}

		case "message_stop":
			// no-op：循环结束的标记，不携带任何要合并的数据。
		}
	}

	return resp, nil
}

// kiroGrowAnthropicContent 按需把 content 切片扩容到能容纳 idx，新增的槽位
// 是 AnthropicContentBlock 的零值，随后由调用方立即整体赋值/按字段追加覆盖。
// 正常情况下 StreamTranslator 产出的 index 是从 0 开始严格递增的，这里的
// "按需扩容"只是防御性的，不依赖这个假设。
func kiroGrowAnthropicContent(content *[]apicompat.AnthropicContentBlock, idx int) {
	if idx < len(*content) {
		return
	}
	grown := make([]apicompat.AnthropicContentBlock, idx+1)
	copy(grown, *content)
	*content = grown
}
