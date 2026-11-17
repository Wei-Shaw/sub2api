package apicompat

import "strings"

// IsServerSideSearchCall reports whether a Responses output item is a search
// that the xAI server runs itself, so the client has nothing to execute.
//
// xAI documents the Responses API form of X Search as a custom_tool_call whose
// call_id starts with xs_, and attachment search as one whose call_id starts
// with as_; both are "handled by the xAI server"
// (https://docs.x.ai/developers/tools/tool-usage-details, "Using Responses API"
// table; https://docs.x.ai/developers/tools/x-search, "How X Search Works").
// A custom_tool_call without these prefixes is a client tool call.
//
// This is the only place the prefix rule lives. Converters that turn
// custom_tool_call items into client tool calls skip items for which it
// returns true when the caller says the upstream is xAI, and Grok search
// counting includes them.
func IsServerSideSearchCall(itemType, callID string) bool {
	if itemType != "custom_tool_call" {
		return false
	}
	return strings.HasPrefix(callID, "xs_") || strings.HasPrefix(callID, "as_")
}
