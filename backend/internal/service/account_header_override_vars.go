package service

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"golang.org/x/net/http/httpguts"
)

// 请求头覆写变量（magic variables）：覆写值中的 {{name}} 在转发时替换为本次请求的
// 网关身份，用于把"是哪个用户发的请求"逐层上报给上游，便于排错与违规溯源。
//
// 不变式（改动前必读）：
//   - 仅 api_key 账号解析变量。其它类型（含 Grok OAuth）即使配置了带变量的覆写值，
//     该请求头也整条不发送：官方订阅上游绝不能收到网关内部身份。
//   - 变量取不到值（无用户上下文的后台请求、未知变量名）时整条请求头不发送，并且
//     同名的既有请求头仍会被删除——客户端不能借此伪造上报头。
//   - 只对管理员配置的模板做替换，替换结果不再二次解析；客户端请求内容永远不参与插值。
const (
	headerOverrideVarOpen  = "{{"
	headerOverrideVarClose = "}}"

	HeaderOverrideVarUserID    = "user_id"
	HeaderOverrideVarAPIKeyID  = "api_key_id"
	HeaderOverrideVarGroupID   = "group_id"
	HeaderOverrideVarRequestID = "request_id"
)

var headerOverrideVariableNames = map[string]struct{}{
	HeaderOverrideVarUserID:    {},
	HeaderOverrideVarAPIKeyID:  {},
	HeaderOverrideVarGroupID:   {},
	HeaderOverrideVarRequestID: {},
}

// HeaderOverrideRequestScope 是一次入站请求的上报身份，以及本次请求实际上报给各账号的
// 请求头记录（供错误记录展示）。由 API Key 认证中间件创建并挂到请求 context。
type HeaderOverrideRequestScope struct {
	userID   int64
	apiKeyID int64
	groupID  int64

	mu       sync.Mutex
	reported map[int64]map[string]string // accountID -> 请求头名(小写) -> 实际发送的值
}

type headerOverrideScopeKey struct{}

type headerOverrideConnectionScopeKey struct{}

// withHeaderOverrideConnectionScope 标记 ctx 用于构造"连接级"请求头（OpenAI WS 握手）。
// 一条上游连接会承载多次请求，{{request_id}} 在握手头里无法如实表达，因此不解析，
// 含它的请求头整条不发送；user_id 等在连接生命周期内稳定的变量照常解析。
func withHeaderOverrideConnectionScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, headerOverrideConnectionScopeKey{}, true)
}

// WithHeaderOverrideRequestScope 把本次请求的网关身份挂到 context。
// 非正数的 ID 视为"无该身份"，对应变量解析失败。
func WithHeaderOverrideRequestScope(ctx context.Context, userID, apiKeyID int64, groupID *int64) context.Context {
	scope := &HeaderOverrideRequestScope{userID: userID, apiKeyID: apiKeyID}
	if groupID != nil {
		scope.groupID = *groupID
	}
	return context.WithValue(ctx, headerOverrideScopeKey{}, scope)
}

// withoutHeaderOverrideRequestScope 去掉 ctx 中的入站用户身份。
// 用于结果会被跨用户缓存/共享的上游请求（模型列表等）：这类请求不代表某个用户的
// 推理流量，不上报身份；同时避免逐用户/逐请求的头把按请求头分桶的缓存打散。
func withoutHeaderOverrideRequestScope(ctx context.Context) context.Context {
	if headerOverrideScopeFromContext(ctx) == nil {
		return ctx
	}
	return context.WithValue(ctx, headerOverrideScopeKey{}, (*HeaderOverrideRequestScope)(nil))
}

func headerOverrideScopeFromContext(ctx context.Context) *HeaderOverrideRequestScope {
	if ctx == nil {
		return nil
	}
	scope, _ := ctx.Value(headerOverrideScopeKey{}).(*HeaderOverrideRequestScope)
	return scope
}

func (s *HeaderOverrideRequestScope) record(accountID int64, lowerName, value string) {
	if s == nil || accountID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reported == nil {
		s.reported = make(map[int64]map[string]string)
	}
	headers := s.reported[accountID]
	if headers == nil {
		headers = make(map[string]string)
		s.reported[accountID] = headers
	}
	headers[lowerName] = value
}

// reportedHeaders 返回本次请求上报给指定账号的请求头副本；没有上报时返回 nil。
func (s *HeaderOverrideRequestScope) reportedHeaders(accountID int64) map[string]string {
	if s == nil || accountID <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	headers := s.reported[accountID]
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for name, value := range headers {
		out[name] = value
	}
	return out
}

// headerOverrideReportedHeaders 返回 ctx 所属请求上报给指定账号的请求头。
func headerOverrideReportedHeaders(ctx context.Context, accountID int64) map[string]string {
	return headerOverrideScopeFromContext(ctx).reportedHeaders(accountID)
}

func headerOverrideHasVariable(value string) bool {
	return strings.Contains(value, headerOverrideVarOpen)
}

// walkHeaderOverrideTemplate 逐段扫描模板：字面量交给 literal，变量名交给 variable。
// 任何 "{{" 都必须构成一个完整的 {{name}}（name 两侧允许空白）；否则返回 false。
// variable 返回 false 时中止并整体返回 false。
func walkHeaderOverrideTemplate(value string, literal func(string), variable func(string) bool) bool {
	rest := value
	for {
		open := strings.Index(rest, headerOverrideVarOpen)
		if open < 0 {
			literal(rest)
			return true
		}
		literal(rest[:open])
		rest = rest[open+len(headerOverrideVarOpen):]
		end := strings.Index(rest, headerOverrideVarClose)
		if end < 0 {
			return false
		}
		if !variable(strings.TrimSpace(rest[:end])) {
			return false
		}
		rest = rest[end+len(headerOverrideVarClose):]
	}
}

// validateHeaderOverrideTemplate 校验覆写值中的变量语法与变量名（保存路径）。
func validateHeaderOverrideTemplate(lowerName, value string) error {
	if !headerOverrideHasVariable(value) {
		return nil
	}
	unknown := ""
	ok := walkHeaderOverrideTemplate(value, func(string) {}, func(name string) bool {
		if _, known := headerOverrideVariableNames[name]; !known {
			unknown = name
			return false
		}
		return true
	})
	if ok {
		return nil
	}
	if unknown != "" {
		return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
			"header %q uses unknown variable %q", lowerName, unknown)
	}
	return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
		"header %q has a malformed variable placeholder", lowerName)
}

// headerOverrideRedactedLiteral 在可记录形态中代替模板的固定文本。
const headerOverrideRedactedLiteral = "…"

// expandHeaderOverrideTemplate 用本次请求的身份替换模板中的变量。
// 任一变量无法解析、或结果不是合法的请求头值时 ok 为 false，调用方必须整条不发送。
//
// loggable 是可写入错误记录的形态，固定文本可能是中转密钥（如 "sk-xxx-{{user_id}}"），
// 不得进入日志：
//   - 模板只有变量时，loggable 就是实际发送的值；
//   - 混有固定文本时，固定文本替换为 "…"，变量写成 name=value 以便辨认。
func expandHeaderOverrideTemplate(ctx context.Context, value string) (expanded, loggable string, ok bool) {
	scope := headerOverrideScopeFromContext(ctx)
	var sent, tagged strings.Builder
	hasLiteral := false
	ok = walkHeaderOverrideTemplate(value, func(literal string) {
		_, _ = sent.WriteString(literal)
		if strings.TrimSpace(literal) != "" {
			hasLiteral = true
			_, _ = tagged.WriteString(headerOverrideRedactedLiteral)
		}
	}, func(name string) bool {
		resolved := resolveHeaderOverrideVariable(ctx, scope, name)
		if resolved == "" {
			return false
		}
		_, _ = sent.WriteString(resolved)
		_, _ = tagged.WriteString(name + "=" + resolved)
		return true
	})
	if !ok {
		return "", "", false
	}
	expanded = strings.TrimSpace(sent.String())
	if expanded == "" || len(expanded) > maxHeaderOverrideValueLength || !httpguts.ValidHeaderFieldValue(expanded) {
		return "", "", false
	}
	if !hasLiteral {
		return expanded, expanded, true
	}
	return expanded, tagged.String(), true
}

func resolveHeaderOverrideVariable(ctx context.Context, scope *HeaderOverrideRequestScope, name string) string {
	// 没有入站用户身份（账号测试、模型同步、探针等后台请求）时一律不解析，
	// 避免把管理端操作的 request_id 当成用户请求上报。
	if scope == nil || scope.userID <= 0 {
		return ""
	}
	positive := func(id int64) string {
		if id <= 0 {
			return ""
		}
		return strconv.FormatInt(id, 10)
	}
	switch name {
	case HeaderOverrideVarUserID:
		return positive(scope.userID)
	case HeaderOverrideVarAPIKeyID:
		return positive(scope.apiKeyID)
	case HeaderOverrideVarGroupID:
		return positive(scope.groupID)
	case HeaderOverrideVarRequestID:
		if connectionScoped, _ := ctx.Value(headerOverrideConnectionScopeKey{}).(bool); connectionScoped {
			return ""
		}
		requestID, _ := ctx.Value(ctxkey.RequestID).(string)
		return sanitizeHeaderOverrideRequestID(requestID)
	default:
		return ""
	}
}

const maxHeaderOverrideRequestIDLength = 128

// sanitizeHeaderOverrideRequestID 约束代入模板的请求 ID。
// 请求 ID 可由客户端通过 X-Request-ID 自带，是变量里唯一受客户端控制的内容：
// 原样代入组合模板（如 "u={{user_id}};r={{request_id}}"）会让客户端借分隔符伪造
// 其它身份字段。因此只保留 [A-Za-z0-9._-]，其余字符替换为 "_" 并限长；
// 不直接丢弃整条请求头，否则客户端可借此让上报头消失。
func sanitizeHeaderOverrideRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if len(requestID) > maxHeaderOverrideRequestIDLength {
		requestID = requestID[:maxHeaderOverrideRequestIDLength]
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, requestID)
}

// supportsHeaderOverrideVariables 报告账号是否允许解析覆写变量：仅 api_key 账号。
func (a *Account) supportsHeaderOverrideVariables() bool {
	return a != nil && a.Type == AccountTypeAPIKey
}

// ValidateHeaderOverrideVariableUsage 在保存路径拒绝非 api_key 账号使用覆写变量。
// 运行时 ApplyHeaderOverrides 仍会独立拦截，这里只是让管理员尽早看到明确的错误。
func ValidateHeaderOverrideVariableUsage(accountType string, credentials map[string]any) error {
	if accountType == AccountTypeAPIKey || credentials == nil {
		return nil
	}
	for name, value := range stringMappingFromRaw(credentials[credKeyHeaderOverrides]) {
		if headerOverrideHasVariable(value) {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"header %q uses variables, which are only available on API key accounts", name)
		}
	}
	return nil
}

// headerOverrideReportedIdentity 返回出站请求头中由覆写变量生成的部分的规范串
// （按名排序的 name=value）。用于会跨请求复用的上游连接（OpenAI WS 连接池）：
// 握手头在建连时固定，上报身份不同的请求不得共用同一条连接。
func (a *Account) headerOverrideReportedIdentity(h http.Header) string {
	if !a.supportsHeaderOverrideVariables() {
		return ""
	}
	var names []string
	for name, value := range a.GetHeaderOverrides() {
		if headerOverrideHasVariable(value) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		_, _ = b.WriteString(name)
		_ = b.WriteByte('=')
		_, _ = b.WriteString(strings.Join(headerValuesFold(h, name), ","))
		_ = b.WriteByte('\n')
	}
	return b.String()
}

func headerValuesFold(h http.Header, lowerName string) []string {
	var values []string
	for existing, vals := range h {
		if strings.EqualFold(existing, lowerName) {
			values = append(values, vals...)
		}
	}
	return values
}
