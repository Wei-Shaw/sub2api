package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// legacyParseOpenAIResponsesJSONString is the byte-at-a-time scanner this file
// replaced. It stays here as the differential baseline for parseString.
func legacyParseOpenAIResponsesJSONString(body []byte, pos int) (int, int, string, int) {
	if pos >= len(body) || body[pos] != '"' {
		return 0, 0, "expected string", pos
	}
	start := pos
	pos++
	for pos < len(body) {
		switch body[pos] {
		case '"':
			pos++
			return start, pos, "", 0
		case '\\':
			pos++
			if pos >= len(body) {
				return 0, 0, "unterminated string escape", pos
			}
			switch body[pos] {
			case 'u':
				if pos+4 >= len(body) {
					return 0, 0, "short unicode escape", pos
				}
				for _, digit := range body[pos+1 : pos+5] {
					if !isOpenAIResponsesJSONHexDigit(digit) {
						return 0, 0, "invalid unicode escape", pos
					}
				}
				pos += 5
				continue
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				pos++
			default:
				return 0, 0, "invalid string escape", pos
			}
		default:
			if body[pos] < 0x20 {
				return 0, 0, "control character in string", pos
			}
			pos++
		}
	}
	return 0, 0, "unterminated string", pos
}

type stringScanResult struct {
	start, end int
	err        string
	errPos     int
}

func currentParseOpenAIResponsesJSONString(body []byte, pos int) stringScanResult {
	p := openAIResponsesToolSchemaParser{body: body, pos: pos}
	start, end, err := p.parseString()
	if err != nil {
		message, errPos := splitOpenAIResponsesToolSchemaError(err)
		return stringScanResult{err: message, errPos: errPos}
	}
	return stringScanResult{start: start, end: end}
}

func splitOpenAIResponsesToolSchemaError(err error) (string, int) {
	text := err.Error()
	const prefix = "sanitize OpenAI Responses tool schemas: "
	text = strings.TrimPrefix(text, prefix)
	at := strings.LastIndex(text, " at byte ")
	if at < 0 {
		return text, -1
	}
	var pos int
	if _, scanErr := fmt.Sscanf(text[at+len(" at byte "):], "%d", &pos); scanErr != nil {
		return text, -1
	}
	return text[:at], pos
}

func TestOpenAIResponsesToolSchemaParseStringMatchesLegacy(t *testing.T) {
	escape := string(rune(92))
	cases := []string{
		`""`,
		`"abc"`,
		`"a b c"`,
		`"` + strings.Repeat("x", 64) + `"`,
		`"` + strings.Repeat("x", 63) + `"`, // exercises the 8-byte tail
		`"` + strings.Repeat("x", 65) + `"`,
		`"a\"b"`,
		`"a\\b"`,
		`"a\/b"`,
		`"` + escape + `u00e9"`,
		`"` + escape + `uD83D` + escape + `uDE00"`,
		`"` + escape + `u00E9"`,
		// 控制字符
		"\"a\nb\"",
		"\"a\tb\"",
		"\"a\x00b\"",
		"\"a\x1fb\"",
		"\"a\x7fb\"", // DEL 是合法 JSON
		// 非法转义 / 截断
		`"a\qb"`,
		`"a\u12"`,
		`"a\u12zz"`,
		`"a\u`,
		`"a\`,
		`"unterminated`,
		`"`,
		``,
		`abc`,
		// 多字节 UTF-8
		`"中文 😀"`,
		`"` + strings.Repeat("中", 40) + `"`,
	}

	for _, tc := range cases {
		for _, pos := range []int{0, 1} {
			body := []byte(tc)
			if pos == 1 {
				body = append([]byte("x"), body...)
			}
			wantStart, wantEnd, wantErr, wantErrPos := legacyParseOpenAIResponsesJSONString(body, pos)
			got := currentParseOpenAIResponsesJSONString(body, pos)
			require.Equal(t, stringScanResult{start: wantStart, end: wantEnd, err: wantErr, errPos: wantErrPos}, got,
				"input=%q pos=%d", tc, pos)
		}
	}
}

func FuzzOpenAIResponsesToolSchemaParseString(f *testing.F) {
	f.Add([]byte(`"abc"`), 0)
	f.Add([]byte(`"a\"b"`), 0)
	f.Add([]byte{'"', 'a', '\n', '"'}, 0)
	f.Add([]byte(`"`), 0)
	f.Add([]byte(`x"aé"`), 1)
	f.Fuzz(func(t *testing.T, body []byte, pos int) {
		if pos < 0 || pos > len(body) {
			return
		}
		wantStart, wantEnd, wantErr, wantErrPos := legacyParseOpenAIResponsesJSONString(body, pos)
		got := currentParseOpenAIResponsesJSONString(body, pos)
		require.Equal(t, stringScanResult{start: wantStart, end: wantEnd, err: wantErr, errPos: wantErrPos}, got,
			"body=%q pos=%d", body, pos)
	})
}

// 探测走完的终点必须与「跳过」走完的终点一致，探针结果才能被当作跳过复用。
func TestOpenAIResponsesToolSchemaInputItemSkipMatchesProbe(t *testing.T) {
	values := []string{
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`,
		`{"type":"function","name":"x","parameters":{"type":"object"}}`,
		`{"type":"function_call","arguments":"{}"}`,
		`{"type":"custom","name":"x"}`,
		`{"type":"tool_search"}`,
		`{"type":"namespace","tools":[]}`,
		`{"type":"additional_tools","tools":[{"type":"namespace"}]}`,
		`{"type":"\\u0066unction","parameters":{"type":"null"}}`,
		`{"type": "message" , "content" : [ 1 , 2 , {"a":null} ] }`,
		`{}`,
		`{"a":[{"b":[{"c":"d"}]}]}`,
		`{"type":"message","content":"\\u0000\\u001f"}`,
	}
	for _, value := range values {
		body := []byte(value)
		probe := openAIResponsesToolSchemaParser{body: body, pos: 0}
		require.NoError(t, probe.parseValue(openAIResponsesToolSchemaTypeProbe, false, 0), "value=%s", value)

		skip := openAIResponsesToolSchemaParser{body: body, pos: 0}
		require.NoError(t, skip.parseValue(openAIResponsesToolSchemaSkip, false, 0), "value=%s", value)

		require.Equal(t, skip.pos, probe.pos, "value=%s", value)
		require.Equal(t, len(body), probe.pos, "value=%s", value)
	}
}

// 解析器接受的 body 必须也是 encoding/json 认可的有效 JSON——否则去掉改写前的
// json.Valid 预检查就会放过畸形请求。
func TestOpenAIResponsesToolSchemaParserAcceptsOnlyValidJSON(t *testing.T) {
	corpus := append(openAIResponsesToolSchemaGateCorpus(), []string{
		`{}`, `[]`, `null`, `0`, `""`, `{"a":1}`, `[1,2,3]`,
	}...)
	for _, body := range corpus {
		p := openAIResponsesToolSchemaParser{body: []byte(body)}
		err := p.parseValue(openAIResponsesToolSchemaDocument, false, 0)
		if err != nil {
			continue
		}
		p.skipWhitespace()
		if p.pos != len(body) {
			continue
		}
		require.True(t, json.Valid([]byte(body)), "解析器接受了 encoding/json 拒绝的输入: %q", body)
	}
}

func FuzzOpenAIResponsesToolSchemaParserAcceptsOnlyValidJSON(f *testing.F) {
	for _, seed := range []string{
		`{"tools":[{"type":"function","parameters":{"type":"null"}}]}`,
		`{"a":1,"b":[true,null,"x"]}`,
		`0{"input"[{ "namespace"00`,
		`{"a":01}`,
		`{"a":"\u12zz"}`,
		`{"a":1} trailing`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		if !utf8.Valid(body) {
			return // 入口先做 utf8 校验，这里只看解析器本身
		}
		p := openAIResponsesToolSchemaParser{body: body}
		if err := p.parseValue(openAIResponsesToolSchemaDocument, false, 0); err != nil {
			return
		}
		p.skipWhitespace()
		if p.pos != len(body) {
			return
		}
		require.True(t, json.Valid(body), "解析器接受了 encoding/json 拒绝的输入: %q", body)
	})
}

// openAIResponsesToolSchemaGateCorpus 覆盖两个门控必须放行的每一种写法。
func openAIResponsesToolSchemaGateCorpus() []string {
	escape := string(rune(92))
	pattern := func(p string) string {
		return `{"tools":[{"type":"function","name":"f","parameters":{"type":"object","properties":{"q":{"type":"string","pattern":"` + p + `"}}}}]}`
	}
	return []string{
		// —— 非法正则 ——
		pattern(`(?=x)`),
		pattern(`(?!x)`),
		pattern(`(?<=x)`),
		pattern(`(?<!x)`),
		pattern(`(?<name>x)`),
		pattern(`(?'name'x)`),
		pattern(`a` + escape + escape + `p{L}`),
		pattern(`a` + escape + escape + `R`),
		pattern(`a` + escape + `u005cp`),
		pattern(`a` + escape + `u0028` + escape + `u003f=x)`),
		pattern(`a` + escape + `u0028?=x)`),
		pattern(`a(` + escape + `u003f=x)`),
		pattern(`a` + escape + escape + escape + escape + `d`),
		// patternProperties 的键
		`{"tools":[{"type":"function","parameters":{"type":"object","patternProperties":{"(?=x)":{"type":"string"}}}}]}`,
		`{"tools":[{"type":"function","parameters":{"type":"object","patternProperties":{"` + escape + `u0028` + escape + `u003f=x)":{"type":"string"}}}}]}`,
		// —— 参数类型修复 ——
		`{"tools":[{"type":"function","parameters":{"type":null}}]}`,
		`{"tools":[{"type":"function","parameters":{"required":null}}]}`,
		`{"tools":[{"type":"function","parameters":{"oneOf":[{"type":"object"},{"type":"object"}]}}]}`,
		`{"tools":[{"type":"function","parameters":{"anyOf":[{"properties":{}}]}}]}`,
		// union 关键字用 \u004f / \u0066 转义写出：字面 needle 不命中，但
		// openAIMissingRootObjectUnionTypeEdit 会因为 raw 里有反斜杠而继续解析。
		`{"tools":[{"type":"function","name":"other","parameters":{"one` + escape + `u004ff":[{"type":"object"}]}}]}`,
		`{"tools":[{"type":"function","name":"other","parameters":{"anyO` + escape + `u0066":[{"type":"object"}]}}]}`,
		`{"messages":[{"role":"user","content":"x","input_schema":{"type":null}}]}`,
		// —— 合法正则不应触发修复，但门控允许放行 ——
		pattern(`^a+$`),
		pattern(`a` + escape + escape + `d{3}`),
		`{"input":[{"type":"message","content":"a null mention (and parens)"}]}`,
	}
}

// 门控必须是「会改写」的必要条件：只要消毒真的改了 body，门控就必须放行。
// 反向不成立——门控故意留了余量（例如 body 里出现任何 \\ 就放行），因为算出
// 精确条件比放行一两次解析更贵也更易错。
func requireOpenAIResponsesToolSchemaPatternGateCovers(t *testing.T, body []byte) {
	t.Helper()
	_, changed, err := sanitizeOpenAIResponsesToolSchemas(body,
		openAIResponsesToolSchemaOptions{removeUnsupportedRegexPatterns: true})
	if err != nil {
		return // 解析失败的 body 不会被改写，错误本身交给既有用例覆盖
	}
	if changed {
		require.True(t, openAIResponsesBodyMayContainUnsupportedRegex(body),
			"改写已发生但正则门控放过了: %q", body)
	}
}

// 类型修复原本没有门控，同样用绝对不变量：只要真的改了 body，门控就必须放行。
func requireOpenAIResponsesToolParameterGateCovers(t *testing.T, body []byte) {
	t.Helper()
	_, changed, err := sanitizeOpenAIResponsesToolSchemas(body, openAIResponsesToolSchemaOptions{
		replaceNullParameterTypes:       true,
		injectObjectUnionRootObjectType: true,
		dropNullRequired:                true,
	})
	if err != nil {
		return
	}
	if changed {
		require.True(t, openAIResponsesBodyMayNeedToolParameterRepair(body),
			"改写已发生但类型门控放过了: %q", body)
	}
}

// 门控必须真的能挡住普通请求，否则上面那条不变量会被「永远返回 true」蒙混过关。
func TestOpenAIResponsesToolSchemaGatesSkipOrdinaryTraffic(t *testing.T) {
	clean := buildToolSchemaBenchmarkBody(1<<20, 30, false)
	require.False(t, openAIResponsesBodyMayContainUnsupportedRegex(clean),
		"普通的 1 MiB 请求不该进入正则 pattern 消毒")
	require.False(t, openAIResponsesBodyMayNeedToolParameterRepair(clean),
		"普通的 1 MiB 请求不该进入参数类型修复")
}

func TestOpenAIResponsesToolSchemaGatesAreNecessary(t *testing.T) {
	for _, body := range openAIResponsesToolSchemaGateCorpus() {
		raw := []byte(body)
		requireOpenAIResponsesToolSchemaPatternGateCovers(t, raw)
		requireOpenAIResponsesToolParameterGateCovers(t, raw)
	}
}

func FuzzOpenAIResponsesToolSchemaGatesAreNecessary(f *testing.F) {
	for _, seed := range openAIResponsesToolSchemaGateCorpus() {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) == 0 {
			return
		}
		requireOpenAIResponsesToolSchemaPatternGateCovers(t, body)
		requireOpenAIResponsesToolParameterGateCovers(t, body)
	})
}
