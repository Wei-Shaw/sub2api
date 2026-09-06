//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeCodexModelsManifestBodiesUnionAndOrder(t *testing.T) {
	first := `{"object":"codex.manifest","models":[{"slug":"model-a","display_name":"A from first"},{"slug":"model-b"}]}`
	second := `{"object":"codex.manifest","models":[{"slug":"model-a","display_name":"A from second"},{"slug":"model-c"}]}`

	merged, err := mergeCodexModelsManifestBodies([][]byte{[]byte(first), []byte(second)})
	require.NoError(t, err)

	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(merged, &envelope))
	var objectField string
	require.NoError(t, json.Unmarshal(envelope["object"], &objectField))
	require.Equal(t, "codex.manifest", objectField)

	var models []map[string]any
	require.NoError(t, json.Unmarshal(envelope["models"], &models))
	require.Len(t, models, 3)
	require.Equal(t, "model-a", models[0]["slug"])
	require.Equal(t, "A from first", models[0]["display_name"], "重复 slug 必须取配置顺序靠前账号的条目")
	require.Equal(t, "model-b", models[1]["slug"])
	require.Equal(t, "model-c", models[2]["slug"])
}

func TestMergeCodexModelsManifestBodiesEnvelopeFromFirstBody(t *testing.T) {
	first := `{"object":"codex.manifest","extra_field":"from-first","models":[{"slug":"model-a"}]}`
	second := `{"object":"other","extra_field":"from-second","another":"only-in-second","models":[]}`

	merged, err := mergeCodexModelsManifestBodies([][]byte{[]byte(first), []byte(second)})
	require.NoError(t, err)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(merged, &envelope))
	require.Equal(t, "codex.manifest", envelope["object"])
	require.Equal(t, "from-first", envelope["extra_field"], "顶层字段以第一个信封为基底")
	require.NotContains(t, envelope, "another")
}

func TestMergeCodexModelsManifestBodiesHandlesSluglessEntries(t *testing.T) {
	first := `{"models":[{"slug":"model-a"},{"display_name":"no slug one"}]}`
	second := `{"models":[{"display_name":"no slug one"},{"display_name":"no slug two"},{"slug":"model-a"}]}`

	merged, err := mergeCodexModelsManifestBodies([][]byte{[]byte(first), []byte(second)})
	require.NoError(t, err)

	var envelope struct {
		Models []map[string]any `json:"models"`
	}
	require.NoError(t, json.Unmarshal(merged, &envelope))
	require.Len(t, envelope.Models, 3)
	require.Equal(t, "model-a", envelope.Models[0]["slug"])
	require.Equal(t, "no slug one", envelope.Models[1]["display_name"], "无 slug 条目按出现顺序保留一次")
	require.Equal(t, "no slug two", envelope.Models[2]["display_name"])
}

func TestMergeCodexModelsManifestBodiesSingleBody(t *testing.T) {
	body := `{"models":[{"slug":"model-a"}]}`

	merged, err := mergeCodexModelsManifestBodies([][]byte{[]byte(body)})
	require.NoError(t, err)
	require.JSONEq(t, body, string(merged))
}

func TestMergeCodexModelsManifestBodiesRejectsInvalidInput(t *testing.T) {
	_, err := mergeCodexModelsManifestBodies(nil)
	require.Error(t, err)

	_, err = mergeCodexModelsManifestBodies([][]byte{[]byte(`{`)})
	require.Error(t, err)

	_, err = mergeCodexModelsManifestBodies([][]byte{[]byte(`{}`), []byte(`{"models":{}`)})
	require.Error(t, err, "models 非数组必须报错")
}
