package apicompat

import (
	"bytes"
	"encoding/json"
)

// anthropicInputSchemaUnionKeywords 是 Anthropic 在工具 input_schema 顶层拒绝的
// JSON Schema 联合关键字；嵌套层不受限制，因此只摊平根节点。
var anthropicInputSchemaUnionKeywords = []string{"oneOf", "anyOf", "allOf"}

// flattenAnthropicRootUnions 用单一 object schema 替换顶层的 oneOf/anyOf/allOf：
// 属性取各分支并集，同名属性用嵌套联合保留两侧约束；required 在 oneOf/anyOf
// 下取分支交集、在 allOf 下取分支并集。非对象分支无法用 Anthropic 的 object
// schema 表达，视为不贡献属性与必填字段。
func flattenAnthropicRootUnions(schema map[string]json.RawMessage) {
	present := make([]string, 0, len(anthropicInputSchemaUnionKeywords))
	for _, keyword := range anthropicInputSchemaUnionKeywords {
		if _, ok := schema[keyword]; ok {
			present = append(present, keyword)
		}
	}
	if len(present) == 0 {
		return
	}

	properties := anthropicSchemaProperties(schema)
	rootRequired := anthropicBranchRequired(schema)
	var (
		intersections [][]string
		allOfRequired []string
	)
	for _, keyword := range present {
		raw := schema[keyword]
		delete(schema, keyword)

		branches, ok := decodeAnthropicUnionBranches(raw)
		if !ok {
			continue
		}
		mergeAnthropicObjectBranchProperties(properties, branches)
		if keyword == "allOf" {
			for _, branch := range branches {
				allOfRequired = appendAnthropicRequired(allOfRequired, anthropicBranchRequired(branch))
			}
			continue
		}
		intersections = append(intersections, intersectAnthropicRequired(branches))
	}

	if len(properties) > 0 {
		if encoded, err := json.Marshal(properties); err == nil {
			schema["properties"] = encoded
		}
	}
	required := intersectAnthropicRequiredLists(intersections)
	required = appendAnthropicRequired(required, allOfRequired)
	required = appendAnthropicRequired(required, rootRequired)
	if len(required) > 0 {
		if encoded, err := json.Marshal(required); err == nil {
			schema["required"] = encoded
		}
	}
}

func anthropicSchemaProperties(schema map[string]json.RawMessage) map[string]json.RawMessage {
	properties := make(map[string]json.RawMessage)
	raw, ok := schema["properties"]
	if !ok {
		return properties
	}
	if err := json.Unmarshal(raw, &properties); err != nil || properties == nil {
		return make(map[string]json.RawMessage)
	}
	return properties
}

// decodeAnthropicUnionBranches 解析联合分支并递归摊平分支自身的顶层联合。
// 非对象 JSON 分支以 nil 占位：它不贡献属性，也不要求任何字段。
func decodeAnthropicUnionBranches(raw json.RawMessage) ([]map[string]json.RawMessage, bool) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return nil, false
	}
	branches := make([]map[string]json.RawMessage, 0, len(items))
	for _, item := range items {
		var branch map[string]json.RawMessage
		if err := json.Unmarshal(item, &branch); err != nil || branch == nil {
			branches = append(branches, nil)
			continue
		}
		flattenAnthropicRootUnions(branch)
		branches = append(branches, branch)
	}
	return branches, true
}

func mergeAnthropicObjectBranchProperties(into map[string]json.RawMessage, branches []map[string]json.RawMessage) {
	for _, branch := range branches {
		if !anthropicSchemaBranchIsObject(branch) {
			continue
		}
		raw, ok := branch["properties"]
		if !ok {
			continue
		}
		var properties map[string]json.RawMessage
		if err := json.Unmarshal(raw, &properties); err != nil {
			continue
		}
		for name, property := range properties {
			existing, seen := into[name]
			if !seen {
				into[name] = property
				continue
			}
			if anthropicSchemaJSONEqual(existing, property) {
				continue
			}
			merged, err := json.Marshal(map[string][]json.RawMessage{"anyOf": {existing, property}})
			if err != nil {
				continue
			}
			into[name] = merged
		}
	}
}

// anthropicSchemaBranchIsObject 判断分支是否可能是对象：显式 type=object，或省略
// type 但声明了 properties（缺失 type 等价于不约束，不能据此排除对象分支）。
func anthropicSchemaBranchIsObject(branch map[string]json.RawMessage) bool {
	if branch == nil {
		return false
	}
	if raw, ok := branch["type"]; ok {
		var typ string
		return json.Unmarshal(raw, &typ) == nil && typ == "object"
	}
	_, ok := branch["properties"]
	return ok
}

func anthropicBranchRequired(branch map[string]json.RawMessage) []string {
	if branch == nil {
		return nil
	}
	raw, ok := branch["required"]
	if !ok {
		return nil
	}
	var required []string
	if err := json.Unmarshal(raw, &required); err != nil {
		return nil
	}
	return required
}

// intersectAnthropicRequired 返回所有分支共同要求的字段，顺序沿用第一个分支。
func intersectAnthropicRequired(branches []map[string]json.RawMessage) []string {
	lists := make([][]string, 0, len(branches))
	for _, branch := range branches {
		lists = append(lists, anthropicBranchRequired(branch))
	}
	return intersectAnthropicRequiredLists(lists)
}

func intersectAnthropicRequiredLists(lists [][]string) []string {
	if len(lists) == 0 {
		return nil
	}
	result := lists[0]
	for _, list := range lists[1:] {
		if len(result) == 0 {
			return nil
		}
		next := make([]string, 0, len(result))
		for _, name := range result {
			if containsAnthropicName(list, name) {
				next = append(next, name)
			}
		}
		result = next
	}
	return result
}

func appendAnthropicRequired(required []string, names []string) []string {
	merged := make([]string, 0, len(required))
	merged = append(merged, required...)
	for _, name := range names {
		if !containsAnthropicName(merged, name) {
			merged = append(merged, name)
		}
	}
	return merged
}

func containsAnthropicName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func anthropicSchemaJSONEqual(left, right json.RawMessage) bool {
	var leftBuf, rightBuf bytes.Buffer
	if json.Compact(&leftBuf, left) != nil || json.Compact(&rightBuf, right) != nil {
		return false
	}
	return leftBuf.String() == rightBuf.String()
}
