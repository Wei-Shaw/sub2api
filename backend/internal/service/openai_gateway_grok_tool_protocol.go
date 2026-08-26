package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const grokResponsesClientToolMappingContextKey = "grok_responses_client_tool_mapping"

const grokCodexAsyncCompletionGuidance = "CRITICAL ASYNC COMPLETION RULE: Inside this `exec` JavaScript, a nested `tools.exec_command(...)` can yield an object with `session_id` and an empty `output`. Do not finish the JavaScript or call `text(...)` with that yielded result. Keep the same JavaScript running: call `tools.write_stdin({session_id: r.session_id, chars: \"\", yield_time_ms: 30000, max_output_tokens: 2000})` and repeat while the returned object still has `session_id`. Only after the nested command reaches terminal completion, emit `text(r.output ?? r)`. Separately, if the top-level `exec` function_call_output itself says `Script running with cell ID ...`, your next model action MUST be the top-level `wait` tool using that exact `cell_id`; repeat `wait` until terminal completion. Do not call top-level `exec` again. Do not inspect operating-system processes. Do not emit assistant text before the terminal result. Do not generate a replacement result."

const grokCodexWaitContinuationGuidance = "CRITICAL WAIT CONTINUATION RULE: This top-level tool is the only valid continuation after the top-level `exec` function_call_output says `Script running with cell ID ...`. Pass the exact `cell_id` returned by `exec`. If the cell is still running, call `wait` again with the same `cell_id` until terminal completion. A nested `tools.exec_command` `session_id` must instead be polled inside the same `exec` JavaScript with `tools.write_stdin`; never pass that session ID here. Do not pass an operating-system process ID or call `exec` or a shell command to poll the process."

const grokCodexTerminalOutputGuidance = "CRITICAL TOOL RESULT RULE: The latest `exec` or `wait` function_call_output is present in the conversation input. Read and use that result. Do not claim the result is missing, and do not rerun `exec` merely to recover output that already returned."

func adaptResponsesClientToolsForFunctionUpstream(body []byte, upstream string) ([]byte, apicompat.ResponsesClientToolMapping, error) {
	return adaptResponsesClientToolsForFunctionUpstreamWithMapping(
		body,
		upstream,
		apicompat.ResponsesClientToolMapping{},
	)
}

func adaptResponsesClientToolsForFunctionUpstreamWithMapping(
	body []byte,
	upstream string,
	inherited apicompat.ResponsesClientToolMapping,
	inheritedLoweredTools ...[]any,
) ([]byte, apicompat.ResponsesClientToolMapping, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var requestBody map[string]any
	if err := decoder.Decode(&requestBody); err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, fmt.Errorf("decode %s Responses client tools: %w", upstream, err)
	}

	mapping, changed, err := apicompat.AdaptResponsesClientToolsWithInheritedMapping(requestBody, inherited, inheritedLoweredTools...)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, err
	}
	if !changed {
		return body, mapping, nil
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, fmt.Errorf("encode %s Responses client tools: %w", upstream, err)
	}
	return rebuilt, mapping, nil
}

func adaptGrokResponsesClientTools(body []byte) ([]byte, apicompat.ResponsesClientToolMapping, error) {
	adapted, mapping, err := adaptResponsesClientToolsForFunctionUpstream(body, "Grok")
	if err != nil {
		return nil, apicompat.ResponsesClientToolMapping{}, err
	}
	adapted, err = applyGrokCodexClientToolPolicies(adapted, mapping)
	if err != nil {
		return nil, apicompat.ResponsesClientToolMapping{}, err
	}
	return adapted, mapping, nil
}

func applyGrokCodexClientToolPolicies(body []byte, mapping apicompat.ResponsesClientToolMapping) ([]byte, error) {
	adapted, err := addGrokCodexAsyncCompletionGuidance(body, mapping)
	if err != nil {
		return nil, err
	}
	adapted, err = forceGrokCodexPendingCellWait(adapted, mapping)
	if err != nil {
		return nil, err
	}
	adapted, err = addGrokCodexTerminalOutputGuidance(adapted, mapping)
	if err != nil {
		return nil, err
	}
	return adapted, nil
}

func grokResponsesHasFunctionTool(tools []any, name string) bool {
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(tool["type"])) == "function" &&
			strings.TrimSpace(fmt.Sprint(tool["name"])) == name {
			return true
		}
	}
	return false
}

func isGrokCodexExecDescription(description string) bool {
	return strings.Contains(description, "Run JavaScript code to orchestrate/compose tool calls") &&
		strings.Contains(description, "global `tools` object") &&
		strings.Contains(description, "`text(")
}

func grokResponsesHasCodexExecFunctionTool(tools []any) bool {
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok || strings.TrimSpace(fmt.Sprint(tool["type"])) != "function" ||
			strings.TrimSpace(fmt.Sprint(tool["name"])) != "exec" {
			continue
		}
		description, _ := tool["description"].(string)
		if isGrokCodexExecDescription(description) {
			return true
		}
	}
	return false
}

func addGrokCodexAsyncCompletionGuidance(body []byte, mapping apicompat.ResponsesClientToolMapping) ([]byte, error) {
	if !mapping.CustomTools["exec"] || !json.Valid(body) {
		return body, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var requestBody map[string]any
	if err := decoder.Decode(&requestBody); err != nil {
		return nil, fmt.Errorf("decode Grok async completion guidance: %w", err)
	}
	tools, ok := requestBody["tools"].([]any)
	if !ok || !grokResponsesHasFunctionTool(tools, "wait") || !grokResponsesHasCodexExecFunctionTool(tools) {
		return body, nil
	}
	changed := false
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok || strings.TrimSpace(fmt.Sprint(tool["type"])) != "function" {
			continue
		}
		name := strings.TrimSpace(fmt.Sprint(tool["name"]))
		description, _ := tool["description"].(string)
		switch name {
		case "exec":
			if !isGrokCodexExecDescription(description) || strings.Contains(description, "CRITICAL ASYNC COMPLETION RULE") {
				continue
			}
			tool["description"] = grokCodexAsyncCompletionGuidance + "\n\n" + description
			changed = true
		case "wait":
			if strings.Contains(description, "CRITICAL WAIT CONTINUATION RULE") {
				continue
			}
			if strings.TrimSpace(description) == "" {
				tool["description"] = grokCodexWaitContinuationGuidance
			} else {
				tool["description"] = grokCodexWaitContinuationGuidance + "\n\n" + description
			}
			changed = true
		}
	}
	if !changed {
		return body, nil
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode Grok async completion guidance: %w", err)
	}
	return rebuilt, nil
}

func forceGrokCodexPendingCellWait(body []byte, mapping apicompat.ResponsesClientToolMapping) ([]byte, error) {
	if !mapping.CustomTools["exec"] || !json.Valid(body) {
		return body, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var requestBody map[string]any
	if err := decoder.Decode(&requestBody); err != nil {
		return nil, fmt.Errorf("decode Grok pending cell continuation: %w", err)
	}
	tools, ok := requestBody["tools"].([]any)
	if !ok || !grokResponsesHasFunctionTool(tools, "wait") || !grokResponsesHasCodexExecFunctionTool(tools) {
		return body, nil
	}
	if !grokCodexLatestExecutionIsPending(requestBody["input"]) {
		return body, nil
	}
	requestBody["tool_choice"] = map[string]any{"type": "function", "name": "wait"}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode Grok pending cell continuation: %w", err)
	}
	return rebuilt, nil
}

func grokCodexLatestExecutionIsPending(input any) bool {
	output, ok := grokCodexLatestExecutionOutput(input)
	return ok && grokCodexOutputReportsRunningCell(output)
}

func grokCodexLatestExecutionOutput(input any) (any, bool) {
	items, ok := input.([]any)
	if !ok {
		return nil, false
	}
	toolNames := make(map[string]string)
	var latestOutput any
	seenExecutionOutput := false
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		switch strings.TrimSpace(fmt.Sprint(item["type"])) {
		case "function_call":
			name := strings.TrimSpace(fmt.Sprint(item["name"]))
			if name != "exec" && name != "wait" {
				continue
			}
			callID := strings.TrimSpace(fmt.Sprint(item["call_id"]))
			if callID != "" {
				toolNames[callID] = name
			}
		case "function_call_output":
			callID := strings.TrimSpace(fmt.Sprint(item["call_id"]))
			if name := toolNames[callID]; name == "exec" || name == "wait" {
				latestOutput = item["output"]
				seenExecutionOutput = true
			}
		}
	}
	return latestOutput, seenExecutionOutput
}

func grokCodexOutputReportsRunningCell(value any) bool {
	var text strings.Builder
	var appendText func(any)
	appendText = func(value any) {
		switch typed := value.(type) {
		case string:
			_, _ = text.WriteString(typed)
		case []any:
			for _, child := range typed {
				appendText(child)
			}
		case map[string]any:
			for _, key := range []string{"text", "output"} {
				if child, exists := typed[key]; exists {
					appendText(child)
				}
			}
		}
	}
	appendText(value)
	lowered := strings.ToLower(text.String())
	return strings.Contains(lowered, "script running with cell id") ||
		strings.Contains(lowered, "script running with cell_id")
}

func addGrokCodexTerminalOutputGuidance(body []byte, mapping apicompat.ResponsesClientToolMapping) ([]byte, error) {
	if !mapping.CustomTools["exec"] || !json.Valid(body) {
		return body, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var requestBody map[string]any
	if err := decoder.Decode(&requestBody); err != nil {
		return nil, fmt.Errorf("decode Grok terminal tool output guidance: %w", err)
	}
	tools, ok := requestBody["tools"].([]any)
	if !ok || !grokResponsesHasCodexExecFunctionTool(tools) {
		return body, nil
	}
	output, ok := grokCodexLatestExecutionOutput(requestBody["input"])
	if !ok || grokCodexOutputReportsRunningCell(output) {
		return body, nil
	}
	instructions, _ := requestBody["instructions"].(string)
	if strings.Contains(instructions, "CRITICAL TOOL RESULT RULE") {
		return body, nil
	}
	if strings.TrimSpace(instructions) == "" {
		requestBody["instructions"] = grokCodexTerminalOutputGuidance
	} else {
		requestBody["instructions"] = grokCodexTerminalOutputGuidance + "\n\n" + instructions
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode Grok terminal tool output guidance: %w", err)
	}
	return rebuilt, nil
}

func hasResponsesClientToolMapping(mapping apicompat.ResponsesClientToolMapping) bool {
	return len(mapping.CustomTools) > 0 || mapping.ToolSearch || len(mapping.NamespaceTools) > 0
}

func hasGrokResponsesClientToolMapping(mapping apicompat.ResponsesClientToolMapping) bool {
	return hasResponsesClientToolMapping(mapping)
}

func setGrokResponsesClientToolMapping(c *gin.Context, mapping apicompat.ResponsesClientToolMapping) {
	if c == nil {
		return
	}
	if !hasGrokResponsesClientToolMapping(mapping) {
		clearGrokResponsesClientToolMapping(c)
		return
	}
	c.Set(grokResponsesClientToolMappingContextKey, mapping)
}

func clearGrokResponsesClientToolMapping(c *gin.Context) {
	if c == nil {
		return
	}
	if _, exists := c.Get(grokResponsesClientToolMappingContextKey); !exists {
		return
	}
	c.Set(grokResponsesClientToolMappingContextKey, apicompat.ResponsesClientToolMapping{})
}

func grokResponsesClientToolMapping(c *gin.Context) (apicompat.ResponsesClientToolMapping, bool) {
	if c == nil {
		return apicompat.ResponsesClientToolMapping{}, false
	}
	value, ok := c.Get(grokResponsesClientToolMappingContextKey)
	if !ok {
		return apicompat.ResponsesClientToolMapping{}, false
	}
	mapping, ok := value.(apicompat.ResponsesClientToolMapping)
	return mapping, ok && hasGrokResponsesClientToolMapping(mapping)
}

func restoreGrokResponsesClientToolPayload(c *gin.Context, payload []byte) ([]byte, error) {
	mapping, ok := grokResponsesClientToolMapping(c)
	if !ok || !bytes.Contains(payload, []byte(`"function_call"`)) || !json.Valid(payload) {
		return payload, nil
	}
	restored, _, err := apicompat.RestoreResponsesClientToolPayload(payload, mapping)
	return restored, err
}

type responsesClientToolStreamBody struct {
	*io.PipeReader
	source io.Closer
}

func (b *responsesClientToolStreamBody) Close() error {
	readerErr := b.PipeReader.Close()
	sourceErr := b.source.Close()
	if readerErr != nil {
		return readerErr
	}
	return sourceErr
}

func newResponsesClientToolStreamBody(
	source io.ReadCloser,
	mapping apicompat.ResponsesClientToolMapping,
	maxLineSize int,
) io.ReadCloser {
	reader, writer := io.Pipe()
	body := &responsesClientToolStreamBody{PipeReader: reader, source: source}
	go transformResponsesClientToolStream(source, writer, mapping, maxLineSize)
	return body
}

func newGrokResponsesClientToolStreamBody(
	source io.ReadCloser,
	mapping apicompat.ResponsesClientToolMapping,
	maxLineSize int,
) io.ReadCloser {
	return newResponsesClientToolStreamBody(source, mapping, maxLineSize)
}

func transformResponsesClientToolStream(
	source io.ReadCloser,
	destination *io.PipeWriter,
	mapping apicompat.ResponsesClientToolMapping,
	maxLineSize int,
) {
	defer func() { _ = source.Close() }()
	if maxLineSize <= 0 {
		maxLineSize = defaultMaxLineSize
	}

	scanner := bufio.NewScanner(source)
	scanBuf := getSSEScannerBuf64K()
	defer putSSEScannerBuf64K(scanBuf)
	scanner.Buffer(scanBuf[:0], maxLineSize)
	documents := newOpenAISSEJSONDocumentScanner(scanner)
	restorer := apicompat.NewResponsesClientToolStreamRestorer(mapping)
	buffered := bufio.NewWriterSize(destination, 4*1024)
	pendingFields := make([]string, 0, 2)
	frameHadEventField := false
	frameEmitted := false

	writeLine := func(line string) error {
		if _, err := buffered.WriteString(line); err != nil {
			return err
		}
		return buffered.WriteByte('\n')
	}
	writePendingFields := func(payload []byte, includeNonEvent bool) error {
		eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
		for _, field := range pendingFields {
			if _, isEvent := extractOpenAISSEEventLine(field); isEvent {
				if eventType != "" {
					if err := writeLine("event: " + eventType); err != nil {
						return err
					}
				} else if err := writeLine(field); err != nil {
					return err
				}
				continue
			}
			if includeNonEvent {
				if err := writeLine(field); err != nil {
					return err
				}
			}
		}
		return nil
	}
	writePayloads := func(payloads [][]byte) error {
		for index, payload := range payloads {
			if index == 0 {
				if err := writePendingFields(payload, true); err != nil {
					return err
				}
			} else if frameHadEventField {
				eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
				if eventType != "" {
					if err := writeLine("event: " + eventType); err != nil {
						return err
					}
				}
			}
			if err := writeLine("data: " + string(payload)); err != nil {
				return err
			}
			if err := writeLine(""); err != nil {
				return err
			}
		}
		return buffered.Flush()
	}

	for documents.Scan() {
		line := documents.Text()
		data, isData := extractOpenAISSEDataLine(line)
		if isData {
			payload := []byte(data)
			payloads := [][]byte{payload}
			if json.Valid(payload) {
				var err error
				payloads, _, err = restorer.RestoreEvent(payload)
				if err != nil {
					_ = buffered.Flush()
					_ = destination.CloseWithError(fmt.Errorf("restore Responses client tool event: %w", err))
					return
				}
			}
			if err := writePayloads(payloads); err != nil {
				_ = destination.CloseWithError(err)
				return
			}
			pendingFields = pendingFields[:0]
			frameHadEventField = false
			frameEmitted = true
			continue
		}

		if line == "" {
			if !frameEmitted {
				for _, field := range pendingFields {
					if err := writeLine(field); err != nil {
						_ = destination.CloseWithError(err)
						return
					}
				}
				if len(pendingFields) > 0 {
					if err := writeLine(""); err != nil {
						_ = destination.CloseWithError(err)
						return
					}
					if err := buffered.Flush(); err != nil {
						_ = destination.CloseWithError(err)
						return
					}
				}
			}
			pendingFields = pendingFields[:0]
			frameHadEventField = false
			frameEmitted = false
			continue
		}

		if _, isEvent := extractOpenAISSEEventLine(line); isEvent {
			frameHadEventField = true
		}
		pendingFields = append(pendingFields, line)
	}

	for _, field := range pendingFields {
		if err := writeLine(field); err != nil {
			_ = destination.CloseWithError(err)
			return
		}
	}
	if err := buffered.Flush(); err != nil {
		_ = destination.CloseWithError(err)
		return
	}
	if err := documents.Err(); err != nil {
		_ = destination.CloseWithError(err)
		return
	}
	_ = destination.Close()
}
