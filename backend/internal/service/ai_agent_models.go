package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	agentProtocolChatCompletions = "chat_completions"
	agentProtocolResponses       = "responses"
	agentProtocolMessages        = "messages"
)

func setAgentModelHeaders(request *http.Request, protocol, key string) {
	request.Header.Set("Content-Type", "application/json")
	if protocol == agentProtocolMessages {
		request.Header.Set("x-api-key", key)
		request.Header.Set("anthropic-version", "2023-06-01")
		return
	}
	request.Header.Set("Authorization", "Bearer "+key)
}

func (s *AIAgentService) sendModelRequest(ctx context.Context, config AIAgentConfig, key, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.modelBaseURL(config)+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setAgentModelHeaders(request, config.Protocol, key)
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call Agent model: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	return readAgentResponse(response, 4<<20)
}

func (s *AIAgentService) openAgentModelStream(ctx context.Context, config AIAgentConfig, key, path string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.modelBaseURL(config)+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setAgentModelHeaders(request, config.Protocol, key)
	request.Header.Set("Accept", "text/event-stream")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call Agent model: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer func() { _ = response.Body.Close() }()
		_, readErr := readAgentResponse(response, 4<<20)
		if readErr != nil {
			return nil, readErr
		}
		return nil, fmt.Errorf("upstream returned HTTP %d", response.StatusCode)
	}
	return response, nil
}

func agentSSEData(line string) string {
	if !strings.HasPrefix(line, "data:") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "data:"))
}

func (s *AIAgentService) streamResponses(ctx context.Context, config AIAgentConfig, key string, payload any, onTextDelta func(string)) (agentResponsesResult, error) {
	response, err := s.openAgentModelStream(ctx, config, key, "/v1/responses", payload)
	if err != nil {
		return agentResponsesResult{}, err
	}
	defer func() { _ = response.Body.Close() }()

	var result agentResponsesResult
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		data := agentSSEData(scanner.Text())
		if data == "" || data == "[DONE]" {
			continue
		}
		var event struct {
			Type     string `json:"type"`
			Delta    string `json:"delta"`
			Message  string `json:"message"`
			Response struct {
				Output []json.RawMessage `json:"output"`
				Usage  struct {
					InputTokens       int `json:"input_tokens"`
					InputTokenDetails struct {
						CachedTokens int `json:"cached_tokens"`
					} `json:"input_tokens_details"`
				} `json:"usage"`
			} `json:"response"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}
		switch event.Type {
		case "response.output_text.delta", "response.refusal.delta":
			if event.Delta != "" {
				onTextDelta(event.Delta)
			}
		case "response.completed":
			result = agentResponsesResult{Output: event.Response.Output, InputTokens: event.Response.Usage.InputTokens, CachedInputTokens: event.Response.Usage.InputTokenDetails.CachedTokens}
		case "response.failed", "error":
			message := event.Error.Message
			if message == "" {
				message = event.Message
			}
			if message == "" {
				message = "Agent Responses stream failed"
			}
			return agentResponsesResult{}, errors.New(message)
		}
	}
	if err := scanner.Err(); err != nil {
		return agentResponsesResult{}, fmt.Errorf("read Agent Responses stream: %w", err)
	}
	if len(result.Output) == 0 {
		return agentResponsesResult{}, errors.New("agent Responses stream ended without a completed response")
	}
	return result, nil
}

func (s *AIAgentService) completeChatCompletions(ctx context.Context, config AIAgentConfig, key string, history []agentModelMessage, onTextDelta func(string)) (agentModelMessage, error) {
	messages := make([]agentModelMessage, 0, len(history)+1)
	messages = append(messages, agentModelMessage{Role: "system", Content: agentSystemPrompt})
	messages = append(messages, history...)
	payload := map[string]any{
		"model":       config.Model,
		"messages":    messages,
		"tools":       agentTools,
		"tool_choice": "auto",
		"stream":      onTextDelta != nil,
	}
	if config.ThinkingMode != "" {
		payload["reasoning_effort"] = config.ThinkingMode
	}
	if onTextDelta != nil {
		return s.streamChatCompletions(ctx, config, key, payload, onTextDelta)
	}
	responseBody, err := s.sendModelRequest(ctx, config, key, "/v1/chat/completions", payload)
	if err != nil {
		return agentModelMessage{}, err
	}
	var completion agentCompletionResponse
	if err := json.Unmarshal(responseBody, &completion); err != nil || len(completion.Choices) == 0 {
		return agentModelMessage{}, errors.New("agent Chat Completions response is invalid")
	}
	return completion.Choices[0].Message, nil
}

func (s *AIAgentService) streamChatCompletions(ctx context.Context, config AIAgentConfig, key string, payload any, onTextDelta func(string)) (agentModelMessage, error) {
	response, err := s.openAgentModelStream(ctx, config, key, "/v1/chat/completions", payload)
	if err != nil {
		return agentModelMessage{}, err
	}
	defer func() { _ = response.Body.Close() }()
	message := agentModelMessage{Role: "assistant"}
	toolCalls := make(map[int]*agentToolCall)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		data := agentSSEData(scanner.Text())
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Error.Message != "" {
			return agentModelMessage{}, errors.New(chunk.Error.Message)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				message.Content = modelMessageText(message.Content) + choice.Delta.Content
				onTextDelta(choice.Delta.Content)
			}
			for _, delta := range choice.Delta.ToolCalls {
				call := toolCalls[delta.Index]
				if call == nil {
					call = &agentToolCall{Type: "function"}
					toolCalls[delta.Index] = call
				}
				if delta.ID != "" {
					call.ID = delta.ID
				}
				if delta.Type != "" {
					call.Type = delta.Type
				}
				call.Function.Name += delta.Function.Name
				call.Function.Arguments += delta.Function.Arguments
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return agentModelMessage{}, fmt.Errorf("read Agent Chat Completions stream: %w", err)
	}
	for index := 0; index < len(toolCalls); index++ {
		if call := toolCalls[index]; call != nil {
			message.ToolCalls = append(message.ToolCalls, *call)
		}
	}
	if strings.TrimSpace(modelMessageText(message.Content)) == "" && len(message.ToolCalls) == 0 {
		return agentModelMessage{}, errors.New("agent Chat Completions stream ended without content")
	}
	return message, nil
}

func agentResponsesPromptCacheKey(model string) string {
	tools, _ := json.Marshal(responsesTools())
	digest := sha256.Sum256([]byte(model + "\x00" + agentSystemPrompt + "\x00" + string(tools)))
	return fmt.Sprintf("sub2api-agent-%x", digest[:12])
}

type agentResponsesResult struct {
	Output            []json.RawMessage
	InputTokens       int
	CachedInputTokens int
}

func (s *AIAgentService) completeResponses(ctx context.Context, config AIAgentConfig, key string, history []agentModelMessage, onTextDelta func(string)) (agentModelMessage, error) {
	payload := map[string]any{
		"model":             config.Model,
		"instructions":      agentSystemPrompt,
		"input":             responsesInput(history),
		"tools":             responsesTools(),
		"tool_choice":       "auto",
		"max_output_tokens": 4096,
		"stream":            onTextDelta != nil,
		"prompt_cache_key":  agentResponsesPromptCacheKey(config.Model),
	}
	if config.ThinkingMode != "" {
		payload["reasoning"] = map[string]any{"effort": config.ThinkingMode, "summary": "auto"}
		payload["include"] = []string{"reasoning.encrypted_content"}
	}
	var result agentResponsesResult
	if onTextDelta == nil {
		responseBody, err := s.sendModelRequest(ctx, config, key, "/v1/responses", payload)
		if err != nil {
			return agentModelMessage{}, err
		}
		var response struct {
			Output []json.RawMessage `json:"output"`
			Usage  struct {
				InputTokens       int `json:"input_tokens"`
				InputTokenDetails struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"input_tokens_details"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(responseBody, &response); err != nil || len(response.Output) == 0 {
			return agentModelMessage{}, errors.New("agent Responses response is invalid")
		}
		result = agentResponsesResult{Output: response.Output, InputTokens: response.Usage.InputTokens, CachedInputTokens: response.Usage.InputTokenDetails.CachedTokens}
	} else {
		streamResult, err := s.streamResponses(ctx, config, key, payload, onTextDelta)
		if err != nil {
			return agentModelMessage{}, err
		}
		result = streamResult
	}
	message := agentModelMessage{Role: "assistant", ResponsesOutput: result.Output, InputTokens: result.InputTokens, CachedInputTokens: result.CachedInputTokens}
	var textParts []string
	for _, raw := range result.Output {
		var item struct {
			ID        string          `json:"id"`
			Type      string          `json:"type"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments string          `json:"arguments"`
			Content   json.RawMessage `json:"content"`
		}
		if json.Unmarshal(raw, &item) != nil {
			continue
		}
		switch item.Type {
		case "function_call":
			callID := item.CallID
			if callID == "" {
				callID = item.ID
			}
			message.ToolCalls = append(message.ToolCalls, agentToolCall{ID: callID, Type: "function", Function: agentToolFunction{Name: item.Name, Arguments: item.Arguments}})
		case "message":
			var blocks []struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Refusal string `json:"refusal"`
			}
			if json.Unmarshal(item.Content, &blocks) == nil {
				for _, block := range blocks {
					if block.Type == "output_text" && block.Text != "" {
						textParts = append(textParts, block.Text)
					} else if block.Type == "refusal" && block.Refusal != "" {
						textParts = append(textParts, block.Refusal)
					}
				}
			}
		}
	}
	message.Content = strings.Join(textParts, "\n")
	if len(message.ToolCalls) == 0 && strings.TrimSpace(strings.Join(textParts, "")) == "" {
		return agentModelMessage{}, errors.New("agent Responses response contained no text or tool calls")
	}
	return message, nil
}

func responsesInput(history []agentModelMessage) []any {
	input := make([]any, 0, len(history))
	for _, message := range history {
		switch message.Role {
		case "user":
			input = append(input, map[string]any{"role": "user", "content": modelMessageText(message.Content)})
		case "assistant":
			if len(message.ResponsesOutput) > 0 {
				for _, item := range message.ResponsesOutput {
					input = append(input, item)
				}
				continue
			}
			if content := modelMessageText(message.Content); content != "" {
				input = append(input, map[string]any{"role": "assistant", "content": content})
			}
			for _, call := range message.ToolCalls {
				input = append(input, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Function.Name, "arguments": call.Function.Arguments})
			}
		case "tool":
			input = append(input, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": modelMessageText(message.Content)})
		}
	}
	return input
}

func responsesTools() []map[string]any {
	tools := make([]map[string]any, 0, len(agentTools))
	for _, tool := range agentTools {
		function, _ := tool["function"].(map[string]any)
		tools = append(tools, map[string]any{
			"type":        "function",
			"name":        function["name"],
			"description": function["description"],
			"parameters":  function["parameters"],
		})
	}
	return tools
}

func (s *AIAgentService) completeMessages(ctx context.Context, config AIAgentConfig, key string, history []agentModelMessage, onTextDelta func(string)) (agentModelMessage, error) {
	maxTokens := 4096
	payload := map[string]any{
		"model":       config.Model,
		"system":      agentSystemPrompt,
		"messages":    anthropicMessages(history),
		"tools":       anthropicTools(),
		"tool_choice": map[string]any{"type": "auto"},
		"stream":      onTextDelta != nil,
	}
	if config.ThinkingMode != "" {
		if budget, err := strconv.Atoi(config.ThinkingMode); err == nil {
			if budget < 1024 || budget > 128000 {
				return agentModelMessage{}, errors.New("messages thinking budget must be between 1024 and 128000 tokens")
			}
			payload["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
			maxTokens = budget + 4096
		} else {
			payload["thinking"] = map[string]any{"type": config.ThinkingMode}
			maxTokens = 16384
		}
	}
	payload["max_tokens"] = maxTokens
	if onTextDelta != nil {
		return s.streamMessages(ctx, config, key, payload, onTextDelta)
	}
	responseBody, err := s.sendModelRequest(ctx, config, key, "/v1/messages", payload)
	if err != nil {
		return agentModelMessage{}, err
	}
	var response struct {
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil || len(response.Content) == 0 {
		return agentModelMessage{}, errors.New("agent Messages response is invalid")
	}
	message := agentModelMessage{Role: "assistant", AnthropicContent: response.Content}
	var textParts []string
	for _, raw := range response.Content {
		var block struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		if json.Unmarshal(raw, &block) != nil {
			continue
		}
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case "tool_use":
			arguments := string(block.Input)
			if arguments == "" {
				arguments = "{}"
			}
			message.ToolCalls = append(message.ToolCalls, agentToolCall{ID: block.ID, Type: "function", Function: agentToolFunction{Name: block.Name, Arguments: arguments}})
		}
	}
	message.Content = strings.Join(textParts, "\n")
	if len(message.ToolCalls) == 0 && strings.TrimSpace(strings.Join(textParts, "")) == "" {
		return agentModelMessage{}, errors.New("agent Messages response contained no text or tool calls")
	}
	return message, nil
}

func (s *AIAgentService) streamMessages(ctx context.Context, config AIAgentConfig, key string, payload any, onTextDelta func(string)) (agentModelMessage, error) {
	response, err := s.openAgentModelStream(ctx, config, key, "/v1/messages", payload)
	if err != nil {
		return agentModelMessage{}, err
	}
	defer func() { _ = response.Body.Close() }()
	blocks := make(map[int]map[string]any)
	inputJSON := make(map[int]string)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		data := agentSSEData(scanner.Text())
		if data == "" || data == "[DONE]" {
			continue
		}
		var event struct {
			Type         string         `json:"type"`
			Index        int            `json:"index"`
			ContentBlock map[string]any `json:"content_block"`
			Delta        struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				Signature   string `json:"signature"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}
		switch event.Type {
		case "content_block_start":
			blocks[event.Index] = cloneAgentMap(event.ContentBlock)
		case "content_block_delta":
			block := blocks[event.Index]
			if block == nil {
				block = make(map[string]any)
				blocks[event.Index] = block
			}
			switch event.Delta.Type {
			case "text_delta":
				block["text"] = agentInputString(block["text"]) + event.Delta.Text
				if event.Delta.Text != "" {
					onTextDelta(event.Delta.Text)
				}
			case "thinking_delta":
				block["thinking"] = agentInputString(block["thinking"]) + event.Delta.Thinking
			case "signature_delta":
				block["signature"] = agentInputString(block["signature"]) + event.Delta.Signature
			case "input_json_delta":
				inputJSON[event.Index] += event.Delta.PartialJSON
			}
		case "error":
			if event.Error.Message == "" {
				event.Error.Message = "Agent Messages stream failed"
			}
			return agentModelMessage{}, errors.New(event.Error.Message)
		}
	}
	if err := scanner.Err(); err != nil {
		return agentModelMessage{}, fmt.Errorf("read Agent Messages stream: %w", err)
	}
	message := agentModelMessage{Role: "assistant"}
	for index := 0; index < len(blocks); index++ {
		block := blocks[index]
		if block == nil {
			continue
		}
		if partial := inputJSON[index]; partial != "" {
			var input any
			if json.Unmarshal([]byte(partial), &input) != nil {
				return agentModelMessage{}, errors.New("agent Messages tool input stream is invalid")
			}
			block["input"] = input
		}
		raw, marshalErr := json.Marshal(block)
		if marshalErr != nil {
			return agentModelMessage{}, marshalErr
		}
		message.AnthropicContent = append(message.AnthropicContent, raw)
		switch agentInputString(block["type"]) {
		case "text":
			text := agentInputString(block["text"])
			if text != "" {
				if modelMessageText(message.Content) != "" {
					message.Content = modelMessageText(message.Content) + "\n"
				}
				message.Content = modelMessageText(message.Content) + text
			}
		case "tool_use":
			arguments, _ := json.Marshal(block["input"])
			message.ToolCalls = append(message.ToolCalls, agentToolCall{ID: agentInputString(block["id"]), Type: "function", Function: agentToolFunction{Name: agentInputString(block["name"]), Arguments: string(arguments)}})
		}
	}
	if strings.TrimSpace(modelMessageText(message.Content)) == "" && len(message.ToolCalls) == 0 {
		return agentModelMessage{}, errors.New("agent Messages stream ended without content")
	}
	return message, nil
}

func anthropicMessages(history []agentModelMessage) []map[string]any {
	messages := make([]map[string]any, 0, len(history))
	appendMessage := func(role string, blocks []any) {
		if len(blocks) == 0 {
			return
		}
		if len(messages) > 0 && messages[len(messages)-1]["role"] == role {
			existing, _ := messages[len(messages)-1]["content"].([]any)
			messages[len(messages)-1]["content"] = append(existing, blocks...)
			return
		}
		messages = append(messages, map[string]any{"role": role, "content": blocks})
	}
	for _, message := range history {
		switch message.Role {
		case "user":
			appendMessage("user", []any{map[string]any{"type": "text", "text": modelMessageText(message.Content)}})
		case "assistant":
			if len(message.AnthropicContent) > 0 {
				blocks := make([]any, 0, len(message.AnthropicContent))
				for _, block := range message.AnthropicContent {
					blocks = append(blocks, block)
				}
				appendMessage("assistant", blocks)
				continue
			}
			blocks := make([]any, 0, len(message.ToolCalls)+1)
			if content := modelMessageText(message.Content); content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": content})
			}
			for _, call := range message.ToolCalls {
				var input any
				if json.Unmarshal([]byte(call.Function.Arguments), &input) != nil {
					input = map[string]any{}
				}
				blocks = append(blocks, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Function.Name, "input": input})
			}
			appendMessage("assistant", blocks)
		case "tool":
			appendMessage("user", []any{map[string]any{"type": "tool_result", "tool_use_id": message.ToolCallID, "content": modelMessageText(message.Content)}})
		}
	}
	return messages
}

func anthropicTools() []map[string]any {
	tools := make([]map[string]any, 0, len(agentTools))
	for _, tool := range agentTools {
		function, _ := tool["function"].(map[string]any)
		tools = append(tools, map[string]any{
			"name":         function["name"],
			"description":  function["description"],
			"input_schema": function["parameters"],
		})
	}
	return tools
}

func modelMessageText(content any) string {
	if content == nil {
		return ""
	}
	if text, ok := content.(string); ok {
		return text
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return fmt.Sprint(content)
	}
	return string(encoded)
}
