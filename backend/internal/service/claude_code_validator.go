package service

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// ClaudeCodeValidator 验证请求是否来自 Claude Code 客户端
// 完全学习自 claude-relay-service 项目的验证逻辑
type ClaudeCodeValidator struct{}

var (
	// User-Agent 匹配: claude-cli/x.x.x (仅支持官方 CLI，大小写不敏感)
	claudeCodeUAPattern = regexp.MustCompile(`(?i)^claude-cli/\d+\.\d+\.\d+`)

	// 带捕获组的版本提取正则
	claudeCodeUAVersionPattern = regexp.MustCompile(`(?i)^claude-cli/(\d+\.\d+\.\d+)`)

	// System prompt 相似度阈值（默认 0.5，和 claude-relay-service 一致）
	systemPromptThreshold = 0.5
)

// Claude Code 官方 System Prompt 模板
// 从 claude-relay-service/src/utils/contents.js 提取
var claudeCodeSystemPrompts = []string{
	// claudeOtherSystemPrompt1 - Primary
	"You are Claude Code, Anthropic's official CLI for Claude.",

	// claudeOtherSystemPrompt3 - Agent SDK
	"You are a Claude agent, built on Anthropic's Claude Agent SDK.",

	// claudeOtherSystemPrompt4 - Compact Agent SDK
	"You are Claude Code, Anthropic's official CLI for Claude, running within the Claude Agent SDK.",

	// exploreAgentSystemPrompt
	"You are a file search specialist for Claude Code, Anthropic's official CLI for Claude.",

	// claudeOtherSystemPromptCompact - Compact (用于对话摘要)
	"You are a helpful AI assistant tasked with summarizing conversations.",

	// claudeOtherSystemPrompt2 - Secondary (长提示词的关键部分)
	"You are an interactive CLI tool that helps users",
}

// NewClaudeCodeValidator 创建验证器实例
func NewClaudeCodeValidator() *ClaudeCodeValidator {
	return &ClaudeCodeValidator{}
}

// Validate 验证请求是否来自 Claude Code CLI
// 采用与 claude-relay-service 完全一致的验证策略：
//
//	Step 1: User-Agent 检查 (必需) - 必须是 claude-cli/x.x.x
//	Step 2: 对于非 messages 路径，只要 UA 匹配就通过
//	Step 3: 检查 max_tokens=1 + haiku 探测请求绕过（UA 已验证）
//	Step 4: 对于 messages 路径，进行严格验证：
//	        - System prompt 相似度检查
//	        - X-App header 检查
//	        - anthropic-beta header 检查
//	        - anthropic-version header 检查
//	        - metadata.user_id 格式验证
func (v *ClaudeCodeValidator) Validate(r *http.Request, body map[string]any) bool {
	// Step 1: User-Agent 检查
	ua := r.Header.Get("User-Agent")
	if !claudeCodeUAPattern.MatchString(ua) {
		return false
	}

	// Step 2: 非 messages 路径，只要 UA 匹配就通过
	path := r.URL.Path
	if !strings.Contains(path, "messages") {
		return true
	}

	// Step 3: 检查 max_tokens=1 + haiku 探测请求绕过
	// 这类请求用于 Claude Code 验证 API 连通性，不携带 system prompt
	if isMaxTokensOneHaiku, ok := IsMaxTokensOneHaikuRequestFromContext(r.Context()); ok && isMaxTokensOneHaiku {
		return true // 绕过 system prompt 检查，UA 已在 Step 1 验证
	}

	// Step 4: messages 路径，进行严格验证

	// 4.1 检查 system prompt 相似度
	if !v.hasClaudeCodeSystemPrompt(body) {
		return false
	}

	// 4.2 检查必需的 headers（值不为空即可）
	xApp := r.Header.Get("X-App")
	if xApp == "" {
		return false
	}

	anthropicBeta := r.Header.Get("anthropic-beta")
	if anthropicBeta == "" {
		return false
	}

	anthropicVersion := r.Header.Get("anthropic-version")
	if anthropicVersion == "" {
		return false
	}

	// 4.3 验证 metadata.user_id
	if body == nil {
		return false
	}

	metadata, ok := body["metadata"].(map[string]any)
	if !ok {
		return false
	}

	userID, ok := metadata["user_id"].(string)
	if !ok || userID == "" {
		return false
	}

	if ParseMetadataUserID(userID) == nil {
		return false
	}

	return true
}

// hasClaudeCodeSystemPrompt 检查请求是否包含 Claude Code 系统提示词
// 使用字符串相似度匹配（Dice coefficient）
func (v *ClaudeCodeValidator) hasClaudeCodeSystemPrompt(body map[string]any) bool {
	if body == nil {
		return false
	}

	// 检查 model 字段
	if _, ok := body["model"].(string); !ok {
		return false
	}

	// 获取 system 字段
	systemEntries, ok := body["system"].([]any)
	if !ok {
		return false
	}

	// 检查每个 system entry
	for _, entry := range systemEntries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}

		text, ok := entryMap["text"].(string)
		if !ok || text == "" {
			continue
		}

		// 计算与所有模板的最佳相似度
		bestScore := v.bestSimilarityScore(text)
		if bestScore >= systemPromptThreshold {
			return true
		}
	}

	return false
}

// bestSimilarityScore 计算文本与所有 Claude Code 模板的最佳相似度
func (v *ClaudeCodeValidator) bestSimilarityScore(text string) float64 {
	normalizedText := normalizePrompt(text)
	bestScore := 0.0

	for _, template := range claudeCodeSystemPrompts {
		normalizedTemplate := normalizePrompt(template)
		score := diceCoefficient(normalizedText, normalizedTemplate)
		if score > bestScore {
			bestScore = score
		}
	}

	return bestScore
}

// normalizePrompt 标准化提示词文本（去除多余空白）
func normalizePrompt(text string) string {
	// 将所有空白字符替换为单个空格，并去除首尾空白
	return strings.Join(strings.Fields(text), " ")
}

// diceCoefficient 计算两个字符串的 Dice 系数（Sørensen–Dice coefficient）
// 这是 string-similarity 库使用的算法
// 公式: 2 * |intersection| / (|bigrams(a)| + |bigrams(b)|)
func diceCoefficient(a, b string) float64 {
	if a == b {
		return 1.0
	}

	if len(a) < 2 || len(b) < 2 {
		return 0.0
	}

	// 生成 bigrams
	bigramsA := getBigrams(a)
	bigramsB := getBigrams(b)

	if len(bigramsA) == 0 || len(bigramsB) == 0 {
		return 0.0
	}

	// 计算交集大小
	intersection := 0
	for bigram, countA := range bigramsA {
		if countB, exists := bigramsB[bigram]; exists {
			if countA < countB {
				intersection += countA
			} else {
				intersection += countB
			}
		}
	}

	// 计算总 bigram 数量
	totalA := 0
	for _, count := range bigramsA {
		totalA += count
	}
	totalB := 0
	for _, count := range bigramsB {
		totalB += count
	}

	return float64(2*intersection) / float64(totalA+totalB)
}

// getBigrams 获取字符串的所有 bigrams（相邻字符对）
func getBigrams(s string) map[string]int {
	bigrams := make(map[string]int)
	runes := []rune(strings.ToLower(s))

	for i := 0; i < len(runes)-1; i++ {
		bigram := string(runes[i : i+2])
		bigrams[bigram]++
	}

	return bigrams
}

// ValidateUserAgent 仅验证 User-Agent（用于不需要解析请求体的场景）
func (v *ClaudeCodeValidator) ValidateUserAgent(ua string) bool {
	return claudeCodeUAPattern.MatchString(ua)
}

// IncludesClaudeCodeSystemPrompt 检查请求体是否包含 Claude Code 系统提示词
// 只要存在匹配的系统提示词就返回 true（用于宽松检测）
func (v *ClaudeCodeValidator) IncludesClaudeCodeSystemPrompt(body map[string]any) bool {
	return v.hasClaudeCodeSystemPrompt(body)
}

// IsClaudeCodeClient 从 context 中获取 Claude Code 客户端标识
func IsClaudeCodeClient(ctx context.Context) bool {
	if v, ok := ctx.Value(ctxkey.IsClaudeCodeClient).(bool); ok {
		return v
	}
	return false
}

// SetClaudeCodeClient 将 Claude Code 客户端标识设置到 context 中
func SetClaudeCodeClient(ctx context.Context, isClaudeCode bool) context.Context {
	return context.WithValue(ctx, ctxkey.IsClaudeCodeClient, isClaudeCode)
}

// ExtractVersion 从 User-Agent 中提取 Claude Code 版本号
// 返回 "2.1.22" 形式的版本号，如果不匹配返回空字符串
func (v *ClaudeCodeValidator) ExtractVersion(ua string) string {
	return ExtractCLIVersion(ua)
}

// SetClaudeCodeVersion 将 Claude Code 版本号设置到 context 中
func SetClaudeCodeVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, ctxkey.ClaudeCodeVersion, version)
}

// GetClaudeCodeVersion 从 context 中获取 Claude Code 版本号
func GetClaudeCodeVersion(ctx context.Context) string {
	if v, ok := ctx.Value(ctxkey.ClaudeCodeVersion).(string); ok {
		return v
	}
	return ""
}

// CompareVersions 比较两个 semver 版本号
// 返回: -1 (a < b), 0 (a == b), 1 (a > b)
func CompareVersions(a, b string) int {
	aParts := parseSemver(a)
	bParts := parseSemver(b)
	for i := 0; i < 3; i++ {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}
	return 0
}

// parseSemver 解析 semver 版本号为 [major, minor, patch]
func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	result := [3]int{0, 0, 0}
	for i := 0; i < len(parts) && i < 3; i++ {
		if parsed, err := strconv.Atoi(parts[i]); err == nil {
			result[i] = parsed
		}
	}
	return result
}
