package antigravity

import (
	"fmt"
	"strings"
)

// CleanJSONSchema 清理 JSON Schema，移除 Antigravity/Gemini 不支持的字段
// 参考 Antigravity-Manager/src-tauri/src/proxy/common/json_schema.rs 实现
// 确保 schema 符合 JSON Schema draft 2020-12 且适配 Gemini v1internal
func CleanJSONSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	// 0. 预处理：展开 $ref (Schema Flattening)
	// (Go map 是引用的，直接修改 schema)
	flattenRefs(schema, extractDefs(schema))

	// 递归清理
	cleaned := cleanJSONSchemaRecursive(schema)
	result, ok := cleaned.(map[string]any)
	if !ok {
		return nil
	}

	return result
}

// extractDefs 提取并移除定义的 helper
func extractDefs(schema map[string]any) map[string]any {
	defs := make(map[string]any)
	if d, ok := schema["$defs"].(map[string]any); ok {
		for k, v := range d {
			defs[k] = v
		}
		delete(schema, "$defs")
	}
	if d, ok := schema["definitions"].(map[string]any); ok {
		for k, v := range d {
			defs[k] = v
		}
		delete(schema, "definitions")
	}
	return defs
}

// flattenRefs 递归展开 $ref
func flattenRefs(schema map[string]any, defs map[string]any) {
	if len(defs) == 0 {
		return // 无需展开
	}

	// 检查并替换 $ref
	if ref, ok := schema["$ref"].(string); ok {
		delete(schema, "$ref")
		// 解析引用名 (例如 #/$defs/MyType -> MyType)
		parts := strings.Split(ref, "/")
		refName := parts[len(parts)-1]

		if defSchema, exists := defs[refName]; exists {
			if defMap, ok := defSchema.(map[string]any); ok {
				// 合并定义内容 (不覆盖现有 key)
				for k, v := range defMap {
					if _, has := schema[k]; !has {
						schema[k] = deepCopy(v) // 需深拷贝避免共享引用
					}
				}
				// 递归处理刚刚合并进来的内容
				flattenRefs(schema, defs)
			}
		}
	}

	// 遍历子节点
	for _, v := range schema {
		if subMap, ok := v.(map[string]any); ok {
			flattenRefs(subMap, defs)
		} else if subArr, ok := v.([]any); ok {
			for _, item := range subArr {
				if itemMap, ok := item.(map[string]any); ok {
					flattenRefs(itemMap, defs)
				}
			}
		}
	}
}

// deepCopy 深拷贝 (简单实现，仅针对 JSON 类型)
func deepCopy(src any) any {
	if src == nil {
		return nil
	}
	switch v := src.(type) {
	case map[string]any:
		dst := make(map[string]any)
		for k, val := range v {
			dst[k] = deepCopy(val)
		}
		return dst
	case []any:
		dst := make([]any, len(v))
		for i, val := range v {
			dst[i] = deepCopy(val)
		}
		return dst
	default:
		return src
	}
}

// cleanJSONSchemaRecursive 递归核心清理逻辑
// 返回处理后的值 (通常是 input map，但可能修改内部结构)
func cleanJSONSchemaRecursive(value any) any {
	schemaMap, ok := value.(map[string]any)
	if !ok {
		return value
	}

	// 0. [NEW] 合并 allOf
	mergeAllOf(schemaMap)

	// 1. [CRITICAL] 深度递归处理子项
	if props, ok := schemaMap["properties"].(map[string]any); ok {
		for _, v := range props {
			cleanJSONSchemaRecursive(v)
		}
		// Go 中不需要像 Rust 那样显式处理 nullable_keys remove required，
		// 因为我们在子项处理中会正确设置 type 和 description
	} else if items, ok := schemaMap["items"]; ok {
		// [FIX] Gemini 期望 "items" 是单个 Schema 对象（列表验证），而不是数组（元组验证）。
		if itemsArr, ok := items.([]any); ok {
			// 策略：将元组 [A, B] 视为 A、B 中的最佳匹配项。
			best := extractBestSchemaFromUnion(itemsArr)
			if best == nil {
				// 回退到通用字符串
				best = map[string]any{"type": "string"}
			}
			// 用处理后的对象替换原有数组
			cleanedBest := cleanJSONSchemaRecursive(best)
			schemaMap["items"] = cleanedBest
		} else {
			cleanJSONSchemaRecursive(items)
		}
	} else {
		// 遍历所有值递归
		for _, v := range schemaMap {
			if _, isMap := v.(map[string]any); isMap {
				cleanJSONSchemaRecursive(v)
			} else if arr, isArr := v.([]any); isArr {
				for _, item := range arr {
					cleanJSONSchemaRecursive(item)
				}
			}
		}
	}

	// 2. [FIX] 处理 anyOf/oneOf 联合类型: 合并属性而非直接删除
	var unionArray []any
	typeStr, _ := schemaMap["type"].(string)
	if typeStr == "" || typeStr == "object" {
		if anyOf, ok := schemaMap["anyOf"].([]any); ok {
			unionArray = anyOf
		} else if oneOf, ok := schemaMap["oneOf"].([]any); ok {
			unionArray = oneOf
		}
	}

	if len(unionArray) > 0 {
		if bestBranch := extractBestSchemaFromUnion(unionArray); bestBranch != nil {
			if bestMap, ok := bestBranch.(map[string]any); ok {
				// 合并分支内容
				for k, v := range bestMap {
					if k == "properties" {
						targetProps, _ := schemaMap["properties"].(map[string]any)
						if targetProps == nil {
							targetProps = make(map[string]any)
							schemaMap["properties"] = targetProps
						}
						if sourceProps, ok := v.(map[string]any); ok {
							for pk, pv := range sourceProps {
								if _, exists := targetProps[pk]; !exists {
									targetProps[pk] = deepCopy(pv)
								}
							}
						}
					} else if k == "required" {
						targetReq, _ := schemaMap["required"].([]any)
						if sourceReq, ok := v.([]any); ok {
							for _, rv := range sourceReq {
								// 简单的去重添加
								exists := false
								for _, tr := range targetReq {
									if tr == rv {
										exists = true
										break
									}
								}
								if !exists {
									targetReq = append(targetReq, rv)
								}
							}
							schemaMap["required"] = targetReq
						}
					} else if _, exists := schemaMap[k]; !exists {
						schemaMap[k] = deepCopy(v)
					}
				}
			}
		}
	}

	// 3. [SAFETY] 检查当前对象是否为 JSON Schema 节点
	looksLikeSchema := hasKey(schemaMap, "type") ||
		hasKey(schemaMap, "properties") ||
		hasKey(schemaMap, "items") ||
		hasKey(schemaMap, "enum") ||
		hasKey(schemaMap, "anyOf") ||
		hasKey(schemaMap, "oneOf") ||
		hasKey(schemaMap, "allOf")

	if looksLikeSchema {
		// 4. [ROBUST] 约束迁移
		migrateConstraints(schemaMap)

		// 5. [CRITICAL] 白名单过滤
		allowedFields := map[string]bool{
			"type":        true,
			"description": true,
			"properties":  true,
			"required":    true,
			"items":       true,
			"enum":        true,
			"title":       true,
		}
		for k := range schemaMap {
			if !allowedFields[k] {
				delete(schemaMap, k)
			}
		}

		// 6. [SAFETY] 处理空 Object
		if t, _ := schemaMap["type"].(string); t == "object" {
			hasProps := false
			if props, ok := schemaMap["properties"].(map[string]any); ok && len(props) > 0 {
				hasProps = true
			}
			if !hasProps {
				schemaMap["properties"] = map[string]any{
					"reason": map[string]any{
						"type":        "string",
						"description": "Reason for calling this tool",
					},
				}
				schemaMap["required"] = []any{"reason"}
			}
		}

		// 7. [SAFETY] Required 字段对齐
		if props, ok := schemaMap["properties"].(map[string]any); ok {
			if req, ok := schemaMap["required"].([]any); ok {
				var validReq []any
				for _, r := range req {
					if rStr, ok := r.(string); ok {
						if _, exists := props[rStr]; exists {
							validReq = append(validReq, r)
						}
					}
				}
				if len(validReq) > 0 {
					schemaMap["required"] = validReq
				} else {
					delete(schemaMap, "required")
				}
			}
		}

		// 8. 处理 type 字段 (Lowercase + Nullable 提取)
		isEffectivelyNullable := false
		if typeVal, exists := schemaMap["type"]; exists {
			var selectedType string
			switch v := typeVal.(type) {
			case string:
				lower := strings.ToLower(v)
				if lower == "null" {
					isEffectivelyNullable = true
					selectedType = "string" // fallback
				} else {
					selectedType = lower
				}
			case []any:
				// ["string", "null"]
				for _, t := range v {
					if ts, ok := t.(string); ok {
						lower := strings.ToLower(ts)
						if lower == "null" {
							isEffectivelyNullable = true
						} else if selectedType == "" {
							selectedType = lower
						}
					}
				}
				if selectedType == "" {
					selectedType = "string"
				}
			}
			schemaMap["type"] = selectedType
		} else {
			// 默认 object 如果有 properties (虽然上面白名单过滤可能删了 type 如果它不在... 但 type 必在 allowlist)
			// 如果没有 type，但有 properties，补一个
			if hasKey(schemaMap, "properties") {
				schemaMap["type"] = "object"
			} else {
				// 默认为 string ? or object? Gemini 通常需要明确 type
				schemaMap["type"] = "object"
			}
		}

		if isEffectivelyNullable {
			desc, _ := schemaMap["description"].(string)
			if !strings.Contains(desc, "nullable") {
				if desc != "" {
					desc += " "
				}
				desc += "(nullable)"
				schemaMap["description"] = desc
			}
		}

		// 9. Enum 值强制转字符串
		if enumVals, ok := schemaMap["enum"].([]any); ok {
			hasNonString := false
			for i, val := range enumVals {
				if _, isStr := val.(string); !isStr {
					hasNonString = true
					if val == nil {
						enumVals[i] = "null"
					} else {
						enumVals[i] = fmt.Sprintf("%v", val)
					}
				}
			}
			// If we mandated string values, we must ensure type is string
			if hasNonString {
				schemaMap["type"] = "string"
			}
		}
	}

	return schemaMap
}

func hasKey(m map[string]any, k string) bool {
	_, ok := m[k]
	return ok
}

func migrateConstraints(m map[string]any) {
	constraints := []struct {
		key   string
		label string
	}{
		{"minLength", "minLen"},
		{"maxLength", "maxLen"},
		{"pattern", "pattern"},
		{"minimum", "min"},
		{"maximum", "max"},
		{"multipleOf", "multipleOf"},
		{"exclusiveMinimum", "exclMin"},
		{"exclusiveMaximum", "exclMax"},
		{"minItems", "minItems"},
		{"maxItems", "maxItems"},
		{"propertyNames", "propertyNames"},
		{"format", "format"},
	}

	var hints []string
	for _, c := range constraints {
		if val, ok := m[c.key]; ok && val != nil {
			hints = append(hints, fmt.Sprintf("%s: %v", c.label, val))
		}
	}

	if len(hints) > 0 {
		suffix := fmt.Sprintf(" [Constraint: %s]", strings.Join(hints, ", "))
		desc, _ := m["description"].(string)
		if !strings.Contains(desc, suffix) {
			m["description"] = desc + suffix
		}
	}
}

// mergeAllOf 合并 allOf
func mergeAllOf(m map[string]any) {
	allOf, ok := m["allOf"].([]any)
	if !ok {
		return
	}
	delete(m, "allOf")

	mergedProps := make(map[string]any)
	mergedReq := make(map[string]bool)
	otherFields := make(map[string]any)

	for _, sub := range allOf {
		if subMap, ok := sub.(map[string]any); ok {
			// Props
			if props, ok := subMap["properties"].(map[string]any); ok {
				for k, v := range props {
					mergedProps[k] = v
				}
			}
			// Required
			if reqs, ok := subMap["required"].([]any); ok {
				for _, r := range reqs {
					if s, ok := r.(string); ok {
						mergedReq[s] = true
					}
				}
			}
			// Others
			for k, v := range subMap {
				if k != "properties" && k != "required" && k != "allOf" {
					if _, exists := otherFields[k]; !exists {
						otherFields[k] = v
					}
				}
			}
		}
	}

	// Apply
	for k, v := range otherFields {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	if len(mergedProps) > 0 {
		existProps, _ := m["properties"].(map[string]any)
		if existProps == nil {
			existProps = make(map[string]any)
			m["properties"] = existProps
		}
		for k, v := range mergedProps {
			if _, exists := existProps[k]; !exists {
				existProps[k] = v
			}
		}
	}
	if len(mergedReq) > 0 {
		existReq, _ := m["required"].([]any)
		var validReqs []any
		for _, r := range existReq {
			if s, ok := r.(string); ok {
				validReqs = append(validReqs, s)
				delete(mergedReq, s) // already exists
			}
		}
		// append new
		for r := range mergedReq {
			validReqs = append(validReqs, r)
		}
		m["required"] = validReqs
	}
}

// extractBestSchemaFromUnion 从 anyOf/oneOf 中选取最佳分支
func extractBestSchemaFromUnion(unionArray []any) any {
	var bestOption any
	bestScore := -1

	for _, item := range unionArray {
		score := scoreSchemaOption(item)
		if score > bestScore {
			bestScore = score
			bestOption = item
		}
	}
	return bestOption
}

func scoreSchemaOption(val any) int {
	m, ok := val.(map[string]any)
	if !ok {
		return 0
	}
	typeStr, _ := m["type"].(string)

	if hasKey(m, "properties") || typeStr == "object" {
		return 3
	}
	if hasKey(m, "items") || typeStr == "array" {
		return 2
	}
	if typeStr != "" && typeStr != "null" {
		return 1
	}
	return 0
}

// DeepCleanUndefined 深度清理值为 "[undefined]" 的字段
func DeepCleanUndefined(value any) {
	if value == nil {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		for k, val := range v {
			if s, ok := val.(string); ok && s == "[undefined]" {
				delete(v, k)
				continue
			}
			DeepCleanUndefined(val)
		}
	case []any:
		for _, val := range v {
			DeepCleanUndefined(val)
		}
	}
}
