package requestmodel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConflictingFieldJSONReportsDuplicates(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "重复 model 取值不同", body: `{"model":"claude-3-5-haiku-20241022","max_tokens":64,"model":"claude-opus-4-8"}`, want: "model"},
		{name: "取值相同的重复 model", body: `{"model":"gpt-5.4","input":"hi","model":"gpt-5.4"}`, want: "model"},
		{name: "三个 model", body: `{"model":"a","model":"a","model":"b"}`, want: "model"},
		{name: "大小写变体", body: `{"model":"gpt-5.4-nano","MODEL":"gpt-5.4"}`, want: "MODEL"},
		{name: "取值相同的大小写变体", body: `{"model":"gpt-5.4","Model":"gpt-5.4"}`, want: "Model"},
		{name: "转义写法", body: `{"model":"cheap","\u006dodel":"expensive"}`, want: "model"},
		{name: "代理对转义", body: `{"\ud83d\ude00":1,"` + "\U0001F600" + `":2}`, want: "\U0001F600"},
		{name: "孤立代理项与无效 UTF-8 都解码为 U+FFFD", body: "{\"m\\ud800\":1,\"m\xff\":2}", want: "m\uFFFD"},
		{name: "长 s 折叠为 s", body: "{\"service_tier\":\"flex\",\"\u017Fervice_tier\":\"priority\"}", want: "\u017Fervice_tier"},
		{name: "开尔文符号折叠为 k", body: "{\"top_k\":1,\"top_\u212a\":2}", want: "top_\u212a"},
		{name: "service_tier 取值不同", body: `{"model":"gpt-5.4","service_tier":"default","input":"hi","service_tier":"priority"}`, want: "service_tier"},
		{name: "类型不同", body: `{"model":"cheap","model":null}`, want: "model"},
		{name: "内容字段", body: `{"messages":[],"model":"a","messages":[{"role":"user","content":"hi"}]}`, want: "messages"},
		{name: "session 内的 model", body: `{"sdp":"v=0","session":{"model":"gpt-live-a","model":"gpt-live-b"}}`, want: "session.model"},
		{name: "前导空白", body: " \n" + `{"model":"a","model":"b"}`, want: "model"},
		{name: "UTF-8 BOM", body: "\xef\xbb\xbf" + `{"model":"a","model":"b"}`, want: "model"},
		{name: "嵌套 reasoning.effort 取值不同", body: `{"model":"gpt-5.4","reasoning":{"effort":"low","effort":"xhigh"},"input":"hi"}`, want: "reasoning.effort"},
		{name: "嵌套 reasoning.effort 取值相同", body: `{"model":"gpt-5.4","reasoning":{"effort":"xhigh","effort":"xhigh"},"input":"hi"}`, want: "reasoning.effort"},
		{name: "嵌套大小写变体", body: `{"model":"gpt-5.4","input":"hi","reasoning":{"effort":"low","Effort":"xhigh"}}`, want: "reasoning.Effort"},
		{name: "output_config 大小写变体", body: `{"model":"m","output_config":{"effort":"low","EFFORT":"max"}}`, want: "output_config.EFFORT"},
		{name: "嵌套 thinking.type", body: `{"model":"m","thinking":{"type":"disabled","budget_tokens":1024,"type":"enabled"}}`, want: "thinking.type"},
		{name: "嵌套对象里的重复键", body: `{"model":"a","metadata":{"user_id":"1","user_id":"2"}}`, want: "metadata.user_id"},
		{name: "数组元素里的重复键", body: `{"messages":[{"role":"user","content":"a"},{"role":"user","role":"assistant"}]}`, want: "messages[1].role"},
		{name: "嵌套转义写法", body: `{"reasoning":{"effort":"low","\u0065ffort":"xhigh"}}`, want: "reasoning.effort"},
		{name: "深层嵌套", body: `{"a":[[{"b":{"c":1,"c":2}}]]}`, want: "a[0][0].b.c"},
		{name: "session 内嵌套", body: `{"session":{"audio":{"voice":"a","voice":"b"}}}`, want: "session.audio.voice"},
		{name: "自由键空间内的精确重复", body: `{"tools":[{"input_schema":{"properties":{"id":{},"id":{}}}}]}`, want: "tools[0].input_schema.properties.id"},
		{name: "顶层 input 仍做折叠", body: `{"input":[{"role":"user","Role":"system","content":"x"}]}`, want: "input[0].Role"},
		{name: "超深子树之后的同层重复", body: `{"model":"a","x":` + strings.Repeat("[", maxConflictDepth+5) + strings.Repeat("]", maxConflictDepth+5) + `,"model":"b"}`, want: "model"},
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
		{name: "兄弟对象里的同名键", body: `{"a":{"x":1},"b":{"x":2},"c":[{"x":1},{"x":2}]}`},
		{name: "父子对象同名键", body: `{"model":"m","metadata":{"model":"other"}}`},
		{name: "schema 属性大小写不同", body: `{"tools":[{"input_schema":{"type":"object","properties":{"id":{"type":"string"},"ID":{"type":"integer"}}}}]}`},
		{name: "tool_use 参数大小写不同", body: `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t","name":"f","input":{"path":"a","Path":"b"}}]}]}`},
		{name: "Gemini 函数参数与返回大小写不同", body: `{"contents":[{"parts":[{"functionCall":{"name":"f","args":{"k":1,"K":2}}},{"functionResponse":{"name":"f","response":{"v":1,"V":2}}}]}]}`},
		{name: "metadata 大小写不同", body: `{"metadata":{"user_id":"1","USER_ID":"2"}}`},
		{name: "字符串值里出现字段名", body: `{"model":"a","input":"{\"model\":\"b\",\"model\":\"c\"}"}`},
		{name: "字符串里的括号与逗号", body: `{"a":"{[,\"]}","b":"\\","c":1}`},
		{name: "顶层数组里的不同对象", body: `[{"model":"a"},{"model":"b"}]`},
		{name: "不是 JSON", body: `model=a&model=b`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conflict := ConflictingField("application/json", []byte(tt.body))
			require.False(t, conflict, got)
			require.Empty(t, got)
		})
	}
}

// 回归：JSON handler 不看 Content-Type，把 JSON 请求体标成 multipart 不能绕过检查。
func TestConflictingFieldIgnoresMultipartLabelOnJSONBody(t *testing.T) {
	body := []byte(`{"model":"claude-3-5-haiku-20241022","messages":[],"model":"claude-opus-4-8"}`)
	for _, contentType := range []string{"multipart/form-data", "multipart/form-data; boundary=zzz", `Multipart/Form-Data; boundary="a"`, "text/plain", ""} {
		got, conflict := ConflictingField(contentType, body)
		require.True(t, conflict, contentType)
		require.Equal(t, "model", got)
	}
}

// 规则和它要防的解析器行为绑在一起：未报告冲突的请求体，gjson（首个、大小写
// 敏感）和 encoding/json 结构体绑定（末值、大小写不敏感）读到的值必须一致。
func TestConflictingFieldMatchesParserDisagreement(t *testing.T) {
	tests := []struct {
		body            string
		conflict        bool
		parsersDisagree bool
	}{
		{body: `{"model":"first","model":"last"}`, conflict: true, parsersDisagree: true},
		{body: `{"model":"first","MODEL":"last"}`, conflict: true, parsersDisagree: true},
		{body: `{"MODEL":"first","model":"last"}`, conflict: true},
		{body: `{"model":"same","Model":"same"}`, conflict: true},
		{body: `{"model":"only","reasoning":{"effort":"low","Effort":"xhigh"}}`, conflict: true},
		{body: `{"model":"only","reasoning":{"effort":"low"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			var decoded struct {
				Model     string `json:"model"`
				Reasoning struct {
					Effort string `json:"effort"`
				} `json:"reasoning"`
			}
			require.NoError(t, json.Unmarshal([]byte(tt.body), &decoded))
			first := gjson.Get(tt.body, "model").String()
			require.Equal(t, tt.parsersDisagree, first != decoded.Model)

			_, conflict := ConflictingField("application/json", []byte(tt.body))
			require.Equal(t, tt.conflict, conflict)
			if !conflict {
				require.Equal(t, first, decoded.Model)
				require.Equal(t, gjson.Get(tt.body, "reasoning.effort").String(), decoded.Reasoning.Effort)
			}
		})
	}
}

func TestConflictingFieldReturnsIndependentCopy(t *testing.T) {
	body := []byte(`{"outer":{"model":"a","model":"b"}}`)
	got, conflict := ConflictingField("application/json", body)
	require.True(t, conflict)
	for i := range body {
		body[i] = 'x'
	}
	require.Equal(t, "outer.model", got)
}

func TestConflictingFieldManyMembersUsesIndex(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"model":"m","tool":{`)
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, `"k%d":%d,`, i, i)
	}
	body := b.String()

	got, conflict := ConflictingField("application/json", []byte(body+`"K3":3}}`))
	require.True(t, conflict, "duplicates past the linear-scan limit must be found")
	require.Equal(t, "tool.K3", got)

	_, conflict = ConflictingField("application/json", []byte(body+`"k100":0}}`))
	require.False(t, conflict)
}

// 回归：扫描是线性的，深嵌套不能放大 CPU（逐层递归时 1 MB、9990 层约 1.6 s）。
func TestConflictingFieldDeepNestingIsLinear(t *testing.T) {
	depth := maxConflictDepth - 10
	body := []byte(`{"input":` + strings.Repeat("[", depth) + `"` + strings.Repeat("a", 1<<20) + `"` + strings.Repeat("]", depth) + `}`)
	start := time.Now()
	_, conflict := ConflictingField("application/json", body)
	require.False(t, conflict)
	require.Less(t, time.Since(start), 200*time.Millisecond)

	objects := []byte(strings.Repeat(`{"a":`, maxConflictDepth+50) + "1" + strings.Repeat("}", maxConflictDepth+50))
	_, conflict = ConflictingField("application/json", objects)
	require.False(t, conflict)
}

// 回归：大对象的判重内存与请求体同一量级（逐键存字符串时峰值约为请求体的 16 倍）。
// 64 位哈希 map 含扩容过程的累计分配约 6～7 倍，峰值约 2～3 倍。
func TestConflictingFieldManyKeysMemoryIsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"x":{`)
	const keys = 1 << 20
	for i := 0; i < keys; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"%d":1`, i)
	}
	b.WriteString(`}}`)
	body := []byte(b.String())

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, conflict := ConflictingField("application/json", body)
	runtime.ReadMemStats(&after)
	require.False(t, conflict)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(8*len(body)))
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
	reject := []struct {
		name   string
		fields [][2]string
		want   string
	}{
		{name: "model 字段取值不同", fields: [][2]string{{"model", "gpt-image-1"}, {"prompt", "cat"}, {"model", "gpt-image-1.5"}}, want: "model"},
		{name: "model 字段取值相同", fields: [][2]string{{"model", "gpt-image-1"}, {"model", "gpt-image-1"}}, want: "model"},
		{name: "计费标量 size 重复", fields: [][2]string{{"model", "gpt-image-1"}, {"size", "1024x1024"}, {"size", "1536x1024"}}, want: "size"},
		{name: "计费标量 n 重复", fields: [][2]string{{"n", "1"}, {"n", "8"}}, want: "n"},
		{name: "session 字段重复", fields: [][2]string{{"sdp", "v=0"}, {"session", `{"model":"a"}`}, {"session", `{"model":"b"}`}}, want: "session"},
		{name: "session JSON 内的 model 重复", fields: [][2]string{{"sdp", "v=0"}, {"session", `{"model":"a","instructions":"hi","model":"b"}`}}, want: "session.model"},
		{name: "session JSON 内的嵌套重复", fields: [][2]string{{"sdp", "v=0"}, {"session", `{"model":"a","audio":{"voice":"x","voice":"y"}}`}}, want: "session.audio.voice"},
	}
	for _, tt := range reject {
		t.Run(tt.name, func(t *testing.T) {
			contentType, body := multipartBody(t, tt.fields...)
			got, conflict := ConflictingField(contentType, body)
			require.True(t, conflict)
			require.Equal(t, tt.want, got)
		})
	}

	accept := []struct {
		name   string
		fields [][2]string
	}{
		{name: "普通表单", fields: [][2]string{{"model", "gpt-image-1"}, {"prompt", `{"model":"a","model":"b"}`}}},
		{name: "数组字段", fields: [][2]string{{"model", "whisper-1"}, {"timestamp_granularities[]", "word"}, {"timestamp_granularities[]", "segment"}}},
		{name: "多图输入", fields: [][2]string{{"model", "grok-imagine"}, {"image_url", "https://a"}, {"image_url", "https://b"}, {"image", "https://c"}, {"image", "https://d"}}},
	}
	for _, tt := range accept {
		t.Run(tt.name, func(t *testing.T) {
			contentType, body := multipartBody(t, tt.fields...)
			got, conflict := ConflictingField(contentType, body)
			require.False(t, conflict, got)
		})
	}
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
