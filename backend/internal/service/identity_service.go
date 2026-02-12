package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// 预编译正则表达式（避免每次调用重新编译）
var (
	// 匹配 user_id 格式: user_{64位hexREDACTED_account__session_{uuidREDACTED
	userIDRegex = regexp.MustCompile(`^user_[a-f0-9]{64REDACTED_account__session_([a-f0-9-]{36REDACTED)$`)
	// 匹配 User-Agent 版本号: xxx/x.y.z
	userAgentVersionRegex = regexp.MustCompile(`/(\d+)\.(\d+)\.(\d+)`)
)

// 默认指纹值（当客户端未提供时使用）
var defaultFingerprint = Fingerprint{
	UserAgent:               "claude-cli/2.1.22 (external, cli)",
	StainlessLang:           "js",
	StainlessPackageVersion: "0.70.0",
	StainlessOS:             "Linux",
	StainlessArch:           "arm64",
	StainlessRuntime:        "node",
	StainlessRuntimeVersion: "v24.13.0",
REDACTED

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
REDACTED

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
REDACTED

// IdentityService 管理OAuth账号的请求身份指纹
type IdentityService struct {
	cache IdentityCache
REDACTED

// NewIdentityService 创建新的IdentityService
func NewIdentityService(cache IdentityCache) *IdentityService {
	return &IdentityService{cache: cacheREDACTED
REDACTED

// GetOrCreateFingerprint 获取或创建账号的指纹
// 如果缓存存在，检测user-agent版本，新版本则更新
// 如果缓存不存在，生成随机ClientID并从请求头创建指纹，然后缓存
func (s *IdentityService) GetOrCreateFingerprint(ctx context.Context, accountID int64, headers http.Header) (*Fingerprint, error) {
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
			logger.LegacyPrintf("service.identity", "Updated fingerprint user-agent for account %d: %s", accountID, clientUA)
	REDACTED
		return cached, nil
REDACTED

	// 缓存不存在或解析失败，创建新指纹
	fp := s.createFingerprintFromHeaders(headers)

	// 生成随机ClientID
	fp.ClientID = generateClientID()

	// 保存到缓存（永不过期）
	if err := s.cache.SetFingerprint(ctx, accountID, fp); err != nil {
		logger.LegacyPrintf("service.identity", "Warning: failed to cache fingerprint for account %d: %v", accountID, err)
REDACTED

	logger.LegacyPrintf("service.identity", "Created new fingerprint for account %d with client_id: %s", accountID, fp.ClientID)
	return fp, nil
REDACTED

// createFingerprintFromHeaders 从请求头创建指纹
func (s *IdentityService) createFingerprintFromHeaders(headers http.Header) *Fingerprint {
	fp := &Fingerprint{REDACTED

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
func (s *IdentityService) ApplyFingerprint(req *http.Request, fp *Fingerprint) {
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
//
// 重要：此函数使用 json.RawMessage 保留其他字段的原始字节，
// 避免重新序列化导致 thinking 块等内容被修改。
func (s *IdentityService) RewriteUserID(body []byte, accountID int64, accountUUID, cachedClientID string) ([]byte, error) {
	if len(body) == 0 || accountUUID == "" || cachedClientID == "" {
		return body, nil
REDACTED

	// 使用 RawMessage 保留其他字段的原始字节
	var reqMap map[string]json.RawMessage
	if err := json.Unmarshal(body, &reqMap); err != nil {
		return body, nil
REDACTED

	// 解析 metadata 字段
	metadataRaw, ok := reqMap["metadata"]
	if !ok {
		return body, nil
REDACTED

	var metadata map[string]any
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
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

	// 只重新序列化 metadata 字段
	newMetadataRaw, err := json.Marshal(metadata)
	if err != nil {
		return body, nil
REDACTED
	reqMap["metadata"] = newMetadataRaw

	return json.Marshal(reqMap)
REDACTED

// RewriteUserIDWithMasking 重写body中的metadata.user_id，支持会话ID伪装
// 如果账号启用了会话ID伪装（session_id_masking_enabled），
// 则在完成常规重写后，将 session 部分替换为固定的伪装ID（15分钟内保持不变）
//
// 重要：此函数使用 json.RawMessage 保留其他字段的原始字节，
// 避免重新序列化导致 thinking 块等内容被修改。
func (s *IdentityService) RewriteUserIDWithMasking(ctx context.Context, body []byte, account *Account, accountUUID, cachedClientID string) ([]byte, error) {
	// 先执行常规的 RewriteUserID 逻辑
	newBody, err := s.RewriteUserID(body, account.ID, accountUUID, cachedClientID)
	if err != nil {
		return newBody, err
REDACTED

	// 检查是否启用会话ID伪装
	if !account.IsSessionIDMaskingEnabled() {
		return newBody, nil
REDACTED

	// 使用 RawMessage 保留其他字段的原始字节
	var reqMap map[string]json.RawMessage
	if err := json.Unmarshal(newBody, &reqMap); err != nil {
		return newBody, nil
REDACTED

	// 解析 metadata 字段
	metadataRaw, ok := reqMap["metadata"]
	if !ok {
		return newBody, nil
REDACTED

	var metadata map[string]any
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		return newBody, nil
REDACTED

	userID, ok := metadata["user_id"].(string)
	if !ok || userID == "" {
		return newBody, nil
REDACTED

	// 查找 _session_ 的位置，替换其后的内容
	const sessionMarker = "_session_"
	idx := strings.LastIndex(userID, sessionMarker)
	if idx == -1 {
		return newBody, nil
REDACTED

	// 获取或生成固定的伪装 session ID
	maskedSessionID, err := s.cache.GetMaskedSessionID(ctx, account.ID)
	if err != nil {
		logger.LegacyPrintf("service.identity", "Warning: failed to get masked session ID for account %d: %v", account.ID, err)
		return newBody, nil
REDACTED

	if maskedSessionID == "" {
		// 首次或已过期，生成新的伪装 session ID
		maskedSessionID = generateRandomUUID()
		logger.LegacyPrintf("service.identity", "Generated new masked session ID for account %d: %s", account.ID, maskedSessionID)
REDACTED

	// 刷新 TTL（每次请求都刷新，保持 15 分钟有效期）
	if err := s.cache.SetMaskedSessionID(ctx, account.ID, maskedSessionID); err != nil {
		logger.LegacyPrintf("service.identity", "Warning: failed to set masked session ID for account %d: %v", account.ID, err)
REDACTED

	// 替换 session 部分：保留 _session_ 之前的内容，替换之后的内容
	newUserID := userID[:idx+len(sessionMarker)] + maskedSessionID

	slog.Debug("session_id_masking_applied",
		"account_id", account.ID,
		"before", userID,
		"after", newUserID,
	)

	metadata["user_id"] = newUserID

	// 只重新序列化 metadata 字段
	newMetadataRaw, marshalErr := json.Marshal(metadata)
	if marshalErr != nil {
		return newBody, nil
REDACTED
	reqMap["metadata"] = newMetadataRaw

	return json.Marshal(reqMap)
REDACTED

// generateRandomUUID 生成随机 UUID v4 格式字符串
func generateRandomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// fallback: 使用时间戳生成
		h := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		b = h[:16]
REDACTED

	// 设置 UUID v4 版本和变体位
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
REDACTED

// generateClientID 生成64位十六进制客户端ID（32字节随机数）
func generateClientID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 极罕见的情况，使用时间戳+固定值作为fallback
		logger.LegacyPrintf("service.identity", "Warning: crypto/rand.Read failed: %v, using fallback", err)
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
// 例如：claude-cli/2.1.2 -> (2, 1, 2)
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
