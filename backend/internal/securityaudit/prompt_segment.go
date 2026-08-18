package securityaudit

import (
	"encoding/json"
	"strings"
)

// SegmentKind is the cross-protocol classification of one piece of auditable
// content. Normalizing every protocol to these kinds is what lets the selector
// and the custom_json adapter reason about tool calls/results uniformly, and it
// is what closes the tool-only bypass (design §8.2).
type SegmentKind string

const (
	SegmentHumanText     SegmentKind = "human_text"
	SegmentAssistantText SegmentKind = "assistant_text"
	SegmentToolCall      SegmentKind = "tool_call"
	SegmentToolResult    SegmentKind = "tool_result"
	SegmentSystemText    SegmentKind = "system_text"
	// SegmentUnknown carries an unrecognized-but-non-ignored content block that is
	// still audited (design §8.2 rule 5, plan A): the block is binary-stripped and
	// json-marshaled so a novel/executable block never silently bypasses auditing.
	SegmentUnknown SegmentKind = "unknown"
)

// knownIgnoredContentTypes are reasoning/thinking and pure-binary media block
// types that are legitimately not scanned and are NOT treated as unknown blocks.
// It is deliberately narrow: document/input_file/file are NOT here because they
// can carry text, so they fall through to SegmentUnknown and get audited.
var knownIgnoredContentTypes = map[string]struct{}{
	"reasoning":         {},
	"thinking":          {},
	"redacted_thinking": {},
	"image":             {},
	"input_image":       {},
	"image_url":         {},
	"audio":             {},
	"input_audio":       {},
	"inline_data":       {},
	"inlinedata":        {},
	"file_data":         {},
	"filedata":          {},
}

func isKnownIgnoredContentType(typeName string) bool {
	_, ok := knownIgnoredContentTypes[strings.ToLower(strings.TrimSpace(typeName))]
	return ok
}

// unknownObjectSegment binary-strips and json-marshals an unrecognized block into
// an auditable SegmentUnknown. Returns false for an empty/degenerate result.
func unknownObjectSegment(object any, role string, msgIndex, blockIndex int, source string) (AuditSegment, bool) {
	text := marshalToolContent(object)
	if text == "" || text == "{}" || text == "null" || text == "[]" {
		return AuditSegment{}, false
	}
	return AuditSegment{
		Kind: SegmentUnknown, Role: role, Text: text,
		MessageIndex: msgIndex, BlockIndex: blockIndex, SourceType: source,
	}, true
}

// AuditSegment is the protocol-neutral intermediate representation. Order is
// preserved via slice position; MessageIndex/BlockIndex/SourceType are retained
// for diagnostics and to keep same-message multi-block content together.
type AuditSegment struct {
	Kind         SegmentKind
	Role         string
	ToolName     string
	ToolCallID   string
	Text         string
	MessageIndex int
	BlockIndex   int
	SourceType   string
}

// Reasoning/thinking and binary media block types (reasoning, thinking,
// redacted_thinking, image, inlineData, fileData, executableCode, ...) are
// legitimately not scanned. Each protocol parser skips them in its default case
// rather than treating them as unknown executable blocks, which would
// misclassify normal agent traffic (design §8.2 rule 5, adopted safety note 8).

// binaryPayloadKeys are object keys whose values are opaque binary blobs (base64
// images, inline data). They are stripped before a tool payload is marshaled so
// megabytes of base64 never reach the audit model or the scan text.
var binaryPayloadKeys = map[string]struct{}{
	"data":               {},
	"inlinedata":         {},
	"inline_data":        {},
	"filedata":           {},
	"file_data":          {},
	"b64_json":           {},
	"image":              {},
	"image_url":          {},
	"bytes":              {},
	"bytesbase64encoded": {},
}

// marshalToolContent serializes a tool argument/result value to JSON after
// recursively stripping binary payloads. Using json.Marshal (never fmt.Sprint or
// hand-built strings) is what guarantees the injection defense: encoding/json
// escapes '<' and '>' so a tool output cannot close the <user_input> wrapper.
func marshalToolContent(value any) string {
	cleaned := stripBinaryPayloads(value, 0)
	raw, err := json.Marshal(cleaned)
	if err != nil {
		return ""
	}
	return string(raw)
}

const maxToolStripDepth = 64

// stripBinaryPayloads walks a decoded JSON value and drops binary blobs: values
// under known binary keys, and long base64/data-URI strings anywhere. Depth is
// bounded so a pathologically nested payload cannot exhaust the stack.
func stripBinaryPayloads(value any, depth int) any {
	if depth > maxToolStripDepth {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if _, binary := binaryPayloadKeys[strings.ToLower(strings.TrimSpace(key))]; binary {
				if looksLikeMediaPayloadValue(child) {
					result[key] = "[binary omitted]"
					continue
				}
			}
			result[key] = stripBinaryPayloads(child, depth+1)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, child := range typed {
			result = append(result, stripBinaryPayloads(child, depth+1))
		}
		return result
	case string:
		// Only opaque binary blobs are dropped here. A bare string outside a
		// known binary key is audited as-is: a plain http(s) URL (e.g. a tool's
		// exfil/target URL) is a primary cyber-abuse signal and must reach the
		// scanner, unlike a data: URI or a long base64 body.
		if looksLikeBinaryBlob(typed) {
			return "[binary omitted]"
		}
		return typed
	default:
		return value
	}
}

// looksLikeBinaryBlob reports whether a bare string value is an opaque binary
// blob that would bloat the audited payload: a data: URI for image/video/audio,
// or a long base64 body. Plain http(s) URLs are deliberately NOT treated as
// blobs — they carry auditable intent. Media URLs under an explicit binary key
// are still stripped by the key check in stripBinaryPayloads, not here.
func looksLikeBinaryBlob(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "data:image/") || strings.HasPrefix(lower, "data:video/") ||
		strings.HasPrefix(lower, "data:audio/") {
		return true
	}
	if len(trimmed) >= 256 {
		for _, r := range trimmed {
			alphaNumeric := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
			if !alphaNumeric && r != '+' && r != '/' && r != '=' {
				return false
			}
		}
		return true
	}
	return false
}

// looksLikeMediaPayloadValue reports whether an arbitrary decoded value (not just
// a string) is opaque binary, e.g. a nested {"data":"...base64..."} object.
func looksLikeMediaPayloadValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed != "" && (looksLikeMediaPayload(typed) || len(typed) >= 256)
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

func normalizedPromptSegments(values []AuditSegment) []AuditSegment {
	normalized := make([]AuditSegment, 0, len(values))
	for _, value := range values {
		value.Text = strings.TrimSpace(value.Text)
		if value.Text != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func isUserSegment(segment AuditSegment) bool {
	return segment.Kind == SegmentHumanText
}

func isAssistantOutputSegment(segment AuditSegment) bool {
	return segment.Kind == SegmentAssistantText
}

func isToolSegment(segment AuditSegment) bool {
	return segment.Kind == SegmentToolCall || segment.Kind == SegmentToolResult
}

// isFollowableSegment reports segments that belong to the current agent turn's
// non-human content: tool calls/results and audited unknown blocks. These are
// pulled in from the latest message and from later messages.
func isFollowableSegment(segment AuditSegment) bool {
	return isToolSegment(segment) || segment.Kind == SegmentUnknown
}

func promptSegmentTexts(values []AuditSegment) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Text)
	}
	return result
}

// normalizeSegmentsLatestUserFirst returns the full scan scope with the latest
// user turn hoisted to the front (the priority segment). Because parsers now
// emit tool segments too, this scope naturally includes tool content — an
// additive change that never drops anything it kept before.
func normalizeSegmentsLatestUserFirst(values []AuditSegment) []string {
	normalized := normalizedPromptSegments(values)
	if len(normalized) == 0 {
		return nil
	}
	priorityIndex := len(normalized) - 1
	for index := len(normalized) - 1; index >= 0; index-- {
		if isUserSegment(normalized[index]) {
			priorityIndex = index
			break
		}
	}
	result := make([]string, 0, len(normalized))
	result = append(result, normalized[priorityIndex].Text)
	for index, segment := range normalized {
		if index != priorityIndex {
			result = append(result, segment.Text)
		}
	}
	return result
}

// blockingSegmentsLatestUserAndPreviousOutput narrows synchronous guard input to
// the latest user turn's WHOLE message (all human text plus its tool/unknown
// blocks — so an Anthropic text→tool_result→text message keeps every part), the
// tool/unknown segments that FOLLOW it in later messages (the #5745 fix), and
// the nearest preceding assistant/model output run.
func blockingSegmentsLatestUserAndPreviousOutput(values []AuditSegment) []string {
	normalized := normalizedPromptSegments(values)
	lastHuman := -1
	for index := len(normalized) - 1; index >= 0; index-- {
		if isUserSegment(normalized[index]) {
			lastHuman = index
			break
		}
	}
	if lastHuman < 0 {
		// No user content: a request cannot be narrowed safely (it may be a
		// tool-only/unknown agent turn). Fall back to the full scope, which now
		// includes tool and unknown segments, so it is audited not bypassed.
		return normalizeSegmentsLatestUserFirst(values)
	}
	// The latest human's whole message is the contiguous run sharing its
	// MessageIndex. System/history messages carry a different index and are
	// excluded, matching the narrow-input policy.
	message := normalized[lastHuman].MessageIndex
	start, end := lastHuman, lastHuman
	for start > 0 && normalized[start-1].MessageIndex == message {
		start--
	}
	for end < len(normalized)-1 && normalized[end+1].MessageIndex == message {
		end++
	}
	humanParts := make([]string, 0)
	selected := make([]AuditSegment, 0)
	for _, segment := range normalized[start : end+1] {
		switch {
		case isUserSegment(segment):
			humanParts = append(humanParts, segment.Text)
		case isFollowableSegment(segment):
			selected = append(selected, segment)
		}
	}
	// One priority segment holds every text part of the latest input so it is all
	// scanned before the current-message tool blocks and history.
	priority := []AuditSegment{{Kind: SegmentHumanText, Role: "user", Text: strings.Join(humanParts, "\n\n")}}
	selected = append(priority, selected...)
	// Tool/unknown blocks in later messages carry the current turn's real intent.
	for _, segment := range normalized[end+1:] {
		if isFollowableSegment(segment) {
			selected = append(selected, segment)
		}
	}
	// Retain the nearest preceding assistant/model output run.
	for index := start - 1; index >= 0; index-- {
		if !isAssistantOutputSegment(normalized[index]) {
			continue
		}
		runStart := index
		for runStart > 0 && isAssistantOutputSegment(normalized[runStart-1]) {
			runStart--
		}
		selected = append(selected, normalized[runStart:index+1]...)
		break
	}
	return promptSegmentTexts(selected)
}
