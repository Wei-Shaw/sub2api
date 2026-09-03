package logredact

import (
	"strings"
	"testing"
)

func TestRedactText_JSONLike(t *testing.T) {
	in := `{"access_token":"ya29.a0AfH6SMDUMMY","refresh_token":"1//0gDUMMY","other":"ok"}`
	out := RedactText(in)
	if out == in {
		t.Fatalf("expected redaction, got unchanged")
	}
	if want := `"access_token":"***"`; !strings.Contains(out, want) {
		t.Fatalf("expected %q in %q", want, out)
	}
	if want := `"refresh_token":"***"`; !strings.Contains(out, want) {
		t.Fatalf("expected %q in %q", want, out)
	}
}

func TestRedactText_QueryLike(t *testing.T) {
	in := "access_token=ya29.a0AfH6SMDUMMY refresh_token=1//0gDUMMY"
	out := RedactText(in)
	if strings.Contains(out, "ya29") || strings.Contains(out, "1//0") {
		t.Fatalf("expected tokens redacted, got %q", out)
	}
}

func TestRedactText_GOCSPX(t *testing.T) {
	in := "client_secret=GOCSPX-your-client-secret"
	out := RedactText(in)
	if strings.Contains(out, "your-client-secret") {
		t.Fatalf("expected secret redacted, got %q", out)
	}
	if !strings.Contains(out, "client_secret=***") {
		t.Fatalf("expected key redacted, got %q", out)
	}
}

func TestRedactText_ExtraKeyCacheUsesNormalizedSortedKey(t *testing.T) {
	clearExtraTextPatternCache()

	out1 := RedactText("custom_secret=abc", "Custom_Secret", " custom_secret ")
	out2 := RedactText("custom_secret=xyz", "custom_secret")
	if !strings.Contains(out1, "custom_secret=***") {
		t.Fatalf("expected custom key redacted in first call, got %q", out1)
	}
	if !strings.Contains(out2, "custom_secret=***") {
		t.Fatalf("expected custom key redacted in second call, got %q", out2)
	}

	if got := countExtraTextPatternCacheEntries(); got != 1 {
		t.Fatalf("expected 1 cached pattern set, got %d", got)
	}
}

func TestRedactText_DefaultPathDoesNotUseExtraCache(t *testing.T) {
	clearExtraTextPatternCache()

	out := RedactText("access_token=abc")
	if !strings.Contains(out, "access_token=***") {
		t.Fatalf("expected default key redacted, got %q", out)
	}
	if got := countExtraTextPatternCacheEntries(); got != 0 {
		t.Fatalf("expected extra cache to remain empty, got %d", got)
	}
}

func clearExtraTextPatternCache() {
	extraTextPatternCache.Range(func(key, value any) bool {
		extraTextPatternCache.Delete(key)
		return true
	})
}

func countExtraTextPatternCacheEntries() int {
	count := 0
	extraTextPatternCache.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}

// TestRedactMap_CoversSecretKeyFamily 锁定 L1 修复：默认清单必须覆盖仓库里实际在用的
// 密钥字段名，而不只是 OAuth 簇。map/JSON 路径是整键精确匹配，camelCase 配置键
// （wxpay 的 apiV3Key/privateKey）以其小写形式命中。
func TestRedactMap_CoversSecretKeyFamily(t *testing.T) {
	in := map[string]any{
		"secret_key":  "sk-secret",
		"api_key":     "sk-ant-live",
		"apikey":      "sk-alias",
		"private_key": "-----BEGIN PRIVATE KEY-----",
		"apiV3Key":    "0123456789abcdef0123456789abcdef",
		"privateKey":  "-----BEGIN PRIVATE KEY-----",
		"session_key": "sess-secret",
		"cookie":      "session=abc",
		"Set-Cookie":  "session=abc; HttpOnly",
	}
	out := RedactMap(in)
	for k := range in {
		if got := out[k]; got != "***" {
			t.Fatalf("expected key %q redacted, got %v", k, got)
		}
	}
}

// TestRedactMap_DoesNotOverRedactIDFields 保证新增的宽字段名不会吞掉排障必需的业务 ID。
// map 路径是整键精确匹配，api_key_id / apikey_id / session_id 都不是敏感键。
func TestRedactMap_DoesNotOverRedactIDFields(t *testing.T) {
	in := map[string]any{
		"api_key_id":  float64(42),
		"apikey_id":   float64(43),
		"session_id":  "sess-1",
		"key_version": "v3",
	}
	out := RedactMap(in)
	for k, want := range in {
		if out[k] != want {
			t.Fatalf("expected key %q untouched, got %v", k, out[k])
		}
	}
}

// TestRedactText_SecretKeyFamilyWordBoundary 锁定文本路径的词边界语义：
// api_key=... 会被脱敏，而 api_key_id=42 不会（Go regexp 里 `_` 是词字符，
// \bapi_key\b 命不中 api_key_id），排障时仍能看到是哪一把 key。
func TestRedactText_SecretKeyFamilyWordBoundary(t *testing.T) {
	out := RedactText("api_key=sk-ant-live api_key_id=42 cookie=session-abc")
	if strings.Contains(out, "sk-ant-live") {
		t.Fatalf("expected api_key redacted, got %q", out)
	}
	if strings.Contains(out, "session-abc") {
		t.Fatalf("expected cookie redacted, got %q", out)
	}
	if !strings.Contains(out, "api_key_id=42") {
		t.Fatalf("expected api_key_id preserved, got %q", out)
	}
}

// TestDefaultSensitiveKeys_DerivedFromList 防止两份清单再次漂移。
func TestDefaultSensitiveKeys_DerivedFromList(t *testing.T) {
	if len(defaultSensitiveKeys) != len(defaultSensitiveKeyList) {
		t.Fatalf("key set/list size mismatch: %d vs %d", len(defaultSensitiveKeys), len(defaultSensitiveKeyList))
	}
	for _, k := range defaultSensitiveKeyList {
		if _, ok := defaultSensitiveKeys[k]; !ok {
			t.Fatalf("key %q missing from derived set", k)
		}
	}
}
