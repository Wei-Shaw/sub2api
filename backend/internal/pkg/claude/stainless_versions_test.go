package claude

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStainlessVersionsForCLIVersion(t *testing.T) {
	cases := []struct {
		name      string
		version   string
		want      StainlessVersions
		wantExact bool
	}{
		{"first_range_start", "2.1.193", StainlessVersions{"0.94.0", "v26.3.0"}, true},
		{"first_range_end", "2.1.227", StainlessVersions{"0.94.0", "v26.3.0"}, true},
		{"second_range_start", "2.1.228", StainlessVersions{"0.112.1", "v26.3.0"}, true},
		{"second_range_end", "2.1.241", StainlessVersions{"0.112.1", "v26.3.0"}, true},
		{"gap_after_second_range", "2.1.242", StainlessVersions{"0.112.1", "v26.3.0"}, false},
		{"builtin_baseline_in_gap", "2.1.258", StainlessVersions{"0.112.1", "v26.3.0"}, false},
		{"gap_before_third_range", "2.1.280", StainlessVersions{"0.112.1", "v26.3.0"}, false},
		{"third_range_start", "2.1.281", StainlessVersions{"0.112.1", "v26.3.0"}, true},
		{"third_range_end", "2.1.284", StainlessVersions{"0.112.1", "v26.3.0"}, true},
		{"fourth_range_start", "2.1.285", StainlessVersions{"0.127.0", "v26.3.0"}, true},
		{"fourth_range_end", "2.1.287", StainlessVersions{"0.127.0", "v26.3.0"}, true},
		{"fifth_range_start", "2.1.288", StainlessVersions{"0.128.0", "v26.3.0"}, true},
		{"fifth_range_end", "2.1.310", StainlessVersions{"0.128.0", "v26.3.0"}, true},
		{"last_range_start", "2.1.311", StainlessVersions{"0.128.0", "v26.12.1"}, true},
		{"last_range_open_ended", "9.9.9", StainlessVersions{"0.128.0", "v26.12.1"}, true},
		{"below_all_ranges", "2.1.100", StainlessVersions{"0.94.0", "v26.3.0"}, false},
		{"empty", "", StainlessVersions{"0.94.0", "v26.3.0"}, false},
		{"malformed", "not-a-version", StainlessVersions{"0.94.0", "v26.3.0"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, exact := StainlessVersionsForCLIVersion(tc.version)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantExact, exact)
		})
	}
}

// DefaultHeaders 的 Package/Runtime 必须跟随运行期 UA 版本，不能继续发
// 旧构建常量；OS/Arch/Runtime 是固定的虚拟设备，不参与版本同步。
func TestDefaultHeadersStainlessVersionsFollowEffectiveCLIVersion(t *testing.T) {
	versions := []string{CLICurrentVersion, "2.1.330", "9.9.9"}
	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			withCLIVersionResolver(t, func() string { return version })

			headers := DefaultHeaders()
			want, _ := StainlessVersionsForCLIVersion(EffectiveCLIVersion())
			require.Equal(t, "claude-cli/"+EffectiveCLIVersion()+" (external, cli)", headers["User-Agent"])
			require.Equal(t, want.PackageVersion, headers["X-Stainless-Package-Version"])
			require.Equal(t, want.RuntimeVersion, headers["X-Stainless-Runtime-Version"])
			require.Equal(t, "Linux", headers["X-Stainless-OS"])
			require.Equal(t, "arm64", headers["X-Stainless-Arch"])
			require.Equal(t, "node", headers["X-Stainless-Runtime"])
		})
	}
}
