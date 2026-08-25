package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// Keep this aligned with the gateway's default maximum request-body size. A
	// caller with a larger custom limit must not make this compatibility pass an
	// unbounded parser or patch accumulator.
	openAIResponsesToolSchemaMaxBodySize = 256 << 20
	openAIResponsesToolSchemaMaxDepth    = 128
	openAIResponsesToolSchemaMaxEdits    = 1 << 20
	openAIResponsesObjectUnionMaxSize    = 1 << 20
	openAIResponsesObjectUnionMaxDepth   = 32

	openAIResponsesToolSchemaFallbackType = `"object"`
)

var errOpenAIResponsesToolSchemaLimit = errors.New("OpenAI Responses tool schema safety limit exceeded")

// shouldRepairOpenAIResponsesNullToolSchemaType reports whether the upstream
// path requires a concrete object type at a function tool's parameter root.
// This defect is shared by the OpenAI, Anthropic, and CN-compatible paths.
func shouldRepairOpenAIResponsesNullToolSchemaType(platform string) bool {
	return platform == PlatformOpenAI || platform == PlatformAnthropic || IsCNProvider(platform)
}

// shouldSanitizeOpenAIResponsesToolSchemaPatterns is intentionally narrower:
// regex lookaround rejection is an OpenAI-specific schema constraint.
func shouldSanitizeOpenAIResponsesToolSchemaPatterns(platform string) bool {
	return platform == PlatformOpenAI
}

func sanitizeOpenAIResponsesToolSchemasForPlatform(body []byte, platform string) ([]byte, bool, error) {
	normalized := body
	changed := false
	if shouldRepairOpenAIResponsesNullToolSchemaType(platform) {
		next, repaired, err := sanitizeOpenAIResponsesToolParameterTypes(normalized)
		if err != nil {
			return body, false, fmt.Errorf("sanitize OpenAI Responses tool parameters: %w", err)
		}
		if repaired {
			normalized = next
			changed = true
		}
	}
	if shouldSanitizeOpenAIResponsesToolSchemaPatterns(platform) {
		next, sanitized, err := sanitizeOpenAIResponsesToolSchemaPatterns(normalized)
		if err != nil {
			return body, false, fmt.Errorf("sanitize OpenAI Responses tool schema patterns: %w", err)
		}
		if sanitized {
			normalized = next
			changed = true
		}
	}
	return normalized, changed, nil
}

// sanitizeOpenAIResponsesToolSchemaPatterns removes only schema constraints
// containing regex lookaround, which OpenAI rejects. It deliberately does not
// descend into instance-valued keywords such as default, examples, const, or
// enum.
func sanitizeOpenAIResponsesToolSchemaPatterns(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !openAIResponsesBodyMayContainRegexLookaround(body) {
		return body, false, nil
	}
	return sanitizeOpenAIResponsesToolSchemas(body, openAIResponsesToolSchemaOptions{removeLookaroundPatterns: true})
}

// sanitizeOpenAIResponsesToolParameterTypes replaces an explicitly null type
// on a tool's parameter root with object. It also repairs the known Codex
// missing root type when it is unambiguously implied by an object-only
// oneOf/anyOf. Other missing types and nested property schemas are left
// unchanged.
func sanitizeOpenAIResponsesToolParameterTypes(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}
	return sanitizeOpenAIResponsesToolSchemas(body, openAIResponsesToolSchemaOptions{
		replaceNullParameterTypes:       true,
		injectObjectUnionRootObjectType: true,
	})
}

func openAIResponsesBodyMayContainRegexLookaround(body []byte) bool {
	// An opening parenthesis may be literal or encoded as JSON's canonical
	// Unicode escape. Other lookaround characters may themselves be escaped, so
	// the full decoded pattern is checked only after scoped parsing.
	return bytes.Contains(body, []byte("(")) || bytes.Contains(body, []byte(`\u0028`))
}

func hasRegexLookaround(pattern string) bool {
	return strings.Contains(pattern, "(?=") || strings.Contains(pattern, "(?!") ||
		strings.Contains(pattern, "(?<=") || strings.Contains(pattern, "(?<!")
}

type openAIResponsesToolSchemaOptions struct {
	removeLookaroundPatterns        bool
	replaceNullParameterTypes       bool
	injectObjectUnionRootObjectType bool
}

type openAIResponsesToolSchemaContext uint8

const (
	openAIResponsesToolSchemaSkip openAIResponsesToolSchemaContext = iota
	openAIResponsesToolSchemaDocument
	openAIResponsesToolSchemaTools
	openAIResponsesToolSchemaTool
	openAIResponsesToolSchemaInput
	openAIResponsesToolSchemaInputItem
	openAIResponsesToolSchemaToolCarrier
	openAIResponsesToolSchemaTypeProbe
	openAIResponsesToolSchemaFunction
	openAIResponsesToolSchema
	openAIResponsesToolSchemaArray
	openAIResponsesToolSchemaMap
	openAIResponsesToolSchemaOrArray
)

type openAIResponsesToolSchemaEdit struct {
	start       int
	end         int
	replacement string
}

type openAIResponsesToolSchemaParser struct {
	body      []byte
	pos       int
	options   openAIResponsesToolSchemaOptions
	edits     []openAIResponsesToolSchemaEdit
	probeType string
}

func sanitizeOpenAIResponsesToolSchemas(
	body []byte, options openAIResponsesToolSchemaOptions,
) ([]byte, bool, error) {
	if len(body) > openAIResponsesToolSchemaMaxBodySize {
		return body, false, nil
	}
	if !utf8.Valid(body) {
		return nil, false, fmt.Errorf("sanitize OpenAI Responses tool schemas: invalid UTF-8")
	}
	p := openAIResponsesToolSchemaParser{body: body, options: options}
	if err := p.parseValue(openAIResponsesToolSchemaDocument, false, 0); err != nil {
		if errors.Is(err, errOpenAIResponsesToolSchemaLimit) {
			return body, false, nil
		}
		return nil, false, err
	}
	p.skipWhitespace()
	if p.pos != len(body) {
		return nil, false, fmt.Errorf("sanitize OpenAI Responses tool schemas: trailing data at byte %d", p.pos)
	}
	if !json.Valid(body) {
		return nil, false, fmt.Errorf("sanitize OpenAI Responses tool schemas: invalid JSON")
	}
	if len(p.edits) == 0 {
		return body, false, nil
	}
	return applyOpenAIResponsesToolSchemaEdits(body, p.edits)
}

func (p *openAIResponsesToolSchemaParser) parseValue(
	context openAIResponsesToolSchemaContext, schemaRoot bool, depth int,
) error {
	if depth > openAIResponsesToolSchemaMaxDepth {
		return errOpenAIResponsesToolSchemaLimit
	}
	p.skipWhitespace()
	if p.pos >= len(p.body) {
		return p.syntaxError("expected value")
	}
	if p.body[p.pos] == '{' {
		switch context {
		case openAIResponsesToolSchemaInput:
			context = openAIResponsesToolSchemaInputItem
		case openAIResponsesToolSchemaOrArray, openAIResponsesToolSchemaArray:
			context = openAIResponsesToolSchema
		}
	}
	if context == openAIResponsesToolSchemaInputItem && p.body[p.pos] == '{' {
		probe := openAIResponsesToolSchemaParser{body: p.body, pos: p.pos}
		if err := probe.parseValue(openAIResponsesToolSchemaTypeProbe, false, depth); err != nil {
			return err
		}
		switch probe.probeType {
		case "function", "custom", "tool_search", "namespace":
			context = openAIResponsesToolSchemaTool
		case "additional_tools":
			context = openAIResponsesToolSchemaToolCarrier
		default:
			context = openAIResponsesToolSchemaSkip
		}
	}
	switch p.body[p.pos] {
	case '{':
		return p.parseObject(context, schemaRoot, depth+1)
	case '[':
		return p.parseArray(context, depth+1)
	case '"':
		_, _, err := p.parseString()
		return err
	case 't':
		return p.parseKeyword("true")
	case 'f':
		return p.parseKeyword("false")
	case 'n':
		return p.parseKeyword("null")
	default:
		return p.parseNumber()
	}
}

func (p *openAIResponsesToolSchemaParser) parseObject(
	context openAIResponsesToolSchemaContext, schemaRoot bool, depth int,
) error {
	p.pos++
	p.skipWhitespace()
	if p.consume('}') {
		return nil
	}
	previousComma := -1
	deleteRunStart := -1
	deleteRunPreviousComma := -1
	rootTypeCount := 0
	nullRootTypeStart := -1
	nullRootTypeEnd := -1
	parameterCount := 0
	parameterValueStart := -1
	parameterValueEnd := -1
	for {
		p.skipWhitespace()
		memberStart := p.pos
		keyStart, keyEnd, err := p.parseString()
		if err != nil {
			return err
		}
		key := p.body[keyStart:keyEnd]
		p.skipWhitespace()
		if !p.consume(':') {
			return p.syntaxError("expected colon")
		}
		p.skipWhitespace()
		valueStart := p.pos

		childContext, childSchemaRoot := openAIResponsesToolSchemaChildContext(context, key)
		if err := p.parseValue(childContext, childSchemaRoot, depth); err != nil {
			return err
		}
		valueEnd := p.pos
		p.skipWhitespace()

		deleteMember := false
		if context == openAIResponsesToolSchemaTypeProbe && openAIResponsesJSONStringEquals(key, "type") {
			if value, ok := decodeOpenAIResponsesJSONStringValue(p.body[valueStart:valueEnd]); ok {
				p.probeType = strings.TrimSpace(value)
			}
		}
		if (context == openAIResponsesToolSchemaTool || context == openAIResponsesToolSchemaFunction) &&
			openAIResponsesJSONStringEquals(key, "parameters") {
			parameterCount++
			parameterValueStart = valueStart
			parameterValueEnd = valueEnd
		}
		if context == openAIResponsesToolSchema && openAIResponsesJSONStringEquals(key, "pattern") && p.options.removeLookaroundPatterns {
			if pattern, ok := decodeOpenAIResponsesJSONStringValue(p.body[valueStart:valueEnd]); ok && hasRegexLookaround(pattern) {
				deleteMember = true
			}
		}
		if context == openAIResponsesToolSchema && schemaRoot && openAIResponsesJSONStringEquals(key, "type") {
			// Duplicate JSON keys have parser-dependent effective values. Repair a
			// root type only when it is unique rather than choosing a winner and
			// potentially changing the client's effective schema.
			rootTypeCount++
			if bytes.Equal(p.body[valueStart:valueEnd], []byte("null")) {
				nullRootTypeStart = valueStart
				nullRootTypeEnd = valueEnd
			}
		}

		if p.pos >= len(p.body) {
			return p.syntaxError("unterminated object")
		}
		if deleteMember && deleteRunStart < 0 {
			deleteRunStart = memberStart
			deleteRunPreviousComma = previousComma
		}
		if !deleteMember && deleteRunStart >= 0 {
			// Remove through the delimiter after the deleted run, but retain the
			// whitespace that originally preceded the next member.
			if err := p.addEdit(deleteRunStart, previousComma+1, ""); err != nil {
				return err
			}
			deleteRunStart = -1
			deleteRunPreviousComma = -1
		}
		if p.body[p.pos] == ',' {
			comma := p.pos
			p.pos++
			previousComma = comma
			continue
		}
		if p.body[p.pos] != '}' {
			return p.syntaxError("expected comma or closing brace")
		}
		if deleteRunStart >= 0 {
			start := deleteRunStart
			if deleteRunPreviousComma >= 0 {
				start = deleteRunPreviousComma
			}
			if err := p.addEdit(start, p.pos, ""); err != nil {
				return err
			}
		}
		if p.options.replaceNullParameterTypes && rootTypeCount == 1 && nullRootTypeStart >= 0 {
			if err := p.addEdit(nullRootTypeStart, nullRootTypeEnd, openAIResponsesToolSchemaFallbackType); err != nil {
				return err
			}
		}
		if p.options.injectObjectUnionRootObjectType &&
			(context == openAIResponsesToolSchemaTool || context == openAIResponsesToolSchemaFunction) &&
			parameterCount == 1 && parameterValueStart >= 0 {
			if edit, ok := openAIMissingRootObjectUnionTypeEdit(
				p.body[parameterValueStart:parameterValueEnd], parameterValueStart,
			); ok {
				if err := p.addEdit(edit.start, edit.end, edit.replacement); err != nil {
					return err
				}
			}
		}
		p.pos++
		return nil
	}
}

var (
	openAIResponsesJSONOneOfKey     = []byte(`"oneOf"`)
	openAIResponsesJSONAnyOfKey     = []byte(`"anyOf"`)
	openAIResponsesJSONEscapeNeedle = []byte{'\\'}
)

func openAIMissingRootObjectUnionTypeEdit(raw []byte, absoluteStart int) (openAIResponsesToolSchemaEdit, bool) {
	if len(raw) > openAIResponsesObjectUnionMaxSize {
		return openAIResponsesToolSchemaEdit{}, false
	}
	if !bytes.Contains(raw, openAIResponsesJSONOneOfKey) && !bytes.Contains(raw, openAIResponsesJSONAnyOfKey) &&
		!bytes.Contains(raw, openAIResponsesJSONEscapeNeedle) {
		return openAIResponsesToolSchemaEdit{}, false
	}
	var schema map[string]json.RawMessage
	if len(raw) < 2 || raw[0] != '{' || json.Unmarshal(raw, &schema) != nil {
		return openAIResponsesToolSchemaEdit{}, false
	}
	if _, hasType := schema["type"]; hasType || !openAIResponsesSchemaHasObjectOnlyUnion(schema, 0) {
		return openAIResponsesToolSchemaEdit{}, false
	}
	return openAIResponsesToolSchemaEdit{
		start:       absoluteStart + 1,
		end:         absoluteStart + 1,
		replacement: `"type":"object",`,
	}, true
}

func openAIResponsesSchemaHasObjectOnlyUnion(schema map[string]json.RawMessage, depth int) bool {
	if depth > openAIResponsesObjectUnionMaxDepth {
		return false
	}
	found := false
	for _, keyword := range []string{"oneOf", "anyOf"} {
		raw, ok := schema[keyword]
		if !ok {
			continue
		}
		found = true
		var branches []json.RawMessage
		if json.Unmarshal(raw, &branches) != nil || len(branches) == 0 {
			return false
		}
		for _, branch := range branches {
			if !openAIResponsesSchemaIsObjectOnly(branch, depth+1) {
				return false
			}
		}
	}
	return found
}

func openAIResponsesSchemaIsObjectOnly(raw json.RawMessage, depth int) bool {
	if depth > openAIResponsesObjectUnionMaxDepth {
		return false
	}
	var schema map[string]json.RawMessage
	if json.Unmarshal(raw, &schema) != nil {
		return false
	}
	if rawType, ok := schema["type"]; ok {
		var schemaType string
		return json.Unmarshal(rawType, &schemaType) == nil && schemaType == "object"
	}
	return openAIResponsesSchemaHasObjectOnlyUnion(schema, depth+1)
}

func (p *openAIResponsesToolSchemaParser) parseArray(
	context openAIResponsesToolSchemaContext, depth int,
) error {
	p.pos++
	p.skipWhitespace()
	if p.consume(']') {
		return nil
	}
	childContext := openAIResponsesToolSchemaSkip
	switch context {
	case openAIResponsesToolSchemaTools:
		childContext = openAIResponsesToolSchemaTool
	case openAIResponsesToolSchemaInput:
		childContext = openAIResponsesToolSchemaInputItem
	case openAIResponsesToolSchemaArray, openAIResponsesToolSchemaOrArray:
		childContext = openAIResponsesToolSchema
	}
	for {
		if err := p.parseValue(childContext, false, depth); err != nil {
			return err
		}
		p.skipWhitespace()
		if p.consume(',') {
			continue
		}
		if !p.consume(']') {
			return p.syntaxError("expected comma or closing bracket")
		}
		return nil
	}
}

func openAIResponsesToolSchemaChildContext(
	context openAIResponsesToolSchemaContext, key []byte,
) (openAIResponsesToolSchemaContext, bool) {
	switch context {
	case openAIResponsesToolSchemaDocument:
		switch {
		case openAIResponsesJSONStringEquals(key, "tools"):
			return openAIResponsesToolSchemaTools, false
		case openAIResponsesJSONStringEquals(key, "input"):
			return openAIResponsesToolSchemaInput, false
		}
	case openAIResponsesToolSchemaTool:
		switch {
		case openAIResponsesJSONStringEquals(key, "parameters"):
			return openAIResponsesToolSchema, true
		case openAIResponsesJSONStringEquals(key, "function"):
			return openAIResponsesToolSchemaFunction, false
		case openAIResponsesJSONStringEquals(key, "tools"):
			return openAIResponsesToolSchemaTools, false
		}
	case openAIResponsesToolSchemaToolCarrier:
		if openAIResponsesJSONStringEquals(key, "tools") {
			return openAIResponsesToolSchemaTools, false
		}
	case openAIResponsesToolSchemaFunction:
		switch {
		case openAIResponsesJSONStringEquals(key, "parameters"):
			return openAIResponsesToolSchema, true
		case openAIResponsesJSONStringEquals(key, "tools"):
			return openAIResponsesToolSchemaTools, false
		}
	case openAIResponsesToolSchema:
		switch {
		case openAIResponsesJSONStringMatchesAny(key,
			"additionalProperties", "additionalItems", "contains", "not", "if", "then", "else",
			"propertyNames", "unevaluatedProperties", "unevaluatedItems"):
			return openAIResponsesToolSchema, false
		case openAIResponsesJSONStringEquals(key, "items"):
			return openAIResponsesToolSchemaOrArray, false
		case openAIResponsesJSONStringMatchesAny(key, "anyOf", "oneOf", "allOf", "prefixItems"):
			return openAIResponsesToolSchemaArray, false
		case openAIResponsesJSONStringMatchesAny(key,
			"properties", "patternProperties", "$defs", "definitions", "dependentSchemas", "dependencies"):
			return openAIResponsesToolSchemaMap, false
		}
	case openAIResponsesToolSchemaMap:
		return openAIResponsesToolSchema, false
	}
	return openAIResponsesToolSchemaSkip, false
}

func (p *openAIResponsesToolSchemaParser) parseString() (int, int, error) {
	if p.pos >= len(p.body) || p.body[p.pos] != '"' {
		return 0, 0, p.syntaxError("expected string")
	}
	start := p.pos
	p.pos++
	for p.pos < len(p.body) {
		switch p.body[p.pos] {
		case '"':
			p.pos++
			return start, p.pos, nil
		case '\\':
			p.pos++
			if p.pos >= len(p.body) {
				return 0, 0, p.syntaxError("unterminated string escape")
			}
			switch p.body[p.pos] {
			case 'u':
				if p.pos+4 >= len(p.body) {
					return 0, 0, p.syntaxError("short unicode escape")
				}
				for _, digit := range p.body[p.pos+1 : p.pos+5] {
					if !isOpenAIResponsesJSONHexDigit(digit) {
						return 0, 0, p.syntaxError("invalid unicode escape")
					}
				}
				p.pos += 5
				continue
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				p.pos++
			default:
				return 0, 0, p.syntaxError("invalid string escape")
			}
		default:
			if p.body[p.pos] < 0x20 {
				return 0, 0, p.syntaxError("control character in string")
			}
			p.pos++
		}
	}
	return 0, 0, p.syntaxError("unterminated string")
}

func isOpenAIResponsesJSONHexDigit(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func (p *openAIResponsesToolSchemaParser) parseKeyword(keyword string) error {
	if len(p.body)-p.pos < len(keyword) || string(p.body[p.pos:p.pos+len(keyword)]) != keyword {
		return p.syntaxError("invalid literal")
	}
	p.pos += len(keyword)
	return nil
}

func (p *openAIResponsesToolSchemaParser) parseNumber() error {
	start := p.pos
	if p.consume('-') && p.pos >= len(p.body) {
		return p.syntaxError("invalid number")
	}
	if p.consume('0') {
		if p.pos < len(p.body) && p.body[p.pos] >= '0' && p.body[p.pos] <= '9' {
			return p.syntaxError("invalid leading zero")
		}
	} else {
		if p.pos >= len(p.body) || p.body[p.pos] < '1' || p.body[p.pos] > '9' {
			return p.syntaxError("invalid number")
		}
		for p.pos < len(p.body) && p.body[p.pos] >= '0' && p.body[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.consume('.') {
		fractionStart := p.pos
		for p.pos < len(p.body) && p.body[p.pos] >= '0' && p.body[p.pos] <= '9' {
			p.pos++
		}
		if p.pos == fractionStart {
			return p.syntaxError("invalid fraction")
		}
	}
	if p.pos < len(p.body) && (p.body[p.pos] == 'e' || p.body[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.body) && (p.body[p.pos] == '+' || p.body[p.pos] == '-') {
			p.pos++
		}
		exponentStart := p.pos
		for p.pos < len(p.body) && p.body[p.pos] >= '0' && p.body[p.pos] <= '9' {
			p.pos++
		}
		if p.pos == exponentStart {
			return p.syntaxError("invalid exponent")
		}
	}
	if p.pos == start {
		return p.syntaxError("invalid value")
	}
	return nil
}

func (p *openAIResponsesToolSchemaParser) addEdit(start, end int, replacement string) error {
	if start < 0 || end < start || end > len(p.body) {
		return p.syntaxError("invalid patch span")
	}
	if len(p.edits) >= openAIResponsesToolSchemaMaxEdits {
		return errOpenAIResponsesToolSchemaLimit
	}
	p.edits = append(p.edits, openAIResponsesToolSchemaEdit{start: start, end: end, replacement: replacement})
	return nil
}

func (p *openAIResponsesToolSchemaParser) skipWhitespace() {
	for p.pos < len(p.body) {
		switch p.body[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *openAIResponsesToolSchemaParser) consume(want byte) bool {
	if p.pos < len(p.body) && p.body[p.pos] == want {
		p.pos++
		return true
	}
	return false
}

func (p *openAIResponsesToolSchemaParser) syntaxError(message string) error {
	return fmt.Errorf("sanitize OpenAI Responses tool schemas: %s at byte %d", message, p.pos)
}

func decodeOpenAIResponsesJSONString(raw []byte) (string, error) {
	decoded, err := strconv.Unquote(string(raw))
	if err != nil {
		return "", err
	}
	return decoded, nil
}

func openAIResponsesJSONStringEquals(raw []byte, want string) bool {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return false
	}
	if !bytes.Contains(raw, []byte{'\\'}) {
		return bytes.Equal(raw[1:len(raw)-1], []byte(want))
	}
	decoded, err := decodeOpenAIResponsesJSONString(raw)
	return err == nil && decoded == want
}

func openAIResponsesJSONStringMatchesAny(raw []byte, candidates ...string) bool {
	for _, candidate := range candidates {
		if openAIResponsesJSONStringEquals(raw, candidate) {
			return true
		}
	}
	return false
}

func decodeOpenAIResponsesJSONStringValue(raw []byte) (string, bool) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
	}
	decoded, err := decodeOpenAIResponsesJSONString(raw)
	return decoded, err == nil
}

func applyOpenAIResponsesToolSchemaEdits(
	body []byte, edits []openAIResponsesToolSchemaEdit,
) ([]byte, bool, error) {
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].start != edits[j].start {
			return edits[i].start < edits[j].start
		}
		return edits[i].end < edits[j].end
	})

	merged := edits[:0]
	for _, edit := range edits {
		if len(merged) == 0 || edit.start >= merged[len(merged)-1].end {
			merged = append(merged, edit)
			continue
		}
		previous := &merged[len(merged)-1]
		if previous.replacement != "" || edit.replacement != "" {
			return nil, false, fmt.Errorf("sanitize OpenAI Responses tool schemas: overlapping patches")
		}
		if edit.end > previous.end {
			previous.end = edit.end
		}
	}

	delta := 0
	for _, edit := range merged {
		delta += len(edit.replacement) - (edit.end - edit.start)
	}
	sanitized := make([]byte, 0, len(body)+delta)
	cursor := 0
	for _, edit := range merged {
		sanitized = append(sanitized, body[cursor:edit.start]...)
		sanitized = append(sanitized, edit.replacement...)
		cursor = edit.end
	}
	sanitized = append(sanitized, body[cursor:]...)
	if !json.Valid(sanitized) {
		return nil, false, fmt.Errorf("sanitize OpenAI Responses tool schemas: patch produced invalid JSON")
	}
	return sanitized, true, nil
}
