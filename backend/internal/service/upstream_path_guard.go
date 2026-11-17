package service

import (
	"fmt"
	"strings"
)

// 上游 URL 路径片段护栏。
//
// 网关会把若干客户端可控的字符串（Responses 子路径、Gemini 模型名）拼进上游请求
// 的 URL path。约定：这些字符串只允许"结构惰性"的字符，即拼进去之后不可能改变
// 上游请求的路径结构；不符合的一律拒绝。
//
// 实现上是**闭集允许清单（默认拒绝）**，请勿改成"逐个拒绝已知的坏字符"：后者要求
// 穷举，漏一项就失效；默认拒绝则相反，清单外的写法天然被挡住，无需跟着改代码。
//
// 另外注意：到达业务代码的 c.Request.URL.Path 已经是百分号解码后的结果，因此校验
// 必须放在这一层，不能假设上游看到的路径与客户端书写形式一致。
//
// 这里只校验、不改写：把不合规输入自动修正成合规路径，会让上游收到与客户端意图
// 不同的请求，也会掩盖调用方的错误。

const (
	// maxUpstreamPathSegmentLen 单个路径片段长度上限。真实的 response id、模型名
	// 都远短于此，留足余量只为拒绝异常输入。
	maxUpstreamPathSegmentLen = 128
	// maxUpstreamPathSegments 后缀允许的片段数上限（如 /{id}/cancel 为 2）。
	maxUpstreamPathSegments = 8
)

// isSafeUpstreamPathSegmentByte 是闭集允许清单：只放行 `\w`（即 [A-Za-z0-9_]）
// 以及真实取值必需的 `-` 与 `.`。其余字符（含控制字符与非 ASCII）一律拒绝。
func isSafeUpstreamPathSegmentByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '_', b == '-', b == '.':
		return true
	default:
		return false
	}
}

// isSafeUpstreamPathSegment 判断 segment 能否原样拼进上游 URL 的一个 path 片段。
//
// 允许清单里唯一在路径语义中有特殊含义的字符是 `.`，因此额外要求片段不能只由点
// 组成——各类实现对这种片段的解释并不一致，直接拒绝最省心。
func isSafeUpstreamPathSegment(segment string) bool {
	if segment == "" || len(segment) > maxUpstreamPathSegmentLen {
		return false
	}
	dotsOnly := true
	for i := 0; i < len(segment); i++ {
		if !isSafeUpstreamPathSegmentByte(segment[i]) {
			return false
		}
		if segment[i] != '.' {
			dotsOnly = false
		}
	}
	return !dotsOnly
}

// sanitizedUpstreamPathSuffix 校验 "/a/b" 形态的路径后缀。
// ok=false 表示后缀不可转发，调用方必须拒绝请求，而不是降级成空后缀——否则
// /responses/compact 之类的请求语义会被静默改写。空后缀合法，表示"没有子路径"。
func sanitizedUpstreamPathSuffix(raw string) (string, bool) {
	suffix := strings.TrimSpace(raw)
	if suffix == "" {
		return "", true
	}
	if !strings.HasPrefix(suffix, "/") {
		return "", false
	}
	segments := strings.Split(strings.TrimPrefix(suffix, "/"), "/")
	if len(segments) > maxUpstreamPathSegments {
		return "", false
	}
	for _, segment := range segments {
		if !isSafeUpstreamPathSegment(segment) {
			return "", false
		}
	}
	return suffix, true
}

// validateUpstreamPathSegment 供 URL 构造点使用：不合规的片段直接变成显式错误，
// 不再继续构造与发出上游请求。
func validateUpstreamPathSegment(kind, segment string) error {
	if isSafeUpstreamPathSegment(strings.TrimSpace(segment)) {
		return nil
	}
	// 不回显原始输入，避免把它写进日志与错误响应。
	return fmt.Errorf("invalid %s for upstream url path", kind)
}
