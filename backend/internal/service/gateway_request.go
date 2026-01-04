package service

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ParsedRequest 保存网关请求的预解析结果
//
// 性能优化说明：
// 原实现在多个位置重复解析请求体（Handler、Service 各解析一次）：
// 1. gateway_handler.go 解析获取 model 和 stream
// 2. gateway_service.go 再次解析获取 system、messages、metadata
// 3. GenerateSessionHash 又一次解析获取会话哈希所需字段
//
// 新实现一次解析，多处复用：
// 1. 在 Handler 层统一调用 ParseGatewayRequest 一次性解析
// 2. 将解析结果 ParsedRequest 传递给 Service 层
// 3. 避免重复 json.Unmarshal，减少 CPU 和内存开销
type ParsedRequest struct {
	Body           []byte // 原始请求体（保留用于转发）
	Model          string // 请求的模型名称
	Stream         bool   // 是否为流式请求
	MetadataUserID string // metadata.user_id（用于会话亲和）
	System         any    // system 字段内容
	Messages       []any  // messages 数组
	HasSystem      bool   // 是否包含 system 字段（包含 null 也视为显式传入）
REDACTED

// ParseGatewayRequest 解析网关请求体并返回结构化结果
// 性能优化：一次解析提取所有需要的字段，避免重复 Unmarshal
func ParseGatewayRequest(body []byte) (*ParsedRequest, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
REDACTED

	parsed := &ParsedRequest{
		Body: body,
REDACTED

	if rawModel, exists := req["model"]; exists {
		model, ok := rawModel.(string)
		if !ok {
			return nil, fmt.Errorf("invalid model field type")
	REDACTED
		parsed.Model = model
REDACTED
	if rawStream, exists := req["stream"]; exists {
		stream, ok := rawStream.(bool)
		if !ok {
			return nil, fmt.Errorf("invalid stream field type")
	REDACTED
		parsed.Stream = stream
REDACTED
	if metadata, ok := req["metadata"].(map[string]any); ok {
		if userID, ok := metadata["user_id"].(string); ok {
			parsed.MetadataUserID = userID
	REDACTED
REDACTED
	// system 字段只要存在就视为显式提供（即使为 null），
	// 以避免客户端传 null 时被默认 system 误注入。
	if system, ok := req["system"]; ok {
		parsed.HasSystem = true
		parsed.System = system
REDACTED
	if messages, ok := req["messages"].([]any); ok {
		parsed.Messages = messages
REDACTED

	return parsed, nil
REDACTED

// FilterThinkingBlocks removes thinking blocks from request body
// Returns filtered body or original body if filtering fails (fail-safe)
// This prevents 400 errors from invalid thinking block signatures
//
// Strategy:
//   - When thinking.type != "enabled": Remove all thinking blocks
//   - When thinking.type == "enabled": Only remove thinking blocks without valid signatures
//     (blocks with missing/empty/dummy signatures that would cause 400 errors)
func FilterThinkingBlocks(body []byte) []byte {
	return filterThinkingBlocksInternal(body, false)
REDACTED

// FilterThinkingBlocksForRetry converts thinking blocks to text blocks for retry scenarios
// This preserves the thinking content while avoiding signature validation errors.
// Note: redacted_thinking blocks are removed because they cannot be converted to text.
func FilterThinkingBlocksForRetry(body []byte) []byte {
	// Fast path: check for presence of thinking-related keys
	if !bytes.Contains(body, []byte(`"type":"thinking"`)) &&
		!bytes.Contains(body, []byte(`"type": "thinking"`)) &&
		!bytes.Contains(body, []byte(`"type":"redacted_thinking"`)) &&
		!bytes.Contains(body, []byte(`"type": "redacted_thinking"`)) &&
		!bytes.Contains(body, []byte(`"thinking":`)) &&
		!bytes.Contains(body, []byte(`"thinking" :`)) {
		return body
REDACTED

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
REDACTED

	messages, ok := req["messages"].([]any)
	if !ok {
		return body
REDACTED

	modified := false
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
	REDACTED

		content, ok := msgMap["content"].([]any)
		if !ok {
			continue
	REDACTED

		newContent := make([]any, 0, len(content))
		modifiedThisMsg := false

		for _, block := range content {
			blockMap, ok := block.(map[string]any)
			if !ok {
				newContent = append(newContent, block)
				continue
		REDACTED

			blockType, _ := blockMap["type"].(string)

			// Case 1: Standard thinking block - convert to text
			if blockType == "thinking" {
				thinkingText, _ := blockMap["thinking"].(string)
				if thinkingText != "" {
					newContent = append(newContent, map[string]any{
						"type": "text",
						"text": thinkingText,
				REDACTED)
			REDACTED
				modifiedThisMsg = true
				continue
		REDACTED

			// Case 2: Redacted thinking block - remove (cannot convert encrypted content)
			if blockType == "redacted_thinking" {
				modifiedThisMsg = true
				continue
		REDACTED

			// Case 3: Untyped block with "thinking" field - convert to text
			if blockType == "" {
				if thinkingText, hasThinking := blockMap["thinking"].(string); hasThinking {
					if thinkingText != "" {
						newContent = append(newContent, map[string]any{
							"type": "text",
							"text": thinkingText,
					REDACTED)
				REDACTED
					modifiedThisMsg = true
					continue
			REDACTED
		REDACTED

			newContent = append(newContent, block)
	REDACTED

		if modifiedThisMsg {
			msgMap["content"] = newContent
			modified = true
	REDACTED
REDACTED

	if !modified {
		return body
REDACTED

	newBody, err := json.Marshal(req)
	if err != nil {
		return body
REDACTED
	return newBody
REDACTED

// filterThinkingBlocksInternal removes invalid thinking blocks from request
// Strategy:
//   - When thinking.type != "enabled": Remove all thinking blocks
//   - When thinking.type == "enabled": Only remove thinking blocks without valid signatures
func filterThinkingBlocksInternal(body []byte, _ bool) []byte {
	// Fast path: if body doesn't contain "thinking", skip parsing
	if !bytes.Contains(body, []byte(`"type":"thinking"`)) &&
		!bytes.Contains(body, []byte(`"type": "thinking"`)) &&
		!bytes.Contains(body, []byte(`"type":"redacted_thinking"`)) &&
		!bytes.Contains(body, []byte(`"type": "redacted_thinking"`)) &&
		!bytes.Contains(body, []byte(`"thinking":`)) &&
		!bytes.Contains(body, []byte(`"thinking" :`)) {
		return body
REDACTED

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
REDACTED

	// Check if thinking is enabled
	thinkingEnabled := false
	if thinking, ok := req["thinking"].(map[string]any); ok {
		if thinkType, ok := thinking["type"].(string); ok && thinkType == "enabled" {
			thinkingEnabled = true
	REDACTED
REDACTED

	messages, ok := req["messages"].([]any)
	if !ok {
		return body
REDACTED

	filtered := false
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
	REDACTED

		role, _ := msgMap["role"].(string)
		content, ok := msgMap["content"].([]any)
		if !ok {
			continue
	REDACTED

		newContent := make([]any, 0, len(content))
		filteredThisMessage := false

		for _, block := range content {
			blockMap, ok := block.(map[string]any)
			if !ok {
				newContent = append(newContent, block)
				continue
		REDACTED

			blockType, _ := blockMap["type"].(string)

			if blockType == "thinking" || blockType == "redacted_thinking" {
				// When thinking is enabled and this is an assistant message,
				// only keep thinking blocks with valid signatures
				if thinkingEnabled && role == "assistant" {
					signature, _ := blockMap["signature"].(string)
					if signature != "" && signature != "skip_thought_signature_validator" {
						newContent = append(newContent, block)
						continue
				REDACTED
			REDACTED
				filtered = true
				filteredThisMessage = true
				continue
		REDACTED

			// Handle blocks without type discriminator but with "thinking" key
			if blockType == "" {
				if _, hasThinking := blockMap["thinking"]; hasThinking {
					filtered = true
					filteredThisMessage = true
					continue
			REDACTED
		REDACTED

			newContent = append(newContent, block)
	REDACTED

		if filteredThisMessage {
			msgMap["content"] = newContent
	REDACTED
REDACTED

	if !filtered {
		return body
REDACTED

	newBody, err := json.Marshal(req)
	if err != nil {
		return body
REDACTED
	return newBody
REDACTED
