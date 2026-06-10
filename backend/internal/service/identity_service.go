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

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 预编译正则表达式（避免每次调用重新编译）
var (
	// 匹配 User-Agent 版本号: xxx/x.y.z
	userAgentVersionRegex = regexp.MustCompile(`/(\d+)\.(\d+)\.(\d+)`)
)

// 默认指纹值（当客户端未提供时使用）。
// Must stay in sync with claude.DefaultHeaders to avoid fingerprint mismatch.
var defaultFingerprint = Fingerprint{
	UserAgent:               "claude-cli/2.1.116 (external, cli)",
	StainlessLang:           "js",
	StainlessPackageVersion: "0.94.0",
	StainlessOS:             "Linux",
	StainlessArch:           "arm64",
	StainlessRuntime:        "node",
	StainlessRuntimeVersion: "v22.11.0",
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
	// SetMaskedSessionID 设置固定的会话ID，指定过期时间
	SetMaskedSessionID(ctx context.Context, accountID int64, sessionID string, ttl time.Duration) error
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
//
// UA 白名单策略 (v0.1.127+): 只有 User-Agent 以 `claude-cli/X.Y.Z` 开头的请求
// 才会读写每账号 fingerprint 缓存。非 CC 客户端 (OpenClaw / ai-sdk / OpenAI SDK 等)
// 每次都现拿 defaultFingerprint 的副本,不读不写共享 Redis,避免跨客户端污染
// (2026-04 实测: OpenClaw UA=OpenAI/JS 会把共享账号的 fingerprint 缓存改成
// OpenAI SDK 指纹,导致真 Claude Code CLI 请求也带着 OpenAI/JS 头上游,触发
// Anthropic Third-party detection 拒绝)。
//
// 对真 CC 客户端:
//   - 缓存存在且 UA 仍是 claude-cli → 走原升级合并逻辑
//   - 缓存存在但 UA 被污染 (非 claude-cli) → 丢弃并重建
//   - 缓存不存在 → 从当前请求头创建并落盘
//
// Deprecated: 使用 GetOrCreateFingerprintForAccount，它支持 P1-1 的 per-account
// registration fingerprint。本方法保留向后兼容，对非 CC 客户端走 defaultFingerprint。
func (s *IdentityService) GetOrCreateFingerprint(ctx context.Context, accountID int64, headers http.Header) (*Fingerprint, error) {
	return s.getOrCreateFingerprintInternal(ctx, accountID, nil, headers)
}

// GetOrCreateFingerprintForAccount 获取或创建账号的指纹 (P1-1)。
//
// 与 GetOrCreateFingerprint 的关键区别：对非 claude-cli 客户端，会从 account.Extra 中
// 读取 registration_fingerprint 并覆盖 OS/Arch 字段。这样一个 macOS 浏览器注册的账号
// 即使被 OpenClaw 等非 CC 客户端使用，对外报告的 x-stainless-os 仍然是 MacOS，与
// Anthropic OAuth 流程记录的设备特征一致。
//
// 对 claude-cli 客户端，行为与 GetOrCreateFingerprint 完全一致（registration fp 被忽略，
// 因为 CC 客户端有自己的真实 stainless headers）。
func (s *IdentityService) GetOrCreateFingerprintForAccount(ctx context.Context, account *Account, headers http.Header) (*Fingerprint, error) {
	if account == nil {
		return s.getOrCreateFingerprintInternal(ctx, 0, nil, headers)
	}
	return s.getOrCreateFingerprintInternal(ctx, account.ID, account, headers)
}

// getOrCreateFingerprintInternal 是 GetOrCreateFingerprint 和 GetOrCreateFingerprintForAccount
// 的公共实现。account 可以为 nil（旧 API 调用方）；非 nil 时启用 registration_fingerprint。
func (s *IdentityService) getOrCreateFingerprintInternal(ctx context.Context, accountID int64, account *Account, headers http.Header) (*Fingerprint, error) {
	clientUA := headers.Get("User-Agent")

	// 非 claude-cli 客户端: 每次返回 defaultFingerprint 副本 (带随机 ClientID),
	// 不读不写共享缓存,防止跨客户端污染。
	// P1-1: 优先使用 account.Extra 中的 registration_fingerprint 覆盖 OS/Arch。
	// P1-2: 使用 PickCLIVersionForAccount 做 per-account 版本扰动（75/20/5 latest/N-1/N-2）。
	if !claudeCliUserAgentRe.MatchString(clientUA) {
		fp := defaultFingerprint
		if account != nil {
			// P1-2: 用账号 ID 稳定地从 cli_recent_versions 中选一个版本号，
			// 避免所有 OAuth 账号在同一时刻被打上完全一致的 UA 指纹。
			if v := PickCLIVersionForAccount(account.ID); v != "" {
				fp.UserAgent = claude.BuildUserAgentForVersion(v)
			}
			if regFp := GetRegistrationFingerprint(account); regFp != nil {
				if regFp.OS != "" {
					fp.StainlessOS = regFp.OS
				}
				if regFp.Arch != "" {
					fp.StainlessArch = regFp.Arch
				}
				// runtime/runtime_version 故意保持 default (node/v22.11.0)，因为我们要
				// mimic Claude CLI；浏览器真实 runtime 反而会暴露代理身份。
			}
		} else {
			// 无 account 时（旧 API 兼容），使用全局当前版本。
			fp.UserAgent = claude.BuildUserAgentForVersion(claude.GetCLICurrentVersion())
		}
		fp.ClientID = generateClientID()
		fp.UpdatedAt = time.Now().Unix()
		return &fp, nil
	}

	// 尝试从缓存获取指纹
	cached, err := s.cache.GetFingerprint(ctx, accountID)
	if err == nil && cached != nil {
		// 缓存 UA 是否也是真 CC?不是说明这份缓存是被污染的历史残留 (v0.1.127 之前
		// 的旧数据),直接丢弃并重建。
		if !claudeCliUserAgentRe.MatchString(cached.UserAgent) {
			logger.LegacyPrintf("service.identity",
				"Discarding polluted fingerprint for account %d (cached UA=%q, not claude-cli)",
				accountID, cached.UserAgent)
		} else {
			needWrite := false
			if isNewerVersion(clientUA, cached.UserAgent) {
				// 版本升级：merge 语义 — 仅更新请求中实际携带的字段，保留缓存值
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
	}

	// 缓存不存在、解析失败或已丢弃污染数据，从当前请求头创建新指纹
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

// RewriteUserID 重写body中的metadata.user_id
// 支持旧拼接格式和新 JSON 格式的 user_id 解析，
// 根据 fingerprintUA 版本选择输出格式。
//
// 重要：此函数使用 json.RawMessage 保留其他字段的原始字节，
// 避免重新序列化导致 thinking 块等内容被修改。
func (s *IdentityService) RewriteUserID(body []byte, accountID int64, accountUUID, cachedClientID, fingerprintUA string) ([]byte, error) {
	if len(body) == 0 || accountUUID == "" || cachedClientID == "" {
		return body, nil
	}

	metadata := gjson.GetBytes(body, "metadata")
	if !metadata.Exists() || metadata.Type == gjson.Null {
		return body, nil
	}
	if !strings.HasPrefix(strings.TrimSpace(metadata.Raw), "{") {
		return body, nil
	}

	userIDResult := metadata.Get("user_id")
	if !userIDResult.Exists() || userIDResult.Type != gjson.String {
		return body, nil
	}
	userID := userIDResult.String()
	if userID == "" {
		return body, nil
	}

	// 解析 user_id（兼容旧拼接格式和新 JSON 格式）
	parsed := ParseMetadataUserID(userID)
	if parsed == nil {
		return body, nil
	}

	sessionTail := parsed.SessionID // 原始session UUID

	// 生成新的session hash: SHA256(accountID::sessionTail) -> UUID格式
	seed := fmt.Sprintf("%d::%s", accountID, sessionTail)
	newSessionHash := generateUUIDFromSeed(seed)

	// 根据客户端版本选择输出格式
	version := ExtractCLIVersion(fingerprintUA)
	newUserID := FormatMetadataUserID(cachedClientID, accountUUID, newSessionHash, version)
	if newUserID == userID {
		return body, nil
	}

	newBody, err := sjson.SetBytes(body, "metadata.user_id", newUserID)
	if err != nil {
		return body, nil
	}
	return newBody, nil
}

// RewriteUserIDWithMasking 重写body中的metadata.user_id，支持会话ID伪装
// 如果账号启用了会话ID伪装（默认启用），
// 则在完成常规重写后，将 session 部分替换为随机 UUID（30-285 分钟随机 TTL 后自动过期）
//
// 重要：此函数使用 json.RawMessage 保留其他字段的原始字节，
// 避免重新序列化导致 thinking 块等内容被修改。
func (s *IdentityService) RewriteUserIDWithMasking(ctx context.Context, body []byte, account *Account, accountUUID, cachedClientID, fingerprintUA string) ([]byte, error) {
	// 先执行常规的 RewriteUserID 逻辑
	newBody, err := s.RewriteUserID(body, account.ID, accountUUID, cachedClientID, fingerprintUA)
	if err != nil {
		return newBody, err
	}

	// 检查是否启用会话ID伪装
	if !account.IsSessionIDMaskingEnabled() {
		return newBody, nil
	}

	metadata := gjson.GetBytes(newBody, "metadata")
	if !metadata.Exists() || metadata.Type == gjson.Null {
		return newBody, nil
	}
	if !strings.HasPrefix(strings.TrimSpace(metadata.Raw), "{") {
		return newBody, nil
	}

	userIDResult := metadata.Get("user_id")
	if !userIDResult.Exists() || userIDResult.Type != gjson.String {
		return newBody, nil
	}
	userID := userIDResult.String()
	if userID == "" {
		return newBody, nil
	}

	// 解析已重写的 user_id
	uidParsed := ParseMetadataUserID(userID)
	if uidParsed == nil {
		return newBody, nil
	}

	// 获取或生成固定的伪装 session ID
	maskedSessionID, err := s.cache.GetMaskedSessionID(ctx, account.ID)
	if err != nil {
		logger.LegacyPrintf("service.identity", "Warning: failed to get masked session ID for account %d: %v", account.ID, err)
		return newBody, nil
	}

	if maskedSessionID == "" {
		// 首次或已过期，生成新的伪装 session ID
		maskedSessionID = generateRandomUUID()

		// TTL 选型理由（2026-04 调整）：
		// 旧值是 30～285 分钟（< 5h），平均 ~2.6h。这个分布会让上游看到的
		// metadata.user_id session 段平均每 2-3 小时就翻新一次 → 像"用户在反复
		// 启停 CLI"，本身就是合流账号的特征之一（真实重度用户单个 IDE session
		// 一般持续半个工作日：4-10 小时）。
		//
		// 改为 240～600 分钟（4-10h），平均 ~7h，匹配真实 IDE workday 形态。
		// 上限 10h 避免 24/7 不间断的僵尸会话特征；下限 4h 保证短工作日也合理。
		// 256 个桶（uint8）映射到 360 分钟跨度 ≈ 每桶 84 秒，足够分散。
		b := make([]byte, 1)
		_, _ = rand.Read(b)
		const minMinutes = 240
		const spanMinutes = 360 // [240, 600] minutes = 4–10 h
		ttl := time.Duration(minMinutes+int(b[0])*spanMinutes/256) * time.Minute

		if err := s.cache.SetMaskedSessionID(ctx, account.ID, maskedSessionID, ttl); err != nil {
			logger.LegacyPrintf("service.identity", "Warning: failed to set masked session ID for account %d: %v", account.ID, err)
		}
		logger.LegacyPrintf("service.identity", "Generated new masked session ID for account %d: %s, TTL: %v", account.ID, maskedSessionID, ttl)
	}
	// 注意：此处不再在每次请求时无脑刷新 TTL，让会话根据生成的 TTL 自然消亡并重新开始。

	// 用 FormatMetadataUserID 重建（保持与 RewriteUserID 相同的格式）
	version := ExtractCLIVersion(fingerprintUA)
	newUserID := FormatMetadataUserID(uidParsed.DeviceID, uidParsed.AccountUUID, maskedSessionID, version)

	slog.Debug("session_id_masking_applied",
		"account_id", account.ID,
		"before", userID,
		"after", newUserID,
	)

	if newUserID == userID {
		return newBody, nil
	}

	maskedBody, setErr := sjson.SetBytes(newBody, "metadata.user_id", newUserID)
	if setErr != nil {
		return newBody, nil
	}
	return maskedBody, nil
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

// generateUUIDFromSeed 从种子生成确定性UUID v4格式字符串
func generateUUIDFromSeed(seed string) string {
	hash := sha256.Sum256([]byte(seed))
	bytes := hash[:16]

	// 设置UUID v4版本和变体位
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
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
