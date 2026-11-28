package service

// 工具提升的请求形状矩阵：覆盖 Codex 增量下发工具的常见组合，避免"某种形状下
// 子工具被静默丢弃"这类回归再次出现（含 PR #8592 review 反馈的三种形状）。

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type hoistCase struct {
	name string
	body string
	// wantTop 顶层工具声明，形如 "function:read" / "custom:exec" / "namespace:functions"。
	wantTop []string
	// wantNS namespace 名 -> 子工具名集合（仅校验出现的 namespace）。
	wantNS map[string][]string
	// wantChanged false 表示请求体应保持原样。
	wantChanged bool
}

func TestHoistRequestShapeMatrix(t *testing.T) {
	cases := []hoistCase{
		{
			name:        "无可提升内容时不改写",
			body:        `{"model":"m","input":[{"type":"message","role":"user","content":"namespace 这个词只是正文"}]}`,
			wantChanged: false,
		},
		{
			name: "additional_tools 里的 function/custom 全部提升",
			body: `{"input":[{"type":"additional_tools","tools":[
				{"type":"function","name":"read"},
				{"type":"custom","name":"exec"}
			]},{"type":"message","role":"user","content":"hi"}]}`,
			wantTop:     []string{"function:read", "custom:exec"},
			wantChanged: true,
		},
		{
			name: "顶层 tools 与 additional_tools 共享同名 namespace",
			body: `{"tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"read"}]}],
				"input":[{"type":"additional_tools","tools":[
					{"type":"namespace","name":"functions","tools":[{"type":"function","name":"write"}]}
				]},{"type":"message","role":"user","content":"hi"}]}`,
			wantTop:     []string{"namespace:functions"},
			wantNS:      map[string][]string{"functions": {"read", "write"}},
			wantChanged: true,
		},
		{
			name: "多个 additional_tools 共享同名 namespace",
			body: `{"input":[
				{"type":"additional_tools","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"read"}]}]},
				{"type":"message","role":"user","content":"hi"},
				{"type":"additional_tools","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"write"}]}]}
			]}`,
			wantTop:     []string{"namespace:functions"},
			wantNS:      map[string][]string{"functions": {"read", "write"}},
			wantChanged: true,
		},
		{
			name: "后声明 namespace 里的 custom 提升到顶层",
			body: `{"tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"read"}]}],
				"input":[{"type":"additional_tools","tools":[
					{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec"}]}
				]},{"type":"message","role":"user","content":"hi"}]}`,
			wantTop:     []string{"namespace:functions", "custom:exec"},
			wantNS:      map[string][]string{"functions": {"read"}},
			wantChanged: true,
		},
		{
			name: "完全重复的 namespace 子工具按身份去重",
			body: `{"input":[
				{"type":"additional_tools","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"read"}]}]},
				{"type":"additional_tools","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"read"}]}]},
				{"type":"message","role":"user","content":"hi"}
			]}`,
			wantTop:     []string{"namespace:functions"},
			wantNS:      map[string][]string{"functions": {"read"}},
			wantChanged: true,
		},
		{
			name: "顶层同名 custom 存在时不提升 namespace 内 custom",
			body: `{"tools":[{"type":"custom","name":"exec"}],
				"input":[{"type":"additional_tools","tools":[
					{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec"}]}
				]},{"type":"message","role":"user","content":"hi"}]}`,
			wantTop:     []string{"custom:exec", "namespace:functions"},
			wantNS:      map[string][]string{"functions": {"exec"}},
			wantChanged: true,
		},
		{
			name:        "additional_tools 缺 tools 字段时不报错",
			body:        `{"input":[{"type":"additional_tools"},{"type":"message","role":"user","content":"hi"}]}`,
			wantChanged: true,
		},
		{
			name:        "首包只有 additional_tools 时保留原 item",
			body:        `{"input":[{"type":"additional_tools","tools":[{"type":"custom","name":"exec"}]}]}`,
			wantTop:     []string{"custom:exec"},
			wantChanged: true,
		},
		{
			name: "namespace 里同时有 function 与 custom",
			body: `{"input":[{"type":"additional_tools","tools":[
				{"type":"namespace","name":"functions","tools":[
					{"type":"function","name":"wait"},
					{"type":"custom","name":"exec"}
				]}
			]},{"type":"message","role":"user","content":"hi"}]}`,
			wantTop:     []string{"namespace:functions", "custom:exec"},
			wantNS:      map[string][]string{"functions": {"wait"}},
			wantChanged: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, changed, err := hoistCodexRuntimeTools([]byte(tc.body))
			require.NoError(t, err)
			require.Equal(t, tc.wantChanged, changed)
			if !tc.wantChanged {
				require.Equal(t, tc.body, string(out))
				return
			}

			var gotTop []string
			childrenByNS := map[string][]string{}
			for _, tool := range gjson.GetBytes(out, "tools").Array() {
				typ := tool.Get("type").String()
				name := tool.Get("name").String()
				gotTop = append(gotTop, typ+":"+name)
				if typ == "namespace" {
					var kids []string
					for _, child := range tool.Get("tools").Array() {
						kids = append(kids, child.Get("name").String())
					}
					sort.Strings(kids)
					childrenByNS[name] = kids
				}
			}
			sort.Strings(gotTop)
			wantTop := append([]string(nil), tc.wantTop...)
			sort.Strings(wantTop)
			require.Equal(t, wantTop, gotTop, "顶层声明不符")

			for ns, wantKids := range tc.wantNS {
				want := append([]string(nil), wantKids...)
				sort.Strings(want)
				require.Equal(t, want, childrenByNS[ns], "namespace %q 子工具不符", ns)
			}
		})
	}
}

// 首包只有 additional_tools 时不能被清空：上游会 400 "Input items array must not be empty"。
func TestHoistKeepsNonEmptyInputMatrix(t *testing.T) {
	body := `{"input":[{"type":"additional_tools","tools":[{"type":"custom","name":"exec"}]}]}`
	out, _, err := hoistCodexRuntimeTools([]byte(body))
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(out, "input").Array(), 1)
	require.True(t, hasTopLevelTool(out, "custom", "exec"))
}
