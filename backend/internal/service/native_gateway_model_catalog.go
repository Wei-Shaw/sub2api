package service

import (
	"slices"
	"strings"
)

// GatewayGLMCodingResponsesModelFull is the second model that the GLM Coding
// Plan documents for every tier ("All plans support GLM-5.3"). The enrollment
// anchor stays GatewayGLMCodingResponsesModel.
const GatewayGLMCodingResponsesModelFull = "glm-5.3"

// gatewayCatalogStaticModelIDs is the static lower bound of the protected
// API-key profiles whose catalogue native owns: the enrollment anchor first,
// then models the provider documents for the plan. It is returned as a fresh
// slice so no caller can widen it. The provider's live list
// (native_gateway_live_models.go) is added on top of it, never instead of it. A
// profile that returns nil has no such catalogue.
func gatewayCatalogStaticModelIDs(profile string) []string {
	switch profile {
	case GatewayGLMCodingResponsesProfile:
		return []string{GatewayGLMCodingResponsesModel, GatewayGLMCodingResponsesModelFull}
	case GatewayKimiCodeResponsesProfile:
		return []string{GatewayKimiCodeResponsesModel}
	case GatewayMiniMaxMPlanResponsesProfile:
		return []string{GatewayMiniMaxMPlanResponsesModel}
	case GatewayNVIDIABuildChatProfile:
		return []string{GatewayNVIDIABuildAnchorModel}
	}
	return nil
}

// GatewayModelCatalogProfile reports whether the profile serves a native model
// catalogue (static lower bound plus the provider's live list): the API-key
// coding plans and NVIDIA Build.
func GatewayModelCatalogProfile(profile string) bool {
	return gatewayCatalogStaticModelIDs(profile) != nil
}

// gatewayModelCatalogSource names the snapshot source of a catalogue profile in
// the Gateway contract.
func gatewayModelCatalogSource(profile string) string {
	if profile == GatewayNVIDIABuildChatProfile {
		return "nvidia"
	}
	return "native_catalog"
}

// GatewayCatalogStaticModelAdmitted is exact string membership in the static
// lower bound; there is no prefix, alias or case folding. Dispatch combines it
// with the live list in gatewayModelCatalogueAdmitted.
func GatewayCatalogStaticModelAdmitted(profile, model string) bool {
	return slices.Contains(gatewayCatalogStaticModelIDs(profile), model)
}

// gatewayReviewModelID keeps a provider-listed model only when its ID is
// structurally valid and does not name a family that cannot review code:
// embeddings, rerankers, retrievers, safety and guard classifiers, reward models,
// image encoders, detectors and translators. This is the one filter for every
// live provider list; markers instead of exact IDs also drop new models of the
// same families (a marker is matched anywhere in the ID, so "nemoguard" and
// "nvclip" are caught too). The static lower bound is never filtered.
func gatewayReviewModelID(id string) bool {
	if !GatewayAccountModelIDValid(id) {
		return false
	}
	lower := strings.ToLower(id)
	for _, marker := range [...]string{"embed", "rerank", "retriev", "guard", "shield", "safety", "reward", "clip", "detect", "translat"} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

// gatewayCatalogAccountModels projects the catalogue with the only controls
// native supports for these profiles: function tools, no reasoning, no fast mode.
func gatewayCatalogAccountModels(profile, anchor string, live []string) []GatewayAccountModel {
	ids := gatewayModelCatalogue(profile, anchor, live)
	models := make([]GatewayAccountModel, 0, len(ids))
	for _, id := range ids {
		models = append(models, GatewayAccountModel{
			ModelID: id,
			Tools:   GatewayAccountModelChoices{Allowed: []string{"function"}, Default: "function"},
		})
	}
	return models
}
