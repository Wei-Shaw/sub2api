package service

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

// codexToolNameMapping 定义 Codex 原生工具名称到 OpenCode 工具名称的映射
var codexToolNameMapping = map[string]string{
	"apply_patch":  "edit",
	"applyPatch":   "edit",
	"update_plan":  "todowrite",
	"updatePlan":   "todowrite",
	"read_plan":    "todoread",
	"readPlan":     "todoread",
	"search_files": "grep",
	"searchFiles":  "grep",
	"list_files":   "glob",
	"listFiles":    "glob",
	"read_file":    "read",
	"readFile":     "read",
	"write_file":   "write",
	"writeFile":    "write",
	"execute_bash": "bash",
	"executeBash":  "bash",
	"exec_bash":    "bash",
	"execBash":     "bash",
REDACTED

// ToolCorrectionStats 记录工具修正的统计信息
type ToolCorrectionStats struct {
	TotalCorrected    int            `json:"total_corrected"`
	CorrectionsByTool map[string]int `json:"corrections_by_tool"`
	mu                sync.RWMutex
REDACTED

// CodexToolCorrector 处理 Codex 工具调用的自动修正
type CodexToolCorrector struct {
	stats ToolCorrectionStats
REDACTED

// NewCodexToolCorrector 创建新的工具修正器
func NewCodexToolCorrector() *CodexToolCorrector {
	return &CodexToolCorrector{
		stats: ToolCorrectionStats{
			CorrectionsByTool: make(map[string]int),
	REDACTED,
REDACTED
REDACTED

// CorrectToolCallsInSSEData 修正 SSE 数据中的工具调用
// 返回修正后的数据和是否进行了修正
func (c *CodexToolCorrector) CorrectToolCallsInSSEData(data string) (string, bool) {
	if data == "" || data == "\n" {
		return data, false
REDACTED

	// 尝试解析 JSON
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		// 不是有效的 JSON，直接返回原数据
		return data, false
REDACTED

	corrected := false

	// 处理 tool_calls 数组
	if toolCalls, ok := payload["tool_calls"].([]any); ok {
		if c.correctToolCallsArray(toolCalls) {
			corrected = true
	REDACTED
REDACTED

	// 处理 function_call 对象
	if functionCall, ok := payload["function_call"].(map[string]any); ok {
		if c.correctFunctionCall(functionCall) {
			corrected = true
	REDACTED
REDACTED

	// 处理 delta.tool_calls
	if delta, ok := payload["delta"].(map[string]any); ok {
		if toolCalls, ok := delta["tool_calls"].([]any); ok {
			if c.correctToolCallsArray(toolCalls) {
				corrected = true
		REDACTED
	REDACTED
		if functionCall, ok := delta["function_call"].(map[string]any); ok {
			if c.correctFunctionCall(functionCall) {
				corrected = true
		REDACTED
	REDACTED
REDACTED

	// 处理 choices[].message.tool_calls 和 choices[].delta.tool_calls
	if choices, ok := payload["choices"].([]any); ok {
		for _, choice := range choices {
			if choiceMap, ok := choice.(map[string]any); ok {
				// 处理 message 中的工具调用
				if message, ok := choiceMap["message"].(map[string]any); ok {
					if toolCalls, ok := message["tool_calls"].([]any); ok {
						if c.correctToolCallsArray(toolCalls) {
							corrected = true
					REDACTED
				REDACTED
					if functionCall, ok := message["function_call"].(map[string]any); ok {
						if c.correctFunctionCall(functionCall) {
							corrected = true
					REDACTED
				REDACTED
			REDACTED
				// 处理 delta 中的工具调用
				if delta, ok := choiceMap["delta"].(map[string]any); ok {
					if toolCalls, ok := delta["tool_calls"].([]any); ok {
						if c.correctToolCallsArray(toolCalls) {
							corrected = true
					REDACTED
				REDACTED
					if functionCall, ok := delta["function_call"].(map[string]any); ok {
						if c.correctFunctionCall(functionCall) {
							corrected = true
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	if !corrected {
		return data, false
REDACTED

	// 序列化回 JSON
	correctedBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[CodexToolCorrector] Failed to marshal corrected data: %v", err)
		return data, false
REDACTED

	return string(correctedBytes), true
REDACTED

// correctToolCallsArray 修正工具调用数组中的工具名称
func (c *CodexToolCorrector) correctToolCallsArray(toolCalls []any) bool {
	corrected := false
	for _, toolCall := range toolCalls {
		if toolCallMap, ok := toolCall.(map[string]any); ok {
			if function, ok := toolCallMap["function"].(map[string]any); ok {
				if c.correctFunctionCall(function) {
					corrected = true
			REDACTED
		REDACTED
	REDACTED
REDACTED
	return corrected
REDACTED

// correctFunctionCall 修正单个函数调用的工具名称和参数
func (c *CodexToolCorrector) correctFunctionCall(functionCall map[string]any) bool {
	name, ok := functionCall["name"].(string)
	if !ok || name == "" {
		return false
REDACTED

	corrected := false

	// 查找并修正工具名称
	if correctName, found := codexToolNameMapping[name]; found {
		functionCall["name"] = correctName
		c.recordCorrection(name, correctName)
		corrected = true
		name = correctName // 使用修正后的名称进行参数修正
REDACTED

	// 修正工具参数（基于工具名称）
	if c.correctToolParameters(name, functionCall) {
		corrected = true
REDACTED

	return corrected
REDACTED

// correctToolParameters 修正工具参数以符合 OpenCode 规范
func (c *CodexToolCorrector) correctToolParameters(toolName string, functionCall map[string]any) bool {
	arguments, ok := functionCall["arguments"]
	if !ok {
		return false
REDACTED

	// arguments 可能是字符串（JSON）或已解析的 map
	var argsMap map[string]any
	switch v := arguments.(type) {
	case string:
		// 解析 JSON 字符串
		if err := json.Unmarshal([]byte(v), &argsMap); err != nil {
			return false
	REDACTED
	case map[string]any:
		argsMap = v
	default:
		return false
REDACTED

	corrected := false

	// 根据工具名称应用特定的参数修正规则
	switch toolName {
	case "bash":
		// 移除 workdir 参数（OpenCode 不支持）
		if _, exists := argsMap["workdir"]; exists {
			delete(argsMap, "workdir")
			corrected = true
			log.Printf("[CodexToolCorrector] Removed 'workdir' parameter from bash tool")
	REDACTED
		if _, exists := argsMap["work_dir"]; exists {
			delete(argsMap, "work_dir")
			corrected = true
			log.Printf("[CodexToolCorrector] Removed 'work_dir' parameter from bash tool")
	REDACTED

	case "edit":
		// OpenCode edit 使用 old_string/new_string，Codex 可能使用其他名称
		// 这里可以添加参数名称的映射逻辑
		if _, exists := argsMap["file_path"]; !exists {
			if path, exists := argsMap["path"]; exists {
				argsMap["file_path"] = path
				delete(argsMap, "path")
				corrected = true
				log.Printf("[CodexToolCorrector] Renamed 'path' to 'file_path' in edit tool")
		REDACTED
	REDACTED
REDACTED

	// 如果修正了参数，需要重新序列化
	if corrected {
		if _, wasString := arguments.(string); wasString {
			// 原本是字符串，序列化回字符串
			if newArgsJSON, err := json.Marshal(argsMap); err == nil {
				functionCall["arguments"] = string(newArgsJSON)
		REDACTED
	REDACTED else {
			// 原本是 map，直接赋值
			functionCall["arguments"] = argsMap
	REDACTED
REDACTED

	return corrected
REDACTED

// recordCorrection 记录一次工具名称修正
func (c *CodexToolCorrector) recordCorrection(from, to string) {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()

	c.stats.TotalCorrected++
	key := fmt.Sprintf("%s->%s", from, to)
	c.stats.CorrectionsByTool[key]++

	log.Printf("[CodexToolCorrector] Corrected tool call: %s -> %s (total: %d)",
		from, to, c.stats.TotalCorrected)
REDACTED

// GetStats 获取工具修正统计信息
func (c *CodexToolCorrector) GetStats() ToolCorrectionStats {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()

	// 返回副本以避免并发问题
	statsCopy := ToolCorrectionStats{
		TotalCorrected:    c.stats.TotalCorrected,
		CorrectionsByTool: make(map[string]int, len(c.stats.CorrectionsByTool)),
REDACTED
	for k, v := range c.stats.CorrectionsByTool {
		statsCopy.CorrectionsByTool[k] = v
REDACTED

	return statsCopy
REDACTED

// ResetStats 重置统计信息
func (c *CodexToolCorrector) ResetStats() {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()

	c.stats.TotalCorrected = 0
	c.stats.CorrectionsByTool = make(map[string]int)
REDACTED

// CorrectToolName 直接修正工具名称（用于非 SSE 场景）
func CorrectToolName(name string) (string, bool) {
	if correctName, found := codexToolNameMapping[name]; found {
		return correctName, true
REDACTED
	return name, false
REDACTED

// GetToolNameMapping 获取工具名称映射表
func GetToolNameMapping() map[string]string {
	// 返回副本以避免外部修改
	mapping := make(map[string]string, len(codexToolNameMapping))
	for k, v := range codexToolNameMapping {
		mapping[k] = v
REDACTED
	return mapping
REDACTED
