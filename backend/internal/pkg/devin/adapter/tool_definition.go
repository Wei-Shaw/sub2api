// 本文件把中间工具定义转换为 Devin 原生函数工具，并把被屏蔽的工具说明注入系统提示词。
package adapter

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

var descriptionListItemPattern = regexp.MustCompile(`^(?:[-*+]\s+|\d+[.):]\s+|\[\d+\]\s+)(.+)$`)

// withToolDescriptions 把非空工具说明追加到 Devin system prompt，供模型理解原生工具用途。
func withToolDescriptions(systemPrompt string, tools []llm.ToolDefinition) string {
	var section strings.Builder
	for _, tool := range tools {
		description := strings.TrimSpace(tool.Description)
		if description == "" {
			continue
		}
		if section.Len() == 0 {
			section.WriteString("# tools descriptions")
		}
		section.WriteString("\n<tool name=\"")
		section.WriteString(escapeXMLAttribute(tool.Name))
		section.WriteString("\">\n")
		section.WriteString(escapeXMLText(formatToolDescription(description)))
		section.WriteString("\n</tool>")
	}
	if section.Len() == 0 {
		return systemPrompt
	}
	trimmedPrompt := strings.TrimRight(systemPrompt, "\r\n")
	if strings.TrimSpace(trimmedPrompt) == "" {
		return section.String()
	}
	return trimmedPrompt + "\n\n" + section.String()
}

// formatToolDescription 把自然语言句子改为有序条目，并保留代码块和 JSON 示例的原有结构。
func formatToolDescription(description string) string {
	description = strings.ReplaceAll(description, "\r\n", "\n")
	description = strings.ReplaceAll(description, "\r", "\n")
	lines := strings.Split(description, "\n")
	output := make([]string, 0, len(lines))
	prose := make([]string, 0, len(lines))
	itemNumber := 1
	inCodeFence := false

	appendSentences := func(value string) {
		for _, sentence := range splitDescriptionSentences(value) {
			output = append(output, fmt.Sprintf("%d. %s", itemNumber, sentence))
			itemNumber++
		}
	}
	appendBlankLine := func() {
		if len(output) > 0 && output[len(output)-1] != "" {
			output = append(output, "")
		}
	}
	flushProse := func() {
		if len(prose) == 0 {
			return
		}
		paragraph := strings.TrimSpace(strings.Join(prose, "\n"))
		prose = prose[:0]
		if paragraph == "" {
			return
		}
		if json.Valid([]byte(paragraph)) {
			output = append(output, paragraph)
			return
		}
		appendSentences(paragraph)
	}

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "```") || strings.HasPrefix(trimmedLine, "~~~") {
			flushProse()
			output = append(output, line)
			inCodeFence = !inCodeFence
			continue
		}
		if inCodeFence {
			output = append(output, line)
			continue
		}
		if trimmedLine == "" {
			flushProse()
			appendBlankLine()
			continue
		}
		if listItem := descriptionListItemPattern.FindStringSubmatch(trimmedLine); listItem != nil {
			flushProse()
			appendSentences(strings.TrimSpace(listItem[1]))
			continue
		}
		prose = append(prose, line)
	}
	flushProse()
	return strings.TrimSpace(strings.Join(output, "\n"))
}

func splitDescriptionSentences(paragraph string) []string {
	sentences := make([]string, 0, 1)
	start := 0
	for offset := 0; offset < len(paragraph); {
		character, size := utf8.DecodeRuneInString(paragraph[offset:])
		end := offset + size
		if isSentenceTerminator(character) {
			next := end
			for next < len(paragraph) {
				nextCharacter, nextSize := utf8.DecodeRuneInString(paragraph[next:])
				if !unicode.IsSpace(nextCharacter) {
					break
				}
				next += nextSize
			}
			hasSentenceBoundary := next > end || character == '。' || character == '！' || character == '？'
			if hasSentenceBoundary && next < len(paragraph) && !endsWithAbbreviation(paragraph[start:end]) {
				sentences = append(sentences, strings.TrimSpace(paragraph[start:end]))
				start = next
				offset = next
				continue
			}
		}
		offset = end
	}
	if remaining := strings.TrimSpace(paragraph[start:]); remaining != "" {
		sentences = append(sentences, remaining)
	}
	return sentences
}

func isSentenceTerminator(character rune) bool {
	switch character {
	case '.', '!', '?', '。', '！', '？':
		return true
	default:
		return false
	}
}

func endsWithAbbreviation(fragment string) bool {
	fields := strings.Fields(strings.ToLower(fragment))
	if len(fields) == 0 {
		return false
	}
	word := strings.Trim(fields[len(fields)-1], "\"'()[]{}")
	switch word {
	case "e.g.", "i.e.", "etc.", "vs.", "mr.", "mrs.", "dr.", "prof.", "no.":
		return true
	default:
		return false
	}
}

func escapeXMLAttribute(value string) string {
	var escaped strings.Builder
	_ = xml.EscapeText(&escaped, []byte(value))
	return escaped.String()
}

// escapeXMLText 仅处理会破坏 XML 文本边界的字符，保留代码示例中的普通引号。
func escapeXMLText(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(value)
}

// toolDefinitionCache 以 (name, schema) 缓存 convertToolDefinition 的产物：
// 同一客户端的工具集逐请求原样重发，strip+normalize 双程是纯重复劳动。
// 命中返回同一值——调用方只 marshal 不修改，共享安全。
// 条目数超上限时整体清空重建，避免无界增长。
var toolDefinitionCache = struct {
	sync.Mutex
	items map[string]devin.ChatToolDef
}{items: make(map[string]devin.ChatToolDef)}

// convertToolDefinition 保留工具身份和 JSON Schema 约束，仅移除自然语言注释。
// 工具名先做本地校验：上游实测只接受 [A-Za-z0-9_-]（mcp__a__b 合法，
// a.b / mcp::x / CJK 全部 invalid_argument: an internal error occurred），
// 提前报成可读的 invalid_argument，比上游的模糊文案可排障。
// 不做静默改名——改写会让客户端历史回灌的 tool_call 名对不上。
func convertToolDefinition(tool llm.ToolDefinition) (devin.ChatToolDef, error) {
	if !validToolName(tool.Name) {
		return devin.ChatToolDef{}, fmt.Errorf("invalid_argument: tool name %q contains characters outside [A-Za-z0-9_-], which the upstream rejects", tool.Name)
	}
	cacheKey := tool.Name + "\x00" + string(tool.InputSchema)
	toolDefinitionCache.Lock()
	cached, ok := toolDefinitionCache.items[cacheKey]
	toolDefinitionCache.Unlock()
	if ok {
		return cached, nil
	}
	schema, err := stripSchemaAnnotations(tool.InputSchema)
	if err != nil {
		return devin.ChatToolDef{}, fmt.Errorf("sanitize Devin tool %q schema: %w", tool.Name, err)
	}
	schema, err = normalizeSchema(schema)
	if err != nil {
		return devin.ChatToolDef{}, fmt.Errorf("normalize Devin tool %q schema: %w", tool.Name, err)
	}
	converted := devin.ChatToolDef{
		Name:        tool.Name,
		Description: tool.Name,
		SchemaJSON:  string(schema),
	}
	toolDefinitionCache.Lock()
	if len(toolDefinitionCache.items) >= 512 {
		clear(toolDefinitionCache.items)
	}
	toolDefinitionCache.items[cacheKey] = converted
	toolDefinitionCache.Unlock()
	return converted, nil
}

// validToolName 匹配上游实测的工具名字符集。
func validToolName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r != '_' && r != '-' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func stripSchemaAnnotations(schema json.RawMessage) (json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(schema, &value); err != nil {
		return nil, err
	}
	cleaned := stripSchemaValueAnnotations(value, false)
	encoded, err := json.Marshal(cleaned)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func stripSchemaValueAnnotations(value any, propertyNames bool) any {
	switch typed := value.(type) {
	case []any:
		for index, item := range typed {
			typed[index] = stripSchemaValueAnnotations(item, false)
		}
		return typed
	case map[string]any:
		cleaned := make(map[string]any, len(typed))
		for key, child := range typed {
			if isNaturalLanguageAnnotation(key) && !propertyNames {
				continue
			}
			if isSchemaLiteral(key) && !propertyNames {
				cleaned[key] = child
				continue
			}
			cleaned[key] = stripSchemaValueAnnotations(child, key == "properties")
		}
		return cleaned
	default:
		return value
	}
}

// isSchemaLiteral 标识内容属于业务值而非可递归清理的 Schema 定义。
func isSchemaLiteral(key string) bool {
	switch key {
	case "const", "default", "enum", "example", "examples":
		return true
	default:
		return false
	}
}

// isNaturalLanguageAnnotation 标识已确认会触发 Devin 上游工具分类的 Schema 元数据。
func isNaturalLanguageAnnotation(key string) bool {
	switch key {
	case "description", "title", "$comment":
		return true
	default:
		return strings.HasPrefix(strings.ToLower(key), "x-")
	}
}

// normalizeSchema 消除上游确定性拒绝的两种 schema 形态（实测）：
//  1. 本地 $ref（"#/$defs/x" 等 JSON-pointer）inline 展开，顶层
//     $defs/definitions/$schema 一并剥掉；
//  2. 顶层没有任何 schema 关键字、值全为对象的「裸属性 map」包一层
//     {"type":"object","properties":…}。
//
// 循环或解不开的引用丢掉 $ref 键、保留同层其余约束（等价 any），比整请求
// 打回上游拿模糊 invalid_argument 更可排障。其余形态上游全容忍，不做改写。
func normalizeSchema(schema json.RawMessage) (json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(schema, &value); err != nil {
		return nil, err
	}
	normalized := normalizeSchemaValue(value, value, map[string]bool{}, 0)
	if object, ok := normalized.(map[string]any); ok {
		delete(object, "$defs")
		delete(object, "definitions")
		delete(object, "$schema")
		if isBarePropertyMap(object) {
			normalized = map[string]any{"type": "object", "properties": object}
		}
	}
	return json.Marshal(normalized)
}

// maxSchemaRefDepth 限制 $ref 展开深度，病态嵌套 schema 不至于无限膨胀。
const maxSchemaRefDepth = 32

func normalizeSchemaValue(value any, root any, resolving map[string]bool, depth int) any {
	if depth > maxSchemaRefDepth {
		return value
	}
	switch typed := value.(type) {
	case []any:
		for index, item := range typed {
			typed[index] = normalizeSchemaValue(item, root, resolving, depth+1)
		}
		return typed
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok && strings.HasPrefix(ref, "#") {
			if target, found := resolveLocalRef(root, ref); found && !resolving[ref] {
				resolving[ref] = true
				resolved := normalizeSchemaValue(target, root, resolving, depth+1)
				delete(resolving, ref)
				delete(typed, "$ref")
				// $ref 与兄弟键并存时合并：兄弟键覆盖被引用方的同名字段。
				if resolvedObject, ok := resolved.(map[string]any); ok {
					for key, child := range resolvedObject {
						if _, exists := typed[key]; !exists {
							typed[key] = child
						}
					}
				}
			} else {
				// 外部 URL、解不开的路径或循环引用：丢 $ref 保其余键。
				delete(typed, "$ref")
			}
		}
		for key, child := range typed {
			// 业务值字面量里的 map 不是 schema，跳过防止误展开其中的 $ref 键。
			if isSchemaLiteral(key) {
				continue
			}
			typed[key] = normalizeSchemaValue(child, root, resolving, depth+1)
		}
		return typed
	default:
		return value
	}
}

// resolveLocalRef 解析 "#/a/b" 形态的本地 JSON-pointer，处理 ~0/~1 转义。
func resolveLocalRef(root any, ref string) (any, bool) {
	if ref == "#" {
		return root, true
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	current := root
	for _, segment := range strings.Split(ref[2:], "/") {
		segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		if current, ok = object[segment]; !ok {
			return nil, false
		}
	}
	return current, true
}

// schemaKeywords 是判断「该 map 是不是 schema」的关键字集合；
// 判定只需要存在性，不要求穷尽 JSON Schema 全部关键字。
var schemaKeywords = map[string]bool{
	"type": true, "properties": true, "items": true, "required": true,
	"additionalProperties": true, "allOf": true, "anyOf": true, "oneOf": true,
	"not": true, "enum": true, "const": true, "format": true, "pattern": true,
	"minLength": true, "maxLength": true, "minimum": true, "maximum": true,
	"exclusiveMinimum": true, "exclusiveMaximum": true, "multipleOf": true,
	"minItems": true, "maxItems": true, "uniqueItems": true, "contains": true,
	"minProperties": true, "maxProperties": true, "patternProperties": true,
	"propertyNames": true, "dependentRequired": true, "dependentSchemas": true,
	"prefixItems": true, "if": true, "then": true, "else": true,
	"readOnly": true, "writeOnly": true, "deprecated": true,
	"description": true, "title": true, "default": true, "examples": true,
}

// isBarePropertyMap 判定对象是否是上游会拒绝的「裸属性 map」：
// 没有 schema 关键字、没有 $/x- 前缀键，且每个值都是对象（即属性子 schema）。
func isBarePropertyMap(object map[string]any) bool {
	if len(object) == 0 {
		return false
	}
	for key, child := range object {
		if schemaKeywords[key] || strings.HasPrefix(key, "$") || strings.HasPrefix(strings.ToLower(key), "x-") {
			return false
		}
		if _, ok := child.(map[string]any); !ok {
			return false
		}
	}
	return true
}
