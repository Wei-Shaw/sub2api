package service

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Responses 的 response 对象回显上游实际收到的 instructions。网关补写、追加或
// 搬运 instructions 后，回显按客户端请求里的原始取值还原；客户端未传时还原为 null。

const (
	openAIResponsesClientInstructionsKey      = "openai_responses_client_instructions"
	openAIResponsesInstructionsEchoRestoreKey = "openai_responses_instructions_echo_restore"
)

var openAIResponsesInstructionsFieldMarker = []byte(`"instructions"`)

type openAIResponsesClientInstructions struct {
	// raw 是客户端 instructions 压缩成单行后的 JSON；未传时为空串。
	// 单行保证还原值写回 SSE data 行时不会拆帧。
	raw string
}

// rememberOpenAIResponsesClientInstructions 记录本次转发尝试的客户端原始
// instructions，并清空还原值，使未登记上游请求体的转发分支不做还原。
func rememberOpenAIResponsesClientInstructions(c *gin.Context, body []byte) {
	if c == nil {
		return
	}
	raw := ""
	if value := gjson.GetBytes(body, "instructions"); value.Exists() {
		raw = compactOpenAIResponsesInstructionsRaw(value.Raw)
	}
	c.Set(openAIResponsesClientInstructionsKey, openAIResponsesClientInstructions{raw: raw})
	c.Set(openAIResponsesInstructionsEchoRestoreKey, "")
}

// syncOpenAIResponsesInstructionsEcho 比较即将发往上游的 instructions 与客户端
// 原始取值，不一致时登记回显还原值。
func syncOpenAIResponsesInstructionsEcho(c *gin.Context, upstreamBody []byte) {
	if c == nil {
		return
	}
	stored, ok := c.Get(openAIResponsesClientInstructionsKey)
	if !ok {
		return
	}
	client, ok := stored.(openAIResponsesClientInstructions)
	if !ok {
		return
	}
	upstreamRaw := ""
	if value := gjson.GetBytes(upstreamBody, "instructions"); value.Exists() {
		upstreamRaw = value.Raw
	}
	restore := ""
	if !openAIResponsesInstructionsRawEqual(client.raw, upstreamRaw) {
		restore = client.raw
		if restore == "" {
			restore = "null"
		}
	}
	c.Set(openAIResponsesInstructionsEchoRestoreKey, restore)
}

// compactOpenAIResponsesInstructionsRaw 去掉 JSON 值里的格式空白；无法解析时
// 返回 null。
func compactOpenAIResponsesInstructionsRaw(raw string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(raw)); err != nil {
		return "null"
	}
	return buf.String()
}

func openAIResponsesInstructionsRawEqual(left, right string) bool {
	if left == right {
		return true
	}
	if left == "" || right == "" {
		return false
	}
	l := gjson.Parse(left)
	r := gjson.Parse(right)
	if l.Type != r.Type {
		return false
	}
	if l.Type == gjson.String {
		return l.Str == r.Str
	}
	return compactOpenAIResponsesInstructionsRaw(l.Raw) == compactOpenAIResponsesInstructionsRaw(r.Raw)
}

func openAIResponsesInstructionsEchoRestoreValue(c *gin.Context) string {
	if c == nil {
		return ""
	}
	stored, ok := c.Get(openAIResponsesInstructionsEchoRestoreKey)
	if !ok {
		return ""
	}
	restore, _ := stored.(string)
	return restore
}

func restoreOpenAIResponsesInstructionsAtPath(data []byte, path, restore string) []byte {
	if restore == "" || !bytes.Contains(data, openAIResponsesInstructionsFieldMarker) {
		return data
	}
	if !gjson.GetBytes(data, path).Exists() {
		return data
	}
	restored, err := sjson.SetRawBytes(data, path, []byte(restore))
	if err != nil {
		return data
	}
	return restored
}

// restoreOpenAIResponsesInstructionsEchoInEvent 还原单个 SSE 事件里
// response.instructions 的回显。
func restoreOpenAIResponsesInstructionsEchoInEvent(c *gin.Context, data []byte) []byte {
	return restoreOpenAIResponsesInstructionsAtPath(data, "response.instructions", openAIResponsesInstructionsEchoRestoreValue(c))
}

// restoreOpenAIResponsesInstructionsEchoInResponse 还原非流式 response 对象里
// instructions 的回显。
func restoreOpenAIResponsesInstructionsEchoInResponse(c *gin.Context, body []byte) []byte {
	return restoreOpenAIResponsesInstructionsAtPath(body, "instructions", openAIResponsesInstructionsEchoRestoreValue(c))
}

// restoreOpenAIResponsesInstructionsEchoInSSEBody 逐个 data 行还原整段 SSE 文本里的回显。
func restoreOpenAIResponsesInstructionsEchoInSSEBody(c *gin.Context, body string) string {
	restore := openAIResponsesInstructionsEchoRestoreValue(c)
	if restore == "" || !strings.Contains(body, `"instructions"`) {
		return body
	}
	lines := strings.Split(body, "\n")
	changed := false
	for i, line := range lines {
		data, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
		}
		restored := restoreOpenAIResponsesInstructionsAtPath([]byte(data), "response.instructions", restore)
		if string(restored) == data {
			continue
		}
		lines[i] = "data: " + string(restored)
		changed = true
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}
