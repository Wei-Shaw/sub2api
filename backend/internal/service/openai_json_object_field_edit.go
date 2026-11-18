package service

import (
	"encoding/json"

	"github.com/tidwall/gjson"
)

// jsonObjectStringFieldEdit 把 object（请求体 JSON 数组里的一个对象元素）的字符串字段设为 value。
type jsonObjectStringFieldEdit struct {
	object gjson.Result
	value  string
}

// setStringFieldInJSONObjects 只复制一次 body，完成多处"给数组里的对象设置字符串字段"，
// 结果与按顺序逐个调用 sjson.SetBytes(body, "<数组路径>.<i>.<field>", value) 逐字节相同。
//
// 逐个 sjson.SetBytes 每次都复制整个 body 并从头定位路径，k 处修改、body 长 n 时是
// O(k·n)：几百轮的 agent 对话有几百条 assistant 消息、请求体几百 KB，一个请求就要
// 复制上百 MB。
//
// edits 必须按 object 在 body 中的位置升序排列；object 必须直接取自对 body 的 gjson
// 解析，其 Index 才是在 body 中的绝对偏移。field 必须是不含 gjson 路径特殊字符的普通键名。
// 任何一处无法可靠定位时返回 ok=false，调用方应回退到逐个 sjson.SetBytes。
func setStringFieldInJSONObjects(body []byte, field string, edits []jsonObjectStringFieldEdit) ([]byte, bool) {
	if len(edits) == 0 {
		return body, true
	}
	out := make([]byte, 0, len(body)+len(edits)*(len(field)+8))
	last := 0
	for _, edit := range edits {
		obj := edit.object
		start, end := obj.Index, obj.Index+len(obj.Raw)
		if start <= 0 || start < last || end > len(body) || len(obj.Raw) < 2 ||
			obj.Raw[0] != '{' || obj.Raw[len(obj.Raw)-1] != '}' || string(body[start:end]) != obj.Raw {
			return nil, false
		}
		if existing := obj.Get(field); existing.Exists() {
			// 与 sjson 相同：替换第一个同名键的值。Result.Get 返回的 Index 已加上
			// obj.Index，是在 body 中的绝对偏移。
			valueStart := existing.Index
			valueEnd := valueStart + len(existing.Raw)
			if valueStart <= start || valueEnd > end || string(body[valueStart:valueEnd]) != existing.Raw {
				return nil, false
			}
			out = append(out, body[last:valueStart]...)
			out = appendSJSONString(out, edit.value)
			last = valueEnd
			continue
		}
		// 与 sjson 相同：在最后一个 '}' 之前追加 `,"field":value`（空对象不加逗号）。
		out = append(out, body[last:end-1]...)
		if jsonObjectHasMembers(obj.Raw) {
			out = append(out, ',')
		}
		out = appendSJSONString(out, field)
		out = append(out, ':')
		out = appendSJSONString(out, edit.value)
		out = append(out, '}')
		last = end
	}
	return append(out, body[last:]...), true
}

// jsonObjectHasMembers 与 sjson 判断是否需要逗号的方式相同：'{' 之后第一个非空白字符不是 '}'。
func jsonObjectHasMembers(raw string) bool {
	for i := 1; i < len(raw); i++ {
		if raw[i] <= ' ' {
			continue
		}
		return raw[i] != '}'
	}
	return false
}

// appendSJSONString 与 sjson 写字符串值的编码完全相同：含控制字符、非 ASCII、
// 引号或反斜杠时用 encoding/json 编码，否则直接加引号。
func appendSJSONString(buf []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		if s[i] < ' ' || s[i] > 0x7f || s[i] == '"' || s[i] == '\\' {
			encoded, _ := json.Marshal(s)
			return append(buf, encoded...)
		}
	}
	buf = append(buf, '"')
	buf = append(buf, s...)
	return append(buf, '"')
}
