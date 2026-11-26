package service

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"unicode/utf8"
	"unsafe"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type toolNameStreamRestorer struct {
	rw         *ToolNameRewrite
	maxFakeLen int
	carry      map[int]toolNameStreamCarry
	// pending 是本请求可能原样出现在流里的全部假名（动态 ReverseOrdered 的假名侧加静态
	// 映射的假名侧），构造时一次转成只读视图；heldSuffixLen 逐 delta 只遍历它。
	pending [][]byte
	// pendingFirst 标记 pending 中出现过的首字节，用来跳过不可能匹配的起点。
	pendingFirst [256]bool
}

// 承载文本的 delta 类型 → 载荷字段名。
var toolNameStreamDeltaFields = map[string]string{
	"text_delta":       "text",
	"input_json_delta": "partial_json",
}

// 事件 type 值的廉价预筛：不含这两个子串的事件不进 gjson。
var (
	sseContentBlockDeltaBytes = []byte("content_block_delta")
	sseContentBlockStopBytes  = []byte("content_block_stop")
)

type toolNameStreamCarry struct {
	deltaType, field, text string
}

func newToolNameStreamRestorer(rw *ToolNameRewrite) *toolNameStreamRestorer {
	if rw == nil || len(rw.ReverseOrdered) == 0 {
		return nil
	}
	// 静态假前缀不经映射也会被还原，同样要能留尾巴。
	r := &toolNameStreamRestorer{rw: rw, carry: map[int]toolNameStreamCarry{}, pending: make([][]byte, 0, len(rw.ReverseOrdered)+len(staticToolNameRewrites))}
	for _, pair := range rw.ReverseOrdered {
		r.maxFakeLen = max(r.maxFakeLen, r.addPending(pair[0]))
	}
	for _, fake := range staticToolNameRewrites {
		r.maxFakeLen = max(r.maxFakeLen, r.addPending(fake))
	}
	return r
}

// addPending 把 fake 存进 pending 并登记首字节，返回其字节长度。
// 假名在请求期内不可变（静态表是进程级常量），存只读视图即可，不复制。
func (r *toolNameStreamRestorer) addPending(fake string) int {
	b := unsafe.Slice(unsafe.StringData(fake), len(fake))
	r.pending = append(r.pending, b)
	if len(b) > 0 {
		r.pendingFirst[b[0]] = true
	}
	return len(b)
}

// restoreInto 处理一个流事件 JSON，把要发出的事件写进 out 并返回条数：0 表示整段
// 扣住；1 表示 out[0] 是原事件或它的改写；2 仅出现在 content_block_stop 上，
// out[0] 是 stop 前补发的 carry 事件、out[1] 是 stop 本身。
// 不改动事件时 out[0] 就是 data 本身，不复制也不分配；改写的分支才新建字节。
// 调用方在 r 为 nil 时跳过本方法，走与无还原器时相同的代码。
func (r *toolNameStreamRestorer) restoreInto(data []byte, out *[2][]byte) int {
	if !bytes.Contains(data, sseContentBlockDeltaBytes) && !bytes.Contains(data, sseContentBlockStopBytes) {
		out[0] = data
		return 1
	}
	view := openAIWSPayloadStringView(data)
	eventType := gjson.Get(view, "type").String()
	idxRes := gjson.Get(view, "index")
	if !idxRes.Exists() || (eventType != "content_block_delta" && eventType != "content_block_stop") {
		out[0] = data
		return 1
	}
	idx := int(idxRes.Int())
	carry := r.carry[idx]
	if eventType == "content_block_stop" {
		if carry.text == "" {
			out[0] = data
			return 1
		}
		delete(r.carry, idx)
		out[0] = r.carryEvent(idx, carry)
		out[1] = data
		return 2
	}
	delta := gjson.Get(view, "delta")
	deltaType := delta.Get("type").String()
	field, known := toolNameStreamDeltaFields[deltaType]
	var payload gjson.Result
	if known {
		payload = delta.Get(field)
	}
	if !known || payload.Type != gjson.String {
		// 不是承载文本的 delta（citations_delta、未知类型、载荷不是字符串）：原样放行，
		// 不动该 index 的 carry。若在这里补发，被切名字的后半段就再也拼不回来；carry
		// 留到下一个同 index 文本 delta 或 content_block_stop / 结束路径再出。
		out[0] = data
		return 1
	}
	payloadText := payload.String()
	var raw []byte
	if carry.text == "" {
		raw = unsafe.Slice(unsafe.StringData(payloadText), len(payloadText))
	} else {
		raw = make([]byte, len(carry.text)+len(payloadText))
		copy(raw, carry.text)
		copy(raw[len(carry.text):], payloadText)
	}
	keep := r.heldSuffixLen(raw)
	if carry.text != "" || keep > 0 {
		delete(r.carry, idx)
	}
	if keep > 0 {
		r.carry[idx] = toolNameStreamCarry{deltaType: deltaType, field: field, text: string(raw[len(raw)-keep:])}
	}
	if keep == len(raw) {
		return 0 // 整个载荷都是假名开头，等后续事件或结束路径补发
	}
	emit := restoreToolNamesInBytes(raw[:len(raw)-keep], r.rw)
	if carry.text == "" && bytes.Equal(emit, raw) {
		out[0] = data
		return 1
	}
	out[0] = setStreamDeltaField(data, payload, emit)
	return 1
}

// setStreamDeltaField 用 emit 替换事件里 delta 载荷的值，一次分配。
// gjson 的嵌套 Result.Get 会把父结果的 Index 加到子结果上，payload.Index 因此相对原文
// data，payload.Raw 含两侧引号。
func setStreamDeltaField(data []byte, payload gjson.Result, emit []byte) []byte {
	next := make([]byte, 0, len(data)+len(emit)+2)
	next = append(next, data[:payload.Index]...)
	next = appendJSONText(next, string(emit))
	return append(next, data[payload.Index+len(payload.Raw):]...)
}

// streamEventBytes 返回 s 的只读视图，不复制。事件 JSON 只被读取（restoreInto、
// gjson、json.Unmarshal），调用方不得写入；拼接时 append 因 cap 用尽会自己复制。
func streamEventBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// jsonHexDigits 是 appendJSONText 写 \u00XX 用的十六进制字符表。
const jsonHexDigits = "0123456789abcdef"

// appendJSONText 把 s 作为 JSON 字符串追加到 dst，只转义 JSON 要求的字节：
// 双引号、反斜杠与 U+0000–U+001F 的控制字符。其余字节原样输出，< > & 与非 ASCII
// （按 UTF-8 原样）因此不会被改写成 \u003c / \u0026 / \u00e9，解出的值与输入
// 逐字节相等。无需转义时只补两侧引号，不再额外分配。
func appendJSONText(dst []byte, s string) []byte {
	if !needsJSONEscape(s) {
		dst = append(dst, '"')
		dst = append(dst, s...)
		return append(dst, '"')
	}
	dst = append(dst, '"')
	from := 0
	for i := 0; i < len(s); i++ {
		var short byte
		switch c := s[i]; {
		case c == '"' || c == '\\':
			short = c
		case c == '\n':
			short = 'n'
		case c == '\r':
			short = 'r'
		case c == '\t':
			short = 't'
		case c >= ' ':
			continue
		default: // 其余控制字节写成 \u00XX
			dst = append(dst, s[from:i]...)
			dst = append(dst, '\\', 'u', '0', '0', jsonHexDigits[c>>4], jsonHexDigits[c&0x0f])
			from = i + 1
			continue
		}
		dst = append(dst, s[from:i]...)
		dst = append(dst, '\\', short)
		from = i + 1
	}
	dst = append(dst, s[from:]...)
	return append(dst, '"')
}

// needsJSONEscape 报告 s 中是否有 appendJSONText 必须转义的字节。
func needsJSONEscape(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < ' ' || c == '"' || c == '\\' {
			return true
		}
	}
	return false
}

// flush 按 index 升序补发全部残留 carry，返回合成的 content_block_delta 事件。
// 调用方负责 r 非 nil。
func (r *toolNameStreamRestorer) flush() [][]byte {
	var out [][]byte
	for _, i := range slices.Sorted(maps.Keys(r.carry)) {
		carry := r.carry[i]
		out = append(out, r.carryEvent(i, toolNameStreamCarry{carry.deltaType, carry.field, string(restoreToolNamesInBytes([]byte(carry.text), r.rw))}))
	}
	clear(r.carry)
	return out
}

// heldSuffixLen 返回 s 末尾要扣住的最长 l（1 ≤ l ≤ maxFakeLen-1）：后 l 字节是某个假名
// 的真前缀、该假名比 l 长，且 s[len-l] 是 rune 起点。候选与首字节表在构造时备好，
// 这里不对 map 做 range，不做 string↔[]byte 转换，不分配。
func (r *toolNameStreamRestorer) heldSuffixLen(s []byte) int {
	for l := min(r.maxFakeLen-1, len(s)); l >= 1; l-- {
		start := len(s) - l
		if !r.pendingFirst[s[start]] || !utf8.RuneStart(s[start]) {
			continue
		}
		for _, fake := range r.pending {
			if len(fake) > l && bytes.HasPrefix(fake, s[start:]) {
				return l
			}
		}
	}
	return 0
}

func (r *toolNameStreamRestorer) carryEvent(idx int, carry toolNameStreamCarry) []byte {
	b := make([]byte, 0, 96+len(carry.text))
	b = append(b, `{"type":"content_block_delta","index":`...)
	b = strconv.AppendInt(b, int64(idx), 10)
	b = append(b, `,"delta":{"type":"`...)
	b = append(b, carry.deltaType...)
	b = append(b, `","`...)
	b = append(b, carry.field...)
	b = append(b, `":`...)
	b = appendJSONText(b, carry.text)
	return append(b, '}', '}')
}

func flushToolNameCarry(c *gin.Context, wrote bool, r *toolNameStreamRestorer, handle func(*apicompat.AnthropicStreamEvent)) {
	if !wrote || r == nil {
		return
	}
	for _, data := range r.flush() {
		var event apicompat.AnthropicStreamEvent
		if err := json.Unmarshal(data, &event); err != nil {
			continue
		}
		handle(&event)
	}
}
