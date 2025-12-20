package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"sub2api/internal/service/ports"
	"time"
)

// 预编译正则表达式（避免每次调用重新编译）
var (
	// 匹配 user_id 格式: user_{64位hex}_account__session_{uuid}
	userIDRegex = regexp.MustCompile(`^user_[a-f0-9]{64}_account__session_([a-f0-9-]{36})$`)
	// 匹配 User-Agent 版本号: xxx/x.y.z
	userAgentVersionRegex = regexp.MustCompile(`/(\d+)\.(\d+)\.(\d+)`)
)

// 默认指纹值（当客户端未提供时使用）
var defaultFingerprint = ports.Fingerprint{
	UserAgent:               "claude-cli/2.0.62 (external, cli)",
	StainlessLang:           "js",
	StainlessPackageVersion: "0.52.0",
	StainlessOS:             "Linux",
	StainlessArch:           "x64",
	StainlessRuntime:        "node",
	StainlessRuntimeVersion: "v22.14.0",
}

// IdentityService 管理OAuth账号的请求身份指纹
type IdentityService struct {
	cache ports.IdentityCache
}

// NewIdentityService 创建新的IdentityService
func NewIdentityService(cache ports.IdentityCache) *IdentityService {
	return &IdentityService{cache: cache}
}

// GetOrCreateFingerprint 获取或创建账号的指纹
// 如果缓存存在，检测user-agent版本，新版本则更新
// 如果缓存不存在，生成随机ClientID并从请求头创建指纹，然后缓存
func (s *IdentityService) GetOrCreateFingerprint(ctx context.Context, accountID int64, headers http.Header) (*ports.Fingerprint, error) {
	// 尝试从缓存获取指纹
	cached, err := s.cache.GetFingerprint(ctx, accountID)
	if err == nil && cached != nil {
		// 检查客户端的user-agent是否是更新版本
		clientUA := headers.Get("User-Agent")
		if clientUA != "" && isNewerVersion(clientUA, cached.UserAgent) {
			// 更新user-agent
			cached.UserAgent = clientUA
			// 保存更新后的指纹
			_ = s.cache.SetFingerprint(ctx, accountID, cached)
			log.Printf("Updated fingerprint user-agent for account %d: %s", accountID, clientUA)
		}
		return cached, nil
	}

	// 缓存不存在或解析失败，创建新指纹
	fp := s.createFingerprintFromHeaders(headers)

	// 生成随机ClientID
	fp.ClientID = generateClientID()

	// 保存到缓存（永不过期）
	if err := s.cache.SetFingerprint(ctx, accountID, fp); err != nil {
		log.Printf("Warning: failed to cache fingerprint for account %d: %v", accountID, err)
	}

	log.Printf("Created new fingerprint for account %d with client_id: %s", accountID, fp.ClientID)
	return fp, nil
}

// createFingerprintFromHeaders 从请求头创建指纹
func (s *IdentityService) createFingerprintFromHeaders(headers http.Header) *ports.Fingerprint {
	fp := &ports.Fingerprint{}

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

// getHeaderOrDefault 获取header值，如果不存在则返回默认值
func getHeaderOrDefault(headers http.Header, key, defaultValue string) string {
	if v := headers.Get(key); v != "" {
		return v
	}
	return defaultValue
}

// ApplyFingerprint 将指纹应用到请求头（覆盖原有的x-stainless-*头）
func (s *IdentityService) ApplyFingerprint(req *http.Request, fp *ports.Fingerprint) {
	if fp == nil {
		return
	}

	// 设置User-Agent
	if fp.UserAgent != "" {
		req.Header.Set("User-Agent", fp.UserAgent)
	}

	// 设置x-stainless-*头（使用正确的大小写）
	if fp.StainlessLang != "" {
		req.Header.Set("X-Stainless-Lang", fp.StainlessLang)
	}
	if fp.StainlessPackageVersion != "" {
		req.Header.Set("X-Stainless-Package-Version", fp.StainlessPackageVersion)
	}
	if fp.StainlessOS != "" {
		req.Header.Set("X-Stainless-OS", fp.StainlessOS)
	}
	if fp.StainlessArch != "" {
		req.Header.Set("X-Stainless-Arch", fp.StainlessArch)
	}
	if fp.StainlessRuntime != "" {
		req.Header.Set("X-Stainless-Runtime", fp.StainlessRuntime)
	}
	if fp.StainlessRuntimeVersion != "" {
		req.Header.Set("X-Stainless-Runtime-Version", fp.StainlessRuntimeVersion)
	}
}

// RewriteUserID 重写body中的metadata.user_id
// 输入格式：user_{clientId}_account__session_{sessionUUID}
// 输出格式：user_{cachedClientID}_account_{accountUUID}_session_{newHash}
func (s *IdentityService) RewriteUserID(body []byte, accountID int64, accountUUID, cachedClientID string) ([]byte, error) {
	if len(body) == 0 || accountUUID == "" || cachedClientID == "" {
		return body, nil
	}

	// 解析JSON
	var reqMap map[string]interface{}
	if err := json.Unmarshal(body, &reqMap); err != nil {
		return body, nil
	}

	metadata, ok := reqMap["metadata"].(map[string]interface{})
	if !ok {
		return body, nil
	}

	userID, ok := metadata["user_id"].(string)
	if !ok || userID == "" {
		return body, nil
	}

	// 匹配格式: user_{64位hex}_account__session_{uuid}
	matches := userIDRegex.FindStringSubmatch(userID)
	if matches == nil {
		return body, nil
	}

	sessionTail := matches[1] // 原始session UUID

	// 生成新的session hash: SHA256(accountID::sessionTail) -> UUID格式
	seed := fmt.Sprintf("%d::%s", accountID, sessionTail)
	newSessionHash := generateUUIDFromSeed(seed)

	// 构建新的user_id
	// 格式: user_{cachedClientID}_account_{account_uuid}_session_{newSessionHash}
	newUserID := fmt.Sprintf("user_%s_account_%s_session_%s", cachedClientID, accountUUID, newSessionHash)

	metadata["user_id"] = newUserID
	reqMap["metadata"] = metadata

	return json.Marshal(reqMap)
}

// generateClientID 生成64位十六进制客户端ID（32字节随机数）
func generateClientID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 极罕见的情况，使用时间戳+固定值作为fallback
		log.Printf("Warning: crypto/rand.Read failed: %v, using fallback", err)
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
// 例如：claude-cli/2.0.62 -> (2, 0, 62)
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

// isNewerVersion 比较版本号，判断newUA是否比cachedUA更新
func isNewerVersion(newUA, cachedUA string) bool {
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
