package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// CodexImageGenerationBridgeOverride / CodexImageGenerationExplicitToolPolicy
// moved to domain in Phase 3 (Account BC hybrid). Re-export the policy consts
// and the channel feature key alias so the gateway forwarders compile under the
// original unexported names. Method call sites need no change (type alias).

const featureKeyCodexImageGenerationBridge = domain.FeatureKeyCodexImageGenerationBridge

const (
	codexImageGenerationExplicitToolPolicyAllow = domain.CodexImageGenerationExplicitToolPolicyAllow
	codexImageGenerationExplicitToolPolicyStrip = domain.CodexImageGenerationExplicitToolPolicyStrip
)

const featureKeyCodexImageGenerationExplicitToolPolicy = domain.FeatureKeyCodexImageGenerationExplicitToolPolicy
