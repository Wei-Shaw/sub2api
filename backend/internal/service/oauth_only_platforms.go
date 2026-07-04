package service

func isOAuthOnlyRestrictedPlatform(platform string) bool {
	switch platform {
	case PlatformOpenAI, PlatformAntigravity, PlatformAnthropic, PlatformGemini, PlatformKiro, PlatformGrok:
		return true
	default:
		return false
	}
}
