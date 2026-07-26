package service

import (
	"bufio"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

func splitOpenAIConcatenatedJSONDocuments(payload []byte) ([][]byte, bool) {
	return openai.SplitConcatenatedJSONDocuments(payload)
}

// openAISSEJSONDocumentScanner is a service-local alias of the pkg/openai helper.
// Kept unexported so existing call sites compile without import churn.
type openAISSEJSONDocumentScanner = openai.SSEJSONDocumentScanner

func newOpenAISSEJSONDocumentScanner(scanner *bufio.Scanner) *openAISSEJSONDocumentScanner {
	return openai.NewSSEJSONDocumentScanner(scanner)
}
