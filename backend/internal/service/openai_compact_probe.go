package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// AccountTestModeDefault drives the standard /responses connection test.
	AccountTestModeDefault = "default"
	// AccountTestModeCompact drives the native /responses remote-compaction probe.
	AccountTestModeCompact = "compact"

	openAICompactProbeProtocolVersion = 2
	openAICompactProbeMaxAge          = 30 * 24 * time.Hour
	openAICompactProbeMaxBodyBytes    = 2 << 20

	openAICompactProbeSupportedExtraKey          = "openai_compact_supported"
	openAICompactProbeVersionExtraKey            = "openai_compact_probe_version"
	openAICompactProbeCheckedAtExtraKey          = "openai_compact_checked_at"
	openAICompactProbeLastStatusExtraKey         = "openai_compact_last_status"
	openAICompactProbeLastErrorExtraKey          = "openai_compact_last_error"
	OpenAICompactProbeObservedAtUnixNanoExtraKey = "openai_compact_probe_observed_at_unix_nano"
)

func normalizeAccountTestMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AccountTestModeCompact:
		return AccountTestModeCompact
	default:
		return AccountTestModeDefault
	}
}

// createOpenAICompactProbePayload mirrors Codex remote compaction v2: a
// streaming /responses request whose final input item is compaction_trigger.
func createOpenAICompactProbePayload(model string, isOAuth bool) map[string]any {
	payload := map[string]any{
		"model":               strings.TrimSpace(model),
		"instructions":        "You are a helpful coding assistant.",
		"tools":               []any{},
		"parallel_tool_calls": true,
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": "Respond with OK.",
			},
			map[string]any{"type": "compaction_trigger"},
		},
		"stream": true,
	}
	if isOAuth {
		payload["store"] = false
		metadata, _ := json.Marshal(map[string]any{
			"request_kind": "compaction",
			"compaction": map[string]any{
				"trigger":        "manual",
				"reason":         "user_requested",
				"implementation": "responses_compaction_v2",
				"phase":          "standalone_turn",
				"strategy":       "memento",
			},
		})
		payload["client_metadata"] = map[string]any{
			"x-codex-turn-metadata": string(metadata),
		}
	}
	return payload
}

type openAICompactProbeEvent struct {
	Type string `json:"type"`
	Item struct {
		Type string `json:"type"`
	} `json:"item"`
}

// evaluateOpenAICompactProbeSSE mirrors Codex collect_compaction_output:
// success requires exactly one compaction output_item.done and one completed
// terminal. Other completed output items are permitted and diagnostic only.
func evaluateOpenAICompactProbeSSE(body []byte) (openAIProbeVerdict, string) {
	if len(bytes.TrimSpace(body)) == 0 {
		return openAIProbeVerdictUnknown, "empty compact probe response"
	}

	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Split(splitOpenAISSELines)
	scanner.Buffer(make([]byte, 64*1024), len(body)+1)
	dataLines := make([]string, 0, 1)
	totalDone := 0
	compactionDone := 0
	completed := 0
	terminalFailure := ""
	terminalSeen := false
	streamDone := false
	seenSSEField := false
	protocolErr := ""
	firstLine := true

	consumeEvent := func() {
		if protocolErr != "" || len(dataLines) == 0 {
			dataLines = dataLines[:0]
			return
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if payload == "" {
			return
		}
		// The official Codex collector stops at the first response.completed (or
		// stream error). Bytes after that point are not part of the attempt.
		if terminalSeen || streamDone {
			return
		}
		if payload == "[DONE]" {
			streamDone = true
			return
		}
		var event openAICompactProbeEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			protocolErr = "invalid compact probe SSE JSON"
			return
		}
		switch strings.TrimSpace(event.Type) {
		case "response.output_item.done":
			totalDone++
			if isResponsesCompactionItemType(event.Item.Type) {
				compactionDone++
			}
		case "response.completed":
			terminalSeen = true
			completed++
		case "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
			terminalSeen = true
			terminalFailure = strings.TrimSpace(event.Type)
		}
	}

	for scanner.Scan() {
		if terminalSeen || streamDone {
			continue
		}
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if firstLine {
			line = strings.TrimPrefix(line, "\ufeff")
			firstLine = false
		}
		if line == "" {
			consumeEvent()
			continue
		}
		if strings.HasPrefix(line, ":") {
			seenSSEField = true
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			// The SSE grammar permits a field without a colon; its value is
			// the empty string. This matters for legal `data`/`event` lines.
			field = line
			value = ""
		}
		seenSSEField = true
		if field == "data" {
			dataLines = append(dataLines, strings.TrimPrefix(value, " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return openAIProbeVerdictUnknown, "failed to parse compact probe SSE"
	}
	consumeEvent()
	if !seenSSEField {
		return openAIProbeVerdictUnknown, "compact probe response is not SSE"
	}
	if protocolErr != "" {
		return openAIProbeVerdictUnknown, protocolErr
	}
	if terminalFailure != "" {
		return openAIProbeVerdictUnknown, "compact probe terminated with " + terminalFailure
	}
	if completed != 1 {
		return openAIProbeVerdictUnknown, "compact probe must contain exactly one response.completed event"
	}
	if compactionDone > 1 {
		return openAIProbeVerdictUnknown, "compact probe produced multiple compaction output items"
	}
	if compactionDone == 0 {
		return openAIProbeVerdictUnsupported, "completed response did not produce a compaction output item (output items=" + strconv.Itoa(totalDone) + ")"
	}
	return openAIProbeVerdictSupported, ""
}

// splitOpenAISSELines accepts all line endings allowed by the SSE grammar:
// LF, CRLF, and a lone CR. bufio.ScanLines does not split lone CR records.
func splitOpenAISSELines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		switch b {
		case '\n':
			return i + 1, data[:i], nil
		case '\r':
			if i+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			advance := i + 1
			if i+1 < len(data) && data[i+1] == '\n' {
				advance++
			}
			return advance, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func shouldMarkOpenAICompactUnsupported(status int, body []byte) bool {
	switch status {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true
	case http.StatusBadRequest, http.StatusForbidden, http.StatusUnprocessableEntity:
		lower := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body) + " " + string(body)))
		if strings.Contains(lower, "compact") {
			for _, keyword := range []string{
				"unsupported",
				"not support",
				"does not support",
				"not available",
				"disabled",
			} {
				if strings.Contains(lower, keyword) {
					return true
				}
			}
		}
	}
	return false
}

func evaluateOpenAICompactProbeHTTP(resp *http.Response, body []byte) (openAIProbeVerdict, string) {
	if resp == nil {
		return openAIProbeVerdictUnknown, "compact probe failed"
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return evaluateOpenAICompactProbeSSE(body)
	}
	if shouldMarkOpenAICompactUnsupported(resp.StatusCode, body) {
		return openAIProbeVerdictUnsupported, strings.TrimSpace(extractUpstreamErrorMessage(body))
	}
	return openAIProbeVerdictUnknown, strings.TrimSpace(extractUpstreamErrorMessage(body))
}

func buildOpenAICompactProbeExtraUpdates(resp *http.Response, body []byte, probeErr error, verdict openAIProbeVerdict, verdictReason string, startedAt, now time.Time) map[string]any {
	updates := map[string]any{
		openAICompactProbeLastStatusExtraKey:         nil,
		OpenAICompactProbeObservedAtUnixNanoExtraKey: startedAt.UTC().UnixNano(),
	}
	if resp != nil {
		updates[openAICompactProbeLastStatusExtraKey] = resp.StatusCode
	}

	switch {
	case probeErr != nil:
		errMsg := verdictReason
		if errMsg == "" {
			errMsg = probeErr.Error()
		}
		updates[openAICompactProbeLastErrorExtraKey] = truncateString(sanitizeUpstreamErrorMessage(errMsg), 2048)
	case resp == nil:
		updates[openAICompactProbeLastErrorExtraKey] = "compact probe failed"
	default:
		errMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
		if errMsg == "" && len(body) > 0 {
			errMsg = strings.TrimSpace(string(body))
		}
		if errMsg == "" && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
			errMsg = "HTTP " + strconv.Itoa(resp.StatusCode)
		}
		errMsg = truncateString(sanitizeUpstreamErrorMessage(errMsg), 2048)
		if verdictReason != "" {
			errMsg = verdictReason
		}
		switch verdict {
		case openAIProbeVerdictSupported:
			updates[openAICompactProbeSupportedExtraKey] = true
			updates[openAICompactProbeVersionExtraKey] = openAICompactProbeProtocolVersion
			updates[openAICompactProbeCheckedAtExtraKey] = now.UTC().Format(time.RFC3339Nano)
			updates[openAICompactProbeLastErrorExtraKey] = ""
		case openAIProbeVerdictUnsupported:
			updates[openAICompactProbeSupportedExtraKey] = false
			updates[openAICompactProbeVersionExtraKey] = openAICompactProbeProtocolVersion
			updates[openAICompactProbeCheckedAtExtraKey] = now.UTC().Format(time.RFC3339Nano)
			updates[openAICompactProbeLastErrorExtraKey] = truncateString(sanitizeUpstreamErrorMessage(errMsg), 2048)
		default:
			updates[openAICompactProbeLastErrorExtraKey] = truncateString(sanitizeUpstreamErrorMessage(errMsg), 2048)
		}
	}
	return updates
}

func openAICompactProbeSnapshotFresh(extra map[string]any, now time.Time) bool {
	version, versionExists := extra[openAICompactProbeVersionExtraKey]
	if !versionExists || !numericExtraEquals(version, openAICompactProbeProtocolVersion) {
		return false
	}
	checkedAt, ok := extra[openAICompactProbeCheckedAtExtraKey].(string)
	if !ok {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(checkedAt))
	if err != nil || parsed.After(now.Add(5*time.Minute)) {
		return false
	}
	return now.Sub(parsed) <= openAICompactProbeMaxAge
}

func numericExtraEquals(value any, expected int) bool {
	switch typed := value.(type) {
	case int:
		return typed == expected
	case int64:
		return typed == int64(expected)
	case float64:
		return typed == float64(expected)
	case json.Number:
		actual, err := typed.Int64()
		return err == nil && actual == int64(expected)
	default:
		return false
	}
}

func openAICompactProbeReadError(err error) string {
	if errors.Is(err, errOpenAIProbeBodyTooLarge) {
		return "compact probe response exceeded 2 MiB limit"
	}
	if err == nil {
		return ""
	}
	return "failed to read compact probe response: " + err.Error()
}

func compactProbeSessionID(accountID int64) string {
	seed := "anonymous"
	if accountID > 0 {
		seed = strconv.FormatInt(accountID, 10)
	}
	return deriveStableUUIDv4("sub2api:codex-compact-probe:v1:" + seed)
}
