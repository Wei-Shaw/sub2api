package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
)

// --- Bedrock constants ---

const defaultBedrockRegion = "us-east-1"

// bedrockCrossRegionPrefixes lists all Bedrock cross-region inference prefixes.
var bedrockCrossRegionPrefixes = []string{
	"us.", "eu.", "apac.", "jp.", "au.", "us-gov.", "global.",
}

// defaultBedrockModelMapping maps standard Claude model names to Bedrock model IDs.
// The "us." prefix is the default; resolveBedrockModel adjusts it per account region.
var defaultBedrockModelMapping = map[string]string{
	// Claude Opus
	"claude-opus-4-7":          "us.anthropic.claude-opus-4-7-v1",
	"claude-opus-4-6-thinking": "us.anthropic.claude-opus-4-6-v1",
	"claude-opus-4-6":          "us.anthropic.claude-opus-4-6-v1",
	"claude-opus-4-5-thinking": "us.anthropic.claude-opus-4-5-20251101-v1:0",
	"claude-opus-4-5-20251101": "us.anthropic.claude-opus-4-5-20251101-v1:0",
	"claude-opus-4-1":          "us.anthropic.claude-opus-4-1-20250805-v1:0",
	"claude-opus-4-20250514":   "us.anthropic.claude-opus-4-20250514-v1:0",
	// Claude Sonnet
	"claude-sonnet-4-6-thinking": "us.anthropic.claude-sonnet-4-6",
	"claude-sonnet-4-6":          "us.anthropic.claude-sonnet-4-6",
	"claude-sonnet-4-5":          "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-5-thinking": "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-5-20250929": "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-20250514":   "us.anthropic.claude-sonnet-4-20250514-v1:0",
	// Claude Haiku
	"claude-haiku-4-5":          "us.anthropic.claude-haiku-4-5-20251001-v1:0",
	"claude-haiku-4-5-20251001": "us.anthropic.claude-haiku-4-5-20251001-v1:0",
}

// --- Bedrock URL building ---

// buildBedrockURL constructs the Bedrock InvokeModel endpoint URL.
// stream=true uses the invoke-with-response-stream endpoint.
func buildBedrockURL(region, modelID string, stream bool) string {
	if region == "" {
		region = defaultBedrockRegion
	}
	encoded := url.PathEscape(modelID)
	// url.PathEscape does not encode colons; Bedrock expects %3A.
	encoded = strings.ReplaceAll(encoded, ":", "%3A")
	if stream {
		return fmt.Sprintf(
			"https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke-with-response-stream",
			region, encoded,
		)
	}
	return fmt.Sprintf(
		"https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke",
		region, encoded,
	)
}

// --- Bedrock model resolution ---

// bedrockCrossRegionPrefix returns the inference profile prefix for a region.
func bedrockCrossRegionPrefix(region string) string {
	switch {
	case strings.HasPrefix(region, "us-gov"):
		return "us-gov"
	case strings.HasPrefix(region, "us-"):
		return "us"
	case strings.HasPrefix(region, "eu-"):
		return "eu"
	case region == "ap-northeast-1":
		return "jp"
	case region == "ap-southeast-2":
		return "au"
	case strings.HasPrefix(region, "ap-"):
		return "apac"
	case strings.HasPrefix(region, "ca-"), strings.HasPrefix(region, "sa-"):
		return "us"
	default:
		return "us"
	}
}

// adjustBedrockModelRegionPrefix replaces the cross-region prefix in a model
// ID to match the target region. "global" forces the global. prefix.
func adjustBedrockModelRegionPrefix(modelID, region string) string {
	targetPrefix := bedrockCrossRegionPrefix(region)
	if region == "global" {
		targetPrefix = "global"
	}
	for _, p := range bedrockCrossRegionPrefixes {
		if strings.HasPrefix(modelID, p) {
			if p == targetPrefix+"." {
				return modelID
			}
			return targetPrefix + "." + modelID[len(p):]
		}
	}
	return modelID
}

func isRegionalBedrockModelID(modelID string) bool {
	for _, prefix := range bedrockCrossRegionPrefixes {
		if strings.HasPrefix(modelID, prefix) {
			return true
		}
	}
	return false
}

func isLikelyBedrockModelID(modelID string) bool {
	lower := strings.ToLower(strings.TrimSpace(modelID))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, "arn:") {
		return true
	}
	for _, prefix := range []string{
		"anthropic.", "amazon.", "meta.", "mistral.", "cohere.",
		"ai21.", "deepseek.", "stability.", "writer.", "nova.",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return isRegionalBedrockModelID(lower)
}

// resolveBedrockModel resolves a requested model to a Bedrock model ID,
// applying account model_mapping, default mapping, and region prefix adjustment.
func resolveBedrockModel(requestedModel string, credentialsJSON []byte) (string, bool) {
	creds := parseBedrockCredentialsRaw(credentialsJSON)

	// Apply account-level model_mapping first.
	mappedModel := requestedModel
	if mm := gatewayutil.ExtractModelMapping(credentialsJSON); len(mm) > 0 {
		if m, ok := mm[requestedModel]; ok {
			mappedModel = m
		}
	}

	// Normalise to Bedrock model ID.
	modelID, shouldAdjust, ok := normalizeBedrockModelID(mappedModel)
	if !ok {
		return "", false
	}

	// Adjust region prefix.
	if shouldAdjust {
		region := creds.region
		if region == "" {
			region = defaultBedrockRegion
		}
		if creds.forceGlobal {
			region = "global"
		}
		modelID = adjustBedrockModelRegionPrefix(modelID, region)
	}
	return modelID, true
}

func normalizeBedrockModelID(modelID string) (normalized string, shouldAdjustRegion bool, ok bool) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return "", false, false
	}
	if mapped, exists := defaultBedrockModelMapping[modelID]; exists {
		return mapped, true, true
	}
	if isRegionalBedrockModelID(modelID) {
		return modelID, true, true
	}
	if isLikelyBedrockModelID(modelID) {
		return modelID, false, true
	}
	return "", false, false
}

// --- Bedrock credentials ---

type bedrockCreds struct {
	apiKey          string // Bedrock cross-region API Key (Bearer token)
	accessKeyID     string // AWS SigV4 access key
	secretAccessKey string // AWS SigV4 secret key
	sessionToken    string // AWS STS session token (optional)
	region          string
	forceGlobal     bool
}

// parseBedrockCredentialsRaw extracts Bedrock credential fields from raw JSON.
func parseBedrockCredentialsRaw(credentialsJSON []byte) *bedrockCreds {
	if len(credentialsJSON) == 0 {
		return &bedrockCreds{region: defaultBedrockRegion}
	}
	var raw map[string]any
	if err := json.Unmarshal(credentialsJSON, &raw); err != nil {
		return &bedrockCreds{region: defaultBedrockRegion}
	}
	str := func(key string) string {
		v, _ := raw[key].(string)
		return strings.TrimSpace(v)
	}
	c := &bedrockCreds{
		apiKey:          str("api_key"),
		accessKeyID:     str("aws_access_key_id"),
		secretAccessKey: str("aws_secret_access_key"),
		sessionToken:    str("aws_session_token"),
		region:          str("aws_region"),
		forceGlobal:     str("aws_force_global") == "true",
	}
	if c.region == "" {
		c.region = defaultBedrockRegion
	}
	return c
}

// isBedrockAPIKey returns true when the account uses API Key auth (not SigV4).
func (c *bedrockCreds) isAPIKey() bool {
	return c.apiKey != ""
}
