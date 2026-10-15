package service

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openCodeSessionHeader         = "X-OpenCode-Session"
	openCodeInboundBodyContextKey = "opencode_inbound_body"

	// openCodeUpstreamUserAgent 是发往官方 OpenCode 上游的规范 User-Agent。
	// opencode.ai 前置 Cloudflare 按客户端 UA 签名做 bot 拦截：Python-urllib 等
	// 编程库 UA 会被 CF error code 1010 拒绝并返回 403。网关透传白名单放行
	// user-agent，客户端自报身份原样到达上游会命中 WAF；该 403 再被计入账号
	// 凭证类 strike，健康账号因此被自动禁用。
	//
	// 取值必须满足 Zen free tier 的版本闸门：低于 opencode/1.18.0 的 UA 会被
	// Zen 以 426 UpgradeRequired 拒绝（"OpenCode 1.18.0 or newer is required to
	// use the free tier"），该文案来自真实 Zen 响应。格式沿用真实 opencode 客户端
	// 的完整 UA（opencode/<ver> ai-sdk/provider-utils/<ver> runtime/bun/<ver>）。
	openCodeUpstreamUserAgent = "opencode/1.18.35 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14"

	// openCodeUpstreamUserAgentPrefix 标识真实 opencode 客户端 UA。命中该前缀的
	// 客户端自带版本号信息，可判定是否已满足 Zen 的1.18.0 版本下限。
	openCodeUpstreamUserAgentPrefix = "opencode/"

)

// openCodeFreeTierMinClientVersion 是 Zen free tier 要求的最低 opencode 客户端
// 版本（semver 三元组）。低于该版本返回 426 而非 403。
var openCodeFreeTierMinClientVersion = [3]int{1, 18, 0}

// rememberOpenCodeInboundBody keeps the client request body so protocol
// conversion (Responses↔Anthropic↔Chat Completions) can still recover
// prompt_cache_key / metadata.user_id after those fields are dropped.
func rememberOpenCodeInboundBody(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	c.Set(openCodeInboundBodyContextKey, body)
}

func openCodeInboundBodies(c *gin.Context) [][]byte {
	if c == nil {
		return nil
	}
	raw, ok := c.Get(openCodeInboundBodyContextKey)
	if !ok {
		return nil
	}
	body, ok := raw.([]byte)
	if !ok || len(body) == 0 {
		return nil
	}
	return [][]byte{body}
}

const openCodeBase62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var (
	openCodeIDMu    sync.Mutex
	openCodeLastMS  int64
	openCodeCounter int64
)

// generateOpenCodeID generates OpenCode client IDs using the official algorithm.
// Sessions use descending (descending=true) order; Messages use ascending order.
func generateOpenCodeID(prefix string, descending bool) string {
	openCodeIDMu.Lock()
	nowMS := time.Now().UnixMilli()
	if nowMS != openCodeLastMS {
		openCodeLastMS = nowMS
		openCodeCounter = 0
	}
	openCodeCounter++
	counter := openCodeCounter
	openCodeIDMu.Unlock()

	val := (uint64(nowMS) * 4096) + uint64(counter)
	if descending {
		val = ^val
	}

	buf := make([]byte, 6)
	for i := 0; i < 6; i++ {
		buf[i] = byte((val >> (40 - 8*i)) & 0xFF)
	}
	hexPart := hex.EncodeToString(buf)

	randBytes := make([]byte, 14)
	if _, err := rand.Read(randBytes); err != nil {
		for i := range randBytes {
			randBytes[i] = byte(i)
		}
	}
	randStr := make([]byte, 14)
	for i, b := range randBytes {
		randStr[i] = openCodeBase62Chars[b%62]
	}

	return prefix + "_" + hexPart + string(randStr)
}

// applyOpenCodeSessionHeader sets x-opencode-session on outbound inference
// requests. OpenCode Go requires a per-conversation value from 2026-09-05
// (MissingSessionID). Prefer caller headers, then the documented body session
// fields (OpenAI prompt_cache_key / Anthropic metadata.user_id), then any
// already applied account override. A generated UUID is last-resort only for
// probes and clients that omit every stable identifier — a new UUID each turn
// would miss upstream prompt cache.
func applyOpenCodeSessionHeader(c *gin.Context, account *Account, targetURL string, headers http.Header, bodies ...[]byte) {
	if account == nil || account.Type != AccountTypeAPIKey || headers == nil {
		return
	}
	if !shouldSendOpenCodeSessionHeader(account, targetURL) {
		return
	}

	payloads := append(openCodeInboundBodies(c), bodies...)
	sessionID := resolveOpenCodeSessionID(c, headers, shouldGenerateOpenCodeSession(account, targetURL), payloads...)
	if account.IsOpenCodeFreeTier() && (sessionID == "" || !strings.HasPrefix(sessionID, "ses_")) {
		sessionID = generateOpenCodeID("ses", true)
	}
	if sessionID != "" {
		for key := range headers {
			if strings.EqualFold(key, openCodeSessionHeader) || strings.EqualFold(key, "X-OpenCode-Session-ID") {
				delete(headers, key)
			}
		}
		headers.Set(openCodeSessionHeader, sessionID)
		headers.Set("X-OpenCode-Session-ID", sessionID)
	}

	if account.IsOpenCodeFreeTier() {
		if headers.Get("x-opencode-client") == "" {
			headers.Set("x-opencode-client", "cli")
		}
		if headers.Get("x-opencode-request") == "" {
			headers.Set("x-opencode-request", generateOpenCodeID("msg", false))
		}
	}

	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lowerKey := strings.ToLower(key)
			if strings.HasPrefix(lowerKey, "x-opencode-") {
				if strings.EqualFold(lowerKey, strings.ToLower(openCodeSessionHeader)) || strings.EqualFold(lowerKey, "x-opencode-session-id") {
					continue
				}
				for k := range headers {
					if strings.EqualFold(k, key) {
						delete(headers, k)
					}
				}
				for _, v := range values {
					headers.Add(key, v)
				}
			}
		}
	}
}

func shouldSendOpenCodeSessionHeader(account *Account, targetURL string) bool {
	if account != nil && account.IsOpenCodeGoPlan() {
		return true
	}
	return isOfficialOpenCodeHost(targetURL)
}

func shouldGenerateOpenCodeSession(account *Account, targetURL string) bool {
	if account != nil && (account.IsOpenCodeGoPlan() || account.IsOpenCodeFreeTier()) {
		return true
	}
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") &&
		strings.EqualFold(parsed.Hostname(), "opencode.ai") &&
		strings.Contains(parsed.Path, "/zen/go")
}

func isOfficialOpenCodeHost(targetURL string) bool {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") && strings.EqualFold(parsed.Hostname(), "opencode.ai")
}

func isOfficialCommandCodeHost(targetURL string) bool {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") && strings.EqualFold(parsed.Hostname(), "api.commandcode.ai")
}

// applyOpenCodeUpstreamUserAgent 将发往官方 OpenCode / Command Code 上游的出站
// User-Agent 收敛为规范客户端身份，覆盖客户端透传值与平台默认 UA。判定规则与
// x-opencode-session 同源：opencode 平台账号，或（任意平台账号）目标为官方主机。
// 必须先于 Account.ApplyHeaderOverrides 调用——账号 header_overrides 中显式
// 配置的 user-agent 仍拥有最终决定权。
//
// 真实 opencode 客户端自带满足 Zen 版本闸门的 UA 时原样保留：Zen 的 free tier
// 会对客户端版本做准入判断，出站降级到旧版本会把合法请求打成 426。编程库 UA
// （Python-urllib 等）仍收敛为规范值，避免命中前置 Cloudflare bot 拦截。
func applyOpenCodeUpstreamUserAgent(c *gin.Context, account *Account, targetURL string, headers http.Header) {
	if headers == nil {
		return
	}

	userAgent := ""
	switch {
	case isOfficialCommandCodeHost(targetURL):
		userAgent = CodexCanonicalUserAgent()
	case account != nil && account.IsOpenCodeGo(), isOfficialOpenCodeHost(targetURL):
		userAgent = openCodeCanonicalUserAgentForClient(c)
	default:
		return
	}
	if userAgent == "" {
		return
	}

	// 先删任意大小写变体再写入：透传链路与覆写直写 map 可能残留非 canonical 键。
	for key := range headers {
		if strings.EqualFold(key, "User-Agent") {
			delete(headers, key)
		}
	}
	headers.Set("User-Agent", userAgent)
}

// openCodeCanonicalUserAgentForClient 返回本次出站使用的 opencode 客户端 UA。
// 入站请求确实来自满足 Zen 版本下限的真实 opencode 客户端（携带 opencode/ 前缀
// 且版本足够新）时透传该 UA；否则回落到规范兜底值。c 为 nil 表示调用方没有
// 入站上下文（如账号连通性测试），此时一律使用兜底值。
func openCodeCanonicalUserAgentForClient(c *gin.Context) string {
	if c != nil && c.Request != nil {
		if incoming := strings.TrimSpace(c.Request.UserAgent()); incoming != "" {
			if meetsOpenCodeFreeTierMinVersion(incoming) {
				return incoming
			}
		}
	}
	return openCodeUpstreamUserAgent
}

// meetsOpenCodeFreeTierMinVersion 判断客户端 UA 是否声明了不低于 Zen free tier
// 要求的 opencode 版本。非 opencode/ 前缀或版本号不可解析时返回 false，交由调用方
// 回落到规范兜底 UA——保留 opencode/<version> 前缀即可通过 Cloudflare 侧识别。
func meetsOpenCodeFreeTierMinVersion(userAgent string) bool {
	if !strings.HasPrefix(userAgent, openCodeUpstreamUserAgentPrefix) {
		return false
	}
	version, ok := parseOpenCodeClientVersion(userAgent)
	if !ok {
		return false
	}
	return compareOpenCodeClientVersion(version, openCodeFreeTierMinClientVersion) >= 0
}

// parseOpenCodeClientVersion 从 opencode/<major>.<minor>.<patch> UA 中提取版本号。
// UA 形如 "opencode/1.18.34 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14"，
// 版本段之后以空白分隔，因此只解析第一段且容忍额外后缀。
func parseOpenCodeClientVersion(userAgent string) ([3]int, bool) {
	if !strings.HasPrefix(userAgent, openCodeUpstreamUserAgentPrefix) {
		return [3]int{}, false
	}
	rest := strings.TrimPrefix(userAgent, openCodeUpstreamUserAgentPrefix)
	version := rest
	if idx := strings.IndexFunc(rest, func(r rune) bool {
		return r == ' ' || r == '\t'
	}); idx >= 0 {
		version = rest[:idx]
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, part := range parts {
		if part == "" {
			return [3]int{}, false
		}
		n := 0
		for _, r := range part {
			if r < '0' || r > '9' {
				return [3]int{}, false
			}
			n = n*10 + int(r-'0')
			if n > 1<<20 {
				return [3]int{}, false
			}
		}
		out[i] = n
	}
	return out, true
}

// compareOpenCodeClientVersion 按 major→minor→patch 顺序比较两个版本号。
func compareOpenCodeClientVersion(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func resolveOpenCodeSessionID(c *gin.Context, headers http.Header, generate bool, bodies ...[]byte) string {
	if c != nil && c.Request != nil {
		if sessionID := sanitizeSessionID(c.GetHeader(openCodeSessionHeader)); sessionID != "" {
			return sessionID
		}
		if sessionID := sanitizeSessionID(c.GetHeader("X-OpenCode-Session-ID")); sessionID != "" {
			return sessionID
		}
		if sessionID := sanitizeSessionID(explicitOpenAIHeaderSessionID(c)); sessionID != "" {
			return sessionID
		}
		if sessionID := sanitizeSessionID(ClaudeCodeSessionIDFromHeader(c)); sessionID != "" {
			return sessionID
		}
	}
	for _, body := range bodies {
		if sessionID := sanitizeSessionID(openCodeSessionIDFromPayload(body)); sessionID != "" {
			return sessionID
		}
	}
	if sessionID := sanitizeSessionID(existingOpenCodeSessionHeader(headers)); sessionID != "" {
		return sessionID
	}
	if generate {
		return generateOpenCodeID("ses", true)
	}
	return ""
}

// openCodeSessionIDFromPayload reads the stable conversation id from documented
// request-body fields. Kimi Code / OpenAI clients send prompt_cache_key on
// Chat Completions and Responses; Anthropic clients send metadata.user_id on
// Messages. Neither field changes model behavior.
func openCodeSessionIDFromPayload(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	view := openAIRequestPayloadView(body)
	if sessionID := strings.TrimSpace(view.Get("prompt_cache_key").String()); sessionID != "" {
		return sessionID
	}
	return openCodeSessionIDFromMetadataUserID(view.Get("metadata.user_id").String())
}

func openCodeSessionIDFromMetadataUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	if strings.HasPrefix(userID, "{") {
		if sessionID := strings.TrimSpace(gjson.Get(userID, "session_id").String()); sessionID != "" {
			return sessionID
		}
	}
	return userID
}

func openCodeSessionHintBody(promptCacheKey string) []byte {
	key := strings.TrimSpace(promptCacheKey)
	if key == "" {
		return nil
	}
	return []byte(`{"prompt_cache_key":` + strconv.Quote(key) + `}`)
}

func existingOpenCodeSessionHeader(headers http.Header) string {
	if headers == nil {
		return ""
	}
	for key, values := range headers {
		if (strings.EqualFold(key, openCodeSessionHeader) || strings.EqualFold(key, "X-OpenCode-Session-ID")) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}
