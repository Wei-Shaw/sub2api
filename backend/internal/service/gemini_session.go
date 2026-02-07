package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/cespare/xxhash/v2"
)

// Gemini 会话 ID Fallback 相关常量
const (
	// geminiSessionTTLSeconds Gemini 会话缓存 TTL（5 分钟）
	geminiSessionTTLSeconds = 300

	// geminiSessionKeyPrefix Gemini 会话 Redis key 前缀
	geminiSessionKeyPrefix = "gemini:sess:"
)

// GeminiSessionTTL 返回 Gemini 会话缓存 TTL
func GeminiSessionTTL() time.Duration {
	return geminiSessionTTLSeconds * time.Second
REDACTED

// shortHash 使用 XXHash64 + Base36 生成短 hash（16 字符）
// XXHash64 比 SHA256 快约 10 倍，Base36 比 Hex 短约 20%
func shortHash(data []byte) string {
	h := xxhash.Sum64(data)
	return strconv.FormatUint(h, 36)
REDACTED

// BuildGeminiDigestChain 根据 Gemini 请求生成摘要链
// 格式: s:<hash>-u:<hash>-m:<hash>-u:<hash>-...
// s = systemInstruction, u = user, m = model
func BuildGeminiDigestChain(req *antigravity.GeminiRequest) string {
	if req == nil {
		return ""
REDACTED

	var parts []string

	// 1. system instruction
	if req.SystemInstruction != nil && len(req.SystemInstruction.Parts) > 0 {
		partsData, _ := json.Marshal(req.SystemInstruction.Parts)
		parts = append(parts, "s:"+shortHash(partsData))
REDACTED

	// 2. contents
	for _, c := range req.Contents {
		prefix := "u" // user
		if c.Role == "model" {
			prefix = "m"
	REDACTED
		partsData, _ := json.Marshal(c.Parts)
		parts = append(parts, prefix+":"+shortHash(partsData))
REDACTED

	return strings.Join(parts, "-")
REDACTED

// GenerateGeminiPrefixHash 生成前缀 hash（用于分区隔离）
// 组合: userID + apiKeyID + ip + userAgent + platform + model
// 返回 16 字符的 Base64 编码的 SHA256 前缀
func GenerateGeminiPrefixHash(userID, apiKeyID int64, ip, userAgent, platform, model string) string {
	// 组合所有标识符
	combined := strconv.FormatInt(userID, 10) + ":" +
		strconv.FormatInt(apiKeyID, 10) + ":" +
		ip + ":" +
		userAgent + ":" +
		platform + ":" +
		model

	hash := sha256.Sum256([]byte(combined))
	// 取前 12 字节，Base64 编码后正好 16 字符
	return base64.RawURLEncoding.EncodeToString(hash[:12])
REDACTED

// BuildGeminiSessionKey 构建 Gemini 会话 Redis key
// 格式: gemini:sess:{groupIDREDACTED:{prefixHashREDACTED:{digestChainREDACTED
func BuildGeminiSessionKey(groupID int64, prefixHash, digestChain string) string {
	return geminiSessionKeyPrefix + strconv.FormatInt(groupID, 10) + ":" + prefixHash + ":" + digestChain
REDACTED

// GenerateDigestChainPrefixes 生成摘要链的所有前缀（从长到短）
// 用于 MGET 批量查询最长匹配
func GenerateDigestChainPrefixes(chain string) []string {
	if chain == "" {
		return nil
REDACTED

	var prefixes []string
	c := chain

	for c != "" {
		prefixes = append(prefixes, c)
		// 找到最后一个 "-" 的位置
		if i := strings.LastIndex(c, "-"); i > 0 {
			c = c[:i]
	REDACTED else {
			break
	REDACTED
REDACTED

	return prefixes
REDACTED

// ParseGeminiSessionValue 解析 Gemini 会话缓存值
// 格式: {uuidREDACTED:{accountIDREDACTED
func ParseGeminiSessionValue(value string) (uuid string, accountID int64, ok bool) {
	if value == "" {
		return "", 0, false
REDACTED

	// 找到最后一个 ":" 的位置（因为 uuid 可能包含 ":"）
	i := strings.LastIndex(value, ":")
	if i <= 0 || i >= len(value)-1 {
		return "", 0, false
REDACTED

	uuid = value[:i]
	accountID, err := strconv.ParseInt(value[i+1:], 10, 64)
	if err != nil {
		return "", 0, false
REDACTED

	return uuid, accountID, true
REDACTED

// FormatGeminiSessionValue 格式化 Gemini 会话缓存值
// 格式: {uuidREDACTED:{accountIDREDACTED
func FormatGeminiSessionValue(uuid string, accountID int64) string {
	return uuid + ":" + strconv.FormatInt(accountID, 10)
REDACTED

// geminiDigestSessionKeyPrefix Gemini 摘要 fallback 会话 key 前缀
const geminiDigestSessionKeyPrefix = "gemini:digest:"

// geminiTrieKeyPrefix Gemini Trie 会话 key 前缀
const geminiTrieKeyPrefix = "gemini:trie:"

// BuildGeminiTrieKey 构建 Gemini Trie Redis key
// 格式: gemini:trie:{groupIDREDACTED:{prefixHashREDACTED
func BuildGeminiTrieKey(groupID int64, prefixHash string) string {
	return geminiTrieKeyPrefix + strconv.FormatInt(groupID, 10) + ":" + prefixHash
REDACTED

// GenerateGeminiDigestSessionKey 生成 Gemini 摘要 fallback 的 sessionKey
// 组合 prefixHash 前 8 位 + uuid 前 8 位，确保不同会话产生不同的 sessionKey
// 用于在 SelectAccountWithLoadAwareness 中保持粘性会话
func GenerateGeminiDigestSessionKey(prefixHash, uuid string) string {
	prefix := prefixHash
	if len(prefixHash) >= 8 {
		prefix = prefixHash[:8]
REDACTED
	uuidPart := uuid
	if len(uuid) >= 8 {
		uuidPart = uuid[:8]
REDACTED
	return geminiDigestSessionKeyPrefix + prefix + ":" + uuidPart
REDACTED
