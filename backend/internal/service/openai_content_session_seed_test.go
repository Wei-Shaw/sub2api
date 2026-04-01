package service

import (
	"testing"

	"github.com/stretchr/testify/require"
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
