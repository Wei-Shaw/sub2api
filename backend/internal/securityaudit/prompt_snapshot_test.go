package securityaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestExtractPromptSnapshotProtocols(t *testing.T) {
	tests := []struct {
		protocol, body, first string
		count                 int
REDACTED{
		{"openai_chat_completions", `{"messages":[{"role":"user","content":"old"REDACTED,{"role":"assistant","content":"ignore"REDACTED,{"role":"user","content":[{"type":"text","text":"最新😀"REDACTED]REDACTED]REDACTED`, "最新😀", 2REDACTED,
		{"openai_responses", `{"input":[{"role":"user","content":[{"type":"input_text","text":"response text"REDACTED]REDACTED]REDACTED`, "response text", 1REDACTED,
		{"anthropic_messages", `{"messages":[{"role":"user","content":[{"type":"text","text":"claude"REDACTED]REDACTED]REDACTED`, "claude", 1REDACTED,
		{"gemini", `{"contents":[{"role":"user","parts":[{"text":"gemini"REDACTED,{"inline_data":{"data":"BASE64"REDACTEDREDACTED]REDACTED]REDACTED`, "gemini", 1REDACTED,
		{"openai_images", `{"prompt":"draw a cat","image":"BASE64SECRET"REDACTED`, "draw a cat", 1REDACTED,
		{"responses_websocket", `{"type":"response.create","response":{"input":"turn two"REDACTEDREDACTED`, "turn two", 1REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			snapshot, err := ExtractPromptSnapshot(Request{Protocol: tt.protocol, Body: []byte(tt.body), Stage: "http"REDACTED)
		REDACTED
			require.True(t, strings.HasPrefix(snapshot.ScanText, tt.first))
			require.Equal(t, tt.count, snapshot.MessageCount)
			require.Equal(t, utf8.RuneCountInString(snapshot.ScanText), snapshot.PromptLength)
			require.NotEmpty(t, snapshot.PromptHash)
			require.NotContains(t, snapshot.ScanText, "BASE64SECRET")
	REDACTED)
REDACTED
REDACTED

func TestSnapshotRedactsCanariesAndPreservesHashOfScanText(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"PROMPT_CANARY_ABC123 email@example.com +86 138 0013 8000 Bearer AUTH_CANARY_XYZ sk-secretvalue123 password=supersecret123"REDACTED]REDACTED`
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_chat_completions", Body: []byte(body)REDACTED)
REDACTED
	require.NotContains(t, snapshot.RedactedPreview, "ABC123")
	require.NotContains(t, snapshot.RedactedPreview, "email@example.com")
	require.NotContains(t, snapshot.RedactedPreview, "AUTH_CANARY_XYZ")
	require.NotContains(t, snapshot.RedactedPreview, "secretvalue123")
	require.NotContains(t, snapshot.RedactedPreview, "supersecret123")
	require.NotContains(t, snapshot.RedactedPreview, "138 0013 8000")
	require.Contains(t, snapshot.ScanText, "PROMPT_CANARY_ABC123")
	require.NotEqual(t, snapshot.ScanText, snapshot.RedactedPreview)
	digest := sha256.Sum256([]byte(snapshot.ScanText))
	require.Equal(t, hex.EncodeToString(digest[:]), snapshot.PromptHash)
	require.Empty(t, snapshot.Redacted().ScanText)
REDACTED

func TestSplitRunesDoesNotSplitUTF8(t *testing.T) {
	chunks := SplitRunes("中文😀éabc", 2)
	require.Equal(t, []string{"中文", "😀e", "́a", "bc"REDACTED, chunks)
	for _, chunk := range chunks {
		require.True(t, utf8.ValidString(chunk))
REDACTED
	require.Equal(t, "中文😀éabc", strings.Join(chunks, ""))
REDACTED

func TestPromptSnapshotLatestUserMessageIsOnePrioritizedSegment(t *testing.T) {
	body := []byte(`{
		"messages":[
			{"role":"user","content":"历史输入"REDACTED,
			{"role":"assistant","content":"assistant output must be ignored"REDACTED,
			{"role":"tool","content":"tool output must be ignored"REDACTED,
			{"role":"user","content":[
				{"type":"text","text":"最新第一块😀"REDACTED,
				{"type":"image_url","image_url":{"url":"data:image/png;base64,IMAGE_CANARY_BASE64"REDACTEDREDACTED,
				{"type":"text","text":"最新第二块é"REDACTED
			]REDACTED
		]
REDACTED`)
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_chat_completions", Body: bodyREDACTED)
REDACTED
	require.Equal(t, 2, snapshot.MessageCount)
	require.Equal(t, "最新第一块😀\n最新第二块é\n\n历史输入", snapshot.ScanText)
	require.NotContains(t, snapshot.ScanText, "assistant output")
	require.NotContains(t, snapshot.ScanText, "tool output")
	require.NotContains(t, snapshot.ScanText, "IMAGE_CANARY_BASE64")
	require.Equal(t, utf8.RuneCountInString(snapshot.ScanText), snapshot.PromptLength)
REDACTED

func TestPromptSnapshotResponsesShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
REDACTED{
		{name: "string", body: `{"input":"plain response input"REDACTED`, want: "plain response input"REDACTED,
		{name: "message array", body: `{"input":[{"role":"assistant","content":"ignore"REDACTED,{"role":"user","content":[{"type":"input_text","text":"message block"REDACTED]REDACTED]REDACTED`, want: "message block"REDACTED,
		{name: "direct input text", body: `{"input":[{"type":"input_text","text":"direct block"REDACTED]REDACTED`, want: "direct block"REDACTED,
		{name: "single object", body: `{"input":{"role":"user","content":[{"type":"input_text","text":"single object"REDACTED]REDACTEDREDACTED`, want: "single object"REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_responses", Body: []byte(tt.body)REDACTED)
		REDACTED
			require.Equal(t, tt.want, snapshot.ScanText)
	REDACTED)
REDACTED
REDACTED

func TestPromptSnapshotGeminiBatchShapesAndMediaExclusion(t *testing.T) {
	body := []byte(`{
		"contents":{"role":"user","parts":[{"text":"root content"REDACTED,{"inlineData":{"data":"ROOT_BASE64"REDACTEDREDACTED]REDACTED,
		"instances":[{"prompt":"instance prompt"REDACTED],
		"requests":[
			{"contents":[{"role":"model","parts":[{"text":"ignore model"REDACTED]REDACTED,{"role":"user","parts":[{"text":"nested user"REDACTED]REDACTED]REDACTED,
			{"instances":[{"prompt":"nested instance"REDACTED]REDACTED
		]
REDACTED`)
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "gemini", Body: bodyREDACTED)
REDACTED
	require.True(t, strings.HasPrefix(snapshot.ScanText, "nested instance"))
	for _, expected := range []string{"root content", "instance prompt", "nested user", "nested instance"REDACTED {
		require.Contains(t, snapshot.ScanText, expected)
REDACTED
	require.NotContains(t, snapshot.ScanText, "ROOT_BASE64")
	require.NotContains(t, snapshot.ScanText, "ignore model")
REDACTED

func TestPromptSnapshotMediaOnlyExtractsDeterministicTextPrompts(t *testing.T) {
	body := []byte(`{
		"prompt":"draw a lighthouse",
		"image":"data:image/png;base64,IMAGE_CANARY",
		"input":{"negative_prompt":"no fog","image_prompt":"https://example.test/input.png","prompt":"draw a lighthouse"REDACTED,
		"request":{"lyrics":"ocean song","input":"` + strings.Repeat("A", 300) + `"REDACTED,
		"images":[{"description":"nested textual direction","image_url":"https://example.test/image.png"REDACTED]
REDACTED`)
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "grok_media", Body: bodyREDACTED)
REDACTED
	require.Equal(t, 4, snapshot.MessageCount)
	for _, expected := range []string{"draw a lighthouse", "no fog", "ocean song", "nested textual direction"REDACTED {
		require.Contains(t, snapshot.ScanText, expected)
REDACTED
	require.Equal(t, 1, strings.Count(snapshot.ScanText, "draw a lighthouse"))
	require.NotContains(t, snapshot.ScanText, "IMAGE_CANARY")
	require.NotContains(t, snapshot.ScanText, "example.test")
	require.NotContains(t, snapshot.ScanText, strings.Repeat("A", 100))
REDACTED

func TestResponsesWebSocketOnlyAuditsResponseCreateAndPreservesStage(t *testing.T) {
	for _, stage := range []string{"first_turn", "subsequent_turn"REDACTED {
		snapshot, err := ExtractPromptSnapshot(Request{
			Protocol: "openai_responses", Stage: stage,
			Body: []byte(`{"type":"response.create","response":{"model":"gpt-test","input":[{"role":"user","content":[{"type":"input_text","text":"ws turn"REDACTED]REDACTED]REDACTEDREDACTED`),
	REDACTED)
	REDACTED
		require.Equal(t, "ws turn", snapshot.ScanText)
		require.Equal(t, stage, snapshot.Stage)
REDACTED
	_, err := ExtractPromptSnapshot(Request{
		Protocol: "openai_responses", Stage: "subsequent_turn",
		Body: []byte(`{"type":"conversation.item.create","response":{"input":"must not scan this frame"REDACTEDREDACTED`),
REDACTED)
	require.True(t, errors.Is(err, ErrNoPromptText))
REDACTED

func TestPromptSnapshotEmptyAndLongUnicodeInput(t *testing.T) {
	_, err := ExtractPromptSnapshot(Request{Protocol: "openai_chat_completions", Body: []byte(`{"messages":[{"role":"assistant","content":"not user"REDACTED,{"role":"user","content":"  "REDACTED]REDACTED`)REDACTED)
	require.True(t, errors.Is(err, ErrNoPromptText))

	latest := strings.Repeat("最新😀é", 80)
	history := strings.Repeat("历史中文", 80)
	body := []byte(`{"messages":[{"role":"user","content":` + string(mustJSON(t, history)) + `REDACTED,{"role":"user","content":` + string(mustJSON(t, latest)) + `REDACTED]REDACTED`)
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "openai_chat_completions", Body: bodyREDACTED)
REDACTED
	require.True(t, strings.HasPrefix(snapshot.ScanText, latest))
	chunks := SplitRunes(snapshot.ScanText, 127)
	require.Equal(t, snapshot.ScanText, strings.Join(chunks, ""))
	for _, chunk := range chunks {
		require.LessOrEqual(t, len([]rune(chunk)), 127)
		require.True(t, utf8.ValidString(chunk))
REDACTED
REDACTED

func mustJSON(t *testing.T, value string) []byte {
REDACTED
	raw, err := json.Marshal(value)
REDACTED
	return raw
REDACTED
