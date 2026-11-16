package httputil

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"net/http"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// legacyNormalizeLenientJSONRequestBody 是加入快速路径之前的实现，作为差分基准。
// 它总会把整个 body 走一遍逐字节状态机。
func legacyNormalizeLenientJSONRequestBody(body []byte, maxNormalizedBytes int64) ([]byte, error) {
	if maxNormalizedBytes <= 0 {
		maxNormalizedBytes = maxDecompressedBodySize
	}
	body = trimUTF8BOM(body)
	if len(body) == 0 {
		return body, nil
	}
	if int64(len(body)) > maxNormalizedBytes {
		return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
	}

	normalizedSize := int64(len(body))
	inString := false
	escaped := false
	for _, b := range body {
		if inString && isJSONControlByte(b) {
			if maxNormalizedBytes-normalizedSize < 5 {
				return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
			}
			normalizedSize += 5
			escaped = false
			continue
		}
		switch {
		case escaped:
			escaped = false
		case inString && b == '\\':
			escaped = true
		case b == '"':
			inString = !inString
		}
	}
	if normalizedSize == int64(len(body)) {
		return body, nil
	}
	out := make([]byte, 0, int(normalizedSize))
	inString, escaped = false, false
	for _, b := range body {
		if inString && isJSONControlByte(b) {
			out = appendJSONUnicodeEscape(out, b)
			escaped = false
			continue
		}
		switch {
		case escaped:
			escaped = false
		case inString && b == '\\':
			escaped = true
		case b == '"':
			inString = !inString
		}
		out = append(out, b)
	}
	return out, nil
}

func normalizeForTest(t *testing.T, body []byte, limit int64) ([]byte, bool, error) {
	t.Helper()
	out, err := normalizeLenientJSONRequestBody(context.Background(), body, limit)
	return out.data, out.jsonValid, err
}

// 快速路径不得改变任何输入下的输出字节。
func TestNormalizeLenientJSONRequestBodyMatchesLegacy(t *testing.T) {
	escape := string(rune(92))
	cases := []string{
		``, `{}`, `[]`, `null`, `0`, `"x"`,
		`{"a":1}`,
		`{"a":"plain"}`,
		"{\"a\":\"with \\\"escaped\"}",
		`{"a":"` + escape + `n"}`,
		// 需要改写的裸控制字节
		"{\"a\":\"x\ny\"}",
		"{\"a\":\"x\ty\"}",
		"{\"a\":\"x\x00y\"}",
		"{\"a\":\"x\x1fy\"}",
		// 0x7f：合法 JSON 但会被改写
		"{\"a\":\"x\x7fy\"}",
		"{\"a\":\"x\x7fy\",\"b\":\"z\"}",
		// 字符串外的控制字节：不改写
		"{\"a\":1,\n\"b\":2}",
		"{\n\t\"a\" : 1\n}",
		// 转义状态
		"{\"a\":\"\\\\\"}",      // JSON 字符串里是 "\"
		"{\"a\":\"\\\\\\\"x\"}", // JSON 字符串里是 "\\\"x"
		// 不合法结构
		`{"a":}`, `{"a":1`, `[1,2,`, `{"a":"unterminated`,
		// 多字节
		`{"a":"中文 😀"}`,
		// BOM
		"\xef\xbb\xbf{\"a\":1}",
		"\xef\xbb\xbf{\"a\":\"x\ny\"}",
	}

	for _, tc := range cases {
		body := []byte(tc)
		legacyOut, legacyErr := legacyNormalizeLenientJSONRequestBody(append([]byte(nil), body...), 1)
		if legacyErr != nil {
			// 上限 1 会命中，换个大限额重跑
			legacyOut, legacyErr = legacyNormalizeLenientJSONRequestBody(append([]byte(nil), body...), 0)
			gotOut, _, gotErr := normalizeForTest(t, append([]byte(nil), body...), 0)
			require.Equal(t, legacyErr, gotErr, "input=%q", tc)
			require.Equal(t, string(legacyOut), string(gotOut), "input=%q", tc)
			continue
		}
		gotOut, _, gotErr := normalizeForTest(t, append([]byte(nil), body...), 1)
		require.NoError(t, gotErr, "input=%q", tc)
		require.Equal(t, string(legacyOut), string(gotOut), "input=%q", tc)
	}
}

// jsonValid=true 时必须真的合法，调用方才会跳过自己的校验。
func TestNormalizeLenientJSONRequestBodyValidHintIsSound(t *testing.T) {
	corpus := []string{
		`{}`, `[]`, `null`, `0`, `"x"`, `{"a":1}`, `{"a":{"b":[1,2,{"c":"d"}]}}`,
		`{"a":"中文 😀"}`,
		`{"a":"` + string(rune(92)) + `u00e9"}`,
		"\xef\xbb\xbf{\"a\":1}",
		`{"a":1} `,
		// 不该被标记为已校验
		"{\"a\":\"x\ny\"}",
		"{\"a\":\"x\x7fy\"}",
		`{"a":}`, `{"a":1`, ``,
		strings.Repeat("[", 10001) + strings.Repeat("]", 10001),
	}

	for _, tc := range corpus {
		body := []byte(tc)
		out, jsonValid, err := normalizeForTest(t, append([]byte(nil), body...), 0)
		require.NoError(t, err, "input=%q", truncateForTest(tc))
		if !jsonValid {
			continue
		}
		require.True(t, json.Valid(out), "jsonValid 但 encoding/json 不认: %q", truncateForTest(tc))
		// handler 用的是 gjson.ValidBytes，跳过它的前提是它也认。
		require.True(t, gjson.ValidBytes(out), "jsonValid 但 gjson 不认: %q", truncateForTest(tc))
	}
}

func TestNormalizeLenientJSONRequestBodyEscapesDel(t *testing.T) {
	body := append(append([]byte(`{"a":"x`), 0x7f), []byte(`y"}`)...)
	out, jsonValid, err := normalizeForTest(t, body, 0)
	require.NoError(t, err)
	require.Equal(t, `{"a":"x\u007fy"}`, string(out), "0x7f 必须继续被转义")
	require.False(t, jsonValid, "改写过的 body 交回调用方重新校验即可")
}

func truncateForTest(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

func FuzzNormalizeLenientJSONRequestBody(f *testing.F) {
	f.Add([]byte(`{"a":"x"}`))
	f.Add([]byte("{\"a\":\"x\ny\"}"))
	f.Add([]byte("{\"a\":\"x\x7fy\"}"))
	f.Add([]byte("\xef\xbb\xbf{}"))
	f.Add([]byte(`{"a":"\\"}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		want, wantErr := legacyNormalizeLenientJSONRequestBody(append([]byte(nil), body...), 0)
		got, jsonValid, gotErr := normalizeForTest(t, append([]byte(nil), body...), 0)
		require.Equal(t, fmt.Sprint(wantErr), fmt.Sprint(gotErr), "body=%q", body)
		if wantErr != nil {
			return
		}
		require.Equal(t, string(want), string(got), "body=%q", body)
		if jsonValid {
			require.True(t, json.Valid(got), "jsonValid 但 encoding/json 不认: %q", body)
			require.True(t, gjson.ValidBytes(got), "jsonValid 但 gjson 不认: %q", body)
		}
	})
}

func BenchmarkNormalizeLenientJSONRequestBody(b *testing.B) {
	body := buildLenientBenchmarkBody(512 << 10)
	b.Run("legacy", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		for range b.N {
			_, _ = legacyNormalizeLenientJSONRequestBody(body, 0)
		}
	})
	b.Run("current", func(b *testing.B) {
		b.SetBytes(int64(len(body)))
		for range b.N {
			_, _ = normalizeLenientJSONRequestBody(context.Background(), body, 0)
		}
	})
	b.Run("escape_path_legacy", func(b *testing.B) {
		withControl := append([]byte(nil), body...)
		withControl[len(withControl)/2] = '\n'
		b.SetBytes(int64(len(withControl)))
		for range b.N {
			_, _ = legacyNormalizeLenientJSONRequestBody(withControl, 0)
		}
	})
	b.Run("escape_path", func(b *testing.B) {
		withControl := append([]byte(nil), body...)
		withControl[len(withControl)/2] = '\n'
		b.SetBytes(int64(len(withControl)))
		for range b.N {
			_, _ = normalizeLenientJSONRequestBody(context.Background(), withControl, 0)
		}
	})
}

func buildLenientBenchmarkBody(targetBytes int) []byte {
	var sb strings.Builder
	sb.Grow(targetBytes + 1024)
	write := func(s string) { _, _ = sb.WriteString(s) }
	write(`{"model":"gpt-5.6","input":[`)
	item := `{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("x", 4000) + `"}]}`
	for i := 0; sb.Len() < targetBytes; i++ {
		if i > 0 {
			write(",")
		}
		write(item)
	}
	write(`]}`)
	return []byte(sb.String())
}
