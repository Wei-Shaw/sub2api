package service

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本文件覆盖「上游文本里的非法 UTF-8 让整批历史点静默丢失」这条链路的两个源头：
//  1. 上游响应体本身被截断/污染（sanitizeErrorMessage 负责归一化）
//  2. 我们自己按字节硬切超长文本，把多字节字符切成半个（truncateMessage 负责回退到字符边界）
//
// 两者都必须保证「送进 PostgreSQL text 列的字节一定合法」，否则 Postgres 会拒收整批 INSERT。

// invalidUTF8Samples 覆盖几种真实会遇到的非法 UTF-8 形态。
func invalidUTF8Samples() map[string]string {
	truncated := "中文错误"
	return map[string]string{
		// 孤立的三字节前导字节（历史事故里实际观测到的 0xe3）
		"lone_lead_byte": "upstream error \xe3..",
		// 被截断的多字节字符（"中文错误" 的前两个字节）
		"truncated_runewidth": "body=" + truncated[:2],
		// Latin-1 错误页里常见的裸高字节
		"latin1_page": "<html>caf\xe9 not found</html>",
		// 半截 gzip / 二进制片段
		"binary_fragment": "\x1f\x8b\x08\x00\x00\x00",
	}
}

func TestSanitizeErrorMessage_InvalidUTF8BecomesValid(t *testing.T) {
	for name, input := range invalidUTF8Samples() {
		t.Run(name, func(t *testing.T) {
			require.False(t, utf8.ValidString(input), "前置条件：样本本身必须是非法 UTF-8")

			got := sanitizeErrorMessage(input)

			assert.True(t, utf8.ValidString(got), "清洗后必须是合法 UTF-8，否则 Postgres 会拒收整批 insert")
			assert.Contains(t, got, monitorUTF8Replacement, "非法字节应被替换为 U+FFFD")
		})
	}
}

func TestSanitizeErrorMessage_KeepsValidTextUntouched(t *testing.T) {
	input := "upstream HTTP 503: 服务暂时不可用，请稍后重试"
	assert.Equal(t, input, sanitizeErrorMessage(input))
	assert.Equal(t, "", sanitizeErrorMessage(""))
}

func TestSanitizeErrorMessage_StillRedactsSecrets(t *testing.T) {
	cases := map[string]struct{ in, want, notWant string }{
		"openai_key": {
			in:      "unauthorized: sk-abcdefghijklmnopqrstuvwxyz0123456789",
			want:    "sk-***REDACTED***",
			notWant: "abcdefghijklmnopqrstuvwxyz",
		},
		"anthropic_key_wins_over_generic_sk": {
			in:      "invalid key sk-ant-abcdefghijklmnopqrstuvwxyz",
			want:    "sk-ant-***REDACTED***",
			notWant: "abcdefghijklmnopqrstuvwxyz",
		},
		"xai_key": {
			in:      "bad xai-abcdef123456",
			want:    "xai-***REDACTED***",
			notWant: "abcdef123456",
		},
		"gemini_key": {
			in:      "AIzaSyA1234567890abcdefghijklmnopqrstuv",
			want:    "AIza***REDACTED***",
			notWant: "SyA1234567890",
		},
		"query_param_key": {
			in:      `Get "https://api.example.com/v1?key=supersecret&x=1": dial tcp: timeout`,
			want:    "key=REDACTED",
			notWant: "supersecret",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := sanitizeErrorMessage(tc.in)
			assert.Contains(t, got, tc.want)
			assert.NotContains(t, got, tc.notWant)
		})
	}
}

func TestSanitizeErrorMessage_NormalizesBeforeRedacting(t *testing.T) {
	// 归一化必须排在脱敏之前：否则带非法字节的文本会先被正则处理，
	// 得到一个"合法但漏了密钥"或"密钥被脱敏但仍非法"的半成品。
	got := sanitizeErrorMessage("sk-abcdefghijklmnopqrstuvwxyz0123456789 \xe3..")

	assert.True(t, utf8.ValidString(got), "同时含密钥与非法字节时，两者都要被处理")
	assert.Contains(t, got, "sk-***REDACTED***")
	assert.NotContains(t, got, "abcdefghijklmnopqrstuvwxyz")
}

func TestTruncateMessage_ShortTextUnchanged(t *testing.T) {
	for _, in := range []string{"", "ok", "challenge passed", strings.Repeat("a", monitorMessageMaxBytes)} {
		assert.Equal(t, in, truncateMessage(in))
	}
}

func TestTruncateMessage_NeverSplitsRune(t *testing.T) {
	// "测试失败" 是 4 个字符 12 字节。若只用纯 3 字节字符重复，486（上限 500 减去截断标记 14）
	// 恰好落在字符边界上（486 = 40*12 + 6），根本切不坏 —— 所以先加 1 个 ASCII 字节前缀
	// 把截断点推到一个多字节字符的中间，这正是旧实现会产出非法 UTF-8 的位置。
	long := "x" + strings.Repeat("测试失败", 200)
	require.Greater(t, len(long), monitorMessageMaxBytes, "前置条件：样本必须超长")

	rawCutoff := monitorMessageMaxBytes - len("...(truncated)")
	require.False(t, utf8.ValidString(long[:rawCutoff]),
		"前置条件：朴素的按字节切必须真的会切坏字符，否则这个用例证明不了任何事")

	got := truncateMessage(long)

	assert.True(t, utf8.ValidString(got), "截断结果必须仍然是合法 UTF-8")
	assert.LessOrEqual(t, len(got), monitorMessageMaxBytes, "必须尊重列宽上限")
	assert.True(t, strings.HasSuffix(got, "...(truncated)"), "应带截断标记")
	assert.True(t, strings.HasPrefix(got, "x测试失败"), "应保留前缀内容")
}

func TestTruncateMessage_EveryCutoffPointStaysValid(t *testing.T) {
	// 把上限临时压到多字节字符的每一个落点上，确认任意切点都不会产出半个字符。
	// 直接遍历 3 字节字符的三种偏移，等价于覆盖 all rune-boundary 情况。
	for _, filler := range []string{"a", "ab", "abc"} {
		t.Run("offset_"+filler, func(t *testing.T) {
			// 前缀用 ASCII 撑出偏移，后面全是 3 字节中文字符。
			long := filler + strings.Repeat("错", 300)
			require.Greater(t, len(long), monitorMessageMaxBytes)

			got := truncateMessage(long)

			assert.True(t, utf8.ValidString(got))
			assert.LessOrEqual(t, len(got), monitorMessageMaxBytes)
		})
	}
}

func TestSanitizeThenTruncate_ProductionPipelineStaysValid(t *testing.T) {
	// 生产链路就是「先 sanitizeErrorMessage 再 truncateMessage」，
	// 这里用「非法字节 + 超长中文」的组合跑一遍完整链路。
	upstream := "上游返回 " + strings.Repeat("失败 ", 300) + " \xe3.. tail"

	got := truncateMessage(sanitizeErrorMessage(upstream))

	assert.True(t, utf8.ValidString(got), "链路末端必须合法，这样才不会触发 pq: invalid byte sequence")
	assert.LessOrEqual(t, len(got), monitorMessageMaxBytes)
}
