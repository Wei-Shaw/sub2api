package httputil

import "errors"

const (
	// MaxJSONNestingDepth 是请求体进入任何 gjson 解析前允许的最大 JSON 容器嵌套层数。
	//
	// 正常 AI 网关请求非常浅：messages、content parts、tools、schema 通常只有十几到几十层。
	// 512 对合法客户端已经足够宽松，同时能在 gjson.ValidBytes/GetBytes 递归进入多百万层
	// 恶意 JSON 前直接拒绝请求，避免触发 Go runtime 不可 recover 的 stack overflow。
	MaxJSONNestingDepth = 512
)

// ErrJSONNestingTooDeep 表示请求体疑似 JSON 容器且嵌套深度超过安全上限。
var ErrJSONNestingTooDeep = errors.New("request JSON nesting too deep")

// ValidateJSONNestingDepth 对 JSON 容器嵌套深度做栈安全的迭代式预检。
//
// 这里有意不做完整 JSON 合法性校验：非法 JSON 仍交给原有 parser/validator 处理，避免改变
// 调用方既有语义。本函数只负责在 gjson.ValidBytes/GetBytes 可能递归解析前，拦截会导致栈
// 耗尽的超深容器。扫描会正确跳过字符串内容，因此字符串里的括号、转义引号和反斜杠都不会
// 影响容器深度计数。
//
// 只有首个非空白字节是 '{' 或 '[' 时才启动扫描。这样 multipart/form-data 等非 JSON body
// 不会被误判；同时即便客户端省略或伪造 Content-Type，只要正文实际是 JSON 容器仍会被保护。
func ValidateJSONNestingDepth(data []byte) error {
	return ValidateJSONNestingDepthLimit(data, MaxJSONNestingDepth)
}

// ValidateJSONNestingDepthLimit 是 ValidateJSONNestingDepth 的可测试形式。
func ValidateJSONNestingDepthLimit(data []byte, limit int) error {
	if limit <= 0 || !looksLikeJSONContainer(data) {
		return nil
	}

	depth := 0
	inString := false
	escaped := false

	for _, c := range data {
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > limit {
				return ErrJSONNestingTooDeep
			}
		case '}', ']':
			if depth > 0 {
				depth--
			}
		}
	}

	return nil
}

func looksLikeJSONContainer(data []byte) bool {
	for _, c := range data {
		switch c {
		case ' ', '\n', '\r', '\t':
			continue
		case '{', '[':
			return true
		default:
			return false
		}
	}
	return false
}
