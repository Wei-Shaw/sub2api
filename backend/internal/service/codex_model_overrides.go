package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// ValidateCodexModelOverrides 保留未知扩展字段，但校验已知客户端字段的类型。
// 模型 ID 的准入由管理接口根据分组实际目录另行校验。
func ValidateCodexModelOverrides(overrides map[string]map[string]json.RawMessage) error {
	if len(overrides) > 500 {
		return fmt.Errorf("at most 500 model overrides are allowed")
	}
	body, err := json.Marshal(overrides)
	if err != nil || len(body) > 2<<20 {
		return fmt.Errorf("model overrides must be valid JSON no larger than 2 MiB")
	}
	for id, fields := range overrides {
		if id == "" || strings.TrimSpace(id) != id || strings.Contains(id, "*") || len(id) > 256 {
			return fmt.Errorf("invalid model ID %q", id)
		}
		for _, key := range []string{"slug", "id"} {
			if _, exists := fields[key]; exists {
				return fmt.Errorf("%s: %s cannot be overridden", id, key)
			}
		}
		base, _ := json.Marshal(newConfiguredCodexModelDescriptor(id))
		var descriptor map[string]json.RawMessage
		_ = json.Unmarshal(base, &descriptor)
		mergeCodexModelFields(descriptor, fields)
		merged, _ := json.Marshal(descriptor)
		var typed configuredCodexModelDescriptor
		if err := json.Unmarshal(merged, &typed); err != nil {
			return fmt.Errorf("%s: invalid model configuration: %w", id, err)
		}
		if err := validateCodexRequiredFields(fields, reflect.TypeOf(typed)); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		_, hasContext := fields["context_window"]
		_, hasMaxContext := fields["max_context_window"]
		if typed.ContextWindow <= 0 || typed.MaxContextWindow <= 0 || (hasContext && hasMaxContext && typed.MaxContextWindow < typed.ContextWindow) || typed.EffectiveContextWindowPercent <= 0 || typed.EffectiveContextWindowPercent > 100 {
			return fmt.Errorf("%s: invalid context window or effective context percentage", id)
		}
		if len(typed.InputModalities) == 0 {
			return fmt.Errorf("%s: input_modalities cannot be empty", id)
		}
		for _, modality := range typed.InputModalities {
			if modality != "text" && modality != "image" {
				return fmt.Errorf("%s: unsupported input modality %q", id, modality)
			}
		}
		if _, levelsSet := fields["supported_reasoning_levels"]; levelsSet {
			if _, defaultSet := fields["default_reasoning_level"]; defaultSet && typed.DefaultReasoningLevel != nil && *typed.DefaultReasoningLevel != "" {
				found := false
				for _, level := range typed.SupportedReasoningLevels {
					found = found || level.Effort == *typed.DefaultReasoningLevel
				}
				if !found {
					return fmt.Errorf("%s: default reasoning level must be in supported_reasoning_levels", id)
				}
			}
		}
	}
	return nil
}

func validateCodexRequiredFields(fields map[string]json.RawMessage, schema reflect.Type) error {
	for i := 0; i < schema.NumField(); i++ {
		field := schema.Field(i)
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		value, exists := fields[key]
		if !exists {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			if field.Type.Kind() != reflect.Pointer && field.Type.Kind() != reflect.Interface {
				return fmt.Errorf("%s cannot be null", key)
			}
			continue
		}
		if field.Type.Kind() == reflect.Struct {
			var child map[string]json.RawMessage
			if json.Unmarshal(value, &child) == nil {
				if err := validateCodexRequiredFields(child, field.Type); err != nil {
					return fmt.Errorf("%s.%w", key, err)
				}
			}
		}
	}
	return nil
}

// ValidateEffectiveCodexModelOverride 校验与实际基线合并后的数值和推理档位关系。
func ValidateEffectiveCodexModelOverride(id string, base, patch map[string]json.RawMessage) error {
	fields := make(map[string]json.RawMessage, len(base))
	for key, value := range base {
		fields[key] = value
	}
	mergeCodexModelFields(fields, patch)
	delete(fields, "slug")
	delete(fields, "id")
	return ValidateCodexModelOverrides(map[string]map[string]json.RawMessage{id: fields})
}

// mergeCodexModelFields 对对象递归合并；数组整体替换，null 是显式值。
func mergeCodexModelFields(target, patch map[string]json.RawMessage) {
	for key, value := range patch {
		var child, existing map[string]json.RawMessage
		if json.Unmarshal(value, &child) == nil && child != nil && json.Unmarshal(target[key], &existing) == nil && existing != nil {
			mergeCodexModelFields(existing, child)
			target[key], _ = json.Marshal(existing)
		} else {
			target[key] = append(json.RawMessage(nil), value...)
		}
	}
}

// ApplyGroupCodexModelOverrides 在目录过滤之后执行，绝不追加模型或改变 slug。
func ApplyGroupCodexModelOverrides(body []byte, group *Group) ([]byte, error) {
	if group == nil || len(group.CodexModelsManifestConfig.ModelOverrides) == 0 {
		return body, nil
	}
	envelope, entries, err := modelCatalogEntries(body, "models")
	if err != nil {
		return nil, err
	}
	changed := false
	for i, raw := range entries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
		var slug string
		_ = json.Unmarshal(fields["slug"], &slug)
		patch := group.CodexModelsManifestConfig.ModelOverrides[slug]
		if len(patch) == 0 {
			continue
		}
		originalSlug := append(json.RawMessage(nil), fields["slug"]...)
		mergeCodexModelFields(fields, patch)
		fields["slug"] = originalSlug
		entries[i], err = json.Marshal(fields)
		if err != nil {
			return nil, err
		}
		changed = true
	}
	if !changed {
		return body, nil
	}
	envelope["models"], err = json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}
