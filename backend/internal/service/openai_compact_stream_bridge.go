package service

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// openAICompactClientStreamKey 标记 body-signal compact 请求（Codex remote
// compact v2，见 #3777）的原始 body 携带 stream:true。白名单归一化会删除
// stream 字段并让上游走 unary /responses/compact（JSON），但客户端仍按
// Responses SSE 协议消费响应：它必须收到 response.output_item.done（其中恰好
// 一个 type=compaction 的 item）和 response.completed，否则报
// "stream closed before response.completed" 并无限重连（#3875）。
const openAICompactClientStreamKey = "openai_compact_client_stream"

// MarkOpenAICompactClientStream 由 handler 在 body-signal 提升时调用，记录
// 客户端的原始 stream 意图，供响应写回阶段决定是否合成 SSE。
func MarkOpenAICompactClientStream(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(openAICompactClientStreamKey, true)
}

func OpenAICompactClientStreamKeyForTest() string {
	return openAICompactClientStreamKey
}

func openAICompactClientWantsStream(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, ok := c.Get(openAICompactClientStreamKey)
	if !ok {
		return false
	}
	wants, _ := value.(bool)
	return wants
}

// writeOpenAICompactSSEBridge 将 unary compact 的最终 JSON 响应按 Codex remote
// compact v2 的消费协议合成为最小 Responses SSE 流写回客户端。仅当请求被标记
// 为 body-signal 客户端流式、状态码为 2xx 且 body 是合法 JSON 对象时生效；
// 返回 false 表示未写出任何内容，调用方应按原路径写回。
func writeOpenAICompactSSEBridge(c *gin.Context, statusCode int, finalResponse []byte) bool {
	if c == nil || statusCode < 200 || statusCode >= 300 || !openAICompactClientWantsStream(c) {
		return false
	}
	payload, ok := buildOpenAICompactSSEPayload(finalResponse)
	if !ok {
		return false
	}
	header := c.Writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(statusCode)
	_, _ = c.Writer.Write(payload)
	c.Writer.Flush()
	return true
}

// buildOpenAICompactSSEPayload 把 compact 的 Response JSON 转成 SSE 事件序列：
// 每个 output[] item 一条 response.output_item.done，最后一条 response.completed
// 携带完整 response 对象。Codex 的 SSE 解析只从 output_item.done 收集 item，
// 并要求 response.completed 的 response.id 必填、usage（若存在）必须携带
// input_tokens/output_tokens/total_tokens 整数字段，否则整条 completed 事件
// 解析失败，故此处做兜底修补。
func buildOpenAICompactSSEPayload(finalResponse []byte) ([]byte, bool) {
	if len(finalResponse) == 0 || !gjson.ValidBytes(finalResponse) {
		return nil, false
	}
	if !gjson.ParseBytes(finalResponse).IsObject() {
		return nil, false
	}
	// SSE 的 data 行不允许出现裸换行：上游 JSON 可能是 pretty-printed 形态，
	// 嵌入前必须压缩为单行。
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, finalResponse); err != nil {
		return nil, false
	}
	response := compacted.Bytes()
	root := gjson.ParseBytes(response)
	if strings.TrimSpace(root.Get("id").String()) == "" {
		next, err := sjson.SetBytes(response, "id", "resp_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
		if err != nil {
			return nil, false
		}
		response = next
	}
	if usage := gjson.GetBytes(response, "usage"); usage.Exists() && !openAICompactUsageParsableByCodex(usage) {
		next, err := sjson.DeleteBytes(response, "usage")
		if err != nil {
			return nil, false
		}
		response = next
	}

	var buf bytes.Buffer
	outputIndex := 0
	appendEvent := func(eventType string, data []byte) {
		_, _ = buf.WriteString("event: ")
		_, _ = buf.WriteString(eventType)
		_, _ = buf.WriteString("\ndata: ")
		_, _ = buf.Write(data)
		_, _ = buf.WriteString("\n\n")
	}
	for _, item := range gjson.GetBytes(response, "output").Array() {
		if !item.IsObject() {
			continue
		}
		event, err := sjson.SetBytes([]byte(`{"type":"response.output_item.done"}`), "output_index", outputIndex)
		if err != nil {
			return nil, false
		}
		event, err = sjson.SetRawBytes(event, "item", []byte(item.Raw))
		if err != nil {
			return nil, false
		}
		appendEvent("response.output_item.done", event)
		outputIndex++
	}

	completed, err := sjson.SetRawBytes([]byte(`{"type":"response.completed"}`), "response", response)
	if err != nil {
		return nil, false
	}
	appendEvent("response.completed", completed)
	return buf.Bytes(), true
}

func openAICompactUsageParsableByCodex(usage gjson.Result) bool {
	if !usage.IsObject() {
		return false
	}
	for _, field := range []string{"input_tokens", "output_tokens", "total_tokens"} {
		if usage.Get(field).Type != gjson.Number {
			return false
		}
	}
	return true
}
