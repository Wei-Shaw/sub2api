package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const devinTransportTarget = "https://server.codeium.com/exa.api_server_pb.ApiServerService/GetChatMessage"
const devinInboundProtocolKey = "devin_inbound_protocol"
const devinCascadeContextKey = "devin_cascade_id"

func (s *OpenAIGatewayService) forwardDevin(ctx context.Context, c *gin.Context, account *Account, body []byte, protocol, promptCacheKey, defaultMappedModel string) (*OpenAIForwardResult, error) {
	beginUpstreamResponseModelObservation(c)
	ClearActualOpenAIUpstreamEndpoint(c)
	c.Set(devinInboundProtocolKey, protocol)
	cascadeID := devinCascadeID(c, account, body)
	if promptCacheKey != "" {
		cascadeID = isolateDevinCascadeID(c, account, promptCacheKey)
	}
	c.Set(devinCascadeContextKey, cascadeID)
	if err := validateDevinInbound(body, protocol); err != nil {
		writeDevinRequestError(c, err)
		return nil, err
	}
	var result *OpenAIForwardResult
	var err error
	switch protocol {
	case APIProtocolResponses:
		if isOpenAIResponsesCompactPath(c) {
			err = &devin.Error{StatusCode: http.StatusBadRequest, Code: "invalid_request_error", Message: "Devin does not support remote compaction; send the full conversation"}
			writeDevinRequestError(c, err)
			return nil, err
		}
		result, err = s.forwardResponsesViaRawChatCompletions(ctx, c, account, body, time.Now())
	case APIProtocolAnthropic:
		result, err = s.forwardAnthropicViaRawChatCompletions(ctx, c, account, body, defaultMappedModel)
	default:
		if !gjson.GetBytes(body, "messages").Exists() && gjson.GetBytes(body, "input").Exists() {
			if err = validateDevinInbound(body, APIProtocolResponses); err == nil {
				body, err = s.convertResponsesShapedChatBody(body)
			}
			if err != nil {
				writeDevinRequestError(c, err)
				return nil, err
			}
		}
		result, err = s.forwardAsRawChatCompletions(ctx, c, account, body, defaultMappedModel)
	}
	if result != nil {
		result.UpstreamEndpoint = devin.ChatPath
	}
	return result, err
}

// Reject features the cross-protocol converters cannot faithfully carry to Devin
// before conversion can discard them. The wire adapter validates CC fields too.
func validateDevinInbound(body []byte, protocol string) error {
	bad := func(message string) error {
		return &devin.Error{StatusCode: http.StatusBadRequest, Code: "invalid_request_error", Message: message}
	}
	if !gjson.ValidBytes(body) {
		return bad("Invalid JSON request")
	}
	for _, field := range []string{"previous_response_id", "conversation", "background", "audio", "prediction", "functions", "function_call", "context_management"} {
		value := gjson.GetBytes(body, field)
		if value.Exists() && value.Type != gjson.Null && value.Raw != "false" && !(value.Type == gjson.String && value.Str == "") {
			return bad(fmt.Sprintf("Devin does not support %s; send full text/tool conversation history", field))
		}
	}
	if value := gjson.GetBytes(body, "n"); value.Exists() && value.Int() != 1 {
		return bad("Devin supports only n=1")
	}
	if format := gjson.GetBytes(body, "response_format.type").String(); format != "" && format != "text" {
		return bad("Devin does not support structured response formats")
	}
	if format := gjson.GetBytes(body, "text.format.type").String(); format != "" && format != "text" {
		return bad("Devin does not support structured response formats")
	}
	if protocol == APIProtocolAnthropic || protocol == APIProtocolResponses {
		for _, tool := range gjson.GetBytes(body, "tools").Array() {
			typ := tool.Get("type").String()
			if protocol == APIProtocolAnthropic && (typ == "" || typ == "custom") {
				continue
			}
			if protocol == APIProtocolResponses && typ == "function" {
				continue
			}
			return bad("Devin supports function tools only; hosted/server tools are unavailable")
		}
	}
	if protocol == APIProtocolResponses {
		if gjson.GetBytes(body, "store").Bool() {
			return bad("Devin does not support server-stored responses; use store=false")
		}
		for _, item := range gjson.GetBytes(body, "input").Array() {
			switch item.Get("type").String() {
			case "", "message", "function_call", "function_call_output", "reasoning":
			default:
				return bad("Unsupported Devin Responses input item; send messages and function tool history")
			}
			if item.Get("encrypted_content").String() != "" {
				return bad("Devin cannot replay encrypted reasoning; provide plain-text history")
			}
			if item.Get("type").String() == "message" || item.Get("type").String() == "" {
				content := item.Get("content")
				if content.Exists() && content.Type != gjson.String {
					if !content.IsArray() {
						return bad("Devin message content must be text or a content array")
					}
					for _, part := range content.Array() {
						switch part.Get("type").String() {
						case "text", "input_text", "output_text":
						case "input_image", "image_url":
							image := part.Get("image_url")
							if image.IsObject() {
								image = image.Get("url")
							}
							if item.Get("role").String() != "user" || !strings.HasPrefix(image.String(), "data:image/") {
								return bad("Devin Responses images require inline user-message image data")
							}
						default:
							return bad("Unsupported Devin Responses content part")
						}
					}
				}
			}
		}
	}
	if protocol == APIProtocolAnthropic {
		for _, field := range []string{"thinking", "output_config", "top_k"} {
			if value := gjson.GetBytes(body, field); value.Exists() && value.Type != gjson.Null {
				return bad("Devin does not support " + field + "; select an explicit model variant instead")
			}
		}
		if system := gjson.GetBytes(body, "system"); system.Exists() {
			if err := validateDevinAnthropicContent(system, "system", false); err != nil {
				return err
			}
		}
		for _, message := range gjson.GetBytes(body, "messages").Array() {
			role := message.Get("role").String()
			if role != "user" && role != "assistant" {
				return bad("Devin Anthropic messages require user or assistant roles")
			}
			if err := validateDevinAnthropicContent(message.Get("content"), role, false); err != nil {
				return err
			}
		}
	}
	return validateDevinToolHistory(body, protocol)
}

// Shared compatibility conversion removes orphaned or unanswered calls. Reject
// those inputs explicitly rather than silently changing the supplied history.
func validateDevinToolHistory(body []byte, protocol string) error {
	calls, results := map[string]bool{}, map[string]bool{}
	bad := &devin.Error{StatusCode: http.StatusBadRequest, Code: "invalid_request_error", Message: "Devin requires complete, uniquely identified function-call/result history"}
	add := func(set map[string]bool, id string) bool {
		if id == "" || set[id] {
			return false
		}
		set[id] = true
		return true
	}
	if protocol == APIProtocolAnthropic {
		for _, message := range gjson.GetBytes(body, "messages").Array() {
			for _, block := range message.Get("content").Array() {
				switch block.Get("type").String() {
				case "tool_use":
					if !add(calls, block.Get("id").String()) || !block.Get("input").IsObject() {
						return bad
					}
				case "tool_result":
					if !add(results, block.Get("tool_use_id").String()) {
						return bad
					}
				}
			}
		}
	} else if protocol == APIProtocolResponses {
		for _, item := range gjson.GetBytes(body, "input").Array() {
			switch item.Get("type").String() {
			case "function_call":
				if !add(calls, item.Get("call_id").String()) {
					return bad
				}
			case "function_call_output":
				if !add(results, item.Get("call_id").String()) {
					return bad
				}
			}
		}
	}
	if len(calls) != len(results) {
		return bad
	}
	for id := range calls {
		if !results[id] {
			return bad
		}
	}
	return nil
}

func validateDevinAnthropicContent(content gjson.Result, role string, nested bool) error {
	bad := func(message string) error {
		return &devin.Error{StatusCode: http.StatusBadRequest, Code: "invalid_request_error", Message: message}
	}
	if content.Type == gjson.String || !content.Exists() {
		return nil
	}
	if !content.IsArray() {
		return bad("Devin message content must be text or a content array")
	}
	for _, block := range content.Array() {
		switch block.Get("type").String() {
		case "text":
		case "image":
			if role != "user" || block.Get("source.type").String() != "base64" || block.Get("source.data").String() == "" || block.Get("source.media_type").String() == "" {
				return bad("Devin images require inline base64 user content")
			}
		case "tool_use", "thinking":
			if role != "assistant" || nested {
				return bad("Devin tool calls and thinking require assistant content")
			}
		case "tool_result":
			if role != "user" || nested {
				return bad("Devin tool results require user content")
			}
			if err := validateDevinAnthropicContent(block.Get("content"), role, true); err != nil {
				return err
			}
		default:
			return bad("Unsupported Devin message content block")
		}
		if block.Get("signature").String() != "" {
			return bad("Devin cannot replay foreign thinking signatures; provide plain-text reasoning")
		}
	}
	return nil
}

// The shared bridge intentionally drops thinking without tool calls and cannot
// express Anthropic's tool-result error flag. Preserve both in Devin's richer
// wire format without changing conversion behavior for existing providers.
func restoreDevinAnthropicHistory(chatBody, original []byte) ([]byte, error) {
	var thinking []string
	errorsByCall := make(map[string]bool)
	for _, message := range gjson.GetBytes(original, "messages").Array() {
		var parts []string
		for _, block := range message.Get("content").Array() {
			if block.Get("type").String() == "thinking" {
				parts = append(parts, block.Get("thinking").String())
			}
			if block.Get("type").String() == "tool_result" && block.Get("is_error").Bool() {
				errorsByCall[block.Get("tool_use_id").String()] = true
			}
		}
		if message.Get("role").String() == "assistant" {
			thinking = append(thinking, strings.Join(parts, "\n"))
		}
	}
	assistant := 0
	for i, message := range gjson.GetBytes(chatBody, "messages").Array() {
		var err error
		if message.Get("role").String() == "assistant" {
			if assistant < len(thinking) && thinking[assistant] != "" {
				chatBody, err = sjson.SetBytes(chatBody, fmt.Sprintf("messages.%d.reasoning_content", i), thinking[assistant])
			}
			assistant++
		} else if message.Get("role").String() == "tool" && errorsByCall[message.Get("tool_call_id").String()] {
			chatBody, err = sjson.SetBytes(chatBody, fmt.Sprintf("messages.%d.is_error", i), true)
		}
		if err != nil {
			return nil, err
		}
	}
	return chatBody, nil
}

func writeDevinRequestError(c *gin.Context, err error) {
	if c == nil || c.Writer.Written() || errors.Is(err, context.Canceled) {
		return
	}
	status, code, message := http.StatusBadGateway, "upstream_error", "Devin request failed"
	var providerErr *devin.Error
	if errors.As(err, &providerErr) {
		status, code, message = providerErr.StatusCode, providerErr.Code, providerErr.Message
	}
	if c.GetString(devinInboundProtocolKey) == APIProtocolAnthropic {
		writeAnthropicError(c, status, code, message)
	} else {
		writeChatCompletionsError(c, status, code, message)
	}
}
