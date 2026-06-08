//go:build unit

package plugin

import (
	"slices"
	"strings"
	"testing"
)

func TestFilterSensitiveEnv_RemovesSensitiveVars(t *testing.T) {
	t.Parallel()
	input := []string{
		"PATH=/usr/bin",
		"HOME=/home/user",
		"POSTGRES_PASSWORD=secret123",
		"DATABASE_HOST=db.example.com",
		"REDIS_PASSWORD=redis-secret",
		"JWT_SECRET=jwt-key",
		"ADMIN_API_KEY=admin-xxx",
		"ENCRYPTION_KEY=enc-key",
		"SECRET_TOKEN=tok",
		"PASSWORD_HASH=hash",
		"TOTP_ENCRYPTION_KEY=totp",
		"PGPASSWORD=pg",
		"SESSION_SECRET_KEY=sess",
		"COOKIE_SECRET=cookie",
		"STRIPE_SECRET_KEY=sk_live_xxx",
		"GOPATH=/home/user/go",
		"TEMP=/tmp",
	}

	got := filterSensitiveEnv(input)

	// 应该保留的安全变量
	for _, safe := range []string{"PATH=/usr/bin", "HOME=/home/user", "GOPATH=/home/user/go", "TEMP=/tmp"} {
		if !slices.Contains(got, safe) {
			t.Errorf("expected %q to be retained, but missing", safe)
		}
	}
	// 应该被过滤掉的敏感变量
	for _, entry := range got {
		key, _, _ := strings.Cut(entry, "=")
		if isSensitiveEnvKey(key) {
			t.Errorf("sensitive var %q should have been filtered", key)
		}
	}
}

func TestFilterSensitiveEnv_CaseInsensitive(t *testing.T) {
	t.Parallel()
	input := []string{
		"postgres_password=secret",
		"Jwt_Secret=key",
		"admin_api_key=xxx",
		"Redis_Host=r.example.com",
		"PATH=/usr/bin",
	}

	got := filterSensitiveEnv(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(got), got)
	}
	if got[0] != "PATH=/usr/bin" {
		t.Errorf("expected PATH, got %q", got[0])
	}
}

func TestFilterSensitiveEnv_MalformedEntryDropped(t *testing.T) {
	t.Parallel()
	input := []string{
		"VALID_KEY=value",
		"NO_EQUALS_SIGN",
		"ANOTHER=ok",
	}

	got := filterSensitiveEnv(input)
	want := []string{"VALID_KEY=value", "ANOTHER=ok"}
	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("entry[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

func TestFilterSensitiveEnv_EmptyInput(t *testing.T) {
	t.Parallel()
	got := filterSensitiveEnv(nil)
	if len(got) != 0 {
		t.Errorf("expected empty result for nil input, got %v", got)
	}
	got = filterSensitiveEnv([]string{})
	if len(got) != 0 {
		t.Errorf("expected empty result for empty input, got %v", got)
	}
}

func TestFilterSensitiveEnv_ValueContainingEquals(t *testing.T) {
	t.Parallel()
	// 值中包含 '=' 不应被误截断
	input := []string{"SOME_CONFIG=key=value=extra", "POSTGRES_DB=mydb"}
	got := filterSensitiveEnv(input)
	if len(got) != 1 || got[0] != "SOME_CONFIG=key=value=extra" {
		t.Errorf("expected SOME_CONFIG preserved with full value, got %v", got)
	}
}

func TestIsSensitiveEnvKey_KnownKeys(t *testing.T) {
	t.Parallel()
	sensitive := []string{
		"POSTGRES_PASSWORD", "DATABASE_HOST", "REDIS_PASSWORD",
		"JWT_SECRET", "ADMIN_API_KEY", "ENCRYPTION_KEY_V2",
		"SECRET_THING", "PASSWORD_SALT", "TOTP_KEY",
		"PGPASSWORD", "SESSION_SECRET_TOKEN", "COOKIE_SECRET",
		"STRIPE_WEBHOOK_SECRET", "PAYPAL_CLIENT_SECRET",
		"WECHAT_PAY_KEY", "ALIPAY_PRIVATE_KEY",
	}
	for _, k := range sensitive {
		if !isSensitiveEnvKey(k) {
			t.Errorf("%q should be sensitive", k)
		}
	}

	safe := []string{
		"PATH", "HOME", "GOPATH", "GOROOT", "TEMP", "TMP",
		"SYSTEMROOT", "COMSPEC", "LANG", "LC_ALL", "USER",
		"HOSTNAME", "PLUGIN_LOG_LEVEL",
	}
	for _, k := range safe {
		if isSensitiveEnvKey(k) {
			t.Errorf("%q should NOT be sensitive", k)
		}
	}
}
