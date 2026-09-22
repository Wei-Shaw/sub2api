package service

import "strings"

// codexAppServerClient builds App Server identity fields from the outbound client fingerprint.
func codexAppServerClient(profile codexTelemetryProfile) map[string]any {
	clientName, appVersion, _, _, _ := codexUserAgentParts(profile.client.userAgent, profile.client.version)
	transport := "in_process"
	if strings.EqualFold(clientName, "Codex Desktop") {
		transport = "stdio"
	}
	return map[string]any{"product_client_id": clientName, "client_name": clientName, "client_version": appVersion, "rpc_transport": transport, "experimental_api_enabled": true}
}

// codexRuntime derives Codex runtime attributes from the outbound User-Agent.
func codexRuntime(profile codexTelemetryProfile) map[string]any {
	_, _, osName, osVersion, arch := codexUserAgentParts(profile.client.userAgent, profile.client.version)
	runtimeOS := strings.ToLower(osName)
	switch runtimeOS {
	case "mac os":
		runtimeOS = "macos"
	case "ubuntu", "debian", "arch linux":
		runtimeOS = "linux"
	}
	return map[string]any{"codex_rs_version": profile.client.version, "runtime_os": runtimeOS, "runtime_os_version": osVersion, "runtime_arch": arch}
}

// codexClientName returns the product client name used in telemetry events.
func codexClientName(profile codexTelemetryProfile) string {
	name, _, _, _, _ := codexUserAgentParts(profile.client.userAgent, profile.client.version)
	return name
}

// codexUserAgentParts parses client, OS, and architecture fields from a Codex User-Agent.
func codexUserAgentParts(userAgent, fallbackVersion string) (client, appVersion, osName, osVersion, arch string) {
	client = strings.TrimSpace(strings.SplitN(userAgent, "/", 2)[0])
	appVersion = fallbackVersion
	open, close := strings.Index(userAgent, "("), strings.Index(userAgent, ")")
	if open >= 0 && close > open {
		platform := strings.SplitN(userAgent[open+1:close], ";", 2)
		osName, osVersion = splitCodexOS(strings.TrimSpace(platform[0]))
		if len(platform) == 2 {
			arch = strings.TrimSpace(platform[1])
		}
	}
	if lastOpen := strings.LastIndex(userAgent, "("); lastOpen > open && strings.HasSuffix(userAgent, ")") {
		app := strings.SplitN(userAgent[lastOpen+1:len(userAgent)-1], ";", 2)
		if len(app) == 2 {
			appVersion = strings.TrimSpace(app[1])
		}
	}
	return
}

// splitCodexOS splits a platform description into OS name and version.
func splitCodexOS(platform string) (string, string) {
	for _, name := range []string{"Mac OS", "Windows", "Ubuntu", "Linux", "Debian", "Arch Linux"} {
		if strings.HasPrefix(platform, name+" ") {
			return name, strings.TrimSpace(strings.TrimPrefix(platform, name))
		}
	}
	parts := strings.SplitN(platform, " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return platform, "Unknown"
}
