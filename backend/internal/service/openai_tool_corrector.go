package service

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/Usersxiao/ai2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
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

	// Some clients output generic fetch names.
	"fetch":     "webfetch",
	"web_fetch": "webfetch",
	"webFetch":  "webfetch",
}

// ToolCorrectionStats 记录工具修正的统计信息（导出用于 JSON 序列化）
type ToolCorrectionStats struct {
	TotalCorrected    int            `json:"total_corrected"`
	CorrectionsByTool map[string]int `json:"corrections_by_tool"`
}

// CodexToolCorrector 处理 Codex 工具调用的自动修正
type CodexToolCorrector struct {
	stats ToolCorrectionStats
	mu    sync.RWMutex
}

// NewCodexToolCorrector 创建新的工具修正器
func NewCodexToolCorrector() *CodexToolCorrector {
	return &CodexToolCorrector{
		stats: ToolCorrectionStats{
			CorrectionsByTool: make(map[string]int),
		},
	}
}

// CorrectToolCallsInSSEData 修正 SSE 数据中的工具调用
// 返回修正后的数据和是否进行了修正
func (c *CodexToolCorrector) CorrectToolCallsInSSEData(data string) (string, bool) {
	if data == "" || data == "\n" {
		return data, false
	}
	correctedBytes, corrected := c.CorrectToolCallsInSSEBytes([]byte(data))
	if !corrected {
		return data, false
	}
	return string(correctedBytes), true
}

// CorrectToolCallsInSSEBytes 修正 SSE JSON 数据中的工具调用（字节路径）。
// 返回修正后的数据和是否进行了修正。
func (c *CodexToolCorrector) CorrectToolCallsInSSEBytes(data []byte) ([]byte, bool) {
	if len(bytes.TrimSpace(data)) == 0 {
		return data, false
	}
	if !mayContainToolCallPayload(data) {
		return data, false
	}
	if !gjson.ValidBytes(data) {
		// 不是有效 JSON，直接返回原数据
		return data, false
	}

	updated := data
	corrected := false
	collect := func(changed bool, next []byte) {
		if changed {
			corrected = true
			updated = next
		}
	}

	if next, changed := c.correctToolCallsArrayAtPath(updated, "tool_calls"); changed {
		collect(changed, next)
	}
	if next, changed := c.correctFunctionAtPath(updated, "function_call"); changed {
		collect(changed, next)
	}
	if next, changed := c.correctToolCallsArrayAtPath(updated, "delta.tool_calls"); changed {
		collect(changed, next)
	}
	if next, changed := c.correctFunctionAtPath(updated, "delta.function_call"); changed {
		collect(changed, next)
	}

	choicesCount := int(gjson.GetBytes(updated, "choices.#").Int())
	for i := 0; i < choicesCount; i++ {
		prefix := "choices." + strconv.Itoa(i)
		if next, changed := c.correctToolCallsArrayAtPath(updated, prefix+".message.tool_calls"); changed {
			collect(changed, next)
		}
		if next, changed := c.correctFunctionAtPath(updated, prefix+".message.function_call"); changed {
			collect(changed, next)
		}
		if next, changed := c.correctToolCallsArrayAtPath(updated, prefix+".delta.tool_calls"); changed {
			collect(changed, next)
		}
		if next, changed := c.correctFunctionAtPath(updated, prefix+".delta.function_call"); changed {
			collect(changed, next)
		}
	}

	if !corrected {
		return data, false
	}
	return updated, true
}

func mayContainToolCallPayload(data []byte) bool {
	// 快速路径：多数 token / 文本事件不包含工具字段，避免进入 JSON 解析热路径。
	return bytes.Contains(data, []byte(`"tool_calls"`)) ||
		bytes.Contains(data, []byte(`"function_call"`)) ||
		bytes.Contains(data, []byte(`"function":{"name"`))
}

// correctToolCallsArrayAtPath 修正指定路径下 tool_calls 数组中的工具名称。
func (c *CodexToolCorrector) correctToolCallsArrayAtPath(data []byte, toolCallsPath string) ([]byte, bool) {
	count := int(gjson.GetBytes(data, toolCallsPath+".#").Int())
	if count <= 0 {
		return data, false
	}
	updated := data
	corrected := false
	for i := 0; i < count; i++ {
		functionPath := toolCallsPath + "." + strconv.Itoa(i) + ".function"
		if next, changed := c.correctFunctionAtPath(updated, functionPath); changed {
			updated = next
			corrected = true
		}
	}
	return updated, corrected
}

// correctFunctionAtPath 修正指定路径下单个函数调用的工具名称和参数。
func (c *CodexToolCorrector) correctFunctionAtPath(data []byte, functionPath string) ([]byte, bool) {
	namePath := functionPath + ".name"
	nameResult := gjson.GetBytes(data, namePath)
	if !nameResult.Exists() || nameResult.Type != gjson.String {
		return data, false
	}
	name := strings.TrimSpace(nameResult.Str)
	if name == "" {
		return data, false
	}
	updated := data
	corrected := false

	// 查找并修正工具名称
	if correctName, found := codexToolNameMapping[name]; found {
		if next, err := sjson.SetBytes(updated, namePath, correctName); err == nil {
			updated = next
			c.recordCorrection(name, correctName)
			corrected = true
			name = correctName // 使用修正后的名称进行参数修正
		}
	}

	// 修正工具参数（基于工具名称）
	if next, changed := c.correctToolParametersAtPath(updated, functionPath+".arguments", name); changed {
		updated = next
		corrected = true
	}
	return updated, corrected
}

// correctToolParametersAtPath 修正指定路径下 arguments 参数。
func (c *CodexToolCorrector) correctToolParametersAtPath(data []byte, argumentsPath, toolName string) ([]byte, bool) {
	if toolName != "bash" && toolName != "edit" {
		return data, false
	}

	args := gjson.GetBytes(data, argumentsPath)
	if !args.Exists() {
		return data, false
	}

	switch args.Type {
	case gjson.String:
		argsJSON := strings.TrimSpace(args.Str)
		if !gjson.Valid(argsJSON) {
			return data, false
		}
		if !gjson.Parse(argsJSON).IsObject() {
			return data, false
		}
		nextArgsJSON, corrected := c.correctToolArgumentsJSON(argsJSON, toolName)
		if !corrected {
			return data, false
		}
		next, err := sjson.SetBytes(data, argumentsPath, nextArgsJSON)
		if err != nil {
			return data, false
		}
		return next, true
	case gjson.JSON:
		if !args.IsObject() || !gjson.Valid(args.Raw) {
			return data, false
		}
		nextArgsJSON, corrected := c.correctToolArgumentsJSON(args.Raw, toolName)
		if !corrected {
			return data, false
		}
		next, err := sjson.SetRawBytes(data, argumentsPath, []byte(nextArgsJSON))
		if err != nil {
			return data, false
		}
		return next, true
	default:
		return data, false
	}
}

// correctToolArgumentsJSON 修正工具参数 JSON（对象字符串），返回修正后的 JSON 与是否变更。
func (c *CodexToolCorrector) correctToolArgumentsJSON(argsJSON, toolName string) (string, bool) {
	if !gjson.Valid(argsJSON) {
		return argsJSON, false
	}
	if !gjson.Parse(argsJSON).IsObject() {
		return argsJSON, false
	}

	updated := argsJSON
	corrected := false

	// 根据工具名称应用特定的参数修正规则
	switch toolName {
	case "bash":
		// OpenCode bash 支持 workdir；有些来源会输出 work_dir。
		if !gjson.Get(updated, "workdir").Exists() {
			if next, changed := moveJSONField(updated, "work_dir", "workdir"); changed {
				updated = next
				corrected = true
				logger.LegacyPrintf("service.openai_tool_corrector", "[CodexToolCorrector] Renamed 'work_dir' to 'workdir' in bash tool")
			}
		} else {
			if next, changed := deleteJSONField(updated, "work_dir"); changed {
				updated = next
				corrected = true
				logger.LegacyPrintf("service.openai_tool_corrector", "[CodexToolCorrector] Removed duplicate 'work_dir' parameter from bash tool")
			}
		}

	case "edit":
		// OpenCode edit 参数为 filePath/oldString/newString（camelCase）。
		if !gjson.Get(updated, "filePath").Exists() {
			if next, changed := moveJSONField(updated, "file_path", "filePath"); changed {
				updated = next
				corrected = true
				logger.LegacyPrintf("service.openai_tool_corrector", "[CodexToolCorrector] Renamed 'file_path' to 'filePath' in edit tool")
			} else if next, changed := moveJSONField(updated, "path", "filePath"); changed {
				updated = next
				corrected = true
				logger.LegacyPrintf("service.openai_tool_corrector", "[CodexToolCorrector] Renamed 'path' to 'filePath' in edit tool")
			} else if next, changed := moveJSONField(updated, "file", "filePath"); changed {
				updated = next
				corrected = true
				logger.LegacyPrintf("service.openai_tool_corrector", "[CodexToolCorrector] Renamed 'file' to 'filePath' in edit tool")
			}
		}

		if next, changed := moveJSONField(updated, "old_string", "oldString"); changed {
			updated = next
			corrected = true
			logger.LegacyPrintf("service.openai_tool_corrector", "[CodexToolCorrector] Renamed 'old_string' to 'oldString' in edit tool")
		}

		if next, changed := moveJSONField(updated, "new_string", "newString"); changed {
			updated = next
			corrected = true
			logger.LegacyPrintf("service.openai_tool_corrector", "[CodexToolCorrector] Renamed 'new_string' to 'newString' in edit tool")
		}

		if next, changed := moveJSONField(updated, "replace_all", "replaceAll"); changed {
			updated = next
			corrected = true
			logger.LegacyPrintf("service.openai_tool_corrector", "[CodexToolCorrector] Renamed 'replace_all' to 'replaceAll' in edit tool")
		}
	}
	return updated, corrected
}

func moveJSONField(input, from, to string) (string, bool) {
	if gjson.Get(input, to).Exists() {
		return input, false
	}
	src := gjson.Get(input, from)
	if !src.Exists() {
		return input, false
	}
	next, err := sjson.SetRaw(input, to, src.Raw)
	if err != nil {
		return input, false
	}
	next, err = sjson.Delete(next, from)
	if err != nil {
		return input, false
	}
	return next, true
}

func deleteJSONField(input, path string) (string, bool) {
	if !gjson.Get(input, path).Exists() {
		return input, false
	}
	next, err := sjson.Delete(input, path)
	if err != nil {
		return input, false
	}
	return next, true
}

// recordCorrection 记录一次工具名称修正
func (c *CodexToolCorrector) recordCorrection(from, to string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stats.TotalCorrected++
	key := fmt.Sprintf("%s->%s", from, to)
	c.stats.CorrectionsByTool[key]++

	logger.LegacyPrintf("service.openai_tool_corrector", "[CodexToolCorrector] Corrected tool call: %s -> %s (total: %d)",
		from, to, c.stats.TotalCorrected)
}

// GetStats 获取工具修正统计信息
func (c *CodexToolCorrector) GetStats() ToolCorrectionStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 返回副本以避免并发问题
	statsCopy := ToolCorrectionStats{
		TotalCorrected:    c.stats.TotalCorrected,
		CorrectionsByTool: make(map[string]int, len(c.stats.CorrectionsByTool)),
	}
	for k, v := range c.stats.CorrectionsByTool {
		statsCopy.CorrectionsByTool[k] = v
	}

	return statsCopy
}

// ResetStats 重置统计信息
func (c *CodexToolCorrector) ResetStats() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stats.TotalCorrected = 0
	c.stats.CorrectionsByTool = make(map[string]int)
}

// CorrectToolName 直接修正工具名称（用于非 SSE 场景）
func CorrectToolName(name string) (string, bool) {
	if correctName, found := codexToolNameMapping[name]; found {
		return correctName, true
	}
	return name, false
}

// GetToolNameMapping 获取工具名称映射表
func GetToolNameMapping() map[string]string {
	// 返回副本以避免外部修改
	mapping := make(map[string]string, len(codexToolNameMapping))
	for k, v := range codexToolNameMapping {
		mapping[k] = v
	}
	return mapping
}
