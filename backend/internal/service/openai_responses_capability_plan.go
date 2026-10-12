package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type openAIResponsesToolCapabilities struct {
	customTools map[string]bool
	namespaces  bool
	toolSearch  bool
}

type openAIResponsesAdaptationPlan struct {
	lowerClientTools       bool
	normalizeAgentMessages bool
}

func resolveOpenAIResponsesToolCapabilities(account *Account) (openAIResponsesToolCapabilities, bool) {
	if account == nil || account.Type != AccountTypeAPIKey {
		return openAIResponsesToolCapabilities{}, false
	}
	if account.Platform == PlatformOpenAI {
		return openAIResponsesToolCapabilities{}, true
	}
	if !account.UsesNativeResponsesProtocol() {
		return openAIResponsesToolCapabilities{}, false
	}

	capabilities := openAIResponsesToolCapabilities{}
	switch account.Platform {
	case PlatformDeepseek, PlatformZhipu:
		capabilities.customTools = map[string]bool{"apply_patch": true}
	}
	return capabilities, true
}

func planOpenAIResponsesAdaptation(account *Account, body []byte) openAIResponsesAdaptationPlan {
	plan := openAIResponsesAdaptationPlan{
		normalizeAgentMessages: account != nil && !account.UsesOpenAICodexProtocol() &&
			gjson.GetBytes(body, `input.#(type=="agent_message")`).Exists(),
	}
	if account != nil && account.Type == AccountTypeAPIKey && account.Platform == PlatformOpenAI {
		plan.lowerClientTools = needsOpenAIResponsesClientToolAdaptation(body)
		return plan
	}

	capabilities, managed := resolveOpenAIResponsesToolCapabilities(account)
	if !managed {
		return plan
	}
	var visit func(gjson.Result) bool
	visit = func(value gjson.Result) bool {
		if value.IsObject() {
			typ := strings.TrimSpace(value.Get("type").String())
			switch typ {
			case "custom", "custom_tool_call":
				name := strings.TrimSpace(value.Get("name").String())
				if name != "" && !capabilities.customTools[name] {
					plan.lowerClientTools = true
					return false
				}
			case "namespace":
				if !capabilities.namespaces {
					plan.lowerClientTools = true
					return false
				}
			case "tool_search", "tool_search_call", "tool_search_output":
				if !capabilities.toolSearch {
					plan.lowerClientTools = true
					return false
				}
			}
		}
		if value.IsObject() || value.IsArray() {
			value.ForEach(func(_, child gjson.Result) bool { return visit(child) })
		}
		return !plan.lowerClientTools
	}
	visit(gjson.ParseBytes(body))
	return plan
}

func shouldAdaptOpenAIResponsesClientTools(account *Account, c *gin.Context, body []byte) bool {
	if account == nil || isOpenAIResponsesCompactPath(c) || isOpenAINativeCompactionV2(c) {
		return false
	}
	return planOpenAIResponsesAdaptation(account, body).lowerClientTools
}

func normalizeOpenAIResponsesAgentMessages(body []byte) ([]byte, bool, error) {
	var requestBody map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &requestBody); err != nil {
		return body, false, fmt.Errorf("decode Responses agent messages: %w", err)
	}
	if !apicompat.NormalizeResponsesAgentMessages(requestBody) {
		return body, false, nil
	}
	normalized, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, false, fmt.Errorf("encode Responses agent messages: %w", err)
	}
	return normalized, true, nil
}
