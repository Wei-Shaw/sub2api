package urlvalidator

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateURLFormat(t *testing.T) {
	if _, err := ValidateURLFormat("", false); err == nil {
		t.Fatalf("expected empty url to fail")
	}
	if _, err := ValidateURLFormat("://bad", false); err == nil {
		t.Fatalf("expected invalid url to fail")
	}
	if _, err := ValidateURLFormat("http://example.com", false); err == nil {
		t.Fatalf("expected http to fail when allow_insecure_http is false")
	}
	if _, err := ValidateURLFormat("https://example.com", false); err != nil {
		t.Fatalf("expected https to pass, got %v", err)
	}
	if _, err := ValidateURLFormat("http://example.com", true); err != nil {
		t.Fatalf("expected http to pass when allow_insecure_http is true, got %v", err)
	}
	if _, err := ValidateURLFormat("https://example.com:bad", true); err == nil {
		t.Fatalf("expected invalid port to fail")
	}

	// 验证末尾斜杠被移除
	normalized, err := ValidateURLFormat("https://example.com/", false)
	if err != nil {
		t.Fatalf("expected trailing slash url to pass, got %v", err)
	}
	if normalized != "https://example.com" {
		t.Fatalf("expected trailing slash to be removed, got %s", normalized)
	}

	// 验证多个末尾斜杠被移除
	normalized, err = ValidateURLFormat("https://example.com///", false)
	if err != nil {
		t.Fatalf("expected multiple trailing slashes to pass, got %v", err)
	}
	if normalized != "https://example.com" {
		t.Fatalf("expected all trailing slashes to be removed, got %s", normalized)
	}

	// 验证带路径的 URL 末尾斜杠被移除
	normalized, err = ValidateURLFormat("https://example.com/api/v1/", false)
	if err != nil {
		t.Fatalf("expected trailing slash url with path to pass, got %v", err)
	}
	if normalized != "https://example.com/api/v1" {
		t.Fatalf("expected trailing slash to be removed from path, got %s", normalized)
	}
}

func TestValidateHTTPURL(t *testing.T) {
	if _, err := ValidateHTTPURL("http://example.com", false, ValidationOptions{}); err == nil {
		t.Fatalf("expected http to fail when allow_insecure_http is false")
	}
	if _, err := ValidateHTTPURL("http://example.com", true, ValidationOptions{}); err != nil {
		t.Fatalf("expected http to pass when allow_insecure_http is true, got %v", err)
	}
	if _, err := ValidateHTTPURL("https://example.com", false, ValidationOptions{RequireAllowlist: true}); err == nil {
		t.Fatalf("expected require allowlist to fail when empty")
	}
	if _, err := ValidateHTTPURL("https://example.com", false, ValidationOptions{AllowedHosts: []string{"api.example.com"}}); err == nil {
		t.Fatalf("expected host not in allowlist to fail")
	}
	if _, err := ValidateHTTPURL("https://api.example.com", false, ValidationOptions{AllowedHosts: []string{"api.example.com"}}); err != nil {
		t.Fatalf("expected allowlisted host to pass, got %v", err)
	}
	if _, err := ValidateHTTPURL("https://sub.api.example.com", false, ValidationOptions{AllowedHosts: []string{"*.example.com"}}); err != nil {
		t.Fatalf("expected wildcard allowlist to pass, got %v", err)
	}
	if _, err := ValidateHTTPURL("https://localhost", false, ValidationOptions{AllowPrivate: false}); err == nil {
		t.Fatalf("expected localhost to be blocked when allow_private_hosts is false")
	}
}

// withBlockPrivateUpstreams 临时设置全局私网策略，并在用例结束后恢复。
func withBlockPrivateUpstreams(t *testing.T, block bool) {
	t.Helper()
	prev := BlockPrivateUpstreams()
	SetBlockPrivateUpstreams(block)
	t.Cleanup(func() { SetBlockPrivateUpstreams(prev) })
}

// TestValidateURLFormatBlocksPrivateTargets 固化安全审计 M2 的修复：
// 关闭主机白名单时的降级路径不再是「纯格式校验」，私网/环回/链路本地目标
// 由 security.url_allowlist.allow_private_hosts 单独控制（默认阻断）。
func TestValidateURLFormatBlocksPrivateTargets(t *testing.T) {
	withBlockPrivateUpstreams(t, true)

	blocked := []string{
		"http://169.254.169.254",         // 云元数据服务
		"http://169.254.169.254/latest/", // 带路径
		"http://127.0.0.1:8080",          // 环回
		"http://[::1]:8080",              // IPv6 环回
		"http://localhost:3000",          // localhost 字面量
		"http://api.localhost",           // *.localhost
		"http://10.0.0.5",                // RFC1918
		"http://172.16.0.5",              // RFC1918
		"http://192.168.1.10:3000",       // RFC1918
		"http://0.0.0.0:8080",            // unspecified
		"http://[::ffff:127.0.0.1]:8080", // IPv4-mapped 环回
		"https://192.168.1.10",           // https 同样阻断
	}
	for _, raw := range blocked {
		if _, err := ValidateURLFormat(raw, true); err == nil {
			t.Fatalf("expected %q to be blocked by the private-host policy", raw)
		} else {
			if !errors.Is(err, ErrPrivateHostBlocked) {
				t.Fatalf("expected %q to fail with ErrPrivateHostBlocked, got %v", raw, err)
			}
			// 报错必须点名放行开关，否则运营者无从得知如何合法放行。
			if !strings.Contains(err.Error(), AllowPrivateHostsSettingKey) {
				t.Fatalf("error for %q must name %s, got %v", raw, AllowPrivateHostsSettingKey, err)
			}
			if !strings.Contains(err.Error(), AllowPrivateHostsEnvKey) {
				t.Fatalf("error for %q must name env %s, got %v", raw, AllowPrivateHostsEnvKey, err)
			}
		}
	}

	allowed := []string{
		"https://api.anthropic.com",
		"http://relay.example.com:8080",
		"https://8.8.8.8",
	}
	for _, raw := range allowed {
		if _, err := ValidateURLFormat(raw, true); err != nil {
			t.Fatalf("expected %q to pass, got %v", raw, err)
		}
	}
}

// TestValidateURLFormatAllowsPrivateTargetsWhenOperatorOptsIn 固化运营者出路：
// 自建/LAN 上游在显式放行后必须仍然可用（自托管部署不能被一刀切死）。
func TestValidateURLFormatAllowsPrivateTargetsWhenOperatorOptsIn(t *testing.T) {
	withBlockPrivateUpstreams(t, false)

	for _, raw := range []string{"http://127.0.0.1:8080", "http://192.168.1.10:3000", "http://localhost:11434"} {
		normalized, err := ValidateURLFormat(raw, true)
		if err != nil {
			t.Fatalf("expected %q to pass when private upstreams are allowed, got %v", raw, err)
		}
		if normalized != raw {
			t.Fatalf("expected %q to be returned unchanged, got %q", raw, normalized)
		}
	}
}

// TestValidateHTTPURLPrivateErrorNamesSetting 保证白名单路径的私网报错同样点名开关。
func TestValidateHTTPURLPrivateErrorNamesSetting(t *testing.T) {
	_, err := ValidateHTTPURL("https://169.254.169.254", false, ValidationOptions{AllowPrivate: false})
	if err == nil {
		t.Fatalf("expected link-local host to be rejected")
	}
	if !errors.Is(err, ErrPrivateHostBlocked) {
		t.Fatalf("expected ErrPrivateHostBlocked, got %v", err)
	}
	if !strings.Contains(err.Error(), AllowPrivateHostsSettingKey) {
		t.Fatalf("error must name %s, got %v", AllowPrivateHostsSettingKey, err)
	}
}

// TestValidateResolvedIPBlocksLiteralPrivateIPs 验证解析后 IP 校验（DNS Rebinding /
// 重定向防护）对字面量 IP 不需要 DNS 即可判定，且判定集合与静态校验一致。
func TestValidateResolvedIPBlocksLiteralPrivateIPs(t *testing.T) {
	for _, host := range []string{"169.254.169.254", "127.0.0.1", "10.1.2.3", "192.168.0.1", "::1", "0.0.0.0"} {
		if err := ValidateResolvedIP(host); err == nil {
			t.Fatalf("expected resolved ip %q to be rejected", host)
		} else if !errors.Is(err, ErrPrivateHostBlocked) {
			t.Fatalf("expected ErrPrivateHostBlocked for %q, got %v", host, err)
		}
	}
	if err := ValidateResolvedIP("1.1.1.1"); err != nil {
		t.Fatalf("expected public ip to pass, got %v", err)
	}
}
