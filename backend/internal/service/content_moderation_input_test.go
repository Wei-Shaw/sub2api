package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 当数组末尾不是用户消息时（典型场景：Agent 工具循环结束于 tool/assistant），
// 应直接跳过审计——不再回溯查找历史中的某条用户消息。

func TestExtractContentModerationInput_AnthropicAgentToolLoopSkipsAudit(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"调用一下天气工具"REDACTED,
			{"role":"assistant","content":[{"type":"tool_use","id":"tool_1","name":"weather","input":{REDACTEDREDACTED]REDACTED,
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_1","content":"晴 25 度"REDACTED]REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolAnthropicMessages, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
REDACTED

func TestExtractContentModerationInput_AnthropicFirstTurnExtractsUser(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"Q1"REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolAnthropicMessages, body)

	require.Equal(t, "Q1", input.Text)
REDACTED

func TestExtractContentModerationInput_AnthropicMultiTurnExtractsLatestUser(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"Q1"REDACTED,
			{"role":"assistant","content":"A1"REDACTED,
			{"role":"user","content":"Q2"REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolAnthropicMessages, body)

	require.Equal(t, "Q2", input.Text)
REDACTED

func TestExtractContentModerationInput_AnthropicStreamResendExtractsResend(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"原问题"REDACTED,
			{"role":"assistant","content":"部分回答……"REDACTED,
			{"role":"user","content":"重发"REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolAnthropicMessages, body)

	require.Equal(t, "重发", input.Text)
REDACTED

func TestExtractContentModerationInput_OpenAIChatAgentToolLoopSkipsAudit(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"system","content":"sys"REDACTED,
			{"role":"user","content":"列出我的订单"REDACTED,
			{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"orders","arguments":"{REDACTED"REDACTEDREDACTED]REDACTED,
			{"role":"tool","tool_call_id":"call_1","content":"[]"REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIChat, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
REDACTED

func TestExtractContentModerationInput_OpenAIChatMultiTurnExtractsLatestUser(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"Q1"REDACTED,
			{"role":"assistant","content":"A1"REDACTED,
			{"role":"user","content":"Q2"REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIChat, body)

	require.Equal(t, "Q2", input.Text)
REDACTED

func TestExtractContentModerationInput_GeminiAgentToolLoopSkipsAudit(t *testing.T) {
	body := []byte(`{
		"contents": [
			{"role":"user","parts":[{"text":"查询天气"REDACTED]REDACTED,
			{"role":"model","parts":[{"functionCall":{"name":"weather","args":{REDACTEDREDACTEDREDACTED]REDACTED,
			{"role":"user","parts":[{"functionResponse":{"name":"weather","response":{"temp":25REDACTEDREDACTEDREDACTED]REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolGemini, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
REDACTED

func TestExtractContentModerationInput_GeminiFirstTurnExtractsUser(t *testing.T) {
	body := []byte(`{
		"contents": [
			{"role":"user","parts":[{"text":"你好"REDACTED]REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolGemini, body)

	require.Equal(t, "你好", input.Text)
REDACTED

func TestExtractContentModerationInput_GeminiMultiTurnExtractsLatestUser(t *testing.T) {
	body := []byte(`{
		"contents": [
			{"role":"user","parts":[{"text":"Q1"REDACTED]REDACTED,
			{"role":"model","parts":[{"text":"A1"REDACTED]REDACTED,
			{"role":"user","parts":[{"text":"Q2"REDACTED]REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolGemini, body)

	require.Equal(t, "Q2", input.Text)
REDACTED

func TestExtractContentModerationInput_ResponsesAgentToolLoopSkipsAudit(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"运行测试"REDACTED]REDACTED,
			{"type":"function_call","call_id":"call_1","name":"run_tests","arguments":"{REDACTED"REDACTED,
			{"type":"function_call_output","call_id":"call_1","output":"all passed"REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
REDACTED

func TestExtractContentModerationInput_ResponsesLastUserMessageExtracted(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"first"REDACTED]REDACTED,
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"REDACTED]REDACTED,
			{"type":"message","role":"user","content":[{"type":"input_text","text":"latest"REDACTED]REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Equal(t, "latest", input.Text)
REDACTED

func TestExtractContentModerationInput_ResponsesLastIsAssistantSkipped(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"q1"REDACTED]REDACTED,
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"a1"REDACTED]REDACTED
		]
REDACTED`)

	input := ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body)

	require.Empty(t, input.Text)
	require.Empty(t, input.Images)
REDACTED
