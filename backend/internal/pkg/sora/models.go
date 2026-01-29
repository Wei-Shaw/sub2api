package sora

// ModelConfig 定义 Sora 模型配置。
type ModelConfig struct {
	Type           string
	Width          int
	Height         int
	Orientation    string
	NFrames        int
	Model          string
	Size           string
	RequirePro     bool
	ExpansionLevel string
	DurationS      int
REDACTED

// ModelConfigs 定义所有模型配置。
var ModelConfigs = map[string]ModelConfig{
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
		NFrames:     300,
REDACTED,
	"sora2-portrait-10s": {
		Type:        "video",
		Orientation: "portrait",
		NFrames:     300,
REDACTED,
	"sora2-landscape-15s": {
		Type:        "video",
		Orientation: "landscape",
		NFrames:     450,
REDACTED,
	"sora2-portrait-15s": {
		Type:        "video",
		Orientation: "portrait",
		NFrames:     450,
REDACTED,
	"sora2-landscape-25s": {
		Type:        "video",
		Orientation: "landscape",
		NFrames:     750,
		Model:       "sy_8",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2-portrait-25s": {
		Type:        "video",
		Orientation: "portrait",
		NFrames:     750,
		Model:       "sy_8",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-landscape-10s": {
		Type:        "video",
		Orientation: "landscape",
		NFrames:     300,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-portrait-10s": {
		Type:        "video",
		Orientation: "portrait",
		NFrames:     300,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-landscape-15s": {
		Type:        "video",
		Orientation: "landscape",
		NFrames:     450,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-portrait-15s": {
		Type:        "video",
		Orientation: "portrait",
		NFrames:     450,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-landscape-25s": {
		Type:        "video",
		Orientation: "landscape",
		NFrames:     750,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-portrait-25s": {
		Type:        "video",
		Orientation: "portrait",
		NFrames:     750,
		Model:       "sy_ore",
		Size:        "small",
		RequirePro:  true,
REDACTED,
	"sora2pro-hd-landscape-10s": {
		Type:        "video",
		Orientation: "landscape",
		NFrames:     300,
		Model:       "sy_ore",
		Size:        "large",
		RequirePro:  true,
REDACTED,
	"sora2pro-hd-portrait-10s": {
		Type:        "video",
		Orientation: "portrait",
		NFrames:     300,
		Model:       "sy_ore",
		Size:        "large",
		RequirePro:  true,
REDACTED,
	"sora2pro-hd-landscape-15s": {
		Type:        "video",
		Orientation: "landscape",
		NFrames:     450,
		Model:       "sy_ore",
		Size:        "large",
		RequirePro:  true,
REDACTED,
	"sora2pro-hd-portrait-15s": {
		Type:        "video",
		Orientation: "portrait",
		NFrames:     450,
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

// ModelListItem 返回模型列表条目。
type ModelListItem struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	OwnedBy     string `json:"owned_by"`
	Description string `json:"description"`
REDACTED

// ListModels 生成模型列表。
func ListModels() []ModelListItem {
	models := make([]ModelListItem, 0, len(ModelConfigs))
	for id, cfg := range ModelConfigs {
		description := ""
		switch cfg.Type {
		case "image":
			description = "Image generation"
			if cfg.Width > 0 && cfg.Height > 0 {
				description += " - " + itoa(cfg.Width) + "x" + itoa(cfg.Height)
		REDACTED
		case "video":
			description = "Video generation"
			if cfg.Orientation != "" {
				description += " - " + cfg.Orientation
		REDACTED
		case "prompt_enhance":
			description = "Prompt enhancement"
			if cfg.ExpansionLevel != "" {
				description += " - " + cfg.ExpansionLevel
		REDACTED
			if cfg.DurationS > 0 {
				description += " (" + itoa(cfg.DurationS) + "s)"
		REDACTED
		default:
			description = "Sora model"
	REDACTED
		models = append(models, ModelListItem{
			ID:          id,
			Object:      "model",
			OwnedBy:     "sora",
			Description: description,
	REDACTED)
REDACTED
	return models
REDACTED

func itoa(val int) string {
	if val == 0 {
		return "0"
REDACTED
	neg := false
	if val < 0 {
		neg = true
		val = -val
REDACTED
	buf := [12]byte{REDACTED
	i := len(buf)
	for val > 0 {
		i--
		buf[i] = byte('0' + val%10)
		val /= 10
REDACTED
	if neg {
		i--
		buf[i] = '-'
REDACTED
	return string(buf[i:])
REDACTED
