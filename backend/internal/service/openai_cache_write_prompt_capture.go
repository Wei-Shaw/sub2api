package service

import (
	"io"
	"net/http"
)

// Capture the final request body, after request builders and compatibility
// rewrites have run. GetBody is independent of the reader sent upstream. Only
// hashes escape this function; unreadable or unbounded bodies fail closed.
func captureOpenAICacheWritePromptEvidence(req *http.Request) openAICacheWritePromptEvidence {
	if req == nil || req.GetBody == nil || req.ContentLength > openAICacheWriteEvidenceMaxBodyBytes {
		return openAICacheWritePromptEvidence{}
	}
	body, err := req.GetBody()
	if err != nil || body == nil {
		return openAICacheWritePromptEvidence{}
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, openAICacheWriteEvidenceMaxBodyBytes+1))
	if err != nil || len(raw) > openAICacheWriteEvidenceMaxBodyBytes {
		return openAICacheWritePromptEvidence{}
	}
	return buildOpenAICacheWritePromptEvidence(raw)
}
