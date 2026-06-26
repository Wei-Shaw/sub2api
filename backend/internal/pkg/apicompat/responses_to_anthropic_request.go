package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ResponsesToAnthropicRequest converts a Responses API request into an
// Anthropic Messages request. This is the reverse of AnthropicToResponses and
// enables Anthropic platform groups to accept OpenAI Responses API requests
// by converting them to the native /v1/messages format before forwarding upstream.
func ResponsesToAnthropicRequest(req *ResponsesRequest) (*AnthropicRequest, error) {
	system, messages, err := convertResponsesInputToAnthropic(req.Input)
	if err != nil {
		return nil, err
REDACTED

	out := &AnthropicRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
REDACTED

	if len(system) > 0 {
		out.System = system
REDACTED

	// max_output_tokens → max_tokens
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		out.MaxTokens = *req.MaxOutputTokens
REDACTED
	if out.MaxTokens == 0 {
		// Anthropic requires max_tokens; default to a sensible value.
		out.MaxTokens = 8192
REDACTED

	// Convert tools
	if len(req.Tools) > 0 {
		out.Tools = convertResponsesToAnthropicTools(req.Tools)
REDACTED

	// Convert tool_choice (reverse of convertAnthropicToolChoiceToResponses)
	if len(req.ToolChoice) > 0 {
		tc, err := convertResponsesToAnthropicToolChoice(req.ToolChoice)
		if err != nil {
			return nil, fmt.Errorf("convert tool_choice: %w", err)
	REDACTED
		out.ToolChoice = tc
REDACTED

	// reasoning.effort → output_config.effort + thinking
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		effort := mapResponsesEffortToAnthropic(req.Reasoning.Effort)
		out.OutputConfig = &AnthropicOutputConfig{Effort: effortREDACTED
		// Enable thinking for non-low efforts
		if effort != "low" {
			out.Thinking = &AnthropicThinking{
				Type:         "enabled",
				BudgetTokens: defaultThinkingBudget(effort),
		REDACTED
	REDACTED
REDACTED

	return out, nil
REDACTED

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
REDACTED
REDACTED

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
REDACTED
	return effort // low→low, medium→medium, high→high, unknown→passthrough
REDACTED

// convertResponsesInputToAnthropic extracts system prompt and messages from
// a Responses API input array. Returns the system as raw JSON (for Anthropic's
// polymorphic system field) and a list of Anthropic messages.
func convertResponsesInputToAnthropic(inputRaw json.RawMessage) (json.RawMessage, []AnthropicMessage, error) {
	// Try as plain string input.
	var inputStr string
	if err := json.Unmarshal(inputRaw, &inputStr); err == nil {
		content, _ := json.Marshal(inputStr)
		return nil, []AnthropicMessage{{Role: "user", Content: contentREDACTEDREDACTED, nil
REDACTED

	var items []ResponsesInputItem
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return nil, nil, fmt.Errorf("parse responses input: %w", err)
REDACTED

	var system json.RawMessage
	var messages []AnthropicMessage

	for _, item := range items {
		switch {
		case item.Role == "system":
			// System prompt → Anthropic system field
			text := extractTextFromContent(item.Content)
			if text != "" {
				system, _ = json.Marshal(text)
		REDACTED

		case item.Type == "function_call":
			// function_call → assistant message with tool_use block
			input := json.RawMessage("{REDACTED")
			if item.Arguments != "" {
				input = json.RawMessage(item.Arguments)
		REDACTED
			block := AnthropicContentBlock{
				Type:  "tool_use",
				ID:    fromResponsesCallIDToAnthropic(item.CallID),
				Name:  item.Name,
				Input: input,
		REDACTED
			blockJSON, _ := json.Marshal([]AnthropicContentBlock{blockREDACTED)
			messages = append(messages, AnthropicMessage{
				Role:    "assistant",
				Content: blockJSON,
		REDACTED)

		case item.Type == "function_call_output":
			// function_call_output → user message with tool_result block
			outputContent := item.Output
			if outputContent == "" {
				outputContent = "(empty)"
		REDACTED
			contentJSON, _ := json.Marshal(outputContent)
			block := AnthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: fromResponsesCallIDToAnthropic(item.CallID),
				Content:   contentJSON,
		REDACTED
			blockJSON, _ := json.Marshal([]AnthropicContentBlock{blockREDACTED)
			messages = append(messages, AnthropicMessage{
				Role:    "user",
				Content: blockJSON,
		REDACTED)

		case item.Role == "user":
			content, err := convertResponsesUserToAnthropicContent(item.Content)
			if err != nil {
				return nil, nil, err
		REDACTED
			messages = append(messages, AnthropicMessage{
				Role:    "user",
				Content: content,
		REDACTED)

		case item.Role == "assistant":
			content, err := convertResponsesAssistantToAnthropicContent(item.Content)
			if err != nil {
				return nil, nil, err
		REDACTED
			messages = append(messages, AnthropicMessage{
				Role:    "assistant",
				Content: content,
		REDACTED)

		default:
			// Unknown role/type — attempt as user message
			if item.Content != nil {
				messages = append(messages, AnthropicMessage{
					Role:    "user",
					Content: item.Content,
			REDACTED)
		REDACTED
	REDACTED
REDACTED

	// Repair tool_use/tool_result pairing, then merge consecutive same-role
	// messages (Anthropic requires alternating roles). The first merge groups
	// parallel calls (and their results) so the pairing pass sees them together;
	// the pairing pass may re-split a user turn (e.g. when an injected message
	// sat between a call and its output), so a second merge restores alternation.
	messages = mergeConsecutiveMessages(messages)
	messages = normalizeAnthropicToolPairing(messages)
	messages = mergeConsecutiveMessages(messages)

	return system, messages, nil
REDACTED

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
	REDACTED
		for _, b := range parseContentBlocks(m.Content) {
			if b.Type == "tool_result" && b.ToolUseID != "" {
				results[b.ToolUseID] = b
		REDACTED
	REDACTED
REDACTED

	out := make([]AnthropicMessage, 0, len(messages))
	for _, m := range messages {
		blocks := parseContentBlocks(m.Content)
		switch m.Role {
		case "assistant":
			var toolUses, others []AnthropicContentBlock
			for _, b := range blocks {
				if b.Type == "tool_use" {
					toolUses = append(toolUses, b)
			REDACTED else {
					others = append(others, b)
			REDACTED
		REDACTED
			if len(toolUses) == 0 {
				out = append(out, m)
				continue
		REDACTED
			kept := make([]AnthropicContentBlock, 0, len(toolUses))
			for _, tu := range toolUses {
				if _, ok := results[tu.ID]; ok {
					kept = append(kept, tu)
			REDACTED
		REDACTED
			if len(kept) == 0 {
				// No answered calls: keep any non-tool content, else drop.
				if len(others) > 0 {
					out = append(out, anthropicMessageFromBlocks("assistant", others))
			REDACTED
				continue
		REDACTED
			asstBlocks := make([]AnthropicContentBlock, 0, len(others)+len(kept))
			asstBlocks = append(asstBlocks, others...)
			asstBlocks = append(asstBlocks, kept...)
			out = append(out, anthropicMessageFromBlocks("assistant", asstBlocks))

			resBlocks := make([]AnthropicContentBlock, 0, len(kept))
			for _, tu := range kept {
				resBlocks = append(resBlocks, results[tu.ID])
		REDACTED
			out = append(out, anthropicMessageFromBlocks("user", resBlocks))

		case "user":
			var nonResult []AnthropicContentBlock
			hasResult := false
			for _, b := range blocks {
				if b.Type == "tool_result" {
					hasResult = true
					continue
			REDACTED
				nonResult = append(nonResult, b)
		REDACTED
			if !hasResult {
				out = append(out, m)
				continue
		REDACTED
			// The tool_result blocks are re-emitted next to their call; keep any
			// other content of this user turn in place, drop it if there is none.
			if len(nonResult) > 0 {
				out = append(out, anthropicMessageFromBlocks("user", nonResult))
		REDACTED

		default:
			out = append(out, m)
	REDACTED
REDACTED
	return out
REDACTED

// anthropicMessageFromBlocks builds an AnthropicMessage whose content is the
// marshaled block array.
func anthropicMessageFromBlocks(role string, blocks []AnthropicContentBlock) AnthropicMessage {
	content, _ := json.Marshal(blocks)
	return AnthropicMessage{Role: role, Content: contentREDACTED
REDACTED

// extractTextFromContent extracts text from a content field that may be a
// plain string or an array of content parts.
func extractTextFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
REDACTED
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
REDACTED
	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var texts []string
		for _, p := range parts {
			if (p.Type == "input_text" || p.Type == "output_text" || p.Type == "text") && p.Text != "" {
				texts = append(texts, p.Text)
		REDACTED
	REDACTED
		return strings.Join(texts, "\n\n")
REDACTED
	return ""
REDACTED

// convertResponsesUserToAnthropicContent converts a Responses user message
// content field into Anthropic content blocks JSON.
func convertResponsesUserToAnthropicContent(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.Marshal("") // empty string content
REDACTED

	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal(s)
REDACTED

	// Array of content parts → Anthropic content blocks.
	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		// Pass through as-is if we can't parse
		return raw, nil
REDACTED

	var blocks []AnthropicContentBlock
	for _, p := range parts {
		switch p.Type {
		case "input_text", "text":
			if p.Text != "" {
				blocks = append(blocks, AnthropicContentBlock{
					Type: "text",
					Text: p.Text,
			REDACTED)
		REDACTED
		case "input_image":
			src := dataURIToAnthropicImageSource(p.ImageURL)
			if src != nil {
				blocks = append(blocks, AnthropicContentBlock{
					Type:   "image",
					Source: src,
			REDACTED)
		REDACTED
	REDACTED
REDACTED

	if len(blocks) == 0 {
		return json.Marshal("")
REDACTED
	return json.Marshal(blocks)
REDACTED

// convertResponsesAssistantToAnthropicContent converts a Responses assistant
// message content field into Anthropic content blocks JSON.
func convertResponsesAssistantToAnthropicContent(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.Marshal([]AnthropicContentBlock{{Type: "text", Text: ""REDACTEDREDACTED)
REDACTED

	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal([]AnthropicContentBlock{{Type: "text", Text: sREDACTEDREDACTED)
REDACTED

	// Array of content parts → Anthropic content blocks.
	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return raw, nil
REDACTED

	var blocks []AnthropicContentBlock
	for _, p := range parts {
		switch p.Type {
		case "output_text", "text":
			if p.Text != "" {
				blocks = append(blocks, AnthropicContentBlock{
					Type: "text",
					Text: p.Text,
			REDACTED)
		REDACTED
	REDACTED
REDACTED

	if len(blocks) == 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: ""REDACTED)
REDACTED
	return json.Marshal(blocks)
REDACTED

// fromResponsesCallIDToAnthropic converts an OpenAI function call ID back to
// Anthropic format. Reverses toResponsesCallID.
func fromResponsesCallIDToAnthropic(id string) string {
	// If it has our "fc_" prefix wrapping a known Anthropic prefix, strip it
	if after, ok := strings.CutPrefix(id, "fc_"); ok {
		if strings.HasPrefix(after, "toolu_") || strings.HasPrefix(after, "call_") {
			return after
	REDACTED
REDACTED
	// Generate a synthetic Anthropic tool ID
	if !strings.HasPrefix(id, "toolu_") && !strings.HasPrefix(id, "call_") {
		return "toolu_" + id
REDACTED
	return id
REDACTED

// dataURIToAnthropicImageSource parses a data URI into an AnthropicImageSource.
func dataURIToAnthropicImageSource(dataURI string) *AnthropicImageSource {
	if !strings.HasPrefix(dataURI, "data:") {
		return nil
REDACTED
	// Format: data:<media_type>;base64,<data>
	rest := strings.TrimPrefix(dataURI, "data:")
	semicolonIdx := strings.Index(rest, ";")
	if semicolonIdx < 0 {
		return nil
REDACTED
	mediaType := rest[:semicolonIdx]
	rest = rest[semicolonIdx+1:]
	if !strings.HasPrefix(rest, "base64,") {
		return nil
REDACTED
	data := strings.TrimPrefix(rest, "base64,")
	return &AnthropicImageSource{
		Type:      "base64",
		MediaType: mediaType,
		Data:      data,
REDACTED
REDACTED

// mergeConsecutiveMessages merges consecutive messages with the same role
// because Anthropic requires alternating user/assistant turns.
func mergeConsecutiveMessages(messages []AnthropicMessage) []AnthropicMessage {
	if len(messages) <= 1 {
		return messages
REDACTED

	var merged []AnthropicMessage
	for _, msg := range messages {
		if len(merged) == 0 || merged[len(merged)-1].Role != msg.Role {
			merged = append(merged, msg)
			continue
	REDACTED

		// Same role — merge content arrays
		last := &merged[len(merged)-1]
		lastBlocks := parseContentBlocks(last.Content)
		newBlocks := parseContentBlocks(msg.Content)
		combined := append(lastBlocks, newBlocks...)
		last.Content, _ = json.Marshal(combined)
REDACTED
	return merged
REDACTED

// parseContentBlocks attempts to parse content as []AnthropicContentBlock.
// If it's a string, wraps it in a text block.
func parseContentBlocks(raw json.RawMessage) []AnthropicContentBlock {
	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return blocks
REDACTED
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []AnthropicContentBlock{{Type: "text", Text: sREDACTEDREDACTED
REDACTED
	return nil
REDACTED

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
		REDACTED)
		case "function":
			out = append(out, AnthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
		REDACTED)
		case "custom":
			out = append(out, AnthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
		REDACTED)
		default:
			// Pass through unknown tool types
			out = append(out, AnthropicTool{
				Type:        t.Type,
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
		REDACTED)
	REDACTED
REDACTED
	return out
REDACTED

// normalizeAnthropicInputSchema ensures input_schema is a valid object schema.
func normalizeAnthropicInputSchema(schema json.RawMessage) json.RawMessage {
	const emptyObjectSchema = `{"type":"object","properties":{REDACTEDREDACTED`

	trimmed := strings.TrimSpace(string(schema))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage(emptyObjectSchema)
REDACTED

	var m map[string]json.RawMessage
	if err := json.Unmarshal(schema, &m); err != nil {
		return json.RawMessage(`{"type":"object","properties":{REDACTEDREDACTED`)
REDACTED

	typeRaw, ok := m["type"]
	if !ok || strings.TrimSpace(string(typeRaw)) == "" || string(typeRaw) == "null" {
		m["type"] = json.RawMessage(`"object"`)
REDACTED else {
		var typ string
		if err := json.Unmarshal(typeRaw, &typ); err != nil || typ != "object" {
			return json.RawMessage(emptyObjectSchema)
	REDACTED
REDACTED

	if _, ok := m["properties"]; !ok {
		m["properties"] = json.RawMessage(`{REDACTED`)
REDACTED

	out, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(emptyObjectSchema)
REDACTED
	return out
REDACTED

// convertResponsesToAnthropicToolChoice maps Responses tool_choice to Anthropic format.
// Reverse of convertAnthropicToolChoiceToResponses.
//
//	"auto"                                     → {"type":"auto"REDACTED
//	"required"                                 → {"type":"any"REDACTED
//	"none"                                     → {"type":"none"REDACTED
//	{"type":"function","name":"X"REDACTED                 → {"type":"tool","name":"X"REDACTED
//	{"type":"function","function":{"name":"X"REDACTEDREDACTED     → {"type":"tool","name":"X"REDACTED // legacy
func convertResponsesToAnthropicToolChoice(raw json.RawMessage) (json.RawMessage, error) {
	// Try as string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto":
			return json.Marshal(map[string]string{"type": "auto"REDACTED)
		case "required":
			return json.Marshal(map[string]string{"type": "any"REDACTED)
		case "none":
			return json.Marshal(map[string]string{"type": "none"REDACTED)
		default:
			return raw, nil
	REDACTED
REDACTED

	// Try as object with type=function
	var tc struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Function struct {
			Name string `json:"name"`
	REDACTED `json:"function"`
REDACTED
	if err := json.Unmarshal(raw, &tc); err == nil && tc.Type == "function" {
		name := strings.TrimSpace(tc.Name)
		if name == "" {
			name = strings.TrimSpace(tc.Function.Name)
	REDACTED
		if name == "" {
			return raw, nil
	REDACTED
		return json.Marshal(map[string]string{
			"type": "tool",
			"name": name,
	REDACTED)
REDACTED

	// Pass through unknown
	return raw, nil
REDACTED
