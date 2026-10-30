package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"

// stripChatHistoryReasoning 清空 Chat Completions 历史消息里的推理内容：assistant
// 消息的 reasoning_content/reasoning 字符串，以及 content 数组里的 thinking/reasoning
// 片段。
//
// chatCompletionsToResponses 会把历史推理拼成 "<thinking>…</thinking>" 正文回传上游，
// Gemini 模型看到该格式会当成自己上一轮的输出照抄，思考因此写进可见正文；而 Gemini 的
// 思考连续性由 thoughtSignature 承载，历史推理本就不需要回传。所有走 Gemini 上游的
// Chat Completions 兼容路径都应在转换前调用本函数。
func stripChatHistoryReasoning(req *apicompat.ChatCompletionsRequest) {
	if req == nil {
		return
	}
	for i := range req.Messages {
		req.Messages[i].ReasoningContent = ""
		req.Messages[i].Reasoning = ""
		if req.Messages[i].Role == "assistant" {
			req.Messages[i].Content = dropChatReasoningContentParts(req.Messages[i].Content)
		}
	}
}
