package requestmodel

import (
	"bytes"
	"hash/maphash"
	"io"
	"mime"
	"mime/multipart"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// utf8BOM 是部分客户端写在 JSON 文档前的 UTF-8 字节序标记。网关 handler 的宽容
// 读取（httputil.ReadLenientJSONRequestBodyWithPrealloc）会先剥掉它再解析，
// 准入与歧义检查必须看同一份字节。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// TrimUTF8BOM 去掉请求体开头的 UTF-8 BOM（与宽容读取的规则一致），否则原样返回。
func TrimUTF8BOM(body []byte) []byte {
	return bytes.TrimPrefix(body, utf8BOM)
}

// LooksLikeJSON 判断（已去 BOM 的）请求体首个非空白字节是否为 JSON 对象或数组。
//
// 是否按 JSON 检查只看内容、不看 Content-Type：JSON handler 从不校验请求头，
// 把 JSON 请求体标成 multipart/form-data 不能绕过检查。
func LooksLikeJSON(body []byte) bool {
	for _, c := range body {
		// 与 gjson 一致：首个值之前所有 <= 0x20 的字节都当空白跳过。
		if c <= ' ' {
			continue
		}
		return c == '{' || c == '['
	}
	return false
}

// ConflictingField 返回请求体中第一个重复声明的字段路径。
//
// 网关读取 model、service_tier、reasoning.effort 等字段用的是 gjson（重复键取
// 第一个、大小写敏感），sjson 改写（模型映射、推理强度降级、fast policy 删除
// service_tier）也只改第一个；而改写并转发请求体的路径会经过 encoding/json
// （解码到 map 时重复键取最后一个，绑定到结构体时键名大小写不敏感、取最后一个），
// 上游解析器也可能取最后一个。同名字段一旦重复，准入、计费、策略看到的值和上游
// 实际收到的值就可能分叉，例如 {"model":"便宜模型",...,"model":"贵模型"} 按便宜
// 模型计费、转发贵模型。取值相同的重复同样拒绝：网关自己的 sjson 改写只动第一个，
// 改写后两者就不再相同。调用方应在任何解析器读取字段之前拒绝这类请求。
//
// 判定范围：
//   - JSON（按内容识别，见 LooksLikeJSON）：任意深度的对象（含数组元素）内，
//     成员名经转义解码（无效 UTF-8 与孤立代理项按 encoding/json 替换为 U+FFFD）
//     后按 encoding/json 结构体绑定的大小写折叠规则相同。用户自定义键空间
//     （JSON Schema 的 properties/$defs、工具调用参数、模板变量等，见
//     isFreeformMember）内只比较精确名字，因为 id 与 ID 在那里是合法的不同键。
//     字段路径形如 reasoning.effort、messages[1].role。
//   - multipart（按 Content-Type 声明）：见 multipartConflictingField。
//   - 声明为 multipart、请求体又以 JSON 开头且解析出了表单 part：两种解读并存，
//     报告 Content-Type。
//
// 扫描是一次线性遍历，键名以带随机种子的 64 位哈希判重，CPU 与内存都与请求体
// 大小成线性关系；嵌套超过 maxConflictDepth 的部分只计括号不检查（encoding/json
// 本身拒绝这种深度），其后的同层成员仍继续检查。扫描在首个顶层值结束时停止，
// 与 gjson 读取的范围一致（其后的内容 encoding/json 会拒绝）。
func ConflictingField(contentType string, body []byte) (string, bool) {
	// 两种解读都要检查：JSON handler 不看请求头，按 JSON 读首个值；multipart
	// handler 按 boundary 解析，并跳过首个 boundary 之前的前导内容（RFC 2046
	// preamble），前导内容可以是任意 JSON。
	looksJSON := LooksLikeJSON(TrimUTF8BOM(body))
	if looksJSON {
		if field, conflict := scanDuplicateMember(TrimUTF8BOM(body)); conflict {
			return field, true
		}
	}
	if !isMultipartContentType(contentType) {
		return "", false
	}
	field, conflict, parts := multipartConflictingField(contentType, body)
	if conflict {
		return field, true
	}
	if looksJSON && parts > 0 {
		// 同一请求体既是 JSON 又含 multipart 字段：合成路由、白名单、JSON 与
		// multipart handler 会各取一种解读（例如前导 JSON 的 model 与表单的
		// model），整体视为歧义。
		return "Content-Type", true
	}
	return "", false
}

// maxConflictDepth 与 encoding/json 的嵌套上限一致。
const maxConflictDepth = 10000

// conflictLinearScanLimit 以内的对象线性比较哈希，超过后改用 map。
const conflictLinearScanLimit = 32

// conflictHashSeed 每个进程随机生成：客户端无法构造哈希碰撞。碰撞只会让请求被
// 误拒（fail closed），不会放过重复键。
var conflictHashSeed = maphash.MakeSeed()

type scanFrame struct {
	object  bool
	fold    bool // 是否按大小写折叠比较本对象的成员名
	wantKey bool // object：下一个字符串是成员名
	// 当前成员名在请求体中的原始范围（不含引号），用于路径与自由键判定。
	keyStart, keyEnd int
	keyEscaped       bool
	index            int // array：当前元素下标
	hashStart        int // object：本对象成员哈希在 scanner.hashes 中的起点
	seen             map[uint64]struct{}
}

type duplicateScanner struct {
	body   []byte
	stack  []scanFrame
	hashes []uint64
	hash   maphash.Hash
	runes  []byte
	names  []byte
}

func scanDuplicateMember(body []byte) (string, bool) {
	s := duplicateScanner{body: body}
	s.hash.SetSeed(conflictHashSeed)
	overflow := 0 // 超过 maxConflictDepth 后未闭合的括号数
	for i := 0; i < len(body); {
		switch c := body[i]; c {
		case '{', '[':
			if overflow > 0 || len(s.stack) >= maxConflictDepth {
				overflow++
			} else {
				s.push(c == '{')
			}
			i++
		case '}', ']':
			if overflow > 0 {
				overflow--
			} else if n := len(s.stack); n > 0 {
				if top := s.stack[n-1]; top.object {
					s.hashes = s.hashes[:top.hashStart]
				}
				s.stack = s.stack[:n-1]
				if n == 1 {
					return "", false
				}
			}
			i++
		case ',':
			if n := len(s.stack); overflow == 0 && n > 0 {
				if top := &s.stack[n-1]; top.object {
					top.wantKey = true
				} else {
					top.index++
				}
			}
			i++
		case '"':
			end, escaped := scanJSONString(body, i+1)
			if n := len(s.stack); overflow == 0 && n > 0 && s.stack[n-1].object && s.stack[n-1].wantKey {
				top := &s.stack[n-1]
				top.wantKey = false
				top.keyStart, top.keyEnd, top.keyEscaped = i+1, end, escaped
				if s.repeated(top) {
					return s.path(), true
				}
			}
			i = end + 1
		default:
			i++
		}
	}
	return "", false
}

func (s *duplicateScanner) push(object bool) {
	fold := true
	if n := len(s.stack); n > 0 {
		parent := &s.stack[n-1]
		fold = parent.fold
		if fold && parent.object {
			s.names = appendMemberName(s.names[:0], s.body[parent.keyStart:parent.keyEnd], parent.keyEscaped, false)
			fold = !isFreeformMember(s.names, n > 1)
		}
	}
	s.stack = append(s.stack, scanFrame{object: object, fold: fold, wantKey: object, hashStart: len(s.hashes)})
}

// repeated 登记 top 的当前成员名，返回它是否与本对象中更早的成员重名。
func (s *duplicateScanner) repeated(top *scanFrame) bool {
	s.hash.Reset()
	s.runes = appendMemberName(s.runes[:0], s.body[top.keyStart:top.keyEnd], top.keyEscaped, top.fold)
	_, _ = s.hash.Write(s.runes)
	sum := s.hash.Sum64()

	if top.seen != nil {
		if _, dup := top.seen[sum]; dup {
			return true
		}
		top.seen[sum] = struct{}{}
		return false
	}
	for _, seen := range s.hashes[top.hashStart:] {
		if seen == sum {
			return true
		}
	}
	s.hashes = append(s.hashes, sum)
	if len(s.hashes)-top.hashStart > conflictLinearScanLimit {
		// 子对象的哈希总在其闭合时截掉，本对象的哈希在切片末尾连续。
		top.seen = make(map[uint64]struct{}, 4*conflictLinearScanLimit)
		for _, seen := range s.hashes[top.hashStart:] {
			top.seen[seen] = struct{}{}
		}
		s.hashes = s.hashes[:top.hashStart]
	}
	return false
}

// path 把当前栈拼成 a.b[0].c；结果是新分配的字符串，不引用请求体。
func (s *duplicateScanner) path() string {
	var b strings.Builder
	for _, frame := range s.stack {
		if !frame.object {
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(frame.index))
			b.WriteByte(']')
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.Write(appendMemberName(nil, s.body[frame.keyStart:frame.keyEnd], frame.keyEscaped, false))
	}
	return b.String()
}

// freeformMembers 的值是用户自定义键空间：JSON Schema 的属性表与取值、工具调用
// 参数、提示词模板变量、MCP 请求头、标签。这些子树内的成员名只做精确比较（成员名
// 解码后不区分大小写地匹配本表）。它们都不在网关计费/策略读取、也不被结构体绑定
// 的字段路径上；input / response 只在非顶层时算（顶层 input 是 Responses 的输入
// 列表，顶层 response 是 realtime response.create 的参数）。metadata 不在表内：
// Antigravity 把 metadata.user_id 绑定到结构体（大小写不敏感），而网关的粘性会话
// 用 gjson 读它。
var freeformMembers = []string{
	"properties", "patternProperties", "$defs", "definitions", "dependentSchemas",
	"dependentRequired", "dependencies", "default", "const", "examples", "enum",
	"args", "arguments", "variables", "headers", "labels",
}

func isFreeformMember(name []byte, nested bool) bool {
	for _, candidate := range freeformMembers {
		if bytes.EqualFold(name, []byte(candidate)) {
			return true
		}
	}
	return nested && (bytes.EqualFold(name, []byte("input")) || bytes.EqualFold(name, []byte("response")))
}

// scanJSONString 从字符串内容起点 i 找到闭合引号的下标（找不到返回 len(b)），
// 并报告内容中是否含转义。每个字节最多访问常数次：先找下一个引号，再看它前面
// 连续反斜杠的个数，奇数说明引号被转义，从引号之后继续找。
func scanJSONString(b []byte, i int) (int, bool) {
	escaped := false
	for i < len(b) {
		quote := bytes.IndexByte(b[i:], '"')
		if quote < 0 {
			return len(b), escaped || bytes.IndexByte(b[i:], '\\') >= 0
		}
		end := i + quote
		if !escaped && bytes.IndexByte(b[i:end], '\\') >= 0 {
			escaped = true
		}
		backslashes := 0
		for j := end - 1; j >= i && b[j] == '\\'; j-- {
			backslashes++
		}
		if backslashes%2 == 0 {
			return end, escaped
		}
		i = end + 1
	}
	return len(b), escaped
}

// appendMemberName 把原始成员名按 encoding/json 的规则解码（转义、无效 UTF-8 与
// 孤立代理项替换为 U+FFFD）后追加到 dst；fold 时逐字符做大小写折叠。
func appendMemberName(dst, raw []byte, escaped, fold bool) []byte {
	for i := 0; i < len(raw); {
		var r rune
		if raw[i] == '\\' && escaped && i+1 < len(raw) {
			r, i = decodeJSONEscape(raw, i)
		} else if raw[i] < utf8.RuneSelf {
			r = rune(raw[i])
			i++
		} else {
			var size int
			r, size = utf8.DecodeRune(raw[i:])
			i += size
		}
		if fold {
			r = foldRune(r)
		}
		dst = utf8.AppendRune(dst, r)
	}
	return dst
}

// decodeJSONEscape 解码 raw[i] 处的转义序列，返回字符与下一个下标。
func decodeJSONEscape(raw []byte, i int) (rune, int) {
	switch raw[i+1] {
	case 'b':
		return '\b', i + 2
	case 'f':
		return '\f', i + 2
	case 'n':
		return '\n', i + 2
	case 'r':
		return '\r', i + 2
	case 't':
		return '\t', i + 2
	case 'u':
		r, ok := hex4(raw, i+2)
		if !ok {
			return rune(raw[i+1]), i + 2
		}
		if utf16.IsSurrogate(r) {
			if i+11 < len(raw) && raw[i+6] == '\\' && raw[i+7] == 'u' {
				if r2, ok := hex4(raw, i+8); ok {
					if dec := utf16.DecodeRune(r, r2); dec != unicode.ReplacementChar {
						return dec, i + 12
					}
				}
			}
			return unicode.ReplacementChar, i + 6
		}
		return r, i + 6
	default:
		return rune(raw[i+1]), i + 2
	}
}

func hex4(raw []byte, i int) (rune, bool) {
	if i+4 > len(raw) {
		return 0, false
	}
	var r rune
	for _, c := range raw[i : i+4] {
		switch {
		case '0' <= c && c <= '9':
			c -= '0'
		case 'a' <= c && c <= 'f':
			c = c - 'a' + 10
		case 'A' <= c && c <= 'F':
			c = c - 'A' + 10
		default:
			return 0, false
		}
		r = r*16 + rune(c)
	}
	return r, true
}

// foldRune 返回 r 所在大小写折叠轨道上的固定代表，只有大小写不同的字符映射到
// 同一个值（与 encoding/json 结构体字段的大小写不敏感匹配一致，覆盖 ſ、开尔文
// 符号 K）。拉丁字母取小写。
func foldRune(r rune) rune {
	if r < utf8.RuneSelf {
		// ASCII 字母的轨道代表就是其小写（k、s 的非 ASCII 成员都更大）。
		if 'A' <= r && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}
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

// isRepeatableMultipartField 报告表单字段是否按列表语义合法重复：以 [] 结尾的
// 数组字段（如 timestamp_granularities[]）、多图输入（image、image[...]、
// Grok 媒体的 image_url）。其余文本字段（model、n、size、quality、duration 等
// 计费/策略标量）出现多次即为歧义。
func isRepeatableMultipartField(name string) bool {
	return strings.HasSuffix(name, "[]") || name == "image" || name == "image_url" || strings.HasPrefix(name, "image[")
}

// multipartConflictingField 检查 multipart 表单：同名标量字段出现多次（文件与
// 文本 part 合并计数，列表字段见 isRepeatableMultipartField）；名为 model /
// session 的文件 part（网关 handler 跳过文件 part，宽容的上游可能当字段读）；
// 没有可解析名字或带多个 Content-Disposition 头的 part（各解析器可能取不同的
// 名字）；session 字段的 JSON 值内部按 JSON 规则检查。
//
// parts 返回成功解析出的 part 数，供调用方判断请求体是否同时含 multipart 内容。
func multipartConflictingField(contentType string, body []byte) (field string, conflict bool, parts int) {
	_, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		return "", false, 0
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return "", false, 0
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	seen := make(map[string]struct{})
	for {
		part, err := reader.NextPart()
		if err != nil {
			return "", false, parts
		}
		parts++
		name := strings.TrimSpace(part.FormName())
		if name == "" || len(part.Header.Values("Content-Disposition")) > 1 {
			return "Content-Disposition", true, parts
		}
		isFile := part.FileName() != ""
		if isFile && (name == "model" || name == "session") {
			return name, true, parts
		}
		if isRepeatableMultipartField(name) {
			continue
		}
		if _, dup := seen[name]; dup {
			return name, true, parts
		}
		seen[name] = struct{}{}
		if isFile || name != "session" {
			continue
		}
		data, err := io.ReadAll(part)
		if err != nil {
			continue
		}
		if doc := TrimUTF8BOM(data); LooksLikeJSON(doc) {
			if field, conflict := scanDuplicateMember(doc); conflict {
				return "session." + field, true, parts
			}
		}
	}
}
