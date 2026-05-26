package service

import "testing"

func TestGatewayServiceIsModelSupportedByAccount_BedrockDefaultMappingRestrictsModels(t *testing.T) {
	svc := &GatewayService{}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeBedrock,
		Credentials: map[string]any{
			"aws_region": "us-east-1",
		},
	}

	if !svc.isModelSupportedByAccount(account, "claude-sonnet-4-5") {
		t.Fatalf("expected default Bedrock alias to be supported")
	}

	if svc.isModelSupportedByAccount(account, "claude-3-5-sonnet-20241022") {
		t.Fatalf("expected unsupported alias to be rejected for Bedrock account")
	}
}

func TestGatewayServiceIsModelSupportedByAccount_BedrockCustomMappingStillActsAsAllowlist(t *testing.T) {
	svc := &GatewayService{}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeBedrock,
		Credentials: map[string]any{
			"aws_region": "eu-west-1",
			"model_mapping": map[string]any{
				"claude-sonnet-*": "claude-sonnet-4-6",
			},
		},
	}

	if !svc.isModelSupportedByAccount(account, "claude-sonnet-4-6") {
		t.Fatalf("expected matched custom mapping to be supported")
	}

	if !svc.isModelSupportedByAccount(account, "claude-opus-4-6") {
		t.Fatalf("expected default Bedrock alias fallback to remain supported")
	}

	if svc.isModelSupportedByAccount(account, "claude-3-5-sonnet-20241022") {
		t.Fatalf("expected unsupported model to still be rejected")
	}
}
