package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

const (
	openAIResponsesEndpoint          = "/v1/responses"
	openAIResponsesCompactEndpoint   = "/v1/responses/compact"
	imageGenerationPermissionMessage = "Image generation is not enabled for this group"
)

// ImageGenerationPermissionMessage returns the stable end-user error text for disabled groups.
func ImageGenerationPermissionMessage() string {
	return imageGenerationPermissionMessage
REDACTED

// GroupAllowsImageGeneration preserves ungrouped-key behavior and enforces the flag when a group is present.
func GroupAllowsImageGeneration(group *Group) bool {
	return group == nil || group.AllowImageGeneration
REDACTED

// IsImageGenerationIntent classifies requests that can produce generated images.
func IsImageGenerationIntent(endpoint string, requestedModel string, body []byte) bool {
	if IsImageGenerationEndpoint(endpoint) {
		return true
REDACTED
	if isOpenAIImageGenerationModel(requestedModel) {
		return true
REDACTED
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
REDACTED
	if model := strings.TrimSpace(gjson.GetBytes(body, "model").String()); isOpenAIImageGenerationModel(model) {
		return true
REDACTED
	if openAIJSONToolsContainImageGeneration(gjson.GetBytes(body, "tools")) {
		return true
REDACTED
	if openAIJSONInputContainsImageGenTool(gjson.GetBytes(body, "input")) {
		return true
REDACTED
	return openAIJSONToolChoiceSelectsImageGeneration(gjson.GetBytes(body, "tool_choice"))
REDACTED

// IsImageGenerationIntentMap is the map-backed variant used after service-side request mutation.
func IsImageGenerationIntentMap(endpoint string, requestedModel string, reqBody map[string]any) bool {
	if IsImageGenerationEndpoint(endpoint) {
		return true
REDACTED
	if isOpenAIImageGenerationModel(requestedModel) {
		return true
REDACTED
	if reqBody == nil {
		return false
REDACTED
	if isOpenAIImageGenerationModel(firstNonEmptyString(reqBody["model"])) {
		return true
REDACTED
	if hasOpenAIImageGenerationTool(reqBody) {
		return true
REDACTED
	return openAIAnyToolChoiceSelectsImageGeneration(reqBody["tool_choice"])
REDACTED

// IsImageGenerationEndpoint identifies dedicated generated-image endpoints.
func IsImageGenerationEndpoint(endpoint string) bool {
	switch normalizeImageGenerationEndpoint(endpoint) {
	case "/v1/images/generations", "/v1/images/edits", "/images/generations", "/images/edits":
		return true
	default:
		return false
REDACTED
REDACTED

func normalizeImageGenerationEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(strings.ToLower(endpoint))
	if endpoint == "" {
		return ""
REDACTED
	endpoint = strings.TrimPrefix(endpoint, "https://api.openai.com")
	if idx := strings.IndexByte(endpoint, '?'); idx >= 0 {
		endpoint = endpoint[:idx]
REDACTED
	return strings.TrimRight(endpoint, "/")
REDACTED

func openAIJSONToolsContainImageGeneration(tools gjson.Result) bool {
	if !tools.IsArray() {
		return false
REDACTED
	found := false
	tools.ForEach(func(_, item gjson.Result) bool {
		if openAIJSONString(item.Get("type")) == "image_generation" {
			found = true
			return false
	REDACTED
		if isImageGenNamespaceTool(item) {
			found = true
			return false
	REDACTED
		return true
REDACTED)
	return found
REDACTED

// isImageGenNamespaceTool detects the Codex namespace-style image generation
// tool declaration: { "type": "namespace", "name": "image_gen", ... REDACTED.
// Codex /image uses this instead of the flat { "type": "image_generation" REDACTED.
func isImageGenNamespaceTool(tool gjson.Result) bool {
	return openAIJSONString(tool.Get("type")) == "namespace" &&
		openAIJSONString(tool.Get("name")) == "image_gen"
REDACTED

// openAIJSONInputContainsImageGenTool scans Responses input items for
// additional_tools entries that declare the image_gen namespace. This covers
// the "Responses Lite" format where tools are embedded inside input items
// rather than top-level tools.
func openAIJSONInputContainsImageGenTool(input gjson.Result) bool {
	if !input.IsArray() {
		return false
REDACTED
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if openAIJSONString(item.Get("type")) != "additional_tools" {
			return true
	REDACTED
		tools := item.Get("tools")
		if !tools.IsArray() {
			return true
	REDACTED
		tools.ForEach(func(_, tool gjson.Result) bool {
			if isImageGenNamespaceTool(tool) {
				found = true
				return false
		REDACTED
			return true
	REDACTED)
		return !found
REDACTED)
	return found
REDACTED

func openAIRequestBodyHasImageGenerationTool(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
REDACTED
	return openAIJSONToolsContainImageGeneration(gjson.GetBytes(body, "tools"))
REDACTED

func openAIRequestBodyImageGenerationToolNeedsNormalization(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
REDACTED
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
REDACTED
	needsNormalization := false
	tools.ForEach(func(_, item gjson.Result) bool {
		if openAIJSONString(item.Get("type")) != "image_generation" {
			return true
	REDACTED
		// 只有旧字段需要迁移时才进入 map 修改，纯计费读取保持 raw 路径。
		if item.Get("format").Exists() || item.Get("compression").Exists() {
			needsNormalization = true
			return false
	REDACTED
		return true
REDACTED)
	return needsNormalization
REDACTED

func openAIJSONToolChoiceSelectsImageGeneration(choice gjson.Result) bool {
	if !choice.Exists() {
		return false
REDACTED
	if choice.Type == gjson.String {
		return strings.TrimSpace(choice.String()) == "image_generation"
REDACTED
	if !choice.IsObject() {
		return false
REDACTED
	if strings.TrimSpace(choice.Get("type").String()) == "image_generation" {
		return true
REDACTED
	if strings.TrimSpace(choice.Get("tool.type").String()) == "image_generation" {
		return true
REDACTED
	if strings.TrimSpace(choice.Get("function.name").String()) == "image_generation" {
		return true
REDACTED
	return false
REDACTED

func openAIAnyToolChoiceSelectsImageGeneration(choice any) bool {
	switch v := choice.(type) {
	case string:
		return strings.TrimSpace(v) == "image_generation"
	case map[string]any:
		if strings.TrimSpace(firstNonEmptyString(v["type"])) == "image_generation" {
			return true
	REDACTED
		if tool, ok := v["tool"].(map[string]any); ok && strings.TrimSpace(firstNonEmptyString(tool["type"])) == "image_generation" {
			return true
	REDACTED
		if fn, ok := v["function"].(map[string]any); ok && strings.TrimSpace(firstNonEmptyString(fn["name"])) == "image_generation" {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func getAPIKeyFromContext(c interface{ Get(string) (any, bool) REDACTED) *APIKey {
	if c == nil {
		return nil
REDACTED
	v, exists := c.Get("api_key")
	if !exists {
		return nil
REDACTED
	apiKey, _ := v.(*APIKey)
	return apiKey
REDACTED

func apiKeyGroup(apiKey *APIKey) *Group {
	if apiKey == nil {
		return nil
REDACTED
	return apiKey.Group
REDACTED

type OpenAIResponsesImageBillingConfig struct {
	Model     string
	SizeTier  string
	InputSize string
REDACTED

func resolveOpenAIResponsesImageBillingConfigDetailed(reqBody map[string]any, fallbackModel string) (OpenAIResponsesImageBillingConfig, error) {
	imageModel := ""
	imageSize := ""
	hasImageTool := false
	if reqBody != nil {
		rawTools, _ := reqBody["tools"].([]any)
		for _, rawTool := range rawTools {
			toolMap, ok := rawTool.(map[string]any)
			if !ok || strings.TrimSpace(firstNonEmptyString(toolMap["type"])) != "image_generation" {
				continue
		REDACTED
			hasImageTool = true
			imageModel = strings.TrimSpace(firstNonEmptyString(toolMap["model"]))
			imageSize = strings.TrimSpace(firstNonEmptyString(toolMap["size"]))
			break
	REDACTED
		if imageSize == "" {
			imageSize = strings.TrimSpace(firstNonEmptyString(reqBody["size"]))
	REDACTED
REDACTED
	if imageModel == "" && reqBody != nil {
		bodyModel := strings.TrimSpace(firstNonEmptyString(reqBody["model"]))
		if isOpenAIImageBillingModelAlias(bodyModel) || !hasImageTool {
			imageModel = bodyModel
	REDACTED
REDACTED
	if imageModel == "" && hasImageTool {
		imageModel = "gpt-image-2"
REDACTED
	if imageModel == "" {
		imageModel = strings.TrimSpace(fallbackModel)
REDACTED
	sizeTier := normalizeOpenAIImageSizeTier(imageSize)
	return OpenAIResponsesImageBillingConfig{
		Model:     imageModel,
		SizeTier:  sizeTier,
		InputSize: imageSize,
REDACTED, nil
REDACTED

func resolveOpenAIResponsesImageBillingConfigFromBody(body []byte, fallbackModel string) (string, string, error) {
	cfg, err := resolveOpenAIResponsesImageBillingConfigDetailedFromBody(body, fallbackModel)
	if err != nil {
		return "", "", err
REDACTED
	return cfg.Model, cfg.SizeTier, nil
REDACTED

func resolveOpenAIResponsesImageBillingConfigDetailedFromBody(body []byte, fallbackModel string) (OpenAIResponsesImageBillingConfig, error) {
	imageModel := ""
	imageSize := ""
	hasImageTool := false
	if len(body) > 0 && gjson.ValidBytes(body) {
		tools := gjson.GetBytes(body, "tools")
		if tools.IsArray() {
			tools.ForEach(func(_, item gjson.Result) bool {
				if openAIJSONString(item.Get("type")) != "image_generation" {
					return true
			REDACTED
				hasImageTool = true
				imageModel = openAIJSONString(item.Get("model"))
				imageSize = openAIJSONString(item.Get("size"))
				return false
		REDACTED)
	REDACTED
		if imageSize == "" {
			imageSize = openAIJSONString(gjson.GetBytes(body, "size"))
	REDACTED
		if imageModel == "" {
			bodyModel := openAIJSONString(gjson.GetBytes(body, "model"))
			if isOpenAIImageBillingModelAlias(bodyModel) || !hasImageTool {
				imageModel = bodyModel
		REDACTED
	REDACTED
REDACTED
	if imageModel == "" && hasImageTool {
		imageModel = "gpt-image-2"
REDACTED
	if imageModel == "" {
		imageModel = strings.TrimSpace(fallbackModel)
REDACTED
	return OpenAIResponsesImageBillingConfig{
		Model:     imageModel,
		SizeTier:  normalizeOpenAIImageSizeTier(imageSize),
		InputSize: imageSize,
REDACTED, nil
REDACTED

func isOpenAIImageBillingModelAlias(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return false
REDACTED
	return isOpenAIImageGenerationModel(normalized) || strings.Contains(normalized, "image")
REDACTED

func openAIJSONString(value gjson.Result) string {
	if value.Type != gjson.String {
		return ""
REDACTED
	return strings.TrimSpace(value.String())
REDACTED
