package openai

import (
	"regexp"
	"strings"
)

const (
	// PreviousResponseIDKindEmpty indicates previous_response_id was blank.
	PreviousResponseIDKindEmpty = "empty"
	// PreviousResponseIDKindResponseID indicates a canonical resp_... identifier.
	PreviousResponseIDKindResponseID = "response_id"
	// PreviousResponseIDKindMessageID indicates a message/item/chatcmpl style id.
	PreviousResponseIDKindMessageID = "message_id"
	// PreviousResponseIDKindUnknown indicates an unrecognized identifier shape.
	PreviousResponseIDKindUnknown = "unknown"
)

var (
	responseIDPattern = regexp.MustCompile(`^resp_[A-Za-z0-9_-]{1,256}$`)
	messageIDPattern  = regexp.MustCompile(`^(msg|message|item|chatcmpl)_[A-Za-z0-9_-]{1,256}$`)
)

// ClassifyPreviousResponseIDKind classifies previous_response_id to improve diagnostics.
func ClassifyPreviousResponseIDKind(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return PreviousResponseIDKindEmpty
	}
	if responseIDPattern.MatchString(trimmed) {
		return PreviousResponseIDKindResponseID
	}
	if messageIDPattern.MatchString(strings.ToLower(trimmed)) {
		return PreviousResponseIDKindMessageID
	}
	return PreviousResponseIDKindUnknown
}

// IsPreviousResponseIDLikelyMessageID reports whether id looks like a message/item id
// rather than a Responses response_id.
func IsPreviousResponseIDLikelyMessageID(id string) bool {
	return ClassifyPreviousResponseIDKind(id) == PreviousResponseIDKindMessageID
}
