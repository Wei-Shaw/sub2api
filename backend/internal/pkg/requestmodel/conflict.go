package requestmodel

import (
	"bytes"
	"strings"
	"unicode"
	"unsafe"

	"github.com/tidwall/gjson"
)

// utf8BOM 是部分客户端写在 JSON 文档前的 UTF-8 字节序标记。宽容读取会跳过它
// 再解析对象，歧义检查也要看同一个对象。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ConflictingField 返回请求体里第一个「重复声明且取值不一致」的字段名。
//
// 网关读取 model、service_tier 等字段用的是 gjson（重复键取第一个、大小写
// 敏感），而改写并转发请求体的路径会经过 encoding/json（重复键取最后一个，
// 结构体绑定不区分大小写）。同名字段取值不一致时，准入、计费看到的值和上游
// 实际收到的值会分叉：{"model":"便宜模型",...,"model":"贵模型"} 按便宜模型
// 计费，转发的却是贵模型。调用方应在任何解析器读取字段之前拒绝这类请求。
//
// 判定范围：
//   - JSON：顶层成员名在转义解码、大小写折叠后相同，且原始取值不同；顶层
//     session 对象（Live 入口的模型所在位置）内部同理，字段名报告为
//     session.<name>。
//   - multipart：model / session 文本字段出现多次且取值不同；session 字段的
//     JSON 值内部同理。
//
// 取值完全相同的重复字段不算歧义：任何解析器绑定到的值都一样，这与白名单
// 准入对同值重复字段的放行保持一致。不是 JSON 对象也不是 multipart 的请求体
// 返回 false，其合法性由各入口自己的解析器判断。
func ConflictingField(contentType string, body []byte) (string, bool) {
	if isMultipartContentType(contentType) {
		return multipartConflictingField(contentType, body)
	}
	return jsonConflictingField(body)
}

// HasConflictingModels 判断模型候选集（见 FromBodyCandidates）里是否存在不同的
// 取值。候选集覆盖重复键与大小写变体，取值不一致说明不同解析器会绑定到不同
// 模型。
func HasConflictingModels(candidates []string) bool {
	for i := 1; i < len(candidates); i++ {
		if candidates[i] != candidates[0] {
			return true
		}
	}
	return false
}

// jsonConflictingField 检查 JSON 对象的顶层成员，以及顶层 session 对象的成员。
func jsonConflictingField(body []byte) (string, bool) {
	body = bytes.TrimPrefix(body, utf8BOM)
	if len(body) == 0 {
		return "", false
	}
	// 零拷贝视图只在本次检查内使用；返回的字段名会单独复制。
	root := gjson.Parse(*(*string)(unsafe.Pointer(&body)))
	if !root.IsObject() {
		return "", false
	}
	if name, ok := conflictingMember(root); ok {
		return name, true
	}
	conflict := ""
	root.ForEach(func(key, value gjson.Result) bool {
		if !strings.EqualFold(key.String(), "session") || !value.IsObject() {
			return true
		}
		if name, ok := conflictingMember(value); ok {
			conflict = "session." + name
			return false
		}
		return true
	})
	return conflict, conflict != ""
}

// conflictingMember 返回对象里第一个与更早的同名成员取值不同的成员名。
// 成员名按 encoding/json 结构体绑定的规则比较：先解码转义，再做 Unicode
// 简单大小写折叠，所以 model、MODEL 和 "\u006dodel" 是同一个名字。取值按原始
// JSON 文本比较，写法不同的等价值（如 1 和 1.0）按不一致处理。
func conflictingMember(object gjson.Result) (string, bool) {
	seen := make(map[string]string)
	conflict := ""
	object.ForEach(func(key, value gjson.Result) bool {
		folded := strings.Map(foldRune, key.String())
		first, repeated := seen[folded]
		if !repeated {
			seen[folded] = value.Raw
			return true
		}
		if first != value.Raw {
			conflict = strings.Clone(key.String())
			return false
		}
		return true
	})
	return conflict, conflict != ""
}

// foldRune 返回 r 所在大小写折叠轨道上的固定代表，只有大小写不同的字符映射到
// 同一个值。拉丁字母取小写，常见的小写字段名无需改写。
func foldRune(r rune) rune {
	folded := r
	for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
		if next < folded {
			folded = next
		}
	}
	if folded >= 'A' && folded <= 'Z' {
		folded += 'a' - 'A'
	}
	return folded
}

// multipartConflictingField 检查 multipart 表单：model / session 文本字段多次
// 出现且取值不同，或某个 session 字段的 JSON 值内部存在取值不一致的重复成员。
func multipartConflictingField(contentType string, body []byte) (string, bool) {
	models, sessions := multipartModelCandidates(contentType, body)
	if HasConflictingModels(models) {
		return "model", true
	}
	if HasConflictingModels(sessions) {
		return "session", true
	}
	for _, raw := range sessions {
		session := gjson.Parse(raw)
		if !session.IsObject() {
			continue
		}
		if name, ok := conflictingMember(session); ok {
			return "session." + name, true
		}
	}
	return "", false
}
