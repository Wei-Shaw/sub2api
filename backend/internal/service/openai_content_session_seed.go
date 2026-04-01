package service

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

// contentSessionSeedPrefix prevents collisions between content-derived seeds
// and explicit session IDs (e.g. "sess-xxx" or "compat_cc_xxx").
const contentSessionSeedPrefix = "compat_cs_"

// deriveOpenAIContentSessionSeed builds a stable session seed from an
// OpenAI-format request body. Only fields constant across conversation turns
// are included: model, tools/functions definitions, system/developer prompts,
// instructions (Responses API), and the first user message.
// Supports both Chat Completions (messages) and Responses API (input).
func deriveOpenAIContentSessionSeed(body []byte) string {
	if len(body) == 0 {
		return ""
REDACTED

	var b strings.Builder

	if model := gjson.GetBytes(body, "model").String(); model != "" {
		b.WriteString("model=")
		b.WriteString(model)
REDACTED

	if tools := gjson.GetBytes(body, "tools"); tools.Exists() && tools.IsArray() && tools.Raw != "[]" {
		b.WriteString("|tools=")
		b.WriteString(normalizeCompatSeedJSON(json.RawMessage(tools.Raw)))
REDACTED

	if funcs := gjson.GetBytes(body, "functions"); funcs.Exists() && funcs.IsArray() && funcs.Raw != "[]" {
		b.WriteString("|functions=")
		b.WriteString(normalizeCompatSeedJSON(json.RawMessage(funcs.Raw)))
REDACTED

	if instr := gjson.GetBytes(body, "instructions").String(); instr != "" {
		b.WriteString("|instructions=")
		b.WriteString(instr)
REDACTED

	firstUserCaptured := false

	msgs := gjson.GetBytes(body, "messages")
	if msgs.Exists() && msgs.IsArray() {
		msgs.ForEach(func(_, msg gjson.Result) bool {
			role := msg.Get("role").String()
			switch role {
			case "system", "developer":
				b.WriteString("|system=")
				if c := msg.Get("content"); c.Exists() {
					b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
			REDACTED
			case "user":
				if !firstUserCaptured {
					b.WriteString("|first_user=")
					if c := msg.Get("content"); c.Exists() {
						b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
				REDACTED
					firstUserCaptured = true
			REDACTED
		REDACTED
			return true
	REDACTED)
REDACTED else if inp := gjson.GetBytes(body, "input"); inp.Exists() {
		if inp.Type == gjson.String {
			b.WriteString("|input=")
			b.WriteString(inp.String())
	REDACTED else if inp.IsArray() {
			inp.ForEach(func(_, item gjson.Result) bool {
				role := item.Get("role").String()
				switch role {
				case "system", "developer":
					b.WriteString("|system=")
					if c := item.Get("content"); c.Exists() {
						b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
				REDACTED
				case "user":
					if !firstUserCaptured {
						b.WriteString("|first_user=")
						if c := item.Get("content"); c.Exists() {
							b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
					REDACTED
						firstUserCaptured = true
				REDACTED
			REDACTED
				if !firstUserCaptured && item.Get("type").String() == "input_text" {
					b.WriteString("|first_user=")
					if text := item.Get("text").String(); text != "" {
						b.WriteString(text)
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
