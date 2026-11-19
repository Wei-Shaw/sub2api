//go:build unit

package service

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// jsonUEscape spells the six-byte JSON escape backslash, 'u', hex at run time.
func jsonUEscape(hex string) string { return `\` + "u" + hex }

// referenceNormalizeCompactionTriggerInputOrder always takes the decode path.
func referenceNormalizeCompactionTriggerInputOrder(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}
	var payload map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &payload); err != nil {
		return body, false, err
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) == 0 {
		return body, false, nil
	}
	triggerCount := 0
	normalized := make([]any, 0, len(input))
	for _, raw := range input {
		item, itemOK := raw.(map[string]any)
		if itemOK && item["type"] == "compaction_trigger" {
			triggerCount++
			continue
		}
		normalized = append(normalized, raw)
	}
	if triggerCount == 0 {
		return body, false, nil
	}
	if triggerCount == 1 {
		if last, ok := input[len(input)-1].(map[string]any); ok && last["type"] == "compaction_trigger" {
			return body, false, nil
		}
	}
	normalized = append(normalized, map[string]any{"type": "compaction_trigger"})
	payload["input"] = normalized
	encoded, err := marshalOpenAIUpstreamJSON(payload)
	if err != nil {
		return body, false, err
	}
	return encoded, true, nil
}

func requireSameCompactionNormalization(t *testing.T, body []byte) {
	t.Helper()
	got, changed, err := NormalizeCompactionTriggerInputOrder(body)
	want, wantChanged, wantErr := referenceNormalizeCompactionTriggerInputOrder(body)
	require.Equal(t, wantErr != nil, err != nil, "error mismatch for %s", body)
	require.Equal(t, wantChanged, changed, "changed mismatch for %s", body)
	require.Equal(t, string(want), string(got), "body mismatch for %s", body)
}

func TestCompactionTriggerOrderFastPathMatchesDecode(t *testing.T) {
	trigger := `{"type":"compaction_trigger"}`
	msg := `{"type":"message","role":"user","content":"hi"}`
	cases := map[string]string{
		"trigger last":                   `{"model":"m","input":[` + msg + `,` + trigger + `]}`,
		"trigger only":                   `{"input":[` + trigger + `]}`,
		"trigger first":                  `{"input":[` + trigger + `,` + msg + `]}`,
		"two triggers":                   `{"input":[` + trigger + `,` + msg + `,` + trigger + `]}`,
		"token in text, trigger last":    `{"input":[{"type":"message","content":"compaction_trigger"},` + trigger + `]}`,
		"token only in text":             `{"input":[{"type":"message","content":"compaction_trigger"}]}`,
		"duplicate input key":            `{"input":[` + msg + `,` + trigger + `],"input":[` + msg + `]}`,
		"duplicate input key reversed":   `{"input":[` + msg + `],"input":[` + msg + `,` + trigger + `]}`,
		"duplicate type shadows trigger": `{"input":[` + msg + `,{"type":"compaction_trigger","type":"message"}]}`,
		"duplicate type reveals trigger": `{"input":[{"type":"message","type":"compaction_trigger"},` + msg + `]}`,
		"escaped trigger last":           `{"input":[` + msg + `,{"type":"` + jsonUEscape("0063") + `ompaction_trigger"}]}`,
		"escaped key trigger first":      `{"input":[{"` + jsonUEscape("0074") + `ype":"compaction_trigger"},` + msg + `]}`,
		"non-ascii escape, trigger last": `{"input":[{"type":"message","content":"caf` + jsonUEscape("00e9") + `"},` + trigger + `]}`,
		"trigger value not string":       `{"input":[` + msg + `,{"type":["compaction_trigger"]}]}`,
		"last item not object":           `{"input":[` + trigger + `,"compaction"]}`,
		"input not array":                `{"input":{"type":"compaction_trigger"}}`,
		"trailing garbage":               `{"input":[` + trigger + `]} x`,
		"truncated":                      `{"input":[` + trigger,
		"top-level array":                `[{"input":[` + trigger + `]}]`,
		"leading whitespace":             " \n" + `{"input":[` + msg + `,` + trigger + `]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			requireSameCompactionNormalization(t, []byte(body))
		})
	}
}

func TestCompactionTriggerOrderFastPathMatchesDecodeRandomized(t *testing.T) {
	rng := rand.New(rand.NewSource(20261114))
	items := []string{
		`{"type":"compaction_trigger"}`,
		`{"type":"message","role":"user","content":"hello"}`,
		`{"type":"message","content":"mentions compaction_trigger"}`,
		`{"type":"compaction_trigger","type":"message"}`,
		`{"type":"message","type":"compaction_trigger"}`,
		`{"type":"` + jsonUEscape("0063") + `ompaction_trigger"}`,
		`{"type":"reasoning","encrypted_content":"` + jsonUEscape("001b") + `x"}`,
		`"compaction_trigger"`,
		`{"type":"function_call_output","output":"<a ` + jsonUEscape("0026") + ` b>"}`,
	}
	for i := 0; i < 20000; i++ {
		var parts []string
		for n := rng.Intn(5); n >= 0; n-- {
			parts = append(parts, items[rng.Intn(len(items))])
		}
		body := `{"model":"m","input":[` + strings.Join(parts, ",") + `]`
		if rng.Intn(5) == 0 {
			body += `,"input":[` + items[rng.Intn(len(items))] + `]`
		}
		body += `}`
		if rng.Intn(20) == 0 {
			body = body[:rng.Intn(len(body))]
		}
		requireSameCompactionNormalization(t, []byte(body))
	}
}

func TestHasCompactionTriggerInInputLiteralGate(t *testing.T) {
	require.True(t, HasCompactionTriggerInInput([]byte(`{"input":[{"type":"compaction_trigger"}]}`)))
	require.True(t, HasCompactionTriggerInInput([]byte(`{"input":[{"type":"`+jsonUEscape("0063")+`ompaction_trigger"}]}`)))
	require.False(t, HasCompactionTriggerInInput([]byte(`{"input":[{"type":"message","content":"caf`+jsonUEscape("00e9")+`"}]}`)))
	require.False(t, HasCompactionTriggerInInput([]byte(`{"input":[{"type":"message","content":"compaction_trigger"}]}`)))
}

func TestResponsesInputHasFunctionCallOutputNamed(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"declared tool only", `{"tools":[{"type":"function","name":"automation_update"}],"input":[{"type":"function_call_output","call_id":"c","name":"other","output":"ok"}]}`, false},
		{"matching output", `{"input":[{"type":"function_call_output","name":"automation_update","output":"x"}]}`, true},
		{"prefix match", `{"input":[{"type":"function_call_output","name":"codex_apps_selected_page_part_1_of_2"}]}`, true},
		{"escaped key", `{"input":[{"` + jsonUEscape("0074") + `ype":"function_call_output","name":"automation_update"}]}`, true},
		{"escaped value", `{"input":[{"type":"function_call_output","name":"` + jsonUEscape("0061") + `utomation_update"}]}`, true},
		{"type not output", `{"input":[{"type":"function_call","name":"automation_update"}]}`, false},
		{"non-object item", `{"input":["function_call_output automation_update"]}`, false},
		{"input not array", `{"input":{"type":"function_call_output","name":"automation_update"}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ResponsesInputHasFunctionCallOutputNamed([]byte(tc.body), "automation_update", "codex_apps_selected_page_part_"))
		})
	}
}

// referenceOpenAIRequestBodyHasTools is the scan without the literal gate.
func referenceOpenAIRequestBodyHasTools(body []byte) bool {
	if tools := gjson.GetBytes(body, "tools"); tools.IsArray() && len(tools.Array()) > 0 {
		return true
	}
	for _, item := range gjson.GetBytes(body, "input").Array() {
		if strings.TrimSpace(item.Get("type").String()) != "additional_tools" {
			continue
		}
		if tools := item.Get("tools"); tools.IsArray() && len(tools.Array()) > 0 {
			return true
		}
	}
	return false
}

func TestOpenAIRequestBodyHasToolsMatchesScan(t *testing.T) {
	bodies := []string{
		`{"tools":[{"type":"function","name":"f"}],"input":[]}`,
		`{"tools":[],"input":[]}`,
		`{"tools":[null]}`,
		`{"tools":{},"input":"hi"}`,
		`{"input":[{"type":"additional_tools","tools":[{"type":"namespace"}]}]}`,
		`{"input":[{"type":" additional_tools ","tools":[{"type":"namespace"}]}]}`,
		`{"input":[{"type":"additional_tools","tools":[]}]}`,
		`{"input":[{"type":"` + jsonUEscape("0061") + `dditional_tools","tools":[1]}]}`,
		`{"input":{"type":"additional_tools","tools":[1]}}`,
		`{"input":null}`,
		`{"input":"additional_tools"}`,
		`{"input":[{"type":"message","content":"caf` + jsonUEscape("00e9") + `"}]}`,
	}
	for _, body := range bodies {
		require.Equal(t, referenceOpenAIRequestBodyHasTools([]byte(body)), openAIRequestBodyHasTools([]byte(body)), body)
	}
}

func TestDeleteJSONPathIfPresent(t *testing.T) {
	body := []byte(`{"model":"m","stream":true,"messages":[]}`)
	got, err := deleteJSONPathIfPresent(body, "metadata")
	require.NoError(t, err)
	require.Same(t, &body[0], &got[0], "absent path must return the original buffer")

	for _, path := range []string{"model", "stream", "messages"} {
		got, err := deleteJSONPathIfPresent(body, path)
		require.NoError(t, err)
		want, wantErr := sjson.DeleteBytes(body, path)
		require.NoError(t, wantErr)
		require.Equal(t, string(want), string(got))
	}
	require.Equal(t, `{"model":"m","stream":true,"messages":[]}`, string(body))

	allocs := testing.AllocsPerRun(20, func() {
		_, _ = deleteJSONPathIfPresent(body, "provider")
	})
	require.Zero(t, allocs)
}

// referenceSanitizeBedrockCacheControl is the copying implementation.
func referenceSanitizeBedrockCacheControl(body []byte, modelID string) []byte {
	isClaude45 := isBedrockClaude45OrNewer(modelID)
	systemArr := gjson.GetBytes(body, "system")
	if systemArr.Exists() && systemArr.IsArray() {
		for i, item := range systemArr.Array() {
			if !item.IsObject() {
				continue
			}
			cc := item.Get("cache_control")
			if !cc.Exists() || !cc.IsObject() {
				continue
			}
			body = deleteCacheControlUnsupportedFields(body, fmt.Sprintf("system.%d.cache_control", i), cc, isClaude45)
		}
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}
	for mi, msg := range messages.Array() {
		if !msg.IsObject() {
			continue
		}
		content := msg.Get("content")
		if !content.Exists() || !content.IsArray() {
			continue
		}
		for ci, block := range content.Array() {
			if !block.IsObject() {
				continue
			}
			cc := block.Get("cache_control")
			if !cc.Exists() || !cc.IsObject() {
				continue
			}
			body = deleteCacheControlUnsupportedFields(body, fmt.Sprintf("messages.%d.content.%d.cache_control", mi, ci), cc, isClaude45)
		}
	}
	return body
}

func TestSanitizeBedrockCacheControlMatchesCopyingScan(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	ccs := []string{
		`{"type":"ephemeral"}`,
		`{"type":"ephemeral","scope":"global"}`,
		`{"type":"ephemeral","ttl":"5m"}`,
		`{"type":"ephemeral","ttl":"1h","scope":"global"}`,
		`{"type":"ephemeral","ttl":"30m"}`,
		`"ephemeral"`,
	}
	block := func() string {
		switch rng.Intn(4) {
		case 0:
			return `"plain"`
		case 1:
			return `{"type":"text","text":"a"}`
		default:
			return `{"type":"text","text":"b","cache_control":` + ccs[rng.Intn(len(ccs))] + `}`
		}
	}
	blocks := func() string {
		var parts []string
		for n := rng.Intn(4); n >= 0; n-- {
			parts = append(parts, block())
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	for i := 0; i < 3000; i++ {
		var msgs []string
		for n := rng.Intn(4); n >= 0; n-- {
			switch rng.Intn(4) {
			case 0:
				msgs = append(msgs, `{"role":"user","content":"text"}`)
			case 1:
				msgs = append(msgs, `"not an object"`)
			default:
				msgs = append(msgs, `{"role":"user","content":`+blocks()+`}`)
			}
		}
		body := `{"system":` + blocks() + `,"messages":[` + strings.Join(msgs, ",") + `]}`
		for _, model := range []string{"anthropic.claude-sonnet-4-6", "anthropic.claude-3-5-sonnet-20241022-v2:0"} {
			want := referenceSanitizeBedrockCacheControl([]byte(body), model)
			got := sanitizeBedrockCacheControl([]byte(body), model)
			require.Equal(t, string(want), string(got), body)
		}
	}
}

func TestExtractBedrockChunkDataReadsEscapedAndPlainBase64(t *testing.T) {
	require.Equal(t, "hi", string(extractBedrockChunkData([]byte(`{"bytes":"aGk="}`))))
	require.Equal(t, "?>?", string(extractBedrockChunkData([]byte(`{"bytes":"Pz4\/"}`))))
	require.Nil(t, extractBedrockChunkData([]byte(`{"bytes":""}`)))
	require.Nil(t, extractBedrockChunkData([]byte(`{"bytes":"%%%"}`)))
	require.Nil(t, extractBedrockChunkData([]byte(`{"other":"aGk="}`)))
}
