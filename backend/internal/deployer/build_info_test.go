package deployer

import "testing"

func TestHealthSeparatesProtocolVersionFromBuildIdentity(t *testing.T) {
	manager := &Manager{
		buildInfo: BuildInfo{
			Version: "0.1.168-ts.1",
			Commit:  "0123456789abcdef",
			Date:    "2026-07-29T00:00:00Z",
			Type:    "release",
			Arch:    "amd64",
		},
	}

	health := manager.Health()
	if health.Version != ControlProtocolVersion {
		t.Fatalf("Health.Version = %q, want protocol version %q", health.Version, ControlProtocolVersion)
	}
	if health.Build != manager.buildInfo {
		t.Fatalf("Health.Build = %#v, want %#v", health.Build, manager.buildInfo)
	}
}

func TestNormalizeBuildInfoUsesDevelopmentDefaults(t *testing.T) {
	info := normalizeBuildInfo(BuildInfo{})
	if info.Version != "dev" || info.Commit != "none" || info.Date != "unknown" || info.Type != "dev" || info.Arch == "" {
		t.Fatalf("normalizeBuildInfo() = %#v", info)
	}
}
