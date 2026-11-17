//go:build unit

package service

import "testing"

func TestGatewayNVIDIABuildProfileIsFixedAndIndependent(t *testing.T) {
	if !GatewayNativeAPIKeyProfileValid(GatewayNVIDIABuildChatProfile, GatewayNVIDIABuildBaseURL, GatewayNVIDIABuildAnchorModel) {
		t.Fatal("fixed NVIDIA profile must be valid")
	}
	// The historical MiMo Chat bridge accepts variable endpoints by design;
	// other fixed-vendor profiles must never accept the NVIDIA tuple.
	for _, profile := range []string{GatewayOpenRouterResponsesProfile, GatewayGLMCodingResponsesProfile} {
		if GatewayNativeAPIKeyProfileValid(profile, GatewayNVIDIABuildBaseURL, GatewayNVIDIABuildAnchorModel) {
			t.Fatalf("another provider must not accept NVIDIA credentials: %s", profile)
		}
	}
	for _, candidate := range []struct{ url, anchor string }{
		{"https://other.example/v1", GatewayNVIDIABuildAnchorModel},
		{"http://integrate.api.nvidia.com/v1", GatewayNVIDIABuildAnchorModel},
		{"https://integrate.api.nvidia.com/v1/", GatewayNVIDIABuildAnchorModel},
		{"https://integrate.api.nvidia.com/v1?redirect=true", GatewayNVIDIABuildAnchorModel},
		{GatewayNVIDIABuildBaseURL, "z-ai/glm-5.3-flash"},
		{GatewayNVIDIABuildBaseURL, "deepseek-ai/latest"},
	} {
		if GatewayNativeAPIKeyProfileValid(GatewayNVIDIABuildChatProfile, candidate.url, candidate.anchor) {
			t.Fatalf("unqualified NVIDIA tuple accepted: %+v", candidate)
		}
	}
}
