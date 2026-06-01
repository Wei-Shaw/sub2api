package main

import (
	"encoding/json"
	"fmt"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"net/url"
	"regexp"
	"strings"
)

// --- Vertex Anthropic constants ---

const (
	vertexDefaultLocation     = "us-central1"
	vertexAnthropicVersion    = "vertex-2023-10-16"
	accountTypeServiceAccount = pluginsdk.AccountTypeServiceAccount
)

var (
	vertexLocationPattern                = regexp.MustCompile(`^[a-z0-9-]+$`)
	vertexAnthropicDatedModelIDPattern   = regexp.MustCompile(`^(.+)-([0-9]{8})$`)
	vertexAnthropicAlreadyDatedIDPattern = regexp.MustCompile(`^.+@[0-9]{8}$`)
)

// --- Vertex credentials ---

// vertexCreds holds parsed Vertex AI credential fields.
type vertexCreds struct {
	serviceAccountJSON string            // raw JSON of the Google service account key
	projectID          string            // GCP project ID (override or from SA key)
	location           string            // GCP region (default: us-central1)
	modelLocations     map[string]string // per-model location overrides
}

// parseVertexCredentialsRaw extracts Vertex credential fields from raw JSON.
func parseVertexCredentialsRaw(credentialsJSON []byte) *vertexCreds {
	if len(credentialsJSON) == 0 {
		return &vertexCreds{location: vertexDefaultLocation}
	}
	var raw map[string]any
	if err := json.Unmarshal(credentialsJSON, &raw); err != nil {
		return &vertexCreds{location: vertexDefaultLocation}
	}
	str := func(key string) string {
		v, _ := raw[key].(string)
		return strings.TrimSpace(v)
	}

	c := &vertexCreds{
		projectID: str("project_id"),
		location:  str("location"),
	}
	if c.location == "" {
		c.location = str("vertex_location")
	}
	if c.location == "" {
		c.location = vertexDefaultLocation
	}

	// Extract service account JSON (string or nested object).
	if sa := str("service_account_json"); sa != "" {
		c.serviceAccountJSON = sa
	} else if sa = str("service_account"); sa != "" {
		c.serviceAccountJSON = sa
	} else if nested, ok := raw["service_account_json"].(map[string]any); ok {
		b, _ := json.Marshal(nested)
		c.serviceAccountJSON = string(b)
	} else if nested, ok := raw["service_account"].(map[string]any); ok {
		b, _ := json.Marshal(nested)
		c.serviceAccountJSON = string(b)
	}

	// Per-model location overrides.
	if locs, ok := raw["vertex_model_locations"].(map[string]any); ok {
		c.modelLocations = make(map[string]string, len(locs))
		for k, v := range locs {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				c.modelLocations[k] = strings.TrimSpace(s)
			}
		}
	}

	return c
}

// locationForModel returns the region for a specific model, falling back
// to the account-level location.
func (c *vertexCreds) locationForModel(model string) string {
	if model != "" && len(c.modelLocations) > 0 {
		if loc, ok := c.modelLocations[model]; ok {
			return loc
		}
	}
	return c.location
}

// projectIDFromKey returns the project ID from the credential override,
// or falls back to parsing it from the service account JSON.
func (c *vertexCreds) resolveProjectID() (string, error) {
	if c.projectID != "" {
		return c.projectID, nil
	}
	if c.serviceAccountJSON == "" {
		return "", fmt.Errorf("vertex project_id is required")
	}
	var key struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal([]byte(c.serviceAccountJSON), &key); err != nil {
		return "", fmt.Errorf("parse service account json for project_id: %w", err)
	}
	if strings.TrimSpace(key.ProjectID) == "" {
		return "", fmt.Errorf("service account json missing project_id")
	}
	return strings.TrimSpace(key.ProjectID), nil
}

// --- Vertex URL building ---

// buildVertexAnthropicURL constructs the Vertex AI rawPredict endpoint URL
// for Anthropic models.
func buildVertexAnthropicURL(projectID, location, model string, stream bool) (string, error) {
	projectID = strings.TrimSpace(projectID)
	location = strings.TrimSpace(location)
	model = strings.TrimSpace(model)
	if projectID == "" {
		return "", fmt.Errorf("vertex project_id is required")
	}
	if location == "" {
		location = vertexDefaultLocation
	}
	if !vertexLocationPattern.MatchString(location) {
		return "", fmt.Errorf("invalid vertex location: %s", location)
	}
	if model == "" {
		return "", fmt.Errorf("vertex model is required")
	}

	action := "rawPredict"
	if stream {
		action = "streamRawPredict"
	}
	host := fmt.Sprintf("%s-aiplatform.googleapis.com", location)
	if location == "global" {
		host = "aiplatform.googleapis.com"
	}
	escapedModel := strings.ReplaceAll(url.PathEscape(model), "%40", "@")
	return fmt.Sprintf(
		"https://%s/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:%s",
		host,
		url.PathEscape(projectID),
		url.PathEscape(location),
		escapedModel,
		action,
	), nil
}

// --- Vertex request body ---

// buildVertexAnthropicRequestBody transforms the Anthropic Messages API body
// for Vertex: removes "model", adds "anthropic_version".
func buildVertexAnthropicRequestBody(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse vertex request body: %w", err)
	}
	delete(payload, "model")
	payload["anthropic_version"] = vertexAnthropicVersion
	return json.Marshal(payload)
}

// --- Vertex model normalization ---

// normalizeVertexAnthropicModelID converts dated model IDs from hyphen format
// (claude-sonnet-4-5-20250929) to at-sign format (claude-sonnet-4-5@20250929)
// as required by Vertex AI.
func normalizeVertexAnthropicModelID(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || vertexAnthropicAlreadyDatedIDPattern.MatchString(model) {
		return model
	}
	if m := vertexAnthropicDatedModelIDPattern.FindStringSubmatch(model); len(m) == 3 {
		return m[1] + "@" + m[2]
	}
	return model
}

// resolveVertexModel applies model mapping and Vertex normalization.
func resolveVertexModel(requestedModel string, credentialsJSON []byte) string {
	// Check account-level model_mapping first.
	if mm := gatewayutil.ExtractModelMapping(credentialsJSON); len(mm) > 0 {
		if mapped, ok := mm[requestedModel]; ok {
			return normalizeVertexAnthropicModelID(mapped)
		}
	}
	// Default: normalize the requested model for Vertex format.
	return normalizeVertexAnthropicModelID(requestedModel)
}
