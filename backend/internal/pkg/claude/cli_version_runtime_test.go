package claude

import (
	"strings"
	"testing"
)

// withCLIVersionResolver 注入测试解析器并在用例结束时还原，避免用例间污染。
func withCLIVersionResolver(t *testing.T, resolver func() string) {
	t.Helper()
	SetCLIVersionResolver(resolver)
	t.Cleanup(func() { SetCLIVersionResolver(nil) })
}

// a. resolver 未注入时，EffectiveCLIVersion() 必须等于 CLIVersion()，
// 且 DefaultHeaders()["User-Agent"] 与改造前的 init 固化值完全一致。
func TestEffectiveCLIVersionDefaultsToCLIVersion(t *testing.T) {
	SetCLIVersionResolver(nil)
	if got := EffectiveCLIVersion(); got != CLIVersion() {
		t.Fatalf("EffectiveCLIVersion() = %q, want CLIVersion() = %q", got, CLIVersion())
	}
	wantUA := "claude-cli/" + CLIVersion() + " (external, cli)"
	if got := DefaultHeaders()["User-Agent"]; got != wantUA {
		t.Fatalf("DefaultHeaders()[User-Agent] = %q, want %q", got, wantUA)
	}
}

// b. 注入返回更高版本的 resolver 后，DefaultHeaders()["User-Agent"] 跟随变化。
func TestEffectiveCLIVersionFollowsResolver(t *testing.T) {
	const upgraded = "9.9.9"
	withCLIVersionResolver(t, func() string { return upgraded })

	if got := EffectiveCLIVersion(); got != upgraded {
		t.Fatalf("EffectiveCLIVersion() = %q, want %q", got, upgraded)
	}
	wantUA := "claude-cli/" + upgraded + " (external, cli)"
	if got := DefaultHeaders()["User-Agent"]; got != wantUA {
		t.Fatalf("DefaultHeaders()[User-Agent] = %q, want %q", got, wantUA)
	}
	if got := DefaultHeaders()["X-App"]; got != "cli" {
		t.Fatalf("DefaultHeaders()[X-App] = %q, want cli", got)
	}
}

// c. resolver 返回非法值（空串、非 semver、低于内置基线）时回退 CLIVersion()。
func TestEffectiveCLIVersionFallsBackOnInvalidResolverValues(t *testing.T) {
	for name, invalid := range map[string]string{
		"empty":          "",
		"not semver":     "abc",
		"below baseline": "1.0.0",
		"whitespace":     "   ",
	} {
		t.Run(name, func(t *testing.T) {
			withCLIVersionResolver(t, func() string { return invalid })
			if got := EffectiveCLIVersion(); got != CLIVersion() {
				t.Fatalf("resolver %q: EffectiveCLIVersion() = %q, want fallback %q", invalid, got, CLIVersion())
			}
			ua := DefaultHeaders()["User-Agent"]
			if !strings.HasPrefix(ua, "claude-cli/"+CLIVersion()+" ") {
				t.Fatalf("UA %q does not carry fallback version %q", ua, CLIVersion())
			}
		})
	}
}

// 撤销注入（传 nil）后回到 CLIVersion()。
func TestSetCLIVersionResolverNilRestoresFallback(t *testing.T) {
	withCLIVersionResolver(t, func() string { return "9.9.9" })
	SetCLIVersionResolver(nil)
	if got := EffectiveCLIVersion(); got != CLIVersion() {
		t.Fatalf("after nil resolver: EffectiveCLIVersion() = %q, want %q", got, CLIVersion())
	}
}
