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

// shouldSanitizeOpenAIResponsesToolSchemas centralizes the platform boundary
// for callers. These rewrites describe OpenAI Responses constraints, not the
// behavior of every provider routed through the generic OpenAI gateway.
func shouldSanitizeOpenAIResponsesToolSchemas(platform string) bool {
	return platform == PlatformOpenAI
REDACTED

// sanitizeOpenAIResponsesToolSchemaPatterns removes only schema constraints
// containing regex lookaround, which OpenAI rejects. It deliberately does not
// descend into instance-valued keywords such as default, examples, const, or
// enum.
func sanitizeOpenAIResponsesToolSchemaPatterns(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !openAIResponsesBodyMayContainRegexLookaround(body) {
		return body, false, nil
REDACTED
	return sanitizeOpenAIResponsesToolSchemas(body, openAIResponsesToolSchemaOptions{removeLookaroundPatterns: trueREDACTED)
REDACTED

// sanitizeOpenAIResponsesToolParameterTypes replaces an explicitly null type
// on a tool's parameter root with object. It also repairs the known Codex
// missing root type when it is unambiguously implied by an object-only
// oneOf/anyOf. Other missing types and nested property schemas are left
// unchanged.
func sanitizeOpenAIResponsesToolParameterTypes(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
REDACTED
	return sanitizeOpenAIResponsesToolSchemas(body, openAIResponsesToolSchemaOptions{
		replaceNullParameterTypes:       true,
		injectObjectUnionRootObjectType: true,
REDACTED)
REDACTED

func openAIResponsesBodyMayContainRegexLookaround(body []byte) bool {
	// An opening parenthesis may be literal or encoded as JSON's canonical
	// Unicode escape. Other lookaround characters may themselves be escaped, so
	// the full decoded pattern is checked only after scoped parsing.
	return bytes.Contains(body, []byte("(")) || bytes.Contains(body, []byte(`\u0028`))
REDACTED

func hasRegexLookaround(pattern string) bool {
	return strings.Contains(pattern, "(?=") || strings.Contains(pattern, "(?!") ||
		strings.Contains(pattern, "(?<=") || strings.Contains(pattern, "(?<!")
REDACTED

type openAIResponsesToolSchemaOptions struct {
	removeLookaroundPatterns        bool
	replaceNullParameterTypes       bool
	injectObjectUnionRootObjectType bool
REDACTED

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
REDACTED

type openAIResponsesToolSchemaParser struct {
	body      []byte
	pos       int
	options   openAIResponsesToolSchemaOptions
	edits     []openAIResponsesToolSchemaEdit
	probeType string
REDACTED

func sanitizeOpenAIResponsesToolSchemas(
	body []byte, options openAIResponsesToolSchemaOptions,
) ([]byte, bool, error) {
	if len(body) > openAIResponsesToolSchemaMaxBodySize {
		return body, false, nil
REDACTED
	if !utf8.Valid(body) {
		return nil, false, fmt.Errorf("sanitize OpenAI Responses tool schemas: invalid UTF-8")
REDACTED
	p := openAIResponsesToolSchemaParser{body: body, options: optionsREDACTED
	if err := p.parseValue(openAIResponsesToolSchemaDocument, false, 0); err != nil {
		if errors.Is(err, errOpenAIResponsesToolSchemaLimit) {
			return body, false, nil
	REDACTED
		return nil, false, err
REDACTED
	p.skipWhitespace()
	if p.pos != len(body) {
		return nil, false, fmt.Errorf("sanitize OpenAI Responses tool schemas: trailing data at byte %d", p.pos)
REDACTED
	if !json.Valid(body) {
		return nil, false, fmt.Errorf("sanitize OpenAI Responses tool schemas: invalid JSON")
REDACTED
	if len(p.edits) == 0 {
		return body, false, nil
REDACTED
	return applyOpenAIResponsesToolSchemaEdits(body, p.edits)
REDACTED

func (p *openAIResponsesToolSchemaParser) parseValue(
	context openAIResponsesToolSchemaContext, schemaRoot bool, depth int,
) error {
	if depth > openAIResponsesToolSchemaMaxDepth {
		return errOpenAIResponsesToolSchemaLimit
REDACTED
	p.skipWhitespace()
	if p.pos >= len(p.body) {
		return p.syntaxError("expected value")
REDACTED
	if p.body[p.pos] == '{' {
		switch context {
		case openAIResponsesToolSchemaInput:
			context = openAIResponsesToolSchemaInputItem
		case openAIResponsesToolSchemaOrArray, openAIResponsesToolSchemaArray:
			context = openAIResponsesToolSchema
	REDACTED
REDACTED
	if context == openAIResponsesToolSchemaInputItem && p.body[p.pos] == '{' {
		probe := openAIResponsesToolSchemaParser{body: p.body, pos: p.posREDACTED
		if err := probe.parseValue(openAIResponsesToolSchemaTypeProbe, false, depth); err != nil {
			return err
	REDACTED
		switch probe.probeType {
		case "function", "custom", "tool_search", "namespace":
			context = openAIResponsesToolSchemaTool
		case "additional_tools":
			context = openAIResponsesToolSchemaToolCarrier
		default:
			context = openAIResponsesToolSchemaSkip
	REDACTED
REDACTED
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
REDACTED
REDACTED

func (p *openAIResponsesToolSchemaParser) parseObject(
	context openAIResponsesToolSchemaContext, schemaRoot bool, depth int,
) error {
	p.pos++
	p.skipWhitespace()
	if p.consume('REDACTED') {
		return nil
REDACTED
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
	REDACTED
		key := p.body[keyStart:keyEnd]
		p.skipWhitespace()
		if !p.consume(':') {
			return p.syntaxError("expected colon")
	REDACTED
		p.skipWhitespace()
		valueStart := p.pos

		childContext, childSchemaRoot := openAIResponsesToolSchemaChildContext(context, key)
		if err := p.parseValue(childContext, childSchemaRoot, depth); err != nil {
			return err
	REDACTED
		valueEnd := p.pos
		p.skipWhitespace()

		deleteMember := false
		if context == openAIResponsesToolSchemaTypeProbe && openAIResponsesJSONStringEquals(key, "type") {
			if value, ok := decodeOpenAIResponsesJSONStringValue(p.body[valueStart:valueEnd]); ok {
				p.probeType = strings.TrimSpace(value)
		REDACTED
	REDACTED
		if (context == openAIResponsesToolSchemaTool || context == openAIResponsesToolSchemaFunction) &&
			openAIResponsesJSONStringEquals(key, "parameters") {
			parameterCount++
			parameterValueStart = valueStart
			parameterValueEnd = valueEnd
	REDACTED
		if context == openAIResponsesToolSchema && openAIResponsesJSONStringEquals(key, "pattern") && p.options.removeLookaroundPatterns {
			if pattern, ok := decodeOpenAIResponsesJSONStringValue(p.body[valueStart:valueEnd]); ok && hasRegexLookaround(pattern) {
				deleteMember = true
		REDACTED
	REDACTED
		if context == openAIResponsesToolSchema && schemaRoot && openAIResponsesJSONStringEquals(key, "type") {
			// Duplicate JSON keys have parser-dependent effective values. Repair a
			// root type only when it is unique rather than choosing a winner and
			// potentially changing the client's effective schema.
			rootTypeCount++
			if bytes.Equal(p.body[valueStart:valueEnd], []byte("null")) {
				nullRootTypeStart = valueStart
				nullRootTypeEnd = valueEnd
		REDACTED
	REDACTED

		if p.pos >= len(p.body) {
			return p.syntaxError("unterminated object")
	REDACTED
		if deleteMember && deleteRunStart < 0 {
			deleteRunStart = memberStart
			deleteRunPreviousComma = previousComma
	REDACTED
		if !deleteMember && deleteRunStart >= 0 {
			// Remove through the delimiter after the deleted run, but retain the
			// whitespace that originally preceded the next member.
			if err := p.addEdit(deleteRunStart, previousComma+1, ""); err != nil {
				return err
		REDACTED
			deleteRunStart = -1
			deleteRunPreviousComma = -1
	REDACTED
		if p.body[p.pos] == ',' {
			comma := p.pos
			p.pos++
			previousComma = comma
			continue
	REDACTED
		if p.body[p.pos] != 'REDACTED' {
			return p.syntaxError("expected comma or closing brace")
	REDACTED
		if deleteRunStart >= 0 {
			start := deleteRunStart
			if deleteRunPreviousComma >= 0 {
				start = deleteRunPreviousComma
		REDACTED
			if err := p.addEdit(start, p.pos, ""); err != nil {
				return err
		REDACTED
	REDACTED
		if p.options.replaceNullParameterTypes && rootTypeCount == 1 && nullRootTypeStart >= 0 {
			if err := p.addEdit(nullRootTypeStart, nullRootTypeEnd, openAIResponsesToolSchemaFallbackType); err != nil {
				return err
		REDACTED
	REDACTED
		if p.options.injectObjectUnionRootObjectType &&
			(context == openAIResponsesToolSchemaTool || context == openAIResponsesToolSchemaFunction) &&
			parameterCount == 1 && parameterValueStart >= 0 {
			if edit, ok := openAIMissingRootObjectUnionTypeEdit(
				p.body[parameterValueStart:parameterValueEnd], parameterValueStart,
			); ok {
				if err := p.addEdit(edit.start, edit.end, edit.replacement); err != nil {
					return err
			REDACTED
		REDACTED
	REDACTED
		p.pos++
		return nil
REDACTED
REDACTED

func openAIMissingRootObjectUnionTypeEdit(raw []byte, absoluteStart int) (openAIResponsesToolSchemaEdit, bool) {
	if len(raw) > openAIResponsesObjectUnionMaxSize {
		return openAIResponsesToolSchemaEdit{REDACTED, false
REDACTED
	if !bytes.Contains(raw, []byte(`"oneOf"`)) && !bytes.Contains(raw, []byte(`"anyOf"`)) &&
		!bytes.Contains(raw, []byte{'\\'REDACTED) {
		return openAIResponsesToolSchemaEdit{REDACTED, false
REDACTED
	var schema map[string]json.RawMessage
	if len(raw) < 2 || raw[0] != '{' || json.Unmarshal(raw, &schema) != nil {
		return openAIResponsesToolSchemaEdit{REDACTED, false
REDACTED
	if _, hasType := schema["type"]; hasType || !openAIResponsesSchemaHasObjectOnlyUnion(schema, 0) {
		return openAIResponsesToolSchemaEdit{REDACTED, false
REDACTED
	return openAIResponsesToolSchemaEdit{
		start:       absoluteStart + 1,
		end:         absoluteStart + 1,
		replacement: `"type":"object",`,
REDACTED, true
REDACTED

func openAIResponsesSchemaHasObjectOnlyUnion(schema map[string]json.RawMessage, depth int) bool {
	if depth > openAIResponsesObjectUnionMaxDepth {
		return false
REDACTED
	found := false
	for _, keyword := range []string{"oneOf", "anyOf"REDACTED {
		raw, ok := schema[keyword]
		if !ok {
			continue
	REDACTED
		found = true
		var branches []json.RawMessage
		if json.Unmarshal(raw, &branches) != nil || len(branches) == 0 {
			return false
	REDACTED
		for _, branch := range branches {
			if !openAIResponsesSchemaIsObjectOnly(branch, depth+1) {
				return false
		REDACTED
	REDACTED
REDACTED
	return found
REDACTED

func openAIResponsesSchemaIsObjectOnly(raw json.RawMessage, depth int) bool {
	if depth > openAIResponsesObjectUnionMaxDepth {
		return false
REDACTED
	var schema map[string]json.RawMessage
	if json.Unmarshal(raw, &schema) != nil {
		return false
REDACTED
	if rawType, ok := schema["type"]; ok {
		var schemaType string
		return json.Unmarshal(rawType, &schemaType) == nil && schemaType == "object"
REDACTED
	return openAIResponsesSchemaHasObjectOnlyUnion(schema, depth+1)
REDACTED

func (p *openAIResponsesToolSchemaParser) parseArray(
	context openAIResponsesToolSchemaContext, depth int,
) error {
	p.pos++
	p.skipWhitespace()
	if p.consume(']') {
		return nil
REDACTED
	childContext := openAIResponsesToolSchemaSkip
	switch context {
	case openAIResponsesToolSchemaTools:
		childContext = openAIResponsesToolSchemaTool
	case openAIResponsesToolSchemaInput:
		childContext = openAIResponsesToolSchemaInputItem
	case openAIResponsesToolSchemaArray, openAIResponsesToolSchemaOrArray:
		childContext = openAIResponsesToolSchema
REDACTED
	for {
		if err := p.parseValue(childContext, false, depth); err != nil {
			return err
	REDACTED
		p.skipWhitespace()
		if p.consume(',') {
			continue
	REDACTED
		if !p.consume(']') {
			return p.syntaxError("expected comma or closing bracket")
	REDACTED
		return nil
REDACTED
REDACTED

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
	REDACTED
	case openAIResponsesToolSchemaTool:
		switch {
		case openAIResponsesJSONStringEquals(key, "parameters"):
			return openAIResponsesToolSchema, true
		case openAIResponsesJSONStringEquals(key, "function"):
			return openAIResponsesToolSchemaFunction, false
		case openAIResponsesJSONStringEquals(key, "tools"):
			return openAIResponsesToolSchemaTools, false
	REDACTED
	case openAIResponsesToolSchemaToolCarrier:
		if openAIResponsesJSONStringEquals(key, "tools") {
			return openAIResponsesToolSchemaTools, false
	REDACTED
	case openAIResponsesToolSchemaFunction:
		switch {
		case openAIResponsesJSONStringEquals(key, "parameters"):
			return openAIResponsesToolSchema, true
		case openAIResponsesJSONStringEquals(key, "tools"):
			return openAIResponsesToolSchemaTools, false
	REDACTED
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
	REDACTED
	case openAIResponsesToolSchemaMap:
		return openAIResponsesToolSchema, false
REDACTED
	return openAIResponsesToolSchemaSkip, false
REDACTED

func (p *openAIResponsesToolSchemaParser) parseString() (int, int, error) {
	if p.pos >= len(p.body) || p.body[p.pos] != '"' {
		return 0, 0, p.syntaxError("expected string")
REDACTED
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
		REDACTED
			switch p.body[p.pos] {
			case 'u':
				if p.pos+4 >= len(p.body) {
					return 0, 0, p.syntaxError("short unicode escape")
			REDACTED
				for _, digit := range p.body[p.pos+1 : p.pos+5] {
					if !isOpenAIResponsesJSONHexDigit(digit) {
						return 0, 0, p.syntaxError("invalid unicode escape")
				REDACTED
			REDACTED
				p.pos += 5
				continue
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				p.pos++
			default:
				return 0, 0, p.syntaxError("invalid string escape")
		REDACTED
		default:
			if p.body[p.pos] < 0x20 {
				return 0, 0, p.syntaxError("control character in string")
		REDACTED
			p.pos++
	REDACTED
REDACTED
	return 0, 0, p.syntaxError("unterminated string")
REDACTED

func isOpenAIResponsesJSONHexDigit(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
REDACTED

func (p *openAIResponsesToolSchemaParser) parseKeyword(keyword string) error {
	if len(p.body)-p.pos < len(keyword) || string(p.body[p.pos:p.pos+len(keyword)]) != keyword {
		return p.syntaxError("invalid literal")
REDACTED
	p.pos += len(keyword)
	return nil
REDACTED

func (p *openAIResponsesToolSchemaParser) parseNumber() error {
	start := p.pos
	if p.consume('-') && p.pos >= len(p.body) {
		return p.syntaxError("invalid number")
REDACTED
	if p.consume('0') {
		if p.pos < len(p.body) && p.body[p.pos] >= '0' && p.body[p.pos] <= '9' {
			return p.syntaxError("invalid leading zero")
	REDACTED
REDACTED else {
		if p.pos >= len(p.body) || p.body[p.pos] < '1' || p.body[p.pos] > '9' {
			return p.syntaxError("invalid number")
	REDACTED
		for p.pos < len(p.body) && p.body[p.pos] >= '0' && p.body[p.pos] <= '9' {
			p.pos++
	REDACTED
REDACTED
	if p.consume('.') {
		fractionStart := p.pos
		for p.pos < len(p.body) && p.body[p.pos] >= '0' && p.body[p.pos] <= '9' {
			p.pos++
	REDACTED
		if p.pos == fractionStart {
			return p.syntaxError("invalid fraction")
	REDACTED
REDACTED
	if p.pos < len(p.body) && (p.body[p.pos] == 'e' || p.body[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.body) && (p.body[p.pos] == '+' || p.body[p.pos] == '-') {
			p.pos++
	REDACTED
		exponentStart := p.pos
		for p.pos < len(p.body) && p.body[p.pos] >= '0' && p.body[p.pos] <= '9' {
			p.pos++
	REDACTED
		if p.pos == exponentStart {
			return p.syntaxError("invalid exponent")
	REDACTED
REDACTED
	if p.pos == start {
		return p.syntaxError("invalid value")
REDACTED
	return nil
REDACTED

func (p *openAIResponsesToolSchemaParser) addEdit(start, end int, replacement string) error {
	if start < 0 || end < start || end > len(p.body) {
		return p.syntaxError("invalid patch span")
REDACTED
	if len(p.edits) >= openAIResponsesToolSchemaMaxEdits {
		return errOpenAIResponsesToolSchemaLimit
REDACTED
	p.edits = append(p.edits, openAIResponsesToolSchemaEdit{start: start, end: end, replacement: replacementREDACTED)
	return nil
REDACTED

func (p *openAIResponsesToolSchemaParser) skipWhitespace() {
	for p.pos < len(p.body) {
		switch p.body[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
	REDACTED
REDACTED
REDACTED

func (p *openAIResponsesToolSchemaParser) consume(want byte) bool {
	if p.pos < len(p.body) && p.body[p.pos] == want {
		p.pos++
		return true
REDACTED
	return false
REDACTED

func (p *openAIResponsesToolSchemaParser) syntaxError(message string) error {
	return fmt.Errorf("sanitize OpenAI Responses tool schemas: %s at byte %d", message, p.pos)
REDACTED

func decodeOpenAIResponsesJSONString(raw []byte) (string, error) {
	decoded, err := strconv.Unquote(string(raw))
	if err != nil {
		return "", err
REDACTED
	return decoded, nil
REDACTED

func openAIResponsesJSONStringEquals(raw []byte, want string) bool {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return false
REDACTED
	if !bytes.Contains(raw, []byte{'\\'REDACTED) {
		return bytes.Equal(raw[1:len(raw)-1], []byte(want))
REDACTED
	decoded, err := decodeOpenAIResponsesJSONString(raw)
	return err == nil && decoded == want
REDACTED

func openAIResponsesJSONStringMatchesAny(raw []byte, candidates ...string) bool {
	for _, candidate := range candidates {
		if openAIResponsesJSONStringEquals(raw, candidate) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func decodeOpenAIResponsesJSONStringValue(raw []byte) (string, bool) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
REDACTED
	decoded, err := decodeOpenAIResponsesJSONString(raw)
	return decoded, err == nil
REDACTED

func applyOpenAIResponsesToolSchemaEdits(
	body []byte, edits []openAIResponsesToolSchemaEdit,
) ([]byte, bool, error) {
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].start != edits[j].start {
			return edits[i].start < edits[j].start
	REDACTED
		return edits[i].end < edits[j].end
REDACTED)

	merged := edits[:0]
	for _, edit := range edits {
		if len(merged) == 0 || edit.start >= merged[len(merged)-1].end {
			merged = append(merged, edit)
			continue
	REDACTED
		previous := &merged[len(merged)-1]
		if previous.replacement != "" || edit.replacement != "" {
			return nil, false, fmt.Errorf("sanitize OpenAI Responses tool schemas: overlapping patches")
	REDACTED
		if edit.end > previous.end {
			previous.end = edit.end
	REDACTED
REDACTED

	delta := 0
	for _, edit := range merged {
		delta += len(edit.replacement) - (edit.end - edit.start)
REDACTED
	sanitized := make([]byte, 0, len(body)+delta)
	cursor := 0
	for _, edit := range merged {
		sanitized = append(sanitized, body[cursor:edit.start]...)
		sanitized = append(sanitized, edit.replacement...)
		cursor = edit.end
REDACTED
	sanitized = append(sanitized, body[cursor:]...)
	if !json.Valid(sanitized) {
		return nil, false, fmt.Errorf("sanitize OpenAI Responses tool schemas: patch produced invalid JSON")
REDACTED
	return sanitized, true, nil
REDACTED
