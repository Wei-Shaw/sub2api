package service

import "slices"

// GatewayGLMCodingResponsesModelFull is the second model that the GLM Coding
// Plan documents for every tier ("All plans support GLM-5.3"). The enrollment
// anchor stays GatewayGLMCodingResponsesModel.
const GatewayGLMCodingResponsesModelFull = "glm-5.3"

// gatewayCodingPlanModelIDs is the single source of qualified models for the
// protected API-key coding plans that have more than the enrollment anchor. It
// returns a fresh slice so no caller can widen the allowlist. The anchor is
// always first. A profile that returns nil admits only its anchor. A model is
// added only with evidence that it belongs to the plan; an upstream /models
// answer is not evidence (ai-docs/architecture/64 in ReviewRouter).
func gatewayCodingPlanModelIDs(profile string) []string {
	switch profile {
	case GatewayGLMCodingResponsesProfile:
		return []string{GatewayGLMCodingResponsesModel, GatewayGLMCodingResponsesModelFull}
	}
	return nil
}

// GatewayCodingPlanCatalogProfile reports whether the profile has a catalog
// that is larger than its enrollment anchor.
func GatewayCodingPlanCatalogProfile(profile string) bool {
	return gatewayCodingPlanModelIDs(profile) != nil
}

// GatewayCodingPlanModelAdmitted is the one request-time model check for the
// Kimi, GLM and MiniMax coding plans. It is exact string membership; there is no
// prefix, alias or case folding.
func GatewayCodingPlanModelAdmitted(profile, model string) bool {
	if models := gatewayCodingPlanModelIDs(profile); models != nil {
		return slices.Contains(models, model)
	}
	switch profile {
	case GatewayKimiCodeResponsesProfile:
		return model == GatewayKimiCodeResponsesModel
	case GatewayMiniMaxMPlanResponsesProfile:
		return model == GatewayMiniMaxMPlanResponsesModel
	}
	return false
}

// gatewayCodingPlanAccountModels projects the catalog with the only controls
// native supports for these plans: function tools, no reasoning, no fast mode.
func gatewayCodingPlanAccountModels(profile string) []GatewayAccountModel {
	ids := gatewayCodingPlanModelIDs(profile)
	models := make([]GatewayAccountModel, 0, len(ids))
	for _, id := range ids {
		models = append(models, GatewayAccountModel{
			ModelID: id,
			Tools:   GatewayAccountModelChoices{Allowed: []string{"function"}, Default: "function"},
		})
	}
	return models
}
