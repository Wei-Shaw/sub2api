package service

import "strings"

func lastOpenAIModelSegment(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
REDACTED
	if strings.Contains(model, "/") {
		parts := strings.Split(model, "/")
		model = parts[len(parts)-1]
REDACTED
	return strings.TrimSpace(model)
REDACTED

func canonicalizeOpenAIModelAliasSpelling(model string) string {
	model = strings.ToLower(lastOpenAIModelSegment(model))
	if model == "" {
		return ""
REDACTED

	normalized := strings.ReplaceAll(model, "_", "-")
	normalized = strings.Join(strings.Fields(normalized), "-")
	for strings.Contains(normalized, "--") {
		normalized = strings.ReplaceAll(normalized, "--", "-")
REDACTED

	if strings.HasPrefix(normalized, "gpt5") {
		normalized = "gpt-5" + strings.TrimPrefix(normalized, "gpt5")
REDACTED
	if !strings.HasPrefix(normalized, "gpt-") && !strings.Contains(normalized, "codex") {
		return ""
REDACTED

	replacements := []struct {
		from string
		to   string
REDACTED{
		{"gpt-5.4mini", "gpt-5.4-mini"REDACTED,
		{"gpt-5.4nano", "gpt-5.4-nano"REDACTED,
		{"gpt-5.3-codexspark", "gpt-5.3-codex-spark"REDACTED,
		{"gpt-5.3codexspark", "gpt-5.3-codex-spark"REDACTED,
		{"gpt-5.3codex", "gpt-5.3-codex"REDACTED,
REDACTED
	for _, replacement := range replacements {
		normalized = strings.ReplaceAll(normalized, replacement.from, replacement.to)
REDACTED
	return normalized
REDACTED

func normalizeKnownOpenAICodexModel(model string) string {
	normalized := canonicalizeOpenAIModelAliasSpelling(model)
	if normalized == "" {
		return ""
REDACTED

	if mapped := getNormalizedCodexModel(normalized); mapped != "" {
		return mapped
REDACTED
	if strings.HasSuffix(normalized, "-openai-compact") {
		if mapped := getNormalizedCodexModel(strings.TrimSuffix(normalized, "-openai-compact")); mapped != "" {
			return mapped
	REDACTED
REDACTED

	switch {
	case strings.Contains(normalized, "gpt-5.6-sol"):
		return "gpt-5.6-sol"
	case strings.Contains(normalized, "gpt-5.6-terra"):
		return "gpt-5.6-terra"
	case strings.Contains(normalized, "gpt-5.6-luna"):
		return "gpt-5.6-luna"
	case strings.Contains(normalized, "gpt-5.5-pro"):
		return "gpt-5.5-pro"
	case strings.Contains(normalized, "gpt-5.5"):
		return "gpt-5.5"
	case strings.Contains(normalized, "gpt-5.4-mini"):
		return "gpt-5.4-mini"
	case strings.Contains(normalized, "gpt-5.4-nano"):
		return "gpt-5.4-nano"
	case strings.Contains(normalized, "gpt-5.4"):
		return "gpt-5.4"
	case strings.Contains(normalized, "gpt-5.2"):
		return "gpt-5.2"
	case strings.Contains(normalized, "gpt-5.3-codex-spark"):
		return "gpt-5.3-codex-spark"
	case strings.Contains(normalized, "gpt-5.3-codex"):
		return "gpt-5.3-codex"
	case strings.Contains(normalized, "gpt-5.3"):
		return "gpt-5.3-codex"
	case strings.Contains(normalized, "codex"):
		return "gpt-5.3-codex"
	case strings.Contains(normalized, "gpt-5"):
		return "gpt-5.4"
	default:
		return ""
REDACTED
REDACTED

func appendUsageBillingModelCandidate(candidates []string, seen map[string]struct{REDACTED, model string) []string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return candidates
REDACTED
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
	REDACTED
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			return
	REDACTED
		seen[key] = struct{REDACTED{REDACTED
		candidates = append(candidates, candidate)
REDACTED

	add(trimmed)
	if canonical := canonicalizeOpenAIModelAliasSpelling(trimmed); canonical != "" {
		add(canonical)
REDACTED
	if normalized := normalizeKnownOpenAICodexModel(trimmed); normalized != "" {
		add(normalized)
REDACTED
	return candidates
REDACTED

func usageBillingModelCandidates(primary string, alternates ...string) []string {
	seen := make(map[string]struct{REDACTED, 1+len(alternates))
	candidates := appendUsageBillingModelCandidate(nil, seen, primary)
	for _, alternate := range alternates {
		candidates = appendUsageBillingModelCandidate(candidates, seen, alternate)
REDACTED
	return candidates
REDACTED

func firstUsageBillingModel(candidates []string) string {
	for _, candidate := range candidates {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
	REDACTED
REDACTED
	return ""
REDACTED
