package securityaudit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecisionsAuditIncludesQuestionsJSONAndToolEvidence(t *testing.T) {
	var document any
	require.NoError(t, json.Unmarshal([]byte(`{
		"input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,PRIVATEIMAGE"},{"type":"input_json","data":{"image_url":"evidence text","total":42}}]},
		{"type":"function_call","arguments":"tool arguments"},{"type":"function_call_output","output":"tool evidence"}],
		"questions":[{"name":"category","type":"choice","instructions":[{"type":"input_text","text":"question instructions"},{"type":"input_image","image_url":"https://example.com/PRIVATEIMAGE"}],"choices":["refund","reject"]}]
	}`), &document))
	segments := extractProtocolSegments("openai_decisions", document)
	require.Len(t, segments, 1)
	text := segments[0].text
	for _, evidence := range []string{"question instructions", "refund", "reject", "evidence text", "42", "tool arguments", "tool evidence"} {
		require.Contains(t, text, evidence)
	}
	require.False(t, strings.Contains(text, "PRIVATEIMAGE"))
}
