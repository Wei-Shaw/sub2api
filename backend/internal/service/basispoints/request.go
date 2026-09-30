// Package basispoints adapts Responses clients to the ChatGPT Excel gateway.
package basispoints

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

const ResponsesURL = "https://bps.openai.com/basispoints/api/responses"

// ImagesGenerationsURL is the non-streaming text-to-image endpoint.
const ImagesGenerationsURL = "https://bps.openai.com/basispoints/api/images/generations"

// ImagesEditsURL is the multipart image-edit endpoint (exactly one image file).
const ImagesEditsURL = "https://bps.openai.com/basispoints/api/images/edits"

type object = map[string]any

type Bridge struct {
	nativeToolImages map[string]bool
	RequestedEffort  string
	Effort           string
	Warnings         []string
	tools            map[string]tool
	unsupportedTools map[string]bool
	structured       *structuredOutput
	replay           *ReplayCache
	scope            string
	stagedReplays    *[]replayWrite
	hasToolHistory   bool
	disallowParallel bool
	clientStream     bool
}

func decode(raw []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected one JSON value")
	}
	return nil
}

func text(value any) string {
	s, _ := value.(string)
	return s
}

// NormalizeEffort caps unsupported high tiers explicitly instead of falling back to medium.
func NormalizeEffort(effort string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "", "medium":
		return "medium", nil
	case "low", "high":
		return strings.ToLower(strings.TrimSpace(effort)), nil
	case "xhigh", "x-high", "extra-high", "extra_high", "max", "ultra":
		return "xhigh", nil
	case "none", "minimal":
		return "low", nil
	default:
		return "", fmt.Errorf("basispoints reasoning effort %q is unsupported", effort)
	}
}

func fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16])
}

func message(role, content string) object {
	return object{"type": "message", "role": role, "content": []any{object{"type": "input_text", "text": content}}}
}

// Prepare preserves the requested model and uses a whitelist for the Excel wire body.
func Prepare(raw []byte, scope string, replay *ReplayCache) ([]byte, *Bridge, error) {
	return prepare(raw, scope, replay, nil)
}

func prepare(raw []byte, scope string, replay *ReplayCache, nativeToolImages map[string]bool) ([]byte, *Bridge, error) {
	var source object
	if err := decode(raw, &source); err != nil || source == nil {
		return nil, nil, fmt.Errorf("invalid Basispoints request JSON")
	}
	model := strings.TrimSpace(text(source["model"]))
	if model == "" {
		return nil, nil, fmt.Errorf("basispoints requires a model")
	}
	if text(source["previous_response_id"]) != "" {
		return nil, nil, fmt.Errorf("basispoints requires expanded history instead of previous_response_id")
	}
	requested := text(source["reasoning_effort"])
	if reasoning, ok := source["reasoning"].(object); ok {
		requested = text(reasoning["effort"])
		if mode := text(reasoning["mode"]); mode != "" && mode != "standard" {
			return nil, nil, fmt.Errorf("basispoints does not support reasoning mode %q", mode)
		}
	}
	effort, err := NormalizeEffort(requested)
	if err != nil {
		return nil, nil, err
	}
	structured, err := prepareStructuredOutput(source["text"])
	if err != nil {
		return nil, nil, err
	}
	b := &Bridge{nativeToolImages: nativeToolImages, RequestedEffort: requested, Effort: effort, tools: make(map[string]tool), unsupportedTools: make(map[string]bool), structured: structured, replay: replay, scope: scope}
	b.clientStream, _ = source["stream"].(bool)
	choice := source["tool_choice"]
	if choice != nil && text(choice) != "auto" && text(choice) != "none" {
		return nil, nil, fmt.Errorf("basispoints supports tool_choice auto or none only")
	}
	if parallel, exists := source["parallel_tool_calls"]; exists {
		enabled, ok := parallel.(bool)
		if !ok {
			return nil, nil, fmt.Errorf("basispoints parallel_tool_calls must be a boolean")
		}
		b.disallowParallel = !enabled
	}
	if items, ok := source["input"].([]any); ok {
		for _, v := range items {
			item, _ := v.(object)
			if isTool(item) || strings.HasSuffix(text(item["type"]), "_call_output") {
				b.hasToolHistory = true
			}
		}
	}
	var catalog []any
	if text(choice) != "none" {
		catalog, err = b.collectTools(source["tools"], "")
		if err != nil {
			return nil, nil, err
		}
		if input, ok := source["input"].([]any); ok {
			for _, raw := range input {
				item, _ := raw.(object)
				if text(item["type"]) == "additional_tools" {
					additional, err := b.collectTools(item["tools"], "")
					if err != nil {
						return nil, nil, err
					}
					catalog = append(catalog, additional...)
				}
			}
		}
	}
	var input []any
	switch v := source["input"].(type) {
	case string:
		input = []any{message("user", v)}
	case []any:
		input = v
	default:
		return nil, nil, fmt.Errorf("basispoints input must be text or a Responses item array")
	}
	translated, err := b.translateHistory(input)
	if err != nil {
		return nil, nil, err
	}
	normalizeMessageFileImages(translated)
	prologue := make([]any, 0, 2)
	if instructions := text(source["instructions"]); instructions != "" {
		prologue = append(prologue, message("developer", instructions))
	}
	protocol := "This request comes from an external Responses client. Return assistant text. Do not call Excel, Office, workbook or connector tools."
	if len(catalog) > 0 {
		protocol = "This request comes from an external Responses client. Use only the client tools in the catalog below. " +
			"There is no live Excel workbook for this request. The proxy intercepts run_officejs as a transport and never executes Office code. " +
			"To call one client tool, call native run_officejs using the transport matching its catalog type. " +
			"FUNCTION: code must contain one serialized JSON object {\"name\":\"CATALOG_NAME\",\"arguments\":{...}}. Arguments is an object, not an extra JSON string. " +
			"FUNCTION_CODE: when a catalog function explicitly specifies this transport, set summary to exactly codex2api.function_code/CATALOG_NAME, put its exact code argument directly in native code, and serialize all other arguments as one JSON object in extended_summary ({} if none). Never put code in extended_summary. This replaces the FUNCTION envelope for that tool, so do not JSON-wrap, fence or re-escape the code text. " +
			"FUNCTION_CMD: when a catalog function explicitly specifies this transport, set summary to exactly codex2api.function_cmd/CATALOG_NAME, put the exact cmd argument directly in native code, and serialize all other arguments as one JSON object in extended_summary ({} if none). Never include cmd in extended_summary or wrap command text in another JSON envelope. " +
			"CUSTOM: set summary to exactly codex2api.custom/CATALOG_NAME and put the exact raw tool input directly in code. Do not wrap custom input in another JSON object or add Markdown fences. " +
			"Use a CUSTOM marker only for a tool declared as custom in the current catalog. " +
			"CATALOG_NAME includes its exact namespace. Outer arguments also include extended_summary, destructive=false and references=[]. For ordinary FUNCTION transport, use a descriptive summary; FUNCTION_CODE and FUNCTION_CMD use their exact markers and metadata JSON instead. " +
			"Never nest run_officejs inside code. Serialize outer native arguments with proper JSON escaping. For FUNCTION envelopes also escape all quotes, backslashes, newline, carriage return and tab characters within JSON string values. " +
			"Call one client tool at a time, including update_plan through this transport. After receiving its result continue the task; do not repeat completed calls. " +
			"Tool results replayed under run_officejs are the named client tool's results. When a tool is needed, emit its call in this response instead of only announcing it. " +
			"Do not call other native tools or claim that shell, filesystem or workspace access is unavailable when a suitable catalog tool exists. " +
			"If no tool is needed, answer as assistant text. Client tool catalog:\n" + describeCatalog(catalog) +
			"\nEnd of catalog. Invoke native run_officejs once. Follow each tool's specified transport: FUNCTION uses a JSON envelope; FUNCTION_CODE uses raw code plus metadata JSON in extended_summary; FUNCTION_CMD uses raw cmd plus metadata JSON; CUSTOM uses its exact marker and raw input. No Office code is executed by the proxy."
		protocol += b.toolExamples()
	}
	if len(b.unsupportedTools) > 0 {
		kinds := make([]string, 0, len(b.unsupportedTools))
		for kind := range b.unsupportedTools {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		warning := "Hosted tools unavailable through Basispoints: " + strings.Join(kinds, ", ")
		b.Warnings = append(b.Warnings, warning)
		protocol += "\n" + warning + ". These declarations were omitted. Do not claim to have used them. If the task requires one, explain the limitation or use a suitable declared client tool."
	}
	if structured != nil {
		protocol += "\n" + structured.instructions()
	}
	prologue = append(prologue, message("developer", protocol))
	cacheKey := text(source["prompt_cache_key"])
	conversation := cacheKey
	if conversation == "" && len(input) > 0 {
		conversation = fingerprint(input[0])
	}
	turnEnd := 0
	if len(input) > 0 {
		turnEnd = 1
	}
	iteration := 1
	for i := len(input) - 1; i >= 0; i-- {
		item, _ := input[i].(object)
		if text(item["role"]) == "user" {
			turnEnd = i + 1
			break
		}
		if strings.HasSuffix(text(item["type"]), "_call_output") {
			iteration++
		}
	}
	output := object{
		"model": model, "model_selection": "explicit", "stream": true, "store": false,
		"input": append(prologue, translated...), "reasoning_effort": effort,
		"context_management": []any{object{"type": "compaction", "compact_threshold": 920000}},
		"metadata": object{
			"task_id": fingerprint([]any{scope, conversation}),
			"turn_id": fingerprint([]any{scope, input[:turnEnd]}), "agent_iteration": fmt.Sprint(iteration),
		},
	}
	if cacheKey != "" {
		output["prompt_cache_key"] = "bps-" + fingerprint([]any{scope, cacheKey})
	}
	if management, ok := source["context_management"].([]any); ok {
		output["context_management"] = management
	}
	body, err := json.Marshal(output)
	return body, b, err
}
