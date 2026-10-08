//go:build unit

package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDuplicateModelKeyParserDiscrepancy 固化本修复所依据的解析器行为假设：
// 重复的顶层 model 键会被 gjson 绑定到首个值、被 encoding/json 绑定到最后一个值。
// 该差异正是「计费/准入（gjson 首键）」与「协议转换/上游执行（末键）」指向不同
// 模型的根因；若依赖的解析器行为发生变化，此测试应最先失败。
func TestDuplicateModelKeyParserDiscrepancy(t *testing.T) {
	body := []byte(`{"model":"cheap","messages":[{"role":"user","content":"hi"}],"model":"expensive"}`)

	var cc struct {
		Model string `json:"model"`
	}
	require.NoError(t, json.Unmarshal(body, &cc))
	require.Equal(t, "expensive", cc.Model, "encoding/json 对重复键取末值")
	require.Equal(t, "cheap", gjson.GetBytes(body, "model").String(), "gjson 对重复键取首值")
}

func TestHasDuplicateTopLevelKey(t *testing.T) {
	cases := []struct {
		name string
		body string
		key  string
		want bool
	}{
		{"single model key", `{"model":"a","messages":[]}`, "model", false},
		{"duplicate model key", `{"model":"a","messages":[],"model":"b"}`, "model", true},
		{"nested same key is not top level", `{"model":"a","messages":[{"model":"b"}]}`, "model", false},
		{"duplicate of another key ignored", `{"stream":false,"model":"a","stream":true}`, "model", false},
		{"key missing", `{"messages":[]}`, "model", false},
		{"empty body", ``, "model", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, requestmodel.HasDuplicateTopLevelKey([]byte(tc.body), tc.key))
		})
	}
}

func TestReplaceModelInBodyCollapsesDuplicateModelKeys(t *testing.T) {
	t.Run("duplicate keys collapse to single key with new value", func(t *testing.T) {
		body := []byte(`{"model":"cheap","messages":[{"role":"user","content":"hi"}],"model":"expensive"}`)

		out := ReplaceModelInBody(body, "mapped")

		// 重复键被收敛，其余键的相对顺序保持不变。
		require.JSONEq(t, `{"messages":[{"role":"user","content":"hi"}],"model":"mapped"}`, string(out))
		require.Equal(t, 1, countTopLevelModelKeys(t, out))
	})

	t.Run("first key equal to new value still collapses remaining duplicates", func(t *testing.T) {
		// 旧实现的首键快速路径在这里会原样返回，把走私键留在请求体内。
		body := []byte(`{"alpha":1,"model":"cheap","messages":[],"model":"expensive","omega":2}`)

		out := ReplaceModelInBody(body, "cheap")

		require.Equal(t, 1, countTopLevelModelKeys(t, out))
		require.Equal(t, "cheap", gjson.GetBytes(out, "model").String())
		// 其余键相对顺序不变，model 键保留在原重复键位置。
		assertTopLevelOrder(t, out, `"alpha"`, `"messages"`, `"omega"`)
	})
}

func TestReplaceModelInBodySingleKey(t *testing.T) {
	t.Run("same model returns body unchanged", func(t *testing.T) {
		body := []byte(`{"model":"a","messages":[]}`)
		require.JSONEq(t, string(body), string(ReplaceModelInBody(body, "a")))
	})
	t.Run("different model is replaced in place", func(t *testing.T) {
		body := []byte(`{"alpha":1,"model":"a","messages":[],"omega":2}`)
		out := ReplaceModelInBody(body, "b")
		require.Equal(t, "b", gjson.GetBytes(out, "model").String())
		require.False(t, requestmodel.HasDuplicateTopLevelKey(out, "model"))
		assertTopLevelOrder(t, out, `"alpha"`, `"model"`, `"messages"`, `"omega"`)
	})
	t.Run("empty body returned as is", func(t *testing.T) {
		require.Empty(t, ReplaceModelInBody(nil, "b"))
	})
}

func countTopLevelModelKeys(t *testing.T, body []byte) int {
	t.Helper()
	count := 0
	gjson.ParseBytes(body).ForEach(func(k, _ gjson.Result) bool {
		if k.String() == "model" {
			count++
		}
		return true
	})
	return count
}

func assertTopLevelOrder(t *testing.T, body []byte, tokens ...string) {
	t.Helper()
	last := -1
	for _, token := range tokens {
		pos := strings.Index(string(body), token)
		require.NotEqualf(t, -1, pos, "missing token %s in body %s", token, body)
		require.Greaterf(t, pos, last, "token %s should appear after previous tokens in body %s", token, body)
		last = pos
	}
}

func TestReplaceModelInBodyCollapsesAmbiguousKeys(t *testing.T) {
	for _, body := range []string{
		`{"model":"a","messages":[{"model":"nested"}],"model":"b","n":900719925474099312345}`,
		`{"Model":"b","messages":[{"model":"nested"}],"\u006dodel":"a","n":900719925474099312345}`,
		`{"model":"a","messages":[{"model":"nested"}],"MODEL":null,"n":900719925474099312345}`,
	} {
		out := ReplaceModelInBody([]byte(body), "a")
		require.Equal(t, `{"messages":[{"model":"nested"}],"model":"a","n":900719925474099312345}`, string(out))
	}
	many := `{` + strings.Repeat(`"model":"b",`, 10000) + `"model":"a","n":900719925474099312345}`
	require.Equal(t, `{"model":"mapped","n":900719925474099312345}`, string(ReplaceModelInBody([]byte(many), "mapped")))
}
