package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/openai"

const (
	OpenAIPreviousResponseIDKindEmpty      = openai.PreviousResponseIDKindEmpty
	OpenAIPreviousResponseIDKindResponseID = openai.PreviousResponseIDKindResponseID
	OpenAIPreviousResponseIDKindMessageID  = openai.PreviousResponseIDKindMessageID
	OpenAIPreviousResponseIDKindUnknown    = openai.PreviousResponseIDKindUnknown
)

// ClassifyOpenAIPreviousResponseIDKind classifies previous_response_id to improve diagnostics.
func ClassifyOpenAIPreviousResponseIDKind(id string) string {
	return openai.ClassifyPreviousResponseIDKind(id)
}

func IsOpenAIPreviousResponseIDLikelyMessageID(id string) bool {
	return openai.IsPreviousResponseIDLikelyMessageID(id)
}
