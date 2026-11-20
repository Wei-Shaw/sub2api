package requestmodel

import (
	"bytes"
	"strconv"
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
//   - JSON 嵌套对象（任意深度，含数组元素）：成员名转义解码后完全相同且原始
//     取值不同，字段名按路径报告（如 reasoning.effort、messages[0].role）。
//     reasoning.effort 等嵌套字段同样参与计费倍率与分组策略，gjson 读首个、
//     sjson 只改首个、重新编码保留末个的分叉与顶层一致。嵌套层不做大小写
//     折叠：工具 JSON Schema 等用户数据里 id 与 ID 是合法的不同属性。
//   - multipart：model / session 文本字段出现多次且取值不同；session 字段的
//     JSON 值内部同理（含嵌套）。
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
	if conflict != "" {
		return conflict, true
	}
	return nestedConflictingField(root, "")
}

// maxConflictDepth 与 encoding/json 的嵌套上限一致：更深的请求体下游解码必然
// 失败，不再下钻，同时避免恶意深嵌套耗尽栈。
const maxConflictDepth = 10000

// conflictLinearScanLimit 以内的对象线性扫描判重，避免为每个小对象分配 map。
const conflictLinearScanLimit = 16

// nestedConflictingField 检查 object 各成员值内部（不含 object 自身这一层）的
// 所有嵌套对象，返回第一个取值不一致的重复成员路径；prefix 为路径前缀。
func nestedConflictingField(object gjson.Result, prefix string) (string, bool) {
	w := conflictWalker{}
	if prefix != "" {
		w.push(prefix)
	}
	object.ForEach(func(key, value gjson.Result) bool {
		if !value.IsObject() && !value.IsArray() {
			return true
		}
		w.push(key.String())
		found := w.walk(value, 1)
		w.pop()
		return !found
	})
	return w.conflict, w.conflict != ""
}

type conflictMember struct {
	name string
	raw  string
}

// conflictPathSegment 是成员名或数组下标（index >= 0）；只在报告冲突时拼接成字符串。
type conflictPathSegment struct {
	name  string
	index int
}

type conflictWalker struct {
	path     []conflictPathSegment
	members  []conflictMember
	conflict string
}

func (w *conflictWalker) push(name string) {
	w.path = append(w.path, conflictPathSegment{name: name, index: -1})
}
func (w *conflictWalker) pushIndex(index int) {
	w.path = append(w.path, conflictPathSegment{index: index})
}
func (w *conflictWalker) pop() { w.path = w.path[:len(w.path)-1] }

// walk 遍历 v；发现冲突时写入 w.conflict 并返回 true。
func (w *conflictWalker) walk(v gjson.Result, depth int) bool {
	if depth >= maxConflictDepth {
		return false
	}
	if v.IsArray() {
		index := 0
		found := false
		v.ForEach(func(_, item gjson.Result) bool {
			if item.IsObject() || item.IsArray() {
				w.pushIndex(index)
				found = w.walk(item, depth+1)
				w.pop()
			}
			index++
			return !found
		})
		return found
	}
	if !v.IsObject() {
		return false
	}

	start := len(w.members)
	defer func() { w.members = w.members[:start] }()
	var index map[string]string
	found := false
	v.ForEach(func(key, item gjson.Result) bool {
		name := key.String()
		if w.repeatsWithDifferentValue(start, &index, name, item.Raw) {
			w.push(name)
			w.conflict = joinConflictPath(w.path)
			w.pop()
			found = true
			return false
		}
		// 子对象在返回前会把 w.members 截回原长度，本对象的成员保持连续。
		w.members = append(w.members, conflictMember{name: name, raw: item.Raw})
		if item.IsObject() || item.IsArray() {
			w.push(name)
			found = w.walk(item, depth+1)
			w.pop()
		}
		return !found
	})
	return found
}

// repeatsWithDifferentValue 判断 name 是否与本对象（w.members[start:]）中更早
// 的同名成员取值不同；成员较多时改用 map 索引。
func (w *conflictWalker) repeatsWithDifferentValue(start int, index *map[string]string, name, raw string) bool {
	if *index != nil {
		first, repeated := (*index)[name]
		if !repeated {
			(*index)[name] = raw
			return false
		}
		return first != raw
	}
	for _, seen := range w.members[start:] {
		if seen.name == name {
			return seen.raw != raw
		}
	}
	if len(w.members)-start >= conflictLinearScanLimit {
		*index = make(map[string]string, 2*conflictLinearScanLimit)
		for _, seen := range w.members[start:] {
			if _, ok := (*index)[seen.name]; !ok {
				(*index)[seen.name] = seen.raw
			}
		}
		(*index)[name] = raw
	}
	return false
}

// joinConflictPath 把路径段拼成 a.b[0].c；结果是新分配的字符串，不引用请求体。
func joinConflictPath(path []conflictPathSegment) string {
	var b strings.Builder
	for i, segment := range path {
		if segment.index >= 0 {
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(segment.index))
			b.WriteByte(']')
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(segment.name)
	}
	return b.String()
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
		if name, ok := nestedConflictingField(session, "session"); ok {
			return name, true
		}
	}
	return "", false
}
