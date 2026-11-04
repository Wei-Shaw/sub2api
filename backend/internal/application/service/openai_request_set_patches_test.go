package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func serialOpenAIRequestPatches(body []byte, patches []openAIRequestPatch) ([]byte, error) {
	for _, patch := range patches {
		var err error
		if patch.delete {
			body, err = sjson.DeleteBytes(body, patch.path)
		} else {
			body, err = sjson.SetBytes(body, patch.path, patch.value)
		}
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func TestOpenAIRequestSetPatchesMatchSequentialSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		patches      []openAIRequestPatch
		batched      bool
	}{
		{"insert roots", `"model":"gpt-5"`, []openAIRequestPatch{{path: "instructions", value: "help"}, {path: "reasoning.effort", value: "none"}}, true},
		{"replace roots in reverse order", `"model":"gpt-5","reasoning":{"effort":"high","extension":9007199254740993}`, []openAIRequestPatch{{path: "reasoning.effort", value: "low"}, {path: "model", value: "gpt-5.1"}}, true},
		{"same root and parent replacement", `"reasoning":null`, []openAIRequestPatch{{path: "reasoning.effort", value: "low"}, {path: "reasoning", value: map[string]any{"summary": "auto"}}, {path: "reasoning.effort", value: "high"}}, true},
		{"escaped source key", `"m\u006fdel":"gpt-5"`, []openAIRequestPatch{{path: "model", value: "gpt-5.1"}, {path: "store", value: false}}, true},
		{"unicode and raw numbers", `"model":"gpt-5"`, []openAIRequestPatch{{path: "instructions", value: "你好🙂\n<>&\u2028"}, {path: "metadata", value: json.RawMessage(`{"nonce":9007199254740993}`)}}, true},
		{"no changes", `"model":"gpt-5","stream":true`, []openAIRequestPatch{{path: "model", value: "gpt-5"}, {path: "stream", value: true}}, true},
		{"duplicate source root fallback", `"model":"first","model":"second"`, []openAIRequestPatch{{path: "model", value: "third"}, {path: "stream", value: true}}, false},
		{"delete fallback", `"model":"gpt-5","previous_response_id":"resp_1"`, []openAIRequestPatch{{path: "model", value: "gpt-5.1"}, {path: "previous_response_id", delete: true}}, false},
		{"array path fallback", `"tools":[{"name":"old"}]`, []openAIRequestPatch{{path: "tools.0.name", value: "new"}, {path: "stream", value: false}}, false},
		{"large subtree fallback", `"model":"gpt-5"`, []openAIRequestPatch{{path: "input", value: []any{}}, {path: "stream", value: true}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(" {" + tc.fields + `,"input":[{"nonce":9007199254740993,"content":"` + strings.Repeat("payload ", 10000) + `"}]} ` + "\n")
			original := bytes.Clone(body)
			want, err := serialOpenAIRequestPatches(body, tc.patches)
			require.NoError(t, err)
			_, batched, err := applyOpenAIRequestSetPatches(body, tc.patches)
			require.NoError(t, err)
			require.Equal(t, tc.batched, batched)
			view := newOpenAIRequestView(body)
			view.patches = tc.patches
			got, err := view.ApplyPatches()
			require.NoError(t, err)
			require.True(t, json.Valid(got))
			require.JSONEq(t, string(want), string(got))
			require.Equal(t, original, body, "patches must not mutate a body shared with failover attempts")
			if tc.name != "large subtree fallback" {
				require.Equal(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(got, "input").Raw, "untouched input must stay byte-exact")
			}
		})
	}
}

func TestOpenAIRequestSetPatchesEmptyObjectAndEncodingError(t *testing.T) {
	body := []byte("{" + strings.Repeat(" ", openAIRequestPatchBatchMinBytes) + "}\n")
	patches := []openAIRequestPatch{{path: "model", value: "gpt-5"}, {path: "reasoning.effort", value: "none"}}
	got, applied, err := applyOpenAIRequestSetPatches(body, patches)
	require.NoError(t, err)
	require.True(t, applied)
	require.JSONEq(t, `{"model":"gpt-5","reasoning":{"effort":"none"}}`, string(got))
	patches[1].value = make(chan int)
	got, applied, err = applyOpenAIRequestSetPatches(body, patches)
	require.Error(t, err)
	require.True(t, applied)
	require.Nil(t, got)
	require.NotContains(t, string(body), "model")
}

func FuzzOpenAIRequestSetPatches(f *testing.F) {
	for _, body := range []string{`{}`, `{"model":"gpt-5","reasoning":{"effort":"high"}}`, `{"model":"first","model":"second"}`, `{"reasoning":null}`, `{"reasoning":"text"}`, `{"reasoning":{"effort":"a","effort":"b"}}`} {
		f.Add(body, "reasoning.effort", "model")
	}
	f.Fuzz(func(t *testing.T, raw, first, second string) {
		if len(raw) > 4096 || len(first) > 80 || len(second) > 80 || !json.Valid([]byte(raw)) ||
			!isOpenAIRequestLiteralPatchPath(first) || !isOpenAIRequestLiteralPatchPath(second) {
			return
		}
		trimmed := bytes.TrimSpace([]byte(raw))
		if len(trimmed) < 2 || trimmed[0] != '{' {
			return
		}
		body := bytes.Clone(trimmed[:len(trimmed)-1])
		if len(bytes.TrimSpace(body[1:])) > 0 {
			body = append(body, ',')
		}
		body = append(body, `"_padding":"`...)
		body = append(body, bytes.Repeat([]byte{'x'}, openAIRequestPatchBatchMinBytes)...)
		body = append(body, '"', '}')
		patches := []openAIRequestPatch{{path: first, value: "你好<>&\n"}, {path: second, value: int64(9007199254740993)}}
		want, wantErr := serialOpenAIRequestPatches(body, patches)
		view := newOpenAIRequestView(body)
		view.patches = patches
		got, err := view.ApplyPatches()
		if wantErr != nil {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		require.JSONEq(t, string(want), string(got))
	})
}
