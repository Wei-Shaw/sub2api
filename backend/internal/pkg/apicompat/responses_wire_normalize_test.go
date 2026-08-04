package apicompat

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeResponsesStrictClientWireJSON_CompletedMessage(t *testing.T) {
	// Mirrors the Grok failure payload: message without id/status, output_text
	// without annotations/logprobs.
	in := []byte(`{
		"type":"response.completed",
		"sequence_number":38,
		"response":{
			"id":"resp_03f08335219c85ae016a71d2c1e0a48190867c3b3f1b4e6667",
			"object":"response",
			"status":"completed",
			"output":[{
				"type":"message",
				"role":"assistant",
				"content":[{"type":"output_text","text":"直接执行"}]
			}]
		}
	}`)

	out, changed := NormalizeResponsesStrictClientWireJSON(in)
	require.True(t, changed)
	require.Equal(t, "resp_03f08335219c85ae016a71d2c1e0a48190867c3b3f1b4e6667", gjson.GetBytes(out, "response.id").String())

	itemID := gjson.GetBytes(out, "response.output.0.id").String()
	require.True(t, strings.HasPrefix(itemID, "msg_"), "got %q", itemID)
	require.Equal(t, "completed", gjson.GetBytes(out, "response.output.0.status").String())
	require.Equal(t, "直接执行", gjson.GetBytes(out, "response.output.0.content.0.text").String())
	require.True(t, gjson.GetBytes(out, "response.output.0.content.0.annotations").IsArray())
	require.True(t, gjson.GetBytes(out, "response.output.0.content.0.logprobs").IsArray())
	require.Len(t, gjson.GetBytes(out, "response.output.0.content.0.annotations").Array(), 0)
	require.Len(t, gjson.GetBytes(out, "response.output.0.content.0.logprobs").Array(), 0)

	// Idempotent: already-normalized payload must not change.
	again, changedAgain := NormalizeResponsesStrictClientWireJSON(out)
	require.False(t, changedAgain)
	require.JSONEq(t, string(out), string(again))
}

func TestNormalizeResponsesStrictClientWireJSON_PreservesExistingFields(t *testing.T) {
	in := []byte(`{
		"type":"response.completed",
		"response":{
			"id":"resp_keep",
			"status":"completed",
			"output":[{
				"type":"message",
				"id":"msg_existing",
				"status":"completed",
				"role":"assistant",
				"content":[{
					"type":"output_text",
					"text":"hi",
					"annotations":[{"type":"url_citation","url":"https://example.com"}],
					"logprobs":[{"token":"hi"}]
				}]
			}]
		}
	}`)
	out, changed := NormalizeResponsesStrictClientWireJSON(in)
	require.False(t, changed)
	require.Equal(t, "msg_existing", gjson.GetBytes(out, "response.output.0.id").String())
	require.Equal(t, "https://example.com", gjson.GetBytes(out, "response.output.0.content.0.annotations.0.url").String())
	require.Equal(t, "hi", gjson.GetBytes(out, "response.output.0.content.0.logprobs.0.token").String())
}

func TestNormalizeResponsesStrictClientWireJSON_BareResponseBody(t *testing.T) {
	in := []byte(`{
		"id":"resp_bare",
		"object":"response",
		"status":"completed",
		"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]
	}`)
	out, changed := NormalizeResponsesStrictClientWireJSON(in)
	require.True(t, changed)
	require.True(t, strings.HasPrefix(gjson.GetBytes(out, "output.0.id").String(), "msg_"))
	require.Equal(t, "completed", gjson.GetBytes(out, "output.0.status").String())
	require.True(t, gjson.GetBytes(out, "output.0.content.0.annotations").IsArray())
}

func TestNormalizeResponsesStrictClientWireJSON_RepairsInvalidCollections(t *testing.T) {
	in := []byte(`{
		"type":"response.completed",
		"response":{"status":"completed","output":[{
			"type":"message",
			"id":"msg_invalid",
			"status":"completed",
			"content":[{
				"type":"output_text",
				"text":null,
				"annotations":null,
				"logprobs":{"invalid":true}
			}]
		}]}
	}`)

	out, changed := NormalizeResponsesStrictClientWireJSON(in)
	require.True(t, changed)
	require.Equal(t, "", gjson.GetBytes(out, "response.output.0.content.0.text").String())
	require.True(t, gjson.GetBytes(out, "response.output.0.content.0.annotations").IsArray())
	require.True(t, gjson.GetBytes(out, "response.output.0.content.0.logprobs").IsArray())

	again, changedAgain := NormalizeResponsesStrictClientWireJSON(out)
	require.False(t, changedAgain)
	require.Equal(t, string(out), string(again))
}

func TestNormalizeResponsesStrictClientWireJSON_RepairsNullMessageContent(t *testing.T) {
	in := []byte(`{
		"type":"response.completed",
		"response":{"status":"completed","output":[{
			"type":"message","id":"msg_null_content","status":"completed","content":null
		}]}
	}`)

	out, changed := NormalizeResponsesStrictClientWireJSON(in)
	require.True(t, changed)
	require.True(t, gjson.GetBytes(out, "response.output.0.content").IsArray())
	require.Empty(t, gjson.GetBytes(out, "response.output.0.content").Array())
}

func TestNormalizeResponsesStrictClientWireJSON_LeavesNonOutputTextPartUnchanged(t *testing.T) {
	in := []byte(`{
		"type":"response.content_part.added",
		"part":{"type":"refusal","refusal":"no","vendor_extension":{"keep":true}}
	}`)

	out, changed := NormalizeResponsesStrictClientWireJSON(in)
	require.False(t, changed)
	require.Equal(t, string(in), string(out))
}

func TestResponsesStrictClientWireNormalizer_ReusesMessageIDAcrossEvents(t *testing.T) {
	normalizer := NewResponsesStrictClientWireNormalizer()

	added, changed := normalizer.Normalize([]byte(`{
		"type":"response.output_item.added",
		"output_index":0,
		"item":{"type":"message","role":"assistant"}
	}`))
	require.True(t, changed)
	addedID := gjson.GetBytes(added, "item.id").String()
	require.True(t, strings.HasPrefix(addedID, "msg_"))

	done, changed := normalizer.Normalize([]byte(`{
		"type":"response.output_item.done",
		"output_index":0,
		"item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}
	}`))
	require.True(t, changed)

	completed, changed := normalizer.Normalize([]byte(`{
		"type":"response.completed",
		"response":{"status":"completed","output":[{
			"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]
		}]}
	}`))
	require.True(t, changed)

	require.Equal(t, addedID, gjson.GetBytes(done, "item.id").String())
	require.Equal(t, addedID, gjson.GetBytes(completed, "response.output.0.id").String())
	require.Equal(t, "in_progress", gjson.GetBytes(added, "item.status").String())
	require.Equal(t, "completed", gjson.GetBytes(done, "item.status").String())
	require.Equal(t, "completed", gjson.GetBytes(completed, "response.output.0.status").String())
}
