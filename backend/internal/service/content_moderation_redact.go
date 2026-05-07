package service

import (
	"regexp"
	"strings"
)

var contentModerationSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>，。；、]+`),
	regexp.MustCompile(`(?i)\b((?:api[_-]?key|apikey|access[_-]?token|refresh[_-]?token|id[_-]?token|session[_-]?token|token|session|cookie|set[_-]?cookie|authorization|bearer|password|passwd|pwd|secret|client[_-]?secret|private[_-]?key)\s*[:=]\s*)(["']?)[^"'\s,;，。；、]{6,REDACTED`),
	regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]{12,REDACTED`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,REDACTED\.[A-Za-z0-9_-]{8,REDACTED\.[A-Za-z0-9_-]{8,REDACTED\b`),
	regexp.MustCompile(`(?i)\b(?:sk|sk-proj|sk-ant|sess|rk|pk|ak|api|key|token|secret)[_-][A-Za-z0-9._~+/=-]{12,REDACTED\b`),
	regexp.MustCompile(`\b[0-9a-fA-F]{32,REDACTED\b`),
	regexp.MustCompile(`\b[A-Za-z0-9_-]{48,REDACTED\b`),
	regexp.MustCompile(`\b[A-Za-z0-9+/]{48,REDACTED={0,2REDACTED\b`),
	regexp.MustCompile(`\b[0-9a-fA-F]{8REDACTED-[0-9a-fA-F]{4REDACTED-[0-9a-fA-F]{4REDACTED-[0-9a-fA-F]{4REDACTED-[0-9a-fA-F]{12REDACTED\b`),
REDACTED

func redactContentModerationSecrets(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
REDACTED
	out := text
	for idx, pattern := range contentModerationSecretPatterns {
		switch idx {
		case 1:
			out = pattern.ReplaceAllString(out, `${1REDACTED${2REDACTED[已脱敏]`)
		case 2:
			out = pattern.ReplaceAllString(out, `${1REDACTED[已脱敏]`)
		default:
			out = pattern.ReplaceAllString(out, `[已脱敏]`)
	REDACTED
REDACTED
	return out
REDACTED
