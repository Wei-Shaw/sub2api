package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIWebSearchProxyName   = "web_search"
	openAIWebSearchProxyDesc   = "Search the web for real-time information, news, references, and documentation."
	openAIWebSearchProxySchema = `{"type":"object","properties":{"query":{"type":"string","description":"The search query to execute"}},"required":["query"]}`

	contextKeyOpenAIWebSearchEmulated = "openai_web_search_emulated"
	contextKeyOpenAIWebSearchStripped = "openai_web_search_stripped"
)

// isUpstreamNativeWebSearchSupported reports whether the upstream account natively supports
// OpenAI Responses server-side web_search tools ({"type": "web_search"}).
// Official OpenAI (api.openai.com, ChatGPT OAuth) and Grok support this natively.
// Third-party providers (Ollama, 9router, AMD, CommandCode, DeepSeek, etc.) do NOT.
func isUpstreamNativeWebSearchSupported(account *Account) bool {
	if account == nil {
		return false
	}
	if account.Platform == PlatformGrok {
		return true
	}
	if account.IsOpenAIOAuth() {
		return true
	}
	if account.IsOpenAI() {
		baseURL := strings.TrimSpace(account.GetCredential("base_url"))
		if baseURL == "" {
			baseURL = account.GetOpenAIBaseURL()
		}
		if baseURL == "" {
			return true
		}
		parsed, err := url.Parse(baseURL)
		if err == nil && (strings.EqualFold(parsed.Hostname(), "api.openai.com") || parsed.Hostname() == "") {
			return true
		}
	}
	return false
}

// shouldEmulateOpenAIWebSearch checks whether OpenAI Responses web search emulation should be active.
func (s *OpenAIGatewayService) shouldEmulateOpenAIWebSearch(ctx context.Context, account *Account, groupID int64) bool {
	if getWebSearchManager() == nil {
		return false
	}
	if s.settingService == nil || !s.settingService.IsWebSearchEmulationEnabled(ctx) {
		return false
	}
	mode := account.GetWebSearchEmulationMode()
	switch mode {
	case WebSearchModeEnabled:
		return true
	case WebSearchModeDisabled:
		return false
	default:
		// Default mode: check channel config if present
		if groupID > 0 && s.channelService != nil {
			ch, err := s.channelService.GetChannelForGroup(ctx, groupID)
			if err == nil && ch != nil && ch.FeaturesConfig != nil {
				if wse, ok := ch.FeaturesConfig[featureKeyWebSearchEmulation].(map[string]any); ok {
					if enabled, has := wse[account.Platform].(bool); has {
						return enabled
					}
				}
			}
		}
		// If channel doesn't explicitly disable it, default to true since global setting is enabled
		return true
	}
}

// isResponsesWebSearchToolJSON checks if a tool definition in JSON is a web_search server tool.
func isResponsesWebSearchToolJSON(tool gjson.Result) bool {
	toolType := tool.Get("type").String()
	if toolType == "function" || toolType == "custom" {
		return false
	}
	if strings.HasPrefix(toolType, toolTypeWebSearchPrefix) || toolType == toolTypeGoogleSearch {
		return true
	}
	if tool.Get("parameters").Exists() || tool.Get("input_schema").Exists() {
		return false
	}
	name := tool.Get("name").String()
	switch name {
	case toolNameWebSearch, toolNameGoogleSearch, toolNameWebSearch2025:
		return true
	}
	return false
}

// adaptOpenAIResponsesWebSearchTool inspects request body tools. If the upstream provider does not
// natively support web_search:
// - If emulation is enabled: transforms {"type": "web_search"} into a standard function tool.
// - If emulation is disabled: strips {"type": "web_search"} from tools so the upstream does not reject with 400.
func (s *OpenAIGatewayService) adaptOpenAIResponsesWebSearchTool(ctx context.Context, c *gin.Context, account *Account, body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}
	toolsRes := gjson.GetBytes(body, "tools")
	if !toolsRes.Exists() || !toolsRes.IsArray() {
		return body, false, nil
	}

	// If upstream natively supports web_search, let it pass through unmodified.
	if isUpstreamNativeWebSearchSupported(account) {
		return body, false, nil
	}

	toolsArray := toolsRes.Array()
	hasWebSearch := false
	hasFunctionConflict := false
	for _, tool := range toolsArray {
		if isResponsesWebSearchToolJSON(tool) {
			hasWebSearch = true
		}
		if tool.Get("type").String() == "function" && tool.Get("name").String() == openAIWebSearchProxyName {
			hasFunctionConflict = true
		}
	}

	if !hasWebSearch {
		return body, false, nil
	}

	groupID := getOpenAIGroupIDFromContext(c)
	emulate := s.shouldEmulateOpenAIWebSearch(ctx, account, groupID)

	var newTools []any
	for _, tool := range toolsArray {
		if isResponsesWebSearchToolJSON(tool) {
			if emulate && !hasFunctionConflict {
				var schemaObj map[string]any
				_ = json.Unmarshal([]byte(openAIWebSearchProxySchema), &schemaObj)
				newTools = append(newTools, map[string]any{
					"type":        "function",
					"name":        openAIWebSearchProxyName,
					"description": openAIWebSearchProxyDesc,
					"parameters":  schemaObj,
				})
			}
			// if !emulate or hasFunctionConflict, strip this server-side tool
			continue
		}
		newTools = append(newTools, tool.Value())
	}

	var adaptedBody []byte
	var err error
	if len(newTools) == 0 {
		adaptedBody, err = sjson.DeleteBytes(body, "tools")
		if err != nil {
			return body, false, err
		}
		// Also clean up tool_choice if tools array was eliminated
		if gjson.GetBytes(adaptedBody, "tool_choice").Exists() {
			adaptedBody, err = sjson.DeleteBytes(adaptedBody, "tool_choice")
			if err != nil {
				return body, false, err
			}
		}
	} else {
		adaptedBody, err = sjson.SetBytes(body, "tools", newTools)
		if err != nil {
			return body, false, err
		}
	}

	if emulate && !hasFunctionConflict {
		if c != nil {
			c.Set(contextKeyOpenAIWebSearchEmulated, true)
			mapping, _ := openAIResponsesClientToolMapping(c)
			mapping.WebSearch = true
			setOpenAIResponsesClientToolMapping(c, mapping)
		}
		slog.Info("openai responses web search: adapted tool to function for third-party upstream",
			"account_id", account.ID, "account_name", account.Name, "platform", account.Platform)
	} else {
		if c != nil {
			c.Set(contextKeyOpenAIWebSearchStripped, true)
		}
		slog.Info("openai responses web search: stripped unsupported tool for third-party upstream",
			"account_id", account.ID, "account_name", account.Name, "platform", account.Platform)
	}

	return adaptedBody, true, nil
}

// synthesizeWebSearchCallOutput creates a ResponsesOutput item representing a completed web_search_call.
func synthesizeWebSearchCallOutput(query string) apicompat.ResponsesOutput {
	return apicompat.ResponsesOutput{
		Type:   "web_search_call",
		ID:     "ws_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16],
		Status: "completed",
		Action: &apicompat.WebSearchAction{
			Type:  "search",
			Query: query,
		},
	}
}
