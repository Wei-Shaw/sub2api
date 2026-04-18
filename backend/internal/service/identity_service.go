package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 预编译正则表达式（避免每次调用重新编译）
var (
	// 匹配 User-Agent 版本号: xxx/x.y.z
	userAgentVersionRegex = regexp.MustCompile(`/(\d+)\.(\d+)\.(\d+)`)
)

// sessionHashCtxKeyType keys the per-request sticky-session hash that
// RewriteUserIDWithMasking uses to keep metadata.session_id stable across
// concurrent requests belonging to the same client conversation.
type sessionHashCtxKeyType struct{}

var sessionHashCtxKey sessionHashCtxKeyType

// WithSessionHash returns a context carrying the given session hash. Gateway
// entry points (Forward, ForwardCountTokens) stash the hash they compute for
// sticky account routing so that RewriteUserIDWithMasking can reuse it when
// generating a sticky session UUID.
func WithSessionHash(ctx context.Context, sessionHash string) context.Context {
	if sessionHash == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionHashCtxKey, sessionHash)
}

// SessionHashFromContext extracts the session hash previously stashed by
// WithSessionHash. Returns "" when absent.
func SessionHashFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(sessionHashCtxKey).(string)
	return v
}

// 默认指纹值（当客户端未提供时使用）
//
// Re-verified 2026-04-17 against a live capture of Claude Code 2.1.111 on
// Node.js 24.14.1 / macOS arm64 (capture tool: backend/tools/capture_fingerprint).
// UA bumped to 2.1.112 (latest on npm as of 2026-04-17) — patch-level Claude
// Code releases keep the same Stainless fields. Bundled @anthropic-ai/sdk is
// still 0.81.0. These values match the real CLI's request headers exactly —
// particularly:
//   - UserAgent:               "claude-cli/2.1.112 (external, sdk-cli)"  (note: "sdk-cli", NOT "cli")
//   - StainlessPackageVersion: "0.81.0"
//   - StainlessOS:             "MacOS"     (case: mixed, not "Linux")
//   - StainlessRuntimeVersion: "v24.14.1"
var defaultFingerprint = Fingerprint{
	UserAgent:               "claude-cli/2.1.112 (external, sdk-cli)",
	StainlessLang:           "js",
	StainlessPackageVersion: "0.81.0",
	StainlessOS:             "MacOS",
	StainlessArch:           "arm64",
	StainlessRuntime:        "node",
	StainlessRuntimeVersion: "v24.14.1",
}

// Fingerprint represents account fingerprint data
type Fingerprint struct {
	ClientID                string
	UserAgent               string
	StainlessLang           string
	StainlessPackageVersion string
	StainlessOS             string
	StainlessArch           string
	StainlessRuntime        string
	StainlessRuntimeVersion string
	UpdatedAt               int64 `json:",omitempty"` // Unix timestamp，用于判断是否需要续期TTL
}

// IdentityCache defines cache operations for identity service
type IdentityCache interface {
	GetFingerprint(ctx context.Context, accountID int64) (*Fingerprint, error)
	SetFingerprint(ctx context.Context, accountID int64, fp *Fingerprint) error
	// GetMaskedSessionID 获取固定的会话ID（用于会话ID伪装功能）
	// 返回的 sessionID 是一个 UUID 格式的字符串
	// 如果不存在或已过期（15分钟无请求），返回空字符串
	GetMaskedSessionID(ctx context.Context, accountID int64) (string, error)
	// SetMaskedSessionID 设置固定的会话ID，TTL 为 15 分钟
	// 每次调用都会刷新 TTL
	SetMaskedSessionID(ctx context.Context, accountID int64, sessionID string) error
	// GetStickySessionUUID 返回与 (accountID, sessionHash) 绑定的随机 session UUID。
	// 用于让一次 CLI 会话生命周期内多次请求共用同一个 metadata.session_id，
	// 贴近真实 Claude Code 行为（一次 CLI 调用整个会话共用一个 UUID，退出/新会话换）。
	// 未命中返回 "" + nil。
	GetStickySessionUUID(ctx context.Context, accountID int64, sessionHash string) (string, error)
	// SetStickySessionUUID 以 30 分钟 TTL 存储会话级 session UUID。
	SetStickySessionUUID(ctx context.Context, accountID int64, sessionHash string, sessionUUID string) error
}

// IdentityService 管理OAuth账号的请求身份指纹
type IdentityService struct {
	cache IdentityCache
}

// NewIdentityService 创建新的IdentityService
func NewIdentityService(cache IdentityCache) *IdentityService {
	return &IdentityService{cache: cache}
}

// GetOrCreateFingerprint 获取或创建账号的指纹
// 如果缓存存在，检测user-agent版本，新版本则更新
// 如果缓存不存在，生成随机ClientID并从请求头创建指纹，然后缓存
func (s *IdentityService) GetOrCreateFingerprint(ctx context.Context, accountID int64, headers http.Header) (*Fingerprint, error) {
	// 尝试从缓存获取指纹
	cached, err := s.cache.GetFingerprint(ctx, accountID)
	if err == nil && cached != nil {
		needWrite := false

		// 检查客户端的user-agent是否是更新版本
		clientUA := headers.Get("User-Agent")
		if clientUA != "" && isNewerVersion(clientUA, cached.UserAgent) {
			// 版本升级：merge 语义 — 仅更新请求中实际携带的字段，保留缓存值
			// 避免缺失的头被硬编码默认值覆盖（如新 CLI 版本 + 旧 SDK 默认值的不一致）
			mergeHeadersIntoFingerprint(cached, headers)
			needWrite = true
			logger.LegacyPrintf("service.identity", "Updated fingerprint for account %d: %s (merge update)", accountID, clientUA)
		} else if time.Since(time.Unix(cached.UpdatedAt, 0)) > 24*time.Hour {
			// 距上次写入超过24小时，续期TTL
			needWrite = true
		}

		if needWrite {
			cached.UpdatedAt = time.Now().Unix()
			if err := s.cache.SetFingerprint(ctx, accountID, cached); err != nil {
				logger.LegacyPrintf("service.identity", "Warning: failed to refresh fingerprint for account %d: %v", accountID, err)
			}
		}
		return cached, nil
	}

	// 缓存不存在或解析失败，创建新指纹
	fp := s.createFingerprintFromHeaders(headers)

	// 生成随机ClientID
	fp.ClientID = generateClientID()
	fp.UpdatedAt = time.Now().Unix()

	// 保存到缓存（7天TTL，每24小时自动续期）
	if err := s.cache.SetFingerprint(ctx, accountID, fp); err != nil {
		logger.LegacyPrintf("service.identity", "Warning: failed to cache fingerprint for account %d: %v", accountID, err)
	}

	logger.LegacyPrintf("service.identity", "Created new fingerprint for account %d with client_id: %s", accountID, fp.ClientID)
	return fp, nil
}

// createFingerprintFromHeaders 从请求头创建指纹
func (s *IdentityService) createFingerprintFromHeaders(headers http.Header) *Fingerprint {
	fp := &Fingerprint{}

	// 获取User-Agent
	if ua := headers.Get("User-Agent"); ua != "" {
		fp.UserAgent = ua
	} else {
		fp.UserAgent = defaultFingerprint.UserAgent
	}

	// 获取x-stainless-*头，如果没有则使用默认值
	fp.StainlessLang = getHeaderOrDefault(headers, "X-Stainless-Lang", defaultFingerprint.StainlessLang)
	fp.StainlessPackageVersion = getHeaderOrDefault(headers, "X-Stainless-Package-Version", defaultFingerprint.StainlessPackageVersion)
	fp.StainlessOS = getHeaderOrDefault(headers, "X-Stainless-OS", defaultFingerprint.StainlessOS)
	fp.StainlessArch = getHeaderOrDefault(headers, "X-Stainless-Arch", defaultFingerprint.StainlessArch)
	fp.StainlessRuntime = getHeaderOrDefault(headers, "X-Stainless-Runtime", defaultFingerprint.StainlessRuntime)
	fp.StainlessRuntimeVersion = getHeaderOrDefault(headers, "X-Stainless-Runtime-Version", defaultFingerprint.StainlessRuntimeVersion)

	return fp
}

// mergeHeadersIntoFingerprint 将请求头中实际存在的字段合并到现有指纹中（用于版本升级场景）
// 关键语义：请求中有的字段 → 用新值覆盖；缺失的头 → 保留缓存中的已有值
// 与 createFingerprintFromHeaders 的区别：后者用于首次创建，缺失头回退到 defaultFingerprint；
// 本函数用于升级更新，缺失头保留缓存值，避免将已知的真实值退化为硬编码默认值
func mergeHeadersIntoFingerprint(fp *Fingerprint, headers http.Header) {
	// User-Agent：版本升级的触发条件，一定存在
	if ua := headers.Get("User-Agent"); ua != "" {
		fp.UserAgent = ua
	}
	// X-Stainless-* 头：仅在请求中实际携带时才更新，否则保留缓存值
	mergeHeader(headers, "X-Stainless-Lang", &fp.StainlessLang)
	mergeHeader(headers, "X-Stainless-Package-Version", &fp.StainlessPackageVersion)
	mergeHeader(headers, "X-Stainless-OS", &fp.StainlessOS)
	mergeHeader(headers, "X-Stainless-Arch", &fp.StainlessArch)
	mergeHeader(headers, "X-Stainless-Runtime", &fp.StainlessRuntime)
	mergeHeader(headers, "X-Stainless-Runtime-Version", &fp.StainlessRuntimeVersion)
}

// mergeHeader 如果请求头中存在该字段则更新目标值，否则保留原值
func mergeHeader(headers http.Header, key string, target *string) {
	if v := headers.Get(key); v != "" {
		*target = v
	}
}

// getHeaderOrDefault 获取header值，如果不存在则返回默认值
func getHeaderOrDefault(headers http.Header, key, defaultValue string) string {
	if v := headers.Get(key); v != "" {
		return v
	}
	return defaultValue
}

// ApplyFingerprint 将指纹应用到请求头（覆盖原有的x-stainless-*头）
// 使用 setHeaderRaw 保持原始大小写（如 X-Stainless-OS 而非 X-Stainless-Os）
func (s *IdentityService) ApplyFingerprint(req *http.Request, fp *Fingerprint) {
	if fp == nil {
		return
	}

	// 设置user-agent
	if fp.UserAgent != "" {
		setHeaderRaw(req.Header, "User-Agent", fp.UserAgent)
	}

	// 设置x-stainless-*头（保持与 claude.DefaultHeaders 一致的大小写）
	if fp.StainlessLang != "" {
		setHeaderRaw(req.Header, "X-Stainless-Lang", fp.StainlessLang)
	}
	if fp.StainlessPackageVersion != "" {
		setHeaderRaw(req.Header, "X-Stainless-Package-Version", fp.StainlessPackageVersion)
	}
	if fp.StainlessOS != "" {
		setHeaderRaw(req.Header, "X-Stainless-OS", fp.StainlessOS)
	}
	if fp.StainlessArch != "" {
		setHeaderRaw(req.Header, "X-Stainless-Arch", fp.StainlessArch)
	}
	if fp.StainlessRuntime != "" {
		setHeaderRaw(req.Header, "X-Stainless-Runtime", fp.StainlessRuntime)
	}
	if fp.StainlessRuntimeVersion != "" {
		setHeaderRaw(req.Header, "X-Stainless-Runtime-Version", fp.StainlessRuntimeVersion)
	}
}

// extractRewriteTarget runs the shared preflight: empty-arg checks,
// metadata-object existence, user_id presence, parse. Returns "" when
// there's nothing to rewrite (caller should return body unchanged).
func extractRewriteTarget(body []byte, accountUUID, cachedClientID string) string {
	if len(body) == 0 || accountUUID == "" || cachedClientID == "" {
		return ""
	}
	metadata := gjson.GetBytes(body, "metadata")
	if !metadata.Exists() || metadata.Type == gjson.Null {
		return ""
	}
	if !strings.HasPrefix(strings.TrimSpace(metadata.Raw), "{") {
		return ""
	}
	r := metadata.Get("user_id")
	if !r.Exists() || r.Type != gjson.String {
		return ""
	}
	uid := r.String()
	if uid == "" {
		return ""
	}
	if ParseMetadataUserID(uid) == nil {
		return ""
	}
	return uid
}

// spliceUserID writes the new metadata.user_id back via sjson, short-
// circuiting when the new value equals the old.
func spliceUserID(body []byte, oldUserID, cachedClientID, accountUUID, sessionUUID, fingerprintUA string) []byte {
	version := ExtractCLIVersion(fingerprintUA)
	newUserID := FormatMetadataUserID(cachedClientID, accountUUID, sessionUUID, version)
	if newUserID == oldUserID {
		return body
	}
	newBody, err := sjson.SetBytes(body, "metadata.user_id", newUserID)
	if err != nil {
		return body
	}
	return newBody
}

// RewriteUserID 重写body中的metadata.user_id
// 支持旧拼接格式和新 JSON 格式的 user_id 解析，
// 根据 fingerprintUA 版本选择输出格式。
//
// Anti-fingerprinting fix (2026-04-15): mint a fresh cryptographically-random
// UUID per request. Device id (cachedClientID) remains stable for the account,
// but session churn looks natural — see RewriteUserIDWithMasking for the
// sticky-session variant that reuses one UUID across a CLI conversation.
func (s *IdentityService) RewriteUserID(body []byte, accountID int64, accountUUID, cachedClientID, fingerprintUA string) ([]byte, error) {
	userID := extractRewriteTarget(body, accountUUID, cachedClientID)
	if userID == "" {
		return body, nil
	}
	newSessionHash := generateRandomUUID()
	return spliceUserID(body, userID, cachedClientID, accountUUID, newSessionHash, fingerprintUA), nil
}

// RewriteUserIDWithMasking 重写body中的metadata.user_id
//
// Sticky session fix (2026-04-17): the previous implementation minted a fresh
// random UUID on *every* request, which also looks unnatural — a real Claude
// Code CLI session reuses the same session_id for the full lifetime of the
// invocation (dozens of turns, many tool calls). With per-request random UUIDs
// a gateway account exhibits thousands of distinct session_ids per day, each
// appearing exactly once, which is as easy to fingerprint as the old
// deterministic hash.
//
// New behavior:
//   - If the caller stashed a session hash via WithSessionHash, we look up a
//     sticky UUID in Redis keyed by (accountID, sessionHash). Cache hit →
//     reuse; miss → generate a fresh random UUID and store it (30-min TTL).
//   - Without a session hash, we fall back to RewriteUserID's random-per-call
//     behavior (preserves old semantics for callers that can't identify the
//     conversation).
func (s *IdentityService) RewriteUserIDWithMasking(ctx context.Context, body []byte, account *Account, accountUUID, cachedClientID, fingerprintUA string) ([]byte, error) {
	if account == nil {
		return body, nil
	}
	sessionHash := SessionHashFromContext(ctx)
	if sessionHash == "" || s.cache == nil {
		return s.RewriteUserID(body, account.ID, accountUUID, cachedClientID, fingerprintUA)
	}
	userID := extractRewriteTarget(body, accountUUID, cachedClientID)
	if userID == "" {
		return body, nil
	}
	sessionUUID, err := s.cache.GetStickySessionUUID(ctx, account.ID, sessionHash)
	if err != nil {
		slog.Warn("identity.sticky_session_lookup_failed",
			"account_id", account.ID, "error", err)
		sessionUUID = ""
	}
	if sessionUUID == "" {
		sessionUUID = generateRandomUUID()
		if setErr := s.cache.SetStickySessionUUID(ctx, account.ID, sessionHash, sessionUUID); setErr != nil {
			slog.Warn("identity.sticky_session_persist_failed",
				"account_id", account.ID, "error", setErr)
		}
	}
	return spliceUserID(body, userID, cachedClientID, accountUUID, sessionUUID, fingerprintUA), nil
}

// generateRandomUUID 生成随机 UUID v4 格式字符串
func generateRandomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// fallback: 使用时间戳生成
		h := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		b = h[:16]
	}

	// 设置 UUID v4 版本和变体位
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// generateClientID 生成64位十六进制客户端ID（32字节随机数）
func generateClientID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 极罕见的情况，使用时间戳+固定值作为fallback
		logger.LegacyPrintf("service.identity", "Warning: crypto/rand.Read failed: %v, using fallback", err)
		// 使用SHA256(当前纳秒时间)作为fallback
		h := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		return hex.EncodeToString(h[:])
	}
	return hex.EncodeToString(b)
}

// parseUserAgentVersion 解析user-agent版本号
// 例如：claude-cli/2.1.2 -> (2, 1, 2)
func parseUserAgentVersion(ua string) (major, minor, patch int, ok bool) {
	// 匹配 xxx/x.y.z 格式
	matches := userAgentVersionRegex.FindStringSubmatch(ua)
	if len(matches) != 4 {
		return 0, 0, 0, false
	}
	major, _ = strconv.Atoi(matches[1])
	minor, _ = strconv.Atoi(matches[2])
	patch, _ = strconv.Atoi(matches[3])
	return major, minor, patch, true
}

// extractProduct 提取 User-Agent 中 "/" 前的产品名
// 例如：claude-cli/2.1.22 (external, cli) -> "claude-cli"
func extractProduct(ua string) string {
	if idx := strings.Index(ua, "/"); idx > 0 {
		return strings.ToLower(ua[:idx])
	}
	return ""
}

// isNewerVersion 比较版本号，判断newUA是否比cachedUA更新
// 要求产品名一致（防止浏览器 UA 如 Mozilla/5.0 误判为更新版本）
func isNewerVersion(newUA, cachedUA string) bool {
	// 校验产品名一致性
	newProduct := extractProduct(newUA)
	cachedProduct := extractProduct(cachedUA)
	if newProduct == "" || cachedProduct == "" || newProduct != cachedProduct {
		return false
	}

	newMajor, newMinor, newPatch, newOk := parseUserAgentVersion(newUA)
	cachedMajor, cachedMinor, cachedPatch, cachedOk := parseUserAgentVersion(cachedUA)

	if !newOk || !cachedOk {
		return false
	}

	// 比较版本号
	if newMajor > cachedMajor {
		return true
	}
	if newMajor < cachedMajor {
		return false
	}

	if newMinor > cachedMinor {
		return true
	}
	if newMinor < cachedMinor {
		return false
	}

	return newPatch > cachedPatch
}
