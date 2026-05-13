package service

import (
	"regexp"
	"strings"
)

const (
	OpenAIPreviousResponseIDKindEmpty      = "empty"
	OpenAIPreviousResponseIDKindResponseID = "response_id"
	OpenAIPreviousResponseIDKindMessageID  = "message_id"
	OpenAIPreviousResponseIDKindUnknown    = "unknown"
)

var (
	openAIResponseIDPattern = regexp.MustCompile(`^resp_[A-Za-z0-9_-]{1,256}$`)
	openAIMessageIDPattern  = regexp.MustCompile(`^(msg|message|item|chatcmpl)_[A-Za-z0-9_-]{1,256}$`)
)

// ClassifyOpenAIPreviousResponseIDKind classifies previous_response_id to improve diagnostics.
func ClassifyOpenAIPreviousResponseIDKind(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return OpenAIPreviousResponseIDKindEmpty
	}
	if openAIResponseIDPattern.MatchString(trimmed) {
		return OpenAIPreviousResponseIDKindResponseID
	}
	if openAIMessageIDPattern.MatchString(strings.ToLower(trimmed)) {
		return OpenAIPreviousResponseIDKindMessageID
	}
	return OpenAIPreviousResponseIDKindUnknown
}
