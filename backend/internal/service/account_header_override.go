package service

import (
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"golang.org/x/net/http/httpguts"
)

// 请求头覆写（header override）：对 Anthropic / OpenAI 平台的 api_key 账号
// 以及 Grok 平台的 api_key / oauth 账号生效。
// 管理员在账号上配置一组 header name -> value，转发到上游前用配置值覆盖同名请求头
// （匹配不区分大小写）；value 为空的条目视为"未填写"，不参与覆盖。
const (
	credKeyHeaderOverrideEnabled = "header_override_enabled"
	credKeyHeaderOverrides       = "header_overrides"

	maxHeaderOverrideEntries     = 64
	maxHeaderOverrideNameLength  = 200
	maxHeaderOverrideValueLength = 8192
)

// headerOverrideBlockedNames 禁止覆写的请求头（小写）。
//   - 连接控制/逐跳头：由 HTTP 栈管理，覆写会破坏请求传输；
//   - host/content-length：由 Go 的 Request.Host / ContentLength 字段管理，header 覆写不生效或产生冲突；
//   - content-type：承载报文框架信息（multipart boundary 为每请求随机值），静态覆写必然与 body 不匹配；
//   - authorization/x-api-key/cookie 等：上游认证头由账号凭据统一注入，禁止通过覆写篡改或重新引入；
//   - accept-encoding：强制压缩会破坏网关对上游流式响应（SSE/usage）的解析；
//   - sec-websocket-*：WebSocket 握手头由拨号器管理（OpenAI WS 模式）；
//   - session_id/x-claude-code-session-id/x-grok-conv-id 等：逐请求会话隔离头，
//     固定值会造成会话串扰。
var headerOverrideBlockedNames = map[string]struct{}{
	"host":                     {},
	"content-length":           {},
	"content-type":             {},
	"transfer-encoding":        {},
	"connection":               {},
	"keep-alive":               {},
	"proxy-authenticate":       {},
	"proxy-authorization":      {},
	"proxy-connection":         {},
	"te":                       {},
	"trailer":                  {},
	"upgrade":                  {},
	"authorization":            {},
	"x-api-key":                {},
	"x-goog-api-key":           {},
	"cookie":                   {},
	"accept-encoding":          {},
	"sec-websocket-key":        {},
	"sec-websocket-version":    {},
	"sec-websocket-extensions": {},
	"sec-websocket-protocol":   {},
	"sec-websocket-accept":     {},
	"session_id":               {},
	"conversation_id":          {},
	"x-codex-turn-state":       {},
	"x-codex-turn-metadata":    {},
	"chatgpt-account-id":       {},
	"x-claude-code-session-id": {},
	"x-client-request-id":      {},
	"x-grok-conv-id":           {},
}

func isHeaderOverrideBlockedName(lowerName string) bool {
	_, blocked := headerOverrideBlockedNames[lowerName]
	return blocked
}

// IsHeaderOverrideEligible 报告账号类型是否支持请求头覆写。
// Anthropic / OpenAI 仅开放 api_key 账号；Grok 额外开放 oauth 账号——
// 订阅流量改发自定义转发地址时，通常需要补充中间层要求的准入头。
func (a *Account) IsHeaderOverrideEligible() bool {
	if a == nil {
		return false
	}
	switch a.Platform {
	case PlatformAnthropic, PlatformOpenAI:
		return a.Type == AccountTypeAPIKey
	case PlatformGrok:
		return a.Type == AccountTypeAPIKey || a.Type == AccountTypeOAuth
	default:
		return false
	}
}

// IsHeaderOverrideEnabled 报告账号是否启用了请求头覆写。
func (a *Account) IsHeaderOverrideEnabled() bool {
	if !a.IsHeaderOverrideEligible() || a.Credentials == nil {
		return false
	}
	enabled, ok := a.Credentials[credKeyHeaderOverrideEnabled].(bool)
	return ok && enabled
}

// GetHeaderOverrides 返回生效的请求头覆写表（key 统一小写）。
// 未启用、不符合平台/类型条件或配置为空时返回 nil。
// 空 value 的条目（模板占位）与非法/禁止的 header 名会被跳过。
// 结果带热路径缓存（同 GetModelMapping 先例）：同一 credentials 映射在
// 一次请求 / 一条 WS 会话内的多次调用只做一次解析与校验。
func (a *Account) GetHeaderOverrides() map[string]string {
	if !a.IsHeaderOverrideEnabled() {
		return nil
	}
	rawMapping, rawIsAnyMap := a.Credentials[credKeyHeaderOverrides].(map[string]any)
	if !rawIsAnyMap {
		// 非 JSON 反序列化产物（如直接注入的 map[string]string）：直接解析，不缓存
		return resolveHeaderOverrides(stringMappingFromRaw(a.Credentials[credKeyHeaderOverrides]))
	}

	credentialsPtr := mapPtr(a.Credentials)
	rawPtr := mapPtr(rawMapping)
	rawLen := len(rawMapping)
	rawSig := uint64(0)
	rawSigReady := false

	if a.headerOverrideCacheReady &&
		a.headerOverrideCacheCredentialsPtr == credentialsPtr &&
		a.headerOverrideCacheRawPtr == rawPtr &&
		a.headerOverrideCacheRawLen == rawLen {
		rawSig = modelMappingSignature(rawMapping)
		rawSigReady = true
		if a.headerOverrideCacheRawSig == rawSig {
			return a.headerOverrideCache
		}
	}

	overrides := resolveHeaderOverrides(stringMappingFromRaw(rawMapping))
	if !rawSigReady {
		rawSig = modelMappingSignature(rawMapping)
	}

	a.headerOverrideCache = overrides
	a.headerOverrideCacheReady = true
	a.headerOverrideCacheCredentialsPtr = credentialsPtr
	a.headerOverrideCacheRawPtr = rawPtr
	a.headerOverrideCacheRawLen = rawLen
	a.headerOverrideCacheRawSig = rawSig
	return overrides
}

// resolveHeaderOverrides 解析并防御性过滤原始覆写表：保存路径已做校验，
// 这里兜底未经 Normalize 落库的数据（含名单扩充前保存的旧配置），非法条目直接跳过。
func resolveHeaderOverrides(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	result := make(map[string]string, len(raw))
	for name, value := range raw {
		lowerName, value, err := normalizeHeaderOverrideEntry(name, value)
		if err != nil || lowerName == "" || value == "" {
			continue
		}
		result[lowerName] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// HeaderOverrideValue 返回指定 header（小写名）的生效覆写值。
// 供转发链路在 header 写入前感知覆写结果（如 anthropic-beta 需要参与 body 净化）。
func (a *Account) HeaderOverrideValue(lowerName string) (string, bool) {
	value, ok := a.GetHeaderOverrides()[lowerName]
	return value, ok
}

// ApplyHeaderOverrides 将账号配置的请求头覆写应用到出站请求头。
// 对每个覆写条目：先删除所有大小写变体（转发链路会以 wire casing 直接写入 map，
// 可能存在非 canonical key），再按已知 wire casing 写入，避免产生重复头。
// 账号未启用或不符合条件时为 no-op，可安全地在 OAuth/api_key 共用的构建器中调用。
func (a *Account) ApplyHeaderOverrides(h http.Header) {
	if h == nil {
		return
	}
	overrides := a.GetHeaderOverrides()
	if len(overrides) == 0 {
		return
	}
	// 覆写名两两不同（大小写不敏感）且各自只操作同名键，应用顺序不影响结果。
	// 全量 EqualFold 扫描兜底删除任意 casing 的既有键：透传链路可能保留客户端
	// 原始 casing，非 canonical/wire casing 的键 deleteHeaderAllForms 覆盖不到。
	for name, value := range overrides {
		for existing := range h {
			if strings.EqualFold(existing, name) {
				delete(h, existing)
			}
		}
		h[resolveWireCasing(name)] = []string{value}
	}
}

// NormalizeHeaderOverrideCredentials 校验并原地规范化 credentials 中的请求头覆写字段。
// 供账号创建/更新/批量更新的保存路径调用；credentials 未携带相关字段时为 no-op。
// 规范化内容：header 名转小写并去除首尾空白，value 去除首尾空白，丢弃名和值均为空的条目。
func NormalizeHeaderOverrideCredentials(credentials map[string]any) error {
	if credentials == nil {
		return nil
	}
	if raw, ok := credentials[credKeyHeaderOverrideEnabled]; ok && raw != nil {
		if _, isBool := raw.(bool); !isBool {
			return infraerrors.New(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"header_override_enabled must be a boolean")
		}
	}
	raw, ok := credentials[credKeyHeaderOverrides]
	if !ok || raw == nil {
		return nil
	}

	var entries map[string]any
	switch m := raw.(type) {
	case map[string]any:
		entries = m
	case map[string]string:
		entries = make(map[string]any, len(m))
		for k, v := range m {
			entries[k] = v
		}
	default:
		return infraerrors.New(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
			"header_overrides must be an object of header name to string value")
	}

	if len(entries) > maxHeaderOverrideEntries {
		return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
			"header_overrides supports at most %d entries", maxHeaderOverrideEntries)
	}

	normalized := make(map[string]any, len(entries))
	for name, rawValue := range entries {
		value, isString := rawValue.(string)
		if !isString {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"header %q value must be a string", name)
		}
		lowerName, value, err := normalizeHeaderOverrideEntry(name, value)
		if err != nil {
			return err
		}
		if lowerName == "" {
			continue // 丢弃完全为空的占位行
		}
		if _, dup := normalized[lowerName]; dup {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"duplicate header name %q (matching is case-insensitive)", lowerName)
		}
		normalized[lowerName] = value
	}
	credentials[credKeyHeaderOverrides] = normalized
	return nil
}

// normalizeHeaderOverrideEntry 校验并规范化单个覆写条目，保存路径（Normalize，err → 400）
// 与应用路径（resolveHeaderOverrides，err → 跳过）共用同一套规则，避免两处校验漂移。
// 名和值均为空表示空占位行，返回 ("", "", nil)；空 value 的具名条目合法（模板占位）。
func normalizeHeaderOverrideEntry(name, value string) (string, string, error) {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	value = strings.TrimSpace(value)
	if lowerName == "" {
		if value == "" {
			return "", "", nil
		}
		return "", "", infraerrors.New(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
			"header name must not be empty")
	}
	if len(lowerName) > maxHeaderOverrideNameLength {
		return "", "", infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
			"header name %q exceeds %d characters", lowerName, maxHeaderOverrideNameLength)
	}
	if !httpguts.ValidHeaderFieldName(lowerName) {
		return "", "", infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
			"invalid header name %q", lowerName)
	}
	if isHeaderOverrideBlockedName(lowerName) {
		return "", "", infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
			"header %q is not allowed to be overridden", lowerName)
	}
	if len(value) > maxHeaderOverrideValueLength {
		return "", "", infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
			"header %q value exceeds %d characters", lowerName, maxHeaderOverrideValueLength)
	}
	if !httpguts.ValidHeaderFieldValue(value) {
		return "", "", infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
			"header %q has an invalid value", lowerName)
	}
	return lowerName, value, nil
}
