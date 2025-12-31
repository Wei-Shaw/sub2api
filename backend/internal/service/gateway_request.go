package service

import (
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
	HasSystem      bool   // 是否包含 system 字段
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
	if system, ok := req["system"]; ok && system != nil {
		parsed.HasSystem = true
		parsed.System = system
REDACTED
	if messages, ok := req["messages"].([]any); ok {
		parsed.Messages = messages
REDACTED

	return parsed, nil
REDACTED
