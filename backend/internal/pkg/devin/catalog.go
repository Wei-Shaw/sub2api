// catalog.go 解码 GetCliModelConfigs 模型目录并把扁平目录按
// base×level×variant 归组为对客户端暴露的分组模型（thinkingLevelMap
// 语义对齐 devin-connect 插件：对外模型 id 是族名，effort 映射到
// 具体上游 uid，如 swe-2:max → swe-2-max）。
package devin

import (
	"strings"
)

// Model 是上游目录中的一个可用模型（扁平条目，uid 即对外/上游标识）。
type Model struct {
	ID                        string
	Name                      string
	SupportsImages            bool
	SupportsThinking          bool
	SupportsToolCalls         bool
	SupportsParallelToolCalls bool
	PreserveThinking          bool
	IsRouter                  bool
	ContextWindow             int
	MaxTokens                 int
	OwnedBy                   string
}

// decodeFeatures 解析 model_info.model_features（ExaCodeiumCommonPb_ModelFeatures）。
func decodeFeatures(buf []byte) (images, tools, parallel, thinking, preserve bool) {
	iterFields(buf, func(f protoField) bool {
		switch f.num {
		case 11:
			images = f.bool()
		case 12:
			tools = f.bool()
		case 21:
			parallel = f.bool()
		case 15:
			thinking = f.bool()
		case 25:
			preserve = f.bool()
		}
		return true
	})
	return
}

// decodeModelInfo 解析 model_info（ExaCodeiumCommonPb_ModelInfo）。
func decodeModelInfo(buf []byte) (maxTokens, maxOutput int, isRouter bool, features struct {
	images, tools, parallel, thinking, preserve bool
}) {
	features.tools = true // 目录未声明 features 时按支持工具处理
	iterFields(buf, func(f protoField) bool {
		switch f.num {
		case 4:
			maxTokens = int(f.int())
		case 6:
			features.images, features.tools, features.parallel, features.thinking, features.preserve = decodeFeatures(f.bytes())
		case 13:
			maxOutput = int(f.int())
		case 25:
			isRouter = f.bool()
		}
		return true
	})
	return
}

// decodeAliasUid 取 model_or_alias.model_uid（f3）。
func decodeAliasUid(buf []byte) string { return fieldString(buf, 3) }

// decodeClientModel 解析一个 ClientModelConfig；disabled/无 uid 返回 nil。
func decodeClientModel(buf []byte) *Model {
	var label, uid, provider string
	var disabled, supportsImages bool
	var contextWindow int
	var infoMaxTokens, infoMaxOutput int
	var infoIsRouter bool
	var infoFeatures struct {
		images, tools, parallel, thinking, preserve bool
	}
	hasInfo := false
	iterFields(buf, func(f protoField) bool {
		switch f.num {
		case 1:
			label = f.string()
		case 2:
			if uid == "" {
				uid = decodeAliasUid(f.bytes())
			}
		case 4:
			disabled = f.bool()
		case 5:
			supportsImages = f.bool()
		case 10:
			provider = modelProviderName(int(f.int()))
		case 18:
			contextWindow = int(f.int())
		case 22:
			if s := f.string(); s != "" {
				uid = s
			}
		case 23:
			hasInfo = true
			infoMaxTokens, infoMaxOutput, infoIsRouter, infoFeatures = decodeModelInfo(f.bytes())
		}
		return true
	})
	if uid == "" || disabled {
		return nil
	}
	model := &Model{
		ID:               uid,
		Name:             label,
		SupportsImages:   supportsImages || infoFeatures.images,
		IsRouter:         infoIsRouter,
		ContextWindow:    contextWindow,
		MaxTokens:        infoMaxOutput,
		SupportsThinking: infoFeatures.thinking,
	}
	if !hasInfo {
		model.SupportsToolCalls = true
	} else {
		model.SupportsToolCalls = infoFeatures.tools
	}
	model.SupportsParallelToolCalls = infoFeatures.parallel
	model.PreserveThinking = infoFeatures.preserve
	if model.ContextWindow == 0 {
		if infoMaxTokens > 0 {
			model.ContextWindow = infoMaxTokens
		} else {
			model.ContextWindow = 256_000
		}
	}
	if model.MaxTokens == 0 {
		model.MaxTokens = 128_000
	}
	if model.Name == "" {
		model.Name = uid
	}
	model.OwnedBy = providerOwnedBy(provider)
	return model
}

// modelProviderName 把 ExaCodeiumCommonPb_ModelProvider 枚举值映射为
// proto 枚举名尾段（devin2api 用 c.GetProvider().String() 后取 "_" 尾段）。
func modelProviderName(v int) string {
	switch v {
	case 1:
		return "windsurf"
	case 2:
		return "openai"
	case 3:
		return "anthropic"
	case 4:
		return "google"
	case 5:
		return "xai"
	case 6:
		return "deepseek"
	case 7:
		return "moonshot"
	case 8:
		return "qwen"
	case 9:
		return "zai"
	case 10:
		return "minimax"
	case 11:
		return "nvidia"
	case 12:
		return "thinking_machines"
	}
	return ""
}

// providerOwnedBy 从 provider 枚举名取尾段。
func providerOwnedBy(provider string) string {
	if provider == "" {
		return "devin"
	}
	if i := strings.LastIndex(provider, "_"); i >= 0 && i+1 < len(provider) {
		return strings.ToLower(provider[i+1:])
	}
	return strings.ToLower(provider)
}

// DecodeModelCatalog 解析 GetCliModelConfigs 响应体。
func DecodeModelCatalog(body []byte) []Model {
	var models []Model
	seen := make(map[string]struct{})
	iterFields(body, func(f protoField) bool {
		if f.num != 1 {
			return true
		}
		model := decodeClientModel(f.bytes())
		if model == nil {
			return true
		}
		if _, ok := seen[model.ID]; ok {
			return true
		}
		seen[model.ID] = struct{}{}
		models = append(models, *model)
		return true
	})
	return models
}

// MarshalGetCliModelConfigs 构造 GetCliModelConfigs 请求体。
func MarshalGetCliModelConfigs(token, clientVersion, os string) []byte {
	var body []byte
	body = appendMessage(body, 1, BuildMetadata(token, clientVersion, os, true))
	return body
}

// MarshalAssignModel 构造 AssignModel 请求体（metadata + router uid +
// cascade id——jwt 绑 cascade_id，上游实测）。
func MarshalAssignModel(token, clientVersion, os, routerUID, cascadeID string) []byte {
	var body []byte
	body = appendMessage(body, 1, BuildMetadata(token, clientVersion, os, false))
	body = appendString(body, 2, routerUID)
	body = appendString(body, 3, cascadeID)
	return body
}

// ModelAssignment 是 AssignModel 的解析结果。
type ModelAssignment struct {
	ModelUID      string
	AssignmentJWT string
}

// DecodeAssignModelResponse 解析 AssignModel 响应：
// assignment{1: assignment_jwt, 2: model_uid}。
func DecodeAssignModelResponse(body []byte) (ModelAssignment, error) {
	var out ModelAssignment
	iterFields(body, func(f protoField) bool {
		if f.num != 1 {
			return true
		}
		iterFields(f.bytes(), func(inner protoField) bool {
			switch inner.num {
			case 1:
				out.AssignmentJWT = inner.string()
			case 2:
				out.ModelUID = inner.string()
			}
			return true
		})
		return true
	})
	return out, nil
}

// ---------------------------------------------------------------------------
// 目录归组：把扁平目录（base × thinking level × variant）收敛回分组模型，
// effort 经 thinkingLevelMap 解析到具体 uid。
// ---------------------------------------------------------------------------

// PiLevel 是 Devin 系模型可暴露的思考档位。
type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"
	ThinkingMax     ThinkingLevel = "max"
)

// LevelOrder 是档位的强度序。
var LevelOrder = []ThinkingLevel{
	ThinkingOff, ThinkingMinimal, ThinkingLow, ThinkingMedium,
	ThinkingHigh, ThinkingXHigh, ThinkingMax,
}

// groupedModel 是对外暴露的分组模型条目。
type GroupedModel struct {
	// ID 是对外模型 id（族名，如 "swe-2"）。
	ID string
	// Name 是显示名。
	Name string
	// ThinkingLevelMap 是 effort → 上游 uid 的映射；nil 表示无分级。
	ThinkingLevelMap map[ThinkingLevel]string
	// Reasoning 表示该模型支持 thinking。
	Reasoning bool
	// SupportsImages 表示支持图片输入。
	SupportsImages bool
	// ContextWindow / MaxTokens 是聚合成员的最大值。
	ContextWindow int
	MaxTokens     int
	// OwnedBy 是供应商归属（devin 自营模型为 "devin"）。
	OwnedBy string
}

// parsedModel 是 uid/显示名拆解出的 (base, level, variant)。
type parsedModel struct {
	base    string
	level   ThinkingLevel // "" 表示无档位；"on" 表示未分级的 thinking 形态
	variant string        // "" / "fast" / "1m"
}

// parseModelName 拆显示名为 base + thinking level + 速度/上下文变体。
func parseModelName(name string) parsedModel {
	rest := strings.TrimSpace(name)
	variant := ""
	for {
		if hasSuffixFold(rest, " Fast") {
			variant = "fast"
			rest = rest[:len(rest)-5]
		} else if strings.HasSuffix(rest, " 1M") {
			variant = "1m"
			rest = rest[:len(rest)-3]
		} else {
			break
		}
	}
	var level ThinkingLevel
	if m := matchModelLevelSuffix(rest); m != nil {
		level = m.level
		rest = rest[:len(rest)-m.trimmed]
	} else if m := matchModelThinkingSuffix(rest); m != nil {
		level = m.level
		rest = rest[:len(rest)-m.trimmed]
	}
	return parsedModel{base: strings.TrimSpace(rest), level: level, variant: variant}
}

func hasSuffixFold(s, suffix string) bool {
	return len(s) >= len(suffix) && strings.EqualFold(s[len(s)-len(suffix):], suffix)
}

type levelMatch struct {
	level   ThinkingLevel
	trimmed int // 需要从 base 尾端剥掉的字节数
}

// matchModelLevelSuffix 匹配 " (None|Minimal|Low|Medium|High|XHigh|X-High|Max)( Thinking)?$"。
func matchModelLevelSuffix(rest string) *levelMatch {
	for _, word := range []struct {
		pat   string
		level ThinkingLevel
	}{
		{"X-High", ThinkingXHigh},
		{"XHigh", ThinkingXHigh},
		{"Minimal", ThinkingMinimal},
		{"Medium", ThinkingMedium},
		{"None", ThinkingOff},
		{"High", ThinkingHigh},
		{"Low", ThinkingLow},
		{"Max", ThinkingMax},
	} {
		for _, suffix := range []string{" " + word.pat + " Thinking", " " + word.pat} {
			if hasSuffixFold(rest, suffix) {
				return &levelMatch{level: word.level, trimmed: len(suffix)}
			}
		}
	}
	return nil
}

// matchModelThinkingSuffix 匹配 " (No Thinking|Thinking)$"。
func matchModelThinkingSuffix(rest string) *levelMatch {
	for _, pair := range []struct {
		pat   string
		level ThinkingLevel
	}{
		{"No Thinking", ThinkingOff},
		{"Thinking", "on"},
	} {
		suffix := " " + pair.pat
		if hasSuffixFold(rest, suffix) {
			return &levelMatch{level: pair.level, trimmed: len(suffix)}
		}
	}
	return nil
}

// UidThinkingLevel 从具体上游 uid 回推思考档位（如 swe-2-max → "max"）。
// uid 未携带可识别的档位词时返回 ""——供用量记录推导实际生效的 effort。
func UidThinkingLevel(uid string) string {
	if level, _, ok := parseModelUid(uid); ok {
		return string(level)
	}
	return ""
}

// parseModelUid 是显示名无档位/变体时从 uid 回推的兜底。
func parseModelUid(uid string) (level ThinkingLevel, variant string, ok bool) {
	lower := strings.ToLower(uid)
	for i := len(lower); i > 0; i-- {
		if lower[i-1] != '-' && lower[i-1] != '_' {
			continue
		}
		rest := lower[i-1:]
		if v, ok := parseUidLevelWord(rest); ok {
			return v.level, v.variant, true
		}
	}
	return "", "", false
}

type uidParse struct {
	level   ThinkingLevel
	variant string
}

// parseUidLevelWord 匹配 uid 尾段 "-(none|minimal|low|medium|high|xhigh|max|thinking)(-(priority|fast|1m))?$"。
func parseUidLevelWord(rest string) (uidParse, bool) {
	// rest 以 - 或 _ 开头。
	body := rest[1:]
	// 先拆变体尾段。
	variant := ""
	for _, v := range []string{"-priority", "-fast", "-1m", "_priority", "_fast", "_1m"} {
		if strings.HasSuffix(body, v) {
			variant = v[1:]
			body = body[:len(body)-len(v)]
			break
		}
	}
	if variant == "priority" {
		variant = "fast"
	}
	switch body {
	case "none":
		return uidParse{level: ThinkingOff, variant: variant}, true
	case "thinking":
		return uidParse{level: "on", variant: variant}, true
	case "minimal":
		return uidParse{level: ThinkingMinimal, variant: variant}, true
	case "low":
		return uidParse{level: ThinkingLow, variant: variant}, true
	case "medium":
		return uidParse{level: ThinkingMedium, variant: variant}, true
	case "high":
		return uidParse{level: ThinkingHigh, variant: variant}, true
	case "xhigh":
		return uidParse{level: ThinkingXHigh, variant: variant}, true
	case "max":
		return uidParse{level: ThinkingMax, variant: variant}, true
	}
	return uidParse{}, false
}

// parseModel 综合显示名与 uid 得到 (base, level, variant)。
func parseModel(model Model) parsedModel {
	fromName := parseModelName(model.Name)
	if fromName.level != "" && fromName.variant != "" {
		return fromName
	}
	uidLevel, uidVariant, ok := parseModelUid(model.ID)
	if !ok {
		return parsedModel{base: fromName.base, level: fromName.level, variant: fromName.variant}
	}
	level := fromName.level
	if level == "" {
		level = uidLevel
	}
	variant := fromName.variant
	if variant == "" {
		variant = uidVariant
	}
	return parsedModel{base: fromName.base, level: level, variant: variant}
}

// slugify 把族名转为小写连字符 id 形式。
func slugify(text string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "model"
	}
	return out
}

// rank 决定分组模型在对外列表中的展示序。
func rank(id string) int {
	s := strings.ToLower(id)
	switch {
	case s == "swe-2":
		return 0
	case strings.HasPrefix(s, "swe-2"):
		return 1
	case strings.HasPrefix(s, "swe-"):
		return 2
	case strings.Contains(s, "fable-5-1"):
		return 3
	case strings.Contains(s, "astra"):
		return 4
	case strings.Contains(s, "fable"):
		return 5
	case strings.Contains(s, "opus-5"):
		return 6
	case strings.Contains(s, "sonnet-5"):
		return 7
	case s == "adaptive" || strings.HasPrefix(s, "fusion"):
		return 8
	}
	return 20
}

type modelGroup struct {
	base    string
	variant string
	members []Model
	levels  map[ThinkingLevel]string
	onUID   string
	plain   []Model
}

func variantLabel(variant string) (idSuffix, nameSuffix string) {
	switch variant {
	case "fast":
		return "-fast", " Fast"
	case "1m":
		return "-1m", " 1M"
	}
	return "", ""
}

// GroupModels 把上游扁平目录归组为对外暴露的模型列表。
// 规则与 devin-connect catalog.ts 的 toProviderModels 一致：
//   - 带 thinking 档位的族合并为一个 id，effort 经 thinkingLevelMap 解析；
//   - 无档位但 supportsThinking 的成员单独成项（effort 只有 high）；
//   - 普通 "Thinking"（无分级）成员占用第一个空的高档槽位；
//   - 非 thinking 成员作为该族的 "off" 档。
func GroupModels(models []Model) []GroupedModel {
	groups := make(map[string]*modelGroup)
	order := []string{}
	for _, model := range models {
		parsed := parseModel(model)
		key := parsed.base + "|" + parsed.variant
		group := groups[key]
		if group == nil {
			group = &modelGroup{base: parsed.base, variant: parsed.variant, levels: make(map[ThinkingLevel]string)}
			groups[key] = group
			order = append(order, key)
		}
		group.members = append(group.members, model)
		switch {
		case parsed.level == "on":
			if group.onUID == "" {
				group.onUID = model.ID
			}
		case parsed.level != "":
			if _, ok := group.levels[parsed.level]; !ok {
				group.levels[parsed.level] = model.ID
			}
		case model.SupportsThinking:
			group.plain = append(group.plain, model)
		default:
			if _, ok := group.levels[ThinkingOff]; !ok {
				group.levels[ThinkingOff] = model.ID
			}
		}
	}

	var out []GroupedModel
	usedIDs := make(map[string]bool)
	for _, key := range order {
		group := groups[key]
		idSuffix, nameSuffix := variantLabel(group.variant)
		name := group.base + nameSuffix
		thinkingLevelMap := map[ThinkingLevel]string{
			ThinkingOff: "", ThinkingMinimal: "", ThinkingLow: "", ThinkingMedium: "",
			ThinkingHigh: "", ThinkingXHigh: "", ThinkingMax: "",
		}
		for level, uid := range group.levels {
			thinkingLevelMap[level] = uid
		}
		if group.onUID != "" {
			for _, slot := range []ThinkingLevel{ThinkingHigh, ThinkingMedium, ThinkingLow, ThinkingMinimal, ThinkingXHigh, ThinkingMax} {
				if thinkingLevelMap[slot] == "" {
					thinkingLevelMap[slot] = group.onUID
					break
				}
			}
		}
		hasLevels := false
		for _, level := range LevelOrder {
			if thinkingLevelMap[level] != "" {
				hasLevels = true
				break
			}
		}
		if hasLevels {
			id := slugify(group.base) + idSuffix
			for i := 2; usedIDs[id]; i++ {
				id = slugify(group.base) + idSuffix + "-" + itoa(i)
			}
			usedIDs[id] = true
			out = append(out, GroupedModel{
				ID:               id,
				Name:             name,
				ThinkingLevelMap: thinkingLevelMap,
				Reasoning:        anyMember(group.members, func(m Model) bool { return m.SupportsThinking }),
				SupportsImages:   anyMember(group.members, func(m Model) bool { return m.SupportsImages }),
				ContextWindow:    maxMember(group.members, func(m Model) int { return orDefault(m.ContextWindow, 256_000) }),
				MaxTokens:        maxMember(group.members, func(m Model) int { return orDefault(m.MaxTokens, 128_000) }),
				OwnedBy:          "devin",
			})
		}
		for _, model := range group.plain {
			singleName := name
			if len(group.plain) > 1 {
				singleName = model.Name
			}
			out = append(out, singleGroupedModel(model, singleName))
		}
		if !hasLevels && len(group.plain) == 0 {
			for _, model := range group.members {
				out = append(out, singleGroupedModel(model, model.Name))
			}
		}
	}
	sortGroupedModels(out)
	return out
}

func anyMember(members []Model, pred func(Model) bool) bool {
	for _, m := range members {
		if pred(m) {
			return true
		}
	}
	return false
}

func maxMember(members []Model, value func(Model) int) int {
	max := 0
	for _, m := range members {
		if v := value(m); v > max {
			max = v
		}
	}
	return max
}

func orDefault(v, d int) int {
	if v == 0 {
		return d
	}
	return v
}

// singleGroupedModel 把无分级条目展开为独立模型（id=uid）。
func singleGroupedModel(model Model, name string) GroupedModel {
	var thinkingLevelMap map[ThinkingLevel]string
	if model.SupportsThinking {
		thinkingLevelMap = map[ThinkingLevel]string{
			ThinkingOff: "", ThinkingMinimal: "", ThinkingLow: "", ThinkingMedium: "",
			ThinkingHigh: model.ID, ThinkingXHigh: "", ThinkingMax: "",
		}
	}
	return GroupedModel{
		ID:               model.ID,
		Name:             name,
		ThinkingLevelMap: thinkingLevelMap,
		Reasoning:        model.SupportsThinking,
		SupportsImages:   model.SupportsImages,
		ContextWindow:    orDefault(model.ContextWindow, 256_000),
		MaxTokens:        orDefault(model.MaxTokens, 128_000),
		OwnedBy:          model.OwnedBy,
	}
}

func sortGroupedModels(models []GroupedModel) {
	for i := 1; i < len(models); i++ {
		for j := i; j > 0; j-- {
			a, b := models[j-1], models[j]
			if rank(a.ID) < rank(b.ID) || (rank(a.ID) == rank(b.ID) && a.Name <= b.Name) {
				break
			}
			models[j-1], models[j] = b, a
		}
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// ResolveModelUID 把对外模型 id + effort 解析为上游 uid。
// 无映射时直接返回 id（裸 uid 直通）；map 命中时先向更高档找，
// 再回落更低档（与插件 resolveModelUid 一致）。
func ResolveModelUID(grouped GroupedModel, reasoning string) string {
	if grouped.ThinkingLevelMap == nil {
		return grouped.ID
	}
	wanted := ThinkingHigh
	if reasoning != "" {
		for _, level := range LevelOrder {
			if string(level) == reasoning {
				wanted = level
				break
			}
		}
	}
	from := indexOfLevel(wanted)
	for i := from; i < len(LevelOrder); i++ {
		if uid := grouped.ThinkingLevelMap[LevelOrder[i]]; uid != "" {
			return uid
		}
	}
	for i := from - 1; i >= 0; i-- {
		if uid := grouped.ThinkingLevelMap[LevelOrder[i]]; uid != "" {
			return uid
		}
	}
	return grouped.ID
}

func indexOfLevel(level ThinkingLevel) int {
	for i, l := range LevelOrder {
		if l == level {
			return i
		}
	}
	return 4 // ThinkingHigh
}

// PickOverflowUID 在请求被上游判定过长时挑一个更大上下文的 uid。
// 优先级：fusion 配对（sidekick 保留当前 uid）> 最大上下文的非 router
// 条目（按 rank 序）。catalog 为上游目录原始扁平模型集。
func PickOverflowUID(currentUID string, catalog []Model) string {
	// fusion 路由项：sidekick 保留当前 uid 的配对优先。
	for _, lead := range []string{"gpt-6-astra-high", "claude-fable-5-1-high", "claude-opus-5-high", "gpt-5-6-sol-high"} {
		for _, model := range catalog {
			if model.IsRouter && strings.HasSuffix(model.ID, "-sidekick-"+currentUID) && strings.Contains(model.ID, "-"+lead+"-") {
				return model.ID
			}
		}
	}
	for _, model := range catalog {
		if model.IsRouter && strings.HasSuffix(model.ID, "-sidekick-"+currentUID) {
			return model.ID
		}
	}
	best := ""
	bestRank := 1 << 30
	for _, model := range catalog {
		if model.IsRouter || model.ID == currentUID {
			continue
		}
		if model.ContextWindow < 1_000_000 {
			continue
		}
		if r := rank(model.ID); r < bestRank {
			bestRank = r
			best = model.ID
		}
	}
	return best
}

// ResolveCatalogModelUID 把对外暴露的分组模型 id（如 swe-2）解析为真实上游 uid。
// 语义与 devin-connect catalog.ts 的 resolveModelUid 一致（内部委托
// ResolveModelUID）：未指定 effort 档默认 "high"，沿 LevelOrder 先向
// 更高档再向更低档取最近可用 uid。支持 "model:level" 后缀语法（如
// swe-2:max），显式 effort 参数优先；model 不是任何分组 id（原始 uid /
// 未知模型）时原样透传，交给上游裁决。
func ResolveCatalogModelUID(groups []GroupedModel, model, effort string) string {
	m := strings.TrimSpace(model)
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" {
		if base, level, ok := SplitModelLevelSuffix(m); ok {
			effort = level
			m = base
		}
	}
	for i := range groups {
		if groups[i].ID == m {
			return ResolveModelUID(groups[i], effort)
		}
	}
	return m
}

// SplitModelLevelSuffix 剥离 "model:level" 语法中的 level 后缀（如
// swe-2:max → (swe-2, max, true)）。仅当冒号后缀是合法思考档位时
// 才拆分；否则原样返回 model 与 ok=false。供调度/白名单层在匹配
// model_mapping 键时剥离档位后缀（档位与模型选择正交）。
func SplitModelLevelSuffix(model string) (base string, level string, ok bool) {
	m := strings.TrimSpace(model)
	if i := strings.LastIndex(m, ":"); i > 0 && i < len(m)-1 {
		if lvl := strings.ToLower(m[i+1:]); indexOfLevel(ThinkingLevel(lvl)) >= 0 {
			return m[:i], lvl, true
		}
	}
	return m, "", false
}
