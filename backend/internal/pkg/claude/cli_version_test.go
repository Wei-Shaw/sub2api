package claude

import "testing"

func TestIsSupportedCLIVersion(t *testing.T) {
	cases := []struct {
		name    string
		version string
		want    bool
	}{
		{"内置基线本身", CLICurrentVersion, true},
		{"高于基线的补丁位", "2.1.251", true},
		{"高于基线的次版本", "2.2.0", true},
		{"高于基线的主版本", "3.0.0", true},
		{"低于基线", "2.1.219", false},
		{"远低于基线", "1.0.0", false},
		{"空值", "", false},
		{"只有两段", "2.2", false},
		{"带 v 前缀", "v2.2.0", false},
		{"预发布后缀", "2.2.0-local", false},
		{"构建元数据", "2.2.0+build1", false},
		{"哨兵版本带后缀", "999.0.0-local", false},
		{"纯数字哨兵仍然合法（形态没问题，运维自负）", "999.0.0", true},
		{"非数字", "abc", false},
		{"多余的段", "2.2.0.1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSupportedCLIVersion(tc.version); got != tc.want {
				t.Fatalf("IsSupportedCLIVersion(%q) = %v, want %v", tc.version, got, tc.want)
			}
		})
	}
}

func TestResolveCLIVersion(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"未配置时用内置基线", "", CLICurrentVersion},
		{"只有空白等同未配置", "   ", CLICurrentVersion},
		{"合法覆盖生效", "2.1.251", "2.1.251"},
		{"两侧空白被裁掉", "  2.1.251  ", "2.1.251"},
		{"非法值回落基线", "not-a-version", CLICurrentVersion},
		{"向下覆盖被拒", "2.0.0", CLICurrentVersion},
		{"预发布后缀被拒（会毒化账号指纹）", "2.2.0-local", CLICurrentVersion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveCLIVersion(tc.raw); got != tc.want {
				t.Fatalf("resolveCLIVersion(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// 伪装身份的自洽性：User-Agent 头里的版本号必须与 CLIVersion() 一致。
// 两者由不同代码路径写入同一个请求，不一致会被上游判为非正版客户端。
func TestDefaultHeadersUserAgentMatchesCLIVersion(t *testing.T) {
	want := "claude-cli/" + CLIVersion() + " (external, cli)"
	if got := DefaultHeaders["User-Agent"]; got != want {
		t.Fatalf("DefaultHeaders[User-Agent] = %q, want %q", got, want)
	}
}

// 没有配置覆盖时，CLIVersion() 必须等于内置基线——保证本 PR 对既有部署零行为变化。
func TestCLIVersionDefaultsToBuiltinPin(t *testing.T) {
	if got := CLIVersion(); got != CLICurrentVersion {
		t.Fatalf("CLIVersion() = %q, want built-in pin %q (测试进程未设置 %s)", got, CLICurrentVersion, CLIVersionEnv)
	}
}
