package apicompat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ResponsesToAnthropicRequest converts a Responses API request into an
// Anthropic Messages request. This is the reverse of AnthropicToResponses and
// enables Anthropic platform groups to accept OpenAI Responses API requests
// by converting them to the native /v1/messages format before forwarding upstream.
func ResponsesToAnthropicRequest(req *ResponsesRequest) (*AnthropicRequest, error) {
	system, messages, err := convertResponsesInputToAnthropic(req.Instructions, req.Input)
	if err != nil {
		return nil, err
	}

	out := &AnthropicRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}

	if len(system) > 0 {
		out.System = system
	}

	// max_output_tokens → max_tokens
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		out.MaxTokens = *req.MaxOutputTokens
	}
	if out.MaxTokens == 0 {
		// Anthropic requires max_tokens; default to a sensible value.
		out.MaxTokens = 8192
	}

	// Convert tools
	if len(req.Tools) > 0 {
		out.Tools = convertResponsesToAnthropicTools(req.Tools)
	}

	// Convert tool_choice (reverse of convertAnthropicToolChoiceToResponses)
	if len(req.ToolChoice) > 0 {
		tc, err := convertResponsesToAnthropicToolChoice(req.ToolChoice)
		if err != nil {
			return nil, fmt.Errorf("convert tool_choice: %w", err)
		}
		out.ToolChoice = tc
	}

	// reasoning.effort → output_config.effort + thinking
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		effort := mapResponsesEffortToAnthropic(req.Reasoning.Effort)
		out.OutputConfig = &AnthropicOutputConfig{Effort: effort}
		// Enable thinking for non-low efforts
		if effort != "low" {
			out.Thinking = &AnthropicThinking{
				Type:         "enabled",
				BudgetTokens: defaultThinkingBudget(effort),
			}
		}
	}

	return out, nil
}

// defaultThinkingBudget returns a sensible thinking budget based on effort level.
func defaultThinkingBudget(effort string) int {
	switch effort {
	case "low":
		return 1024
	case "medium":
		return 4096
	case "high":
		return 10240
	case "max":
		return 32768
	default:
		return 10240
	}
}

// mapResponsesEffortToAnthropic converts OpenAI Responses reasoning effort to
// Anthropic effort levels. Reverse of mapAnthropicEffortToResponses.
//
//	low    → low
//	medium → medium
//	high   → high
//	xhigh  → max
func mapResponsesEffortToAnthropic(effort string) string {
	if effort == "xhigh" {
		return "max"
	}
	return effort // low→low, medium→medium, high→high, unknown→passthrough
}

// convertResponsesInputToAnthropic extracts system prompt and messages from
// a Responses API instructions + input array. Returns the system as raw JSON
// (for Anthropic's polymorphic system field) and a list of Anthropic messages.
func convertResponsesInputToAnthropic(instructions string, inputRaw json.RawMessage) (json.RawMessage, []AnthropicMessage, error) {
	var systemParts []string
	if strings.TrimSpace(instructions) != "" {
		systemParts = append(systemParts, strings.TrimSpace(instructions))
	}

	// Try as plain string input.
	var inputStr string
	if err := json.Unmarshal(inputRaw, &inputStr); err == nil {
		content, _ := json.Marshal(inputStr)
		var system json.RawMessage
		if len(systemParts) > 0 {
			system, _ = json.Marshal(strings.Join(systemParts, "\n\n"))
		}
		return system, []AnthropicMessage{{Role: "user", Content: content}}, nil
	}

	var rawItems []json.RawMessage
	trimmedInput := bytes.TrimSpace(inputRaw)
	switch {
	case len(trimmedInput) == 0:
		return nil, nil, fmt.Errorf("parse responses input: empty input")
	case trimmedInput[0] == '[':
		if err := json.Unmarshal(trimmedInput, &rawItems); err != nil {
			return nil, nil, fmt.Errorf("parse responses input: %w", err)
		}
	case trimmedInput[0] == '{':
		rawItems = []json.RawMessage{trimmedInput}
	default:
		return nil, nil, fmt.Errorf("parse responses input: expected string, object, or array")
	}

	var messages []AnthropicMessage

	for _, rawItem := range rawItems {
		var item ResponsesInputItem
		if err := json.Unmarshal(rawItem, &item); err != nil {
			return nil, nil, fmt.Errorf("parse responses input item: %w", err)
		}

		switch {
		case item.Role == "system" || item.Role == "developer":
			text := extractTextFromContent(item.Content)
			if text != "" {
				systemParts = append(systemParts, text)
			}

		case item.Type == "input_text" || item.Type == "text":
			blocks := convertResponsesContentPartToAnthropicRawBlocks(rawItem, false)
			if len(blocks) > 0 {
				content, _ := marshalAnthropicRawBlocks(blocks...)
				messages = append(messages, AnthropicMessage{
					Role:    "user",
					Content: content,
				})
			}

		case item.Type == "input_image" || item.Type == "image_url":
			blocks := convertResponsesContentPartToAnthropicRawBlocks(rawItem, false)
			if len(blocks) > 0 {
				content, _ := marshalAnthropicRawBlocks(blocks...)
				messages = append(messages, AnthropicMessage{
					Role:    "user",
					Content: content,
				})
			}

		case item.Type == "function_call":
			// function_call → assistant message with tool_use block
			input := json.RawMessage("{}")
			if item.Arguments != "" {
				input = json.RawMessage(item.Arguments)
			}
			block := AnthropicContentBlock{
				Type:  "tool_use",
				ID:    fromResponsesCallIDToAnthropic(item.CallID),
				Name:  item.Name,
				Input: input,
			}
			blockJSON, _ := json.Marshal([]AnthropicContentBlock{block})
			messages = append(messages, AnthropicMessage{
				Role:    "assistant",
				Content: blockJSON,
			})

		case item.Type == "function_call_output":
			// function_call_output → user message with tool_result block
			contentJSON := responsesFunctionOutputToAnthropicContent(item)
			block := AnthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: fromResponsesCallIDToAnthropic(item.CallID),
				Content:   contentJSON,
			}
			blockJSON, _ := json.Marshal([]AnthropicContentBlock{block})
			messages = append(messages, AnthropicMessage{
				Role:    "user",
				Content: blockJSON,
			})

		case item.Role == "user":
			content, err := convertResponsesUserToAnthropicContent(item.Content)
			if err != nil {
				return nil, nil, err
			}
			messages = append(messages, AnthropicMessage{
				Role:    "user",
				Content: content,
			})

		case item.Role == "assistant":
			content, err := convertResponsesAssistantToAnthropicContent(item.Content)
			if err != nil {
				return nil, nil, err
			}
			messages = append(messages, AnthropicMessage{
				Role:    "assistant",
				Content: content,
			})

		default:
			// Unknown role/type — attempt as user message
			if item.Content != nil {
				content, err := convertResponsesUserToAnthropicContent(item.Content)
				if err != nil {
					return nil, nil, err
				}
				messages = append(messages, AnthropicMessage{
					Role:    "user",
					Content: content,
				})
			}
		}
	}

	// Repair tool_use/tool_result pairing, then merge consecutive same-role
	// messages (Anthropic requires alternating roles). The first merge groups
	// parallel calls (and their results) so the pairing pass sees them together;
	// the pairing pass may re-split a user turn (e.g. when an injected message
	// sat between a call and its output), so a second merge restores alternation.
	messages = mergeConsecutiveMessages(messages)
	messages = normalizeAnthropicToolPairing(messages)
	messages = mergeConsecutiveMessages(messages)

	var system json.RawMessage
	if len(systemParts) > 0 {
		system, _ = json.Marshal(strings.Join(systemParts, "\n\n"))
	}

	return system, messages, nil
}

func responsesFunctionOutputToAnthropicContent(item ResponsesInputItem) json.RawMessage {
	if len(item.outputRaw) == 0 {
		output := item.Output
		if output == "" {
			output = "(empty)"
		}
		content, _ := json.Marshal(output)
		return content
	}

	var parts []ResponsesContentPart
	if err := json.Unmarshal(item.outputRaw, &parts); err == nil {
		blocks := make([]AnthropicContentBlock, 0, len(parts))
		for _, part := range parts {
			switch part.Type {
			case "input_text", "output_text", "text":
				if part.Text != "" {
					blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: part.Text})
				}
			case "input_image":
				if source := dataURIToAnthropicImageSource(part.ImageURL); source != nil {
					blocks = append(blocks, AnthropicContentBlock{Type: "image", Source: source})
				}
			}
		}
		if len(blocks) > 0 {
			content, _ := json.Marshal(blocks)
			return content
		}
		if len(parts) == 0 {
			content, _ := json.Marshal("(empty)")
			return content
		}
	}

	content, _ := json.Marshal(item.Output)
	return content
}

// normalizeAnthropicToolPairing rebuilds the message sequence so it satisfies
// Anthropic's tool_use/tool_result invariants, which the naive item-by-item
// conversion violates whenever the Responses history interleaves anything
// between a function_call and its function_call_output:
//
//   - every tool_result block must have a matching tool_use in the immediately
//     preceding assistant message ("tool_result ... must have a corresponding
//     tool_use block in the previous message");
//   - every tool_use block must be answered by a tool_result in the immediately
//     following user message (Anthropic rejects unanswered tool_use ids);
//   - user/assistant turns must alternate.
//
// codex (Responses, store:false) re-sends the whole history each turn and
// frequently injects items between a call and its output — a developer/approval
// notice, or a sibling parallel call whose output never arrived. The unrepaired
// converter emits each function_call as its own assistant message and each
// output as its own user message, so any such interleaving breaks
// tool_use↔tool_result adjacency and yields an upstream 400.
//
// The repair indexes every tool_result by its tool_use id, then for each
// assistant message carrying tool_use blocks keeps only the answered ones
// (dropping unanswered/dangling calls — and the assistant message entirely if it
// has no other content) and emits the matching tool_result blocks, in call
// order, as the very next user message. Standalone tool_result blocks are
// dropped from their original position (re-emitted adjacent to their call);
// orphan tool_results with no announcing tool_use are dropped. Non-tool content
// passes through in place. This mirrors normalizeChatMessages on the
// Responses→Chat path.
func normalizeAnthropicToolPairing(messages []AnthropicMessage) []AnthropicMessage {
	// Index every tool_result block by its tool_use id (last wins on dup).
	results := make(map[string]AnthropicContentBlock)
	for _, m := range messages {
		if m.Role != "user" {
			continue
		}
		for _, b := range parseContentBlocks(m.Content) {
			if b.Type == "tool_result" && b.ToolUseID != "" {
				results[b.ToolUseID] = b
			}
		}
	}

	out := make([]AnthropicMessage, 0, len(messages))
	for _, m := range messages {
		blocks := parseContentBlocks(m.Content)
		switch m.Role {
		case "assistant":
			var toolUses, others []AnthropicContentBlock
			for _, b := range blocks {
				if b.Type == "tool_use" {
					toolUses = append(toolUses, b)
				} else {
					others = append(others, b)
				}
			}
			if len(toolUses) == 0 {
				out = append(out, m)
				continue
			}
			kept := make([]AnthropicContentBlock, 0, len(toolUses))
			for _, tu := range toolUses {
				if _, ok := results[tu.ID]; ok {
					kept = append(kept, tu)
				}
			}
			if len(kept) == 0 {
				// No answered calls: keep any non-tool content, else drop.
				if len(others) > 0 {
					out = append(out, anthropicMessageFromBlocks("assistant", others))
				}
				continue
			}
			asstBlocks := make([]AnthropicContentBlock, 0, len(others)+len(kept))
			asstBlocks = append(asstBlocks, others...)
			asstBlocks = append(asstBlocks, kept...)
			out = append(out, anthropicMessageFromBlocks("assistant", asstBlocks))

			resBlocks := make([]AnthropicContentBlock, 0, len(kept))
			for _, tu := range kept {
				resBlocks = append(resBlocks, results[tu.ID])
			}
			out = append(out, anthropicMessageFromBlocks("user", resBlocks))

		case "user":
			var nonResult []AnthropicContentBlock
			hasResult := false
			for _, b := range blocks {
				if b.Type == "tool_result" {
					hasResult = true
					continue
				}
				nonResult = append(nonResult, b)
			}
			if !hasResult {
				out = append(out, m)
				continue
			}
			// The tool_result blocks are re-emitted next to their call; keep any
			// other content of this user turn in place, drop it if there is none.
			if len(nonResult) > 0 {
				out = append(out, anthropicMessageFromBlocks("user", nonResult))
			}

		default:
			out = append(out, m)
		}
	}
	return out
}

// anthropicMessageFromBlocks builds an AnthropicMessage whose content is the
// marshaled block array.
func anthropicMessageFromBlocks(role string, blocks []AnthropicContentBlock) AnthropicMessage {
	content, _ := json.Marshal(blocks)
	return AnthropicMessage{Role: role, Content: content}
}

// extractTextFromContent extracts text from a content field that may be a
// plain string or an array of content parts.
func extractTextFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var texts []string
		for _, p := range parts {
			if (p.Type == "input_text" || p.Type == "output_text" || p.Type == "text") && p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n\n")
	}
	return ""
}

// convertResponsesUserToAnthropicContent converts a Responses user message
// content field into Anthropic content blocks JSON.
func convertResponsesUserToAnthropicContent(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.Marshal("") // empty string content
	}

	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal(s)
	}

	blocks := convertResponsesContentToAnthropicRawBlocks(raw, false)

	if len(blocks) == 0 {
		return json.Marshal("")
	}
	return marshalAnthropicRawBlocks(blocks...)
}

// convertResponsesAssistantToAnthropicContent converts a Responses assistant
// message content field into Anthropic content blocks JSON.
func convertResponsesAssistantToAnthropicContent(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.Marshal([]AnthropicContentBlock{{Type: "text", Text: ""}})
	}

	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal([]AnthropicContentBlock{{Type: "text", Text: s}})
	}

	blocks := convertResponsesContentToAnthropicRawBlocks(raw, true)

	if len(blocks) == 0 {
		blocks = append(blocks, makeAnthropicTextRawBlock("", nil))
	}
	return marshalAnthropicRawBlocks(blocks...)
}

func convertResponsesContentToAnthropicRawBlocks(raw json.RawMessage, assistant bool) []json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}

	var parts []json.RawMessage
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(trimmed, &parts); err != nil {
			return nil
		}
	case '{':
		parts = []json.RawMessage{trimmed}
	default:
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return []json.RawMessage{makeAnthropicTextRawBlock(s, nil)}
		}
		return nil
	}

	var blocks []json.RawMessage
	for _, part := range parts {
		blocks = append(blocks, convertResponsesContentPartToAnthropicRawBlocks(part, assistant)...)
	}
	return blocks
}

func convertResponsesContentPartToAnthropicRawBlocks(raw json.RawMessage, assistant bool) []json.RawMessage {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []json.RawMessage{makeAnthropicTextRawBlock(s, nil)}
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}

	partType := rawJSONFieldString(obj, "type")
	switch partType {
	case "input_text", "output_text", "text":
		if text := rawJSONFieldString(obj, "text"); text != "" {
			return []json.RawMessage{makeAnthropicTextRawBlock(text, obj["cache_control"])}
		}
	case "input_image", "image_url":
		if source := responsesImageSourceFromObject(obj); source != nil {
			return []json.RawMessage{makeAnthropicImageRawBlock(source, obj["cache_control"])}
		}
	case "image":
		if _, ok := obj["source"]; ok {
			return []json.RawMessage{raw}
		}
		if source := responsesImageSourceFromObject(obj); source != nil {
			return []json.RawMessage{makeAnthropicImageRawBlock(source, obj["cache_control"])}
		}
	case "tool_use":
		if assistant {
			return []json.RawMessage{raw}
		}
	case "tool_result":
		if !assistant {
			return []json.RawMessage{raw}
		}
	case "thinking":
		if assistant {
			return []json.RawMessage{raw}
		}
	case "document", "server_tool_use", "tool_reference", "web_search_tool_result":
		return []json.RawMessage{raw}
	}

	if text := rawJSONFieldString(obj, "text"); text != "" {
		return []json.RawMessage{makeAnthropicTextRawBlock(text, obj["cache_control"])}
	}
	return nil
}

func marshalAnthropicRawBlocks(blocks ...json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(blocks)
}

func makeAnthropicTextRawBlock(text string, cacheControl json.RawMessage) json.RawMessage {
	obj := map[string]json.RawMessage{
		"type": json.RawMessage(`"text"`),
		"text": mustMarshalRaw(text),
	}
	if len(cacheControl) > 0 {
		obj["cache_control"] = cacheControl
	}
	raw, _ := json.Marshal(obj)
	return raw
}

func makeAnthropicImageRawBlock(source *AnthropicImageSource, cacheControl json.RawMessage) json.RawMessage {
	obj := map[string]json.RawMessage{
		"type":   json.RawMessage(`"image"`),
		"source": mustMarshalRaw(source),
	}
	if len(cacheControl) > 0 {
		obj["cache_control"] = cacheControl
	}
	raw, _ := json.Marshal(obj)
	return raw
}

func responsesImageSourceFromObject(obj map[string]json.RawMessage) *AnthropicImageSource {
	if rawSource, ok := obj["source"]; ok {
		var source AnthropicImageSource
		if err := json.Unmarshal(rawSource, &source); err == nil && source.Type != "" {
			return &source
		}
	}

	if url := rawJSONFieldString(obj, "image_url"); url != "" {
		return dataURIToAnthropicImageSource(url)
	}

	var imageURLObj map[string]json.RawMessage
	if rawImageURL, ok := obj["image_url"]; ok && json.Unmarshal(rawImageURL, &imageURLObj) == nil {
		if url := rawJSONFieldString(imageURLObj, "url"); url != "" {
			return dataURIToAnthropicImageSource(url)
		}
	}

	if url := rawJSONFieldString(obj, "url"); url != "" {
		return dataURIToAnthropicImageSource(url)
	}
	return nil
}

func rawJSONFieldString(obj map[string]json.RawMessage, key string) string {
	raw, ok := obj[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func mustMarshalRaw(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

// fromResponsesCallIDToAnthropic converts an OpenAI function call ID back to
// Anthropic format. Reverses toResponsesCallID.
func fromResponsesCallIDToAnthropic(id string) string {
	// If it has our "fc_" prefix wrapping a known Anthropic prefix, strip it
	if after, ok := strings.CutPrefix(id, "fc_"); ok {
		if strings.HasPrefix(after, "toolu_") || strings.HasPrefix(after, "call_") {
			return after
		}
	}
	// Generate a synthetic Anthropic tool ID
	if !strings.HasPrefix(id, "toolu_") && !strings.HasPrefix(id, "call_") {
		return "toolu_" + id
	}
	return id
}

// dataURIToAnthropicImageSource parses a data URI into an AnthropicImageSource.
func dataURIToAnthropicImageSource(dataURI string) *AnthropicImageSource {
	if !strings.HasPrefix(dataURI, "data:") {
		return nil
	}
	// Format: data:<media_type>;base64,<data>
	rest := strings.TrimPrefix(dataURI, "data:")
	semicolonIdx := strings.Index(rest, ";")
	if semicolonIdx < 0 {
		return nil
	}
	mediaType := rest[:semicolonIdx]
	rest = rest[semicolonIdx+1:]
	if !strings.HasPrefix(rest, "base64,") {
		return nil
	}
	data := strings.TrimPrefix(rest, "base64,")
	return &AnthropicImageSource{
		Type:      "base64",
		MediaType: mediaType,
		Data:      data,
	}
}

// mergeConsecutiveMessages merges consecutive messages with the same role
// because Anthropic requires alternating user/assistant turns.
func mergeConsecutiveMessages(messages []AnthropicMessage) []AnthropicMessage {
	if len(messages) <= 1 {
		return messages
	}

	var merged []AnthropicMessage
	for _, msg := range messages {
		if len(merged) == 0 || merged[len(merged)-1].Role != msg.Role {
			merged = append(merged, msg)
			continue
		}

		// Same role — merge content arrays
		last := &merged[len(merged)-1]
		lastBlocks := parseContentBlocks(last.Content)
		newBlocks := parseContentBlocks(msg.Content)
		combined := append(lastBlocks, newBlocks...)
		last.Content, _ = json.Marshal(combined)
	}
	return merged
}

// parseContentBlocks attempts to parse content as []AnthropicContentBlock.
// If it's a string, wraps it in a text block.
func parseContentBlocks(raw json.RawMessage) []AnthropicContentBlock {
	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return blocks
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []AnthropicContentBlock{{Type: "text", Text: s}}
	}
	return nil
}

// convertResponsesToAnthropicTools maps Responses API tools to Anthropic format.
// Reverse of convertAnthropicToolsToResponses.
func convertResponsesToAnthropicTools(tools []ResponsesTool) []AnthropicTool {
	var out []AnthropicTool
	for _, t := range tools {
		switch t.Type {
		case "web_search", "google_search", "web_search_20250305":
			out = append(out, AnthropicTool{
				Type: "web_search_20250305",
				Name: "web_search",
			})
		case "function":
			out = append(out, AnthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
			})
		case "custom":
			out = append(out, AnthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
			})
		default:
			// Pass through unknown tool types
			out = append(out, AnthropicTool{
				Type:        t.Type,
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
			})
		}
	}
	return out
}

// normalizeAnthropicInputSchema ensures input_schema is a valid object schema.
func normalizeAnthropicInputSchema(schema json.RawMessage) json.RawMessage {
	const emptyObjectSchema = `{"type":"object","properties":{}}`

	trimmed := strings.TrimSpace(string(schema))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage(emptyObjectSchema)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(schema, &m); err != nil {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}

	typeRaw, ok := m["type"]
	if !ok || strings.TrimSpace(string(typeRaw)) == "" || string(typeRaw) == "null" {
		m["type"] = json.RawMessage(`"object"`)
	} else {
		var typ string
		if err := json.Unmarshal(typeRaw, &typ); err != nil || typ != "object" {
			return json.RawMessage(emptyObjectSchema)
		}
	}

	if _, ok := m["properties"]; !ok {
		m["properties"] = json.RawMessage(`{}`)
	}

	out, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(emptyObjectSchema)
	}
	return out
}

// convertResponsesToAnthropicToolChoice maps Responses tool_choice to Anthropic format.
// Reverse of convertAnthropicToolChoiceToResponses.
//
//	"auto"                                     → {"type":"auto"}
//	"required"                                 → {"type":"any"}
//	"none"                                     → {"type":"none"}
//	{"type":"function","name":"X"}                 → {"type":"tool","name":"X"}
//	{"type":"function","function":{"name":"X"}}     → {"type":"tool","name":"X"} // legacy
func convertResponsesToAnthropicToolChoice(raw json.RawMessage) (json.RawMessage, error) {
	// Try as string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto":
			return json.Marshal(map[string]string{"type": "auto"})
		case "required":
			return json.Marshal(map[string]string{"type": "any"})
		case "none":
			return json.Marshal(map[string]string{"type": "none"})
		default:
			return raw, nil
		}
	}

	// Try as object with type=function
	var tc struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &tc); err == nil && tc.Type == "function" {
		name := strings.TrimSpace(tc.Name)
		if name == "" {
			name = strings.TrimSpace(tc.Function.Name)
		}
		if name == "" {
			return raw, nil
		}
		return json.Marshal(map[string]string{
			"type": "tool",
			"name": name,
		})
	}

	// Pass through unknown
	return raw, nil
}
