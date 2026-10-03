package service

import (
	"bufio"
	"errors"
	"sync/atomic"
	"time"
)

const (
	anthropicFirstVisibleOutputMaxBytes = 8 * 1024 * 1024
	anthropicTerminalUsageGrace         = 500 * time.Millisecond
)

var errAnthropicFirstVisibleOutputLimit = errors.New("anthropic first-visible-output buffer limit exceeded")

// 未结束的 SSE 行同样受限，避免 Scanner 在首可见输出前增长到 MaxLineSize。
// 只有释放了本次尝试的缓冲，才解除此限制。
func anthropicFirstVisibleOutputScanLines(guard *atomic.Bool) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		advance, token, err = bufio.ScanLines(data, atEOF)
		if err != nil || !guard.Load() {
			return advance, token, err
		}
		if len(token) > anthropicFirstVisibleOutputMaxBytes || (token == nil && len(data) > anthropicFirstVisibleOutputMaxBytes) {
			return 0, nil, errAnthropicFirstVisibleOutputLimit
		}
		return advance, token, nil
	}
}

// anthropicVisibleOutputTracker 区分客户端可用输出与思考、服务端工具。
// input delta 自身不能说明工具归属；必须由相同 index 的 start 声明 tool_use。
type anthropicVisibleOutputTracker struct {
	blocks  map[int]anthropicVisibleOutputBlock
	visible bool
}

type anthropicVisibleOutputBlock struct {
	kind       string
	clientTool bool
}

func (t *anthropicVisibleOutputTracker) observe(event map[string]any) {
	eventType, _ := event["type"].(string)
	index, hasIndex := sseEventIndex(event)
	switch eventType {
	case "content_block_start":
		if !hasIndex {
			return
		}
		block, _ := event["content_block"].(map[string]any)
		kind, _ := block["type"].(string)
		if t.blocks == nil {
			t.blocks = make(map[int]anthropicVisibleOutputBlock)
		}
		id, _ := block["id"].(string)
		name, _ := block["name"].(string)
		clientTool := kind == "tool_use" && id != "" && name != ""
		t.blocks[index] = anthropicVisibleOutputBlock{kind: kind, clientTool: clientTool}
		if clientTool {
			if input, ok := block["input"].(map[string]any); ok && len(input) > 0 {
				t.visible = true
			}
		}
	case "content_block_delta":
		delta, _ := event["delta"].(map[string]any)
		kind, _ := delta["type"].(string)
		block := t.blocks[index]
		switch kind {
		case "text_delta":
			text, _ := delta["text"].(string)
			// 兼容省略文本 block_start 的上游；已知服务端工具/思考 index
			// 即使带 text_delta 也不能冒充客户端正文。
			if text != "" && (block.kind == "" || block.kind == "text") {
				t.visible = true
			}
		case "input_json_delta":
			partial, _ := delta["partial_json"].(string)
			if hasIndex && block.clientTool && partial != "" {
				t.visible = true
			}
		}
	case "content_block_stop":
		if hasIndex {
			// 完整结束的零参数客户端工具也是可用输出；start 上的空对象
			// 本身只是占位，不能提前解除保护。
			if t.blocks[index].clientTool {
				t.visible = true
			}
			delete(t.blocks, index)
		}
	}
}
