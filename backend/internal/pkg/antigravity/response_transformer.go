package antigravity

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TransformGeminiToClaude 将 Gemini 响应转换为 Claude 格式（非流式）
func TransformGeminiToClaude(geminiResp []byte, originalModel string) ([]byte, *ClaudeUsage, error) {
	// 解包 v1internal 响应
	var v1Resp V1InternalResponse
	if err := json.Unmarshal(geminiResp, &v1Resp); err != nil {
		// 尝试直接解析为 GeminiResponse
		var directResp GeminiResponse
		if err2 := json.Unmarshal(geminiResp, &directResp); err2 != nil {
			return nil, nil, fmt.Errorf("parse gemini response: %w", err)
	REDACTED
		v1Resp.Response = directResp
		v1Resp.ResponseID = directResp.ResponseID
		v1Resp.ModelVersion = directResp.ModelVersion
REDACTED

	// 使用处理器转换
	processor := NewNonStreamingProcessor()
	claudeResp := processor.Process(&v1Resp.Response, v1Resp.ResponseID, originalModel)

	// 序列化
	respBytes, err := json.Marshal(claudeResp)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal claude response: %w", err)
REDACTED

	return respBytes, &claudeResp.Usage, nil
REDACTED

// NonStreamingProcessor 非流式响应处理器
type NonStreamingProcessor struct {
	contentBlocks     []ClaudeContentItem
	textBuilder       string
	thinkingBuilder   string
	thinkingSignature string
	trailingSignature string
	hasToolCall       bool
REDACTED

// NewNonStreamingProcessor 创建非流式响应处理器
func NewNonStreamingProcessor() *NonStreamingProcessor {
	return &NonStreamingProcessor{
		contentBlocks: make([]ClaudeContentItem, 0),
REDACTED
REDACTED

// Process 处理 Gemini 响应
func (p *NonStreamingProcessor) Process(geminiResp *GeminiResponse, responseID, originalModel string) *ClaudeResponse {
	// 获取 parts
	var parts []GeminiPart
	if len(geminiResp.Candidates) > 0 && geminiResp.Candidates[0].Content != nil {
		parts = geminiResp.Candidates[0].Content.Parts
REDACTED

	// 处理所有 parts
	for _, part := range parts {
		p.processPart(&part)
REDACTED

	if len(geminiResp.Candidates) > 0 {
		if grounding := geminiResp.Candidates[0].GroundingMetadata; grounding != nil {
			p.processGrounding(grounding)
	REDACTED
REDACTED

	// 刷新剩余内容
	p.flushThinking()
	p.flushText()

	// 处理 trailingSignature
	if p.trailingSignature != "" {
		p.contentBlocks = append(p.contentBlocks, ClaudeContentItem{
			Type:      "thinking",
			Thinking:  "",
			Signature: p.trailingSignature,
	REDACTED)
REDACTED

	// 构建响应
	return p.buildResponse(geminiResp, responseID, originalModel)
REDACTED

// processPart 处理单个 part
func (p *NonStreamingProcessor) processPart(part *GeminiPart) {
	signature := part.ThoughtSignature

	// 1. FunctionCall 处理
	if part.FunctionCall != nil {
		p.flushThinking()
		p.flushText()

		// 处理 trailingSignature
		if p.trailingSignature != "" {
			p.contentBlocks = append(p.contentBlocks, ClaudeContentItem{
				Type:      "thinking",
				Thinking:  "",
				Signature: p.trailingSignature,
		REDACTED)
			p.trailingSignature = ""
	REDACTED

		p.hasToolCall = true

		// 生成 tool_use id
		toolID := part.FunctionCall.ID
		if toolID == "" {
			toolID = fmt.Sprintf("%s-%s", part.FunctionCall.Name, generateRandomID())
	REDACTED

		item := ClaudeContentItem{
			Type:  "tool_use",
			ID:    toolID,
			Name:  part.FunctionCall.Name,
			Input: part.FunctionCall.Args,
	REDACTED

		if signature != "" {
			item.Signature = signature
	REDACTED

		p.contentBlocks = append(p.contentBlocks, item)
		return
REDACTED

	// 2. Text 处理
	if part.Text != "" || part.Thought {
		if part.Thought {
			// Thinking part
			p.flushText()

			// 处理 trailingSignature
			if p.trailingSignature != "" {
				p.flushThinking()
				p.contentBlocks = append(p.contentBlocks, ClaudeContentItem{
					Type:      "thinking",
					Thinking:  "",
					Signature: p.trailingSignature,
			REDACTED)
				p.trailingSignature = ""
		REDACTED

			p.thinkingBuilder += part.Text
			if signature != "" {
				p.thinkingSignature = signature
		REDACTED
	REDACTED else {
			// 普通 Text
			if part.Text == "" {
				// 空 text 带签名 - 暂存
				if signature != "" {
					p.trailingSignature = signature
			REDACTED
				return
		REDACTED

			p.flushThinking()

			// 处理之前的 trailingSignature
			if p.trailingSignature != "" {
				p.flushText()
				p.contentBlocks = append(p.contentBlocks, ClaudeContentItem{
					Type:      "thinking",
					Thinking:  "",
					Signature: p.trailingSignature,
			REDACTED)
				p.trailingSignature = ""
		REDACTED

			p.textBuilder += part.Text

			// 非空 text 带签名 - 立即刷新并输出空 thinking 块
			if signature != "" {
				p.flushText()
				p.contentBlocks = append(p.contentBlocks, ClaudeContentItem{
					Type:      "thinking",
					Thinking:  "",
					Signature: signature,
			REDACTED)
		REDACTED
	REDACTED
REDACTED

	// 3. InlineData (Image) 处理
	if part.InlineData != nil && part.InlineData.Data != "" {
		p.flushThinking()
		markdownImg := fmt.Sprintf("![image](data:%s;base64,%s)",
			part.InlineData.MimeType, part.InlineData.Data)
		p.textBuilder += markdownImg
		p.flushText()
REDACTED
REDACTED

func (p *NonStreamingProcessor) processGrounding(grounding *GeminiGroundingMetadata) {
	groundingText := buildGroundingText(grounding)
	if groundingText == "" {
		return
REDACTED

	p.flushThinking()
	p.flushText()
	p.textBuilder += groundingText
	p.flushText()
REDACTED

// flushText 刷新 text builder
func (p *NonStreamingProcessor) flushText() {
	if p.textBuilder == "" {
		return
REDACTED

	p.contentBlocks = append(p.contentBlocks, ClaudeContentItem{
		Type: "text",
		Text: p.textBuilder,
REDACTED)
	p.textBuilder = ""
REDACTED

// flushThinking 刷新 thinking builder
func (p *NonStreamingProcessor) flushThinking() {
	if p.thinkingBuilder == "" && p.thinkingSignature == "" {
		return
REDACTED

	p.contentBlocks = append(p.contentBlocks, ClaudeContentItem{
		Type:      "thinking",
		Thinking:  p.thinkingBuilder,
		Signature: p.thinkingSignature,
REDACTED)
	p.thinkingBuilder = ""
	p.thinkingSignature = ""
REDACTED

// buildResponse 构建最终响应
func (p *NonStreamingProcessor) buildResponse(geminiResp *GeminiResponse, responseID, originalModel string) *ClaudeResponse {
	var finishReason string
	if len(geminiResp.Candidates) > 0 {
		finishReason = geminiResp.Candidates[0].FinishReason
REDACTED

	stopReason := "end_turn"
	if p.hasToolCall {
		stopReason = "tool_use"
REDACTED else if finishReason == "MAX_TOKENS" {
		stopReason = "max_tokens"
REDACTED

	// 注意：Gemini 的 promptTokenCount 包含 cachedContentTokenCount，
	// 但 Claude 的 input_tokens 不包含 cache_read_input_tokens，需要减去
	usage := ClaudeUsage{REDACTED
	if geminiResp.UsageMetadata != nil {
		cached := geminiResp.UsageMetadata.CachedContentTokenCount
		usage.InputTokens = geminiResp.UsageMetadata.PromptTokenCount - cached
		usage.OutputTokens = geminiResp.UsageMetadata.CandidatesTokenCount
		usage.CacheReadInputTokens = cached
REDACTED

	// 生成响应 ID
	respID := responseID
	if respID == "" {
		respID = geminiResp.ResponseID
REDACTED
	if respID == "" {
		respID = "msg_" + generateRandomID()
REDACTED

	return &ClaudeResponse{
		ID:         respID,
		Type:       "message",
		Role:       "assistant",
		Model:      originalModel,
		Content:    p.contentBlocks,
		StopReason: stopReason,
		Usage:      usage,
REDACTED
REDACTED

func buildGroundingText(grounding *GeminiGroundingMetadata) string {
	if grounding == nil {
		return ""
REDACTED

	var builder strings.Builder

	if len(grounding.WebSearchQueries) > 0 {
		_, _ = builder.WriteString("\n\n---\nWeb search queries: ")
		_, _ = builder.WriteString(strings.Join(grounding.WebSearchQueries, ", "))
REDACTED

	if len(grounding.GroundingChunks) > 0 {
		var links []string
		for i, chunk := range grounding.GroundingChunks {
			if chunk.Web == nil {
				continue
		REDACTED
			title := strings.TrimSpace(chunk.Web.Title)
			if title == "" {
				title = "Source"
		REDACTED
			uri := strings.TrimSpace(chunk.Web.URI)
			if uri == "" {
				uri = "#"
		REDACTED
			links = append(links, fmt.Sprintf("[%d] [%s](%s)", i+1, title, uri))
	REDACTED

		if len(links) > 0 {
			_, _ = builder.WriteString("\n\nSources:\n")
			_, _ = builder.WriteString(strings.Join(links, "\n"))
	REDACTED
REDACTED

	return builder.String()
REDACTED

// generateRandomID 生成随机 ID
func generateRandomID() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 12)
	for i := range result {
		result[i] = chars[i%len(chars)]
REDACTED
	return string(result)
REDACTED
