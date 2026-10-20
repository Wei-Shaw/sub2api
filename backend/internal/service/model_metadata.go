package service

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// modelMetadataFile is the single source of truth for the model plaza catalog.
// Replace this file when the catalog is updated; runtime group/channel/pricing
// configuration remains database-backed.
//
//go:embed model_metadata_v7.json
var modelMetadataFile []byte

// ModelMetadataSettingKey is retained for the legacy metadata import command.
// Runtime model-plaza reads the embedded v7 catalog above instead of this
// settings-table value.
const ModelMetadataSettingKey = "bestloong_model_metadata_v1"

// IncompleteMetadataPlaceholder marks business-owned values that are not
// available yet. It is intentionally visible rather than silently inventing
// prices or descriptions.
const IncompleteMetadataPlaceholder = "待补充"

// AllAuthorizedGroupsMarker is used only by the draft importer while the
// business authorization mapping is still pending.
const AllAuthorizedGroupsMarker = "*"

var modelMetadataCategories = map[string]struct{}{
	"文本": {}, "多模态": {}, "图像": {}, "代码": {}, "语音": {}, "视频": {},
}

// ModelMetadata is the business-owned model catalog record.
// Prices intentionally stay as display strings because a record may contain multiple
// tiers (for example "≤272K ￥216 / >272K ￥432"). Billing remains unchanged.
type ModelMetadata struct {
	CallName         string   `json:"调用名"`
	DisplayName      string   `json:"展示名"`
	Capability       string   `json:"能力边界"`
	UseCases         string   `json:"适用场景"`
	Categories       []string `json:"类别"`
	TierCondition    string   `json:"档位条件"`
	InputPrice       string   `json:"输入价"`
	OutputPrice      string   `json:"输出价"`
	CacheReadPrice   string   `json:"缓存读"`
	CacheWritePrice  string   `json:"缓存写"`
	Glossary         string   `json:"术语解释"`
	LaunchDate       string   `json:"上架日"`
	AuthorizedGroups []string `json:"授权分组"`
	// PriceTable and PricingNotes preserve non-token catalogs such as Seedance.
	// Each row is intentionally open-ended because providers may add dimensions.
	PriceTable   []map[string]string `json:"价格表,omitempty"`
	PricingNotes []string            `json:"计价说明,omitempty"`
}

// ParseAndValidateModelMetadata performs strict, all-or-nothing validation.
// knownGroups may be nil for syntax-only validation; otherwise every referenced
// group must exist in the current database.
func ParseAndValidateModelMetadata(raw []byte, knownGroups map[string]struct{}) ([]ModelMetadata, error) {
	if normalized, ok, err := normalizeV3ModelMetadata(raw); err != nil {
		return nil, err
	} else if ok {
		raw = normalized
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var records []ModelMetadata
	if err := dec.Decode(&records); err != nil {
		return nil, fmt.Errorf("JSON 格式错误或包含非 13 字段: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("元数据不能为空")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("JSON 数组后存在多余内容")
		}
		return nil, fmt.Errorf("JSON 数组后存在无效内容: %w", err)
	}

	seen := make(map[string]int, len(records))
	errs := make([]string, 0)
	for i := range records {
		r := &records[i]
		row := i + 1
		r.CallName = strings.TrimSpace(r.CallName)
		r.DisplayName = strings.TrimSpace(r.DisplayName)
		r.Capability = strings.TrimSpace(r.Capability)
		r.UseCases = strings.TrimSpace(r.UseCases)
		r.TierCondition = strings.TrimSpace(r.TierCondition)
		r.InputPrice = strings.TrimSpace(r.InputPrice)
		r.OutputPrice = strings.TrimSpace(r.OutputPrice)
		r.CacheReadPrice = strings.TrimSpace(r.CacheReadPrice)
		r.CacheWritePrice = strings.TrimSpace(r.CacheWritePrice)
		r.Glossary = strings.TrimSpace(r.Glossary)
		r.LaunchDate = strings.TrimSpace(r.LaunchDate)

		for name, value := range map[string]string{
			"调用名": r.CallName, "展示名": r.DisplayName, "能力边界": r.Capability,
			"适用场景": r.UseCases, "档位条件": r.TierCondition, "输入价": r.InputPrice,
			"输出价": r.OutputPrice, "缓存读": r.CacheReadPrice, "缓存写": r.CacheWritePrice,
			"术语解释": r.Glossary, "上架日": r.LaunchDate,
		} {
			if value == "" {
				errs = append(errs, fmt.Sprintf("第 %d 条（%s）字段「%s」不能为空", row, fallbackName(r.CallName), name))
			}
		}
		if prev, ok := seen[strings.ToLower(r.CallName)]; ok && r.CallName != "" {
			errs = append(errs, fmt.Sprintf("第 %d 条调用名「%s」与第 %d 条重复", row, r.CallName, prev))
		} else if r.CallName != "" {
			seen[strings.ToLower(r.CallName)] = row
		}
		if _, err := time.Parse("2006-01-02", r.LaunchDate); err != nil && r.LaunchDate != "" {
			errs = append(errs, fmt.Sprintf("第 %d 条（%s）上架日必须为 YYYY-MM-DD", row, fallbackName(r.CallName)))
		}

		r.Categories = normalizeUniqueStrings(r.Categories)
		if len(r.Categories) == 0 {
			errs = append(errs, fmt.Sprintf("第 %d 条（%s）字段「类别」必须是非空数组", row, fallbackName(r.CallName)))
		}
		r.Categories = normalizeMetadataCategories(r.Categories)
		for _, category := range r.Categories {
			if _, ok := modelMetadataCategories[category]; !ok {
				errs = append(errs, fmt.Sprintf("第 %d 条（%s）类别「%s」不在文本/多模态/图像/视频/代码/语音内", row, fallbackName(r.CallName), category))
			}
		}

		// The mapping is applied at import time and may add multiple labels. The
		// business-provided labels remain valid seeds for edge cases the draft rules
		// cannot infer reliably.
		r.Categories = normalizeUniqueStrings(append(r.Categories, inferModelCategories(*r)...))
		r.AuthorizedGroups = normalizeUniqueStrings(r.AuthorizedGroups)
		if len(r.AuthorizedGroups) == 0 {
			errs = append(errs, fmt.Sprintf("第 %d 条（%s）字段「授权分组」必须是非空数组", row, fallbackName(r.CallName)))
		}
		if knownGroups != nil {
			for _, group := range r.AuthorizedGroups {
				if _, ok := knownGroups[strings.ToLower(group)]; !ok {
					errs = append(errs, fmt.Sprintf("第 %d 条（%s）授权分组「%s」不存在", row, fallbackName(r.CallName), group))
				}
			}
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("元数据校验失败（共 %d 项）:\n- %s", len(errs), strings.Join(errs, "\n- "))
	}
	return records, nil
}

// normalizeV3ModelMetadata converts the provider-owned v3 envelope into the
// canonical catalog shape while retaining its open-ended price table. The
// canonical shape is still used for storage and the existing API contract.
func normalizeV3ModelMetadata(raw []byte) ([]byte, bool, error) {
	var envelope struct {
		Records []map[string]json.RawMessage `json:"模型元数据v3"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&envelope); err != nil || envelope.Records == nil {
		return nil, false, nil
	}
	if len(envelope.Records) == 0 {
		return nil, true, fmt.Errorf("元数据不能为空")
	}
	rows := make([]ModelMetadata, 0, len(envelope.Records))
	for i, source := range envelope.Records {
		var row ModelMetadata
		decodeString := func(key string, dst *string) error {
			if value, exists := source[key]; exists {
				if err := json.Unmarshal(value, dst); err != nil {
					return fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			}
			return nil
		}
		for key, dst := range map[string]*string{
			"调用名": &row.CallName, "展示名": &row.DisplayName, "能力边界": &row.Capability,
			"适用场景": &row.UseCases, "上架日": &row.LaunchDate,
		} {
			if err := decodeString(key, dst); err != nil {
				return nil, true, err
			}
		}
		if value, exists := source["类别"]; exists {
			if err := json.Unmarshal(value, &row.Categories); err != nil {
				var category string
				if err := json.Unmarshal(value, &category); err != nil {
					return nil, true, fmt.Errorf("第 %d 条字段「类别」必须是字符串或数组", i+1)
				}
				row.Categories = []string{category}
			}
		}
		if value, exists := source["授权分组"]; exists {
			if err := json.Unmarshal(value, &row.AuthorizedGroups); err != nil {
				var group string
				if err := json.Unmarshal(value, &group); err != nil {
					return nil, true, fmt.Errorf("第 %d 条字段「授权分组」必须是字符串或数组", i+1)
				}
				row.AuthorizedGroups = []string{group}
			}
		}
		if value, exists := source["价格表"]; exists {
			if err := json.Unmarshal(value, &row.PriceTable); err != nil {
				return nil, true, fmt.Errorf("第 %d 条字段「价格表」格式错误: %w", i+1, err)
			}
		}
		if value, exists := source["计价说明"]; exists {
			if err := json.Unmarshal(value, &row.PricingNotes); err != nil {
				return nil, true, fmt.Errorf("第 %d 条字段「计价说明」格式错误: %w", i+1, err)
			}
		}
		// The legacy fields are display fallbacks for cards that do not have a
		// provider-specific table. Seedance's real prices are rendered from the table.
		row.Categories = normalizeMetadataCategories(row.Categories)
		row.TierCondition = "见价格表"
		row.InputPrice, row.OutputPrice = "见价格表", "见价格表"
		row.CacheReadPrice, row.CacheWritePrice = "-", "-"
		row.Glossary = strings.Join(row.PricingNotes, "；")
		if row.Glossary == "" {
			row.Glossary = "见价格表"
		}
		rows = append(rows, row)
	}
	canonical, err := json.Marshal(rows)
	return canonical, true, err
}

func normalizeMetadataCategories(categories []string) []string {
	out := make([]string, 0, len(categories))
	for _, category := range categories {
		switch strings.TrimSpace(category) {
		case "视频生成", "生成视频", "视频":
			out = append(out, "视频")
		case "绘画", "图片生成", "图像生成":
			out = append(out, "图像")
		default:
			out = append(out, category)
		}
	}
	return normalizeUniqueStrings(out)
}

// ParseAndNormalizeIncompleteModelMetadata prepares the current draft catalog
// for import. It accepts the original category string format and fills only
// the business fields explicitly known to be pending. Required identity and
// pricing fields that are present in the source remain untouched.
func ParseAndNormalizeIncompleteModelMetadata(raw []byte) ([]ModelMetadata, error) {
	if normalized, ok, err := normalizeV3ModelMetadata(raw); err != nil {
		return nil, err
	} else if ok {
		raw = normalized
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("JSON 格式错误: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("元数据不能为空")
	}

	normalized := make([]ModelMetadata, 0, len(rows))
	for i, row := range rows {
		var record ModelMetadata
		for key, value := range row {
			switch key {
			case "调用名":
				if err := json.Unmarshal(value, &record.CallName); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "展示名":
				if err := json.Unmarshal(value, &record.DisplayName); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "能力边界":
				if err := json.Unmarshal(value, &record.Capability); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "适用场景":
				if err := json.Unmarshal(value, &record.UseCases); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "类别":
				if err := json.Unmarshal(value, &record.Categories); err != nil {
					var category string
					if stringErr := json.Unmarshal(value, &category); stringErr != nil {
						return nil, fmt.Errorf("第 %d 条字段「类别」必须是字符串或数组", i+1)
					}
					record.Categories = []string{category}
				}
			case "档位条件":
				if err := json.Unmarshal(value, &record.TierCondition); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "输入价":
				if err := json.Unmarshal(value, &record.InputPrice); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "输出价":
				if err := json.Unmarshal(value, &record.OutputPrice); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "缓存读":
				if err := json.Unmarshal(value, &record.CacheReadPrice); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "缓存写":
				if err := json.Unmarshal(value, &record.CacheWritePrice); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "术语解释":
				if err := json.Unmarshal(value, &record.Glossary); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "上架日":
				if err := json.Unmarshal(value, &record.LaunchDate); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "授权分组":
				if err := json.Unmarshal(value, &record.AuthorizedGroups); err != nil {
					var group string
					if stringErr := json.Unmarshal(value, &group); stringErr != nil {
						return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
					}
					record.AuthorizedGroups = []string{group}
				}
			case "价格表":
				if err := json.Unmarshal(value, &record.PriceTable); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			case "计价说明":
				if err := json.Unmarshal(value, &record.PricingNotes); err != nil {
					return nil, fmt.Errorf("第 %d 条字段「%s」格式错误: %w", i+1, key, err)
				}
			}
		}
		if strings.TrimSpace(record.OutputPrice) == "" {
			record.OutputPrice = IncompleteMetadataPlaceholder
		}
		if strings.TrimSpace(record.CacheWritePrice) == "" {
			record.CacheWritePrice = IncompleteMetadataPlaceholder
		}
		if strings.TrimSpace(record.Glossary) == "" {
			record.Glossary = IncompleteMetadataPlaceholder
		}
		if len(normalizeUniqueStrings(record.AuthorizedGroups)) == 0 {
			record.AuthorizedGroups = []string{AllAuthorizedGroupsMarker}
		}
		if len(normalizeUniqueStrings(record.Categories)) == 0 {
			record.Categories = inferModelCategories(record)
		}
		normalized = append(normalized, record)
	}

	canonical, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("整理元数据失败: %w", err)
	}
	return ParseAndValidateModelMetadata(canonical, nil)
}

// parseProjectModelMetadata accepts the provider catalog format used by v7.
// It drops non-model header rows, merges duplicate model IDs' metadata, and
// keeps the first (primary) pricing row for each model. Authorization labels
// are preserved for compatibility, but runtime group names remain configurable
// in the database and are not used as a display filter.
func parseProjectModelMetadata(raw []byte) ([]ModelMetadata, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		var envelope struct {
			Rows []map[string]json.RawMessage `json:"模型元数据v3"`
		}
		if envelopeErr := json.Unmarshal(raw, &envelope); envelopeErr != nil || envelope.Rows == nil {
			return nil, fmt.Errorf("JSON 格式错误: %w", err)
		}
		rows = envelope.Rows
	}

	unique := make([]map[string]json.RawMessage, 0, len(rows))
	indexByCallName := make(map[string]int, len(rows))
	for _, row := range rows {
		var callName string
		if value, ok := row["调用名"]; !ok || json.Unmarshal(value, &callName) != nil || strings.TrimSpace(callName) == "" {
			continue
		}
		var priceTable []map[string]string
		if value, ok := row["价格表"]; ok {
			_ = json.Unmarshal(value, &priceTable)
		}
		if len(priceTable) > 0 {
			for key, value := range map[string]string{
				"档位条件": "见价格表", "输入价": "见价格表", "输出价": "见价格表",
				"缓存读": "-", "缓存写": "-",
			} {
				if _, exists := row[key]; !exists {
					row[key], _ = json.Marshal(value)
				}
			}
			if _, exists := row["术语解释"]; !exists {
				var notes []string
				_ = json.Unmarshal(row["计价说明"], &notes)
				glossary := strings.Join(notes, "；")
				if glossary == "" {
					glossary = "见价格表"
				}
				row["术语解释"], _ = json.Marshal(glossary)
			}
		}
		key := strings.ToLower(strings.TrimSpace(callName))
		if existingIndex, ok := indexByCallName[key]; ok {
			mergeProjectMetadataRow(unique[existingIndex], row)
			continue
		}
		indexByCallName[key] = len(unique)
		unique = append(unique, row)
	}
	if len(unique) == 0 {
		return nil, fmt.Errorf("元数据不能为空")
	}
	canonical, err := json.Marshal(unique)
	if err != nil {
		return nil, fmt.Errorf("整理项目元数据失败: %w", err)
	}
	return ParseAndNormalizeIncompleteModelMetadata(canonical)
}

// ProjectModelNamesForPlatform returns model IDs from the embedded project
// catalog for platforms whose model names are not present in the LiteLLM
// pricing catalog. Seedance channels also serve Seedream models, so both
// ByteDance model families are included.
func ProjectModelNamesForPlatform(platform string) ([]string, error) {
	if strings.ToLower(strings.TrimSpace(platform)) != PlatformSeedance {
		return nil, fmt.Errorf("project model catalog is not available for platform %q", platform)
	}

	records, err := parseProjectModelMetadata(modelMetadataFile)
	if err != nil {
		return nil, fmt.Errorf("load project model metadata: %w", err)
	}

	names := make([]string, 0)
	for _, record := range records {
		if isSeedanceCatalogModel(record.CallName) {
			names = append(names, record.CallName)
		}
	}
	sort.Strings(names)
	return names, nil
}

func isSeedanceCatalogModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if slash := strings.LastIndexByte(model, '/'); slash >= 0 {
		model = model[slash+1:]
	}
	for _, prefix := range []string{"doubao-seedance", "doubao-seedream", "seedance", "seedream"} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

func mergeProjectMetadataRow(primary, duplicate map[string]json.RawMessage) {
	groups := make([]string, 0, 2)
	appendGroups := func(raw json.RawMessage) {
		if len(raw) == 0 {
			return
		}
		var many []string
		if json.Unmarshal(raw, &many) == nil {
			groups = append(groups, many...)
			return
		}
		var one string
		if json.Unmarshal(raw, &one) == nil {
			groups = append(groups, one)
		}
	}
	appendGroups(primary["授权分组"])
	appendGroups(duplicate["授权分组"])
	seen := make(map[string]struct{}, len(groups))
	merged := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		key := strings.ToLower(group)
		if group == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, group)
	}
	if encoded, err := json.Marshal(merged); err == nil {
		primary["授权分组"] = encoded
	}
	for _, key := range []string{"价格表", "计价说明"} {
		if _, exists := primary[key]; !exists {
			if value, ok := duplicate[key]; ok {
				primary[key] = value
			}
		}
	}
}

func fallbackName(name string) string {
	if name == "" {
		return "未填写调用名"
	}
	return name
}

func normalizeUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func inferModelCategories(r ModelMetadata) []string {
	text := strings.ToLower(strings.Join([]string{r.CallName, r.DisplayName, r.Capability, r.UseCases}, " "))
	containsAny := func(words ...string) bool {
		for _, word := range words {
			if strings.Contains(text, word) {
				return true
			}
		}
		return false
	}
	var categories []string
	if containsAny("文生图", "图像生成", "图片生成", "image generation", "text-to-image") {
		categories = append(categories, "图像")
	}
	if containsAny("视频生成", "生成视频", "text-to-video", "image-to-video", "video generation", "视频") {
		categories = append(categories, "视频")
	}
	if containsAny("视觉理解", "图像输入", "图片输入", "看图", "多模态", "vision") {
		categories = append(categories, "多模态")
	}
	if containsAny("代码生成", "代码执行", "代码开发", "编程", "coding", " code ") {
		categories = append(categories, "代码")
	}
	if containsAny("语音", "音频", "tts", "asr", "speech", "audio") {
		categories = append(categories, "语音")
	}
	if len(categories) == 0 {
		categories = append(categories, "文本")
	}
	return categories
}

// MarshalModelMetadata returns stable canonical JSON suitable for persistence.
func MarshalModelMetadata(records []ModelMetadata) ([]byte, error) {
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].LaunchDate != records[j].LaunchDate {
			return records[i].LaunchDate > records[j].LaunchDate
		}
		return records[i].CallName < records[j].CallName
	})
	return json.Marshal(records)
}
