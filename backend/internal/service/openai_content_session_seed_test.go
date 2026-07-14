package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeriveOpenAIContentSessionSeed_EmptyInputs(t *testing.T) {
	require.Empty(t, deriveOpenAIContentSessionSeed(nil))
	require.Empty(t, deriveOpenAIContentSessionSeed([]byte{REDACTED))
	require.Empty(t, deriveOpenAIContentSessionSeed([]byte(`{REDACTED`)))
REDACTED

func TestDeriveOpenAIContentSessionSeed_ModelOnly(t *testing.T) {
	seed := deriveOpenAIContentSessionSeed([]byte(`{"model":"gpt-5.4"REDACTED`))
	require.Contains(t, seed, contentSessionSeedPrefix)
	require.Contains(t, seed, "model=gpt-5.4")
REDACTED

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_StableAcrossTurns(t *testing.T) {
	turn1 := []byte(`{
		"model": "gpt-5.4",
		"messages": [
			{"role": "system", "content": "You are helpful."REDACTED,
			{"role": "user", "content": "Hello"REDACTED
		]
REDACTED`)
	turn2 := []byte(`{
		"model": "gpt-5.4",
		"messages": [
			{"role": "system", "content": "You are helpful."REDACTED,
			{"role": "user", "content": "Hello"REDACTED,
			{"role": "assistant", "content": "Hi there!"REDACTED,
			{"role": "user", "content": "How are you?"REDACTED
		]
REDACTED`)
	s1 := deriveOpenAIContentSessionSeed(turn1)
	s2 := deriveOpenAIContentSessionSeed(turn2)
	require.Equal(t, s1, s2, "seed should be stable across later turns")
	require.NotEmpty(t, s1)
REDACTED

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_DifferentFirstUserDiffers(t *testing.T) {
	req1 := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Question A"REDACTED]REDACTED`)
	req2 := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Question B"REDACTED]REDACTED`)
	s1 := deriveOpenAIContentSessionSeed(req1)
	s2 := deriveOpenAIContentSessionSeed(req2)
	require.NotEqual(t, s1, s2)
REDACTED

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_DifferentSystemDiffers(t *testing.T) {
	req1 := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"A"REDACTED,{"role":"user","content":"Hi"REDACTED]REDACTED`)
	req2 := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"B"REDACTED,{"role":"user","content":"Hi"REDACTED]REDACTED`)
	s1 := deriveOpenAIContentSessionSeed(req1)
	s2 := deriveOpenAIContentSessionSeed(req2)
	require.NotEqual(t, s1, s2)
REDACTED

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_DifferentModelDiffers(t *testing.T) {
	req1 := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Hi"REDACTED]REDACTED`)
	req2 := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"Hi"REDACTED]REDACTED`)
	s1 := deriveOpenAIContentSessionSeed(req1)
	s2 := deriveOpenAIContentSessionSeed(req2)
	require.NotEqual(t, s1, s2)
REDACTED

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_WithTools(t *testing.T) {
	withTools := []byte(`{
		"model": "gpt-5.4",
		"tools": [{"type":"function","function":{"name":"get_weather"REDACTEDREDACTED],
		"messages": [{"role": "user", "content": "Hello"REDACTED]
REDACTED`)
	withoutTools := []byte(`{
		"model": "gpt-5.4",
		"messages": [{"role": "user", "content": "Hello"REDACTED]
REDACTED`)
	s1 := deriveOpenAIContentSessionSeed(withTools)
	s2 := deriveOpenAIContentSessionSeed(withoutTools)
	require.NotEqual(t, s1, s2, "tools should affect the seed")
	require.Contains(t, s1, "|tools=")
REDACTED

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_WithFunctions(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"functions": [{"name":"get_weather","parameters":{REDACTEDREDACTED],
		"messages": [{"role": "user", "content": "Hello"REDACTED]
REDACTED`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|functions=")
REDACTED

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_DeveloperRole(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"messages": [
			{"role": "developer", "content": "You are helpful."REDACTED,
			{"role": "user", "content": "Hello"REDACTED
		]
REDACTED`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|system=")
	require.Contains(t, seed, "|first_user=")
REDACTED

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_StructuredContent(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"messages": [
			{"role": "user", "content": [{"type":"text","text":"Hello"REDACTED]REDACTED
		]
REDACTED`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.NotEmpty(t, seed)
	require.Contains(t, seed, "|first_user=")
REDACTED

func TestDeriveOpenAIContentSessionSeed_ResponsesAPI_InputString(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"Hello, how are you?"REDACTED`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|input=Hello, how are you?")
REDACTED

func TestDeriveOpenAIContentSessionSeed_ResponsesAPI_InputArray(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"input": [
			{"role": "system", "content": "You are helpful."REDACTED,
			{"role": "user", "content": "Hello"REDACTED
		]
REDACTED`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|system=")
	require.Contains(t, seed, "|first_user=")
REDACTED

func TestDeriveOpenAIContentSessionSeed_ResponsesAPI_WithInstructions(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"instructions": "You are a coding assistant.",
		"input": "Write a hello world"
REDACTED`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|instructions=You are a coding assistant.")
	require.Contains(t, seed, "|input=Write a hello world")
REDACTED

func TestDeriveOpenAIContentSessionSeed_Deterministic(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"messages": [
			{"role": "system", "content": "You are helpful."REDACTED,
			{"role": "user", "content": "Hello"REDACTED
		]
REDACTED`)
	s1 := deriveOpenAIContentSessionSeed(body)
	s2 := deriveOpenAIContentSessionSeed(body)
	require.Equal(t, s1, s2, "seed must be deterministic")
REDACTED

func TestDeriveOpenAIContentSessionSeed_PrefixPresent(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Hi"REDACTED]REDACTED`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.True(t, len(seed) > len(contentSessionSeedPrefix))
	require.Equal(t, contentSessionSeedPrefix, seed[:len(contentSessionSeedPrefix)])
REDACTED

func TestDeriveOpenAIContentSessionSeed_EmptyToolsIgnored(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","tools":[],"messages":[{"role":"user","content":"Hi"REDACTED]REDACTED`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.NotContains(t, seed, "|tools=")
REDACTED

func TestDeriveOpenAIContentSessionSeed_MessagesPreferredOverInput(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"messages": [{"role": "user", "content": "from messages"REDACTED],
		"input": "from input"
REDACTED`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|first_user=")
	require.NotContains(t, seed, "|input=")
REDACTED

func TestDeriveOpenAIContentSessionSeed_JSONCanonicalisation(t *testing.T) {
	compact := []byte(`{"model":"gpt-5.4","tools":[{"type":"function","function":{"name":"get_weather","description":"Get weather"REDACTEDREDACTED],"messages":[{"role":"user","content":"Hi"REDACTED]REDACTED`)
	spaced := []byte(`{
		"model": "gpt-5.4",
		"tools": [
			{ "type" : "function", "function": { "description": "Get weather", "name": "get_weather" REDACTED REDACTED
		],
		"messages": [ { "role": "user", "content": "Hi" REDACTED ]
REDACTED`)
	s1 := deriveOpenAIContentSessionSeed(compact)
	s2 := deriveOpenAIContentSessionSeed(spaced)
	require.Equal(t, s1, s2, "different formatting of identical JSON should produce the same seed")
REDACTED

func TestDeriveOpenAIContentSessionSeed_SingleScanMatchesLegacyBytes(t *testing.T) {
	largeValue := strings.Repeat("payload", 1<<17)
	tests := []struct {
		name string
		body []byte
REDACTED{
		{
			name: "large chat completions",
			body: []byte(`{"metadata":"` + largeValue + `","model":"gpt-5.4","tools":[{"type":"function","function":{"name":"lookup"REDACTEDREDACTED],"functions":[{"name":"legacy_lookup"REDACTED],"messages":[{"role":"system","content":"System prompt"REDACTED,{"role":"developer","content":[{"type":"text","text":"Developer prompt"REDACTED]REDACTED,{"role":"user","content":"Hello"REDACTED]REDACTED`),
	REDACTED,
		{
			name: "large responses",
			body: []byte(`{"metadata":"` + largeValue + `","model":"gpt-5.4","instructions":"Be concise.","tools":[{"type":"function","name":"lookup"REDACTED],"input":[{"role":"system","content":"System prompt"REDACTED,{"role":"user","content":[{"type":"input_text","text":"Hello"REDACTED]REDACTED]REDACTED`),
	REDACTED,
		{
			name: "fields in reverse order",
			body: []byte(`{"input":"fallback input","messages":[{"role":"user","content":"chat wins"REDACTED],"instructions":"Be concise.","functions":[{"name":"lookup"REDACTED],"tools":[{"type":"function","name":"lookup"REDACTED],"model":"gpt-5.4"REDACTED`),
	REDACTED,
		{
			name: "missing and wrong type fields",
			body: []byte(`{"tools":[],"functions":null,"instructions":0,"messages":{REDACTED,"input":[{"type":"input_text","text":"fallback"REDACTED]REDACTED`),
	REDACTED,
		{
			name: "duplicate fields keep first value",
			body: []byte(`{"model":"first","model":"second","tools":[{"name":"first"REDACTED],"tools":[{"name":"second"REDACTED],"functions":[],"functions":[{"name":"second"REDACTED],"instructions":"first","instructions":"second","messages":null,"messages":[{"role":"user","content":"second"REDACTED],"input":"first input","input":"second input"REDACTED`),
	REDACTED,
		{
			name: "escaped field names",
			body: []byte(`{"mo\u0064el":"gpt-5.4","mess\u0061ges":[{"role":"user","content":"Hello"REDACTED]REDACTED`),
	REDACTED,
		{
			name: "trailing object fields are outside the root",
			body: []byte(`{"foo":1REDACTED{"model":"trailing","input":"trailing input"REDACTED`),
	REDACTED,
		{
			name: "trailing quoted fields are outside the root",
			body: []byte(`{"model":"root"REDACTED"input":"trailing input"`),
	REDACTED,
		{
			name: "leading garbage before the root",
			body: []byte(`garbage{"model":"gpt-5.4","input":"Hello"REDACTED`),
	REDACTED,
		{
			name: "escaped braces remain inside string values",
			body: []byte(`{"metadata":"escaped REDACTED and [ and \" quote","model":"root","input":"Hello"REDACTED{"model":"trailing"REDACTED`),
	REDACTED,
		{
			name: "nested braces do not end the root",
			body: []byte(`{"metadata":{"nested":"REDACTED ]"REDACTED,"model":"root","input":"Hello"REDACTED{"model":"trailing"REDACTED`),
	REDACTED,
		{
			name: "root array does not expose nested or trailing object fields",
			body: []byte(`[{"model":"nested"REDACTED]{"model":"trailing","input":"trailing input"REDACTED`),
	REDACTED,
		{
			name: "trailing messages do not override root input",
			body: []byte(`{"input":"root input"REDACTED{"messages":[{"role":"user","content":"trailing"REDACTED]REDACTED`),
	REDACTED,
		{
			name: "truncated string containing a closing brace",
			body: []byte(`{"model":"root","metadata":"still REDACTED inside`),
	REDACTED,
		{
			name: "lenient truncated body",
			body: []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Hello"REDACTED]`),
	REDACTED,
REDACTED

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, legacyDeriveOpenAIContentSessionSeed(test.body), deriveOpenAIContentSessionSeed(test.body))
	REDACTED)
REDACTED
REDACTED

func TestDeriveOpenAIContentSessionSeed_AllTruncationOffsetsMatchLegacyBytes(t *testing.T) {
	bodies := []string{
		`{"model":"gpt-5.4","tools":[{"type":"function","function":{"name":"lookup"REDACTEDREDACTED],"functions":[{"name":"legacy"REDACTED],"instructions":"escaped \" REDACTED text","messages":[{"role":"system","content":"System"REDACTED,{"role":"user","content":[{"type":"text","text":"Hello"REDACTED]REDACTED],"input":"fallback"REDACTED`,
		`{"model":"gpt-5.4","instructions":"Be concise.","tools":[{"type":"function","name":"lookup"REDACTED],"input":[{"role":"system","content":"System"REDACTED,{"role":"user","content":[{"type":"input_text","text":"Hello"REDACTED]REDACTED]REDACTED`,
REDACTED
	for bodyIndex, body := range bodies {
		for end := 1; end < len(body); end++ {
			truncated := []byte(body[:end])
			require.Equalf(t, legacyDeriveOpenAIContentSessionSeed(truncated), deriveOpenAIContentSessionSeed(truncated), "body %d truncated at byte %d", bodyIndex, end)
	REDACTED
REDACTED
REDACTED

func legacyDeriveOpenAIContentSessionSeed(body []byte) string {
	if len(body) == 0 {
		return ""
REDACTED

	var b strings.Builder

	if model := gjson.GetBytes(body, "model").String(); model != "" {
		_, _ = b.WriteString("model=")
		_, _ = b.WriteString(model)
REDACTED

	if tools := gjson.GetBytes(body, "tools"); tools.Exists() && tools.IsArray() && tools.Raw != "[]" {
		_, _ = b.WriteString("|tools=")
		_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(tools.Raw)))
REDACTED

	if funcs := gjson.GetBytes(body, "functions"); funcs.Exists() && funcs.IsArray() && funcs.Raw != "[]" {
		_, _ = b.WriteString("|functions=")
		_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(funcs.Raw)))
REDACTED

	if instr := gjson.GetBytes(body, "instructions").String(); instr != "" {
		_, _ = b.WriteString("|instructions=")
		_, _ = b.WriteString(instr)
REDACTED

	firstUserCaptured := false

	msgs := gjson.GetBytes(body, "messages")
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
REDACTED else if inp := gjson.GetBytes(body, "input"); inp.Exists() {
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

func TestDeriveOpenAIContentSessionSeed_ResponsesAPI_InputTextTypedItem(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"input": [{"type": "input_text", "text": "Hello world"REDACTED]
REDACTED`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|first_user=")
	require.Contains(t, seed, "Hello world")
REDACTED

func TestDeriveOpenAIContentSessionSeed_ResponsesAPI_TypedMessageItem(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"input": [{"type": "message", "role": "user", "content": "Hello from typed message"REDACTED]
REDACTED`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|first_user=")
	require.Contains(t, seed, "Hello from typed message")
REDACTED

func TestDeriveOpenAIStablePrefixSessionSeed_IgnoresUserContent(t *testing.T) {
	first := []byte(`{
		"model": "grok",
		"instructions": "Be concise.",
		"tools": [{"type":"function","name":"lookup","parameters":{"type":"object"REDACTEDREDACTED],
		"input": [{"role":"user","content":"Question A"REDACTED]
REDACTED`)
	second := []byte(`{
		"model": "grok",
		"instructions": "Be concise.",
		"tools": [{"parameters":{"type":"object"REDACTED,"name":"lookup","type":"function"REDACTED],
		"input": [{"role":"user","content":"Question B"REDACTED]
REDACTED`)

	firstSeed := deriveOpenAIStablePrefixSessionSeed(first)
	secondSeed := deriveOpenAIStablePrefixSessionSeed(second)

	require.NotEmpty(t, firstSeed)
	require.Equal(t, firstSeed, secondSeed)
	require.NotContains(t, firstSeed, "Question A")
	require.NotContains(t, firstSeed, "first_user")
REDACTED

func TestDeriveOpenAIStablePrefixSessionSeed_IsolatesStablePrefixFields(t *testing.T) {
	base := []byte(`{
		"instructions":"Be concise.",
		"tools":[{"type":"function","name":"lookup"REDACTED],
		"input":[{"role":"system","content":"System A"REDACTED,{"role":"user","content":"Question"REDACTED]
REDACTED`)
	differentInstructions := []byte(`{
		"instructions":"Be detailed.",
		"tools":[{"type":"function","name":"lookup"REDACTED],
		"input":[{"role":"system","content":"System A"REDACTED,{"role":"user","content":"Question"REDACTED]
REDACTED`)
	differentTools := []byte(`{
		"instructions":"Be concise.",
		"tools":[{"type":"function","name":"search"REDACTED],
		"input":[{"role":"system","content":"System A"REDACTED,{"role":"user","content":"Question"REDACTED]
REDACTED`)
	differentSystem := []byte(`{
		"instructions":"Be concise.",
		"tools":[{"type":"function","name":"lookup"REDACTED],
		"input":[{"role":"system","content":"System B"REDACTED,{"role":"user","content":"Question"REDACTED]
REDACTED`)

	baseSeed := deriveOpenAIStablePrefixSessionSeed(base)
	require.NotEqual(t, baseSeed, deriveOpenAIStablePrefixSessionSeed(differentInstructions))
	require.NotEqual(t, baseSeed, deriveOpenAIStablePrefixSessionSeed(differentTools))
	require.NotEqual(t, baseSeed, deriveOpenAIStablePrefixSessionSeed(differentSystem))
REDACTED

func TestDeriveOpenAIStablePrefixSessionSeed_ChatSystemAndDeveloper(t *testing.T) {
	first := []byte(`{
		"messages":[
			{"role":"system","content":"System prompt"REDACTED,
			{"role":"developer","content":[{"type":"text","text":"Developer prompt"REDACTED]REDACTED,
			{"role":"user","content":"Question A"REDACTED
		]
REDACTED`)
	second := []byte(`{
		"messages":[
			{"role":"system","content":"System prompt"REDACTED,
			{"role":"developer","content":[{"text":"Developer prompt","type":"text"REDACTED]REDACTED,
			{"role":"user","content":"Question B"REDACTED
		]
REDACTED`)

	firstSeed := deriveOpenAIStablePrefixSessionSeed(first)
	require.Equal(t, firstSeed, deriveOpenAIStablePrefixSessionSeed(second))
	require.Contains(t, firstSeed, "System prompt")
	require.Contains(t, firstSeed, "Developer prompt")
REDACTED

func TestDeriveOpenAIStablePrefixSessionSeed_EncodesSystemAndDeveloperRoles(t *testing.T) {
	systemThenDeveloper := []byte(`{
		"messages":[
			{"role":"system","content":"Prompt A"REDACTED,
			{"role":"developer","content":"Prompt B"REDACTED
		]
REDACTED`)
	developerThenSystem := []byte(`{
		"messages":[
			{"role":"developer","content":"Prompt A"REDACTED,
			{"role":"system","content":"Prompt B"REDACTED
		]
REDACTED`)

	firstSeed := deriveOpenAIStablePrefixSessionSeed(systemThenDeveloper)
	secondSeed := deriveOpenAIStablePrefixSessionSeed(developerThenSystem)

	require.NotEqual(t, firstSeed, secondSeed)
	require.Contains(t, firstSeed, "|system=")
	require.Contains(t, firstSeed, "|developer=")
REDACTED

func TestDeriveOpenAIStablePrefixSessionSeed_EncodesInstructionDelimiters(t *testing.T) {
	instructionOnly := []byte(`{
		"instructions":"foo|system=\"bar\""
REDACTED`)
	instructionAndSystem := []byte(`{
		"instructions":"foo",
		"input":[{"role":"system","content":"bar"REDACTED]
REDACTED`)

	firstSeed := deriveOpenAIStablePrefixSessionSeed(instructionOnly)
	secondSeed := deriveOpenAIStablePrefixSessionSeed(instructionAndSystem)

	require.NotEmpty(t, firstSeed)
	require.NotEmpty(t, secondSeed)
	require.NotEqual(t, firstSeed, secondSeed)
REDACTED

func TestDeriveOpenAIAnchoredContentSessionSeed_RequiresMeaningfulAnchor(t *testing.T) {
	emptyAnchors := [][]byte{
		nil,
		[]byte(`{"model":"grok"REDACTED`),
		[]byte(`{"model":"grok","messages":[{"role":"assistant","content":"answer"REDACTED]REDACTED`),
		[]byte(`{"model":"grok","messages":[{"role":"user","content":"  "REDACTED]REDACTED`),
		[]byte(`{"model":"grok","messages":[{"role":"user","content":[{"type":"text","text":""REDACTED]REDACTED]REDACTED`),
		[]byte(`{"model":"grok","input":"  "REDACTED`),
		[]byte(`{"model":"grok","input":[{"type":"input_text","text":""REDACTED]REDACTED`),
REDACTED
	for _, body := range emptyAnchors {
		require.Empty(t, deriveOpenAIAnchoredContentSessionSeed(body))
REDACTED

	meaningfulAnchors := [][]byte{
		[]byte(`{"model":"grok","messages":[{"role":"user","content":"question"REDACTED]REDACTED`),
		[]byte(`{"model":"grok","messages":[{"role":"user","content":[{"type":"text","text":"question"REDACTED]REDACTED]REDACTED`),
		[]byte(`{"model":"grok","input":"question"REDACTED`),
		[]byte(`{"model":"grok","input":[{"type":"input_text","text":"question"REDACTED]REDACTED`),
REDACTED
	for _, body := range meaningfulAnchors {
		require.NotEmpty(t, deriveOpenAIAnchoredContentSessionSeed(body))
REDACTED
REDACTED

func TestDeriveOpenAIStablePrefixSessionSeed_RequiresMeaningfulPrefix(t *testing.T) {
	tests := [][]byte{
		nil,
		[]byte(`{REDACTED`),
		[]byte(`{"model":"grok","input":"Question A"REDACTED`),
		[]byte(`{"model":"grok","tools":[],"input":"Question A"REDACTED`),
		[]byte(`{"model":"grok","functions":[],"instructions":"  ","messages":[{"role":"system","content":""REDACTED,{"role":"user","content":"Question A"REDACTED]REDACTED`),
REDACTED

	for _, body := range tests {
		require.Empty(t, deriveOpenAIStablePrefixSessionSeed(body))
REDACTED
REDACTED
