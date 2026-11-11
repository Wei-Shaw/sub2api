package service

import (
	"bytes"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// legacyEnforceCacheControlLimit / legacyNormalizeCacheControlTTLOrder /
// legacyForceEphemeralCacheControlTTL are verbatim copies of the
// implementations before the fast path was introduced. The tests below compare
// the optimized code against them byte for byte.
func legacyEnforceCacheControlLimit(body []byte) []byte {
	return legacyNormalizeCacheControlTTLOrder(enforceCacheControlBlockLimit(body))
}

func legacyNormalizeCacheControlTTLOrder(body []byte) []byte {
	_, messagePaths, toolPaths, systemPaths := collectCacheControlPaths(body)
	paths := make([]string, 0, len(toolPaths)+len(systemPaths)+len(messagePaths)+1)
	paths = append(paths, toolPaths...)
	paths = append(paths, systemPaths...)
	paths = append(paths, messagePaths...)
	if gjson.GetBytes(body, "cache_control").Exists() {
		paths = append(paths, "cache_control")
	}
	last1h := -1
	for i, path := range paths {
		if gjson.GetBytes(body, path+".ttl").String() == cacheTTLTarget1h {
			last1h = i
		}
	}
	out := body
	for _, path := range paths[:last1h+1] {
		cc := gjson.GetBytes(out, path)
		if cc.Get("type").String() != "ephemeral" || cc.Get("ttl").String() == cacheTTLTarget1h {
			continue
		}
		if next, err := sjson.SetBytes(out, path+".ttl", cacheTTLTarget1h); err == nil {
			out = next
		}
	}
	return out
}

func cacheControlEquivalenceCases() map[string]string {
	return map[string]string{
		"empty":         ``,
		"no_cc":         `{"model":"claude","messages":[{"role":"user","content":"hi"}]}`,
		"one_cc":        `{"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"hi"}]}`,
		"four_cc":       `{"tools":[{"name":"a","cache_control":{"type":"ephemeral"}}],"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral"}},{"type":"text","text":"b","cache_control":{"type":"ephemeral"}}]}]}`,
		"five_cc":       `{"tools":[{"name":"a","cache_control":{"type":"ephemeral"}},{"name":"b","cache_control":{"type":"ephemeral"}}],"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral"}},{"type":"text","text":"b","cache_control":{"type":"ephemeral"}}]}]}`,
		"thinking_cc":   `{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"x","cache_control":{"type":"ephemeral"}}]}]}`,
		"thinking_text": `{"messages":[{"role":"user","content":[{"type":"text","text":"thinking about it","cache_control":{"type":"ephemeral"}}]}]}`,
		"ttl_order":     `{"tools":[{"name":"a","cache_control":{"type":"ephemeral","ttl":"5m"}}],"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`,
		"ttl_all_1h":    `{"tools":[{"name":"a","cache_control":{"type":"ephemeral","ttl":"1h"}}],"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`,
		"top_level_1h":  `{"cache_control":{"type":"ephemeral","ttl":"1h"},"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"hi"}]}`,
		"1h_in_text":    `{"system":[{"type":"text","text":"\"1h\"","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"1h"}]}`,
		"escaped_key":   `{"messages":[{"role":"user","content":[{"type":"text","text":"a","cache\u005fcontrol":{"type":"ephemeral","ttl":"1h"}}]}],"tools":[{"name":"t","cache_control":{"type":"ephemeral"}}]}`,
		"escaped_ttl":   `{"tools":[{"name":"t","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral","ttl":"\u0031h"}}]}]}`,
		"escaped_think": `{"messages":[{"role":"assistant","content":[{"type":"\u0074hinking","thinking":"x","cache_control":{"type":"ephemeral"}}]}]}`,
		"non_ephemeral": `{"tools":[{"name":"a","cache_control":{"type":"persistent"}}],"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`,
		"cc_in_string":  `{"messages":[{"role":"user","content":"please explain \"cache_control\" and \"thinking\""}]}`,
		"duplicate_key": `{"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral"},"cache_control":{"type":"ephemeral","ttl":"1h"}}]}],"tools":[{"name":"t","cache_control":{"type":"ephemeral"}}]}`,
		"invalid_json":  `{"messages":[`,
	}
}

func TestEnforceCacheControlLimitMatchesLegacy(t *testing.T) {
	for name, raw := range cacheControlEquivalenceCases() {
		t.Run(name, func(t *testing.T) {
			body := []byte(raw)
			want := legacyEnforceCacheControlLimit(append([]byte(nil), body...))
			got := enforceCacheControlLimit(append([]byte(nil), body...))
			require.Equal(t, string(want), string(got))
		})
	}
}

func TestEnforceCacheControlLimitMatchesLegacyRandomized(t *testing.T) {
	rng := rand.New(rand.NewSource(20261111))
	for i := 0; i < 3000; i++ {
		body := randomCacheControlBody(rng)
		want := legacyEnforceCacheControlLimit(append([]byte(nil), body...))
		got := enforceCacheControlLimit(append([]byte(nil), body...))
		require.Equal(t, string(want), string(got), "case %d body=%s", i, body)
	}
}

func TestEnforceCacheControlLimitDoesNotMutateInput(t *testing.T) {
	for name, raw := range cacheControlEquivalenceCases() {
		t.Run(name, func(t *testing.T) {
			body := []byte(raw)
			before := append([]byte(nil), body...)
			_ = enforceCacheControlLimit(body)
			require.Equal(t, string(before), string(body))
		})
	}
}

func TestNormalizeCacheControlTTLOrderMatchesLegacy(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	cases := cacheControlEquivalenceCases()
	for i := 0; i < 2000; i++ {
		cases[fmt.Sprintf("random_%d", i)] = string(randomCacheControlBody(rng))
	}
	for name, raw := range cases {
		body := []byte(raw)
		before := append([]byte(nil), body...)
		want := legacyNormalizeCacheControlTTLOrder(append([]byte(nil), body...))
		got := normalizeCacheControlTTLOrder(body)
		require.Equal(t, string(want), string(got), name)
		require.Equal(t, string(before), string(body), "%s: input must not be mutated", name)
	}
}

func TestForceEphemeralCacheControlTTLDoesNotMutateInput(t *testing.T) {
	body := []byte(`{"cache_control":{"type":"ephemeral"},"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral","ttl":"5m"}}],"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral"}}]}],"tools":[{"name":"t","cache_control":{"type":"ephemeral"}}]}`)
	before := append([]byte(nil), body...)
	got := forceEphemeralCacheControlTTL(body, cacheTTLTarget1h)
	require.Equal(t, string(before), string(body))
	require.Equal(t, 4, strings.Count(string(got), `"ttl":"1h"`))
	require.True(t, gjson.ValidBytes(got))
}

func randomCacheControlBody(rng *rand.Rand) []byte {
	cc := func() string {
		switch rng.Intn(7) {
		case 0:
			return `,"cache_control":{"type":"ephemeral"}`
		case 1:
			return `,"cache_control":{"type":"ephemeral","ttl":"5m"}`
		case 2:
			return `,"cache_control":{"type":"ephemeral","ttl":"1h"}`
		case 3:
			return `,"cache_control":{"type":"persistent","ttl":"1h"}`
		default:
			return ""
		}
	}
	var b bytes.Buffer
	b.WriteString(`{"model":"claude-sonnet"`)
	if rng.Intn(5) == 0 {
		b.WriteString(`,"cache_control":{"type":"ephemeral","ttl":"1h"}`)
	}
	if rng.Intn(2) == 0 {
		b.WriteString(`,"tools":[`)
		n := rng.Intn(4)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"name":"tool_%d"%s}`, i, cc())
		}
		b.WriteByte(']')
	}
	if rng.Intn(2) == 0 {
		b.WriteString(`,"system":[`)
		n := rng.Intn(3)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"type":"text","text":"sys %d"%s}`, i, cc())
		}
		b.WriteByte(']')
	}
	b.WriteString(`,"messages":[`)
	m := rng.Intn(4)
	for i := 0; i < m; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"role":"user","content":[`)
		c := rng.Intn(3) + 1
		for j := 0; j < c; j++ {
			if j > 0 {
				b.WriteByte(',')
			}
			blockType := "text"
			if rng.Intn(6) == 0 {
				blockType = "thinking"
			}
			fmt.Fprintf(&b, `{"type":%q,"text":"m%d-%d"%s}`, blockType, i, j, cc())
		}
		b.WriteString(`]}`)
	}
	b.WriteString(`]}`)
	return b.Bytes()
}

// ---- legacy copies of the array-materializing helpers ----

func legacyIsOnlyWebSearchToolInBody(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
	}
	arr := tools.Array()
	if len(arr) != 1 {
		return false
	}
	return isWebSearchToolJSON(arr[0])
}

func legacyExtractSearchQueryFromBody(body []byte) string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return ""
	}
	arr := messages.Array()
	if len(arr) == 0 {
		return ""
	}
	lastMsg := arr[len(arr)-1]
	if lastMsg.Get("role").String() != "user" {
		return ""
	}
	content := lastMsg.Get("content")
	if content.Type == gjson.String {
		return content.String()
	}
	if content.IsArray() {
		for _, block := range content.Array() {
			if block.Get("type").String() == "text" {
				if text := block.Get("text").String(); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func legacyExtractSystemPreviewFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sys := gjson.GetBytes(body, "system")
	if !sys.Exists() {
		return ""
	}
	switch {
	case sys.IsArray():
		for _, item := range sys.Array() {
			if !item.IsObject() {
				continue
			}
			if strings.EqualFold(item.Get("type").String(), "text") {
				if t := item.Get("text").String(); strings.TrimSpace(t) != "" {
					return t
				}
			}
		}
		return ""
	case sys.Type == gjson.String:
		return sys.String()
	default:
		return ""
	}
}

func legacyRequestNeedsBetaFeatures(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if tools.Exists() && tools.IsArray() && len(tools.Array()) > 0 {
		return true
	}
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	return strings.EqualFold(thinkingType, "enabled") || strings.EqualFold(thinkingType, "adaptive")
}

func requestHelperEquivalenceBodies() []string {
	return []string{
		``,
		`{}`,
		`{"tools":[]}`,
		`{"tools":{}}`,
		`{"tools":null}`,
		`{"tools":[null]}`,
		`{"tools":[{"type":"web_search_20250305","name":"web_search"}]}`,
		`{"tools":[{"name":"web_search"}]}`,
		`{"tools":[{"name":"google_search"}]}`,
		`{"tools":[{"type":"web_search_20250305"},{"name":"bash"}]}`,
		`{"tools":[{"name":"bash"}]}`,
		`{"tools":[{"type":"computer_toolset_20260801"},{"type":"x"}]}`,
		`{"tools":[{"type":"x"},{"type":"browser_toolset_20260801"}]}`,
		`{"thinking":{"type":"adaptive"}}`,
		`{"messages":[]}`,
		`{"messages":[{"role":"assistant","content":"x"}]}`,
		`{"messages":[{"role":"user","content":"hello"}]}`,
		`{"messages":[{"role":"user","content":[{"type":"image"},{"type":"text","text":""},{"type":"text","text":"q"}]}]}`,
		`{"messages":[{"role":"user","content":"a"},{"role":"user","content":[{"type":"text","text":"last"}]}]}`,
		`{"messages":[{"role":"user","content":"a"},null]}`,
		`{"system":"plain system"}`,
		`{"system":[]}`,
		`{"system":[1,"x",{"type":"TEXT","text":"  "},{"type":"text","text":"first"},{"type":"text","text":"second"}]}`,
		`{"system":{"type":"text"}}`,
		`{"messages":[`,
	}
}

func TestRequestJSONHelpersMatchLegacy(t *testing.T) {
	for _, raw := range requestHelperEquivalenceBodies() {
		body := []byte(raw)
		require.Equal(t, legacyIsOnlyWebSearchToolInBody(body), isOnlyWebSearchToolInBody(body), "isOnlyWebSearchToolInBody %s", raw)
		require.Equal(t, legacyExtractSearchQueryFromBody(body), extractSearchQueryFromBody(body), "extractSearchQueryFromBody %s", raw)
		require.Equal(t, legacyExtractSystemPreviewFromBody(body), extractSystemPreviewFromBody(body), "extractSystemPreviewFromBody %s", raw)
		require.Equal(t, legacyRequestNeedsBetaFeatures(body), requestNeedsBetaFeatures(body), "requestNeedsBetaFeatures %s", raw)
	}
}

func TestClaudeMimicDebugLineIsLazyAndMatchesEager(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "claude-cli/2.1.200")
	req.Header.Set("Authorization", "Bearer secret-token")
	body := []byte(`{"metadata":{"user_id":"user_1"},"system":[{"type":"text","text":"You are Claude Code"}]}`)
	account := &Account{ID: 7, Name: "acc"}

	want := buildClaudeMimicDebugLine(req, body, account, "oauth", true)
	lazy := newClaudeMimicDebugLine(req, body, account, "oauth", true)
	require.Empty(t, lazy.line, "line must not be built eagerly")
	require.Equal(t, want, lazy.String())
	require.Equal(t, want, lazy.String(), "second call returns cached line")
	require.Nil(t, lazy.req, "request reference is released after building")
	require.Nil(t, lazy.body, "body reference is released after building")

	c := &ginContextStub{values: map[string]any{claudeMimicDebugInfoKey: newClaudeMimicDebugLine(req, body, account, "oauth", true)}}
	require.Equal(t, want, claudeMimicDebugLineFromContext(c))
	c.values[claudeMimicDebugInfoKey] = "legacy string"
	require.Equal(t, "legacy string", claudeMimicDebugLineFromContext(c))
	delete(c.values, claudeMimicDebugInfoKey)
	require.Empty(t, claudeMimicDebugLineFromContext(c))
	require.Empty(t, claudeMimicDebugLineFromContext(nil))
}

type ginContextStub struct {
	values map[string]any
}

func (c *ginContextStub) Get(key string) (any, bool) {
	v, ok := c.values[key]
	return v, ok
}

func toolBodiesForEquivalence() []string {
	return []string{
		`{}`,
		`{"tools":[]}`,
		`{"tools":{}}`,
		`{"tools":[{"name":"bash"}]}`,
		`{"tools":[{"name":"sessions_list"},{"name":"session_get","type":"custom"},{"name":"web_search","type":"web_search_20250305"}]}`,
		`{"tools":[{"name":"a"},{"name":"b"},{"name":"c"},{"name":"d"},{"name":"e"},{"name":"f"},{"name":"g"}],"tool_choice":{"type":"tool","name":"c"}}`,
		`{"tools":[{"name":"a","defer_loading":true,"cache_control":{"type":"ephemeral"}},{"name":"b"}]}`,
		`{"tools":[{"name":"a"},{"name":"b","custom":{"defer_loading":true},"cache_control":{"type":"ephemeral"}}]}`,
		`{"tools":[{"name":"a","defer_loading":true},{"name":"b","defer_loading":true}]}`,
		`{"tools":[{"name":"a","cache_control":{"type":"ephemeral","ttl":"1h"}}]}`,
		`{"tools":[{"name":"a","cache_control":{"type":"ephemeral"}}]}`,
		`{"tools":[{"name":"a","defer_loading":"true","cache_control":{"type":"ephemeral"}}]}`,
		`{"tools":[{"type":"computer_toolset_20260801"}]}`,
		`{"tools":[{"type":"browser_toolset_20260801"},{"type":"computer_toolset_20260801"}]}`,
	}
}

func TestToolHelpersMatchLegacy(t *testing.T) {
	headers := []string{"", "fine-grained-tool-streaming-2025-05-14", "a,fine-grained-tool-streaming-2025-05-14,b"}
	models := []string{"claude-sonnet-5-5", "claude-haiku-5-5@20260101", "claude-opus-5-5"}
	for _, raw := range toolBodiesForEquivalence() {
		body := []byte(raw)

		requireEquivalentToolNameRewrite(t, legacyBuildToolNameRewriteFromBody(body), buildToolNameRewriteFromBody(body), raw)
		require.Equal(t,
			string(legacyApplyToolsLastCacheBreakpoint(append([]byte(nil), body...))),
			string(applyToolsLastCacheBreakpoint(append([]byte(nil), body...))), raw)
		require.Equal(t,
			string(legacyStripDeferredToolCacheControl(append([]byte(nil), body...))),
			string(stripDeferredToolCacheControl(append([]byte(nil), body...))), raw)
		for _, h := range headers {
			for _, m := range models {
				require.Equal(t, legacyFilterSonnet55ToolsetBeta(h, body, m), filterSonnet55ToolsetBeta(h, body, m), "%s %s %s", raw, h, m)
			}
		}
	}
}

func TestSyncBillingHeaderVersionMatchesLegacy(t *testing.T) {
	bodies := []string{
		`{"system":"plain"}`,
		`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.0.0.abc; cc_entrypoint=cli"}],"messages":[{"role":"user","content":"hello world from a long prompt"}]}`,
		`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.0.0; cc_entrypoint=cli"},{"type":"text","text":"x-anthropic-billing-header: cc_version=1.0.0.xyz"}],"messages":[{"role":"user","content":[{"type":"text","text":"你好世界🙂"}]}]}`,
		`{"system":[{"type":"text","text":"other"}]}`,
	}
	for _, raw := range bodies {
		for _, ua := range []string{"", "claude-cli/2.1.200 (external, cli)", "curl/8"} {
			want := legacySyncBillingHeaderVersion(append([]byte(nil), raw...), ua)
			got := syncBillingHeaderVersion(append([]byte(nil), raw...), ua)
			require.Equal(t, string(want), string(got), "%s ua=%s", raw, ua)
		}
	}
}

// ReverseOrdered is built from map iteration, so entries with equal fake-name
// length come out in random order (both before and after this change). Equal
// length distinct names cannot be substrings of each other, so that order does
// not affect replacement; compare the set and the length ordering instead.
func requireEquivalentToolNameRewrite(t *testing.T, want, got *ToolNameRewrite, msg string) {
	t.Helper()
	if want == nil || got == nil {
		require.Equal(t, want, got, msg)
		return
	}
	require.Equal(t, want.Forward, got.Forward, msg)
	require.Equal(t, want.Reverse, got.Reverse, msg)
	require.ElementsMatch(t, want.ReverseOrdered, got.ReverseOrdered, msg)
	for i := 1; i < len(got.ReverseOrdered); i++ {
		require.GreaterOrEqual(t, len(got.ReverseOrdered[i-1][0]), len(got.ReverseOrdered[i][0]), msg)
	}
}
