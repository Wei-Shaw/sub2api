// Adapted from Oh My Pi (MIT), commit 4049c62fbb0a7f5068954d01f11a73112c942a11,
// packages/ai/src/providers/devin.ts (request mapping).
// See the repository root THIRD_PARTY_NOTICES.md for license and attribution.
package devin

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

type chatMessage struct {
	apicompat.ChatMessage
	IsError bool `json:"is_error,omitempty"`
}

type chatRequest struct {
	apicompat.ChatCompletionsRequest
	Messages []chatMessage `json:"messages"`
	N        *int          `json:"n,omitempty"`
}
type mappedRequest struct {
	req          chatRequest
	wire         []byte
	routerPrompt []byte
}

func strictJSON(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return inputError("Invalid or unsupported Chat Completions input")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return inputError("Invalid Chat Completions JSON")
	}
	return nil
}

// Shape checks prevent fields accepted by the shared compatibility types from
// being silently ignored by this narrower wire adapter (including signatures).
func keys(b []byte, allowed ...string) error {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil || m == nil {
		return inputError("Expected a JSON object")
	}
	for key := range m {
		found := false
		for _, a := range allowed {
			if a == key {
				found = true
				break
			}
		}
		if !found {
			return inputError("Unsupported Chat Completions field")
		}
	}
	return nil
}
func uuid(b []byte) string {
	b = append([]byte(nil), b[:16]...)
	b[6] = (b[6] & 15) | 0x50
	b[8] = (b[8] & 63) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("devin: secure random unavailable")
	}
	return uuid(b[:])
}
func historyID(cascade string, index int, role, toolID string) string {
	seed := fmt.Sprintf("%s\x00%d\x00%s", cascade, index, role)
	if role == "tool" {
		seed += "\x00" + toolID
	}
	hash := sha256.Sum256([]byte(seed))
	id := uuid(hash[:])
	if role == "assistant" {
		id = "bot-" + id
	}
	return id
}
func rawSet(b json.RawMessage) bool {
	return len(b) > 0 && !bytes.Equal(bytes.TrimSpace(b), []byte("null"))
}

func content(b json.RawMessage, imagesAllowed bool) (string, [][]byte, error) {
	if !rawSet(b) {
		return "", nil, nil
	}
	var text string
	if json.Unmarshal(b, &text) == nil {
		return text, nil, nil
	}
	var parts []json.RawMessage
	if json.Unmarshal(b, &parts) != nil {
		return "", nil, inputError("Unsupported message content")
	}
	var images [][]byte
	for _, raw := range parts {
		var part apicompat.ChatContentPart
		if err := strictJSON(raw, &part); err != nil {
			return "", nil, err
		}
		switch part.Type {
		case "text":
			if err := keys(raw, "type", "text"); err != nil {
				return "", nil, err
			}
			text += part.Text
		case "image_url":
			if err := keys(raw, "type", "image_url"); err != nil {
				return "", nil, err
			}
			if !imagesAllowed || part.ImageURL == nil {
				return "", nil, inputError("This Devin model or message role does not support images")
			}
			var shape struct {
				ImageURL json.RawMessage `json:"image_url"`
			}
			_ = json.Unmarshal(raw, &shape)
			if err := keys(shape.ImageURL, "url", "detail"); err != nil {
				return "", nil, err
			}
			if part.ImageURL.Detail != "" && part.ImageURL.Detail != "auto" {
				return "", nil, inputError("Image detail settings are unsupported")
			}
			u := part.ImageURL.URL
			if !strings.HasPrefix(u, "data:image/") {
				return "", nil, inputError("Only inline base64 images are supported")
			}
			header, data, ok := strings.Cut(strings.TrimPrefix(u, "data:"), ",")
			if !ok || !strings.HasSuffix(header, ";base64") {
				return "", nil, inputError("Only inline base64 images are supported")
			}
			mime := strings.TrimSuffix(header, ";base64")
			switch mime {
			case "image/png", "image/jpeg", "image/webp", "image/gif":
			default:
				return "", nil, inputError("Unsupported image format")
			}
			decoded, err := base64.StdEncoding.DecodeString(data)
			if err != nil || len(decoded) == 0 {
				return "", nil, inputError("Invalid inline image")
			}
			images = append(images, ps(ps(nil, 1, data), 2, mime))
		default:
			return "", nil, inputError("Unsupported message content type")
		}
	}
	return text, images, nil
}
func userPrompt(text, id string, images [][]byte) []byte {
	b := ps(nil, 1, id)
	b = pv(b, 2, 1)
	b = ps(b, 3, text)
	for _, image := range images {
		b = pb(b, 10, image)
	}
	return b
}

func mapRequest(body []byte, cascade string, model Model) (*mappedRequest, error) {
	if len(body) > maxPayload {
		return nil, inputError("Request exceeds Devin frame limit")
	}
	var req chatRequest
	if err := strictJSON(body, &req); err != nil {
		return nil, err
	}
	if len(req.Messages) == 0 {
		return nil, inputError("At least one message is required")
	}
	if req.N != nil && *req.N != 1 {
		return nil, inputError("Devin supports n=1 only")
	}
	if rawSet(req.ResponseFormat) || req.ReasoningEffort != "" || req.ServiceTier != "" || rawSet(req.PromptCacheOptions) || len(req.Functions) > 0 || rawSet(req.FunctionCall) {
		return nil, inputError("Unsupported Devin generation setting; select an explicit model variant for effort")
	}
	if req.ParallelToolCalls != nil && *req.ParallelToolCalls && !model.SupportsParallelTools {
		return nil, inputError("This Devin model does not support parallel tool calls")
	}
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 2) {
		return nil, inputError("Invalid temperature")
	}
	if req.TopP != nil && (*req.TopP < 0 || *req.TopP > 1) {
		return nil, inputError("Invalid top_p")
	}
	max := model.MaxTokens
	if max <= 0 {
		max = 64000
	}
	if req.MaxTokens != nil && req.MaxCompletionTokens != nil && *req.MaxTokens != *req.MaxCompletionTokens {
		return nil, inputError("Conflicting token limits")
	}
	if req.MaxTokens != nil {
		max = *req.MaxTokens
	}
	if req.MaxCompletionTokens != nil {
		max = *req.MaxCompletionTokens
	}
	if max <= 0 {
		return nil, inputError("Token limit must be positive")
	}
	var config []byte
	config = pv(config, 1, 1)
	config = pv(config, 2, uint64(max))
	config = pv(config, 3, 200)
	temperature, topP := 0.4, 1.0
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	if req.TopP != nil {
		topP = *req.TopP
	}
	config = pd(config, 5, temperature)
	config = pd(config, 6, temperature)
	config = pv(config, 7, 50)
	config = pd(config, 8, topP)
	config = pd(config, 11, 1)
	stops := []string{"<|user|>", "<|bot|>", "<|context_request|>", "<|endoftext|>", "<|end_of_turn|>"}
	if rawSet(req.Stop) {
		var one string
		var more []string
		if json.Unmarshal(req.Stop, &one) == nil {
			more = []string{one}
		} else if json.Unmarshal(req.Stop, &more) != nil {
			return nil, inputError("Invalid stop sequences")
		}
		for _, stop := range more {
			if stop == "" {
				return nil, inputError("Empty stop sequence")
			}
		}
		stops = append(stops, more...)
	}
	for _, stop := range stops {
		config = ps(config, 9, stop)
	}
	var shape struct {
		Messages []json.RawMessage `json:"messages"`
		Tools    []json.RawMessage `json:"tools"`
	}
	_ = json.Unmarshal(body, &shape)
	var wire, routerPrompt []byte
	var system []string
	if req.Instructions != "" {
		system = append(system, req.Instructions)
	}
	index := 0
	for i, msg := range req.Messages {
		if err := keys(shape.Messages[i], "role", "content", "reasoning_content", "reasoning", "tool_calls", "tool_call_id", "is_error"); err != nil {
			return nil, err
		}
		if msg.ReasoningContent != "" && msg.Reasoning != "" && msg.ReasoningContent != msg.Reasoning {
			return nil, inputError("Conflicting reasoning history")
		}
		if msg.Role != "assistant" && (len(msg.ToolCalls) > 0 || msg.ReasoningContent != "" || msg.Reasoning != "") {
			return nil, inputError("Reasoning and tool calls require assistant role")
		}
		if msg.Role != "tool" && (msg.ToolCallID != "" || msg.IsError) {
			return nil, inputError("Tool call ID and error status require tool role")
		}
		imageOK := model.SupportsImages && (msg.Role == "user" || msg.Role == "developer" || msg.Role == "tool")
		// SWE-1.6's advertised images are silently ignored upstream.
		uid := model.UID
		if uid == "" {
			uid = model.ID
		}
		if uid == "swe-1-6" || uid == "swe-1-6-fast" {
			imageOK = false
		}
		text, images, err := content(msg.Content, imageOK)
		if err != nil {
			return nil, err
		}
		if msg.Role == "system" {
			system = append(system, text)
			continue
		}
		id := historyID(cascade, index, msg.Role, msg.ToolCallID)
		index++
		var prompt []byte
		switch msg.Role {
		case "user", "developer":
			prompt = userPrompt(text, id, images)
			routerPrompt = userPrompt(text, "", images)
		case "assistant":
			prompt = ps(nil, 1, id)
			prompt = pv(prompt, 2, 2)
			prompt = ps(prompt, 3, text)
			reasoning := msg.ReasoningContent
			if reasoning == "" {
				reasoning = msg.Reasoning
			}
			prompt = ps(prompt, 11, reasoning)
			var rawMsg struct {
				Calls []json.RawMessage `json:"tool_calls"`
			}
			_ = json.Unmarshal(shape.Messages[i], &rawMsg)
			for j, tc := range msg.ToolCalls {
				if err := keys(rawMsg.Calls[j], "id", "type", "function"); err != nil {
					return nil, err
				}
				var t struct {
					Function json.RawMessage `json:"function"`
				}
				_ = json.Unmarshal(rawMsg.Calls[j], &t)
				if err := keys(t.Function, "name", "arguments"); err != nil {
					return nil, err
				}
				if tc.Type != "function" || tc.ID == "" || tc.Function.Name == "" || !json.Valid([]byte(tc.Function.Arguments)) {
					return nil, inputError("Invalid function call history")
				}
				call := ps(ps(ps(nil, 1, tc.ID), 2, tc.Function.Name), 3, tc.Function.Arguments)
				prompt = pb(prompt, 6, call)
			}
		case "tool":
			if msg.ToolCallID == "" {
				return nil, inputError("Tool results require tool_call_id")
			}
			prompt = ps(nil, 1, id)
			prompt = pv(prompt, 2, 4)
			prompt = ps(prompt, 3, text)
			prompt = ps(prompt, 7, msg.ToolCallID)
			if msg.IsError {
				prompt = pv(prompt, 9, 1)
			}
			for _, image := range images {
				prompt = pb(prompt, 10, image)
			}
		default:
			return nil, inputError("Unsupported message role")
		}
		wire = pb(wire, 3, prompt)
	}
	wire = ps(wire, 2, strings.Join(system, "\n\n"))
	wire = pb(wire, 8, config)
	if len(req.Tools) > 0 && model.supportsTools != nil && !*model.supportsTools {
		return nil, inputError("This Devin model does not support tools")
	}
	names := map[string]bool{}
	for i, tool := range req.Tools {
		if err := keys(shape.Tools[i], "type", "function"); err != nil {
			return nil, err
		}
		var rawTool struct {
			Function json.RawMessage `json:"function"`
		}
		_ = json.Unmarshal(shape.Tools[i], &rawTool)
		if err := keys(rawTool.Function, "name", "description", "parameters", "strict"); err != nil {
			return nil, err
		}
		if tool.Type != "function" || tool.Function == nil || tool.Function.Name == "" {
			return nil, inputError("Only named function tools are supported")
		}
		f := tool.Function
		if names[f.Name] {
			return nil, inputError("Duplicate function tool")
		}
		names[f.Name] = true
		schema := f.Parameters
		if !rawSet(schema) {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(schema, &object) != nil || object == nil {
			return nil, inputError("Function parameters must be a JSON Schema object")
		}
		compact := new(bytes.Buffer)
		_ = json.Compact(compact, schema)
		t := ps(ps(ps(nil, 1, f.Name), 2, f.Description), 3, compact.String())
		if f.Strict != nil && *f.Strict {
			t = pv(t, 12, 1)
		}
		wire = pb(wire, 10, t)
	}
	choice := ps(nil, 1, "auto")
	if rawSet(req.ToolChoice) {
		var option string
		if json.Unmarshal(req.ToolChoice, &option) == nil {
			switch option {
			case "auto", "none", "required":
			default:
				return nil, inputError("Unsupported tool_choice")
			}
			if option == "required" && len(req.Tools) == 0 {
				return nil, inputError("tool_choice requires tools")
			}
			choice = ps(nil, 1, option)
		} else {
			if err := keys(req.ToolChoice, "type", "function"); err != nil {
				return nil, err
			}
			var named struct {
				Type     string `json:"type"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if err := strictJSON(req.ToolChoice, &named); err != nil {
				return nil, err
			}
			if named.Type != "function" || !names[named.Function.Name] {
				return nil, inputError("Unknown tool_choice function")
			}
			choice = ps(nil, 2, named.Function.Name)
		}
	}
	wire = pb(wire, 12, choice)
	wire = pb(wire, 13, pv(nil, 1, 1))
	wire = pv(wire, 7, 5)
	wire = pv(wire, 20, 1)
	parallel := model.SupportsParallelTools
	if req.ParallelToolCalls != nil {
		parallel = *req.ParallelToolCalls
	}
	if !parallel {
		wire = pv(wire, 11, 1)
	}
	wire = ps(wire, 16, cascade)
	wire = ps(wire, 22, randomID())
	return &mappedRequest{req: req, wire: wire, routerPrompt: routerPrompt}, nil
}
