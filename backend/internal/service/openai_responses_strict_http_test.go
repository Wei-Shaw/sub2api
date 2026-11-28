package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestStrictHTTPResponsesInvalidCallIDs(t *testing.T) {
	for _, id := range []string{``, `,"call_id":null`, `,"call_id":42`, `,"call_id":""`, `,"call_id":" \t\u2003"`} {
		for _, previous := range []string{``, `,"previous_response_id":"resp_history"`} {
			t.Run(id+previous, func(t *testing.T) {
				body := []byte(fmt.Sprintf(`{"unknown":900719925474099312345%s,"input":[{"type":"message","role":"user","content":"before"},{"type":"function_call_output","name":"delegate","namespace":"tasks","output":"\n exact task \t\n"%s},{"type":"function_call_output","output":"unnamed"%s},{"type":"message","role":"user","content":"after"}]}`, previous, id, id))
				next, changed, err := sanitizeStrictHTTPResponsesBody(body)
				require.NoError(t, err)
				require.True(t, changed)
				require.Equal(t, "900719925474099312345", gjson.GetBytes(next, "unknown").Raw)
				require.Len(t, gjson.GetBytes(next, "input").Array(), 3)
				require.Equal(t, "before", gjson.GetBytes(next, "input.0.content").String())
				require.Equal(t, "user", gjson.GetBytes(next, "input.1.role").String())
				require.Contains(t, gjson.GetBytes(next, "input.1.content.0.text").String(), "Recovered tool/delegation context")
				require.Equal(t, "\n exact task \t\n", gjson.GetBytes(next, "input.1.content.1.text").String())
				require.Equal(t, "after", gjson.GetBytes(next, "input.2.content").String())
				again, changed, err := sanitizeStrictHTTPResponsesBody(next)
				require.NoError(t, err)
				require.False(t, changed)
				require.Equal(t, next, again)
			})
		}
	}
}

func TestStrictHTTPResponsesRawPreservation(t *testing.T) {
	fixtures := []string{
		` { "unknown":900719925474099312345, "input":[{"type":"function_call","call_id":"c","name":"tool","arguments":"{}"},{"type":"function_call_output","call_id":"c","output":"ok"}] } `,
		`{"previous_response_id":"resp_history","input":[{"type":"function_call_output","call_id":"server_call","output":"ok"}]}`,
		`{"input":[{"type":"item_reference","id":"c"},{"type":"function_call_output","call_id":"c","output":"ok"}]}`,
	}
	for _, fixture := range fixtures {
		next, changed, err := sanitizeStrictHTTPResponsesBody([]byte(fixture))
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, fixture, string(next))
	}
	reasoning := `{"type":"reasoning", "id":"rs_1", "summary":[{"type":"summary_text","text":" exact "}], "encrypted_content":"cipher", "unknown":900719925474099312345}`
	compaction := `{"type":"compaction", "id":"cmp_1", "encrypted_content":"compact-cipher"}`
	body := []byte(`{"input":[` + reasoning + `,` + compaction + `,{"type":"function_call_output","output":"unreachable"}]}`)
	next, changed, err := sanitizeStrictHTTPResponsesBody(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, reasoning, gjson.GetBytes(next, "input.0").Raw)
	require.Equal(t, compaction, gjson.GetBytes(next, "input.1").Raw)
}

func TestStrictHTTPResponsesStructuredRecovery(t *testing.T) {
	output := `[{"type":"input_image","image_url":"data:image/png;base64,AA=="},{"type":"input_text","text":" exact "},{"unknown":900719925474099312345}]`
	body := []byte(`{"input":[{"type":"function_call_output","name":"task","output":` + output + `}]}`)
	next, changed, err := sanitizeStrictHTTPResponsesBody(body)
	require.NoError(t, err)
	require.True(t, changed)
	recovered := gjson.GetBytes(next, "input.0.content.1.text").String()
	var original, got any
	require.NoError(t, decodeOpenAIJSONUseNumber([]byte(output), &original))
	require.NoError(t, decodeOpenAIJSONUseNumber([]byte(recovered), &got))
	require.Equal(t, original, got)
	require.True(t, json.Valid(next))
}

func TestStrictHTTPResponsesPrecheckPolicy(t *testing.T) {
	for _, fixture := range []string{
		`{"previous_response_id":"resp","input":[{"type":"function_call_output","name":"task","output":"task"}]}`,
		`{"previous_response_id":"resp","input":[],"input":[{"type":"function_call_output","output":"drop"}]}`,
		`{"previous_response_id":"resp","in\u0070ut":[{"type":"function_call_output","output":"drop"}]}`,
	} {
		require.True(t, openAIResponsesInputMayNeedOrphanCleanup([]byte(fixture), true))
		next, changed, err := sanitizeStrictHTTPResponsesBody([]byte(fixture))
		require.NoError(t, err)
		require.True(t, changed)
		require.NotEqual(t, fixture, string(next))
	}
}
