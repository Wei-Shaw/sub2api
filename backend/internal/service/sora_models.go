package service

import (
	"regexp"
	"sort"
	"strconv"
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

// SoraModelFamily 模型家族（前端 Sora 客户端使用）
type SoraModelFamily struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Orientations []string `json:"orientations"`
	Durations    []int    `json:"durations,omitempty"`
REDACTED

var (
	videoSuffixRe = regexp.MustCompile(`-(landscape|portrait)-(\d+)s$`)
	imageSuffixRe = regexp.MustCompile(`-(landscape|portrait)$`)

	soraFamilyNames = map[string]string{
		"sora2":       "Sora 2",
		"sora2pro":    "Sora 2 Pro",
		"sora2pro-hd": "Sora 2 Pro HD",
		"gpt-image":   "GPT Image",
REDACTED
)

// BuildSoraModelFamilies 从 soraModelConfigs 自动聚合模型家族及其支持的方向和时长
func BuildSoraModelFamilies() []SoraModelFamily {
	type familyData struct {
		modelType    string
		orientations map[string]bool
		durations    map[int]bool
REDACTED
	families := make(map[string]*familyData)

	for id, cfg := range soraModelConfigs {
		if cfg.Type == "prompt_enhance" {
			continue
	REDACTED
		var famID, orientation string
		var duration int

		switch cfg.Type {
		case "video":
			if m := videoSuffixRe.FindStringSubmatch(id); m != nil {
				famID = id[:len(id)-len(m[0])]
				orientation = m[1]
				duration, _ = strconv.Atoi(m[2])
		REDACTED
		case "image":
			if m := imageSuffixRe.FindStringSubmatch(id); m != nil {
				famID = id[:len(id)-len(m[0])]
				orientation = m[1]
		REDACTED else {
				famID = id
				orientation = "square"
		REDACTED
	REDACTED
		if famID == "" {
			continue
	REDACTED

		fd, ok := families[famID]
		if !ok {
			fd = &familyData{
				modelType:    cfg.Type,
				orientations: make(map[string]bool),
				durations:    make(map[int]bool),
		REDACTED
			families[famID] = fd
	REDACTED
		if orientation != "" {
			fd.orientations[orientation] = true
	REDACTED
		if duration > 0 {
			fd.durations[duration] = true
	REDACTED
REDACTED

	// 排序：视频在前、图像在后，同类按名称排序
	famIDs := make([]string, 0, len(families))
	for id := range families {
		famIDs = append(famIDs, id)
REDACTED
	sort.Slice(famIDs, func(i, j int) bool {
		fi, fj := families[famIDs[i]], families[famIDs[j]]
		if fi.modelType != fj.modelType {
			return fi.modelType == "video"
	REDACTED
		return famIDs[i] < famIDs[j]
REDACTED)

	result := make([]SoraModelFamily, 0, len(famIDs))
	for _, famID := range famIDs {
		fd := families[famID]
		fam := SoraModelFamily{
			ID:   famID,
			Name: soraFamilyNames[famID],
			Type: fd.modelType,
	REDACTED
		if fam.Name == "" {
			fam.Name = famID
	REDACTED
		for o := range fd.orientations {
			fam.Orientations = append(fam.Orientations, o)
	REDACTED
		sort.Strings(fam.Orientations)
		for d := range fd.durations {
			fam.Durations = append(fam.Durations, d)
	REDACTED
		sort.Ints(fam.Durations)
		result = append(result, fam)
REDACTED
	return result
REDACTED

// BuildSoraModelFamiliesFromIDs 从任意模型 ID 列表聚合模型家族（用于解析上游返回的模型列表）。
// 通过命名约定自动识别视频/图像模型并分组。
func BuildSoraModelFamiliesFromIDs(modelIDs []string) []SoraModelFamily {
	type familyData struct {
		modelType    string
		orientations map[string]bool
		durations    map[int]bool
REDACTED
	families := make(map[string]*familyData)

	for _, id := range modelIDs {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" || strings.HasPrefix(id, "prompt-enhance") {
			continue
	REDACTED

		var famID, orientation, modelType string
		var duration int

		if m := videoSuffixRe.FindStringSubmatch(id); m != nil {
			// 视频模型: {familyREDACTED-{orientationREDACTED-{durationREDACTEDs
			famID = id[:len(id)-len(m[0])]
			orientation = m[1]
			duration, _ = strconv.Atoi(m[2])
			modelType = "video"
	REDACTED else if m := imageSuffixRe.FindStringSubmatch(id); m != nil {
			// 图像模型（带方向）: {familyREDACTED-{orientationREDACTED
			famID = id[:len(id)-len(m[0])]
			orientation = m[1]
			modelType = "image"
	REDACTED else if cfg, ok := soraModelConfigs[id]; ok && cfg.Type == "image" {
			// 已知的无后缀图像模型（如 gpt-image）
			famID = id
			orientation = "square"
			modelType = "image"
	REDACTED else if strings.Contains(id, "image") {
			// 未知但名称包含 image 的模型，推断为图像模型
			famID = id
			orientation = "square"
			modelType = "image"
	REDACTED else {
			continue
	REDACTED

		if famID == "" {
			continue
	REDACTED

		fd, ok := families[famID]
		if !ok {
			fd = &familyData{
				modelType:    modelType,
				orientations: make(map[string]bool),
				durations:    make(map[int]bool),
		REDACTED
			families[famID] = fd
	REDACTED
		if orientation != "" {
			fd.orientations[orientation] = true
	REDACTED
		if duration > 0 {
			fd.durations[duration] = true
	REDACTED
REDACTED

	famIDs := make([]string, 0, len(families))
	for id := range families {
		famIDs = append(famIDs, id)
REDACTED
	sort.Slice(famIDs, func(i, j int) bool {
		fi, fj := families[famIDs[i]], families[famIDs[j]]
		if fi.modelType != fj.modelType {
			return fi.modelType == "video"
	REDACTED
		return famIDs[i] < famIDs[j]
REDACTED)

	result := make([]SoraModelFamily, 0, len(famIDs))
	for _, famID := range famIDs {
		fd := families[famID]
		fam := SoraModelFamily{
			ID:   famID,
			Name: soraFamilyNames[famID],
			Type: fd.modelType,
	REDACTED
		if fam.Name == "" {
			fam.Name = famID
	REDACTED
		for o := range fd.orientations {
			fam.Orientations = append(fam.Orientations, o)
	REDACTED
		sort.Strings(fam.Orientations)
		for d := range fd.durations {
			fam.Durations = append(fam.Durations, d)
	REDACTED
		sort.Ints(fam.Durations)
		result = append(result, fam)
REDACTED
	return result
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
