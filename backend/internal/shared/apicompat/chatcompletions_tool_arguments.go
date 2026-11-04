package apicompat

import "strings"

func appendChatToolArguments(state *ChatCompletionsToResponsesStreamState, index int, call *ChatToolCall, fragment string) {
	if call.Function.Arguments == "" {
		call.Function.Arguments = fragment
		return
	}
	if state.toolArgumentBuffers == nil {
		state.toolArgumentBuffers = make(map[int]*strings.Builder)
	}
	buffer := state.toolArgumentBuffers[index]
	if buffer == nil {
		buffer = &strings.Builder{}
		state.toolArgumentBuffers[index] = buffer
	}
	call.Function.Arguments = appendStreamFragment(buffer, call.Function.Arguments, fragment)
}
