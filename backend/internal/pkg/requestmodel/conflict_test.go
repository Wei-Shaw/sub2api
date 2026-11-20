package requestmodel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConflictingFieldJSONReportsDisagreeingDuplicates(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "重复 model 取值不同", body: `{"model":"claude-3-5-haiku-20241022","max_tokens":64,"model":"claude-opus-4-8"}`, want: "model"},
		{name: "三个 model", body: `{"model":"a","model":"a","model":"b"}`, want: "model"},
		{name: "大小写变体", body: `{"model":"gpt-5.4-nano","MODEL":"gpt-5.4"}`, want: "MODEL"},
		{name: "转义写法", body: `{"model":"cheap","\u006dodel":"expensive"}`, want: "model"},
		{name: "长 s 折叠为 s", body: "{\"service_tier\":\"flex\",\"\u017Fervice_tier\":\"priority\"}", want: "\u017Fervice_tier"},
		{name: "service_tier 取值不同", body: `{"model":"gpt-5.4","service_tier":"default","input":"hi","service_tier":"priority"}`, want: "service_tier"},
		{name: "类型不同", body: `{"model":"cheap","model":null}`, want: "model"},
		{name: "写法不同的等价值按不一致处理", body: `{"n":1,"n":1.0}`, want: "n"},
		{name: "内容字段", body: `{"messages":[],"model":"a","messages":[{"role":"user","content":"hi"}]}`, want: "messages"},
		{name: "session 内的 model", body: `{"sdp":"v=0","session":{"model":"gpt-live-a","model":"gpt-live-b"}}`, want: "session.model"},
		{name: "前导空白", body: " \n" + `{"model":"a","model":"b"}`, want: "model"},
		{name: "UTF-8 BOM", body: "\xef\xbb\xbf" + `{"model":"a","model":"b"}`, want: "model"},
		{name: "嵌套 reasoning.effort 取值不同", body: `{"model":"gpt-5.4","reasoning":{"effort":"low","effort":"xhigh"},"input":"hi"}`, want: "reasoning.effort"},
		{name: "嵌套 thinking.type 取值不同", body: `{"model":"m","thinking":{"type":"disabled","budget_tokens":1024,"type":"enabled"}}`, want: "thinking.type"},
		{name: "嵌套对象里不一致的重复键", body: `{"model":"a","metadata":{"user_id":"1","user_id":"2"}}`, want: "metadata.user_id"},
		{name: "数组元素里的重复键", body: `{"messages":[{"role":"user","content":"a"},{"role":"user","role":"assistant"}]}`, want: "messages[1].role"},
		{name: "嵌套转义写法", body: `{"reasoning":{"effort":"low","\u0065ffort":"xhigh"}}`, want: "reasoning.effort"},
		{name: "深层嵌套", body: `{"a":[[{"b":{"c":1,"c":2}}]]}`, want: "a[0][0].b.c"},
		{name: "session 内嵌套", body: `{"session":{"audio":{"voice":"a","voice":"b"}}}`, want: "session.audio.voice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conflict := ConflictingField("application/json", []byte(tt.body))
			require.True(t, conflict)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConflictingFieldJSONAcceptsUnambiguousBodies(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "空请求体", body: ""},
		{name: "空对象", body: `{}`},
		{name: "普通请求", body: `{"model":"a","max_tokens":1,"stream":true,"messages":[]}`},
		{name: "相近但不同的字段名", body: `{"model":"a","models":"b","model_id":"c"}`},
		{name: "取值相同的重复 model", body: `{"model":"gpt-5.4","input":"hi","model":"gpt-5.4"}`},
		{name: "取值相同的大小写变体", body: `{"model":"gpt-5.4","Model":"gpt-5.4"}`},
		{name: "session 内取值相同的重复 model", body: `{"session":{"model":"a","model":"a"}}`},
		{name: "嵌套对象里取值相同的重复键", body: `{"model":"a","metadata":{"user_id":"1","user_id":"1"}}`},
		{name: "嵌套层大小写不同的键是不同属性", body: `{"tools":[{"input_schema":{"properties":{"id":{"type":"string"},"ID":{"type":"integer"}}}}]}`},
		{name: "兄弟对象里的同名键", body: `{"a":{"x":1},"b":{"x":2},"c":[{"x":1},{"x":2}]}`},
		{name: "父子对象同名键", body: `{"model":"m","metadata":{"model":"other"}}`},
		{name: "顶层数组", body: `[{"model":"a"},{"model":"b"}]`},
		{name: "字符串值里出现字段名", body: `{"model":"a","input":"{\\"model\\":\\"b\\"}"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conflict := ConflictingField("application/json", []byte(tt.body))
			require.False(t, conflict)
			require.Empty(t, got)
		})
	}
}

// 检查规则和它要防的解析器行为绑在一起：未报告冲突的请求体，gjson（首个、
// 大小写敏感）和 encoding/json 结构体绑定（末值、大小写不敏感）读到的 model
// 必须一致；parsersDisagree 标出两者确实读到不同值的请求体，它们都必须被报告。
func TestConflictingFieldMatchesParserDisagreement(t *testing.T) {
	tests := []struct {
		body            string
		conflict        bool
		parsersDisagree bool
	}{
		{body: `{"model":"first","model":"last"}`, conflict: true, parsersDisagree: true},
		{body: `{"model":"first","MODEL":"last"}`, conflict: true, parsersDisagree: true},
		// gjson 精确匹配到第二个键，这里两类解析器恰好一致；规则按保守处理仍然拒绝。
		{body: `{"MODEL":"first","model":"last"}`, conflict: true},
		{body: `{"model":"same","Model":"same"}`},
		{body: `{"model":"same","x":1,"model":"same"}`},
		{body: `{"model":"only"}`},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			var decoded struct {
				Model string `json:"model"`
			}
			require.NoError(t, json.Unmarshal([]byte(tt.body), &decoded))
			first := gjson.Get(tt.body, "model").String()
			require.Equal(t, tt.parsersDisagree, first != decoded.Model)

			_, conflict := ConflictingField("application/json", []byte(tt.body))
			require.Equal(t, tt.conflict, conflict)
			if !conflict {
				require.Equal(t, first, decoded.Model)
			}
		})
	}
}

func TestConflictingFieldReturnsIndependentCopy(t *testing.T) {
	body := []byte(`{"model":"a","model":"b"}`)
	got, conflict := ConflictingField("application/json", body)
	require.True(t, conflict)
	for i := range body {
		body[i] = 'x'
	}
	require.Equal(t, "model", got)
}

func multipartBody(t *testing.T, fields ...[2]string) (string, []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range fields {
		require.NoError(t, writer.WriteField(field[0], field[1]))
	}
	require.NoError(t, writer.Close())
	return writer.FormDataContentType(), body.Bytes()
}

func TestConflictingFieldMultipart(t *testing.T) {
	t.Run("model 字段取值不同", func(t *testing.T) {
		contentType, body := multipartBody(t, [2]string{"model", "gpt-image-1"}, [2]string{"prompt", "cat"}, [2]string{"model", "gpt-image-1.5"})
		got, conflict := ConflictingField(contentType, body)
		require.True(t, conflict)
		require.Equal(t, "model", got)
	})
	t.Run("model 字段取值相同", func(t *testing.T) {
		contentType, body := multipartBody(t, [2]string{"model", "gpt-image-1"}, [2]string{"model", "gpt-image-1"})
		_, conflict := ConflictingField(contentType, body)
		require.False(t, conflict)
	})
	t.Run("session 字段取值不同", func(t *testing.T) {
		contentType, body := multipartBody(t, [2]string{"sdp", "v=0"}, [2]string{"session", `{"model":"a"}`}, [2]string{"session", `{"model":"b"}`})
		got, conflict := ConflictingField(contentType, body)
		require.True(t, conflict)
		require.Equal(t, "session", got)
	})
	t.Run("session JSON 内的 model 取值不同", func(t *testing.T) {
		contentType, body := multipartBody(t, [2]string{"sdp", "v=0"}, [2]string{"session", `{"model":"a","instructions":"hi","model":"b"}`})
		got, conflict := ConflictingField(contentType, body)
		require.True(t, conflict)
		require.Equal(t, "session.model", got)
	})
	t.Run("普通表单", func(t *testing.T) {
		contentType, body := multipartBody(t, [2]string{"model", "gpt-image-1"}, [2]string{"prompt", `{"model":"a","model":"b"}`})
		_, conflict := ConflictingField(contentType, body)
		require.False(t, conflict)
	})
}

func TestConflictingFieldManyNestedMembersUsesIndex(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"model":"m","tool":{`)
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, `"k%d":%d,`, i, i)
	}
	body := b.String()

	got, conflict := ConflictingField("application/json", []byte(body+`"k3":0}}`))
	require.True(t, conflict)
	require.Equal(t, "tool.k3", got)

	_, conflict = ConflictingField("application/json", []byte(body+`"k3":3}}`))
	require.False(t, conflict, "identical duplicate past the linear-scan limit must pass")

	_, conflict = ConflictingField("application/json", []byte(body+`"k100":0}}`))
	require.False(t, conflict)
}

func TestConflictingFieldDeepNestingDoesNotOverflow(t *testing.T) {
	depth := maxConflictDepth + 50
	objects := `{"a":` + strings.Repeat(`{"a":`, depth) + "1" + strings.Repeat("}", depth) + `}`
	_, conflict := ConflictingField("application/json", []byte(objects))
	require.False(t, conflict)

	arrays := `{"a":` + strings.Repeat("[", depth) + strings.Repeat("]", depth) + `}`
	_, conflict = ConflictingField("application/json", []byte(arrays))
	require.False(t, conflict)
}

func TestConflictingFieldMultipartNestedSession(t *testing.T) {
	contentType, body := multipartBody(t, [2]string{"sdp", "v=0"}, [2]string{"session", `{"model":"a","audio":{"voice":"x","voice":"y"}}`})
	got, conflict := ConflictingField(contentType, body)
	require.True(t, conflict)
	require.Equal(t, "session.audio.voice", got)
}

func BenchmarkConflictingField(b *testing.B) {
	var sb strings.Builder
	sb.WriteString(`{"model":"claude-sonnet-4-5","max_tokens":4096,"stream":true,"system":[{"type":"text","text":"sys"}],"tools":[`)
	for i := 0; i < 40; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"name":"tool_%d","description":"%s","input_schema":{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path"]}}`, i, strings.Repeat("d", 200))
	}
	sb.WriteString(`],"messages":[`)
	for i := 0; i < 200; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"role":"user","content":[{"type":"text","text":"%s"}]}`, strings.Repeat("t", 1000))
	}
	sb.WriteString(`]}`)
	body := []byte(sb.String())
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, conflict := ConflictingField("application/json", body); conflict {
			b.Fatal("unexpected conflict")
		}
	}
}

func TestHasConflictingModels(t *testing.T) {
	require.False(t, HasConflictingModels(nil))
	require.False(t, HasConflictingModels([]string{"a"}))
	require.False(t, HasConflictingModels([]string{"a", "a", "a"}))
	require.True(t, HasConflictingModels([]string{"a", "b"}))
	require.True(t, HasConflictingModels([]string{"a", "a", "b"}))
}
