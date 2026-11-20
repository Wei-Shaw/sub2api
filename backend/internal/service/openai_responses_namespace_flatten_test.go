package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// legacyFlattenOpenAIResponsesNamespaces is the whole-body implementation this
// change replaces. Tests below compare the span-based rewrite against it.
func legacyFlattenOpenAIResponsesNamespaces(c *gin.Context, body []byte) ([]byte, error) {
	if !bytes.Contains(body, []byte(`"namespace"`)) {
		return body, nil
	}
	var requestBody map[string]any
	if err := json.Unmarshal(body, &requestBody); err != nil {
		return body, fmt.Errorf("decode OpenAI namespace body: %w", err)
	}
	names, changed, err := apicompat.FlattenResponsesNamespacesExcept(requestBody, map[string]bool{"image_gen": true})
	if err != nil {
		return body, err
	}
	if !changed {
		return body, nil
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, fmt.Errorf("encode OpenAI namespace body: %w", err)
	}
	setOpenAIResponsesNamespaceNames(c, names)
	return rebuilt, nil
}

func flattenEquivalenceBodies() map[string]string {
	return map[string]string{
		"namespace_tool":             `{"model":"gpt-6.1","tools":[{"type":"namespace","name":"mcp","tools":[{"type":"function","name":"search","parameters":{"type":"object"}}]}],"input":[{"type":"function_call","namespace":"mcp","name":"search","call_id":"c1"}]}`,
		"preserved_image_gen":        `{"model":"gpt-6.1","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"draw"}]},{"type":"namespace","name":"mcp","tools":[{"type":"function","name":"search"}]}],"input":[{"type":"function_call","namespace":"mcp","name":"search"}]}`,
		"tool_choice_namespace":      `{"model":"gpt-6.1","tools":[{"type":"namespace","name":"mcp","tools":[{"type":"function","name":"search"}]}],"tool_choice":{"type":"namespace","name":"mcp"}}`,
		"tool_choice_call":           `{"model":"gpt-6.1","tools":[{"type":"namespace","name":"mcp","tools":[{"type":"function","name":"search"}]}],"tool_choice":{"type":"function_call","namespace":"mcp","name":"search"}}`,
		"no_namespace":               `{"model":"gpt-6.1","tools":[{"type":"function","name":"search"}],"input":[{"type":"function_call","name":"search","call_id":"c1"}]}`,
		"non_matching_item":          `{"model":"gpt-6.1","tools":[{"type":"namespace","name":"mcp","tools":[{"type":"function","name":"search"}]}],"input":[{"type":"function_call","namespace":"other","name":"search"},{"type":"message","content":[{"type":"output_text","text":"hi"}]}]}`,
		"namespace_without_children": `{"model":"gpt-6.1","tools":[{"type":"namespace","name":"mcp","tools":[]}],"input":[]}`,
		"conflict_with_top_level":    `{"model":"gpt-6.1","tools":[{"type":"function","name":"mcp__search"},{"type":"namespace","name":"mcp","tools":[{"type":"function","name":"search"}]}]}`,
		"duplicate_flat":             `{"model":"gpt-6.1","tools":[{"type":"namespace","name":"a","tools":[{"type":"function","name":"__b"}]},{"type":"namespace","name":"a__b","tools":[{"type":"function","name":""}]}]}`,
		"large_history":              `{"model":"gpt-6.1","tools":[{"type":"namespace","name":"mcp","tools":[{"type":"function","name":"search"}]}],"input":[{"type":"message","content":[{"type":"output_text","text":"` + string(bytes.Repeat([]byte("x"), 256*1024)) + `"}]},{"type":"function_call","namespace":"mcp","name":"search","call_id":"c1"}]}`,
	}
}

func TestFlattenOpenAIResponsesNamespacesMatchesLegacy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, raw := range flattenEquivalenceBodies() {
		t.Run(name, func(t *testing.T) {
			wantCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
			want, wantErr := legacyFlattenOpenAIResponsesNamespaces(wantCtx, []byte(raw))

			gotCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
			got, gotErr := flattenOpenAIResponsesNamespaces(gotCtx, []byte(raw))

			if wantErr != nil {
				require.Error(t, gotErr)
				return
			}
			require.NoError(t, gotErr)
			require.Equal(t, openAIResponsesNamespaceNames(wantCtx), openAIResponsesNamespaceNames(gotCtx))

			var wantJSON, gotJSON any
			require.NoError(t, json.Unmarshal(want, &wantJSON))
			require.NoError(t, json.Unmarshal(got, &gotJSON))
			require.Equal(t, wantJSON, gotJSON)
		})
	}
}

func TestFlattenOpenAIResponsesNamespacesLeavesNonNamespaceBodiesByteExact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-6.1","input":[{"type":"message","content":"hi"}]}`)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	out, err := flattenOpenAIResponsesNamespaces(ctx, body)
	require.NoError(t, err)
	require.Equal(t, body, out)
}

func TestApplyJSONSpanEdits(t *testing.T) {
	body := []byte(`{"a":1,"b":2,"c":3}`)

	out, ok := applyJSONSpanEdits(body, []jsonSpanEdit{
		{start: 17, end: 18, data: []byte(`9`)}, // "3" -> 9
		{start: 11, end: 12, data: []byte(`8`)}, // "2" -> 8
	})
	require.True(t, ok)
	require.Equal(t, `{"a":1,"b":8,"c":9}`, string(out))

	_, ok = applyJSONSpanEdits(body, []jsonSpanEdit{
		{start: 11, end: 13, data: []byte(`8`)},
		{start: 12, end: 14, data: []byte(`8`)},
	})
	require.False(t, ok, "overlapping spans must be rejected")

	_, ok = applyJSONSpanEdits(body, []jsonSpanEdit{{start: 0, end: len(body) + 1, data: nil}})
	require.False(t, ok, "out-of-range spans must be rejected")

	_, ok = applyJSONSpanEdits(body, nil)
	require.False(t, ok)
}
