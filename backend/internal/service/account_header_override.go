package service

import (
	"net/http"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"golang.org/x/net/http/httpguts"
)

// 请求头覆写（header override）：仅对 Anthropic / OpenAI 平台的 api_key 账号生效。
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
//   - authorization/x-api-key：上游认证头由账号凭据统一注入，禁止通过覆写篡改；
//   - accept-encoding：强制压缩会破坏网关对上游流式响应（SSE/usage）的解析；
//   - sec-websocket-*：WebSocket 握手头由拨号器管理（OpenAI WS 模式）；
//   - session_id/conversation_id 等：逐请求会话隔离头，固定值会造成会话串扰。
var headerOverrideBlockedNames = map[string]struct{REDACTED{
	"host":                     {REDACTED,
	"content-length":           {REDACTED,
	"transfer-encoding":        {REDACTED,
	"connection":               {REDACTED,
	"keep-alive":               {REDACTED,
	"proxy-authenticate":       {REDACTED,
	"proxy-authorization":      {REDACTED,
	"proxy-connection":         {REDACTED,
	"te":                       {REDACTED,
	"trailer":                  {REDACTED,
	"upgrade":                  {REDACTED,
	"authorization":            {REDACTED,
	"x-api-key":                {REDACTED,
	"accept-encoding":          {REDACTED,
	"sec-websocket-key":        {REDACTED,
	"sec-websocket-version":    {REDACTED,
	"sec-websocket-extensions": {REDACTED,
	"sec-websocket-protocol":   {REDACTED,
	"sec-websocket-accept":     {REDACTED,
	"session_id":               {REDACTED,
	"conversation_id":          {REDACTED,
	"x-codex-turn-state":       {REDACTED,
	"x-codex-turn-metadata":    {REDACTED,
	"chatgpt-account-id":       {REDACTED,
REDACTED

func isHeaderOverrideBlockedName(lowerName string) bool {
	_, blocked := headerOverrideBlockedNames[lowerName]
	return blocked
REDACTED

// IsHeaderOverrideEligible 报告账号类型是否支持请求头覆写。
// 目前仅开放 Anthropic / OpenAI 两个平台的 api_key 账号。
func (a *Account) IsHeaderOverrideEligible() bool {
	if a == nil || a.Type != AccountTypeAPIKey {
		return false
REDACTED
	return a.Platform == PlatformAnthropic || a.Platform == PlatformOpenAI
REDACTED

// IsHeaderOverrideEnabled 报告账号是否启用了请求头覆写。
func (a *Account) IsHeaderOverrideEnabled() bool {
	if !a.IsHeaderOverrideEligible() || a.Credentials == nil {
		return false
REDACTED
	enabled, ok := a.Credentials[credKeyHeaderOverrideEnabled].(bool)
	return ok && enabled
REDACTED

// GetHeaderOverrides 返回生效的请求头覆写表（key 统一小写）。
// 未启用、不符合平台/类型条件或配置为空时返回 nil。
// 空 value 的条目（模板占位）与非法/禁止的 header 名会被跳过。
func (a *Account) GetHeaderOverrides() map[string]string {
	if !a.IsHeaderOverrideEnabled() {
		return nil
REDACTED
	raw := stringMappingFromRaw(a.Credentials[credKeyHeaderOverrides])
	if len(raw) == 0 {
		return nil
REDACTED
	result := make(map[string]string, len(raw))
	for name, value := range raw {
		lowerName := strings.ToLower(strings.TrimSpace(name))
		value = strings.TrimSpace(value)
		if lowerName == "" || value == "" {
			continue
	REDACTED
		// 防御性过滤：保存路径已做校验，这里兜底未经 Normalize 落库的数据
		if len(lowerName) > maxHeaderOverrideNameLength || len(value) > maxHeaderOverrideValueLength {
			continue
	REDACTED
		if isHeaderOverrideBlockedName(lowerName) {
			continue
	REDACTED
		if !httpguts.ValidHeaderFieldName(lowerName) || !httpguts.ValidHeaderFieldValue(value) {
			continue
	REDACTED
		result[lowerName] = value
REDACTED
	if len(result) == 0 {
		return nil
REDACTED
	return result
REDACTED

// ApplyHeaderOverrides 将账号配置的请求头覆写应用到出站请求头。
// 对每个覆写条目：先删除所有大小写变体（转发链路会以 wire casing 直接写入 map，
// 可能存在非 canonical key），再按已知 wire casing 写入，避免产生重复头。
// 账号未启用或不符合条件时为 no-op，可安全地在 OAuth/api_key 共用的构建器中调用。
func (a *Account) ApplyHeaderOverrides(h http.Header) {
	if h == nil {
		return
REDACTED
	overrides := a.GetHeaderOverrides()
	if len(overrides) == 0 {
		return
REDACTED
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
REDACTED
	sort.Strings(names)
	for _, name := range names {
		for existing := range h {
			if strings.EqualFold(existing, name) {
				delete(h, existing)
		REDACTED
	REDACTED
		h[resolveWireCasing(name)] = []string{overrides[name]REDACTED
REDACTED
REDACTED

// NormalizeHeaderOverrideCredentials 校验并原地规范化 credentials 中的请求头覆写字段。
// 供账号创建/更新/批量更新的保存路径调用；credentials 未携带相关字段时为 no-op。
// 规范化内容：header 名转小写并去除首尾空白，value 去除首尾空白，丢弃名和值均为空的条目。
func NormalizeHeaderOverrideCredentials(credentials map[string]any) error {
	if credentials == nil {
		return nil
REDACTED
	if raw, ok := credentials[credKeyHeaderOverrideEnabled]; ok && raw != nil {
		if _, isBool := raw.(bool); !isBool {
			return infraerrors.New(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"header_override_enabled must be a boolean")
	REDACTED
REDACTED
	raw, ok := credentials[credKeyHeaderOverrides]
	if !ok || raw == nil {
		return nil
REDACTED

	var entries map[string]any
	switch m := raw.(type) {
	case map[string]any:
		entries = m
	case map[string]string:
		entries = make(map[string]any, len(m))
		for k, v := range m {
			entries[k] = v
	REDACTED
	default:
		return infraerrors.New(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
			"header_overrides must be an object of header name to string value")
REDACTED

	if len(entries) > maxHeaderOverrideEntries {
		return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
			"header_overrides supports at most %d entries", maxHeaderOverrideEntries)
REDACTED

	normalized := make(map[string]any, len(entries))
	for name, rawValue := range entries {
		value, isString := rawValue.(string)
		if !isString {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"header %q value must be a string", name)
	REDACTED
		lowerName := strings.ToLower(strings.TrimSpace(name))
		value = strings.TrimSpace(value)
		if lowerName == "" {
			if value == "" {
				continue // 丢弃完全为空的占位行
		REDACTED
			return infraerrors.New(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"header name must not be empty")
	REDACTED
		if len(lowerName) > maxHeaderOverrideNameLength {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"header name %q exceeds %d characters", lowerName, maxHeaderOverrideNameLength)
	REDACTED
		if !httpguts.ValidHeaderFieldName(lowerName) {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"invalid header name %q", lowerName)
	REDACTED
		if isHeaderOverrideBlockedName(lowerName) {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"header %q is not allowed to be overridden", lowerName)
	REDACTED
		if len(value) > maxHeaderOverrideValueLength {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"header %q value exceeds %d characters", lowerName, maxHeaderOverrideValueLength)
	REDACTED
		if !httpguts.ValidHeaderFieldValue(value) {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"header %q has an invalid value", lowerName)
	REDACTED
		if _, dup := normalized[lowerName]; dup {
			return infraerrors.Newf(http.StatusBadRequest, "INVALID_HEADER_OVERRIDE",
				"duplicate header name %q (matching is case-insensitive)", lowerName)
	REDACTED
		normalized[lowerName] = value
REDACTED
	credentials[credKeyHeaderOverrides] = normalized
	return nil
REDACTED
