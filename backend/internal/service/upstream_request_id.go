package service

import (
	"net/http"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/net/http/httpguts"
)

// AccountExtraUpstreamRequestIDHeader 是账户 extra 中的键，值为直接上游声明请求标识的响应头名。
// 未指定时不记录上游请求标识。
const AccountExtraUpstreamRequestIDHeader = "upstream_request_id_header"

const (
	maxUpstreamRequestIDHeaderNameLen = 64
	// maxUsageUpstreamRequestIDLen 与 usage_logs.upstream_request_id VARCHAR(128) 对齐。
	maxUsageUpstreamRequestIDLen = 128
)

// UpstreamRequestIDHeaderName 返回账户指定的上游请求标识头名，未指定时为空串。
func UpstreamRequestIDHeaderName(account *Account) string {
	if account == nil {
		return ""
	}
	return strings.TrimSpace(account.GetExtraString(AccountExtraUpstreamRequestIDHeader))
}

// UpstreamRequestIDFromHeaders 从直接上游的响应头解析请求标识。
// 只读账户指定的头；账户未指定头名时恒为空串。
func UpstreamRequestIDFromHeaders(account *Account, h http.Header) string {
	if len(h) == 0 {
		return ""
	}
	name := UpstreamRequestIDHeaderName(account)
	if name == "" {
		return ""
	}
	return strings.TrimSpace(h.Get(name))
}

// usageUpstreamRequestIDPtr 生成落库到 usage_logs.upstream_request_id 的值。
// WS 轮次没有 HTTP 响应头，保持 nil；超长时截断到列宽而不是让整条用量行失败。
func usageUpstreamRequestIDPtr(account *Account, h http.Header, wsMode bool) *string {
	if wsMode {
		return nil
	}
	id := UpstreamRequestIDFromHeaders(account, h)
	if id == "" {
		return nil
	}
	if len(id) > maxUsageUpstreamRequestIDLen {
		id = id[:maxUsageUpstreamRequestIDLen]
		for len(id) > 0 && !utf8.ValidString(id) {
			id = id[:len(id)-1]
		}
	}
	if id == "" {
		return nil
	}
	return &id
}

// ValidateUpstreamRequestIDHeaderExtra 校验并规范化 extra 中的上游请求标识头名：
// 必须是合法的 HTTP 头字段名且不超过 64 字节；空白值视为未指定并从 extra 中移除。
func ValidateUpstreamRequestIDHeaderExtra(extra map[string]any) error {
	if extra == nil {
		return nil
	}
	raw, ok := extra[AccountExtraUpstreamRequestIDHeader]
	if !ok || raw == nil {
		return nil
	}
	name, ok := raw.(string)
	if !ok {
		return infraerrors.BadRequest("INVALID_UPSTREAM_REQUEST_ID_HEADER",
			"upstream_request_id_header must be a string")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		delete(extra, AccountExtraUpstreamRequestIDHeader)
		return nil
	}
	if len(name) > maxUpstreamRequestIDHeaderNameLen || !httpguts.ValidHeaderFieldName(name) {
		return infraerrors.BadRequest("INVALID_UPSTREAM_REQUEST_ID_HEADER",
			"upstream_request_id_header must be a valid HTTP header name of at most 64 bytes")
	}
	extra[AccountExtraUpstreamRequestIDHeader] = name
	return nil
}
