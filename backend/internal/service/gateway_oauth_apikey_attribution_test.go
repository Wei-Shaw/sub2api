package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func apiKeyModeClientBody() []byte {
	return []byte(`{"model":"claude-sonnet-5","system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.311.9cf; cc_entrypoint=sdk-cli;"}],"metadata":{"user_id":"{\"device_id\":\"abc\",\"account_uuid\":\"\",\"session_id\":\"sss\"}"},"max_tokens":32000,"messages":[{"role":"user","content":"hi"}],"output_config":{"effort":"high"},"stream":true}`)
}

// api-key 模式的真 CLI 不发 safeguards、metadata.account_uuid 留空串；OAuth 账号转发
// 时必须补齐，否则"UA 声称 CLI + body 是 api-key 形状"与 OAuth 令牌自相矛盾。
func TestPatchOAuthClientAttributionFillsGaps(t *testing.T) {
	acct := &Account{Extra: map[string]any{"account_uuid": "uuid-123"}}
	out := patchOAuthClientAttributionForAPIKeyClient(apiKeyModeClientBody(), acct, "Windows")

	// 键位置必须与真机一致：safeguards 在 max_tokens 之后、output_config 之前
	if want := "model,messages,system,metadata,max_tokens,safeguards,output_config,stream"; strings.Join(topLevelKeys(out), ",") != want {
		t.Fatalf("key order wrong: %v", topLevelKeys(out))
	}
	if got := gjsonGet(out, "safeguards.0.classifier_context.platform"); got != "windows" {
		t.Fatalf("safeguards not injected with client platform, got %q", got)
	}
	if got := gjsonGet(out, "safeguards.0.classifier_context.case_insensitive_paths"); got != "true" {
		t.Fatalf("case_insensitive_paths should follow windows client, got %q", got)
	}

	uid := gjsonGet(out, "metadata.user_id")
	var parsed oauthClientMetadataUserID
	if err := json.Unmarshal([]byte(uid), &parsed); err != nil {
		t.Fatalf("user_id not valid json: %v", err)
	}
	if parsed.AccountUUID != "uuid-123" || parsed.DeviceID != "abc" || parsed.SessionID != "sss" {
		t.Fatalf("user_id not patched in place: %s", uid)
	}
	// 顺序必须保持真机形状，反序列化不能重排
	if want := `{"device_id":"abc","account_uuid":"uuid-123","session_id":"sss"}`; uid != want {
		t.Fatalf("user_id order changed: %s", uid)
	}
}

// 客户端自己发了 safeguards / 已填 account_uuid 时必须原样保留。
func TestPatchOAuthClientAttributionKeepsClientValues(t *testing.T) {
	acct := &Account{Extra: map[string]any{"account_uuid": "uuid-123"}}
	body := []byte(`{"metadata":{"user_id":"{\"device_id\":\"abc\",\"account_uuid\":\"mine\",\"session_id\":\"sss\"}"},"safeguards":[{"type":"dangerous_tool_use","classifier_context":{"platform":"macos"}}]}`)
	out := patchOAuthClientAttributionForAPIKeyClient(body, acct, "Windows")
	if got := gjsonGet(out, "safeguards.0.classifier_context.platform"); got != "macos" {
		t.Fatalf("client safeguards overwritten: %q", got)
	}
	var parsed oauthClientMetadataUserID
	_ = json.Unmarshal([]byte(gjsonGet(out, "metadata.user_id")), &parsed)
	if parsed.AccountUUID != "mine" {
		t.Fatalf("client account_uuid overwritten: %q", parsed.AccountUUID)
	}
}

// 账号没有 account_uuid 时不得写入空串以外的垃圾。
func TestPatchOAuthClientAttributionNoAccountUUID(t *testing.T) {
	out := patchOAuthClientAttributionForAPIKeyClient(apiKeyModeClientBody(), &Account{Extra: map[string]any{}}, "Linux")
	if got := gjsonGet(out, "metadata.user_id"); got != `{"device_id":"abc","account_uuid":"","session_id":"sss"}` {
		t.Fatalf("user_id should stay untouched: %s", got)
	}
}

func gjsonGet(body []byte, path string) string {
	return gjson.GetBytes(body, path).String()
}

// topLevelKeys 返回 JSON 对象顶层键（按出现顺序）。
func topLevelKeys(body []byte) []string {
	keys := []string{}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	if _, err := dec.Token(); err != nil {
		return keys
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return keys
		}
		k, _ := tok.(string)
		keys = append(keys, k)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return keys
		}
	}
	return keys
}
