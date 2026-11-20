// Package modelcatalog defines the declarative openai.codex.model_catalog.v1
// capability. Descriptors remain opaque JSON so new Codex tool fields survive.
package modelcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const Capability = "openai.codex.model_catalog.v1"
const MaxBytes = 1024 * 1024

type Config struct {
	SchemaVersion        int                        `json:"schema_version"`
	Models               []Model                    `json:"models"`
	ExplicitOnlyGroupIDs []int64                    `json:"explicit_only_group_ids,omitempty"`
	Library              catalogArray[LibraryModel] `json:"library,omitempty"`
	Presets              catalogArray[Preset]       `json:"presets,omitempty"`
	GroupCatalogs        catalogArray[GroupCatalog] `json:"group_catalogs,omitempty"`
}

// Optional arrays may be absent but not null. In particular, null must not
// discard a group policy or silently turn a preset override into inheritance.
type catalogArray[T any] []T

func (a *catalogArray[T]) UnmarshalJSON(raw []byte) error {
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return err
	}
	if rows == nil {
		return fmt.Errorf("catalog field must be an array")
	}
	result := make([]T, len(rows))
	for i, row := range rows {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(row, &fields); err != nil {
			return err
		}
		if fields == nil {
			return fmt.Errorf("catalog row %d must be an object", i)
		}
		for key, v := range fields {
			if string(v) == "null" {
				return fmt.Errorf("catalog row %d: %s cannot be null", i, key)
			}
		}
		d := json.NewDecoder(bytes.NewReader(row))
		d.DisallowUnknownFields()
		if err := d.Decode(&result[i]); err != nil {
			return err
		}
	}
	*a = result
	return nil
}

type LibraryModel struct {
	Slug            string   `json:"slug"`
	Kind            string   `json:"kind"`
	DisplayName     *string  `json:"display_name,omitempty"`
	Description     *string  `json:"description,omitempty"`
	InputModalities []string `json:"input_modalities,omitzero"`
	ImageGeneration *bool    `json:"image_generation,omitempty"`
	ImageEdit       *bool    `json:"image_edit,omitempty"`
}

type Selection struct {
	ChatModels       []string `json:"chat_models,omitzero"`
	ImageModels      []string `json:"image_models,omitzero"`
	DefaultChatModel *string  `json:"default_chat_model,omitempty"`
}

type Preset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Selection
}

type GroupCatalog struct {
	GroupID  int64  `json:"group_id"`
	PresetID string `json:"preset_id,omitempty"`
	Selection
}

type Model struct {
	ClientOrder      *int32          `json:"client_order,omitempty"`
	Hidden           bool            `json:"hidden,omitempty"`
	GroupIDs         []int64         `json:"group_ids"`
	MinClientVersion string          `json:"min_client_version,omitempty"`
	MaxClientVersion string          `json:"max_client_version,omitempty"`
	Descriptor       json.RawMessage `json:"descriptor"`
}

var catalogIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*( [A-Za-z0-9._/-]+)*$`)

// Directory entries reference the exact upstream alias, including internal spaces.
func validCatalogID(id string) bool { return len(id) <= 160 && catalogIDPattern.MatchString(id) }

var versionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:[-+][A-Za-z0-9.-]+)?$`)

func Parse(raw []byte) (*Config, error) {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("配置根节点必须是 JSON 对象")
	}
	if len(raw) > MaxBytes {
		return nil, fmt.Errorf("模型目录不能超过 1 MiB")
	}
	c := Config{SchemaVersion: 1, Models: []Model{}}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return nil, fmt.Errorf("模型目录配置: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("配置必须只有一个 JSON 对象")
	}
	if c.SchemaVersion != 1 || len(c.Models) > 128 {
		return nil, fmt.Errorf("目录版本必须是 1，模型条目不能超过 128")
	}
	seen := make(map[string]bool)
	groups := make(map[int64]bool)
	for _, id := range c.ExplicitOnlyGroupIDs {
		if id <= 0 || groups[id] {
			return nil, fmt.Errorf("仅显式发布的分组 ID 必须为正数且不重复")
		}
		groups[id] = true
	}
	for i, m := range c.Models {
		if len(m.GroupIDs) == 0 {
			return nil, fmt.Errorf("模型 %d 必须明确指定可见分组", i+1)
		}
		for _, id := range m.GroupIDs {
			if id <= 0 {
				return nil, fmt.Errorf("分组 ID 必须为正数")
			}
		}
		if m.MinClientVersion != "" && !versionPattern.MatchString(m.MinClientVersion) || m.MaxClientVersion != "" && !versionPattern.MatchString(m.MaxClientVersion) {
			return nil, fmt.Errorf("客户端版本必须采用 x.y.z 格式")
		}
		if m.MinClientVersion != "" && m.MaxClientVersion != "" && CompareVersions(m.MinClientVersion, m.MaxClientVersion) > 0 {
			return nil, fmt.Errorf("客户端版本范围倒置")
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(m.Descriptor, &fields) != nil || fields == nil {
			return nil, fmt.Errorf("模型 %d 的完整描述必须是 JSON 对象", i+1)
		}
		var slug, display, effort, shell, visibility string
		_ = json.Unmarshal(fields["slug"], &slug)
		_ = json.Unmarshal(fields["display_name"], &display)
		_ = json.Unmarshal(fields["default_reasoning_level"], &effort)
		_ = json.Unmarshal(fields["shell_type"], &shell)
		_ = json.Unmarshal(fields["visibility"], &visibility)
		if !validCatalogID(slug) {
			return nil, fmt.Errorf("能力与版本覆盖第 %d 项（分组 %v）：模型 ID %.161q 无效。请使用不超过 160 个字符的上游 ID，以英文字母或数字开头，仅允许英文字母、数字、点、下划线、斜杠、连字符和分词之间的单个空格", i+1, m.GroupIDs, slug)
		}
		if !m.Hidden {
			if strings.TrimSpace(display) == "" || shell == "" || visibility == "" {
				return nil, fmt.Errorf("模型 %d 缺少 slug、display_name、shell_type 或 visibility", i+1)
			}
			var levels []struct {
				Effort      string `json:"effort"`
				Description string `json:"description"`
			}
			if json.Unmarshal(fields["supported_reasoning_levels"], &levels) != nil || len(levels) == 0 {
				return nil, fmt.Errorf("模型 %s 缺少思考档位", slug)
			}
			found := false
			efforts := map[string]bool{}
			for _, level := range levels {
				switch level.Effort {
				case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
				default:
					return nil, fmt.Errorf("模型 %s 有不支持的思考档位", slug)
				}
				if efforts[level.Effort] {
					return nil, fmt.Errorf("模型 %s 思考档位重复", slug)
				}
				efforts[level.Effort] = true
				found = found || level.Effort == effort
			}
			if !found {
				return nil, fmt.Errorf("模型 %s 的默认思考等级不在支持列表中", slug)
			}
			// A complete import must include instructions; replacing the descriptor
			// with a short models-list record would silently drop tool behavior.
			var instructions string
			_ = json.Unmarshal(fields["base_instructions"], &instructions)
			if strings.TrimSpace(instructions) == "" && (len(fields["model_messages"]) == 0 || bytes.Equal(bytes.TrimSpace(fields["model_messages"]), []byte("null"))) {
				return nil, fmt.Errorf("模型 %s 必须导入完整 instructions/model_messages", slug)
			}
		}
		for j := 0; j < i; j++ {
			other := c.Models[j]
			if other.Slug() != slug || !rangesOverlap(m, other) {
				continue
			}
			for _, id := range m.GroupIDs {
				for _, oldID := range other.GroupIDs {
					if id == oldID {
						return nil, fmt.Errorf("模型 %s 在分组 %d 的版本范围重叠", slug, id)
					}
				}
			}
		}
		key := fmt.Sprintf("%s/%v/%s/%s", slug, m.GroupIDs, m.MinClientVersion, m.MaxClientVersion)
		if seen[key] {
			return nil, fmt.Errorf("重复模型 %s", slug)
		}
		seen[key] = true
	}
	if err := c.validateSelections(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validateSelections() error {
	kinds := map[string]string{}
	for _, m := range c.Library {
		if !validCatalogID(m.Slug) || kinds[m.Slug] != "" || (m.Kind != "chat" && m.Kind != "image") {
			return fmt.Errorf("invalid or duplicate library model %q", m.Slug)
		}
		kinds[m.Slug] = m.Kind
		seen := map[string]bool{}
		for _, mode := range m.InputModalities {
			if strings.TrimSpace(mode) == "" || seen[mode] {
				return fmt.Errorf("invalid input modality for %q", m.Slug)
			}
			seen[mode] = true
		}
	}
	check := func(s Selection) error {
		for kind, ids := range map[string][]string{"chat": s.ChatModels, "image": s.ImageModels} {
			seen := map[string]bool{}
			for _, id := range ids {
				if !validCatalogID(id) || seen[id] || (kinds[id] != "" && kinds[id] != kind) {
					return fmt.Errorf("invalid, duplicate or conflicting %s model %q", kind, id)
				}
				seen[id], kinds[id] = true, kind
			}
		}
		if s.DefaultChatModel != nil && *s.DefaultChatModel != "" && !slices.Contains(s.ChatModels, *s.DefaultChatModel) {
			return fmt.Errorf("default_chat_model must be in chat_models")
		}
		return nil
	}
	presets := map[string]bool{}
	for _, p := range c.Presets {
		if strings.TrimSpace(p.ID) == "" || presets[p.ID] || strings.TrimSpace(p.Name) == "" || p.ChatModels == nil || p.ImageModels == nil {
			return fmt.Errorf("invalid or duplicate preset %q", p.ID)
		}
		if err := check(p.Selection); err != nil {
			return err
		}
		presets[p.ID] = true
	}
	groups := map[int64]bool{}
	for _, g := range c.GroupCatalogs {
		if g.GroupID <= 0 || groups[g.GroupID] {
			return fmt.Errorf("invalid or duplicate group catalog %d", g.GroupID)
		}
		groups[g.GroupID] = true
		if g.PresetID != "" {
			if !presets[g.PresetID] || g.ChatModels != nil || g.ImageModels != nil || g.DefaultChatModel != nil {
				return fmt.Errorf("group %d must bind one known preset without overrides", g.GroupID)
			}
		} else if err := check(g.Selection); err != nil {
			return err
		}
	}
	return nil
}

// SelectionForGroup distinguishes an explicitly empty catalog from no policy.
func (c *Config) SelectionForGroup(id int64) *Selection {
	for i := range c.GroupCatalogs {
		g := &c.GroupCatalogs[i]
		if g.GroupID != id {
			continue
		}
		if g.PresetID == "" {
			return &g.Selection
		}
		for j := range c.Presets {
			if c.Presets[j].ID == g.PresetID {
				return &c.Presets[j].Selection
			}
		}
		return &Selection{}
	}
	return nil
}

func (m Model) Slug() string {
	var f struct {
		Slug string `json:"slug"`
	}
	_ = json.Unmarshal(m.Descriptor, &f)
	return f.Slug
}

func (c *Config) ExplicitOnly(groupID int64) bool {
	return slices.Contains(c.ExplicitOnlyGroupIDs, groupID) || c.SelectionForGroup(groupID) != nil
}
func (m Model) Matches(groupID int64, version string) bool {
	if (m.MinClientVersion != "" || m.MaxClientVersion != "") && !versionPattern.MatchString(version) {
		return false
	}
	if m.MinClientVersion != "" && CompareVersions(version, m.MinClientVersion) < 0 || m.MaxClientVersion != "" && CompareVersions(version, m.MaxClientVersion) > 0 {
		return false
	}
	for _, id := range m.GroupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}
func rangesOverlap(a, b Model) bool {
	return !(a.MaxClientVersion != "" && b.MinClientVersion != "" && CompareVersions(a.MaxClientVersion, b.MinClientVersion) < 0 || b.MaxClientVersion != "" && a.MinClientVersion != "" && CompareVersions(b.MaxClientVersion, a.MinClientVersion) < 0)
}

// CompareVersions compares numeric release components; alpha suffixes use the
// same descriptor as their matching client release.
func CompareVersions(a, b string) int {
	x, y := versionPattern.FindStringSubmatch(a), versionPattern.FindStringSubmatch(b)
	if len(x) == 0 || len(y) == 0 {
		return strings.Compare(a, b)
	}
	for i := 1; i <= 3; i++ {
		u, _ := strconv.Atoi(x[i])
		v, _ := strconv.Atoi(y[i])
		if u < v {
			return -1
		}
		if u > v {
			return 1
		}
	}
	return 0
}
