package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	configGuideJSONContentType     = "application/json; charset=utf-8"
	configGuideYAMLContentType     = "application/yaml; charset=utf-8"
	configGuideTextContentType     = "text/plain; charset=utf-8"
	configGuideMarkdownContentType = "text/markdown; charset=utf-8"
)

type ConfigGuideHandler struct {
	openCodeMetadataService *service.OpenCodeMetadataService
	now                     func() time.Time
}

func NewConfigGuideHandler(openCodeMetadataService *service.OpenCodeMetadataService) *ConfigGuideHandler {
	return &ConfigGuideHandler{
		openCodeMetadataService: openCodeMetadataService,
		now:                     time.Now,
	}
}

type configGuideManifest struct {
	SchemaVersion int               `json:"schema_version"`
	Client        string            `json:"client"`
	Title         string            `json:"title"`
	GeneratedAt   string            `json:"generated_at"`
	BaseURL       string            `json:"base_url"`
	Items         []configGuideItem `json:"items"`
	Notes         []string          `json:"notes,omitempty"`
}

type configGuideItem struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Method      string  `json:"method"`
	URL         string  `json:"url"`
	TargetPath  *string `json:"target_path"`
	ContentType string  `json:"content_type"`
}

type configGuideQueryParams struct {
	apiKey          string
	baseURL         string
	explicitBaseURL string
}

type ompModelConfig struct {
	ID            string
	Name          string
	API           string
	Reasoning     bool
	Input         []string
	ContextWindow int
	MaxTokens     int
	Cost          ompModelCost
}

type ompModelCost struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

func setConfigGuideNoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
}

func (h *ConfigGuideHandler) currentTime() time.Time {
	if h != nil && h.now != nil {
		return h.now()
	}
	return time.Now()
}

func (h *ConfigGuideHandler) GetOMPManifest(c *gin.Context) {
	setConfigGuideNoStore(c)

	params, ok := h.configGuideQuery(c)
	if !ok {
		return
	}
	pluginVersion, ok := h.requireOMPProviderToolsVersion(c)
	if !ok {
		return
	}
	_ = pluginVersion
	models, ok := h.requireOpenAIModels(c)
	if !ok {
		return
	}
	if !requireModels(c, models, []string{"gpt-5.5", "gpt-5.4-mini"}) {
		return
	}

	c.JSON(http.StatusOK, configGuideManifest{
		SchemaVersion: 1,
		Client:        "omp",
		Title:         "sub2api OpenAI for OMP",
		GeneratedAt:   h.currentTime().UTC().Format(time.RFC3339),
		BaseURL:       params.baseURL,
		Items: []configGuideItem{
			{
				ID:          "plugin",
				Kind:        "instructions",
				Method:      http.MethodGet,
				URL:         configGuideItemURL("/config-guides/omp-openai/plugin.txt", params),
				TargetPath:  nil,
				ContentType: configGuideTextContentType,
			},
			{
				ID:          "models",
				Kind:        "file",
				Method:      http.MethodGet,
				URL:         configGuideItemURL("/config-guides/omp-openai/models.yml", params),
				TargetPath:  strPtr("~/.omp/agent/models.yml"),
				ContentType: configGuideYAMLContentType,
			},
			{
				ID:          "config",
				Kind:        "file",
				Method:      http.MethodGet,
				URL:         configGuideItemURL("/config-guides/omp-openai/config.yml", params),
				TargetPath:  strPtr("~/.omp/agent/config.yml"),
				ContentType: configGuideYAMLContentType,
			},
			{
				ID:          "image-generator",
				Kind:        "file",
				Method:      http.MethodGet,
				URL:         configGuideItemURL("/config-guides/omp-openai/image-generator.md", params),
				TargetPath:  strPtr("~/.omp/agent/agents/image-generator.md"),
				ContentType: configGuideMarkdownContentType,
			},
		},
		Notes: []string{
			"Run plugin.txt commands before using provider-native web_search or image_generation.",
			"Restart OMP after installing or upgrading plugins and writing agent files.",
		},
	})
}

func (h *ConfigGuideHandler) GetOMPPluginInstructions(c *gin.Context) {
	setConfigGuideNoStore(c)

	if _, ok := h.configGuideQuery(c); !ok {
		return
	}
	pluginVersion, ok := h.requireOMPProviderToolsVersion(c)
	if !ok {
		return
	}

	c.Data(http.StatusOK, configGuideTextContentType, []byte(renderOMPPluginInstructions(pluginVersion)))
}

func (h *ConfigGuideHandler) GetOMPModelsYAML(c *gin.Context) {
	setConfigGuideNoStore(c)

	params, ok := h.configGuideQuery(c)
	if !ok {
		return
	}
	pluginVersion, ok := h.requireOMPProviderToolsVersion(c)
	if !ok {
		return
	}
	models, ok := h.requireOpenAIModels(c)
	if !ok {
		return
	}
	if !requireModels(c, models, []string{"gpt-5.5", "gpt-5.4-mini"}) {
		return
	}

	content, err := renderOMPModelsYAML(params.baseURL, params.apiKey, pluginVersion, models)
	if err != nil {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenAI model metadata incomplete")
		return
	}
	c.Data(http.StatusOK, configGuideYAMLContentType, []byte(content))
}

func (h *ConfigGuideHandler) GetOMPConfigYAML(c *gin.Context) {
	setConfigGuideNoStore(c)

	if _, ok := h.configGuideQuery(c); !ok {
		return
	}
	c.Data(http.StatusOK, configGuideYAMLContentType, []byte(renderOMPSettingsYAML()))
}

func (h *ConfigGuideHandler) GetOMPImageGenerator(c *gin.Context) {
	setConfigGuideNoStore(c)

	if _, ok := h.configGuideQuery(c); !ok {
		return
	}
	c.Data(http.StatusOK, configGuideMarkdownContentType, []byte(renderOMPImageGeneratorMarkdown()))
}

func (h *ConfigGuideHandler) GetOpenCodeManifest(c *gin.Context) {
	setConfigGuideNoStore(c)

	params, ok := h.configGuideQuery(c)
	if !ok {
		return
	}
	models, ok := h.requireOpenAIModels(c)
	if !ok {
		return
	}
	if !requireModels(c, models, []string{"gpt-5.5", "gpt-5.5-fast"}) {
		return
	}

	c.JSON(http.StatusOK, configGuideManifest{
		SchemaVersion: 1,
		Client:        "opencode",
		Title:         "sub2api OpenAI for OpenCode",
		GeneratedAt:   h.currentTime().UTC().Format(time.RFC3339),
		BaseURL:       params.baseURL,
		Items: []configGuideItem{
			{
				ID:          "opencode",
				Kind:        "file",
				Method:      http.MethodGet,
				URL:         configGuideItemURL("/config-guides/opencode-openai/opencode.json", params),
				TargetPath:  strPtr("~/.config/opencode/opencode.json"),
				ContentType: configGuideJSONContentType,
			},
		},
		Notes: []string{
			"This config adds provider sub2api-openai and does not replace OpenCode built-in openai provider.",
		},
	})
}

func (h *ConfigGuideHandler) GetOpenCodeJSON(c *gin.Context) {
	setConfigGuideNoStore(c)

	params, ok := h.configGuideQuery(c)
	if !ok {
		return
	}
	models, ok := h.requireOpenAIModels(c)
	if !ok {
		return
	}
	if !requireModels(c, models, []string{"gpt-5.5", "gpt-5.5-fast"}) {
		return
	}

	content, err := renderOpenCodeOpenAIConfig(params.baseURL, params.apiKey, models)
	if err != nil {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenAI model metadata incomplete")
		return
	}
	c.Data(http.StatusOK, configGuideJSONContentType, content)
}

func (h *ConfigGuideHandler) configGuideQuery(c *gin.Context) (configGuideQueryParams, bool) {
	apiKey := strings.TrimSpace(c.Query("api_key"))
	if apiKey == "" || containsControlCharacter(apiKey) {
		writeConfigGuideError(c, http.StatusBadRequest, "api_key is required")
		return configGuideQueryParams{}, false
	}

	baseURL, explicitBaseURL, err := buildConfigGuideBaseURL(c)
	if err != nil {
		writeConfigGuideError(c, http.StatusBadRequest, "invalid base_url")
		return configGuideQueryParams{}, false
	}

	return configGuideQueryParams{
		apiKey:          apiKey,
		baseURL:         baseURL,
		explicitBaseURL: explicitBaseURL,
	}, true
}

func buildConfigGuideBaseURL(c *gin.Context) (baseURL string, explicitBaseURL string, err error) {
	if _, ok := c.GetQuery("base_url"); ok {
		raw := strings.TrimSpace(c.Query("base_url"))
		if raw == "" || containsControlCharacter(raw) {
			return "", "", errors.New("invalid explicit base_url")
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", "", err
		}
		scheme := strings.ToLower(parsed.Scheme)
		if scheme != "http" && scheme != "https" {
			return "", "", errors.New("invalid explicit base_url scheme")
		}
		if strings.TrimSpace(parsed.Host) == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", "", errors.New("invalid explicit base_url components")
		}
		return strings.TrimRight(raw, "/"), raw, nil
	}

	return deriveConfigGuideBaseURL(c)
}

func deriveConfigGuideBaseURL(c *gin.Context) (string, string, error) {
	scheme := ""
	if c.Request != nil {
		firstForwardedProto := strings.ToLower(strings.TrimSpace(firstForwardedValue(c.Request.Header.Get("X-Forwarded-Proto"))))
		if firstForwardedProto == "http" || firstForwardedProto == "https" {
			scheme = firstForwardedProto
		}
	}
	if scheme == "" {
		if c.Request != nil && c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	host := ""
	if c.Request != nil {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" || containsControlCharacter(host) {
		return "", "", errors.New("invalid request host")
	}

	return strings.TrimRight(scheme+"://"+host, "/") + "/v1", "", nil
}

func firstForwardedValue(raw string) string {
	if idx := strings.Index(raw, ","); idx >= 0 {
		return raw[:idx]
	}
	return raw
}

func configGuideItemURL(path string, params configGuideQueryParams) string {
	query := url.Values{}
	query.Set("api_key", params.apiKey)
	if params.explicitBaseURL != "" {
		query.Set("base_url", params.explicitBaseURL)
	}
	return path + "?" + query.Encode()
}

func (h *ConfigGuideHandler) requireOMPProviderToolsVersion(c *gin.Context) (string, bool) {
	if h == nil || h.openCodeMetadataService == nil {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenCode metadata service unavailable")
		return "", false
	}
	metadata := h.openCodeMetadataService.GetOMPProviderToolsMetadata(c.Request.Context())
	version := strings.TrimSpace(metadata.LatestVersion)
	status := strings.ToLower(strings.TrimSpace(metadata.Status))
	if version == "" || (status != "ok" && status != "cached") {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OMP provider tools metadata unavailable")
		return "", false
	}
	return version, true
}

func (h *ConfigGuideHandler) requireOpenAIModels(c *gin.Context) (map[string]service.OpenCodeOpenAIModel, bool) {
	if h == nil || h.openCodeMetadataService == nil {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenCode metadata service unavailable")
		return nil, false
	}
	models, err := h.openCodeMetadataService.GetOpenAIModels(c.Request.Context())
	if err != nil || len(models) == 0 {
		writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenCode OpenAI model metadata unavailable")
		return nil, false
	}
	return models, true
}

func requireModels(c *gin.Context, models map[string]service.OpenCodeOpenAIModel, ids []string) bool {
	for _, id := range ids {
		if _, ok := models[id]; !ok {
			writeConfigGuideError(c, http.StatusServiceUnavailable, "OpenAI model metadata incomplete")
			return false
		}
	}
	return true
}

func writeConfigGuideError(c *gin.Context, status int, message string) {
	response.Error(c, status, message)
}

func renderOMPPluginInstructions(version string) string {
	return fmt.Sprintf(`# 1. Install or upgrade provider-native tools plugin
omp plugin install npm:omp-openai-provider-tools@%s

# 2. Check plugin health
omp plugin doctor

# 3. Preview the recommended image subagent template
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run

# 4. After reviewing the preview, write ~/.omp/agent/agents/image-generator.md
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys

# If image_generator already exists, the command refuses to overwrite it.
# Use --print to inspect and merge manually; use --force only when you intentionally replace it.
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --print`, version)
}

func renderOMPModelsYAML(baseURL, apiKey, pluginVersion string, models map[string]service.OpenCodeOpenAIModel) (string, error) {
	baseModels := make(map[string]ompModelConfig, len(models))
	for id, model := range models {
		baseModels[id] = normalizeOMPModelConfig(model)
	}
	expanded := withOMPSysVariants(baseModels)

	selectedIDs := []string{"gpt-5.5", "gpt-5.5-Sys", "gpt-5.4-mini", "gpt-5.4-mini-Sys"}
	var selected []string
	for _, id := range selectedIDs {
		model, ok := expanded[id]
		if !ok {
			return "", fmt.Errorf("required OMP model missing: %s", id)
		}
		selected = append(selected, renderOMPModelYAML(model, "      ", nil))
	}

	imageSource, ok := expanded["gpt-5.5-Sys"]
	if !ok {
		return "", fmt.Errorf("required OMP image source missing")
	}
	imageSource.ID = "gpt-5.5-Sys"
	imageSource.Name = "GPT-5.5 Image (Sys)"
	imageYAML := renderOMPModelYAML(imageSource, "      ", []string{
		"        compat:",
		"          openaiProviderTools:",
		"            imageGeneration: true",
	})

	return fmt.Sprintf(`# Image generation and provider-native web_search require this plugin:
#   omp plugin install npm:omp-openai-provider-tools@%s
#   omp plugin doctor
# Recommended image subagent command:
#   npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run
# Restart OMP after installing or upgrading the plugin.
providers:
  sub2api-openai:
    api: openai-responses
    baseUrl: %s
    apiKey: %s
    compat:
      openaiProviderTools:
        enabled: true
    models:
%s

  sub2api-openai-image:
    api: openai-responses
    baseUrl: %s
    apiKey: %s
    compat:
      openaiProviderTools:
        enabled: true
    models:
%s

equivalence:
  overrides:
    sub2api-openai/gpt-5.5: gpt-5.5
    sub2api-openai/gpt-5.5-Sys: gpt-5.5-sys
    sub2api-openai/gpt-5.4-mini: gpt-5.4-mini
    sub2api-openai/gpt-5.4-mini-Sys: gpt-5.4-mini-sys
    sub2api-openai-image/gpt-5.5-Sys: gpt-5.5-image-sys`, pluginVersion, baseURL, apiKey, strings.Join(selected, "\n"), baseURL, apiKey, imageYAML), nil
}

func normalizeOMPModelConfig(model service.OpenCodeOpenAIModel) ompModelConfig {
	input := make([]string, 0, len(model.Modalities.Input))
	for _, item := range model.Modalities.Input {
		if item == "text" || item == "image" {
			input = append(input, item)
		}
	}
	if len(input) == 0 {
		input = []string{"text"}
	}
	return ompModelConfig{
		ID:            model.ID,
		Name:          model.Name,
		API:           "openai-responses",
		Reasoning:     model.Reasoning,
		Input:         input,
		ContextWindow: model.Limit.Context,
		MaxTokens:     model.Limit.Output,
		Cost: ompModelCost{
			Input:      model.Cost.Input,
			Output:     model.Cost.Output,
			CacheRead:  model.Cost.CacheRead,
			CacheWrite: model.Cost.CacheWrite,
		},
	}
}

func withOMPSysVariants(models map[string]ompModelConfig) map[string]ompModelConfig {
	expanded := make(map[string]ompModelConfig, len(models)*2)
	for id, model := range models {
		expanded[id] = cloneOMPModelConfig(model)
		sys := cloneOMPModelConfig(model)
		sys.ID = model.ID + "-Sys"
		sys.Name = model.Name + " (Sys)"
		expanded[id+"-Sys"] = sys
	}
	return expanded
}

func cloneOMPModelConfig(model ompModelConfig) ompModelConfig {
	copy := model
	copy.Input = append([]string(nil), model.Input...)
	return copy
}

func renderOMPModelYAML(model ompModelConfig, indent string, extraLines []string) string {
	lines := []string{
		fmt.Sprintf("%s- id: %s", indent, model.ID),
		fmt.Sprintf("%s  name: %s", indent, model.Name),
		fmt.Sprintf("%s  api: %s", indent, model.API),
		fmt.Sprintf("%s  reasoning: %t", indent, model.Reasoning),
		fmt.Sprintf("%s  input:", indent),
	}
	for _, item := range model.Input {
		lines = append(lines, fmt.Sprintf("%s    - %s", indent, item))
	}
	if model.ContextWindow != 0 {
		lines = append(lines, fmt.Sprintf("%s  contextWindow: %d", indent, model.ContextWindow))
	}
	if model.MaxTokens != 0 {
		lines = append(lines, fmt.Sprintf("%s  maxTokens: %d", indent, model.MaxTokens))
	}
	costLines := renderOMPCostYAML(model.Cost, indent+"    ")
	if len(costLines) > 0 {
		lines = append(lines, fmt.Sprintf("%s  cost:", indent))
		lines = append(lines, costLines...)
	}
	lines = append(lines, extraLines...)
	return strings.Join(lines, "\n")
}

func renderOMPCostYAML(cost ompModelCost, indent string) []string {
	var lines []string
	if cost.Input != 0 {
		lines = append(lines, fmt.Sprintf("%sinput: %s", indent, formatFloat(cost.Input)))
	}
	if cost.Output != 0 {
		lines = append(lines, fmt.Sprintf("%soutput: %s", indent, formatFloat(cost.Output)))
	}
	if cost.CacheRead != 0 {
		lines = append(lines, fmt.Sprintf("%scacheRead: %s", indent, formatFloat(cost.CacheRead)))
	}
	if cost.CacheWrite != 0 {
		lines = append(lines, fmt.Sprintf("%scacheWrite: %s", indent, formatFloat(cost.CacheWrite)))
	}
	return lines
}

func renderOMPSettingsYAML() string {
	return `defaultThinkingLevel: xhigh
serviceTier: priority

modelRoles:
  default: sub2api-openai/gpt-5.5-Sys
  slow: sub2api-openai/gpt-5.5-Sys
  smol: sub2api-openai/gpt-5.4-mini-Sys
  plan: sub2api-openai/gpt-5.5-Sys
  task: sub2api-openai/gpt-5.5-Sys:xhigh
  vision: sub2api-openai/gpt-5.5-Sys

task:
  agentModelOverrides:
    explore: sub2api-openai/gpt-5.4-mini-Sys:xhigh
    librarian: sub2api-openai/gpt-5.4-mini-Sys:xhigh
    reviewer: sub2api-openai/gpt-5.5-Sys:xhigh
    plan: sub2api-openai/gpt-5.5-Sys:xhigh`
}

func renderOMPImageGeneratorMarkdown() string {
	return `---
name: image_generator
description: Generate or iterate images only; do not handle ordinary code modification tasks.
model: sub2api-openai-image/gpt-5.5-Sys:xhigh
---

You are a specialized image generation subagent.

Use the provider-native image generation capability to create or refine images when the user explicitly asks for visual output. Do not take over normal coding, refactoring, debugging, or documentation tasks. Return concise status and generated image references to the caller.`
}

func renderOpenCodeOpenAIConfig(baseURL, apiKey string, models map[string]service.OpenCodeOpenAIModel) ([]byte, error) {
	if _, ok := models["gpt-5.5"]; !ok {
		return nil, fmt.Errorf("gpt-5.5 missing")
	}
	if _, ok := models["gpt-5.5-fast"]; !ok {
		return nil, fmt.Errorf("gpt-5.5-fast missing")
	}

	baseModels := buildOpenCodeOpenAIBaseModels(models)
	openAIModels := withOpenCodeSysVariants(baseModels)
	for _, id := range []string{"gpt-5.5", "gpt-5.5-fast", "gpt-5.5-Sys", "gpt-5.5-fast-Sys"} {
		model, ok := openAIModels[id]
		if !ok {
			return nil, fmt.Errorf("%s missing", id)
		}
		variants := ensureStringAnyMap(model["variants"])
		variants["image"] = openCodeImageVariant()
		model["variants"] = variants
		openAIModels[id] = model
	}

	cfg := map[string]any{
		"provider": map[string]any{
			"sub2api-openai": map[string]any{
				"npm":  "@ai-sdk/openai",
				"name": "sub2api OpenAI",
				"options": map[string]any{
					"baseURL": baseURL,
					"apiKey":  apiKey,
				},
				"models": openAIModels,
			},
		},
		"agent": map[string]any{
			"build": map[string]any{
				"options": map[string]any{"store": false},
			},
			"plan": map[string]any{
				"options": map[string]any{"store": false},
			},
			"image": map[string]any{
				"mode":        "subagent",
				"description": "Generate images with GPT-5.5 Image Fast (Sys)",
				"model":       "sub2api-openai/gpt-5.5-fast-Sys",
				"variant":     "image",
				"options":     map[string]any{"store": false},
			},
		},
		"$schema": "https://opencode.ai/config.json",
	}

	return json.MarshalIndent(cfg, "", "  ")
}

func buildOpenCodeOpenAIBaseModels(source map[string]service.OpenCodeOpenAIModel) map[string]any {
	ids := make([]string, 0, len(source))
	for id := range source {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make(map[string]any, len(source))
	for _, id := range ids {
		model := source[id]
		config := normalizeOpenCodeModelConfig(id, model)
		out[id] = config
	}
	return out
}

func normalizeOpenCodeModelConfig(id string, model service.OpenCodeOpenAIModel) map[string]any {
	config := map[string]any{
		"id":                strings.TrimSuffix(id, "-fast"),
		"name":              model.Name,
		"attachment":        model.Attachment,
		"reasoning":         model.Reasoning,
		"tool_call":         model.ToolCall,
		"structured_output": model.StructuredOutput,
		"temperature":       model.Temperature,
		"options": mergeOpenCodeModelOptions(model.Options, map[string]any{
			"builtin_tools": map[string]any{"web_search": true},
		}, false),
		"variants": buildOpenCodeReasoningVariants(reasoningLevels(id, model)),
	}
	if model.Family != "" {
		config["family"] = model.Family
	}
	if model.Knowledge != "" {
		config["knowledge"] = model.Knowledge
	}
	if model.Interleaved != nil {
		config["interleaved"] = model.Interleaved
	}
	if len(model.Modalities.Input) > 0 || len(model.Modalities.Output) > 0 {
		modalities := map[string]any{}
		if len(model.Modalities.Input) > 0 {
			modalities["input"] = append([]string(nil), model.Modalities.Input...)
		}
		if len(model.Modalities.Output) > 0 {
			modalities["output"] = append([]string(nil), model.Modalities.Output...)
		}
		config["modalities"] = modalities
	}
	if cost := openCodeCostMap(model.Cost); len(cost) > 0 {
		config["cost"] = cost
	}
	if limit := openCodeLimitMap(model.Limit); len(limit) > 0 {
		config["limit"] = limit
	}
	if model.ReleaseDate != "" {
		config["release_date"] = model.ReleaseDate
	}
	if len(model.Headers) > 0 {
		config["headers"] = cloneStringAnyMapFromString(model.Headers)
	}
	return config
}

func withOpenCodeSysVariants(models map[string]any) map[string]map[string]any {
	expanded := make(map[string]map[string]any, len(models)*2)
	ids := make([]string, 0, len(models))
	for id := range models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		config := deepCloneStringAnyMap(models[id].(map[string]any))
		expanded[id] = config
		sys := deepCloneStringAnyMap(config)
		if modelID, ok := sys["id"].(string); ok {
			sys["id"] = modelID + "-Sys"
		}
		if name, ok := sys["name"].(string); ok {
			sys["name"] = name + " (Sys)"
		}
		expanded[id+"-Sys"] = sys
	}
	return expanded
}

func buildOpenCodeReasoningVariants(levels []string) map[string]any {
	variants := make(map[string]any, len(levels))
	for _, level := range levels {
		variants[level] = map[string]any{
			"reasoningEffort":  level,
			"reasoningSummary": "auto",
			"include":          []string{"reasoning.encrypted_content"},
		}
	}
	return variants
}

func reasoningLevels(id string, model service.OpenCodeOpenAIModel) []string {
	if !model.Reasoning {
		return nil
	}
	lower := strings.ToLower(id)
	if lower == "gpt-5-pro" {
		return nil
	}
	if lower == "gpt-5-codex" || lower == "gpt-5.1-codex" || lower == "gpt-5.1-codex-max" || lower == "gpt-5.1-codex-mini" || lower == "codex-mini-latest" {
		return []string{"low", "medium", "high"}
	}
	if lower == "gpt-5.3-codex-spark" || lower == "gpt-5.3-codex" || lower == "gpt-5.2-codex" {
		return []string{"low", "medium", "high", "xhigh"}
	}
	levels := []string{"low", "medium", "high"}
	if strings.Contains(lower, "gpt-5-") || lower == "gpt-5" {
		levels = append([]string{"minimal"}, levels...)
	}
	if model.ReleaseDate >= "2025-11-13" {
		levels = append([]string{"none"}, levels...)
	}
	if model.ReleaseDate >= "2025-12-04" {
		levels = append(levels, "xhigh")
	}
	return levels
}

func openCodeImageVariant() map[string]any {
	return map[string]any{
		"metadata": map[string]any{
			"builtin_tools": map[string]any{
				"web_search": true,
				"image_generation": map[string]any{
					"enabled":       true,
					"model":         "gpt-image-2",
					"output_format": "png",
				},
			},
		},
	}
}

func mergeOpenCodeModelOptions(options map[string]any, metadata map[string]any, image bool) map[string]any {
	out := deepCloneStringAnyMap(options)
	existingMetadata := ensureStringAnyMap(out["metadata"])
	existingBuiltinTools := ensureStringAnyMap(existingMetadata["builtin_tools"])
	if image {
		existingBuiltinTools["image_generation"] = map[string]any{"enabled": true, "model": "gpt-image-2", "output_format": "png"}
	}
	for key, value := range metadata["builtin_tools"].(map[string]any) {
		existingBuiltinTools[key] = value
	}
	existingMetadata["builtin_tools"] = existingBuiltinTools
	out["metadata"] = existingMetadata
	out["store"] = false
	return out
}

func openCodeCostMap(cost service.OpenCodeOpenAIModelCost) map[string]any {
	out := map[string]any{}
	if cost.Input != 0 {
		out["input"] = cost.Input
	}
	if cost.Output != 0 {
		out["output"] = cost.Output
	}
	if cost.CacheRead != 0 {
		out["cache_read"] = cost.CacheRead
	}
	if cost.CacheWrite != 0 {
		out["cache_write"] = cost.CacheWrite
	}
	return out
}

func openCodeLimitMap(limit service.OpenCodeOpenAIModelLimit) map[string]any {
	out := map[string]any{}
	if limit.Context != 0 {
		out["context"] = limit.Context
	}
	if limit.Input != 0 {
		out["input"] = limit.Input
	}
	if limit.Output != 0 {
		out["output"] = limit.Output
	}
	return out
}

func cloneStringAnyMapFromString(src map[string]string) map[string]any {
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func ensureStringAnyMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if mapped, ok := value.(map[string]any); ok {
		return deepCloneStringAnyMap(mapped)
	}
	return map[string]any{}
}

func deepCloneStringAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		switch typed := value.(type) {
		case map[string]any:
			out[key] = deepCloneStringAnyMap(typed)
		case map[string]string:
			out[key] = cloneStringAnyMapFromString(typed)
		case []string:
			out[key] = append([]string(nil), typed...)
		case []any:
			out[key] = append([]any(nil), typed...)
		default:
			out[key] = typed
		}
	}
	return out
}

func containsControlCharacter(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func strPtr(v string) *string {
	return &v
}

func formatFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.10f", value), "0"), ".")
}
