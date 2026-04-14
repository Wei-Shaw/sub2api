package service

func normalizeOpenAIBuiltinTools(raw any) []map[string]any {
	if !hasOpenAIWebSearchBuiltin(raw) {
		return nil
	}

	return []map[string]any{{"type": "web_search"}}
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
