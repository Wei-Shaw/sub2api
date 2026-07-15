//go:build unit

package service

import "strings"

func codeBuddyChatCompletionsSSE(model, requestID, text string) string {
	return strings.Join([]string{
		`data: {"id":"` + requestID + `","object":"chat.completion.chunk","model":"` + model + `","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"` + requestID + `","object":"chat.completion.chunk","model":"` + model + `","choices":[{"index":0,"delta":{"content":"` + text + `"},"finish_reason":null}]}`,
		"",
		`data: {"id":"` + requestID + `","object":"chat.completion.chunk","model":"` + model + `","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"` + requestID + `","object":"chat.completion.chunk","model":"` + model + `","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
}
