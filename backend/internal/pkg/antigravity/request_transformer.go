package antigravity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	sessionRand      = rand.New(rand.NewSource(time.Now().UnixNano()))
	sessionRandMutex sync.Mutex
)

// generateStableSessionID 基于用户消息内容生成稳定的 session ID
func generateStableSessionID(contents []GeminiContent) string {
	// 查找第一个 user 消息的文本
	for _, content := range contents {
		if content.Role == "user" && len(content.Parts) > 0 {
			if text := content.Parts[0].Text; text != "" {
				h := sha256.Sum256([]byte(text))
				n := int64(binary.BigEndian.Uint64(h[:8])) & 0x7FFFFFFFFFFFFFFF
				return "-" + strconv.FormatInt(n, 10)
		REDACTED
	REDACTED
REDACTED
	// 回退：生成随机 session ID
	sessionRandMutex.Lock()
	n := sessionRand.Int63n(9_000_000_000_000_000_000)
	sessionRandMutex.Unlock()
	return "-" + strconv.FormatInt(n, 10)
REDACTED

type TransformOptions struct {
	EnableIdentityPatch bool
	// IdentityPatch 可选：自定义注入到 systemInstruction 开头的身份防护提示词；
	// 为空时使用默认模板（包含 [IDENTITY_PATCH] 及 SYSTEM_PROMPT_BEGIN 标记）。
	IdentityPatch string
REDACTED

func DefaultTransformOptions() TransformOptions {
	return TransformOptions{
		EnableIdentityPatch: true,
REDACTED
REDACTED

// webSearchFallbackModel web_search 请求使用的降级模型
const webSearchFallbackModel = "gemini-2.5-flash"

// TransformClaudeToGemini 将 Claude 请求转换为 v1internal Gemini 格式
func TransformClaudeToGemini(claudeReq *ClaudeRequest, projectID, mappedModel string) ([]byte, error) {
	return TransformClaudeToGeminiWithOptions(claudeReq, projectID, mappedModel, DefaultTransformOptions())
REDACTED

// TransformClaudeToGeminiWithOptions 将 Claude 请求转换为 v1internal Gemini 格式（可配置身份补丁等行为）
func TransformClaudeToGeminiWithOptions(claudeReq *ClaudeRequest, projectID, mappedModel string, opts TransformOptions) ([]byte, error) {
	// 用于存储 tool_use id -> name 映射
	toolIDToName := make(map[string]string)

	// 检测是否有 web_search 工具
	hasWebSearchTool := hasWebSearchTool(claudeReq.Tools)
	requestType := "agent"
	targetModel := mappedModel
	if hasWebSearchTool {
		requestType = "web_search"
		if targetModel != webSearchFallbackModel {
			targetModel = webSearchFallbackModel
	REDACTED
REDACTED

	// 检测是否启用 thinking
	isThinkingEnabled := claudeReq.Thinking != nil && claudeReq.Thinking.Type == "enabled"

	// 只有 Gemini 模型支持 dummy thought workaround
	// Claude 模型通过 Vertex/Google API 需要有效的 thought signatures
	allowDummyThought := strings.HasPrefix(targetModel, "gemini-")

	// 1. 构建 contents
	contents, strippedThinking, err := buildContents(claudeReq.Messages, toolIDToName, isThinkingEnabled, allowDummyThought)
	if err != nil {
		return nil, fmt.Errorf("build contents: %w", err)
REDACTED

	// 2. 构建 systemInstruction
	systemInstruction := buildSystemInstruction(claudeReq.System, claudeReq.Model, opts, claudeReq.Tools)

	// 3. 构建 generationConfig
	reqForConfig := claudeReq
	if strippedThinking {
		// If we had to downgrade thinking blocks to plain text due to missing/invalid signatures,
		// disable upstream thinking mode to avoid signature/structure validation errors.
		reqCopy := *claudeReq
		reqCopy.Thinking = nil
		reqForConfig = &reqCopy
REDACTED
	if targetModel != "" && targetModel != reqForConfig.Model {
		reqCopy := *reqForConfig
		reqCopy.Model = targetModel
		reqForConfig = &reqCopy
REDACTED
	generationConfig := buildGenerationConfig(reqForConfig)

	// 4. 构建 tools
	tools := buildTools(claudeReq.Tools)

	// 5. 构建内部请求
	innerRequest := GeminiRequest{
		Contents: contents,
		// 总是设置 toolConfig，与官方客户端一致
		ToolConfig: &GeminiToolConfig{
			FunctionCallingConfig: &GeminiFunctionCallingConfig{
				Mode: "VALIDATED",
		REDACTED,
	REDACTED,
		// 总是生成 sessionId，基于用户消息内容
		SessionID: generateStableSessionID(contents),
REDACTED

	if systemInstruction != nil {
		innerRequest.SystemInstruction = systemInstruction
REDACTED
	if generationConfig != nil {
		innerRequest.GenerationConfig = generationConfig
REDACTED
	if len(tools) > 0 {
		innerRequest.Tools = tools
REDACTED

	// 如果提供了 metadata.user_id，优先使用
	if claudeReq.Metadata != nil && claudeReq.Metadata.UserID != "" {
		innerRequest.SessionID = claudeReq.Metadata.UserID
REDACTED

	// 6. 包装为 v1internal 请求
	v1Req := V1InternalRequest{
		Project:     projectID,
		RequestID:   "agent-" + uuid.New().String(),
		UserAgent:   "antigravity", // 固定值，与官方客户端一致
		RequestType: requestType,
		Model:       targetModel,
		Request:     innerRequest,
REDACTED

	return json.Marshal(v1Req)
REDACTED

// antigravityIdentity Antigravity identity 提示词
const antigravityIdentity = `<identity>
You are Antigravity, a powerful agentic AI coding assistant designed by the Google Deepmind team working on Advanced Agentic Coding.
You are pair programming with a USER to solve their coding task. The task may require creating a new codebase, modifying or debugging an existing codebase, or simply answering a question.
The USER will send you requests, which you must always prioritize addressing. Along with each USER request, we will attach additional metadata about their current state, such as what files they have open and where their cursor is.
This information may or may not be relevant to the coding task, it is up for you to decide.
</identity>
<communication_style>
- **Proactiveness**. As an agent, you are allowed to be proactive, but only in the course of completing the user's task. For example, if the user asks you to add a new component, you can edit the code, verify build and test statuses, and take any other obvious follow-up actions, such as performing additional research. However, avoid surprising the user. For example, if the user asks HOW to approach something, you should answer their question and instead of jumping into editing a file.</communication_style>`

func defaultIdentityPatch(_ string) string {
	return antigravityIdentity
REDACTED

// GetDefaultIdentityPatch 返回默认的 Antigravity 身份提示词
func GetDefaultIdentityPatch() string {
	return antigravityIdentity
REDACTED

// mcpXMLProtocol MCP XML 工具调用协议（与 Antigravity-Manager 保持一致）
const mcpXMLProtocol = `
==== MCP XML 工具调用协议 (Workaround) ====
当你需要调用名称以 ` + "`mcp__`" + ` 开头的 MCP 工具时：
1) 优先尝试 XML 格式调用：输出 ` + "`<mcp__tool_name>{\"arg\":\"value\"REDACTED</mcp__tool_name>`" + `。
2) 必须直接输出 XML 块，无需 markdown 包装，内容为 JSON 格式的入参。
3) 这种方式具有更高的连通性和容错性，适用于大型结果返回场景。
===========================================`

// hasMCPTools 检测是否有 mcp__ 前缀的工具
func hasMCPTools(tools []ClaudeTool) bool {
	for _, tool := range tools {
		if strings.HasPrefix(tool.Name, "mcp__") {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// filterOpenCodePrompt 过滤 OpenCode 默认提示词，只保留用户自定义指令
func filterOpenCodePrompt(text string) string {
	if !strings.Contains(text, "You are an interactive CLI tool") {
		return text
REDACTED
	// 提取 "Instructions from:" 及之后的部分
	if idx := strings.Index(text, "Instructions from:"); idx >= 0 {
		return text[idx:]
REDACTED
	// 如果没有自定义指令，返回空
	return ""
REDACTED

// buildSystemInstruction 构建 systemInstruction（与 Antigravity-Manager 保持一致）
func buildSystemInstruction(system json.RawMessage, modelName string, opts TransformOptions, tools []ClaudeTool) *GeminiContent {
	var parts []GeminiPart

	// 先解析用户的 system prompt，检测是否已包含 Antigravity identity
	userHasAntigravityIdentity := false
	var userSystemParts []GeminiPart

	if len(system) > 0 {
		// 尝试解析为字符串
		var sysStr string
		if err := json.Unmarshal(system, &sysStr); err == nil {
			if strings.TrimSpace(sysStr) != "" {
				if strings.Contains(sysStr, "You are Antigravity") {
					userHasAntigravityIdentity = true
			REDACTED
				// 过滤 OpenCode 默认提示词
				filtered := filterOpenCodePrompt(sysStr)
				if filtered != "" {
					userSystemParts = append(userSystemParts, GeminiPart{Text: filteredREDACTED)
			REDACTED
		REDACTED
	REDACTED else {
			// 尝试解析为数组
			var sysBlocks []SystemBlock
			if err := json.Unmarshal(system, &sysBlocks); err == nil {
				for _, block := range sysBlocks {
					if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
						if strings.Contains(block.Text, "You are Antigravity") {
							userHasAntigravityIdentity = true
					REDACTED
						// 过滤 OpenCode 默认提示词
						filtered := filterOpenCodePrompt(block.Text)
						if filtered != "" {
							userSystemParts = append(userSystemParts, GeminiPart{Text: filteredREDACTED)
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 仅在用户未提供 Antigravity identity 时注入
	if opts.EnableIdentityPatch && !userHasAntigravityIdentity {
		identityPatch := strings.TrimSpace(opts.IdentityPatch)
		if identityPatch == "" {
			identityPatch = defaultIdentityPatch(modelName)
	REDACTED
		parts = append(parts, GeminiPart{Text: identityPatchREDACTED)
REDACTED

	// 添加用户的 system prompt
	parts = append(parts, userSystemParts...)

	// 检测是否有 MCP 工具，如有则注入 XML 调用协议
	if hasMCPTools(tools) {
		parts = append(parts, GeminiPart{Text: mcpXMLProtocolREDACTED)
REDACTED

	// 如果用户没有提供 Antigravity 身份，添加结束标记
	if !userHasAntigravityIdentity {
		parts = append(parts, GeminiPart{Text: "\n--- [SYSTEM_PROMPT_END] ---"REDACTED)
REDACTED

	if len(parts) == 0 {
		return nil
REDACTED

	return &GeminiContent{
		Role:  "user",
		Parts: parts,
REDACTED
REDACTED

// buildContents 构建 contents
func buildContents(messages []ClaudeMessage, toolIDToName map[string]string, isThinkingEnabled, allowDummyThought bool) ([]GeminiContent, bool, error) {
	var contents []GeminiContent
	strippedThinking := false

	for i, msg := range messages {
		role := msg.Role
		if role == "assistant" {
			role = "model"
	REDACTED

		parts, strippedThisMsg, err := buildParts(msg.Content, toolIDToName, allowDummyThought)
		if err != nil {
			return nil, false, fmt.Errorf("build parts for message %d: %w", i, err)
	REDACTED
		if strippedThisMsg {
			strippedThinking = true
	REDACTED

		// 只有 Gemini 模型支持 dummy thinking block workaround
		// 只对最后一条 assistant 消息添加（Pre-fill 场景）
		// 历史 assistant 消息不能添加没有 signature 的 dummy thinking block
		if allowDummyThought && role == "model" && isThinkingEnabled && i == len(messages)-1 {
			hasThoughtPart := false
			for _, p := range parts {
				if p.Thought {
					hasThoughtPart = true
					break
			REDACTED
		REDACTED
			if !hasThoughtPart && len(parts) > 0 {
				// 在开头添加 dummy thinking block
				parts = append([]GeminiPart{{
					Text:             "Thinking...",
					Thought:          true,
					ThoughtSignature: dummyThoughtSignature,
		REDACTED parts...)
		REDACTED
	REDACTED

		if len(parts) == 0 {
			continue
	REDACTED

		contents = append(contents, GeminiContent{
			Role:  role,
			Parts: parts,
	REDACTED)
REDACTED

	return contents, strippedThinking, nil
REDACTED

// dummyThoughtSignature 用于跳过 Gemini 3 thought_signature 验证
// 参考: https://ai.google.dev/gemini-api/docs/thought-signatures
const dummyThoughtSignature = "skip_thought_signature_validator"

// buildParts 构建消息的 parts
// allowDummyThought: 只有 Gemini 模型支持 dummy thought signature
func buildParts(content json.RawMessage, toolIDToName map[string]string, allowDummyThought bool) ([]GeminiPart, bool, error) {
	var parts []GeminiPart
	strippedThinking := false

	// 尝试解析为字符串
	var textContent string
	if err := json.Unmarshal(content, &textContent); err == nil {
		if textContent != "(no content)" && strings.TrimSpace(textContent) != "" {
			parts = append(parts, GeminiPart{Text: strings.TrimSpace(textContent)REDACTED)
	REDACTED
		return parts, false, nil
REDACTED

	// 解析为内容块数组
	var blocks []ContentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, false, fmt.Errorf("parse content blocks: %w", err)
REDACTED

	for _, block := range blocks {
		switch block.Type {
		case "text":
			if block.Text != "(no content)" && strings.TrimSpace(block.Text) != "" {
				parts = append(parts, GeminiPart{Text: block.TextREDACTED)
		REDACTED

		case "thinking":
			part := GeminiPart{
				Text:    block.Thinking,
				Thought: true,
		REDACTED
			// signature 处理：
			// - Claude 模型（allowDummyThought=false）：必须是上游返回的真实 signature（dummy 视为缺失）
			// - Gemini 模型（allowDummyThought=true）：优先透传真实 signature，缺失时使用 dummy signature
			if block.Signature != "" && (allowDummyThought || block.Signature != dummyThoughtSignature) {
				part.ThoughtSignature = block.Signature
		REDACTED else if !allowDummyThought {
				// Claude 模型需要有效 signature；在缺失时降级为普通文本，并在上层禁用 thinking mode。
				if strings.TrimSpace(block.Thinking) != "" {
					parts = append(parts, GeminiPart{Text: block.ThinkingREDACTED)
			REDACTED
				strippedThinking = true
				continue
		REDACTED else {
				// Gemini 模型使用 dummy signature
				part.ThoughtSignature = dummyThoughtSignature
		REDACTED
			parts = append(parts, part)

		case "image":
			if block.Source != nil && block.Source.Type == "base64" {
				parts = append(parts, GeminiPart{
					InlineData: &GeminiInlineData{
						MimeType: block.Source.MediaType,
						Data:     block.Source.Data,
				REDACTED,
			REDACTED)
		REDACTED

		case "tool_use":
			// 存储 id -> name 映射
			if block.ID != "" && block.Name != "" {
				toolIDToName[block.ID] = block.Name
		REDACTED

			part := GeminiPart{
				FunctionCall: &GeminiFunctionCall{
					Name: block.Name,
					Args: block.Input,
					ID:   block.ID,
			REDACTED,
		REDACTED
			// tool_use 的 signature 处理：
			// - Claude 模型（allowDummyThought=false）：必须是上游返回的真实 signature（dummy 视为缺失）
			// - Gemini 模型（allowDummyThought=true）：优先透传真实 signature，缺失时使用 dummy signature
			if block.Signature != "" && (allowDummyThought || block.Signature != dummyThoughtSignature) {
				part.ThoughtSignature = block.Signature
		REDACTED else if allowDummyThought {
				part.ThoughtSignature = dummyThoughtSignature
		REDACTED
			parts = append(parts, part)

		case "tool_result":
			// 获取函数名
			funcName := block.Name
			if funcName == "" {
				if name, ok := toolIDToName[block.ToolUseID]; ok {
					funcName = name
			REDACTED else {
					funcName = block.ToolUseID
			REDACTED
		REDACTED

			// 解析 content
			resultContent := parseToolResultContent(block.Content, block.IsError)

			parts = append(parts, GeminiPart{
				FunctionResponse: &GeminiFunctionResponse{
					Name: funcName,
					Response: map[string]any{
						"result": resultContent,
				REDACTED,
					ID: block.ToolUseID,
			REDACTED,
		REDACTED)
	REDACTED
REDACTED

	return parts, strippedThinking, nil
REDACTED

// parseToolResultContent 解析 tool_result 的 content
func parseToolResultContent(content json.RawMessage, isError bool) string {
	if len(content) == 0 {
		if isError {
			return "Tool execution failed with no output."
	REDACTED
		return "Command executed successfully."
REDACTED

	// 尝试解析为字符串
	var str string
	if err := json.Unmarshal(content, &str); err == nil {
		if strings.TrimSpace(str) == "" {
			if isError {
				return "Tool execution failed with no output."
		REDACTED
			return "Command executed successfully."
	REDACTED
		return str
REDACTED

	// 尝试解析为数组
	var arr []map[string]any
	if err := json.Unmarshal(content, &arr); err == nil {
		var texts []string
		for _, item := range arr {
			if text, ok := item["text"].(string); ok {
				texts = append(texts, text)
		REDACTED
	REDACTED
		result := strings.Join(texts, "\n")
		if strings.TrimSpace(result) == "" {
			if isError {
				return "Tool execution failed with no output."
		REDACTED
			return "Command executed successfully."
	REDACTED
		return result
REDACTED

	// 返回原始 JSON
	return string(content)
REDACTED

// buildGenerationConfig 构建 generationConfig
func buildGenerationConfig(req *ClaudeRequest) *GeminiGenerationConfig {
	config := &GeminiGenerationConfig{
		MaxOutputTokens: 64000, // 默认最大输出
		StopSequences:   DefaultStopSequences,
REDACTED

	// 如果请求中指定了 MaxTokens，使用请求值
	if req.MaxTokens > 0 {
		config.MaxOutputTokens = req.MaxTokens
REDACTED

	// Thinking 配置
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		config.ThinkingConfig = &GeminiThinkingConfig{
			IncludeThoughts: true,
	REDACTED
		if req.Thinking.BudgetTokens > 0 {
			budget := req.Thinking.BudgetTokens
			// gemini-2.5-flash 上限 24576
			if strings.Contains(req.Model, "gemini-2.5-flash") && budget > 24576 {
				budget = 24576
		REDACTED
			config.ThinkingConfig.ThinkingBudget = budget
	REDACTED
REDACTED

	// 其他参数
	if req.Temperature != nil {
		config.Temperature = req.Temperature
REDACTED
	if req.TopP != nil {
		config.TopP = req.TopP
REDACTED
	if req.TopK != nil {
		config.TopK = req.TopK
REDACTED

	return config
REDACTED

func hasWebSearchTool(tools []ClaudeTool) bool {
	for _, tool := range tools {
		if isWebSearchTool(tool) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func isWebSearchTool(tool ClaudeTool) bool {
	if strings.HasPrefix(tool.Type, "web_search") || tool.Type == "google_search" {
		return true
REDACTED

	name := strings.TrimSpace(tool.Name)
	switch name {
	case "web_search", "google_search", "web_search_20250305":
		return true
	default:
		return false
REDACTED
REDACTED

// buildTools 构建 tools
func buildTools(tools []ClaudeTool) []GeminiToolDeclaration {
	if len(tools) == 0 {
		return nil
REDACTED

	hasWebSearch := hasWebSearchTool(tools)

	// 普通工具
	var funcDecls []GeminiFunctionDecl
	for _, tool := range tools {
		if isWebSearchTool(tool) {
			continue
	REDACTED
		// 跳过无效工具名称
		if strings.TrimSpace(tool.Name) == "" {
			log.Printf("Warning: skipping tool with empty name")
			continue
	REDACTED

		var description string
		var inputSchema map[string]any

		// 检查是否为 custom 类型工具 (MCP)
		if tool.Type == "custom" {
			if tool.Custom == nil || tool.Custom.InputSchema == nil {
				log.Printf("[Warning] Skipping invalid custom tool '%s': missing custom spec or input_schema", tool.Name)
				continue
		REDACTED
			description = tool.Custom.Description
			inputSchema = tool.Custom.InputSchema

	REDACTED else {
			// 标准格式: 从顶层字段获取
			description = tool.Description
			inputSchema = tool.InputSchema
	REDACTED

		// 清理 JSON Schema
		// 1. 深度清理 [undefined] 值
		DeepCleanUndefined(inputSchema)
		// 2. 转换为符合 Gemini v1internal 的 schema
		params := CleanJSONSchema(inputSchema)
		// 为 nil schema 提供默认值
		if params == nil {
			params = map[string]any{
				"type":       "object", // lowercase type
				"properties": map[string]any{REDACTED,
		REDACTED
	REDACTED

		funcDecls = append(funcDecls, GeminiFunctionDecl{
			Name:        tool.Name,
			Description: description,
			Parameters:  params,
	REDACTED)
REDACTED

	if len(funcDecls) == 0 {
		if !hasWebSearch {
			return nil
	REDACTED

		// Web Search 工具映射
		return []GeminiToolDeclaration{{
			GoogleSearch: &GeminiGoogleSearch{
				EnhancedContent: &GeminiEnhancedContent{
					ImageSearch: &GeminiImageSearch{
						MaxResultCount: 5,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTEDREDACTED
REDACTED

	return []GeminiToolDeclaration{{
		FunctionDeclarations: funcDecls,
REDACTEDREDACTED
REDACTED
