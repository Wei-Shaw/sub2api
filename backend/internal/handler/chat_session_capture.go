package handler

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/tidwall/gjson"
)

func recordChatSessionAsync(
	ctx context.Context,
	recorder *service.ChatSessionService,
	apiKey *service.APIKey,
	account *service.Account,
	input *service.ChatSessionRecordInput,
	requestBody []byte,
	finalOutputText string,
) {
	if recorder == nil || apiKey == nil || input == nil {
		return
	}

	input.UserID = apiKey.UserID
	input.APIKeyID = apiKey.ID
	input.GroupID = apiKey.GroupID
	if account != nil {
		input.AccountID = &account.ID
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now()
	}

	messages, _ := buildChatSessionMessages(input.InboundEndpoint, requestBody, finalOutputText)
	if len(messages) == 0 {
		return
	}
	input.Messages = messages
	input.Events = nil

	go func(payload *service.ChatSessionRecordInput) {
		taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = recorder.RecordSession(taskCtx, payload)
	}(input)
}

func buildChatSessionMessages(inboundEndpoint *string, requestBody []byte, finalOutputText string) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	endpoint := ""
	if inboundEndpoint != nil {
		endpoint = strings.TrimSpace(*inboundEndpoint)
	}

	inboundMessages, _ := parseInboundChatMessages(endpoint, requestBody)
	messages := make([]service.ChatMessageRecordInput, 0, len(inboundMessages)+1)
	messages = append(messages, inboundMessages...)
	if text := strings.TrimSpace(finalOutputText); text != "" {
		messages = append(messages, service.ChatMessageRecordInput{
			Role:        "assistant",
			Direction:   "outbound",
			ContentText: text,
		})
	}
	return messages, nil
}

func parseInboundChatMessages(endpoint string, body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	switch {
	case strings.Contains(endpoint, "/chat/completions"):
		return parseChatCompletionsRequestMessages(body)
	case strings.Contains(endpoint, "/responses"):
		return parseResponsesRequestMessages(body)
	case strings.Contains(endpoint, "/messages"):
		return parseAnthropicRequestMessages(body)
	default:
		return parseGenericTextMessages(body, "user", "inbound"), nil
	}
}

func parseOutboundChatMessages(endpoint string, body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	switch {
	case strings.Contains(endpoint, "/chat/completions"):
		return parseChatCompletionsResponseMessages(body)
	case strings.Contains(endpoint, "/responses"):
		return parseResponsesResponseMessages(body)
	case strings.Contains(endpoint, "/messages"):
		return parseAnthropicResponseMessages(body)
	default:
		return parseGenericTextMessages(body, "assistant", "outbound"), nil
	}
}

func parseChatCompletionsRequestMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	items := make([]service.ChatMessageRecordInput, 0)
	for _, msg := range gjson.GetBytes(body, "messages").Array() {
		role := strings.TrimSpace(msg.Get("role").String())
		text := extractChatCompletionsContent(msg.Get("content"))
		if text == "" {
			continue
		}
		if role == "" || role == "user" {
			items = append(items, service.ChatMessageRecordInput{Role: "user", Direction: "inbound", ContentText: text})
		}
	}
	return keepLastInboundUserMessage(items), nil
}

func parseChatCompletionsResponseMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		text := collectChatCompletionsSSEText(trimmed)
		if text == "" {
			return nil, nil
		}
		return []service.ChatMessageRecordInput{{Role: "assistant", Direction: "outbound", ContentText: text}}, nil
	}

	items := make([]service.ChatMessageRecordInput, 0)
	for _, choice := range gjson.GetBytes(trimmed, "choices").Array() {
		text := extractChatCompletionsContent(choice.Get("message.content"))
		if text == "" {
			text = extractChatCompletionsContent(choice.Get("delta.content"))
		}
		if text == "" {
			continue
		}
		items = append(items, service.ChatMessageRecordInput{Role: "assistant", Direction: "outbound", ContentText: text})
	}
	return keepLastAssistantMessage(items), nil
}

func extractChatCompletionsContent(result gjson.Result) string {
	switch result.Type {
	case gjson.String:
		return strings.TrimSpace(result.String())
	case gjson.JSON:
		if result.IsArray() {
			parts := make([]string, 0, len(result.Array()))
			for _, item := range result.Array() {
				if item.Type == gjson.String {
					if text := strings.TrimSpace(item.String()); text != "" {
						parts = append(parts, text)
					}
					continue
				}
				text := strings.TrimSpace(item.Get("text").String())
				if text == "" {
					text = strings.TrimSpace(item.Get("content").String())
				}
				if text != "" {
					parts = append(parts, text)
				}
			}
			return strings.TrimSpace(strings.Join(parts, "\n"))
		}
	}
	return ""
}

func parseAnthropicRequestMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	items := make([]service.ChatMessageRecordInput, 0)
	for _, msg := range gjson.GetBytes(body, "messages").Array() {
		role := strings.TrimSpace(msg.Get("role").String())
		text := extractAnthropicContent(msg.Get("content"))
		if text == "" {
			continue
		}
		if role == "" || role == "user" {
			items = append(items, service.ChatMessageRecordInput{Role: "user", Direction: "inbound", ContentText: text})
		}
	}
	return keepLastInboundUserMessage(items), nil
}

func parseAnthropicResponseMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if bytes.HasPrefix(trimmed, []byte("event:")) || bytes.HasPrefix(trimmed, []byte("data:")) {
		text := collectAnthropicSSEText(trimmed)
		if text == "" {
			return nil, nil
		}
		return []service.ChatMessageRecordInput{{Role: "assistant", Direction: "outbound", ContentText: text}}, nil
	}

	text := extractAnthropicContent(gjson.GetBytes(trimmed, "content"))
	if text == "" {
		return nil, nil
	}
	return []service.ChatMessageRecordInput{{Role: "assistant", Direction: "outbound", ContentText: text}}, nil
}

func extractAnthropicContent(result gjson.Result) string {
	switch {
	case result.Type == gjson.String:
		return strings.TrimSpace(result.String())
	case result.IsArray():
		parts := make([]string, 0, len(result.Array()))
		for _, item := range result.Array() {
			text := strings.TrimSpace(item.Get("text").String())
			if text == "" {
				text = strings.TrimSpace(item.Get("thinking").String())
			}
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return ""
	}
}

func parseResponsesRequestMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	items := make([]service.ChatMessageRecordInput, 0)

	input := gjson.GetBytes(body, "input")
	switch {
	case input.Type == gjson.String:
		text := strings.TrimSpace(input.String())
		if text != "" {
			items = append(items, service.ChatMessageRecordInput{Role: "user", Direction: "inbound", ContentText: text})
		}
	case input.IsArray():
		for _, item := range input.Array() {
			role := strings.TrimSpace(item.Get("role").String())
			text := extractResponsesContent(item)
			if text == "" {
				continue
			}
			if role == "" || role == "user" {
				items = append(items, service.ChatMessageRecordInput{Role: "user", Direction: "inbound", ContentText: text})
			}
		}
	}

	if len(items) == 0 {
		text := strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
		if text != "" {
			items = append(items, service.ChatMessageRecordInput{Role: "user", Direction: "inbound", ContentText: text})
		}
	}
	return keepLastInboundUserMessage(items), nil
}

func parseResponsesResponseMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		text := collectResponsesSSEText(trimmed)
		if text == "" {
			return nil, nil
		}
		return []service.ChatMessageRecordInput{{Role: "assistant", Direction: "outbound", ContentText: text}}, nil
	}

	items := make([]service.ChatMessageRecordInput, 0)
	for _, output := range gjson.GetBytes(trimmed, "output").Array() {
		role := strings.TrimSpace(output.Get("role").String())
		if role == "" {
			role = "assistant"
		}
		text := extractResponsesContent(output)
		if text == "" {
			continue
		}
		items = append(items, service.ChatMessageRecordInput{Role: role, Direction: "outbound", ContentText: text})
	}
	return keepLastAssistantMessage(items), nil
}

func extractResponsesContent(result gjson.Result) string {
	if result.Type == gjson.String {
		return strings.TrimSpace(result.String())
	}
	switch result.Get("type").String() {
	case "input_text", "output_text", "text":
		if text := strings.TrimSpace(result.Get("text").String()); text != "" {
			return text
		}
	}
	content := result.Get("content")
	if content.IsArray() {
		parts := make([]string, 0, len(content.Array()))
		for _, item := range content.Array() {
			switch item.Get("type").String() {
			case "input_text", "output_text", "text":
				if text := strings.TrimSpace(item.Get("text").String()); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}
	if text := strings.TrimSpace(result.Get("text").String()); text != "" {
		return text
	}
	return ""
}

func parseGenericTextMessages(body []byte, role string, direction string) []service.ChatMessageRecordInput {
	if !gjson.ValidBytes(body) {
		return nil
	}
	if text := strings.TrimSpace(gjson.GetBytes(body, "text").String()); text != "" {
		return []service.ChatMessageRecordInput{{Role: role, Direction: direction, ContentText: text}}
	}
	return nil
}

func collectChatCompletionsSSEText(body []byte) string {
	var parts []string
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if bytes.Equal(payload, []byte("[DONE]")) || len(payload) == 0 || !gjson.ValidBytes(payload) {
			continue
		}
		for _, choice := range gjson.GetBytes(payload, "choices").Array() {
			if text := extractChatCompletionsContent(choice.Get("delta.content")); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func collectResponsesSSEText(body []byte) string {
	var parts []string
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if bytes.Equal(payload, []byte("[DONE]")) || len(payload) == 0 || !gjson.ValidBytes(payload) {
			continue
		}
		switch gjson.GetBytes(payload, "type").String() {
		case "response.output_text.delta":
			if text := strings.TrimSpace(gjson.GetBytes(payload, "delta").String()); text != "" {
				parts = append(parts, text)
			}
		case "response.output_item.added", "response.output_item.done":
			if text := extractResponsesContent(gjson.GetBytes(payload, "item")); text != "" {
				parts = append(parts, text)
			}
		case "response.completed", "response.done":
			if text := extractResponsesContent(gjson.GetBytes(payload, "response")); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func collectAnthropicSSEText(body []byte) string {
	var parts []string
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) || !gjson.ValidBytes(payload) {
			continue
		}
		if gjson.GetBytes(payload, "type").String() == "content_block_delta" {
			if text := strings.TrimSpace(gjson.GetBytes(payload, "delta.text").String()); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func buildChatSessionRecordInput(
	apiKey *service.APIKey,
	account *service.Account,
	sessionKey string,
	requestID string,
	reqModel string,
	stream bool,
	requestType service.RequestType,
	httpStatus int,
	inboundEndpoint string,
	upstreamEndpoint string,
	requestedModel string,
	upstreamModel string,
) *service.ChatSessionRecordInput {
	if apiKey == nil {
		return nil
	}
	input := &service.ChatSessionRecordInput{
		SessionKey:     strings.TrimSpace(sessionKey),
		RequestID:      strings.TrimSpace(requestID),
		Platform:       resolveChatSessionPlatform(apiKey, account),
		Model:          strings.TrimSpace(reqModel),
		RequestType:    requestType,
		Stream:         stream,
		Status:         http.StatusText(httpStatus),
		HTTPStatusCode: httpStatus,
		CreatedAt:      time.Now(),
	}
	if input.Status == "" {
		input.Status = "completed"
	}
	if inbound := strings.TrimSpace(inboundEndpoint); inbound != "" {
		input.InboundEndpoint = &inbound
	}
	if upstream := strings.TrimSpace(upstreamEndpoint); upstream != "" {
		input.UpstreamEndpoint = &upstream
	}
	if requested := strings.TrimSpace(requestedModel); requested != "" {
		input.RequestedModel = &requested
	}
	if upstreamModel = strings.TrimSpace(upstreamModel); upstreamModel != "" {
		input.UpstreamModel = &upstreamModel
	}
	if apiKey.GroupID != nil {
		input.GroupID = apiKey.GroupID
	}
	if account != nil {
		input.AccountID = &account.ID
	}
	return input
}

func resolveChatSessionPlatform(apiKey *service.APIKey, account *service.Account) string {
	if account != nil && strings.TrimSpace(account.Platform) != "" {
		return strings.TrimSpace(account.Platform)
	}
	if apiKey != nil && apiKey.Group != nil && strings.TrimSpace(apiKey.Group.Platform) != "" {
		return strings.TrimSpace(apiKey.Group.Platform)
	}
	return ""
}

func keepLastInboundUserMessage(items []service.ChatMessageRecordInput) []service.ChatMessageRecordInput {
	if len(items) == 0 {
		return nil
	}
	return []service.ChatMessageRecordInput{items[len(items)-1]}
}

func keepLastAssistantMessage(items []service.ChatMessageRecordInput) []service.ChatMessageRecordInput {
	if len(items) == 0 {
		return nil
	}
	return []service.ChatMessageRecordInput{items[len(items)-1]}
}
