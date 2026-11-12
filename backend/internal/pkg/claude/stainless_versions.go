package claude

import (
	"strings"

	"golang.org/x/mod/semver"
)

// StainlessVersions 是真实 Claude Code 客户端在同一次构建中固定携带的
// X-Stainless-Package-Version / X-Stainless-Runtime-Version 组合。
// Stainless SDK 的 Package-Version 是编译期常量，Runtime-Version 是内嵌运行时
// 的版本，二者都不随请求变化，只随 CLI 发版跳变。
type StainlessVersions struct {
	PackageVersion string
	RuntimeVersion string
}

// stainlessVersionRange 是 CLI 版本区间到 Stainless 版本组合的映射。
// maxVersion 为开区间上界，空串表示无上界；区间之间允许留空档，
// 表示该 CLI 版本段没有可确认的官方客户端样本。
type stainlessVersionRange struct {
	minVersion string
	maxVersion string
	versions   StainlessVersions
}

// stainlessVersionRanges 来自官方原生二进制与 bundle 的提取结果。
// 更新时保持按 minVersion 升序，并保留空档的原始边界，避免把未抓包的
// 版本段误标成 exact。
var stainlessVersionRanges = []stainlessVersionRange{
	{minVersion: "2.1.193", maxVersion: "2.1.228", versions: StainlessVersions{PackageVersion: "0.94.0", RuntimeVersion: "v26.3.0"}},
	{minVersion: "2.1.228", maxVersion: "2.1.242", versions: StainlessVersions{PackageVersion: "0.112.1", RuntimeVersion: "v26.3.0"}},
	{minVersion: "2.1.281", maxVersion: "2.1.285", versions: StainlessVersions{PackageVersion: "0.112.1", RuntimeVersion: "v26.3.0"}},
	{minVersion: "2.1.285", maxVersion: "2.1.288", versions: StainlessVersions{PackageVersion: "0.127.0", RuntimeVersion: "v26.3.0"}},
	{minVersion: "2.1.288", maxVersion: "2.1.311", versions: StainlessVersions{PackageVersion: "0.128.0", RuntimeVersion: "v26.3.0"}},
	{minVersion: "2.1.311", versions: StainlessVersions{PackageVersion: "0.128.0", RuntimeVersion: "v26.12.1"}},
}

// StainlessVersionsForCLIVersion 返回与给定 CLI 版本同源的 Stainless 版本组合。
// exact 为 false 表示该版本没有直接样本，返回值来自最近的较低版本区间；
// 低于全部区间时取最早一行。调用方应在 floor 抬升后按 exact 决定是否告警。
func StainlessVersionsForCLIVersion(cliVersion string) (versions StainlessVersions, exact bool) {
	version := "v" + strings.TrimSpace(cliVersion)
	if !semver.IsValid(version) || semver.Canonical(version) != version {
		return stainlessVersionRanges[0].versions, false
	}

	var nearest *stainlessVersionRange
	for i := range stainlessVersionRanges {
		r := &stainlessVersionRanges[i]
		if semver.Compare(version, "v"+r.minVersion) < 0 {
			break
		}
		nearest = r
		if r.maxVersion == "" || semver.Compare(version, "v"+r.maxVersion) < 0 {
			return r.versions, true
		}
	}
	if nearest == nil {
		return stainlessVersionRanges[0].versions, false
	}
	return nearest.versions, false
}
