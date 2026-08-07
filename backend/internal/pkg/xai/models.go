package xai

import (
	"strings"
	"sync/atomic"
)

// runtimeMappingOpts holds operator-configured defaults applied when Grok
// accounts leave credentials.model_mapping empty. Updated from settings.
var runtimeMappingOpts atomic.Value // ModelMappingOptions

func init() {
	runtimeMappingOpts.Store(ModelMappingOptions{REDACTED)
REDACTED

// SetRuntimeModelMappingOptions updates process-wide defaults used by
// DefaultModelMapping (e.g. after settings load). Safe for concurrent use.
func SetRuntimeModelMappingOptions(opts ModelMappingOptions) {
	runtimeMappingOpts.Store(opts)
REDACTED

// RuntimeModelMappingOptions returns the last options set via SetRuntimeModelMappingOptions.
func RuntimeModelMappingOptions() ModelMappingOptions {
	if v := runtimeMappingOpts.Load(); v != nil {
		if opts, ok := v.(ModelMappingOptions); ok {
			return opts
	REDACTED
REDACTED
	return ModelMappingOptions{REDACTED
REDACTED

// Model describes an xAI model in OpenAI-compatible /models shape.
type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Type        string `json:"type,omitempty"`
	Created     int64  `json:"created,omitempty"`
	OwnedBy     string `json:"owned_by"`
	DisplayName string `json:"display_name,omitempty"`
REDACTED

// DefaultTextModel is the built-in fallback for empty model fields and Grok
// text aliases (e.g. "grok", "grok-latest"). Operators may override the runtime
// default via settings key grok_default_text_model.
const DefaultTextModel = "grok-4.5"

// Official Imagine model IDs (https://docs.x.ai/docs/models).
const (
	DefaultImagineImageQualityModel  = "grok-imagine-image-quality"
	DefaultImagineImageFastModel     = "grok-imagine-image"
	DefaultImagineVideoModel         = "grok-imagine-video"
	DefaultImagineVideo15LegacyModel = "grok-imagine-video-1.5"
	DefaultImagineVideo15Model       = "grok-imagine-video-1.5-preview"
)

// ModelMappingOptions controls optional expansions of the default mapping.
// Cross-client wildcards (gpt-*/claude-*) are OFF unless explicitly enabled —
// silent rewrite of foreign model names is opt-in for operators who want
// Codex/Claude clients to talk to Grok groups without renaming models.
type ModelMappingOptions struct {
	// DefaultText is the target for empty models and optional cross-client maps.
	// Empty → DefaultTextModel.
	DefaultText string
	// EnableCrossClientMap merges gpt-*/codex-*/o*/claude-* → DefaultText.
	EnableCrossClientMap bool
REDACTED

func (o ModelMappingOptions) defaultText() string {
	if t := strings.TrimSpace(o.DefaultText); t != "" {
		return t
REDACTED
	return DefaultTextModel
REDACTED

var defaultModels = []Model{
	// Text
	{ID: "grok-4.5", Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok 4.5"REDACTED,
	{ID: "grok-4.3", Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok 4.3"REDACTED,
	{ID: "grok-3-mini", Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok 3 Mini"REDACTED,
	{ID: "grok-3-mini-fast", Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok 3 Mini Fast"REDACTED,
	{ID: "grok-build-0.1", Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok Build 0.1"REDACTED,
	{ID: "grok-code-fast-1-0825", Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok Code Fast"REDACTED,
	{ID: "grok-composer-2.5-fast", Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok Composer 2.5 Fast"REDACTED,
	{ID: "grok-4.20-0309-reasoning", Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Reasoning"REDACTED,
	{ID: "grok-4.20-0309-non-reasoning", Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Non Reasoning"REDACTED,
	{ID: "grok-4.20-multi-agent-0309", Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Multi Agent"REDACTED,
	// Imagine
	{ID: DefaultImagineImageQualityModel, Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Image Quality"REDACTED,
	{ID: DefaultImagineImageFastModel, Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Image"REDACTED,
	{ID: "grok-imagine-edit", Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Edit"REDACTED,
	{ID: DefaultImagineVideoModel, Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Video"REDACTED,
	{ID: DefaultImagineVideo15Model, Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Video 1.5 Preview"REDACTED,
	{ID: DefaultImagineVideo15LegacyModel, Object: "model", Type: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Video 1.5 Legacy"REDACTED,
REDACTED

// grokTextResponsesModelAliases is the source of truth for Grok text models
// accepted by the Responses path: client-facing / undated aliases → canonical
// upstream ID. Used by DefaultModelMapping and IsGrokTextResponsesModelID.
var grokTextResponsesModelAliases = map[string]string{
	"grok":                         DefaultTextModel,
	"grok-latest":                  DefaultTextModel,
	"grok-4.5":                     DefaultTextModel,
	"grok-4.5-latest":              DefaultTextModel,
	"grok-4.3":                     "grok-4.3",
	"grok-4.3-latest":              "grok-4.3",
	"grok-3-mini":                  "grok-3-mini",
	"grok-3-mini-fast":             "grok-3-mini-fast",
	"grok-build":                   "grok-build-0.1",
	"grok-build-latest":            "grok-build-0.1",
	"grok-build-0.1":               "grok-build-0.1",
	"grok-composer-2.5-fast":       "grok-composer-2.5-fast",
	"grok-composer":                "grok-composer-2.5-fast",
	"composer-2.5":                 "grok-composer-2.5-fast",
	"grok-code-fast":               "grok-code-fast-1-0825",
	"grok-code-fast-1":             "grok-code-fast-1-0825",
	"grok-code-fast-1-0825":        "grok-code-fast-1-0825",
	"grok-4.20-reasoning":          "grok-4.20-0309-reasoning",
	"grok-4.20-0309-reasoning":     "grok-4.20-0309-reasoning",
	"grok-4.20-non-reasoning":      "grok-4.20-0309-non-reasoning",
	"grok-4.20-0309-non-reasoning": "grok-4.20-0309-non-reasoning",
	"grok-4.20-multi-agent":        "grok-4.20-multi-agent-0309",
	"grok-4.20-multi-agent-latest": "grok-4.20-multi-agent-0309",
	"grok-4.20-multi-agent-0309":   "grok-4.20-multi-agent-0309",
REDACTED

func DefaultModels() []Model {
	out := make([]Model, len(defaultModels))
	copy(out, defaultModels)
	return out
REDACTED

func DefaultModelIDs() []string {
	models := DefaultModels()
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
REDACTED
	return ids
REDACTED

// DefaultModelMapping returns native Grok/Imagine identity + aliases, using
// runtime options (default text model / optional cross-client wildcards).
// Does NOT enable gpt-*/claude-* unless SetRuntimeModelMappingOptions enables them.
func DefaultModelMapping() map[string]string {
	return ModelMappingWithOptions(RuntimeModelMappingOptions())
REDACTED

// ModelMappingWithOptions builds the default Grok mapping with optional
// cross-client wildcards and a configurable default text model.
func ModelMappingWithOptions(opts ModelMappingOptions) map[string]string {
	defaultText := opts.defaultText()
	mapping := make(map[string]string, len(defaultModels)+len(grokTextResponsesModelAliases)+48)
	for _, model := range defaultModels {
		mapping[model.ID] = model.ID
REDACTED
	for alias, canonical := range grokTextResponsesModelAliases {
		// Remap aliases that pointed at DefaultTextModel constant to runtime default.
		if canonical == DefaultTextModel {
			mapping[alias] = defaultText
	REDACTED else {
			mapping[alias] = canonical
	REDACTED
REDACTED
	// Imagine aliases / legacy IDs → official catalog.
	mapping["grok-imagine"] = DefaultImagineImageQualityModel
	mapping["grok-imagine-1"] = DefaultImagineImageQualityModel
	// edit keeps its own id when listed; alias bare names to quality for clients
	// that only send grok-imagine-edit without catalog awareness.
	mapping["grok-imagine-edit"] = "grok-imagine-edit"
	mapping["grok-imagine-image"] = DefaultImagineImageFastModel
	mapping["grok-imagine-image-quality"] = DefaultImagineImageQualityModel
	mapping["grok-imagine-video"] = DefaultImagineVideoModel
	mapping["grok-imagine-video-1.5"] = DefaultImagineVideo15Model
	mapping["grok-imagine-video-1.5-preview"] = DefaultImagineVideo15Model
	mapping["grok-video-1.5"] = DefaultImagineVideo15Model

	if opts.EnableCrossClientMap {
		// Codex / OpenAI Responses client defaults (wildcard patterns).
		mapping["gpt-*"] = defaultText
		mapping["codex-*"] = defaultText
		mapping["o1*"] = defaultText
		mapping["o3*"] = defaultText
		mapping["o4*"] = defaultText
		// Claude Code defaults when operators intentionally enable bridging.
		mapping["claude-*"] = defaultText
REDACTED
	addGrokProviderPrefixedMappings(mapping)
	return mapping
REDACTED

func addGrokProviderPrefixedMappings(mapping map[string]string) {
	snapshot := make(map[string]string, len(mapping))
	for key, value := range mapping {
		snapshot[key] = value
REDACTED
	for key, value := range snapshot {
		if !isGrokNativeOrAlias(key) {
			continue
	REDACTED
		for _, prefix := range []string{"xai/", "x-ai/", "grok/"REDACTED {
			mapping[prefix+key] = value
	REDACTED
REDACTED
REDACTED

func isGrokNativeOrAlias(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" || strings.Contains(model, "*") {
		return false
REDACTED
	return strings.HasPrefix(model, "grok") ||
		strings.HasPrefix(model, "imagine") ||
		strings.HasPrefix(model, "composer")
REDACTED

// StripGrokProviderPrefix removes common provider prefixes accepted for
// xAI/Grok models, returning the native model ID.
func StripGrokProviderPrefix(model string) string {
	trimmed := strings.TrimSpace(model)
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"xai/", "x-ai/", "grok/"REDACTED {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(trimmed[len(prefix):])
	REDACTED
REDACTED
	return trimmed
REDACTED

// IsGrokModelID reports whether model looks like a native Grok/xAI model id
// (including aliases). Claude/OpenAI model names return false.
func IsGrokModelID(model string) bool {
	normalized := strings.ToLower(StripGrokProviderPrefix(model))
	if normalized == "" {
		return false
REDACTED
	if strings.HasPrefix(normalized, "grok") {
		return true
REDACTED
	if strings.HasPrefix(normalized, "imagine") {
		return true
REDACTED
	return false
REDACTED

// IsGrokTextResponsesModelID reports whether model is a known Grok text model
// for the Responses API. Imagine image/video and unknown custom ids return false.
func IsGrokTextResponsesModelID(model string) bool {
	normalized := strings.ToLower(StripGrokProviderPrefix(model))
	_, ok := grokTextResponsesModelAliases[normalized]
	return ok
REDACTED

// ResolveGrokTextResponsesModelID canonicalizes a Grok text alias before upstream.
// empty or bare aliases that resolve via DefaultTextModel use defaultText when set.
func ResolveGrokTextResponsesModelID(model string, defaultText ...string) string {
	fallback := DefaultTextModel
	if len(defaultText) > 0 && strings.TrimSpace(defaultText[0]) != "" {
		fallback = strings.TrimSpace(defaultText[0])
REDACTED
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return fallback
REDACTED
	normalized := strings.ToLower(StripGrokProviderPrefix(trimmed))
	if canonical, ok := grokTextResponsesModelAliases[normalized]; ok {
		if canonical == DefaultTextModel {
			return fallback
	REDACTED
		return canonical
REDACTED
	return StripGrokProviderPrefix(trimmed)
REDACTED

// ResolveDefaultTextModel returns defaultText (or DefaultTextModel) when model is empty.
func ResolveDefaultTextModel(model string, defaultText ...string) string {
	if trimmed := strings.TrimSpace(model); trimmed != "" {
		return trimmed
REDACTED
	if len(defaultText) > 0 && strings.TrimSpace(defaultText[0]) != "" {
		return strings.TrimSpace(defaultText[0])
REDACTED
	return DefaultTextModel
REDACTED

// CanonicalImagineVideoModel normalizes video model ids for pricing tables.
// Legacy "grok-imagine-video-1.5" shares the 1.5 price family with preview.
func CanonicalImagineVideoModel(model string) string {
	m := strings.ToLower(StripGrokProviderPrefix(model))
	switch {
	case m == "" || m == DefaultImagineVideoModel:
		return DefaultImagineVideoModel
	case strings.HasPrefix(m, "grok-imagine-video-1.5") || m == "grok-video-1.5":
		return DefaultImagineVideo15Model
	case strings.HasPrefix(m, "grok-imagine-video"):
		return DefaultImagineVideoModel
	default:
		return m
REDACTED
REDACTED
