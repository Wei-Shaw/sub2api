package service

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// withChatCompletionsMetadataUserID carries the client's top-level
// metadata.user_id of a Chat Completions request into the Anthropic body
// converted from it. ChatCompletionsRequest and ResponsesRequest have no
// metadata field, so the conversion never sets AnthropicRequest.Metadata and
// without this the Anthropic request never sees the id the client sent.
//
// Only a string user_id that ParseMetadataUserID accepts (JSON form with
// non-empty device_id and session_id, or the legacy form) is carried, as the
// raw JSON string token of the original body, so its bytes do not change.
// Every other metadata key is dropped. In every other case anthropicBody is
// returned as it is.
func withChatCompletionsMetadataUserID(chatBody, anthropicBody []byte) []byte {
	userID := gjson.GetBytes(chatBody, "metadata.user_id")
	if userID.Type != gjson.String {
		return anthropicBody
	}
	if parsed := ParseMetadataUserID(userID.String()); parsed == nil || parsed.SessionID == "" {
		return anthropicBody
	}
	out, err := sjson.SetRawBytes(anthropicBody, "metadata", []byte(`{"user_id":`+userID.Raw+`}`))
	if err != nil {
		return anthropicBody
	}
	return out
}
