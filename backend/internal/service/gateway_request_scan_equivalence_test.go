package service

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"unsafe"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 下面两个函数是重构前的 parseGatewayRequestCurrentBody / setGatewayRequestRanges
// 的逐字副本，作为差分测试的基准。任何一处语义变化都会在下面的对比里暴露。
func legacyParseGatewayRequestCurrentBody(parsed *ParsedRequest, protocol string) error {
	if parsed == nil || parsed.Body == nil {
		return fmt.Errorf("empty request body")
	}
	parsed.Body.parsed = nil

	bodyBytes := parsed.Body.Bytes()
	if !gjson.ValidBytes(bodyBytes) {
		return DescribeInvalidJSON(bodyBytes)
	}

	jsonStr := *(*string)(unsafe.Pointer(&bodyBytes))
	clearGatewayRequestDerivedState(parsed)
	parsed.protocol = protocol

	modelResult := gjson.Get(jsonStr, "model")
	if modelResult.Exists() {
		if modelResult.Type != gjson.String {
			return fmt.Errorf("invalid model field type")
		}
		parsed.Model = strings.Clone(modelResult.String())
		if protocol == domain.PlatformAnthropic {
			normalizedModel := normalizeClaudeCodeLongContextModel(parsed.Model)
			if normalizedModel != parsed.Model {
				normalizedBody, err := sjson.SetBytes(bodyBytes, "model", normalizedModel)
				if err != nil {
					return fmt.Errorf("normalize model field: %w", err)
				}
				parsed.Body.Replace(normalizedBody)
				bodyBytes = normalizedBody
				jsonStr = *(*string)(unsafe.Pointer(&bodyBytes))
				parsed.Model = normalizedModel
			}
		}
	}

	streamResult := gjson.Get(jsonStr, "stream")
	if streamResult.Exists() {
		if streamResult.Type != gjson.True && streamResult.Type != gjson.False {
			return fmt.Errorf("invalid stream field type")
		}
		parsed.Stream = streamResult.Bool()
	}

	parsed.MetadataUserID = strings.Clone(gjson.Get(jsonStr, "metadata.user_id").String())

	thinkingType := gjson.Get(jsonStr, "thinking.type").String()
	parsed.ThinkingEnabled = thinkingType == "enabled" || thinkingType == "adaptive" || thinkingType == "between_tools" ||
		(protocol == domain.PlatformAnthropic && isClaude55SignedThinkingModel(parsed.Model))

	parsed.OutputEffort = strings.Clone(strings.TrimSpace(gjson.Get(jsonStr, "output_config.effort").String()))
	if protocol == domain.PlatformAnthropic {
		parsed.Speed = strings.Clone(strings.ToLower(strings.TrimSpace(gjson.Get(jsonStr, "speed").String())))
	}

	maxTokensResult := gjson.Get(jsonStr, "max_tokens")
	if maxTokensResult.Exists() && maxTokensResult.Type == gjson.Number {
		f := maxTokensResult.Float()
		if !math.IsNaN(f) && !math.IsInf(f, 0) && f == math.Trunc(f) &&
			f <= float64(math.MaxInt) && f >= float64(math.MinInt) {
			parsed.MaxTokens = int(f)
		}
	}

	legacySetGatewayRequestRanges(parsed, protocol, jsonStr)
	parsed.saveBodyState()
	return nil
}

func legacySetGatewayRequestRanges(parsed *ParsedRequest, protocol string, jsonStr string) {
	if parsed == nil {
		return
	}
	switch protocol {
	case domain.PlatformGemini:
		if sysParts := gjson.Get(jsonStr, "systemInstruction.parts"); sysParts.Exists() && sysParts.IsArray() {
			parsed.systemRange = rangeFromResult(sysParts)
		}
		if contents := gjson.Get(jsonStr, "contents"); contents.Exists() && contents.IsArray() {
			parsed.messagesRange = rangeFromResult(contents)
		}
	default:
		if sys := gjson.Get(jsonStr, "system"); sys.Exists() {
			parsed.HasSystem = true
			parsed.systemRange = rangeFromResult(sys)
		}
		if msgs := gjson.Get(jsonStr, "messages"); msgs.Exists() && msgs.IsArray() {
			parsed.messagesRange = rangeFromResult(msgs)
		}
		if protocol == "responses" {
			if input := gjson.Get(jsonStr, "input"); input.Exists() {
				parsed.inputRange = rangeFromResult(input)
			}
		}
	}
}

// parsedSnapshot 只记录解析产出的可观测结果，避免比较实现内部的缓存指针。
type parsedSnapshot struct {
	err            string
	model          string
	stream         bool
	metadataUserID string
	thinkingOn     bool
	outputEffort   string
	speed          string
	maxTokens      int
	hasSystem      bool
	systemRange    jsonRange
	messagesRange  jsonRange
	inputRange     jsonRange
	protocol       string
	bodyAfter      string
}

func snapshotOf(parsed *ParsedRequest, err error) parsedSnapshot {
	snap := parsedSnapshot{}
	if err != nil {
		snap.err = err.Error()
	}
	if parsed == nil {
		return snap
	}
	snap.model = parsed.Model
	snap.stream = parsed.Stream
	snap.metadataUserID = parsed.MetadataUserID
	snap.thinkingOn = parsed.ThinkingEnabled
	snap.outputEffort = parsed.OutputEffort
	snap.speed = parsed.Speed
	snap.maxTokens = parsed.MaxTokens
	snap.hasSystem = parsed.HasSystem
	snap.systemRange = parsed.systemRange
	snap.messagesRange = parsed.messagesRange
	snap.inputRange = parsed.inputRange
	snap.protocol = parsed.protocol
	if parsed.Body != nil {
		snap.bodyAfter = string(parsed.Body.Bytes())
	}
	return snap
}

func parseBoth(t *testing.T, protocol string, body []byte) (parsedSnapshot, parsedSnapshot) {
	t.Helper()
	legacyBody := NewRequestBodyRef(append([]byte(nil), body...))
	legacyParsed := &ParsedRequest{Body: legacyBody}
	legacyErr := legacyParseGatewayRequestCurrentBody(legacyParsed, protocol)

	// 直接调被重构的这一步：ParseGatewayRequest 在失败时返回 nil，无法比较错误路径上的中间状态。
	newBody := NewRequestBodyRef(append([]byte(nil), body...))
	newParsed := &ParsedRequest{Body: newBody}
	newErr := parseGatewayRequestBody(newParsed, protocol, true)

	return snapshotOf(legacyParsed, legacyErr), snapshotOf(newParsed, newErr)
}

func TestParseGatewayRequestMatchesLegacy(t *testing.T) {
	escape := string(rune(92))
	cases := []struct {
		name string
		body string
	}{
		{"typical responses", `{"model":"gpt-5.5","stream":true,"input":[{"type":"message"}]}`},
		{"typical anthropic", `{"model":"claude-sonnet-5-5","stream":true,"system":"s","messages":[{"role":"user","content":"hi"}],"max_tokens":1024,"thinking":{"type":"enabled"},"output_config":{"effort":" high "},"speed":"Fast"}`},
		{"typical gemini", `{"contents":[{"parts":[{"text":"hi"}]}],"systemInstruction":{"parts":[{"text":"sys"}]},"generationConfig":{"maxOutputTokens":32}}`},
		{"empty object", `{}`},
		{"empty string", ``},
		{"invalid json", `{"model":`},
		{"invalid json with image tool", `{"tools":[{"type":"image_generation"}]`},
		{"model wrong type", `{"model":123}`},
		{"model null", `{"model":null}`},
		{"stream wrong type", `{"model":"m","stream":"yes"}`},
		{"stream null", `{"model":"m","stream":null}`},
		{"stream false", `{"model":"m","stream":false}`},
		{"max_tokens float", `{"model":"m","max_tokens":1024.5}`},
		{"max_tokens negative", `{"model":"m","max_tokens":-8}`},
		{"max_tokens huge", `{"model":"m","max_tokens":1e30}`},
		{"max_tokens string", `{"model":"m","max_tokens":"1024"}`},
		{"metadata nested", `{"metadata":{"user_id":"u1"}}`},
		{"metadata string first", `{"metadata":"x","metadata":{"user_id":"u2"}}`},
		{"metadata null child", `{"metadata":{"user_id":null},"metadata":{"user_id":"u3"}}`},
		{"metadata wrong shape", `{"metadata":[],"metadata":{"user_id":"u4"}}`},
		{"thinking string first", `{"thinking":"x","thinking":{"type":"enabled"}}`},
		{"thinking adaptive", `{"thinking":{"type":"adaptive"}}`},
		{"thinking between tools", `{"thinking":{"type":"between_tools"}}`},
		{"effort with spaces", `{"output_config":{"effort":"  medium  "}}`},
		{"effort first empty", `{"output_config":{"effort":""},"output_config":{"effort":"high"}}`},
		{"speed uppercase", `{"speed":" FAST "}`},
		{"system string", `{"system":"sys","messages":[{"role":"user"}]}`},
		{"system object", `{"system":{"type":"text"},"messages":[]}`},
		{"system wrong shape", `{"system":123}`},
		{"messages not array", `{"messages":"x"}`},
		{"input present", `{"input":[{"a":1}]}`},
		{"contents not array", `{"contents":"x"}`},
		{"gemini parts not array", `{"systemInstruction":{"parts":"x"}}`},
		{"gemini parts retry", `{"systemInstruction":{"parts":"x"},"systemInstruction":{"parts":[{"a":1}]}}`},
		{"duplicate model", `{"model":"m1","model":"m2"}`},
		{"duplicate stream", `{"stream":true,"stream":false}`},
		{"long context suffix", `{"model":"claude-sonnet-5-5[1m]","messages":[]}`},
		{"long context suffix duplicated", `{"model":"claude-sonnet-5-5[1m][1m]","messages":[]}`},
		{"escaped system key", `{"` + escape + `u0073ystem":"s","messages":[]}`},
		{"escaped model key", `{"` + escape + `u006dodel":"m"}`},
		{"root array", `[1,2,3]`},
		{"root scalar", `"just a string"`},
		{"nested nulls", `{"model":"m","system":null,"messages":[{"a":1}],"input":null}`},
		{"unicode content", `{"model":"m","messages":[{"content":"中文 😀"}]}`},
	}

	protocols := []string{"responses", "chat_completions", domain.PlatformAnthropic, domain.PlatformGemini}

	for _, tc := range cases {
		for _, protocol := range protocols {
			t.Run(tc.name+"/"+protocol, func(t *testing.T) {
				want, got := parseBoth(t, protocol, []byte(tc.body))
				require.Equal(t, want, got, "body=%s", tc.body)
			})
		}
	}
}

// deterministic 随机组合，覆盖键顺序、重复键、类型混杂的组合空间。
func TestParseGatewayRequestMatchesLegacyRandomized(t *testing.T) {
	pieces := []string{
		`"model":"m"`, `"model":"claude-x[1m]"`, `"model":null`, `"model":1`,
		`"stream":true`, `"stream":"x"`, `"stream":null`,
		`"max_tokens":16`, `"max_tokens":16.5`, `"max_tokens":"16"`,
		`"metadata":{"user_id":"u"}`, `"metadata":"s"`, `"metadata":null`,
		`"thinking":{"type":"enabled"}`, `"thinking":{"type":"adaptive"}`, `"thinking":"x"`,
		`"output_config":{"effort":" low "}`, `"output_config":null`,
		`"speed":" FAST "`, `"speed":null`,
		`"system":"sys"`, `"system":[{"type":"text"}]`, `"system":null`,
		`"messages":[]`, `"messages":[{"role":"user"}]`, `"messages":"x"`,
		`"input":[]`, `"input":[{"a":1}]`, `"input":null`,
		`"contents":[]`, `"contents":"x"`,
		`"systemInstruction":{"parts":[]}`, `"systemInstruction":{"parts":"x"}`,
		`"unknown":1`, `"unknown":{"nested":true}`,
	}
	protocols := []string{"responses", "chat_completions", domain.PlatformAnthropic, domain.PlatformGemini}

	seed := uint64(0x9e3779b97f4a7c15)
	next := func() uint64 {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return seed
	}

	for i := 0; i < 3000; i++ {
		n := int(next()%6) + 1
		parts := make([]string, 0, n)
		for j := 0; j < n; j++ {
			parts = append(parts, pieces[int(next()%uint64(len(pieces)))])
		}
		body := []byte("{" + strings.Join(parts, ",") + "}")
		protocol := protocols[int(next()%uint64(len(protocols)))]

		want, got := parseBoth(t, protocol, body)
		require.Equal(t, want, got, "iteration=%d protocol=%s body=%s", i, protocol, body)
	}
}

// 已校验变体只允许在「body 合法」的前提下使用；合法 JSON 上两者必须产出完全相同的结果。
func TestParseGatewayRequestValidatedMatchesValidatingPath(t *testing.T) {
	bodies := []string{
		`{"model":"gpt-5.5","stream":true,"input":[{"type":"message"}]}`,
		`{"model":"claude-x[1m]","messages":[],"system":"s","max_tokens":8}`,
		`{"metadata":{"user_id":"u"},"thinking":{"type":"enabled"},"output_config":{"effort":" high "}}`,
		`{"contents":[],"systemInstruction":{"parts":[]}}`,
		`{}`,
	}
	protocols := []string{"responses", "chat_completions", domain.PlatformAnthropic, domain.PlatformGemini}
	for _, body := range bodies {
		for _, protocol := range protocols {
			validating := &ParsedRequest{Body: NewRequestBodyRef([]byte(body))}
			validatingErr := parseGatewayRequestBody(validating, protocol, true)

			trusted := &ParsedRequest{Body: NewRequestBodyRef([]byte(body))}
			trustedErr := parseGatewayRequestBody(trusted, protocol, false)

			require.Equal(t, snapshotOf(validating, validatingErr), snapshotOf(trusted, trustedErr),
				"body=%s protocol=%s", body, protocol)
		}
	}
}

func FuzzParseGatewayRequestMatchesLegacy(f *testing.F) {
	f.Add([]byte(`{"model":"m","stream":true,"input":[]}`), "responses")
	f.Add([]byte(`{"model":"claude-x[1m]","messages":[],"system":"s"}`), domain.PlatformAnthropic)
	f.Add([]byte(`{"metadata":{"user_id":"u"},"thinking":{"type":"enabled"}}`), "chat_completions")
	f.Add([]byte(`{"contents":[],"systemInstruction":{"parts":[]}}`), domain.PlatformGemini)
	f.Fuzz(func(t *testing.T, body []byte, protocol string) {
		switch protocol {
		case "responses", "chat_completions", domain.PlatformAnthropic, domain.PlatformGemini:
		default:
			return
		}
		want, got := parseBoth(t, protocol, body)
		require.Equal(t, want, got, "body=%q protocol=%s", body, protocol)

		// 合法 JSON 上，跳过校验不得改变结果。
		if gjson.ValidBytes(body) {
			validating := &ParsedRequest{Body: NewRequestBodyRef(append([]byte(nil), body...))}
			validatingErr := parseGatewayRequestBody(validating, protocol, true)
			trusted := &ParsedRequest{Body: NewRequestBodyRef(append([]byte(nil), body...))}
			trustedErr := parseGatewayRequestBody(trusted, protocol, false)
			require.Equal(t, snapshotOf(validating, validatingErr), snapshotOf(trusted, trustedErr),
				"validated path diverged: body=%q protocol=%s", body, protocol)
		}
	})
}
