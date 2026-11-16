package service

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// legacyStripOpenAIResponsesInputNamespaces 是改动前的逐字副本，作为差分基准。
func legacyStripOpenAIResponsesInputNamespaces(body []byte, keepToolCallNamespaces bool) ([]byte, error) {
	if !bytes.Contains(body, []byte(`"namespace"`)) {
		return body, nil
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, nil
	}

	var rebuilt bytes.Buffer
	changed := false
	copyFrom := 0
	var stripErr error
	input.ForEach(func(_, item gjson.Result) bool {
		if !item.IsObject() || !item.Get("namespace").Exists() ||
			(keepToolCallNamespaces && isOpenAIResponsesToolCallItemType(item.Get("type").String())) {
			return true
		}
		itemBody, err := sjson.DeleteBytes([]byte(item.Raw), "namespace")
		if err != nil {
			stripErr = err
			return false
		}
		if !changed {
			rebuilt.Grow(len(input.Raw))
			changed = true
		}
		itemStart := item.Index - input.Index
		_, _ = rebuilt.WriteString(input.Raw[copyFrom:itemStart])
		_, _ = rebuilt.Write(itemBody)
		copyFrom = itemStart + len(item.Raw)
		return true
	})
	if stripErr != nil {
		return body, fmt.Errorf("delete OpenAI input namespace: %w", stripErr)
	}
	if !changed {
		return body, nil
	}
	_, _ = rebuilt.WriteString(input.Raw[copyFrom:])
	stripped, err := sjson.SetRawBytes(body, "input", rebuilt.Bytes())
	if err != nil {
		return body, fmt.Errorf("replace OpenAI input after namespace deletion: %w", err)
	}
	return stripped, nil
}

type stripSnapshot struct {
	err      string
	output   string
	unchanged bool // 是否原样返回了传入的 slice（两个实现都应如此）
}

func stripSnapshotOf(out []byte, err error, body []byte) stripSnapshot {
	snap := stripSnapshot{}
	if err != nil {
		snap.err = err.Error()
	}
	snap.output = string(out)
	if len(out) == len(body) && len(body) > 0 && &out[0] == &body[0] {
		snap.unchanged = true
	}
	if len(out) == 0 && len(body) == 0 {
		snap.unchanged = true
	}
	return snap
}

func stripBoth(t *testing.T, input string, keep bool) (stripSnapshot, stripSnapshot) {
	t.Helper()
	legacyBody := []byte(input)
	legacyOut, legacyErr := legacyStripOpenAIResponsesInputNamespaces(legacyBody, keep)

	newBody := []byte(input)
	newOut, newErr := stripOpenAIResponsesInputNamespaces(newBody, keep)

	return stripSnapshotOf(legacyOut, legacyErr, legacyBody), stripSnapshotOf(newOut, newErr, newBody)
}

func TestStripOpenAIResponsesInputNamespacesMatchesLegacy(t *testing.T) {
	escape := string(rune(92))
	cases := []string{
		``,
		`{}`,
		`{"model":"gpt-5.5"}`,
		// 无 namespace：入口预检查直接返回
		`{"input":[{"type":"message","content":"hi"}]}`,
		// namespace 只在 tools 里（Codex 常态）
		`{"tools":[{"type":"namespace","name":"image_gen"}],"input":[{"type":"message","content":"hi"}]}`,
		// namespace 嵌套在 item 内容里，不是数组项的直接字段
		`{"input":[{"content":{"namespace":"nested"}}]}`,
		`{"tools":[{"type":"namespace"}],"input":[{"content":{"namespace":"nested"}}]}`,
		// 真正要改写
		`{"input":[{"type":"function_call","namespace":"files","name":"search"}]}`,
		`{"input":[{"namespace":"files"},{"type":"message","content":"hi"}]}`,
		`{"input":[{"type":"message","content":"hi"},{"namespace":"files"}]}`,
		`{"input":[{"namespace":"a"},{"namespace":"b"},{"namespace":"c"}]}`,
		`{"input":[{"namespace":null}]}`,
		`{"input":[{"namespace":["x"]}]}`,
		`{"input":[{"namespace":"files","namespace":"dup"}]}`,
		// 工具调用项在 keep 模式下的区分
		`{"input":[{"type":"function_call","namespace":"files"}]}`,
		`{"input":[{"type":"custom_tool_call","namespace":"files"}]}`,
		`{"input":[{"type":"mcp_tool_call","namespace":"files"}]}`,
		`{"input":[{"type":"tool_call","namespace":"files"}]}`,
		`{"input":[{"type":"message","namespace":"files"}]}`,
		`{"input":[{"type":"FUNCTION_CALL","namespace":"files"}]}`,
		// 非对象 item
		`{"input":["x",null,{"namespace":"files"},42]}`,
		// input 不是数组 / 缺失
		`{"input":{"namespace":"x"}}`,
		`{"input":"namespace"}`,
		`{"input":null}`,
		`{"namespace":"top"}`,
		// 重复 input 键：两边都取第一个
		`{"input":[{"namespace":"first"}],"input":[{"namespace":"second"}]}`,
		`{"input":[],"input":[{"namespace":"second"}]}`,
		// 转义的键与值
		`{"` + escape + `u0069nput":[{"namespace":"files"}]}`,
		`{"input":[{"namespace":"fi` + escape + `u006ces"}]}`,
		// 大内容与 unicode
		`{"input":[{"namespace":"files","content":"` + strings.Repeat("x", 4096) + `"},{"type":"message","content":"中文 😀"}]}`,
		// item 内部同时有直接 namespace 与嵌套 namespace
		`{"input":[{"namespace":"files","content":{"namespace":"nested"}}]}`,
		// 非法 JSON
		`{"input":[{"namespace":"files"}]`,
		`{"input":[{"namespace":"files"},`,
		// 根不是对象
		`[{"namespace":"files"}]`,
		`"namespace"`,
	}

	for _, tc := range cases {
		for _, keep := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/keep=%v", truncateForName(tc), keep), func(t *testing.T) {
				want, got := stripBoth(t, tc, keep)
				if !gjson.ValidBytes([]byte(tc)) {
					// 畸形输入上 gjson.GetBytes 与 gjson.Parse 返回的区间不同：
					// 前者会一路扫到结尾，后者遇到非法结构就停。本实现走后者，因此在
					// 无法解析的片段上不改写（更保守），只比较错误。
					require.Equal(t, want.err, got.err, "input=%q keep=%v", tc, keep)
					return
				}
				require.Equal(t, want, got, "input=%q keep=%v", tc, keep)
			})
		}
	}
}

func truncateForName(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	if s == "" {
		return "empty"
	}
	return s
}

// 组合随机：多个 item、namespace 位置随机、keep 随机。
func TestStripOpenAIResponsesInputNamespacesMatchesLegacyRandomized(t *testing.T) {
	items := []string{
		`{"type":"message","content":"hi"}`,
		`{"namespace":"files"}`,
		`{"type":"function_call","namespace":"files","name":"search"}`,
		`{"type":"custom_tool_call","namespace":"files"}`,
		`{"content":{"namespace":"nested"}}`,
		`{"namespace":"files","content":"` + strings.Repeat("y", 200) + `"}`,
		`{"namespace":null}`,
		`"literal"`,
		`42`,
		`null`,
	}
	prefixes := []string{
		`{"input":[`,
		`{"tools":[{"type":"namespace"}],"input":[`,
		`{"metadata":{"user_id":"u"},"input":[`,
	}

	seed := uint64(0x243f6a8885a308d3)
	next := func() uint64 {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return seed
	}

	for i := 0; i < 3000; i++ {
		n := int(next()%5) + 1
		parts := make([]string, 0, n)
		for j := 0; j < n; j++ {
			parts = append(parts, items[int(next()%uint64(len(items)))])
		}
		body := prefixes[int(next()%uint64(len(prefixes)))] + strings.Join(parts, ",") + `]}`
		keep := next()%2 == 0

		want, got := stripBoth(t, body, keep)
		require.Equal(t, want, got, "iteration=%d keep=%v body=%s", i, keep, body)
	}
}

// 畸形输入的行为差异有明确用例固定，避免后续误改：
func TestStripOpenAIResponsesInputNamespacesMalformedRootIsConservative(t *testing.T) {
	const malformed = `0{"input"[{ "namespace"00`
	out, err := stripOpenAIResponsesInputNamespaces([]byte(malformed), true)
	require.NoError(t, err)
	require.Equal(t, malformed, string(out), "改写在无法解析的片段上必须放弃")
}

func FuzzStripOpenAIResponsesInputNamespaces(f *testing.F) {
	f.Add([]byte(`{"input":[{"namespace":"files"}]}`), false)
	f.Add([]byte(`{"tools":[{"type":"namespace"}],"input":[{"type":"message"}]}`), true)
	f.Add([]byte(`{"input":[{"type":"function_call","namespace":"files"}]}`), true)
	f.Add([]byte(`{"input":"namespace"}`), false)
	f.Fuzz(func(t *testing.T, body []byte, keep bool) {
		legacyBody := append([]byte(nil), body...)
		newBody := append([]byte(nil), body...)
		legacyOut, legacyErr := legacyStripOpenAIResponsesInputNamespaces(legacyBody, keep)
		newOut, newErr := stripOpenAIResponsesInputNamespaces(newBody, keep)
		require.Equal(t, fmt.Sprint(legacyErr), fmt.Sprint(newErr), "body=%q", body)
		if legacyErr != nil || !gjson.ValidBytes(body) {
			// 畸形输入两个 gjson API 的取值区间不同，见表驱动测试里的说明。
			return
		}
		require.Equal(t, string(legacyOut), string(newOut), "body=%q keep=%v", body, keep)
	})
}
