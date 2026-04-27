package service

import "strings"

const (
	openAIImageGenerationBuiltinDefaultModel        = "gpt-image-2"
	openAIImageGenerationBuiltinDefaultOutputFormat = "png"
)

var openAIImageGenerationBuiltinAllowedFields = map[string]struct{}{
	"model":              {},
	"size":               {},
	"quality":            {},
	"background":         {},
	"output_format":      {},
	"output_compression": {},
	"moderation":         {},
	"style":              {},
	"partial_images":     {},
}

func normalizeOpenAIBuiltinTools(raw any) []map[string]any {
	tools := make([]map[string]any, 0, 2)
	if hasOpenAIWebSearchBuiltin(raw) {
		tools = append(tools, map[string]any{"type": "web_search"})
	}
	if imageTool, ok := openAIImageGenerationBuiltinTool(raw); ok {
		tools = append(tools, imageTool)
	}
	if len(tools) == 0 {
		return nil
	}
	return tools
}

func openAIImageGenerationBuiltinTool(raw any) (map[string]any, bool) {
	switch value := raw.(type) {
	case []any:
		for _, item := range value {
			if tool, ok := openAIImageGenerationBuiltinItem(item); ok {
				return tool, true
			}
		}
	case []string:
		for _, item := range value {
			if item == "image_generation" {
				return defaultOpenAIImageGenerationBuiltinTool(), true
			}
		}
	case []map[string]any:
		for _, item := range value {
			if tool, ok := openAIImageGenerationBuiltinItem(item); ok {
				return tool, true
			}
		}
	case map[string]any:
		if tool, ok := openAIImageGenerationBuiltinItem(value); ok {
			return tool, true
		}
		rawConfig, ok := value["image_generation"]
		if !ok {
			return nil, false
		}
		switch config := rawConfig.(type) {
		case bool:
			if config {
				return defaultOpenAIImageGenerationBuiltinTool(), true
			}
		case map[string]any:
			return buildOpenAIImageGenerationBuiltinTool(config), true
		}
	}

	return nil, false
}

func openAIImageGenerationBuiltinItem(raw any) (map[string]any, bool) {
	if name, ok := raw.(string); ok {
		if name == "image_generation" {
			return defaultOpenAIImageGenerationBuiltinTool(), true
		}
		return nil, false
	}

	tool, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	typ, _ := tool["type"].(string)
	if typ != "image_generation" {
		return nil, false
	}
	return buildOpenAIImageGenerationBuiltinTool(tool), true
}

func defaultOpenAIImageGenerationBuiltinTool() map[string]any {
	return map[string]any{
		"type":          "image_generation",
		"model":         openAIImageGenerationBuiltinDefaultModel,
		"output_format": openAIImageGenerationBuiltinDefaultOutputFormat,
	}
}

func buildOpenAIImageGenerationBuiltinTool(config map[string]any) map[string]any {
	tool := defaultOpenAIImageGenerationBuiltinTool()
	for key, value := range config {
		if _, ok := openAIImageGenerationBuiltinAllowedFields[key]; !ok || value == nil {
			continue
		}
		if text, ok := value.(string); ok {
			trimmed := strings.TrimSpace(text)
			if trimmed == "" {
				continue
			}
			tool[key] = trimmed
			continue
		}
		tool[key] = value
	}
	return tool
}

func hasOpenAIWebSearchBuiltin(raw any) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case []any:
		for _, item := range value {
			if isOpenAIWebSearchBuiltinItem(item) {
				return true
			}
		}
	case []string:
		for _, item := range value {
			if item == "web_search" {
				return true
			}
		}
	case []map[string]any:
		for _, item := range value {
			if isOpenAIWebSearchBuiltinItem(item) {
				return true
			}
		}
	case map[string]any:
		enabled, ok := value["web_search"].(bool)
		return ok && enabled
	}

	return false
}

func isOpenAIWebSearchBuiltinItem(raw any) bool {
	if name, ok := raw.(string); ok {
		return name == "web_search"
	}

	tool, ok := raw.(map[string]any)
	if !ok {
		return false
	}

	typ, _ := tool["type"].(string)
	return typ == "web_search"
}
