package service

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	opencodeCodexHeaderURL = "https://raw.githubusercontent.com/anomalyco/opencode/dev/packages/opencode/src/session/prompt/codex_header.txt"
	codexCacheTTL          = 15 * time.Minute
)

//go:embed prompts/codex_cli_instructions.md
var codexCLIInstructions string

var codexModelMap = map[string]string{
	"gpt-5.1-codex":             "gpt-5.1-codex",
	"gpt-5.1-codex-low":         "gpt-5.1-codex",
	"gpt-5.1-codex-medium":      "gpt-5.1-codex",
	"gpt-5.1-codex-high":        "gpt-5.1-codex",
	"gpt-5.1-codex-max":         "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-low":     "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-medium":  "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-high":    "gpt-5.1-codex-max",
	"gpt-5.1-codex-max-xhigh":   "gpt-5.1-codex-max",
	"gpt-5.2":                   "gpt-5.2",
	"gpt-5.2-none":              "gpt-5.2",
	"gpt-5.2-low":               "gpt-5.2",
	"gpt-5.2-medium":            "gpt-5.2",
	"gpt-5.2-high":              "gpt-5.2",
	"gpt-5.2-xhigh":             "gpt-5.2",
	"gpt-5.2-codex":             "gpt-5.2-codex",
	"gpt-5.2-codex-low":         "gpt-5.2-codex",
	"gpt-5.2-codex-medium":      "gpt-5.2-codex",
	"gpt-5.2-codex-high":        "gpt-5.2-codex",
	"gpt-5.2-codex-xhigh":       "gpt-5.2-codex",
	"gpt-5.1-codex-mini":        "gpt-5.1-codex-mini",
	"gpt-5.1-codex-mini-medium": "gpt-5.1-codex-mini",
	"gpt-5.1-codex-mini-high":   "gpt-5.1-codex-mini",
	"gpt-5.1":                   "gpt-5.1",
	"gpt-5.1-none":              "gpt-5.1",
	"gpt-5.1-low":               "gpt-5.1",
	"gpt-5.1-medium":            "gpt-5.1",
	"gpt-5.1-high":              "gpt-5.1",
	"gpt-5.1-chat-latest":       "gpt-5.1",
	"gpt-5-codex":               "gpt-5.1-codex",
	"codex-mini-latest":         "gpt-5.1-codex-mini",
	"gpt-5-codex-mini":          "gpt-5.1-codex-mini",
	"gpt-5-codex-mini-medium":   "gpt-5.1-codex-mini",
	"gpt-5-codex-mini-high":     "gpt-5.1-codex-mini",
	"gpt-5":                     "gpt-5.1",
	"gpt-5-mini":                "gpt-5.1",
	"gpt-5-nano":                "gpt-5.1",
REDACTED

type codexTransformResult struct {
	Modified        bool
	NormalizedModel string
	PromptCacheKey  string
REDACTED

type opencodeCacheMetadata struct {
	ETag        string `json:"etag"`
	LastFetch   string `json:"lastFetch,omitempty"`
	LastChecked int64  `json:"lastChecked"`
REDACTED

func applyCodexOAuthTransform(reqBody map[string]any) codexTransformResult {
	result := codexTransformResult{REDACTED

	model := ""
	if v, ok := reqBody["model"].(string); ok {
		model = v
REDACTED
	normalizedModel := normalizeCodexModel(model)
	if normalizedModel != "" {
		if model != normalizedModel {
			reqBody["model"] = normalizedModel
			result.Modified = true
	REDACTED
		result.NormalizedModel = normalizedModel
REDACTED

	if v, ok := reqBody["store"].(bool); !ok || v {
		reqBody["store"] = false
		result.Modified = true
REDACTED
	if v, ok := reqBody["stream"].(bool); !ok || !v {
		reqBody["stream"] = true
		result.Modified = true
REDACTED

	if _, ok := reqBody["max_output_tokens"]; ok {
		delete(reqBody, "max_output_tokens")
		result.Modified = true
REDACTED
	if _, ok := reqBody["max_completion_tokens"]; ok {
		delete(reqBody, "max_completion_tokens")
		result.Modified = true
REDACTED

	if normalizeCodexTools(reqBody) {
		result.Modified = true
REDACTED

	if v, ok := reqBody["prompt_cache_key"].(string); ok {
		result.PromptCacheKey = strings.TrimSpace(v)
REDACTED

	instructions := strings.TrimSpace(getOpenCodeCodexHeader())
	existingInstructions, _ := reqBody["instructions"].(string)
	existingInstructions = strings.TrimSpace(existingInstructions)

	if instructions != "" {
		if existingInstructions != instructions {
			reqBody["instructions"] = instructions
			result.Modified = true
	REDACTED
REDACTED else if existingInstructions == "" {
		// If no opencode instructions available, try codex CLI instructions
		codexInstructions := strings.TrimSpace(getCodexCLIInstructions())
		if codexInstructions != "" {
			reqBody["instructions"] = codexInstructions
			result.Modified = true
	REDACTED
REDACTED

	if input, ok := reqBody["input"].([]any); ok {
		input = filterCodexInput(input)
		reqBody["input"] = input
		result.Modified = true
REDACTED

	return result
REDACTED

func normalizeCodexModel(model string) string {
	if model == "" {
		return "gpt-5.1"
REDACTED

	modelID := model
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
REDACTED

	if mapped := getNormalizedCodexModel(modelID); mapped != "" {
		return mapped
REDACTED

	normalized := strings.ToLower(modelID)

	if strings.Contains(normalized, "gpt-5.2-codex") || strings.Contains(normalized, "gpt 5.2 codex") {
		return "gpt-5.2-codex"
REDACTED
	if strings.Contains(normalized, "gpt-5.2") || strings.Contains(normalized, "gpt 5.2") {
		return "gpt-5.2"
REDACTED
	if strings.Contains(normalized, "gpt-5.1-codex-max") || strings.Contains(normalized, "gpt 5.1 codex max") {
		return "gpt-5.1-codex-max"
REDACTED
	if strings.Contains(normalized, "gpt-5.1-codex-mini") || strings.Contains(normalized, "gpt 5.1 codex mini") {
		return "gpt-5.1-codex-mini"
REDACTED
	if strings.Contains(normalized, "codex-mini-latest") ||
		strings.Contains(normalized, "gpt-5-codex-mini") ||
		strings.Contains(normalized, "gpt 5 codex mini") {
		return "codex-mini-latest"
REDACTED
	if strings.Contains(normalized, "gpt-5.1-codex") || strings.Contains(normalized, "gpt 5.1 codex") {
		return "gpt-5.1-codex"
REDACTED
	if strings.Contains(normalized, "gpt-5.1") || strings.Contains(normalized, "gpt 5.1") {
		return "gpt-5.1"
REDACTED
	if strings.Contains(normalized, "codex") {
		return "gpt-5.1-codex"
REDACTED
	if strings.Contains(normalized, "gpt-5") || strings.Contains(normalized, "gpt 5") {
		return "gpt-5.1"
REDACTED

	return "gpt-5.1"
REDACTED

func getNormalizedCodexModel(modelID string) string {
	if modelID == "" {
		return ""
REDACTED
	if mapped, ok := codexModelMap[modelID]; ok {
		return mapped
REDACTED
	lower := strings.ToLower(modelID)
	for key, value := range codexModelMap {
		if strings.ToLower(key) == lower {
			return value
	REDACTED
REDACTED
	return ""
REDACTED

func getOpenCodeCachedPrompt(url, cacheFileName, metaFileName string) string {
	cacheDir := codexCachePath("")
	if cacheDir == "" {
		return ""
REDACTED
	cacheFile := filepath.Join(cacheDir, cacheFileName)
	metaFile := filepath.Join(cacheDir, metaFileName)

	var cachedContent string
	if content, ok := readFile(cacheFile); ok {
		cachedContent = content
REDACTED

	var meta opencodeCacheMetadata
	if loadJSON(metaFile, &meta) && meta.LastChecked > 0 && cachedContent != "" {
		if time.Since(time.UnixMilli(meta.LastChecked)) < codexCacheTTL {
			return cachedContent
	REDACTED
REDACTED

	content, etag, status, err := fetchWithETag(url, meta.ETag)
	if err == nil && status == http.StatusNotModified && cachedContent != "" {
		return cachedContent
REDACTED
	if err == nil && status >= 200 && status < 300 && content != "" {
		_ = writeFile(cacheFile, content)
		meta = opencodeCacheMetadata{
			ETag:        etag,
			LastFetch:   time.Now().UTC().Format(time.RFC3339),
			LastChecked: time.Now().UnixMilli(),
	REDACTED
		_ = writeJSON(metaFile, meta)
		return content
REDACTED

	return cachedContent
REDACTED

func getOpenCodeCodexHeader() string {
	// Try to get from opencode repository first
	opencodeInstructions := getOpenCodeCachedPrompt(opencodeCodexHeaderURL, "opencode-codex-header.txt", "opencode-codex-header-meta.json")

	// If opencode instructions are available, return them
	if opencodeInstructions != "" {
		return opencodeInstructions
REDACTED

	// Fallback to local codex CLI instructions
	return getCodexCLIInstructions()
REDACTED

func getCodexCLIInstructions() string {
	return codexCLIInstructions
REDACTED

func GetOpenCodeInstructions() string {
	return getOpenCodeCodexHeader()
REDACTED

func GetCodexCLIInstructions() string {
	return getCodexCLIInstructions()
REDACTED

func ReplaceWithCodexInstructions(reqBody map[string]any) bool {
	codexInstructions := strings.TrimSpace(getCodexCLIInstructions())
	if codexInstructions == "" {
		return false
REDACTED

	existingInstructions, _ := reqBody["instructions"].(string)
	if strings.TrimSpace(existingInstructions) != codexInstructions {
		reqBody["instructions"] = codexInstructions
		return true
REDACTED

	return false
REDACTED

func IsInstructionError(errorMessage string) bool {
	if errorMessage == "" {
		return false
REDACTED

	lowerMsg := strings.ToLower(errorMessage)
	instructionKeywords := []string{
		"instruction",
		"instructions",
		"system prompt",
		"system message",
		"invalid prompt",
		"prompt format",
REDACTED

	for _, keyword := range instructionKeywords {
		if strings.Contains(lowerMsg, keyword) {
			return true
	REDACTED
REDACTED

	return false
REDACTED

func filterCodexInput(input []any) []any {
	filtered := make([]any, 0, len(input))
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
	REDACTED
		typ, _ := m["type"].(string)
		if typ == "item_reference" {
			filtered = append(filtered, m)
			continue
	REDACTED
		// Strip per-item ids; keep call_id only for tool call items so outputs can match.
		if isCodexToolCallItemType(typ) {
			callID, _ := m["call_id"].(string)
			if strings.TrimSpace(callID) == "" {
				if id, ok := m["id"].(string); ok && strings.TrimSpace(id) != "" {
					m["call_id"] = id
			REDACTED
		REDACTED
	REDACTED
		delete(m, "id")
		if !isCodexToolCallItemType(typ) {
			delete(m, "call_id")
	REDACTED
		filtered = append(filtered, m)
REDACTED
	return filtered
REDACTED

func isCodexToolCallItemType(typ string) bool {
	if typ == "" {
		return false
REDACTED
	return strings.HasSuffix(typ, "_call") || strings.HasSuffix(typ, "_call_output")
REDACTED

func normalizeCodexTools(reqBody map[string]any) bool {
	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		return false
REDACTED
	tools, ok := rawTools.([]any)
	if !ok {
		return false
REDACTED

	modified := false
	for idx, tool := range tools {
		toolMap, ok := tool.(map[string]any)
		if !ok {
			continue
	REDACTED

		toolType, _ := toolMap["type"].(string)
		if strings.TrimSpace(toolType) != "function" {
			continue
	REDACTED

		function, ok := toolMap["function"].(map[string]any)
		if !ok {
			continue
	REDACTED

		if _, ok := toolMap["name"]; !ok {
			if name, ok := function["name"].(string); ok && strings.TrimSpace(name) != "" {
				toolMap["name"] = name
				modified = true
		REDACTED
	REDACTED
		if _, ok := toolMap["description"]; !ok {
			if desc, ok := function["description"].(string); ok && strings.TrimSpace(desc) != "" {
				toolMap["description"] = desc
				modified = true
		REDACTED
	REDACTED
		if _, ok := toolMap["parameters"]; !ok {
			if params, ok := function["parameters"]; ok {
				toolMap["parameters"] = params
				modified = true
		REDACTED
	REDACTED
		if _, ok := toolMap["strict"]; !ok {
			if strict, ok := function["strict"]; ok {
				toolMap["strict"] = strict
				modified = true
		REDACTED
	REDACTED

		tools[idx] = toolMap
REDACTED

	if modified {
		reqBody["tools"] = tools
REDACTED

	return modified
REDACTED

func codexCachePath(filename string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
REDACTED
	cacheDir := filepath.Join(home, ".opencode", "cache")
	if filename == "" {
		return cacheDir
REDACTED
	return filepath.Join(cacheDir, filename)
REDACTED

func readFile(path string) (string, bool) {
	if path == "" {
		return "", false
REDACTED
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
REDACTED
	return string(data), true
REDACTED

func writeFile(path, content string) error {
	if path == "" {
		return fmt.Errorf("empty cache path")
REDACTED
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
REDACTED
	return os.WriteFile(path, []byte(content), 0o644)
REDACTED

func loadJSON(path string, target any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
REDACTED
	if err := json.Unmarshal(data, target); err != nil {
		return false
REDACTED
	return true
REDACTED

func writeJSON(path string, value any) error {
	if path == "" {
		return fmt.Errorf("empty json path")
REDACTED
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
REDACTED
	data, err := json.Marshal(value)
	if err != nil {
		return err
REDACTED
	return os.WriteFile(path, data, 0o644)
REDACTED

func fetchWithETag(url, etag string) (string, string, int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", "", 0, err
REDACTED
	req.Header.Set("User-Agent", "sub2api-codex")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
REDACTED
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", 0, err
REDACTED
	defer func() {
		_ = resp.Body.Close()
REDACTED()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", resp.StatusCode, err
REDACTED
	return string(body), resp.Header.Get("etag"), resp.StatusCode, nil
REDACTED
