package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// legacyForceEphemeralCacheControlTTL is the per-path sjson implementation the
// span-edit version replaces. Each sjson.SetBytes copies the whole body.
func legacyForceEphemeralCacheControlTTL(body []byte, ttl string) []byte {
	if len(body) == 0 || ttl == "" {
		return body
	}
	out := body
	var paths []string
	addPath := func(path string, value gjson.Result) {
		cc := value.Get("cache_control")
		if !cc.Exists() || cc.Get("type").String() != "ephemeral" {
			return
		}
		if cc.Get("ttl").String() == ttl {
			return
		}
		paths = append(paths, path+".cache_control.ttl")
	}

	if topCC := gjson.GetBytes(body, "cache_control"); topCC.Exists() && topCC.Get("type").String() == "ephemeral" && topCC.Get("ttl").String() != ttl {
		paths = append(paths, "cache_control.ttl")
	}

	system := gjson.GetBytes(body, "system")
	if system.IsArray() {
		idx := -1
		system.ForEach(func(_, block gjson.Result) bool {
			idx++
			addPath(fmt.Sprintf("system.%d", idx), block)
			return true
		})
	}

	messages := gjson.GetBytes(body, "messages")
	if messages.IsArray() {
		msgIdx := -1
		messages.ForEach(func(_, msg gjson.Result) bool {
			msgIdx++
			content := msg.Get("content")
			if !content.IsArray() {
				return true
			}
			contentIdx := -1
			content.ForEach(func(_, block gjson.Result) bool {
				contentIdx++
				addPath(fmt.Sprintf("messages.%d.content.%d", msgIdx, contentIdx), block)
				return true
			})
			return true
		})
	}

	tools := gjson.GetBytes(body, "tools")
	if tools.IsArray() {
		idx := -1
		tools.ForEach(func(_, tool gjson.Result) bool {
			idx++
			addPath(fmt.Sprintf("tools.%d", idx), tool)
			return true
		})
	}

	for _, path := range paths {
		if next, err := sjson.SetBytes(out, path, ttl); err == nil {
			out = next
		}
	}
	return out
}

// legacyStripMessageCacheControl deletes one path at a time, copying the whole
// body for every block that carries cache_control.
func legacyStripMessageCacheControl(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	msgIdx := -1
	messages.ForEach(func(_, msg gjson.Result) bool {
		msgIdx++
		content := msg.Get("content")
		if !content.IsArray() {
			return true
		}
		blockIdx := -1
		content.ForEach(func(_, block gjson.Result) bool {
			blockIdx++
			if !block.Get("cache_control").Exists() {
				return true
			}
			path := fmt.Sprintf("messages.%d.content.%d.cache_control", msgIdx, blockIdx)
			if next, err := sjson.DeleteBytes(body, path); err == nil {
				body = next
			}
			return true
		})
		return true
	})
	return body
}

func requireSameJSON(t *testing.T, want, got []byte) {
	t.Helper()
	var wantValue, gotValue any
	require.NoError(t, json.Unmarshal(want, &wantValue))
	require.NoError(t, json.Unmarshal(got, &gotValue))
	require.Equal(t, wantValue, gotValue)
}

func TestForceEphemeralCacheControlTTLMatchesLegacy(t *testing.T) {
	cases := map[string]string{
		"tools_system_messages": `{"model":"claude-sonnet-5-5","system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral"}}],"tools":[{"name":"a","cache_control":{"type":"ephemeral","ttl":"5m"}}],"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral"}}]}]}`,
		"top_level":             `{"cache_control":{"type":"ephemeral","ttl":"5m"},"messages":[]}`,
		"already_1h":            `{"tools":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}],"messages":[]}`,
		"non_ephemeral":         `{"tools":[{"cache_control":{"type":"persistent"}}],"messages":[]}`,
		"no_cache_control":      `{"model":"m","messages":[{"role":"user","content":"hi"}]}`,
		"content_string":        `{"messages":[{"role":"user","content":"hi"}]}`,
		"many_blocks":           `{"system":[{"cache_control":{"type":"ephemeral"}},{"cache_control":{"type":"ephemeral","ttl":"5m"}},{"cache_control":{"type":"ephemeral","ttl":"1h"}}],"messages":[{"role":"user","content":[{"cache_control":{"type":"ephemeral"}},{"no":"cc"}]}],"tools":[{"cache_control":{"type":"ephemeral"}}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			for _, ttl := range []string{"1h", "5m"} {
				want := legacyForceEphemeralCacheControlTTL([]byte(raw), ttl)
				got := forceEphemeralCacheControlTTL([]byte(raw), ttl)
				requireSameJSON(t, want, got)
			}
		})
	}
}

func TestStripMessageCacheControlMatchesLegacy(t *testing.T) {
	cases := map[string]string{
		"three_blocks":   `{"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral"}},{"type":"text","text":"b","cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"text","text":"c"}]}]}`,
		"no_cache":       `{"messages":[{"role":"user","content":[{"type":"text","text":"a"}]}]}`,
		"content_string": `{"messages":[{"role":"user","content":"hi"}]}`,
		"multiple_msgs":  `{"messages":[{"role":"user","content":[{"cache_control":{"type":"ephemeral"}}]},{"role":"assistant","content":[{"cache_control":{"type":"ephemeral"},"text":"x"}]}]}`,
		"not_array":      `{"messages":"hi"}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			want := legacyStripMessageCacheControl([]byte(raw))
			got := stripMessageCacheControl([]byte(raw))
			requireSameJSON(t, want, got)
		})
	}
}
