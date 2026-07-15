package service

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

// contentSessionSeedPrefix prevents collisions between content-derived seeds
// and explicit session IDs (e.g. "sess-xxx" or "compat_cc_xxx").
const contentSessionSeedPrefix = "compat_cs_"

// contentStablePrefixSessionSeedPrefix distinguishes cache identities derived
// only from request fields that remain stable across independent prompts.
const contentStablePrefixSessionSeedPrefix = "compat_csp_"

// deriveOpenAIContentSessionSeed builds a stable session seed from an
// OpenAI-format request body. Only fields constant across conversation turns
// are included: model, tools/functions definitions, system/developer prompts,
// instructions (Responses API), and the first user message.
// Supports both Chat Completions (messages) and Responses API (input).
func deriveOpenAIContentSessionSeed(body []byte) string {
	if len(body) == 0 {
		return ""
REDACTED

	const (
		modelField = iota
		toolsField
		functionsField
		instructionsField
		messagesField
		inputField
		contentSessionSeedFieldCount
		allContentSessionSeedFields = 1<<contentSessionSeedFieldCount - 1
	)
	var fields [contentSessionSeedFieldCount]gjson.Result
	var seen uint8
	// Match gjson.GetBytes by starting at the first root container, even when
	// malformed input has a non-JSON prefix.
	root := body
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '{':
			root = body[i:]
			goto scanRoot
		case '[':
			return ""
	REDACTED
REDACTED
	return ""

scanRoot:
	nextKeyOffset := 1
	parseRawJSONView(root).ForEach(func(key, value gjson.Result) bool {
		if key.Index < nextKeyOffset || key.Index > len(root) {
			return false
	REDACTED
		// Result.ForEach can continue after the root 'REDACTED' on malformed input.
		// The separator range excludes braces inside the preceding parsed value.
		if bytes.IndexByte(root[nextKeyOffset:key.Index], 'REDACTED') >= 0 {
			return false
	REDACTED
		nextKeyOffset = value.Index + len(value.Raw)

		field := -1
		switch key.Str {
		case "model":
			field = modelField
		case "tools":
			field = toolsField
		case "functions":
			field = functionsField
		case "instructions":
			field = instructionsField
		case "messages":
			field = messagesField
		case "input":
			field = inputField
	REDACTED
		if field < 0 {
			return true
	REDACTED
		mask := uint8(1 << field)
		if seen&mask == 0 {
			fields[field] = value
			seen |= mask
	REDACTED
		return seen != allContentSessionSeedFields
REDACTED)

	var b strings.Builder

	if model := fields[modelField].String(); model != "" {
		_, _ = b.WriteString("model=")
		_, _ = b.WriteString(model)
REDACTED

	if tools := fields[toolsField]; tools.Exists() && tools.IsArray() && tools.Raw != "[]" {
		_, _ = b.WriteString("|tools=")
		_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(tools.Raw)))
REDACTED

	if funcs := fields[functionsField]; funcs.Exists() && funcs.IsArray() && funcs.Raw != "[]" {
		_, _ = b.WriteString("|functions=")
		_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(funcs.Raw)))
REDACTED

	if instr := fields[instructionsField].String(); instr != "" {
		_, _ = b.WriteString("|instructions=")
		_, _ = b.WriteString(instr)
REDACTED

	firstUserCaptured := false

	msgs := fields[messagesField]
	if msgs.Exists() && msgs.IsArray() {
		msgs.ForEach(func(_, msg gjson.Result) bool {
			role := msg.Get("role").String()
			switch role {
			case "system", "developer":
				_, _ = b.WriteString("|system=")
				if c := msg.Get("content"); c.Exists() {
					_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
			REDACTED
			case "user":
				if !firstUserCaptured {
					_, _ = b.WriteString("|first_user=")
					if c := msg.Get("content"); c.Exists() {
						_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
				REDACTED
					firstUserCaptured = true
			REDACTED
		REDACTED
			return true
	REDACTED)
REDACTED else if inp := fields[inputField]; inp.Exists() {
		if inp.Type == gjson.String {
			_, _ = b.WriteString("|input=")
			_, _ = b.WriteString(inp.String())
	REDACTED else if inp.IsArray() {
			inp.ForEach(func(_, item gjson.Result) bool {
				role := item.Get("role").String()
				switch role {
				case "system", "developer":
					_, _ = b.WriteString("|system=")
					if c := item.Get("content"); c.Exists() {
						_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
				REDACTED
				case "user":
					if !firstUserCaptured {
						_, _ = b.WriteString("|first_user=")
						if c := item.Get("content"); c.Exists() {
							_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
					REDACTED
						firstUserCaptured = true
				REDACTED
			REDACTED
				if !firstUserCaptured && item.Get("type").String() == "input_text" {
					_, _ = b.WriteString("|first_user=")
					if text := item.Get("text").String(); text != "" {
						_, _ = b.WriteString(text)
				REDACTED
					firstUserCaptured = true
			REDACTED
				return true
		REDACTED)
	REDACTED
REDACTED

	if b.Len() == 0 {
		return ""
REDACTED
	return contentSessionSeedPrefix + b.String()
REDACTED

// deriveOpenAIAnchoredContentSessionSeed returns the legacy content-derived
// seed only when it contains a meaningful user/input anchor. This preserves
// the existing session derivation while preventing model-only requests from
// becoming a tenant-wide cache routing identity.
func deriveOpenAIAnchoredContentSessionSeed(body []byte) string {
	if !hasOpenAIContentSessionUserAnchor(body) {
		return ""
REDACTED
	return deriveOpenAIContentSessionSeed(body)
REDACTED

func hasOpenAIContentSessionUserAnchor(body []byte) bool {
	if len(body) == 0 {
		return false
REDACTED

	if messages := gjson.GetBytes(body, "messages"); messages.Exists() && messages.IsArray() {
		anchored := false
		messages.ForEach(func(_, message gjson.Result) bool {
			if strings.TrimSpace(message.Get("role").String()) != "user" {
				return true
		REDACTED
			anchored = hasMeaningfulOpenAIContent(message.Get("content"))
			return false
	REDACTED)
		return anchored
REDACTED

	input := gjson.GetBytes(body, "input")
	if !input.Exists() {
		return false
REDACTED
	if input.Type == gjson.String {
		return strings.TrimSpace(input.String()) != ""
REDACTED
	if !input.IsArray() {
		return false
REDACTED

	anchored := false
	input.ForEach(func(_, item gjson.Result) bool {
		if strings.TrimSpace(item.Get("role").String()) == "user" {
			anchored = hasMeaningfulOpenAIContent(item.Get("content"))
			return false
	REDACTED
		if strings.TrimSpace(item.Get("type").String()) == "input_text" {
			anchored = strings.TrimSpace(item.Get("text").String()) != ""
			return false
	REDACTED
		return true
REDACTED)
	return anchored
REDACTED

func hasMeaningfulOpenAIContent(content gjson.Result) bool {
	if !content.Exists() || content.Type == gjson.Null {
		return false
REDACTED
	if content.Type == gjson.String {
		return strings.TrimSpace(content.String()) != ""
REDACTED
	if !content.IsArray() {
		normalized, ok := normalizeNonEmptyCompatSeedJSON(content)
		return ok && strings.TrimSpace(normalized) != ""
REDACTED

	meaningful := false
	content.ForEach(func(_, item gjson.Result) bool {
		if item.Type == gjson.String {
			meaningful = strings.TrimSpace(item.String()) != ""
	REDACTED else if text := item.Get("text"); text.Exists() {
			meaningful = strings.TrimSpace(text.String()) != ""
	REDACTED else {
			_, meaningful = normalizeNonEmptyCompatSeedJSON(item)
	REDACTED
		return !meaningful
REDACTED)
	return meaningful
REDACTED

// deriveOpenAIStablePrefixSessionSeed builds a seed from the reusable prefix
// of an OpenAI-format request. User and assistant content are deliberately
// excluded so independent prompts with the same system/tool prefix can share
// an upstream prompt-cache routing identity.
//
// An empty result means the request has no meaningful stable prefix. Callers
// must then use a narrower fallback instead of grouping all requests by tenant
// and model alone.
func deriveOpenAIStablePrefixSessionSeed(body []byte) string {
	if len(body) == 0 {
		return ""
REDACTED

	var b strings.Builder
	hasStablePrefix := false
	appendJSON := func(label string, value gjson.Result) {
		normalized, ok := normalizeNonEmptyCompatSeedJSON(value)
		if !ok {
			return
	REDACTED
		_, _ = b.WriteString("|")
		_, _ = b.WriteString(label)
		_, _ = b.WriteString("=")
		_, _ = b.WriteString(normalized)
		hasStablePrefix = true
REDACTED

	if tools := gjson.GetBytes(body, "tools"); tools.Exists() && tools.IsArray() {
		appendJSON("tools", tools)
REDACTED
	if funcs := gjson.GetBytes(body, "functions"); funcs.Exists() && funcs.IsArray() {
		appendJSON("functions", funcs)
REDACTED
	if instructions := gjson.GetBytes(body, "instructions"); strings.TrimSpace(instructions.String()) != "" {
		appendJSON("instructions", instructions)
REDACTED

	appendSystemMessages := func(items gjson.Result) {
		items.ForEach(func(_, item gjson.Result) bool {
			role := strings.TrimSpace(item.Get("role").String())
			switch role {
			case "system", "developer":
				appendJSON(role, item.Get("content"))
		REDACTED
			return true
	REDACTED)
REDACTED

	if messages := gjson.GetBytes(body, "messages"); messages.Exists() && messages.IsArray() {
		appendSystemMessages(messages)
REDACTED else if input := gjson.GetBytes(body, "input"); input.Exists() && input.IsArray() {
		appendSystemMessages(input)
REDACTED

	if !hasStablePrefix {
		return ""
REDACTED
	return contentStablePrefixSessionSeedPrefix + b.String()
REDACTED

func normalizeNonEmptyCompatSeedJSON(value gjson.Result) (string, bool) {
	if !value.Exists() || value.Type == gjson.Null {
		return "", false
REDACTED
	normalized := normalizeCompatSeedJSON(json.RawMessage(value.Raw))
	switch normalized {
	case "", `""`, "[]", "{REDACTED", "null":
		return "", false
	default:
		return normalized, true
REDACTED
REDACTED
