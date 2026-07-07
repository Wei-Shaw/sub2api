package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// ToolContinuationSignals 聚合工具续链相关信号，避免重复遍历 input。
type ToolContinuationSignals struct {
	HasFunctionCallOutput              bool
	HasFunctionCallOutputMissingCallID bool
	HasToolCallContext                 bool
	HasItemReference                   bool
	HasItemReferenceForAllCallIDs      bool
	FunctionCallOutputCallIDs          []string
REDACTED

// FunctionCallOutputValidation 汇总 function_call_output 关联性校验结果。
type FunctionCallOutputValidation struct {
	HasFunctionCallOutput              bool
	HasToolCallContext                 bool
	HasFunctionCallOutputMissingCallID bool
	HasItemReferenceForAllCallIDs      bool
REDACTED

func isCodexToolCallContextItemType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "tool_call",
		"function_call",
		"local_shell_call",
		"tool_search_call",
		"custom_tool_call",
		"mcp_tool_call":
		return true
	default:
		return false
REDACTED
REDACTED

func isCodexToolCallOutputItemType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "function_call_output",
		"tool_search_output",
		"custom_tool_call_output",
		"mcp_tool_call_output":
		return true
	default:
		return false
REDACTED
REDACTED

// NeedsToolContinuation 判定请求是否需要工具调用续链处理。
// 满足以下任一信号即视为续链：previous_response_id、input 内包含工具输出/item_reference、
// 或显式声明 tools/tool_choice。
func NeedsToolContinuation(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
REDACTED
	if hasNonEmptyString(reqBody["previous_response_id"]) {
		return true
REDACTED
	if hasToolsSignal(reqBody) {
		return true
REDACTED
	if hasToolChoiceSignal(reqBody) {
		return true
REDACTED
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
REDACTED
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType, _ := itemMap["type"].(string)
		if isCodexToolCallItemType(itemType) || itemType == "item_reference" {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// AnalyzeToolContinuationSignals 单次遍历 input，提取工具输出/工具调用上下文/item_reference 相关信号。
// 字段名保留 FunctionCallOutput 是为了兼容既有调用点；语义覆盖 Codex 的所有工具输出
// （function_call_output/tool_search_output/custom_tool_call_output/mcp_tool_call_output）。
func AnalyzeToolContinuationSignals(reqBody map[string]any) ToolContinuationSignals {
	signals := ToolContinuationSignals{REDACTED
	if reqBody == nil {
		return signals
REDACTED
	input, ok := reqBody["input"].([]any)
	if !ok {
		return signals
REDACTED

	var callIDs map[string]struct{REDACTED
	var referenceIDs map[string]struct{REDACTED

	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType, _ := itemMap["type"].(string)
		switch {
		case isCodexToolCallContextItemType(itemType):
			callID, _ := itemMap["call_id"].(string)
			if strings.TrimSpace(callID) != "" {
				signals.HasToolCallContext = true
		REDACTED
		case isCodexToolCallOutputItemType(itemType):
			signals.HasFunctionCallOutput = true
			callID, _ := itemMap["call_id"].(string)
			callID = strings.TrimSpace(callID)
			if callID == "" {
				signals.HasFunctionCallOutputMissingCallID = true
				continue
		REDACTED
			if callIDs == nil {
				callIDs = make(map[string]struct{REDACTED)
		REDACTED
			callIDs[callID] = struct{REDACTED{REDACTED
		case itemType == "item_reference":
			signals.HasItemReference = true
			idValue, _ := itemMap["id"].(string)
			idValue = strings.TrimSpace(idValue)
			if idValue == "" {
				continue
		REDACTED
			if referenceIDs == nil {
				referenceIDs = make(map[string]struct{REDACTED)
		REDACTED
			referenceIDs[idValue] = struct{REDACTED{REDACTED
	REDACTED
REDACTED

	if len(callIDs) == 0 {
		return signals
REDACTED
	signals.FunctionCallOutputCallIDs = make([]string, 0, len(callIDs))
	allReferenced := len(referenceIDs) > 0
	for callID := range callIDs {
		signals.FunctionCallOutputCallIDs = append(signals.FunctionCallOutputCallIDs, callID)
		if allReferenced {
			if _, ok := referenceIDs[callID]; !ok {
				allReferenced = false
		REDACTED
	REDACTED
REDACTED
	signals.HasItemReferenceForAllCallIDs = allReferenced
	return signals
REDACTED

// ValidateFunctionCallOutputContextBytes 基于 raw JSON 校验工具输出续链，避免 handler 预校验阶段全量解码大 input。
func ValidateFunctionCallOutputContextBytes(body []byte) FunctionCallOutputValidation {
	result := FunctionCallOutputValidation{REDACTED
	if len(body) == 0 {
		return result
REDACTED
	// handler 热路径只读扫描 input，避免 GetBytes 为大 Responses body 复制整段 JSON。
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() {
		return result
REDACTED

	var callIDs map[string]struct{REDACTED
	var referenceIDs map[string]struct{REDACTED
	input.ForEach(func(_, item gjson.Result) bool {
		if !item.IsObject() {
			return true
	REDACTED
		itemType := item.Get("type").String()
		switch {
		case isCodexToolCallOutputItemType(itemType):
			result.HasFunctionCallOutput = true
			callID := strings.TrimSpace(item.Get("call_id").String())
			if callID == "" {
				result.HasFunctionCallOutputMissingCallID = true
				return true
		REDACTED
			if callIDs == nil {
				callIDs = make(map[string]struct{REDACTED)
		REDACTED
			callIDs[callID] = struct{REDACTED{REDACTED
		case isCodexToolCallContextItemType(itemType):
			if strings.TrimSpace(item.Get("call_id").String()) != "" {
				result.HasToolCallContext = true
		REDACTED
		case itemType == "item_reference":
			idValue := strings.TrimSpace(item.Get("id").String())
			if idValue == "" {
				return true
		REDACTED
			if referenceIDs == nil {
				referenceIDs = make(map[string]struct{REDACTED)
		REDACTED
			referenceIDs[idValue] = struct{REDACTED{REDACTED
	REDACTED
		return !result.HasFunctionCallOutput || !result.HasToolCallContext
REDACTED)
	if !result.HasFunctionCallOutput || result.HasToolCallContext || len(callIDs) == 0 || len(referenceIDs) == 0 {
		return result
REDACTED
	allReferenced := true
	for callID := range callIDs {
		if _, ok := referenceIDs[callID]; !ok {
			allReferenced = false
			break
	REDACTED
REDACTED
	result.HasItemReferenceForAllCallIDs = allReferenced
	return result
REDACTED

// ToolCallOutputContextCoverage 描述 input 中工具输出与可重建上下文的覆盖关系，
// 用于判断剥离 previous_response_id 后上游能否仅凭 input 重建工具续链。
type ToolCallOutputContextCoverage struct {
	HasFunctionCallOutput bool
	// ContextCoversAllCallIDs 表示每个工具输出的 call_id 都能在 input 内找到
	// 同 call_id 的工具调用上下文项或同 id 的 item_reference，且不存在缺失 call_id 的输出。
	// 任一输出无法由 input 自身重建时为 false，此时剥离 previous_response_id 会导致
	// 上游以 "No tool call found for function call output" 拒绝请求。
	ContextCoversAllCallIDs bool
REDACTED

// AnalyzeToolCallOutputContextCoverageBytes 全量扫描 input，按 call_id 精确匹配工具输出
// 与可重建上下文。不能复用 ValidateFunctionCallOutputContextBytes 的 HasToolCallContext：
// 该标志只代表"存在某一个上下文项"，部分覆盖的续链仍会被上游拒绝。
func AnalyzeToolCallOutputContextCoverageBytes(body []byte) ToolCallOutputContextCoverage {
	coverage := ToolCallOutputContextCoverage{REDACTED
	if len(body) == 0 {
		return coverage
REDACTED
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() {
		return coverage
REDACTED

	missingCallID := false
	var outputCallIDs map[string]struct{REDACTED
	var contextIDs map[string]struct{REDACTED
	input.ForEach(func(_, item gjson.Result) bool {
		if !item.IsObject() {
			return true
	REDACTED
		itemType := item.Get("type").String()
		switch {
		case isCodexToolCallOutputItemType(itemType):
			coverage.HasFunctionCallOutput = true
			callID := strings.TrimSpace(item.Get("call_id").String())
			if callID == "" {
				missingCallID = true
				return true
		REDACTED
			if outputCallIDs == nil {
				outputCallIDs = make(map[string]struct{REDACTED)
		REDACTED
			outputCallIDs[callID] = struct{REDACTED{REDACTED
		case isCodexToolCallContextItemType(itemType):
			callID := strings.TrimSpace(item.Get("call_id").String())
			if callID == "" {
				return true
		REDACTED
			if contextIDs == nil {
				contextIDs = make(map[string]struct{REDACTED)
		REDACTED
			contextIDs[callID] = struct{REDACTED{REDACTED
		case itemType == "item_reference":
			idValue := strings.TrimSpace(item.Get("id").String())
			if idValue == "" {
				return true
		REDACTED
			if contextIDs == nil {
				contextIDs = make(map[string]struct{REDACTED)
		REDACTED
			contextIDs[idValue] = struct{REDACTED{REDACTED
	REDACTED
		return true
REDACTED)

	if !coverage.HasFunctionCallOutput || missingCallID {
		return coverage
REDACTED
	for callID := range outputCallIDs {
		if _, ok := contextIDs[callID]; !ok {
			return coverage
	REDACTED
REDACTED
	coverage.ContextCoversAllCallIDs = true
	return coverage
REDACTED

// ValidateFunctionCallOutputContext 为 handler 提供低开销校验结果：
// 1) 无工具输出直接返回
// 2) 若已存在工具调用上下文则提前返回
// 3) 仅在无工具上下文时才构建 call_id / item_reference 集合
// 字段名保留 FunctionCallOutput 是为了兼容既有调用点；语义覆盖所有 Codex 工具输出。
func ValidateFunctionCallOutputContext(reqBody map[string]any) FunctionCallOutputValidation {
	result := FunctionCallOutputValidation{REDACTED
	if reqBody == nil {
		return result
REDACTED
	input, ok := reqBody["input"].([]any)
	if !ok {
		return result
REDACTED

	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType, _ := itemMap["type"].(string)
		switch {
		case isCodexToolCallOutputItemType(itemType):
			result.HasFunctionCallOutput = true
		case isCodexToolCallContextItemType(itemType):
			callID, _ := itemMap["call_id"].(string)
			if strings.TrimSpace(callID) != "" {
				result.HasToolCallContext = true
		REDACTED
	REDACTED
		if result.HasFunctionCallOutput && result.HasToolCallContext {
			return result
	REDACTED
REDACTED

	if !result.HasFunctionCallOutput || result.HasToolCallContext {
		return result
REDACTED

	callIDs := make(map[string]struct{REDACTED)
	referenceIDs := make(map[string]struct{REDACTED)
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType, _ := itemMap["type"].(string)
		switch {
		case isCodexToolCallOutputItemType(itemType):
			callID, _ := itemMap["call_id"].(string)
			callID = strings.TrimSpace(callID)
			if callID == "" {
				result.HasFunctionCallOutputMissingCallID = true
				continue
		REDACTED
			callIDs[callID] = struct{REDACTED{REDACTED
		case itemType == "item_reference":
			idValue, _ := itemMap["id"].(string)
			idValue = strings.TrimSpace(idValue)
			if idValue == "" {
				continue
		REDACTED
			referenceIDs[idValue] = struct{REDACTED{REDACTED
	REDACTED
REDACTED

	if len(callIDs) == 0 || len(referenceIDs) == 0 {
		return result
REDACTED
	allReferenced := true
	for callID := range callIDs {
		if _, ok := referenceIDs[callID]; !ok {
			allReferenced = false
			break
	REDACTED
REDACTED
	result.HasItemReferenceForAllCallIDs = allReferenced
	return result
REDACTED

// HasFunctionCallOutput 判断 input 是否包含任意 Codex 工具输出，用于触发续链校验。
// 名称保留 function_call_output 是为了兼容既有调用点。
func HasFunctionCallOutput(reqBody map[string]any) bool {
	return AnalyzeToolContinuationSignals(reqBody).HasFunctionCallOutput
REDACTED

// HasToolCallContext 判断 input 是否包含带 call_id 的工具调用上下文，
// 用于判断工具输出是否具备可关联的上下文。
func HasToolCallContext(reqBody map[string]any) bool {
	return AnalyzeToolContinuationSignals(reqBody).HasToolCallContext
REDACTED

// FunctionCallOutputCallIDs 提取 input 中工具输出的 call_id 集合。
// 仅返回非空 call_id，用于与 item_reference.id 做匹配校验。
func FunctionCallOutputCallIDs(reqBody map[string]any) []string {
	return AnalyzeToolContinuationSignals(reqBody).FunctionCallOutputCallIDs
REDACTED

// HasFunctionCallOutputMissingCallID 判断是否存在缺少 call_id 的工具输出。
func HasFunctionCallOutputMissingCallID(reqBody map[string]any) bool {
	return AnalyzeToolContinuationSignals(reqBody).HasFunctionCallOutputMissingCallID
REDACTED

// HasItemReferenceForCallIDs 判断 item_reference.id 是否覆盖所有 call_id。
// 用于仅依赖引用项完成续链场景的校验。
func HasItemReferenceForCallIDs(reqBody map[string]any, callIDs []string) bool {
	if reqBody == nil || len(callIDs) == 0 {
		return false
REDACTED
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
REDACTED
	referenceIDs := make(map[string]struct{REDACTED)
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType, _ := itemMap["type"].(string)
		if itemType != "item_reference" {
			continue
	REDACTED
		idValue, _ := itemMap["id"].(string)
		idValue = strings.TrimSpace(idValue)
		if idValue == "" {
			continue
	REDACTED
		referenceIDs[idValue] = struct{REDACTED{REDACTED
REDACTED
	if len(referenceIDs) == 0 {
		return false
REDACTED
	for _, callID := range callIDs {
		if _, ok := referenceIDs[strings.TrimSpace(callID)]; !ok {
			return false
	REDACTED
REDACTED
	return true
REDACTED

// hasNonEmptyString 判断字段是否为非空字符串。
func hasNonEmptyString(value any) bool {
	stringValue, ok := value.(string)
	return ok && strings.TrimSpace(stringValue) != ""
REDACTED

// hasToolsSignal 判断 tools 字段是否显式声明（存在且不为空）。
func hasToolsSignal(reqBody map[string]any) bool {
	raw, exists := reqBody["tools"]
	if !exists || raw == nil {
		return false
REDACTED
	if tools, ok := raw.([]any); ok {
		return len(tools) > 0
REDACTED
	return false
REDACTED

// hasToolChoiceSignal 判断 tool_choice 是否显式声明（非空或非 nil）。
func hasToolChoiceSignal(reqBody map[string]any) bool {
	raw, exists := reqBody["tool_choice"]
	if !exists || raw == nil {
		return false
REDACTED
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value) != ""
	case map[string]any:
		return len(value) > 0
	default:
		return false
REDACTED
REDACTED
