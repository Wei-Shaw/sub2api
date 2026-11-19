package requestmodel

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
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
		{name: "只有嵌套对象里有不一致的重复键", body: `{"model":"a","metadata":{"user_id":"1","user_id":"2"}}`},
		{name: "数组元素里的重复键", body: `{"messages":[{"role":"user","role":"assistant"}]}`},
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

func TestHasConflictingModels(t *testing.T) {
	require.False(t, HasConflictingModels(nil))
	require.False(t, HasConflictingModels([]string{"a"}))
	require.False(t, HasConflictingModels([]string{"a", "a", "a"}))
	require.True(t, HasConflictingModels([]string{"a", "b"}))
	require.True(t, HasConflictingModels([]string{"a", "a", "b"}))
}
