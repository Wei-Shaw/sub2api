package service

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Filter before conversion: for example, a Messages server tool could otherwise
// become an ordinary function with the same name. Keep the input for failover.
func filterDeepSeekRequest(body []byte, account *Account) []byte {
	deepseek := account != nil && account.Platform == PlatformDeepseek
	root := parseRawJSONView(body)
	root.ForEach(func(key, value gjson.Result) bool {
		field := key.String()
		switch strings.ToLower(field) {
		case "title", "safety_check":
			if !deepseek || !value.IsObject() {
				body, _ = sjson.DeleteBytes(body, field)
			}
		case "tools":
			if !deepseek {
				body = stripDeepSeekTools(body, field, value, true)
			}
		case "messages":
			if !deepseek && value.IsArray() && mayContainDeepSeekTool(value.Raw) {
				body = stripDeepSeekSystemTools(body, field, value)
			}
		case "input":
			// Codex can declare tools inside input as well as at the top level.
			if !deepseek && value.IsArray() && mayContainDeepSeekTool(value.Raw) {
				value.ForEach(func(i, item gjson.Result) bool {
					if !item.IsObject() || !mayContainDeepSeekTool(item.Raw) {
						return true
					}
					var typ, tools gjson.Result
					var toolsKey string
					item.ForEach(func(k, v gjson.Result) bool {
						switch strings.ToLower(k.String()) {
						case "type":
							typ = v
						case "tools":
							toolsKey, tools = k.String(), v
						}
						return true
					})
					switch typ.String() {
					case "additional_tools", "tool_search_output":
						if toolsKey != "" {
							body = stripDeepSeekTools(body, field+"."+strconv.FormatInt(i.Int(), 10)+"."+toolsKey, tools, false)
						}
					}
					return true
				})
			}
		}
		return true
	})
	return body
}

func preservesDeepSeekWebToolHistory(account *Account) bool {
	if account == nil {
		return false
	}
	return account.Platform == PlatformDeepseek || isDeepseekOfficialHost(account.GetOpenAIBaseURL())
}

func stripDeepSeekSystemTools(body []byte, path string, messages gjson.Result) []byte {
	// Collect edits from the original array so removing a tools-only message
	// cannot shift the index of another system message.
	items := messages.Array()
	changed := false
	for i, msg := range items {
		if !mayContainDeepSeekTool(msg.Raw) {
			continue
		}
		var role, content, tools gjson.Result
		var toolsKey string
		msg.ForEach(func(k, v gjson.Result) bool {
			switch strings.ToLower(k.String()) {
			case "role":
				role = v
			case "content":
				content = v
			case "tools":
				toolsKey, tools = k.String(), v
			}
			return true
		})
		if role.String() != "system" || !tools.IsArray() || !mayContainDeepSeekTool(tools.Raw) {
			continue
		}
		raw := []byte(msg.Raw)
		filtered := stripDeepSeekTools(raw, toolsKey, tools, true)
		if bytes.Equal(raw, filtered) {
			continue
		}
		changed = true
		emptyContent := content.Type == gjson.Null || content.Raw == `""` || (content.IsArray() && !gjsonArrayHasElements(content))
		if emptyContent && !gjson.GetBytes(filtered, toolsKey).Exists() {
			items[i].Raw = ""
		} else {
			items[i].Raw = string(filtered)
		}
	}
	if !changed {
		return body
	}
	kept := make([]json.RawMessage, 0, len(items))
	for _, msg := range items {
		if msg.Raw != "" {
			kept = append(kept, json.RawMessage(msg.Raw))
		}
	}
	body, _ = sjson.SetBytes(body, path, kept)
	return body
}

func mayContainDeepSeekTool(raw string) bool {
	return strings.Contains(raw, "deepseek_") || strings.Contains(raw, `\u`)
}

func stripDeepSeekTools(body []byte, path string, tools gjson.Result, omitEmpty bool) []byte {
	if !tools.IsArray() || !mayContainDeepSeekTool(tools.Raw) {
		return body
	}
	var kept []json.RawMessage
	changed := false
	tools.ForEach(func(_, tool gjson.Result) bool {
		typ := tool.Get("type").String()
		if typ == "" && tool.IsObject() && mayContainDeepSeekTool(tool.Raw) {
			// encoding/json also matches case-insensitive DTO field names.
			tool.ForEach(func(key, value gjson.Result) bool {
				if strings.EqualFold(key.String(), "type") {
					typ = value.String()
				}
				return true
			})
		}
		if apicompat.IsDeepSeekWebTool(typ) {
			changed = true
			return true
		}
		raw := json.RawMessage(tool.Raw)
		if typ == "namespace" && tool.IsObject() {
			filtered := []byte(raw)
			tool.ForEach(func(key, value gjson.Result) bool {
				switch strings.ToLower(key.String()) {
				case "tools", "children":
					filtered = stripDeepSeekTools(filtered, key.String(), value, false)
				}
				return true
			})
			if !bytes.Equal(filtered, raw) {
				changed = true
				hasChild := false
				parseRawJSONView(filtered).ForEach(func(key, value gjson.Result) bool {
					switch strings.ToLower(key.String()) {
					case "tools", "children":
						if value.IsArray() && gjsonArrayHasElements(value) {
							hasChild = true
							return false
						}
					}
					return true
				})
				if !hasChild {
					return true
				}
				raw = filtered
			}
		}
		kept = append(kept, raw)
		return true
	})
	if !changed {
		return body
	}
	if len(kept) == 0 && omitEmpty {
		body, _ = sjson.DeleteBytes(body, path)
	} else if len(kept) == 0 {
		body, _ = sjson.SetRawBytes(body, path, []byte("[]"))
	} else {
		body, _ = sjson.SetBytes(body, path, kept)
	}
	return body
}
