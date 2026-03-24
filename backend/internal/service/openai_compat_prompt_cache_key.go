package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

const compatPromptCacheKeyPrefix = "compat_cc_"

func shouldAutoInjectPromptCacheKeyForCompat(model string) bool {
	switch resolveOpenAIUpstreamModel(strings.TrimSpace(model)) {
	case "gpt-5.4", "gpt-5.3-codex", "gpt-5.3-codex-spark":
		return true
	default:
		return false
REDACTED
REDACTED

func deriveCompatPromptCacheKey(req *apicompat.ChatCompletionsRequest, mappedModel string) string {
	if req == nil {
		return ""
REDACTED

	normalizedModel := resolveOpenAIUpstreamModel(strings.TrimSpace(mappedModel))
	if normalizedModel == "" {
		normalizedModel = resolveOpenAIUpstreamModel(strings.TrimSpace(req.Model))
REDACTED
	if normalizedModel == "" {
		normalizedModel = strings.TrimSpace(req.Model)
REDACTED

	seedParts := []string{"model=" + normalizedModelREDACTED
	if req.ReasoningEffort != "" {
		seedParts = append(seedParts, "reasoning_effort="+strings.TrimSpace(req.ReasoningEffort))
REDACTED
	if len(req.ToolChoice) > 0 {
		seedParts = append(seedParts, "tool_choice="+normalizeCompatSeedJSON(req.ToolChoice))
REDACTED
	if len(req.Tools) > 0 {
		if raw, err := json.Marshal(req.Tools); err == nil {
			seedParts = append(seedParts, "tools="+normalizeCompatSeedJSON(raw))
	REDACTED
REDACTED
	if len(req.Functions) > 0 {
		if raw, err := json.Marshal(req.Functions); err == nil {
			seedParts = append(seedParts, "functions="+normalizeCompatSeedJSON(raw))
	REDACTED
REDACTED

	firstUserCaptured := false
	for _, msg := range req.Messages {
		switch strings.TrimSpace(msg.Role) {
		case "system":
			seedParts = append(seedParts, "system="+normalizeCompatSeedJSON(msg.Content))
		case "user":
			if !firstUserCaptured {
				seedParts = append(seedParts, "first_user="+normalizeCompatSeedJSON(msg.Content))
				firstUserCaptured = true
		REDACTED
	REDACTED
REDACTED

	return compatPromptCacheKeyPrefix + hashSensitiveValueForLog(strings.Join(seedParts, "|"))
REDACTED

func normalizeCompatSeedJSON(v json.RawMessage) string {
	if len(v) == 0 {
		return ""
REDACTED
	var tmp any
	if err := json.Unmarshal(v, &tmp); err != nil {
		return string(v)
REDACTED
	out, err := json.Marshal(tmp)
	if err != nil {
		return string(v)
REDACTED
	return string(out)
REDACTED
