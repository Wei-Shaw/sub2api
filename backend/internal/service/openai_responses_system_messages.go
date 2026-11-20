package service

// promoteResponsesSystemMessages 把 Responses input 中的 role:"system" 文本
// 镜像进 instructions，并按 omitPromoted 决定是否从 input 删除。公开的
// Responses API 允许 input 携带 system，但 Codex 兼容网关（第三方 Codex
// 上游）只接受 instructions 承载系统提示，会返回
// "System messages are not allowed"。纯文本内容提升后无损删除；混合内容
// （如图片）保留为 developer，避免丢失非文本部分。
func promoteResponsesSystemMessages(body []byte, omitPromoted bool) ([]byte, error) {
	var reqBody map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &reqBody); err != nil {
		return nil, err
	}
	if !extractSystemMessagesFromInput(reqBody, omitPromoted) {
		return body, nil
	}
	return marshalOpenAIUpstreamJSON(reqBody)
}
