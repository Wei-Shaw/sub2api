package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// SoraModelConfig Sora 模型配置
type SoraModelConfig struct {
	Type        string
	Width       int
	Height      int
	Orientation string
	Frames      int
	Model       string
	Size        string
	RequirePro  bool
	// Prompt-enhance 专用参数
	ExpansionLevel string
	DurationS      int
REDACTED

var soraModelConfigs = map[string]SoraModelConfig{
	"gpt-image": {
		Type:   "image",
		Width:  360,
		Height: 360,
REDACTED,
	"gpt-image-landscape": {
		Type:   "image",
		Width:  540,
		Height: 360,
REDACTED,
	"gpt-image-portrait": {
		Type:   "image",
		Width:  360,
		Height: 540,
REDACTED,
	"sora2-landscape-10s": {
		Type:        "video",
		Orientation: "landscape",
		Frames:      300,
		Model:       "sy_8",
		Size:        "small",
REDACTED,
	"sora2-portrait-10s": {
		Type:        "video",
		Orientation: "portrait",
		Frames:      300,
		Model:       "sy_8",
		Size:        "small",
REDACTED,
	"sora2-landscape-15s": {
		Type:        "video",
		Orientation: "landscape",
		Frames:      450,
		Model:       "sy_8",
		Size:        "small",
REDACTED,
	"sora2-portrait-15s": {
		Type:        "video",
		Orientation: "portrait",
		Frames:      450,
		Model:       "sy_8",
		Size:        "small",
REDACTED,
	"sora2-landscape-25s": {
		Type:        "video",
		Orientation: "landscape",
		Frames:      750,
		Model:       "sy_8",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2-portrait-25s": {
		Type:        "video",
		Orientation: "portrait",
		Frames:      750,
		Model:       "sy_8",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-landscape-10s": {
		Type:        "video",
		Orientation: "landscape",
		Frames:      300,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-portrait-10s": {
		Type:        "video",
		Orientation: "portrait",
		Frames:      300,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-landscape-15s": {
		Type:        "video",
		Orientation: "landscape",
		Frames:      450,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-portrait-15s": {
		Type:        "video",
		Orientation: "portrait",
		Frames:      450,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-landscape-25s": {
		Type:        "video",
		Orientation: "landscape",
		Frames:      750,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-portrait-25s": {
		Type:        "video",
		Orientation: "portrait",
		Frames:      750,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-hd-landscape-10s": {
		Type:        "video",
		Orientation: "landscape",
		Frames:      300,
		Model:       "sy_ore",
		Size:        "large",
		RequirePro:  true,
REDACTED,
	"sora2pro-hd-portrait-10s": {
		Type:        "video",
		Orientation: "portrait",
		Frames:      300,
		Model:       "sy_ore",
		Size:        "large",
		RequirePro:  true,
REDACTED,
	"sora2pro-hd-landscape-15s": {
		Type:        "video",
		Orientation: "landscape",
		Frames:      450,
		Model:       "sy_ore",
		Size:        "large",
		RequirePro:  true,
REDACTED,
	"sora2pro-hd-portrait-15s": {
		Type:        "video",
		Orientation: "portrait",
		Frames:      450,
		Model:       "sy_ore",
		Size:        "large",
		RequirePro:  true,
REDACTED,
	"prompt-enhance-short-10s": {
		Type:           "prompt_enhance",
		ExpansionLevel: "short",
		DurationS:      10,
REDACTED,
	"prompt-enhance-short-15s": {
		Type:           "prompt_enhance",
		ExpansionLevel: "short",
		DurationS:      15,
REDACTED,
	"prompt-enhance-short-20s": {
		Type:           "prompt_enhance",
		ExpansionLevel: "short",
		DurationS:      20,
REDACTED,
	"prompt-enhance-medium-10s": {
		Type:           "prompt_enhance",
		ExpansionLevel: "medium",
		DurationS:      10,
REDACTED,
	"prompt-enhance-medium-15s": {
		Type:           "prompt_enhance",
		ExpansionLevel: "medium",
		DurationS:      15,
REDACTED,
	"prompt-enhance-medium-20s": {
		Type:           "prompt_enhance",
		ExpansionLevel: "medium",
		DurationS:      20,
REDACTED,
	"prompt-enhance-long-10s": {
		Type:           "prompt_enhance",
		ExpansionLevel: "long",
		DurationS:      10,
REDACTED,
	"prompt-enhance-long-15s": {
		Type:           "prompt_enhance",
		ExpansionLevel: "long",
		DurationS:      15,
REDACTED,
	"prompt-enhance-long-20s": {
		Type:           "prompt_enhance",
		ExpansionLevel: "long",
		DurationS:      20,
REDACTED,
REDACTED

var soraModelIDs = []string{
	"gpt-image",
	"gpt-image-landscape",
	"gpt-image-portrait",
	"sora2-landscape-10s",
	"sora2-portrait-10s",
	"sora2-landscape-15s",
	"sora2-portrait-15s",
	"sora2-landscape-25s",
	"sora2-portrait-25s",
	"sora2pro-landscape-10s",
	"sora2pro-portrait-10s",
	"sora2pro-landscape-15s",
	"sora2pro-portrait-15s",
	"sora2pro-landscape-25s",
	"sora2pro-portrait-25s",
	"sora2pro-hd-landscape-10s",
	"sora2pro-hd-portrait-10s",
	"sora2pro-hd-landscape-15s",
	"sora2pro-hd-portrait-15s",
	"prompt-enhance-short-10s",
	"prompt-enhance-short-15s",
	"prompt-enhance-short-20s",
	"prompt-enhance-medium-10s",
	"prompt-enhance-medium-15s",
	"prompt-enhance-medium-20s",
	"prompt-enhance-long-10s",
	"prompt-enhance-long-15s",
	"prompt-enhance-long-20s",
REDACTED

// GetSoraModelConfig 返回 Sora 模型配置
func GetSoraModelConfig(model string) (SoraModelConfig, bool) {
	key := strings.ToLower(strings.TrimSpace(model))
	cfg, ok := soraModelConfigs[key]
	return cfg, ok
REDACTED

// DefaultSoraModels returns the default Sora model list.
func DefaultSoraModels(cfg *config.Config) []openai.Model {
	models := make([]openai.Model, 0, len(soraModelIDs))
	for _, id := range soraModelIDs {
		models = append(models, openai.Model{
			ID:          id,
			Object:      "model",
			OwnedBy:     "openai",
			Type:        "model",
			DisplayName: id,
	REDACTED)
REDACTED
	if cfg != nil && cfg.Gateway.SoraModelFilters.HidePromptEnhance {
		filtered := models[:0]
		for _, model := range models {
			if strings.HasPrefix(strings.ToLower(model.ID), "prompt-enhance") {
				continue
		REDACTED
			filtered = append(filtered, model)
	REDACTED
		models = filtered
REDACTED
	return models
REDACTED
