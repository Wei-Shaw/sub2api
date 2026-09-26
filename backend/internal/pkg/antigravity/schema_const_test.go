package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildToolsPreservesStringConst(t *testing.T) {
	for _, tc := range []struct {
		name   string
		schema string
		want   string
	}{
		{"typed", `{"type":"string","const":"browser"}`, `{"type":"string","enum":["browser"]}`},
		{"inferred", `{"const":"browser"}`, `{"type":"string","enum":["browser"]}`},
		{"empty", `{"type":"string","const":""}`, `{"type":"string","enum":[""]}`},
		{"existing enum", `{"type":"string","const":"browser","enum":["browser","shell"]}`, `{"type":"string","enum":["browser"]}`},
		{"conflicting enum", `{"type":"string","const":"browser","enum":["shell"]}`, `{"type":"string","enum":[]}`},
		{"ordinary enum", `{"type":"string","enum":["browser","shell"]}`, `{"type":"string","enum":["browser","shell"]}`},
		{"array items", `{"type":"array","items":{"type":"string","const":"browser"}}`, `{"type":"array","items":{"type":"string","enum":["browser"]}}`},
		{"nested property named const", `{"type":"object","properties":{"const":{"const":"browser"}}}`, `{"type":"object","properties":{"const":{"type":"string","enum":["browser"]}}}`},
		{"schema metadata", `{"type":"string","const":"browser","$schema":"https://json-schema.org/draft/2020-12/schema","description":"Tool kind"}`, `{"type":"string","enum":["browser"],"description":"Tool kind"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var property map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.schema), &property))
			tools := buildTools([]ClaudeTool{{
				Name: "dispatch",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"action": property},
				},
			}})
			require.Len(t, tools, 1)
			require.Len(t, tools[0].FunctionDeclarations, 1)
			properties, ok := tools[0].FunctionDeclarations[0].Parameters["properties"].(map[string]any)
			require.True(t, ok, "tool parameters must contain a properties object")
			got, err := json.Marshal(properties["action"])
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(got))
		})
	}
}
