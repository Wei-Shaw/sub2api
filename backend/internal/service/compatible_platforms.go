package service

// CompatiblePlatformResolver resolves cross-platform scheduling compatibility.
// Gateway services use this to determine which non-native platforms can
// participate in mixed scheduling for a given gateway protocol.
type CompatiblePlatformResolver interface {
	// CompatiblePlatforms returns all platform IDs that declared compatibility
	// with the given gateway protocol. Does not include the native platform.
	// Example: CompatiblePlatforms("anthropic") -> ["antigravity", "custom-proxy"]
	CompatiblePlatforms(gatewayProtocol string) []string

	// IsMixedSchedulingPlatform returns true if the platform declared
	// compatibility with any gateway protocol (i.e. it can appear in mixed
	// scheduling buckets for at least one native gateway).
	IsMixedSchedulingPlatform(platform string) bool

	// SupportsProtocol returns true if the platform declared compatibility
	// with the specified gateway protocol.
	SupportsProtocol(platform, gatewayProtocol string) bool

	// AllPlatforms returns all known platform IDs (both native and
	// plugin-provided). Used to enumerate platforms for scheduler bucket
	// rebuilds and snapshot operations.
	AllPlatforms() []string

	// AllGatewayProtocols returns all known gateway protocol identifiers.
	// Used to probe cross-platform compatibility without hardcoding a
	// static list of protocols.
	AllGatewayProtocols() []string
}

// defaultCompatiblePlatformResolver returns a backward-compatible static
// resolver that hardcodes the legacy Antigravity mixed-scheduling rules.
// Used when no plugin registry is wired (e.g. plugins disabled).
func defaultCompatiblePlatformResolver() CompatiblePlatformResolver {
	return &staticCompatiblePlatformResolver{}
}

// staticCompatiblePlatformResolver is the legacy fallback that hardcodes
// Antigravity as compatible with Anthropic and Gemini gateways.
type staticCompatiblePlatformResolver struct{}

func (r *staticCompatiblePlatformResolver) CompatiblePlatforms(gw string) []string {
	if gw == PlatformAnthropic || gw == PlatformGemini {
		return []string{PlatformAntigravity}
	}
	return nil
}

func (r *staticCompatiblePlatformResolver) IsMixedSchedulingPlatform(p string) bool {
	return p == PlatformAntigravity
}

func (r *staticCompatiblePlatformResolver) SupportsProtocol(p, gw string) bool {
	if p == PlatformAntigravity {
		return gw == PlatformAnthropic || gw == PlatformGemini
	}
	return false
}

func (r *staticCompatiblePlatformResolver) AllPlatforms() []string {
	return []string{PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity}
}

func (r *staticCompatiblePlatformResolver) AllGatewayProtocols() []string {
	return []string{PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity}
}
