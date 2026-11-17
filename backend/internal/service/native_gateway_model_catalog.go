package service

import "slices"

// GatewayGLMCodingResponsesModelFull is the second model that the GLM Coding
// Plan documents for every tier ("All plans support GLM-5.3"). The enrollment
// anchor stays GatewayGLMCodingResponsesModel.
const GatewayGLMCodingResponsesModelFull = "glm-5.3"

// gatewayCodingPlanStaticModelIDs is the static, documented lower bound of the
// protected API-key coding plans: the enrollment anchor first, then models the
// provider documents for the plan. It is returned as a fresh slice so no caller
// can widen it. The provider's live list (native_gateway_live_models.go) is added
// on top of it, never instead of it. A profile that returns nil is not a coding
// plan with a catalogue.
func gatewayCodingPlanStaticModelIDs(profile string) []string {
	switch profile {
	case GatewayGLMCodingResponsesProfile:
		return []string{GatewayGLMCodingResponsesModel, GatewayGLMCodingResponsesModelFull}
	case GatewayKimiCodeResponsesProfile:
		return []string{GatewayKimiCodeResponsesModel}
	case GatewayMiniMaxMPlanResponsesProfile:
		return []string{GatewayMiniMaxMPlanResponsesModel}
	}
	return nil
}

// GatewayCodingPlanCatalogProfile reports whether the profile serves a model
// catalogue (static lower bound plus the provider's live list).
func GatewayCodingPlanCatalogProfile(profile string) bool {
	return gatewayCodingPlanStaticModelIDs(profile) != nil
}

// GatewayCodingPlanStaticModelAdmitted is exact string membership in the static
// lower bound; there is no prefix, alias or case folding. Dispatch combines it
// with the live list in gatewayCodingPlanAdmitted.
func GatewayCodingPlanStaticModelAdmitted(profile, model string) bool {
	return slices.Contains(gatewayCodingPlanStaticModelIDs(profile), model)
}

// gatewayCodingPlanAccountModels projects the catalogue with the only controls
// native supports for these plans: function tools, no reasoning, no fast mode.
func gatewayCodingPlanAccountModels(profile, anchor string, live []string) []GatewayAccountModel {
	ids := gatewayCodingPlanCatalogue(profile, anchor, live)
	models := make([]GatewayAccountModel, 0, len(ids))
	for _, id := range ids {
		models = append(models, GatewayAccountModel{
			ModelID: id,
			Tools:   GatewayAccountModelChoices{Allowed: []string{"function"}, Default: "function"},
		})
	}
	return models
}
