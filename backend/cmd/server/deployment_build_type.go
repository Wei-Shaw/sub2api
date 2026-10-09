package main

import "strings"

// deploymentBuildType preserves the build identity unless the operator selects
// immutable deployment updates. Version checks still use the official release.
func deploymentBuildType(buildType, updateMode string) string {
	if strings.EqualFold(strings.TrimSpace(updateMode), "managed") {
		return "managed"
	}
	return buildType
}
