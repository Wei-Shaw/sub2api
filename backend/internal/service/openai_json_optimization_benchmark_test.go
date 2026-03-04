package service

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

var (
	benchmarkToolContinuationBoolSink bool
	benchmarkWSParseStringSink        string
	benchmarkWSParseMapSink           map[string]any
	benchmarkUsageSink                OpenAIUsage
)

func BenchmarkToolContinuationValidationLegacy(b *testing.B) {
	reqBody := benchmarkToolContinuationRequestBody()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkToolContinuationBoolSink = legacyValidateFunctionCallOutputContext(reqBody)
REDACTED
REDACTED

func BenchmarkToolContinuationValidationOptimized(b *testing.B) {
	reqBody := benchmarkToolContinuationRequestBody()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkToolContinuationBoolSink = optimizedValidateFunctionCallOutputContext(reqBody)
REDACTED
REDACTED

func BenchmarkWSIngressPayloadParseLegacy(b *testing.B) {
	raw := benchmarkWSIngressPayloadBytes()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eventType, model, promptCacheKey, previousResponseID, payload, err := legacyParseWSIngressPayload(raw)
		if err == nil {
			benchmarkWSParseStringSink = eventType + model + promptCacheKey + previousResponseID
			benchmarkWSParseMapSink = payload
	REDACTED
REDACTED
REDACTED

func BenchmarkWSIngressPayloadParseOptimized(b *testing.B) {
	raw := benchmarkWSIngressPayloadBytes()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eventType, model, promptCacheKey, previousResponseID, payload, err := optimizedParseWSIngressPayload(raw)
		if err == nil {
			benchmarkWSParseStringSink = eventType + model + promptCacheKey + previousResponseID
			benchmarkWSParseMapSink = payload
	REDACTED
REDACTED
REDACTED

func BenchmarkOpenAIUsageExtractLegacy(b *testing.B) {
	body := benchmarkOpenAIUsageJSONBytes()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		usage, ok := legacyExtractOpenAIUsageFromJSONBytes(body)
		if ok {
			benchmarkUsageSink = usage
	REDACTED
REDACTED
REDACTED

func BenchmarkOpenAIUsageExtractOptimized(b *testing.B) {
	body := benchmarkOpenAIUsageJSONBytes()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		usage, ok := extractOpenAIUsageFromJSONBytes(body)
		if ok {
			benchmarkUsageSink = usage
	REDACTED
REDACTED
REDACTED

func benchmarkToolContinuationRequestBody() map[string]any {
	input := make([]any, 0, 64)
	for i := 0; i < 24; i++ {
		input = append(input, map[string]any{
			"type": "text",
			"text": "benchmark text",
	REDACTED)
REDACTED
	for i := 0; i < 10; i++ {
		callID := "call_" + strconv.Itoa(i)
		input = append(input, map[string]any{
			"type":    "tool_call",
			"call_id": callID,
	REDACTED)
		input = append(input, map[string]any{
			"type":    "function_call_output",
			"call_id": callID,
	REDACTED)
		input = append(input, map[string]any{
			"type": "item_reference",
			"id":   callID,
	REDACTED)
REDACTED
	return map[string]any{
		"model": "gpt-5.3-codex",
		"input": input,
REDACTED
REDACTED

func benchmarkWSIngressPayloadBytes() []byte {
	return []byte(`{"type":"response.create","model":"gpt-5.3-codex","prompt_cache_key":"cache_bench","previous_response_id":"resp_prev_bench","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"REDACTED]REDACTED]REDACTED`)
REDACTED

func benchmarkOpenAIUsageJSONBytes() []byte {
	return []byte(`{"id":"resp_bench","object":"response","model":"gpt-5.3-codex","usage":{"input_tokens":3210,"output_tokens":987,"input_tokens_details":{"cached_tokens":456REDACTEDREDACTEDREDACTED`)
REDACTED

func legacyValidateFunctionCallOutputContext(reqBody map[string]any) bool {
	if !legacyHasFunctionCallOutput(reqBody) {
		return true
REDACTED
	previousResponseID, _ := reqBody["previous_response_id"].(string)
	if strings.TrimSpace(previousResponseID) != "" {
		return true
REDACTED
	if legacyHasToolCallContext(reqBody) {
		return true
REDACTED
	if legacyHasFunctionCallOutputMissingCallID(reqBody) {
		return false
REDACTED
	callIDs := legacyFunctionCallOutputCallIDs(reqBody)
	return legacyHasItemReferenceForCallIDs(reqBody, callIDs)
REDACTED

func optimizedValidateFunctionCallOutputContext(reqBody map[string]any) bool {
	validation := ValidateFunctionCallOutputContext(reqBody)
	if !validation.HasFunctionCallOutput {
		return true
REDACTED
	previousResponseID, _ := reqBody["previous_response_id"].(string)
	if strings.TrimSpace(previousResponseID) != "" {
		return true
REDACTED
	if validation.HasToolCallContext {
		return true
REDACTED
	if validation.HasFunctionCallOutputMissingCallID {
		return false
REDACTED
	return validation.HasItemReferenceForAllCallIDs
REDACTED

func legacyHasFunctionCallOutput(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
REDACTED
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
REDACTED
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType, _ := itemMap["type"].(string)
		if itemType == "function_call_output" {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func legacyHasToolCallContext(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
REDACTED
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
REDACTED
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType, _ := itemMap["type"].(string)
		if itemType != "tool_call" && itemType != "function_call" {
			continue
	REDACTED
		if callID, ok := itemMap["call_id"].(string); ok && strings.TrimSpace(callID) != "" {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func legacyFunctionCallOutputCallIDs(reqBody map[string]any) []string {
	if reqBody == nil {
		return nil
REDACTED
	input, ok := reqBody["input"].([]any)
	if !ok {
		return nil
REDACTED
	ids := make(map[string]struct{REDACTED)
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType, _ := itemMap["type"].(string)
		if itemType != "function_call_output" {
			continue
	REDACTED
		if callID, ok := itemMap["call_id"].(string); ok && strings.TrimSpace(callID) != "" {
			ids[callID] = struct{REDACTED{REDACTED
	REDACTED
REDACTED
	if len(ids) == 0 {
		return nil
REDACTED
	callIDs := make([]string, 0, len(ids))
	for id := range ids {
		callIDs = append(callIDs, id)
REDACTED
	return callIDs
REDACTED

func legacyHasFunctionCallOutputMissingCallID(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
REDACTED
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
REDACTED
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType, _ := itemMap["type"].(string)
		if itemType != "function_call_output" {
			continue
	REDACTED
		callID, _ := itemMap["call_id"].(string)
		if strings.TrimSpace(callID) == "" {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func legacyHasItemReferenceForCallIDs(reqBody map[string]any, callIDs []string) bool {
	if reqBody == nil || len(callIDs) == 0 {
		return false
REDACTED
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
REDACTED
	referenceIDs := make(map[string]struct{REDACTED)
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType, _ := itemMap["type"].(string)
		if itemType != "item_reference" {
			continue
	REDACTED
		idValue, _ := itemMap["id"].(string)
		idValue = strings.TrimSpace(idValue)
		if idValue == "" {
			continue
	REDACTED
		referenceIDs[idValue] = struct{REDACTED{REDACTED
REDACTED
	if len(referenceIDs) == 0 {
		return false
REDACTED
	for _, callID := range callIDs {
		if _, ok := referenceIDs[callID]; !ok {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func legacyParseWSIngressPayload(raw []byte) (eventType, model, promptCacheKey, previousResponseID string, payload map[string]any, err error) {
	values := gjson.GetManyBytes(raw, "type", "model", "prompt_cache_key", "previous_response_id")
	eventType = strings.TrimSpace(values[0].String())
	if eventType == "" {
		eventType = "response.create"
REDACTED
	model = strings.TrimSpace(values[1].String())
	promptCacheKey = strings.TrimSpace(values[2].String())
	previousResponseID = strings.TrimSpace(values[3].String())
	payload = make(map[string]any)
	if err = json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", "", nil, err
REDACTED
	if _, exists := payload["type"]; !exists {
		payload["type"] = "response.create"
REDACTED
	return eventType, model, promptCacheKey, previousResponseID, payload, nil
REDACTED

func optimizedParseWSIngressPayload(raw []byte) (eventType, model, promptCacheKey, previousResponseID string, payload map[string]any, err error) {
	payload = make(map[string]any)
	if err = json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", "", nil, err
REDACTED
	eventType = openAIWSPayloadString(payload, "type")
	if eventType == "" {
		eventType = "response.create"
		payload["type"] = eventType
REDACTED
	model = openAIWSPayloadString(payload, "model")
	promptCacheKey = openAIWSPayloadString(payload, "prompt_cache_key")
	previousResponseID = openAIWSPayloadString(payload, "previous_response_id")
	return eventType, model, promptCacheKey, previousResponseID, payload, nil
REDACTED

func legacyExtractOpenAIUsageFromJSONBytes(body []byte) (OpenAIUsage, bool) {
	var response struct {
		Usage struct {
			InputTokens       int `json:"input_tokens"`
			OutputTokens      int `json:"output_tokens"`
			InputTokenDetails struct {
				CachedTokens int `json:"cached_tokens"`
		REDACTED `json:"input_tokens_details"`
	REDACTED `json:"usage"`
REDACTED
	if err := json.Unmarshal(body, &response); err != nil {
		return OpenAIUsage{REDACTED, false
REDACTED
	return OpenAIUsage{
		InputTokens:          response.Usage.InputTokens,
		OutputTokens:         response.Usage.OutputTokens,
		CacheReadInputTokens: response.Usage.InputTokenDetails.CachedTokens,
REDACTED, true
REDACTED
