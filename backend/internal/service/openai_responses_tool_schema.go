package service

import (
	"bytes"
	"encoding/binary"
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
// This defect is shared by the OpenAI, Anthropic, Grok, and CN-compatible paths.
func shouldRepairOpenAIResponsesNullToolSchemaType(platform string) bool {
	return platform == PlatformOpenAI || platform == PlatformAnthropic || platform == PlatformGrok || platform == PlatformQwen || IsCNProvider(platform)
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

// sanitizeOpenAIResponsesToolSchemaPatterns removes schema constraints whose
// regular expressions OpenAI's Python re validator rejects, including
// lookaround, non-Python named groups, and unsupported backslash escapes. It
// also drops patternProperties keys with such expressions. When dropping a key
// would otherwise close a schema object (additionalProperties: false), the
// object is reopened so arguments that matched the removed pattern stay
// representable. It deliberately does not descend into instance-valued
// keywords such as default, examples, const, or enum.
func sanitizeOpenAIResponsesToolSchemaPatterns(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !openAIResponsesBodyMayContainUnsupportedRegex(body) {
		return body, false, nil
	}
	return sanitizeOpenAIResponsesToolSchemas(body, openAIResponsesToolSchemaOptions{removeUnsupportedRegexPatterns: true})
}

// sanitizeOpenAIResponsesToolParameterTypes replaces an explicitly null type
// on a tool's parameter root with object. It also repairs the known Codex
// missing root type when it is unambiguously implied by an object-only
// oneOf/anyOf. Other missing types and nested property schemas are left
// unchanged.
func sanitizeOpenAIResponsesToolParameterTypes(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !openAIResponsesBodyMayNeedToolParameterRepair(body) {
		return body, false, nil
	}
	return sanitizeOpenAIResponsesToolSchemas(body, openAIResponsesToolSchemaOptions{
		replaceNullParameterTypes:       true,
		injectObjectUnionRootObjectType: true,
		dropNullRequired:                true,
	})
}

var (
	openAIResponsesNullLiteral = []byte("null")
	// Short needles on purpose: bytes.Contains switches to a much slower
	// algorithm for longer needles (a quoted 7-byte key costs ~12x a 5-byte one)
	// and these run on every request.
	openAIResponsesOneOfHint         = []byte("oneOf")
	openAIResponsesAnyOfHint         = []byte("anyOf")
	openAIResponsesRegexGroupBytes   = []byte("(?")
	openAIResponsesJSONUnicodeEscape = []byte(`\u`)
	// Any escaped spelling of an ASCII letter is \u00XX, so this also stands in
	// for a union keyword written that way.
	openAIResponsesUnicodeLetterEscape = []byte(`\u00`)
	openAIResponsesJSONBackslashPair   = []byte(`\\`)
)

// openAIResponsesBodyMayNeedToolParameterRepair is a necessary condition for
// sanitizeOpenAIResponsesToolParameterTypes to change anything.
//
// Every repair reads either a literal null value (type / required), which the
// JSON grammar cannot spell with escapes, or a oneOf/anyOf union. The union
// repair first accepts a parameters object that contains "oneOf", "anyOf" or
// any backslash at all, then decides on the decoded keys - so a union keyword
// written as "one\u004ff" also qualifies. Any escaped spelling of an ASCII
// letter is a \u00XX escape, which is what the last needle covers.
func openAIResponsesBodyMayNeedToolParameterRepair(body []byte) bool {
	return bytes.Contains(body, openAIResponsesNullLiteral) ||
		bytes.Contains(body, openAIResponsesOneOfHint) ||
		bytes.Contains(body, openAIResponsesAnyOfHint) ||
		bytes.Contains(body, openAIResponsesUnicodeLetterEscape)
}

// openAIResponsesBodyMayContainUnsupportedRegex is a necessary condition for
// sanitizeOpenAIResponsesToolSchemaPatterns to change anything. It is
// deliberately much narrower than "the body contains a parenthesis": prose,
// descriptions and code samples contain those constantly, so that test passed
// on nearly every request and made this pass walk every request body.
func openAIResponsesBodyMayContainUnsupportedRegex(body []byte) bool {
	// hasUnsupportedOpenAIRegexPattern 只有三种触发方式，这里逐一给出必要字节条件：
	//
	//   1. 一个反斜杠后跟了非白名单的字母；
	//   2. 字符串以反斜杠结尾；
	//   3. 分组语法 "(?"。
	//
	// 前两种都需要解码后的反斜杠，而 JSON 里它只能写成 "\\" 或 \u005c。
	// 第三种里 "(" 和 "?" 各自都可能是转义写法。
	if bytes.Contains(body, openAIResponsesJSONBackslashPair) {
		return true
	}
	if bytes.Contains(body, openAIResponsesRegexGroupBytes) {
		return true
	}
	if !bytes.Contains(body, openAIResponsesJSONUnicodeEscape) {
		return false
	}
	// 任何 \u00XX 转义都可能写成上述三种形态里的括号、问号或反斜杠。
	return bytes.Contains(body, openAIResponsesUnicodeLetterEscape)
}

// openAIResponsesJSONPatternIsUnsupported reports whether a JSON string span
// (quotes included) decodes to a pattern Python re rejects. Spans without an
// escape - the overwhelming majority - are checked in place, so the common
// "many tools, no bad patterns" body neither decodes nor allocates per pattern.
func openAIResponsesJSONPatternIsUnsupported(raw []byte) bool {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return false
	}
	inner := raw[1 : len(raw)-1]
	if bytes.IndexByte(inner, '\\') < 0 {
		return hasUnsupportedOpenAIRegexPattern(inner)
	}
	decoded, err := decodeOpenAIResponsesJSONString(raw)
	return err == nil && hasUnsupportedOpenAIRegexPattern([]byte(decoded))
}

// hasUnsupportedOpenAIRegexPattern reports whether Python re rejects pattern.
// OpenAI validates tool-schema patterns with Python re, which supports neither
// lookaround nor the .NET/Java (?<name>...) and (?'name'...) group forms, and
// reserves unknown ASCII-letter escapes such as \p and \R.
func hasUnsupportedOpenAIRegexPattern(pattern []byte) bool {
	inCharacterClass := false
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			if i+1 >= len(pattern) {
				return true
			}
			escaped := pattern[i+1]
			if isASCIILetter(escaped) && !pythonRegexEscapeAllowed(escaped) {
				return true
			}
			i++
		case '[':
			inCharacterClass = true
		case ']':
			inCharacterClass = false
		case '(':
			if inCharacterClass || i+1 >= len(pattern) || pattern[i+1] != '?' {
				continue
			}
			if i+2 >= len(pattern) {
				return true
			}
			switch pattern[i+2] {
			case '=', '!', '<', '\'':
				return true
			}
		}
	}
	return false
}

func isASCIILetter(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}

func pythonRegexEscapeAllowed(ch byte) bool {
	switch ch {
	case 'a', 'b', 'f', 'n', 'r', 't', 'v',
		'd', 'D', 's', 'S', 'w', 'W',
		'A', 'Z', 'B', 'N', 'u', 'U', 'x':
		return true
	}
	return false
}

type openAIResponsesToolSchemaOptions struct {
	removeUnsupportedRegexPatterns  bool
	replaceNullParameterTypes       bool
	injectObjectUnionRootObjectType bool
	dropNullRequired                bool
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
	openAIResponsesToolSchemaPatternPropertiesMap
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
	// patternPropertiesDrops records, per patternProperties object start byte,
	// how many unsupported pattern keys were removed. Parents consult it to
	// decide whether a closed object must be reopened.
	patternPropertiesDrops map[int]int
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
	if len(p.edits) == 0 {
		// The walk above already rejected everything encoding/json rejects, so a
		// clean parse needs no second full-body validation. When there are edits
		// the result is validated after patching.
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
			// Nothing inside this item can be edited, so the probe walk is the
			// whole job: reuse where it stopped instead of walking the item a
			// second time just to skip it. Messages dominate input history.
			p.pos = probe.pos
			return nil
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
	objectStart := p.pos
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
	droppedPatternPropertyKeys := 0
	lostPatternProperty := false
	var additionalPropertiesEdits []openAIResponsesToolSchemaEdit
	for {
		p.skipWhitespace()
		memberStart := p.pos
		keyStart, keyEnd, err := p.parseString()
		if err != nil {
			return err
		}
		key := newOpenAIResponsesJSONKey(p.body[keyStart:keyEnd])
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
		if context == openAIResponsesToolSchemaTypeProbe && key.is("type") {
			if value, ok := decodeOpenAIResponsesJSONStringValue(p.body[valueStart:valueEnd]); ok {
				p.probeType = strings.TrimSpace(value)
			}
		}
		if (context == openAIResponsesToolSchemaTool || context == openAIResponsesToolSchemaFunction) &&
			key.is("parameters") {
			parameterCount++
			parameterValueStart = valueStart
			parameterValueEnd = valueEnd
		}
		if p.options.removeUnsupportedRegexPatterns {
			if context == openAIResponsesToolSchema && key.is("patternProperties") {
				// The child parser records how many keys it had to remove. A
				// closed sibling object must be reopened in that case.
				if p.patternPropertiesDrops[valueStart] > 0 {
					lostPatternProperty = true
				}
			}
			if context == openAIResponsesToolSchema && key.is("additionalProperties") &&
				!bytes.Equal(p.body[valueStart:valueEnd], []byte("true")) {
				// Defer the edit: patternProperties may appear after this
				// member, and only actual key removal requires reopening.
				additionalPropertiesEdits = append(additionalPropertiesEdits, openAIResponsesToolSchemaEdit{
					start:       valueStart,
					end:         valueEnd,
					replacement: "true",
				})
			}
			if context == openAIResponsesToolSchema && key.is("pattern") {
				if openAIResponsesJSONPatternIsUnsupported(p.body[valueStart:valueEnd]) {
					deleteMember = true
				}
			}
			if context == openAIResponsesToolSchemaPatternPropertiesMap &&
				openAIResponsesJSONPatternIsUnsupported(key.raw) {
				deleteMember = true
			}
		}
		// JSON Schema requires "required" to be an array. Clients occasionally emit
		// null, which xAI and Moonshot reject outright with a 400. An absent
		// "required" is the semantic equivalent of an empty one, so dropping the
		// member preserves the client's intent while satisfying strict validators.
		if context == openAIResponsesToolSchema && p.options.dropNullRequired &&
			key.is("required") &&
			bytes.Equal(p.body[valueStart:valueEnd], []byte("null")) {
			deleteMember = true
		}
		if context == openAIResponsesToolSchema && schemaRoot && key.is("type") {
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
		if deleteMember && context == openAIResponsesToolSchemaPatternPropertiesMap {
			droppedPatternPropertyKeys++
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
		if droppedPatternPropertyKeys > 0 {
			if p.patternPropertiesDrops == nil {
				p.patternPropertiesDrops = make(map[int]int)
			}
			p.patternPropertiesDrops[objectStart] = droppedPatternPropertyKeys
		}
		if lostPatternProperty {
			// Removing a patternProperties entry turns keys that only matched
			// that pattern into additional properties. Keeping
			// additionalProperties: false would reject argument keys the client
			// declared as valid, so reopen the object. This only widens what the
			// upstream accepts; it does not invent constraints.
			for _, edit := range additionalPropertiesEdits {
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

// openAIResponsesJSONKey is one object member name, prepared for repeated
// comparison. parseObject tests the same key against a dozen candidate names,
// so the escape question is answered once here instead of rescanning the span
// on every comparison.
type openAIResponsesJSONKey struct {
	raw   []byte
	inner []byte
	plain bool
}

func newOpenAIResponsesJSONKey(raw []byte) openAIResponsesJSONKey {
	key := openAIResponsesJSONKey{raw: raw}
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return key
	}
	key.inner = raw[1 : len(raw)-1]
	key.plain = bytes.IndexByte(key.inner, '\\') < 0
	return key
}

// is reports whether the member name equals want. A key carrying an escape
// decodes to fewer bytes than it occupies, so an escape-free span can be
// compared in place; only escaped spellings need to be decoded.
func (k openAIResponsesJSONKey) is(want string) bool {
	if k.inner == nil {
		return false
	}
	if k.plain {
		return len(k.inner) == len(want) && string(k.inner) == want
	}
	decoded, err := decodeOpenAIResponsesJSONString(k.raw)
	return err == nil && decoded == want
}

func (k openAIResponsesJSONKey) isAny(candidates ...string) bool {
	for _, candidate := range candidates {
		if k.is(candidate) {
			return true
		}
	}
	return false
}

func openAIResponsesToolSchemaChildContext(
	context openAIResponsesToolSchemaContext, key openAIResponsesJSONKey,
) (openAIResponsesToolSchemaContext, bool) {
	switch context {
	case openAIResponsesToolSchemaDocument:
		switch {
		case key.is("tools"):
			return openAIResponsesToolSchemaTools, false
		case key.is("input"):
			return openAIResponsesToolSchemaInput, false
		}
	case openAIResponsesToolSchemaTool:
		switch {
		// Anthropic Messages carries the same JSON Schema under input_schema, so
		// /v1/messages tools must reach the sanitizer too.
		case key.isAny("parameters", "input_schema"):
			return openAIResponsesToolSchema, true
		case key.is("function"):
			return openAIResponsesToolSchemaFunction, false
		case key.is("tools"):
			return openAIResponsesToolSchemaTools, false
		}
	case openAIResponsesToolSchemaToolCarrier:
		if key.is("tools") {
			return openAIResponsesToolSchemaTools, false
		}
	case openAIResponsesToolSchemaFunction:
		switch {
		case key.is("parameters"):
			return openAIResponsesToolSchema, true
		case key.is("tools"):
			return openAIResponsesToolSchemaTools, false
		}
	case openAIResponsesToolSchema:
		switch {
		case key.isAny(
			"additionalProperties", "additionalItems", "contains", "not", "if", "then", "else",
			"propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema"):
			return openAIResponsesToolSchema, false
		case key.is("items"):
			return openAIResponsesToolSchemaOrArray, false
		case key.isAny("anyOf", "oneOf", "allOf", "prefixItems"):
			return openAIResponsesToolSchemaArray, false
		case key.is("patternProperties"):
			return openAIResponsesToolSchemaPatternPropertiesMap, false
		case key.isAny(
			"properties", "$defs", "definitions", "dependentSchemas", "dependencies"):
			return openAIResponsesToolSchemaMap, false
		}
	case openAIResponsesToolSchemaMap, openAIResponsesToolSchemaPatternPropertiesMap:
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
	for {
		// Skip the plain run in word-sized steps: string payloads dominate large
		// request bodies, and a byte-at-a-time walk here was the bulk of the
		// cost of the whole sanitizer.
		p.pos += openAIResponsesJSONPlainStringRun(p.body[p.pos:])
		if p.pos >= len(p.body) {
			return 0, 0, p.syntaxError("unterminated string")
		}
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
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				p.pos++
			default:
				return 0, 0, p.syntaxError("invalid string escape")
			}
		default:
			// Everything else that ends the plain run is a control byte.
			return 0, 0, p.syntaxError("control character in string")
		}
	}
}

const (
	openAIResponsesSWARByteOnes = 0x0101010101010101
	openAIResponsesSWARHighBits = 0x8080808080808080
)

// openAIResponsesJSONWordHasZeroByte reports whether any byte of word is 0x00.
func openAIResponsesJSONWordHasZeroByte(word uint64) bool {
	return (word-openAIResponsesSWARByteOnes)&^word&openAIResponsesSWARHighBits != 0
}

// openAIResponsesJSONPlainStringRun returns the length of the leading run of
// bytes in bulk that can be copied through a JSON string verbatim: no closing
// quote, no escape backslash and no control byte. Those three are exactly the
// bytes parseString has to stop at.
func openAIResponsesJSONPlainStringRun(bulk []byte) int {
	offset := 0
	for ; offset+8 <= len(bulk); offset += 8 {
		word := binary.LittleEndian.Uint64(bulk[offset:])
		// '"' and '\\' are found by zero-byte detection on the XOR, which flags
		// a whole word at once; (value & 0xE0) == 0 means value < 0x20.
		if openAIResponsesJSONWordHasZeroByte(word^0x2222222222222222) ||
			openAIResponsesJSONWordHasZeroByte(word^0x5c5c5c5c5c5c5c5c) ||
			openAIResponsesJSONWordHasZeroByte(word&0xe0e0e0e0e0e0e0e0) {
			break
		}
	}
	for ; offset < len(bulk); offset++ {
		value := bulk[offset]
		if value == '"' || value == '\\' || value < 0x20 {
			break
		}
	}
	return offset
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
	// Order by start; at equal starts keep zero-width insertions ahead of the
	// span they precede, then put wider spans first so an enclosing edit is
	// seen before the edits nested inside it.
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].start != edits[j].start {
			return edits[i].start < edits[j].start
		}
		iEmpty, jEmpty := edits[i].start == edits[i].end, edits[j].start == edits[j].end
		if iEmpty != jEmpty {
			return iEmpty
		}
		return edits[i].end > edits[j].end
	})

	merged := edits[:0]
	for _, edit := range edits {
		if len(merged) == 0 || edit.start >= merged[len(merged)-1].end {
			merged = append(merged, edit)
			continue
		}
		previous := &merged[len(merged)-1]
		if edit.end <= previous.end {
			// The enclosing edit deletes or replaces this whole span with
			// content that does not depend on it, e.g. a dropped
			// patternProperties entry or an additionalProperties schema
			// reopened to true. Edits nested inside it are moot.
			continue
		}
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
