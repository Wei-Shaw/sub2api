package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// 预编译正则表达式（避免每次调用重新编译）
var (
	// 匹配 user_id 格式: user_{64位hexREDACTED_account__session_{uuidREDACTED
	userIDRegex = regexp.MustCompile(`^user_[a-f0-9]{64REDACTED_account__session_([a-f0-9-]{36REDACTED)$`)
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
REDACTED

// IdentityService 管理OAuth账号的请求身份指纹
type IdentityService struct {
	cache ports.IdentityCache
REDACTED

// NewIdentityService 创建新的IdentityService
func NewIdentityService(cache ports.IdentityCache) *IdentityService {
	return &IdentityService{cache: cacheREDACTED
REDACTED

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
	REDACTED
		return cached, nil
REDACTED

	// 缓存不存在或解析失败，创建新指纹
	fp := s.createFingerprintFromHeaders(headers)

	// 生成随机ClientID
	fp.ClientID = generateClientID()

	// 保存到缓存（永不过期）
	if err := s.cache.SetFingerprint(ctx, accountID, fp); err != nil {
		log.Printf("Warning: failed to cache fingerprint for account %d: %v", accountID, err)
REDACTED

	log.Printf("Created new fingerprint for account %d with client_id: %s", accountID, fp.ClientID)
	return fp, nil
REDACTED

// createFingerprintFromHeaders 从请求头创建指纹
func (s *IdentityService) createFingerprintFromHeaders(headers http.Header) *ports.Fingerprint {
	fp := &ports.Fingerprint{REDACTED

	// 获取User-Agent
	if ua := headers.Get("User-Agent"); ua != "" {
		fp.UserAgent = ua
REDACTED else {
		fp.UserAgent = defaultFingerprint.UserAgent
REDACTED

	// 获取x-stainless-*头，如果没有则使用默认值
	fp.StainlessLang = getHeaderOrDefault(headers, "X-Stainless-Lang", defaultFingerprint.StainlessLang)
	fp.StainlessPackageVersion = getHeaderOrDefault(headers, "X-Stainless-Package-Version", defaultFingerprint.StainlessPackageVersion)
	fp.StainlessOS = getHeaderOrDefault(headers, "X-Stainless-OS", defaultFingerprint.StainlessOS)
	fp.StainlessArch = getHeaderOrDefault(headers, "X-Stainless-Arch", defaultFingerprint.StainlessArch)
	fp.StainlessRuntime = getHeaderOrDefault(headers, "X-Stainless-Runtime", defaultFingerprint.StainlessRuntime)
	fp.StainlessRuntimeVersion = getHeaderOrDefault(headers, "X-Stainless-Runtime-Version", defaultFingerprint.StainlessRuntimeVersion)

	return fp
REDACTED

// getHeaderOrDefault 获取header值，如果不存在则返回默认值
func getHeaderOrDefault(headers http.Header, key, defaultValue string) string {
	if v := headers.Get(key); v != "" {
		return v
REDACTED
	return defaultValue
REDACTED

// ApplyFingerprint 将指纹应用到请求头（覆盖原有的x-stainless-*头）
func (s *IdentityService) ApplyFingerprint(req *http.Request, fp *ports.Fingerprint) {
	if fp == nil {
		return
REDACTED

	// 设置user-agent
	if fp.UserAgent != "" {
		req.Header.Set("user-agent", fp.UserAgent)
REDACTED

	// 设置x-stainless-*头
	if fp.StainlessLang != "" {
		req.Header.Set("X-Stainless-Lang", fp.StainlessLang)
REDACTED
	if fp.StainlessPackageVersion != "" {
		req.Header.Set("X-Stainless-Package-Version", fp.StainlessPackageVersion)
REDACTED
	if fp.StainlessOS != "" {
		req.Header.Set("X-Stainless-OS", fp.StainlessOS)
REDACTED
	if fp.StainlessArch != "" {
		req.Header.Set("X-Stainless-Arch", fp.StainlessArch)
REDACTED
	if fp.StainlessRuntime != "" {
		req.Header.Set("X-Stainless-Runtime", fp.StainlessRuntime)
REDACTED
	if fp.StainlessRuntimeVersion != "" {
		req.Header.Set("X-Stainless-Runtime-Version", fp.StainlessRuntimeVersion)
REDACTED
REDACTED

// RewriteUserID 重写body中的metadata.user_id
// 输入格式：user_{clientIdREDACTED_account__session_{sessionUUIDREDACTED
// 输出格式：user_{cachedClientIDREDACTED_account_{accountUUIDREDACTED_session_{newHashREDACTED
func (s *IdentityService) RewriteUserID(body []byte, accountID int64, accountUUID, cachedClientID string) ([]byte, error) {
	if len(body) == 0 || accountUUID == "" || cachedClientID == "" {
		return body, nil
REDACTED

	// 解析JSON
	var reqMap map[string]any
	if err := json.Unmarshal(body, &reqMap); err != nil {
		return body, nil
REDACTED

	metadata, ok := reqMap["metadata"].(map[string]any)
	if !ok {
		return body, nil
REDACTED

	userID, ok := metadata["user_id"].(string)
	if !ok || userID == "" {
		return body, nil
REDACTED

	// 匹配格式: user_{64位hexREDACTED_account__session_{uuidREDACTED
	matches := userIDRegex.FindStringSubmatch(userID)
	if matches == nil {
		return body, nil
REDACTED

	sessionTail := matches[1] // 原始session UUID

	// 生成新的session hash: SHA256(accountID::sessionTail) -> UUID格式
	seed := fmt.Sprintf("%d::%s", accountID, sessionTail)
	newSessionHash := generateUUIDFromSeed(seed)

	// 构建新的user_id
	// 格式: user_{cachedClientIDREDACTED_account_{account_uuidREDACTED_session_{newSessionHashREDACTED
	newUserID := fmt.Sprintf("user_%s_account_%s_session_%s", cachedClientID, accountUUID, newSessionHash)

	metadata["user_id"] = newUserID
	reqMap["metadata"] = metadata

	return json.Marshal(reqMap)
REDACTED

// generateClientID 生成64位十六进制客户端ID（32字节随机数）
func generateClientID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 极罕见的情况，使用时间戳+固定值作为fallback
		log.Printf("Warning: crypto/rand.Read failed: %v, using fallback", err)
		// 使用SHA256(当前纳秒时间)作为fallback
		h := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		return hex.EncodeToString(h[:])
REDACTED
	return hex.EncodeToString(b)
REDACTED

// generateUUIDFromSeed 从种子生成确定性UUID v4格式字符串
func generateUUIDFromSeed(seed string) string {
	hash := sha256.Sum256([]byte(seed))
	bytes := hash[:16]

	// 设置UUID v4版本和变体位
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
REDACTED

// parseUserAgentVersion 解析user-agent版本号
// 例如：claude-cli/2.0.62 -> (2, 0, 62)
func parseUserAgentVersion(ua string) (major, minor, patch int, ok bool) {
	// 匹配 xxx/x.y.z 格式
	matches := userAgentVersionRegex.FindStringSubmatch(ua)
	if len(matches) != 4 {
		return 0, 0, 0, false
REDACTED
	major, _ = strconv.Atoi(matches[1])
	minor, _ = strconv.Atoi(matches[2])
	patch, _ = strconv.Atoi(matches[3])
	return major, minor, patch, true
REDACTED

// isNewerVersion 比较版本号，判断newUA是否比cachedUA更新
func isNewerVersion(newUA, cachedUA string) bool {
	newMajor, newMinor, newPatch, newOk := parseUserAgentVersion(newUA)
	cachedMajor, cachedMinor, cachedPatch, cachedOk := parseUserAgentVersion(cachedUA)

	if !newOk || !cachedOk {
		return false
REDACTED

	// 比较版本号
	if newMajor > cachedMajor {
		return true
REDACTED
	if newMajor < cachedMajor {
		return false
REDACTED

	if newMinor > cachedMinor {
		return true
REDACTED
	if newMinor < cachedMinor {
		return false
REDACTED

	return newPatch > cachedPatch
REDACTED
